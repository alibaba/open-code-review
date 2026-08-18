#!/usr/bin/env python3
"""Post an OpenCodeReview result onto a GitLab merge request.

This is the CI-layer "glue" for GitLab, mirroring examples/gerrit_ci and
examples/gitflic_ci: it keeps platform-specific publishing out of the ``ocr``
binary and lives entirely in the pipeline.  It reads the JSON emitted by
``ocr review --format json`` and posts it onto the merge request as GitLab
discussions:

  - one inline discussion per comment that maps onto the diff,
  - a single summary note collecting comments that could not be placed inline,
  - a final summary note.

The script separates a transport-agnostic :func:`publish` (driven by an
injectable ``Poster`` object) from the GitLab REST transport :class:`GitLabPoster`,
so the full posting flow — including retry/backoff, rate-limit throttling,
idempotent reconciliation, incremental filtering and sticky summaries — can be
unit-tested with no network access and no wall-clock sleep cost.

Behaviors aligned with the GitHub Action ``post-review-comments.js``:

  - category/severity badge on every comment,
  - publication policy routing findings to the summary by severity/category,
  - deterministic ordering before posting,
  - detailed warning rendering,
  - backtick-safe fenced code blocks in the fallback note,
  - sticky summary (update-in-place across runs),
  - incremental mode (skip comments overlapping prior bot discussions),
  - idempotent retry (per-comment id tag; reconcile before retrying),
  - 400 line-resolution fallback (classify against the MR diff, drop the
    provably-out-of-diff ones),
  - wait-until-reset when the primary rate limit is exhausted,
  - dotenv stats output and optional severity-based job gating.

Standard library only (json, urllib) so it runs on any stock python3 image.
"""

import argparse
import hashlib
import json
import os
import random
import re
import secrets
import socket
import sys
import time
import urllib.error
import urllib.request

# Injectable so tests can run without real delays; production uses time.sleep.
_sleep = time.sleep


def log(msg):
    print(msg, file=sys.stderr)


# --------------------------------------------------------------------------- #
# Constants (mirrors scripts/github-actions/post-review-comments.js)
# --------------------------------------------------------------------------- #

# HTML marker that tags the cross-run summary note so the sticky upsert can
# find it across runs. Embedded verbatim in the summary body.
SUMMARY_MARKER = "<!-- ocr-summary -->"

# Per-run HTML marker embedded alongside SUMMARY_MARKER so the non-sticky
# summary path can locate THIS run's note (find-by-tag) instead of always
# creating a new one. Lets the pre-review anchor and the finalize phase reuse
# the same note within a run, so non-sticky mode can show a live "posting…"
# body without producing duplicate notes. Format: <!-- ocr-summary-run:TAG -->.
def summary_tag_for(run_tag):
    return "<!-- ocr-summary-run:%s -->" % run_tag


def wrap_summary_body(content, run_tag):
    """Prefix the summary body with both the cross-run marker and the per-run tag.

    Sticky mode finds the note by SUMMARY_MARKER (cross-run reuse); non-sticky
    finds it by the per-run tag (run-internal reuse, run-external new). Both
    markers are always embedded so the same body serves either matching mode.
    """
    return "%s\n%s\n%s" % (SUMMARY_MARKER, summary_tag_for(run_tag), content)

# Reason attached to comments that have no valid line range and therefore can
# never be posted as inline comments. Surfaced in the summary so every
# summary-only comment explains why it is here.
NO_LINE_REASON = "No line information provided"

# Reason attached to comments that have valid line info but could not be posted
# inline because the MR version/diff-refs endpoint was unavailable.
DIFF_REFS_UNAVAILABLE_REASON = "diff refs unavailable"

# Default IoU threshold for the incremental multi-line overlap test. Two
# multi-line comments are considered the same when their line-range IoU
# exceeds this value.
DEFAULT_OVERLAP_THRESHOLD = 0.6

# Enumerations for category/severity routing, sourced from the LLM output
# schema. Used both to validate the routing policy and to normalize metadata.
CATEGORIES = [
    "bug", "security", "performance", "maintainability",
    "test", "style", "documentation", "other",
]
# Severity rank: higher = more severe. critical=4, high=3, medium=2, low=1.
SEVERITIES = ["critical", "high", "medium", "low"]
SEVERITY_RANK = {s: len(SEVERITIES) - i for i, s in enumerate(SEVERITIES)}

# Sentinel policy object: "do not route anything". Returned by build_policy on
# any parse problem so the partition loop falls open to today's behavior.
NO_ROUTING = {"route_by_severity": False, "severity_rank": -1,
              "route_by_category": False, "categories": set()}

# Substring that flags a discussion as authored by this bot (used for
# incremental history detection). The full per-comment id is matched by a
# plain substring in _is_comment_posted, so no regex is needed here.
_BOT_MARKER = "<!-- ocr-"

# GitLab returns 400 (not 422) when a discussion position cannot be resolved.
# These patterns classify such a body as a line-resolution failure so the
# diff-inventory fallback can decide drop-vs-keep. Defensive and intentionally
# broad: a false positive only triggers classification, which on an incomplete
# inventory returns "unknown" (keeps the current fallback behavior).
LINE_RESOLUTION_PATTERNS = [
    re.compile(p, re.IGNORECASE) for p in [
        r"new_line", r"old_line", r"line_range", r"line_code", r"position",
        r"out of (the )?diff", r"could not be resolved", r"is invalid",
        r"line (range )?out of range",
    ]
]


# --------------------------------------------------------------------------- #
# Comment formatting + publication policy (pure)
# --------------------------------------------------------------------------- #


def sanitize_metadata(value):
    """Strip C0/C1 control characters from a metadata value."""
    if value is None:
        return ""
    return re.sub(r"[\x00-\x1f\x7f-\x9f]", "", str(value))


def build_badge(comment):
    """Build the ``[category · severity]`` badge (middot U+00B7).

    Byte-matches the CLI's buildBadge degeneration so review output is
    consistent across surfaces. Returns "" when neither is present, so callers
    can prepend conditionally without leaving a blank line.
    """
    category = sanitize_metadata(comment.get("category") if comment else None)
    severity = sanitize_metadata(comment.get("severity") if comment else None)
    if category and severity:
        return "[%s · %s]" % (category, severity)
    if category:
        return "[%s]" % category
    if severity:
        return "[%s]" % severity
    return ""


def build_policy(severity_threshold, categories):
    """Parse the publication policy from the raw opt-in inputs.

    Returns a normalized policy dict, or :data:`NO_ROUTING` when no routing is
    requested or any value is malformed (fail-open for the policy itself).
    """
    route_by_severity = False
    severity_rank = -1
    if severity_threshold:
        norm = str(severity_threshold).strip().lower()
        if norm in SEVERITY_RANK:
            route_by_severity = True
            severity_rank = SEVERITY_RANK[norm]

    route_by_category = False
    category_set = set()
    if categories:
        tokens = [t.strip().lower() for t in str(categories).split(",") if t.strip()]
        for t in tokens:
            if t in CATEGORIES:
                category_set.add(t)
        if category_set:
            route_by_category = True

    if not route_by_severity and not route_by_category:
        return NO_ROUTING
    return {"route_by_severity": route_by_severity, "severity_rank": severity_rank,
            "route_by_category": route_by_category, "categories": category_set}


def route_comment(comment, policy):
    """Decide whether a comment routes to the summary per the policy.

    Returns ``{"routed": True, "reason": str}`` or ``{"routed": False}``. A
    finding matches when its severity is at-or-below the threshold OR its
    category is in the category list. Unknown/malformed metadata never matches.
    """
    if not policy or (not policy.get("route_by_severity") and not policy.get("route_by_category")):
        return {"routed": False}
    cat_raw = sanitize_metadata(comment.get("category") if comment else None).strip().lower()
    sev_raw = sanitize_metadata(comment.get("severity") if comment else None).strip().lower()
    cat_known = cat_raw != "" and cat_raw in CATEGORIES
    sev_known = sev_raw != "" and sev_raw in SEVERITY_RANK

    if policy.get("route_by_severity") and sev_known and SEVERITY_RANK[sev_raw] <= policy["severity_rank"]:
        suffix = " · category %s" % cat_raw if cat_known else ""
        return {"routed": True, "reason": "Routed to summary (severity %s%s)" % (sev_raw, suffix)}
    if policy.get("route_by_category") and cat_known and cat_raw in policy["categories"]:
        suffix = " · severity %s" % sev_raw if sev_known else ""
        return {"routed": True, "reason": "Routed to summary (category %s%s)" % (cat_raw, suffix)}
    return {"routed": False}


def sort_to_send(items):
    """Deterministically order items (path -> start_line -> end_line -> index).

    Returns a NEW list. Stable ordering means identical reruns produce identical
    post sequences, which is what makes idempotency reconciliation reproducible.
    """
    decorated = []
    for orig_index, it in enumerate(items):
        c = it["comment"]
        decorated.append((
            str(c.get("path", "")),
            int(c.get("start_line") or 0),
            int(c.get("end_line") or 0),
            orig_index,
            it,
        ))
    decorated.sort(key=lambda t: (t[0], t[1], t[2], t[3]))
    return [t[4] for t in decorated]


def safe_fence(content):
    """Pick a backtick fence long enough to enclose ``content`` (min 3)."""
    text = str(content or "")
    max_ticks = 0
    for run in re.findall(r"`+", text):
        max_ticks = max(max_ticks, len(run))
    return "`" * max(3, max_ticks + 1)


def fenced_block(content, language=""):
    """Render ``content`` as a fenced code block with a safe fence."""
    text = str(content or "")
    fence = safe_fence(text)
    block = fence + language + "\n" + text
    if not text.endswith("\n"):
        block += "\n"
    return block + fence


def format_comment(comment, comment_id=None):
    """Assemble the visible inline-discussion body.

    The per-comment id tag (when provided) is prepended as an HTML comment so
    :func:`reconcile_posted_id` can match it back on retry. The category/severity
    badge is then prepended on its own line. The suggestion uses GitLab's
    ``suggestion:-0+0`` info string (kept at the fixed triple-backtick form so
    the "Apply suggestion" button keeps working).
    """
    body = ""
    if comment_id:
        body += "<!-- %s -->\n" % comment_id
    badge = build_badge(comment)
    if badge:
        body += badge + "\n"
    body += comment.get("content", "") or ""
    suggestion = comment.get("suggestion_code", "")
    existing = comment.get("existing_code", "")
    if suggestion and existing:
        span = comment_span(comment)
        suggestion_offset = span["end"] - span["start"] if span is not None else 0
        body += "\n\n**Suggestion:**\n"
        body += "```suggestion:-%d+0\n%s\n```" % (suggestion_offset, suggestion)
    return body


def format_comment_fallback(comment, reason=None):
    """Format a comment for fallback (non-inline) display in a note.

    Uses ``<details><summary>`` HTML so the suggested change is collapsible.
    The Before/After blocks use :func:`fenced_block` so code containing
    backticks still renders.
    """
    path = comment.get("path", "unknown")
    start_line = comment.get("start_line", 0)
    end_line = comment.get("end_line", 0)
    content = comment.get("content", "")

    md = ""
    badge = build_badge(comment)
    if badge:
        md += badge + "\n"
    md += "### 📄 `%s`" % path
    if start_line and end_line:
        md += " (L%d-L%d)" % (start_line, end_line)
    md += "\n\n"
    if reason:
        md += "⚠️ Could not be posted inline: %s\n\n" % reason
    md += content

    existing = comment.get("existing_code", "")
    suggestion = comment.get("suggestion_code", "")
    if suggestion and existing:
        md += "\n\n<details><summary>💡 Suggested Change</summary>\n\n"
        md += "**Before:**\n" + fenced_block(existing) + "\n\n"
        md += "**After:**\n" + fenced_block(suggestion) + "\n\n"
        md += "</details>"
    return md


def format_warning_entry(w):
    """Format a single warning as a compact ``file (type): message`` bullet."""
    if w is None:
        return ""
    if isinstance(w, str):
        return w
    if isinstance(w, dict):
        prefix_parts = []
        if w.get("file") and str(w["file"]) != "":
            prefix_parts.append("`%s`" % w["file"])
        if w.get("type") and str(w["type"]) != "":
            prefix_parts.append("(`%s`)" % w["type"])
        prefix = " ".join(prefix_parts)
        msg = str(w.get("message", "")) if w.get("message") is not None else ""
        if prefix and msg:
            return "%s: %s" % (prefix, msg)
        if msg:
            return msg
        if prefix:
            return prefix
        try:
            return json.dumps(w)
        except (TypeError, ValueError):
            return str(w)
    return str(w)


def format_warnings(warnings):
    """Render warnings as a bulleted list under a heading ("" when empty)."""
    if not warnings:
        return ""
    body = "\n\n---\n\n⚠️ **Warnings:**"
    for w in warnings:
        body += "\n- %s" % format_warning_entry(w)
    return body


def build_summary_body(total, inline, summary, skipped, routed, failed, warnings):
    """Merged summary header. Counts are mutually exclusive and sum to total."""
    body = "🔍 **OpenCodeReview** found **%d** issue(s) in this MR." % total
    if total > 0:
        body += "\n- ✅ Successfully posted inline: %d comment(s)" % inline
        if summary > 0:
            body += "\n- 📝 In summary (no line info): %d comment(s)" % summary
        if routed > 0:
            body += "\n- 📋 Routed to summary by policy: %d comment(s)" % routed
        if skipped > 0:
            body += "\n- ⏭️ Skipped (overlap with history): %d comment(s)" % skipped
        if failed > 0:
            body += "\n- ❌ Failed to post inline: %d comment(s)" % failed
    if warnings:
        body += "\n\n⚠️ %d warning(s) occurred during review." % len(warnings)
    return body


def build_pre_review_summary_body(total, no_line, routed, warnings):
    """Summary body shown in the sticky anchor while inline comments post."""
    body = "🔍 **OpenCodeReview** found **%d** issue(s) in this MR." % total
    if total > 0:
        body += "\n- ⏳ _Posting review comments…_"
        if routed:
            body += "\n- 📋 Routed to summary by policy: %d comment(s)" % len(routed)
    if warnings:
        body += "\n\n⚠️ %d warning(s) occurred during review." % len(warnings)
    body += format_summary_comments(no_line)
    body += format_summary_comments(routed)
    body += format_warnings(warnings)
    return body


def format_summary_comments(items):
    """Render a list of ``{comment, reason}`` as a continuous block."""
    body = ""
    for item in items:
        body += "\n\n---\n\n"
        body += format_comment_fallback(item["comment"], item.get("reason"))
    return body


# --------------------------------------------------------------------------- #
# Incremental overlap helpers (pure)
# --------------------------------------------------------------------------- #


def resolve_threshold(threshold):
    try:
        n = float(threshold)
    except (TypeError, ValueError):
        return DEFAULT_OVERLAP_THRESHOLD
    return n if 0 < n <= 1 else DEFAULT_OVERLAP_THRESHOLD


def _num(v):
    if v is None or v == "":
        return None
    try:
        n = int(v)
    except (TypeError, ValueError):
        return None
    return n if n >= 1 else None


def comment_span(comment):
    """Resolve a review comment into a line span tagged single/multi-line."""
    s = _num(comment.get("start_line"))
    e = _num(comment.get("end_line"))
    if s is None and e is None:
        return None
    if s is not None and e is not None and s != e:
        return {"start": min(s, e), "end": max(s, e), "multiline": True}
    single = e if e is not None else s
    return {"start": single, "end": single, "multiline": False}


def position_span(position):
    """Resolve a GitLab discussion position into a line span (or None)."""
    if not position:
        return None
    line_range = position.get("line_range")
    if line_range:
        start = line_range.get("start") or {}
        end = line_range.get("end") or {}
        s = _num(start.get("new_line"))
        e = _num(end.get("new_line"))
        if s is not None and e is not None:
            if s != e:
                return {"start": min(s, e), "end": max(s, e), "multiline": True}
            return {"start": s, "end": s, "multiline": False}
        return None
    nl = _num(position.get("new_line"))
    if nl is not None:
        return {"start": nl, "end": nl, "multiline": False}
    return None


def same_comment_span(cur, other, threshold):
    """IoU-based same-comment predicate (strict: equal-to-threshold is NOT a match)."""
    if cur["multiline"] != other["multiline"]:
        return False
    if not cur["multiline"]:
        return cur["start"] == other["start"]
    overlap = min(cur["end"], other["end"]) - max(cur["start"], other["start"]) + 1
    if overlap <= 0:
        return False
    union = (cur["end"] - cur["start"] + 1) + (other["end"] - other["start"] + 1) - overlap
    if union <= 0:
        return False
    return overlap / float(union) > threshold


def overlaps_history(comment, cur_span, history, threshold):
    """True when ``cur_span`` overlaps any prior bot discussion on the same path."""
    t = resolve_threshold(threshold)
    path = comment.get("path")
    for h in history:
        if h.get("path") != path:
            continue
        if same_comment_span(cur_span, h["span"], t):
            return True
    return False


# --------------------------------------------------------------------------- #
# 400 line-resolution fallback helpers (pure)
# --------------------------------------------------------------------------- #


def is_line_resolution_failure(error_body):
    """Classify a 400 error body as a position/line-resolution failure."""
    if not error_body:
        return False
    text = str(error_body).lower()
    return any(p.search(text) for p in LINE_RESOLUTION_PATTERNS)


def parse_diff_hunk_inventory(patch):
    """Parse a unified-diff patch into per-hunk new-file line ranges.

    Returns ``(ranges, complete)``. ``complete`` is False when the patch looks
    truncated (observed hunk length != declared), so the caller declines to
    prove a comment out-of-diff on partial data.
    """
    if not patch:
        return [], False
    ranges = []
    hunk_header_re = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@")
    lines = str(patch).split("\n")
    current = None
    saw_hunk = False
    complete = True

    def flush():
        nonlocal complete
        if not current:
            return
        if current["observed"] != current["expected"]:
            complete = False
        if current["end"] >= current["start"]:
            ranges.append({"start": current["start"], "end": current["end"]})

    for line in lines:
        match = hunk_header_re.match(line)
        if match:
            flush()
            start = int(match.group(1))
            expected = 1 if match.group(2) is None else int(match.group(2))
            current = {"start": start, "end": start - 1, "next": start,
                       "expected": expected, "observed": 0}
            saw_hunk = True
            continue
        if not current:
            continue
        if line.startswith("\\") or line.startswith("-"):
            continue
        if line.startswith("+") or line.startswith(" "):
            current["end"] = current["next"]
            current["next"] += 1
            current["observed"] += 1
    flush()
    return ranges, (saw_hunk and complete)


def classify_comment_against_diff(comment, diff):
    """Tri-state: ``"valid"`` | ``"invalid"`` | ``"unknown"``.

    ``invalid`` is provably outside the diff; ``unknown`` means we could not
    prove anything (truncated/empty inventory), so the caller keeps the
    pre-existing fallback behavior.
    """
    if not diff or not diff.get("complete"):
        return "unknown"
    path = comment.get("path")
    known = diff.get("known") or set()
    if path not in known:
        return "invalid"
    files = diff.get("files") or {}
    ranges = files.get(path)
    if not ranges:
        return "unknown"
    start_line = _num(comment.get("start_line"))
    end_line = _num(comment.get("end_line"))
    if end_line is None:
        return "unknown"
    if start_line is None:
        start_line = end_line
    if start_line > end_line:
        return "invalid"
    for r in ranges:
        if start_line >= r["start"] and end_line <= r["end"]:
            return "valid"
    return "invalid"


# --------------------------------------------------------------------------- #
# GitLab REST transport
# --------------------------------------------------------------------------- #


def _get_header(headers, name):
    """Case-insensitive header lookup."""
    if name in headers:
        val = headers[name]
    elif name.lower() in headers:
        val = headers[name.lower()]
    else:
        return None
    return str(val).strip() if val is not None else None


def _parse_rate_limit_header(headers, name):
    val = _get_header(headers, name)
    if val is None:
        return None
    try:
        return int(val)
    except (ValueError, TypeError):
        return None


def _api_request_once(api_base, token, auth_header, endpoint, data=None, method="GET"):
    """Single GitLab API attempt. Returns a rich result dict (no retry)."""
    url = "%s%s" % (api_base, endpoint)
    headers = {auth_header: token, "Content-Type": "application/json"}
    body = json.dumps(data).encode("utf-8") if data is not None else None
    req = urllib.request.Request(url, data=body, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req) as resp:
            raw = resp.read().decode("utf-8", "replace")
            try:
                resp_data = json.loads(raw) if raw else None
            except ValueError:
                resp_data = None
            h = dict(resp.headers) if hasattr(resp, "headers") else {}
            remaining = _parse_rate_limit_header(h, "RateLimit-Remaining")
            limit = _parse_rate_limit_header(h, "RateLimit-Limit")
            if remaining is not None and limit is not None:
                log("RateLimit: %s/%s remaining for %s" % (remaining, limit, endpoint))
            return {"success": True, "data": resp_data,
                    "http_status": getattr(resp, "status", 200),
                    "headers": h, "error_body": None,
                    "is_rate_limit_exhausted": False, "is_transient": False,
                    "is_network_error": False, "rate_limit_remaining": remaining}
    except urllib.error.HTTPError as e:
        error_body = e.read().decode("utf-8", "replace")
        h = dict(e.headers) if hasattr(e, "headers") else {}
        is_rate = e.code == 429 or (
            e.code == 403 and any(kw in error_body.lower() for kw in
                                  ["retry later", "rate limit", "too many requests", "abuse"])
        )
        is_transient = (500 <= e.code < 600) or e.code == 408
        rl_remaining = _parse_rate_limit_header(h, "RateLimit-Remaining")
        return {"success": False, "data": None, "http_status": e.code,
                "headers": h, "error_body": error_body,
                "is_rate_limit_exhausted": is_rate, "is_transient": is_transient,
                "is_network_error": False, "rate_limit_remaining": rl_remaining}
    except urllib.error.URLError as e:
        if isinstance(e.reason, (socket.timeout, TimeoutError)):
            return {"success": False, "data": None, "http_status": None,
                    "headers": {}, "error_body": "network timeout",
                    "is_rate_limit_exhausted": False, "is_transient": False,
                    "is_network_error": True, "rate_limit_remaining": None}
        return {"success": False, "data": None, "http_status": None,
                "headers": {}, "error_body": str(e.reason),
                "is_rate_limit_exhausted": False, "is_transient": True,
                "is_network_error": True, "rate_limit_remaining": None}


def _compute_retry_delay(resp, attempt, config):
    """Compute a retry delay (seconds) for a failed attempt, or None if not retryable.

    Order: Retry-After header -> wait-until-reset (primary limit exhausted) ->
    exponential backoff. Cap applied before ±25% jitter, matching the prior
    implementation so existing timing tests stay exact.
    """
    is_rate = resp.get("is_rate_limit_exhausted")
    is_transient = resp.get("is_transient")
    if not is_rate and not is_transient:
        return None
    headers = resp.get("headers") or {}
    max_retry_delay = config["max_retry_delay"]
    retry_base_delay = config["retry_base_delay"]
    transient_base_delay = config["transient_base_delay"]

    retry_after = _get_header(headers, "Retry-After")
    if retry_after:
        try:
            base = float(retry_after)
        except ValueError:
            base = retry_base_delay * (2 ** attempt)
    elif is_rate:
        remaining = _parse_rate_limit_header(headers, "RateLimit-Remaining")
        reset = _get_header(headers, "RateLimit-Reset")
        if reset is not None and remaining == 0:
            try:
                rv = float(reset)
            except ValueError:
                rv = None
            if rv is not None:
                # GitLab documents RateLimit-Reset as seconds-to-reset; some
                # proxies emit an epoch. Defensively handle both.
                if rv > 1e9:
                    base = max(0.0, rv - time.time())
                else:
                    base = rv
            else:
                base = retry_base_delay * (2 ** attempt)
        else:
            base = retry_base_delay * (2 ** attempt)
    else:
        base = transient_base_delay * (2 ** attempt)

    delay = min(base, max_retry_delay)
    delay = delay * (0.75 + random.random() * 0.5)
    return delay


def _api_request_with_retry(api_base, token, auth_header, config, endpoint,
                            data=None, method="GET"):
    """Make a GitLab API request with retry on rate-limit and transient errors.

    Used for reads and note writes (discussions go through the idempotency-aware
    poster method). Returns the rich result dict from :func:`_api_request_once`.
    """
    max_retries = config["max_retries"]
    last = None
    for attempt in range(max_retries + 1):
        last = _api_request_once(api_base, token, auth_header, endpoint, data, method)
        if last["success"]:
            return last
        retryable = last["is_rate_limit_exhausted"] or last["is_transient"]
        if not (retryable and attempt < max_retries):
            return last
        delay = _compute_retry_delay(last, attempt, config)
        if delay is not None:
            rl_info = ""
            rl_remaining = last.get("rate_limit_remaining")
            if rl_remaining is not None:
                rl_info = " (RateLimit-Remaining: %s)" % rl_remaining
            reason = "rate limit" if last["is_rate_limit_exhausted"] else "transient error (HTTP %s)" % last.get("http_status")
            log("%s hit for %s, retrying in %.1fs (attempt %d/%d)%s"
                % (reason, endpoint, delay, attempt + 1, max_retries, rl_info))
            _sleep(delay)
    return last


def _maybe_reached_server(resp):
    """True when a failed request may still have landed on the server."""
    status = resp.get("http_status")
    if status is None:
        return True
    return status >= 500 or status == 408


def fetch_diff_refs(api_base, token, auth_header, config, expected_shas=None):
    """Fetch MR diff refs from the ``/versions`` endpoint (or None on failure).

    When ``expected_shas`` is provided (e.g. ``{"head": ..., "base": ...}``,
    sourced from the OCR result manifest), select the version whose
    ``head_commit_sha`` (and, when both are supplied, ``base_commit_sha``)
    matches what OCR actually reviewed. Without a match — or when
    ``expected_shas`` is empty — fall back to the newest version by
    ``created_at``.

    Picking the wrong version is the root cause of GitLab's
    ``:line_code=>["can't be blank", "must be a valid line code"]`` 400: the
    MR can be force-pushed or gain a follow-up commit after OCR ran, so
    ``versions[0]`` no longer describes the diff OCR commented on, and every
    inline position then fails to resolve. Matching on the manifest's resolved
    SHAs keeps the position's ``base_sha``/``head_sha`` aligned with the diff
    OCR actually reviewed.
    """
    resp = _api_request_with_retry(api_base, token, auth_header, config,
                                   "/versions", method="GET")
    if not (resp and resp.get("success")):
        return None
    versions = resp.get("data") or []
    if not versions:
        return None
    chosen = _pick_version(versions, expected_shas)
    return {"base_sha": chosen.get("base_commit_sha", ""),
            "start_sha": chosen.get("start_commit_sha", ""),
            "head_sha": chosen.get("head_commit_sha", "")}


def _pick_version(versions, expected_shas):
    """Select the MR diff version that matches what OCR reviewed.

    Prefers an exact ``head_commit_sha`` (and ``base_commit_sha`` when both
    are supplied) match; logs a warning and falls back to the newest version
    by ``created_at`` when no version matches, so inline posting is still
    attempted (the per-comment 400 line-resolution fallback handles any
    residual mismatch). Without ``expected_shas``, returns the newest
    version explicitly sorted by ``created_at`` (the API does not guarantee
    insertion order across GitLab versions / mirrors).
    """
    expected_head = (expected_shas or {}).get("head")
    expected_base = (expected_shas or {}).get("base")
    if expected_head:
        for v in versions:
            if v.get("head_commit_sha") != expected_head:
                continue
            if expected_base and v.get("base_commit_sha") != expected_base:
                continue
            return v
        log("[diff-refs] no MR version matches OCR reviewed SHAs "
            "(expected head=%s base=%s); falling back to newest version."
            % (expected_head, expected_base or "(any)"))
    return sorted(versions, key=lambda v: v.get("created_at") or "",
                   reverse=True)[0]


def _extract_expected_shas(result):
    """Pull the SHAs OCR actually reviewed out of the result manifest.

    Returns ``{"head": ..., "base": ...}`` (either key may be absent) or
    ``{}`` when the manifest is absent (older OCR or non-range mode), in
    which case ``fetch_diff_refs`` falls back to the newest MR version.
    """
    manifest_input = (result.get("manifest") or {}).get("input") or {}
    shas = {}
    head = (manifest_input.get("resolved_head") or "").strip()
    base = (manifest_input.get("resolved_base") or "").strip()
    if head:
        shas["head"] = head
    if base:
        shas["base"] = base
    return shas


class GitLabPoster:
    """Transport object encapsulating all GitLab REST operations.

    Discussions are posted through :meth:`post_discussion`, which owns an
    idempotency-aware retry loop: on a maybe-reached-server failure it queries
    existing discussions for the embedded per-comment id before retrying, so a
    5xx/timeout that actually landed never produces a duplicate. Note/summary
    operations use the simpler :func:`_api_request_with_retry` because the
    sticky upsert (find-by-marker then update) de-dups across runs.
    """

    MAX_PAGES = 10

    def __init__(self, api_base, token, auth_header, config):
        self.api_base = api_base
        self.token = token
        self.auth_header = auth_header
        self.config = config
        self._mr_url = None
        self._mr_url_fetched = False

    # ---- low-level ----

    def _retry(self, endpoint, data=None, method="GET"):
        return _api_request_with_retry(self.api_base, self.token, self.auth_header,
                                       self.config, endpoint, data, method)

    def _read_with_pacing(self, endpoint):
        """Read with proactive pacing, mirroring the GitHub Action's readWithPacing.

        Reads are cheaper than writes but still consume the primary rate limit
        and can trip the secondary limit when issued in a tight loop (a large MR
        pages through list_notes/list_discussions/get_mr_diffs repeatedly). After
        a successful read, sleep ``read_success_delay`` — or
        ``read_low_remaining_spacing`` (a longer spacing) when the response's
        ``RateLimit-Remaining`` is at/below ``rate_limit_threshold``. Retry on
        rate-limit/transient errors is handled by the underlying ``_retry``.
        """
        resp = self._retry(endpoint, method="GET")
        if not resp.get("success"):
            return resp
        remaining = resp.get("rate_limit_remaining")
        threshold = self.config.get("rate_limit_threshold", 0)
        if threshold > 0 and remaining is not None and remaining <= threshold:
            _sleep(self.config.get("read_low_remaining_spacing", 5.0))
        else:
            _sleep(self.config.get("read_success_delay", 0.5))
        return resp

    def mr_url(self):
        """Best-effort MR web URL (cached). None when unavailable."""
        if self._mr_url_fetched:
            return self._mr_url
        self._mr_url_fetched = True
        resp = self._retry("", method="GET")
        if resp.get("success") and isinstance(resp.get("data"), dict):
            self._mr_url = resp["data"].get("web_url")
        return self._mr_url

    # ---- notes ----

    def post_note(self, body):
        resp = self._retry("/notes", data={"body": body}, method="POST")
        url = None
        if resp.get("success") and isinstance(resp.get("data"), dict):
            url = resp["data"].get("web_url")
        return {"success": resp["success"],
                "rate_limit_remaining": resp["rate_limit_remaining"],
                "is_rate_limit_exhausted": resp["is_rate_limit_exhausted"],
                "url": url, "id": (resp.get("data") or {}).get("id") if isinstance(resp.get("data"), dict) else None}

    def update_note(self, note_id, body):
        resp = self._retry("/notes/%s" % note_id, data={"body": body}, method="PUT")
        url = None
        if resp.get("success") and isinstance(resp.get("data"), dict):
            url = resp["data"].get("web_url")
        return {"success": resp["success"], "url": url,
                "http_status": resp.get("http_status"),
                "error_body": resp.get("error_body"),
                "rate_limit_remaining": resp.get("rate_limit_remaining")}

    def list_notes(self):
        """Return all MR notes, or None when the read failed."""
        all_notes = []
        page = 1
        while page <= self.MAX_PAGES:
            resp = self._read_with_pacing("/notes?per_page=100&page=%d" % page)
            if not resp.get("success"):
                return None
            data = resp.get("data") or []
            all_notes.extend(data)
            if len(data) < 100:
                break
            page += 1
        return all_notes

    def list_discussions(self):
        """Return all MR discussions, or None when the read failed."""
        all_disc = []
        page = 1
        while page <= self.MAX_PAGES:
            resp = self._read_with_pacing("/discussions?per_page=100&page=%d" % page)
            if not resp.get("success"):
                return None
            data = resp.get("data") or []
            all_disc.extend(data)
            if len(data) < 100:
                break
            page += 1
        return all_disc

    # ---- discussions (idempotent) ----

    def _is_comment_posted(self, comment_id):
        """True/False when the read succeeds; None when the read API failed."""
        discussions = self.list_discussions()
        if discussions is None:
            return None
        needle = "<!-- %s -->" % comment_id
        for d in discussions:
            for note in (d.get("notes") or []):
                if needle in (note.get("body") or ""):
                    return True
        return False

    def post_discussion(self, discussion, comment_id=None):
        """POST a discussion with idempotent reconciliation on ambiguous failures."""
        max_retries = self.config["max_retries"]
        last = None
        for attempt in range(max_retries + 1):
            last = _api_request_once(self.api_base, self.token, self.auth_header,
                                     "/discussions", data=discussion, method="POST")
            if last["success"]:
                return self._success_result(last)
            retryable = last["is_rate_limit_exhausted"] or last["is_transient"]
            will_retry = retryable and attempt < max_retries
            if not will_retry:
                break
            delay = _compute_retry_delay(last, attempt, self.config)
            if delay is not None:
                _sleep(delay)
            # Reconcile before retrying only when the request may have landed.
            if _maybe_reached_server(last) and comment_id:
                posted = self._is_comment_posted(comment_id)
                if posted is True:
                    log("Comment %s already posted; treating as success." % comment_id)
                    return self._success_result(last, reconciled=True)
                if posted is None:
                    # Read API unavailable — cannot tell. Skip retry to avoid a
                    # duplicate; surface as a failure with an explicit reason.
                    return self._failed_result(last, "idempotency check unavailable (read API failed)")
                # False → genuinely absent; continue to the retry.
        return self._failed_result(last)

    @staticmethod
    def _success_result(resp, reconciled=False):
        return {"success": True, "reconciled": reconciled,
                "rate_limit_remaining": resp.get("rate_limit_remaining"),
                "is_rate_limit_exhausted": False}

    @staticmethod
    def _failed_result(resp, reason=None):
        message = resp.get("error_body") or "POST /discussions failed"
        if reason:
            message = "%s [%s]" % (message, reason)
        return {"success": False, "reconciled": False,
                "rate_limit_remaining": resp.get("rate_limit_remaining"),
                "is_rate_limit_exhausted": resp.get("is_rate_limit_exhausted"),
                "http_status": resp.get("http_status"),
                "error_body": resp.get("error_body"),
                "failed_reason": message}

    # ---- diff inventory (400 line-resolution fallback) ----

    def get_mr_diffs(self):
        """Build a diff inventory from ``GET /merge_requests/:iid/diffs``."""
        known = set()
        files = {}
        complete = True
        per_page = 100
        max_pages = 30
        page = 1
        while page <= max_pages:
            resp = self._read_with_pacing("/diffs?per_page=%d&page=%d" % (per_page, page))
            if not resp.get("success"):
                complete = False
                break
            data = resp.get("data") or []
            for entry in data:
                new_path = entry.get("new_path") or entry.get("old_path")
                if not new_path:
                    continue
                known.add(new_path)
                patch = entry.get("diff")
                if patch:
                    ranges, ok = parse_diff_hunk_inventory(patch)
                    if ok:
                        files[new_path] = ranges
            if len(data) < per_page:
                break
            page += 1
        if page > max_pages:
            complete = False
            log("[400-fallback] MR changed-file list exceeded %d files; inventory incomplete." % (max_pages * per_page))
        if not known:
            complete = False
            log("[400-fallback] MR diff list came back empty; treating inventory as incomplete.")
        return {"files": files, "known": known, "complete": complete}


def make_poster(api_base, token, auth_header, config):
    """Return a :class:`GitLabPoster` bound to the given MR endpoint."""
    return GitLabPoster(api_base, token, auth_header, config)


class DryRunPoster:
    """A Poster that prints instead of touching the network."""

    def _print(self, kind, discussion):
        if "position" in discussion:
            pos = discussion["position"]
            location = "%s:%s" % (pos.get("new_path", ""), pos.get("new_line", ""))
        else:
            location = "general"
        print("--- dry-run %s [%s] ---\n%s\n" % (kind, location, discussion.get("body", "")))

    def post_note(self, body):
        self._print("note", {"body": body})
        return {"success": True, "rate_limit_remaining": None,
                "is_rate_limit_exhausted": False, "url": None, "id": None}

    def update_note(self, note_id, body):
        print("--- dry-run update_note[%s] ---\n%s\n" % (note_id, body))
        return {"success": True, "url": None, "http_status": None,
                "error_body": None, "rate_limit_remaining": None}

    def list_notes(self):
        return []

    def list_discussions(self):
        return []

    def post_discussion(self, discussion, comment_id=None):
        self._print("discussion", discussion)
        return {"success": True, "reconciled": False, "rate_limit_remaining": None,
                "is_rate_limit_exhausted": False}

    def get_mr_diffs(self):
        return {"files": {}, "known": set(), "complete": False}

    def mr_url(self):
        return None


def make_dry_run_poster():
    """Return a :class:`DryRunPoster`."""
    return DryRunPoster()


# --------------------------------------------------------------------------- #
# Summary note helpers (sticky vs new)
# --------------------------------------------------------------------------- #


def _truncate_error(body, limit=300):
    """Collapse whitespace and cap length so log lines stay readable."""
    if not body:
        return ""
    s = str(body).replace("\r", " ").replace("\n", " ")
    return s if len(s) <= limit else s[:limit] + "..."


def find_summary_note(notes, tag=None):
    """Return the newest note carrying the summary marker (or per-run tag).

    ``tag`` selects the matching needle: when provided, match this run's
    per-run ``SUMMARY_TAG`` (used by non-sticky mode so the anchor and finalize
    phases reuse the same note within a run); otherwise match the cross-run
    :data:`SUMMARY_MARKER` (sticky mode). Iterates newest-first so non-sticky
    per-run matching picks the note just created in the anchor phase, not a
    stale one from a prior run that happens to carry the same marker.
    """
    if not notes:
        return None
    needle = tag or SUMMARY_MARKER
    for note in reversed(notes):
        body = note.get("body") or ""
        if needle in body:
            return note
    return None


def upsert_summary(poster, body, sticky, tag=None):
    """Find-or-create/update a summary note.

    Sticky matches the cross-run :data:`SUMMARY_MARKER`; non-sticky matches
    this run's ``tag`` (per-run), so retries within a run update the note in
    place rather than creating duplicates. Returns the note URL (best-effort)
    or None when the read API is unavailable and a write would risk duplicating.
    """
    notes = poster.list_notes()
    if notes is None:
        log("[summary] cannot list notes for %s upsert; skipping to avoid duplicate."
            % ("sticky" if sticky else "non-sticky"))
        return None
    needle = SUMMARY_MARKER if sticky else tag
    existing = find_summary_note(notes, tag=needle) if needle else None
    if existing is not None:
        resp = poster.update_note(existing.get("id"), body)
        if not (resp or {}).get("success"):
            log("[summary] failed to update note %s (HTTP %s): %s; stale content may remain."
                % (existing.get("id"), (resp or {}).get("http_status"),
                   _truncate_error((resp or {}).get("error_body"))))
        return (resp or {}).get("url") or existing.get("web_url")
    resp = poster.post_note(body)
    return (resp or {}).get("url")


def ensure_summary_anchor(poster, body, sticky, tag=None):
    """Phase 1: find-or-create the summary note before posting discussions.

    Runs in both sticky and non-sticky modes so the pre-review body ("⏳
    Posting review comments…") is visible while inline comments post. Sticky
    reuses the cross-run summary note; non-sticky reuses this run's note (so
    :func:`finalize_summary` updates it instead of creating a duplicate).
    Returns the note id (existing or newly created), or None when the read API
    is unavailable so the caller defers to :func:`finalize_summary`.
    """
    notes = poster.list_notes()
    if notes is None:
        log("[summary] cannot check for existing summary before review; skipping anchor.")
        return None
    needle = SUMMARY_MARKER if sticky else tag
    existing = find_summary_note(notes, tag=needle) if needle else None
    if existing is not None:
        return existing.get("id")
    resp = poster.post_note(body)
    return (resp or {}).get("id")


def finalize_summary(poster, body, sticky, anchor_id, tag=None):
    """Phase 2: write the final summary body to the anchored note (or upsert)."""
    if anchor_id is not None:
        resp = poster.update_note(anchor_id, body)
        if (resp or {}).get("success"):
            return (resp or {}).get("url")
        log("[summary] failed to update anchored note %s (HTTP %s): %s; falling back to upsert."
            % (anchor_id, (resp or {}).get("http_status"),
               _truncate_error((resp or {}).get("error_body"))))
    return upsert_summary(poster, body, sticky, tag=tag)


# --------------------------------------------------------------------------- #
# Transport-agnostic publishing
# --------------------------------------------------------------------------- #


def publish(result, diff_refs, poster, config, sleep=_sleep):
    """Post the review result via the injectable ``poster`` object.

    Returns a stats dict with mutually-exclusive counts::

        {total, inline, summary, routed, skipped, failed, summary_url}
    """
    comments = result.get("comments") or []
    warnings = result.get("warnings") or []
    sticky = config.get("sticky_summary", True)
    incremental = config.get("incremental", False)
    overlap_threshold = config.get("incremental_overlap_threshold", DEFAULT_OVERLAP_THRESHOLD)
    policy = build_policy(config.get("route_severity_below", ""),
                          config.get("route_categories", ""))
    success_delay = config["success_delay"]
    failure_delay = config["failure_delay"]
    rate_limit_threshold = config["rate_limit_threshold"]
    stats = {"total": len(comments), "inline": 0, "summary": 0,
             "routed": 0, "skipped": 0, "failed": 0, "summary_url": None}

    # No comments: LGTM summary (sticky-aware).
    if not comments:
        message = result.get("message", "No comments generated. Looks good to me.")
        body = wrap_summary_body("✅ **OpenCodeReview**: %s" % message,
                                 config.get("run_tag", "0-0"))
        stats["summary_url"] = upsert_summary(poster, body, sticky,
                                             tag=summary_tag_for(config.get("run_tag", "0-0")))
        return stats

    # Partition: inline / no-line / routed.
    inline_items = []
    no_line = []
    routed = []
    for comment in comments:
        path = comment.get("path", "")
        start_line = comment.get("start_line", 0)
        end_line = comment.get("end_line", 0)
        # Inline posting needs a valid end_line (it becomes the GitLab position's
        # new_line). A start_line-only comment cannot be positioned and must land
        # in the summary (no_line), not be misclassified as a posting failure.
        has_line = bool(end_line and end_line >= 1)
        if not has_line or not path:
            no_line.append({"comment": comment, "reason": NO_LINE_REASON})
            continue
        route = route_comment(comment, policy)
        if route["routed"]:
            routed.append({"comment": comment, "reason": route["reason"]})
            continue
        cid = new_comment_id(config.get("run_tag", "0-0"))
        inline_items.append({"comment": comment, "body": format_comment(comment, cid),
                             "id": cid})

    stats["summary"] = len(no_line)
    stats["routed"] = len(routed)

    # Deterministic order so reruns reproduce the same post sequence.
    inline_items = sort_to_send(inline_items)

    # Incremental filtering: drop comments overlapping prior bot discussions.
    if incremental and inline_items:
        history = load_incremental_history(poster)
        if history is not None:
            kept = []
            for it in inline_items:
                span = comment_span(it["comment"])
                if span and overlaps_history(it["comment"], span, history, overlap_threshold):
                    stats["skipped"] += 1
                    continue
                kept.append(it)
            if stats["skipped"]:
                log("[incremental] skipped %d overlapping comment(s); %d to post."
                    % (stats["skipped"], len(kept)))
            inline_items = kept

    # Summary anchor (pre-review body) — created before discussions so it pins
    # above them on the first run. Runs in both sticky and non-sticky modes: the
    # pre-review body ("⏳ Posting review comments…") is visible while inline
    # comments post. The per-run SUMMARY_TAG lets non-sticky mode reuse this
    # same note at finalize instead of creating a duplicate.
    run_tag = config.get("run_tag", "0-0")
    anchor_id = None
    anchor_body = wrap_summary_body(
        build_pre_review_summary_body(stats["total"], no_line, routed, warnings),
        run_tag,
    )
    anchor_id = ensure_summary_anchor(poster, anchor_body, sticky,
                                      tag=summary_tag_for(run_tag))

    # Post inline discussions.
    diff_cache = {"diff": None, "fetched": False}
    failed_comments = []
    for it in inline_items:
        comment = it["comment"]
        path = comment.get("path", "")
        path_sha1 = hashlib.sha1(path.encode("utf-8")).hexdigest()
        start_line = comment.get("start_line", 0)
        end_line = comment.get("end_line", 0)
        if not path or not end_line:
            failed_comments.append({"comment": comment, "reason": NO_LINE_REASON})
            continue
        if not diff_refs:
            failed_comments.append({"comment": comment, "reason": DIFF_REFS_UNAVAILABLE_REASON})
            continue
        discussion = {
            "body": it["body"],
            "position": {
                "position_type": "text",
                "new_path": path,
                "old_path": path,
                "new_line": end_line,
                "base_sha": diff_refs["base_sha"],
                "start_sha": diff_refs["start_sha"],
                "head_sha": diff_refs["head_sha"],
                "line_range": {
                    "start": {
                        "line_code": f"{path_sha1}_0_{start_line}",
                        "type": "new",
                        "new_line": start_line,
                    },
                    "end": {
                        "line_code": f"{path_sha1}_0_{end_line}",
                        "type": "new",
                        "new_line": end_line,
                    },
                },
            },
        }
        resp = poster.post_discussion(discussion, comment_id=it["id"])
        if resp.get("success"):
            stats["inline"] += 1
            remaining = resp.get("rate_limit_remaining")
            if rate_limit_threshold > 0 and remaining is not None and remaining <= rate_limit_threshold:
                pace = success_delay * 2
                log("Rate limit quota low (%s remaining), increasing pacing delay to %.1fs" % (remaining, pace))
                sleep(pace)
            else:
                sleep(success_delay)
        else:
            reason = resp.get("failed_reason") or "failed to post inline"
            status = resp.get("http_status")
            if status == 400 and is_line_resolution_failure(resp.get("error_body") or ""):
                diff = get_diff_inventory(poster, diff_cache)
                classification = classify_comment_against_diff(comment, diff)
                if classification == "invalid":
                    reason = "out of diff (line could not be resolved)"
                else:
                    reason = "line resolution failure (%s)" % classification
            failed_comments.append({"comment": comment, "reason": reason})
            is_rl = resp.get("is_rate_limit_exhausted", False)
            sleep(success_delay if is_rl else failure_delay)

    log("Successfully posted %d/%d inline comments." % (stats["inline"], len(comments)))
    stats["failed"] = len(failed_comments)

    # Finalize the summary with the complete body.
    summary_body = build_summary_body(
        stats["total"], stats["inline"], stats["summary"],
        stats["skipped"], stats["routed"], stats["failed"], warnings,
    )
    summary_body += format_summary_comments(no_line)
    summary_body += format_summary_comments(routed)
    summary_body += format_summary_comments(failed_comments)
    if not inline_items and stats["skipped"] > 0:
        summary_body += "\n\n---\n\nℹ️ All inline comments overlapped with existing reviews; nothing new was posted."
    summary_body += format_warnings(warnings)
    run_tag = config.get("run_tag", "0-0")
    full_body = wrap_summary_body(summary_body, run_tag)
    stats["summary_url"] = finalize_summary(poster, full_body, sticky, anchor_id,
                                           tag=summary_tag_for(run_tag))
    if not stats["summary_url"]:
        stats["summary_url"] = poster.mr_url()
    return stats


def load_incremental_history(poster):
    """Return a list of ``{path, span}`` for prior bot discussions.

    None when the read API failed (caller then disables incremental filtering
    rather than risk re-posting nothing or everything).
    """
    discussions = poster.list_discussions()
    if discussions is None:
        log("[incremental] could not list discussions; disabling incremental filter for this run.")
        return None
    history = []
    for d in discussions:
        notes = d.get("notes") or []
        if not any(_BOT_MARKER in (n.get("body") or "") for n in notes):
            continue
        for n in notes:
            span = position_span(n.get("position"))
            if span:
                path = (n.get("position") or {}).get("new_path")
                history.append({"path": path, "span": span})
    return history


def get_diff_inventory(poster, cache):
    """Cached diff inventory for the 400 line-resolution fallback."""
    if cache.get("fetched"):
        return cache.get("diff")
    cache["fetched"] = True
    cache["diff"] = poster.get_mr_diffs()
    return cache["diff"]


def new_comment_id(run_tag):
    """Random per-comment id embedded as an HTML comment for idempotency."""
    return "ocr-%s-%s" % (run_tag, secrets.token_hex(8))


def run_tag_from_env(env):
    return "%s-%s" % (env.get("CI_PIPELINE_ID") or "0", env.get("CI_JOB_ID") or "0")


# --------------------------------------------------------------------------- #
# Config / stats / gating
# --------------------------------------------------------------------------- #


def _parse_bool(value, default=False):
    if value is None:
        return default
    return str(value).strip().lower() in ("1", "true", "yes", "on")


# Zero-filled stats written on every early-exit / error path so the dotenv
# artifact is always present for downstream jobs (see .gitlab-ci.yml
# `reports: dotenv`).
ZERO_STATS = {"total": 0, "inline": 0, "summary": 0,
              "routed": 0, "skipped": 0, "failed": 0, "summary_url": None}


def build_config(env):
    """Build the config dict from environment variables (with defaults)."""
    return {
        # Pacing (used by publish).
        "success_delay": int(env.get("OCR_SUCCESS_DELAY", "2000")) / 1000,
        "failure_delay": int(env.get("OCR_FAILURE_DELAY", "1000")) / 1000,
        "rate_limit_threshold": int(env.get("OCR_RATE_LIMIT_THRESHOLD", "10")),
        # Read-API pacing (cheaper than writes but still consumes the primary
        # rate limit; mirrors the GitHub Action's readWithPacing so a large MR
        # does not hammer list_notes/list_discussions/get_mr_diffs).
        "read_success_delay": int(env.get("OCR_READ_SUCCESS_DELAY", "500")) / 1000,
        "read_low_remaining_spacing": int(env.get("OCR_READ_LOW_REMAINING_SPACING", "5000")) / 1000,
        # Retry (used by make_poster / fetch_diff_refs).
        "retry_base_delay": int(env.get("OCR_RETRY_BASE_DELAY", "2000")) / 1000,
        "max_retries": int(env.get("OCR_MAX_RETRIES", "3")),
        "max_retry_delay": int(env.get("OCR_MAX_RETRY_DELAY", "60000")) / 1000,
        "transient_base_delay": 2.0,
        # Publication policy + summary + incremental.
        "sticky_summary": _parse_bool(env.get("OCR_STICKY_SUMMARY", "true"), default=True),
        "incremental": _parse_bool(env.get("OCR_INCREMENTAL", "false"), default=False),
        "incremental_overlap_threshold": resolve_threshold(env.get("OCR_INCREMENTAL_OVERLAP_THRESHOLD", "0.6")),
        "route_severity_below": env.get("OCR_ROUTE_SEVERITY_BELOW", ""),
        "route_categories": env.get("OCR_ROUTE_CATEGORIES", ""),
        # Job gating.
        "fail_on_severity": env.get("OCR_FAIL_ON_SEVERITY", ""),
        # Per-run identity for idempotency tags.
        "run_tag": run_tag_from_env(env),
    }


def write_stats_file(path, stats):
    """Write a dotenv file of posting stats for downstream CI jobs."""
    lines = []
    for key in ("total", "inline", "summary", "routed", "skipped", "failed"):
        lines.append("OCR_COMMENTS_%s=%d" % (key.upper(), stats.get(key, 0)))
    lines.append("OCR_SUMMARY_URL=%s" % (stats.get("summary_url") or ""))
    parent = os.path.dirname(path)
    if parent:
        try:
            os.makedirs(parent, exist_ok=True)
        except OSError:
            pass  # open() below will surface a real failure if the dir is unusable
    try:
        with open(path, "w", encoding="utf-8") as f:
            f.write("\n".join(lines) + "\n")
    except OSError as e:
        log("warning: could not write stats file %s: %s" % (path, e))


def check_fail_on_severity(comments, threshold):
    """True when any comment's severity is at-or-above ``threshold``."""
    if not threshold:
        return False
    t = str(threshold).strip().lower()
    if t not in SEVERITY_RANK:
        return False
    rank = SEVERITY_RANK[t]
    for c in comments:
        sev = str(c.get("severity") or "").strip().lower()
        if sev in SEVERITY_RANK and SEVERITY_RANK[sev] >= rank:
            return True
    return False


# --------------------------------------------------------------------------- #
# CLI
# --------------------------------------------------------------------------- #


def load_review_result(path):
    """Read the JSON produced by ``ocr review --format json``."""
    with open(path, encoding="utf-8") as f:
        return json.loads(f.read())


def parse_args(argv):
    p = argparse.ArgumentParser(
        description="Post `ocr review --format json` output onto a GitLab merge request."
    )
    p.add_argument("input", nargs="?", default=".ocr/ocr-result.json",
                   help="review result JSON path (default: .ocr/ocr-result.json)")
    p.add_argument("--stderr-log", default=".ocr/ocr-stderr.log",
                   help="OCR stderr log path, read on parse failure (default: .ocr/ocr-stderr.log)")
    p.add_argument("--stats-file", default=".ocr/ocr-stats.env",
                   help="dotenv output path for posting stats (default: .ocr/ocr-stats.env)")
    p.add_argument("--dry-run", action="store_true",
                   help="print discussions/notes instead of posting them")
    return p.parse_args(argv)


def main(argv=None):
    args = parse_args(sys.argv[1:] if argv is None else argv)
    env = os.environ

    # Resolve CI environment.
    gitlab_url = env.get("CI_SERVER_URL", "https://gitlab.com")
    project_id = env.get("CI_PROJECT_ID", "")
    mr_iid = env.get("CI_MERGE_REQUEST_IID", "")
    api_token = env.get("GITLAB_API_TOKEN") or env.get("CI_JOB_TOKEN", "")

    if not args.dry_run:
        missing = [name for name, value in (
            ("CI_PROJECT_ID", project_id),
            ("CI_MERGE_REQUEST_IID", mr_iid),
        ) if not value]
        if missing:
            log("error: missing required %s (set via CI environment)" % ", ".join(missing))
            write_stats_file(args.stats_file, ZERO_STATS)
            return 1
        if not api_token:
            log("ERROR: No API token available (GITLAB_API_TOKEN or CI_JOB_TOKEN). Cannot post comments.")
            write_stats_file(args.stats_file, ZERO_STATS)
            return 1

    api_base = "%s/api/v4/projects/%s/merge_requests/%s" % (gitlab_url, project_id, mr_iid)

    # Determine auth header: PRIVATE-TOKEN for personal/project tokens,
    # JOB-TOKEN for CI_JOB_TOKEN.
    auth_header = "JOB-TOKEN" if not env.get("GITLAB_API_TOKEN") else "PRIVATE-TOKEN"

    config = build_config(env)

    # Read OCR result.
    try:
        result = load_review_result(args.input)
    except (FileNotFoundError, json.JSONDecodeError) as e:
        log("Failed to parse OCR output: %s" % e)
        write_stats_file(args.stats_file, ZERO_STATS)
        if args.dry_run:
            log("(dry-run: skipping error note post)")
            return 0
        stderr_content = ""
        try:
            with open(args.stderr_log, "r") as f:
                stderr_content = f.read().strip()
        except FileNotFoundError:
            pass
        if stderr_content:
            poster = make_poster(api_base, api_token, auth_header, config)
            run_tag = config.get("run_tag", "0-0")
            body = wrap_summary_body(
                "⚠️ **OpenCodeReview** encountered an error:\n%s" % fenced_block(stderr_content),
                run_tag,
            )
            upsert_summary(poster, body, config.get("sticky_summary", True),
                           tag=summary_tag_for(run_tag))
        return 0

    comments = result.get("comments", [])

    if args.dry_run:
        poster = make_dry_run_poster()
        # Synthesize a sentinel so the inline path is exercised (still no network).
        diff_refs = {"base_sha": "(dry)", "start_sha": "(dry)", "head_sha": "(dry)"}
    else:
        poster = make_poster(api_base, api_token, auth_header, config)
        diff_refs = fetch_diff_refs(api_base, api_token, auth_header, config,
                                    _extract_expected_shas(result))
        if not diff_refs:
            log("Warning: Could not fetch MR versions. Inline comments will use fallback.")

    stats = publish(result, diff_refs, poster, config, sleep=_sleep)
    write_stats_file(args.stats_file, stats)

    if not args.dry_run and check_fail_on_severity(comments, config.get("fail_on_severity", "")):
        log("Failing job: review contains severity at or above '%s'." % config["fail_on_severity"])
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())

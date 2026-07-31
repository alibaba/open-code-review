#!/usr/bin/env python3
"""Post an OpenCodeReview result onto a GitLab merge request.

This is the CI-layer "glue" for GitLab, mirroring examples/gerrit_ci and
examples/gitflic_ci: it keeps platform-specific publishing out of the ``ocr``
binary and lives entirely in the pipeline.  It reads the JSON emitted by
``ocr review --format json`` and posts it onto the merge request as GitLab
discussions:

  - one inline discussion per comment that maps onto the diff,
  - a single fallback note collecting comments that could not be placed inline,
  - a final summary note.

The script separates a transport-agnostic :func:`publish` (driven by an
injectable ``post`` callable) from the GitLab REST transport
:func:`make_poster`, so the full posting flow — including retry/backoff and
rate-limit throttling — can be unit-tested with no network access and no
wall-clock sleep cost.

Standard library only (json, urllib) so it runs on any stock python3 image.
"""

import argparse
import json
import os
import random
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
# Comment formatting (pure)
# --------------------------------------------------------------------------- #


def format_comment(comment):
    """Format a single review comment as markdown for an inline discussion.

    Uses GitLab's ``suggestion:-0+0`` syntax so the suggestion renders as a
    one-click "Apply suggestion" button in the MR diff.
    """
    body = comment.get("content", "")

    existing = comment.get("existing_code", "")
    suggestion = comment.get("suggestion_code", "")
    if suggestion and existing:
        body += "\n\n**Suggestion:**\n"
        body += "```suggestion:-0+0\n%s\n```" % suggestion

    return body


def format_comment_fallback(comment):
    """Format a comment for fallback (non-inline) display in a note.

    Uses ``<details><summary>`` HTML so the suggested change is collapsible
    in the MR comment thread.
    """
    path = comment.get("path", "unknown")
    start_line = comment.get("start_line", 0)
    end_line = comment.get("end_line", 0)
    content = comment.get("content", "")

    md = "### 📄 `%s`" % path
    if start_line and end_line:
        md += " (L%d-L%d)" % (start_line, end_line)
    md += "\n\n%s" % content

    existing = comment.get("existing_code", "")
    suggestion = comment.get("suggestion_code", "")
    if suggestion and existing:
        md += "\n\n<details><summary>💡 Suggested Change</summary>\n\n"
        md += "**Before:**\n```\n%s\n```\n\n" % existing
        md += "**After:**\n```\n%s\n```\n\n" % suggestion
        md += "</details>"

    return md


# --------------------------------------------------------------------------- #
# Transport-agnostic publishing
# --------------------------------------------------------------------------- #


def publish(result, diff_refs, post, config, sleep=_sleep):
    """Post the review result via the injectable ``post(discussion)`` callable.

    ``diff_refs`` is a dict with ``base_sha``/``start_sha``/``head_sha``
    (or ``None`` when the ``/versions`` endpoint failed).

    ``post`` receives a discussion dict (``body`` + optional ``position``) and
    returns ``{'success': bool, 'rate_limit_remaining': int|None,
    'is_rate_limit_exhausted': bool}``.

    ``config`` provides pacing values (``success_delay``, ``failure_delay``,
    ``rate_limit_threshold``).

    Returns ``{"inline": int, "fallback": int}``.
    """
    success_delay = config["success_delay"]
    failure_delay = config["failure_delay"]
    rate_limit_threshold = config["rate_limit_threshold"]

    comments = result.get("comments") or []
    if not comments:
        message = result.get("message", "No comments generated. Looks good to me.")
        post({"body": "✅ **OpenCodeReview**: %s" % message})
        return {"inline": 0, "fallback": 0}

    success_count = 0
    failed_comments = []

    for comment in comments:
        path = comment.get("path", "")
        end_line = comment.get("end_line", 0)
        start_line = comment.get("start_line", end_line)
        body = format_comment(comment)

        if not path or not end_line or not diff_refs:
            failed_comments.append(comment)
            continue

        discussion = {
            "body": body,
            "position": {
                "position_type": "text",
                "new_path": path,
                "old_path": path,
                "new_line": end_line,
                "base_sha": diff_refs["base_sha"],
                "start_sha": diff_refs["start_sha"],
                "head_sha": diff_refs["head_sha"],
            },
        }
        result_resp = post(discussion)
        if result_resp and result_resp.get("success"):
            success_count += 1
            # Proactive throttling: slow down when GitLab reports low quota.
            # Applied only on the success path (matching the heredoc).
            remaining = result_resp.get("rate_limit_remaining")
            if rate_limit_threshold > 0 and remaining is not None and remaining <= rate_limit_threshold:
                pace_delay = success_delay * 2
                log("Rate limit quota low (%s remaining), increasing pacing delay to %.1fs" % (remaining, pace_delay))
                sleep(pace_delay)
            else:
                sleep(success_delay)
        else:
            failed_comments.append(comment)
            # Failure pacing: rate-limit-exhausted failures get the longer
            # success_delay; other failures get the shorter failure_delay.
            is_rate_limit_exhausted = (
                result_resp.get("is_rate_limit_exhausted", False)
                if result_resp else False
            )
            post_fail_delay = success_delay if is_rate_limit_exhausted else failure_delay
            sleep(post_fail_delay)

    log("Successfully posted %d/%d inline comments." % (success_count, len(comments)))

    # Post fallback for any failed inline comments
    if failed_comments:
        fallback_body = "🔍 **OpenCodeReview** found issues that could not be posted inline:\n\n---\n\n"
        for comment in failed_comments:
            fallback_body += format_comment_fallback(comment) + "\n\n---\n\n"
        post({"body": fallback_body})

    # Post summary last
    total_count = len(comments)
    failed_count = len(failed_comments)
    summary = "🔍 **OpenCodeReview** found **%d** issue(s) in this MR." % total_count
    if total_count > 0:
        summary += "\n- ✅ %d posted as inline comment(s)" % success_count
        summary += "\n- 📝 %d posted as summary (missing line info)" % failed_count
    warnings = result.get("warnings") or []
    if warnings:
        summary += "\n\n⚠️ %d warning(s) occurred during review." % len(warnings)
    post({"body": summary})

    return {"inline": success_count, "fallback": failed_count}


# --------------------------------------------------------------------------- #
# GitLab REST transport
# --------------------------------------------------------------------------- #


def _get_header(headers, name):
    """Case-insensitive header lookup.

    urllib normalizes response header keys to title-case (e.g. 'Retry-After'),
    but this defensive check also handles original casing so that retry delay
    computation and quota logging never silently miss a header.
    """
    if name in headers:
        val = headers[name]
    elif name.lower() in headers:
        val = headers[name.lower()]
    else:
        return None
    return str(val).strip() if val is not None else None


def _parse_rate_limit_header(headers, name):
    """Safely parse a rate-limit response header as an int."""
    val = _get_header(headers, name)
    if val is None:
        return None
    try:
        return int(val)
    except (ValueError, TypeError):
        return None


def _api_request_with_retry(api_base, token, auth_header, config, endpoint,
                            data=None, method="POST"):
    """Make a GitLab API request with retry on rate-limit and transient errors.

    Returns a dict::

        {'success': bool, 'data': response or None,
         'is_rate_limit_exhausted': bool, 'rate_limit_remaining': int or None}
    """
    max_retries = config["max_retries"]
    retry_base_delay = config["retry_base_delay"]
    max_retry_delay = config["max_retry_delay"]
    transient_base_delay = config["transient_base_delay"]

    for attempt in range(max_retries + 1):
        url = "%s%s" % (api_base, endpoint)
        headers = {
            auth_header: token,
            "Content-Type": "application/json",
        }
        body = json.dumps(data).encode("utf-8") if data else None
        req = urllib.request.Request(url, data=body, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req) as resp:
                resp_data = json.loads(resp.read().decode("utf-8", "replace"))
                remaining = _parse_rate_limit_header(resp.headers, "RateLimit-Remaining")
                limit = _parse_rate_limit_header(resp.headers, "RateLimit-Limit")
                if remaining is not None and limit is not None:
                    log("RateLimit: %s/%s remaining for %s" % (remaining, limit, endpoint))
                return {
                    "success": True,
                    "data": resp_data,
                    "is_rate_limit_exhausted": False,
                    "rate_limit_remaining": remaining,
                }
        except urllib.error.HTTPError as e:
            error_body = e.read().decode("utf-8", "replace")
            is_rate_limit = e.code == 429 or (
                e.code == 403
                and any(kw in error_body.lower() for kw in
                        ["retry later", "rate limit", "too many requests", "abuse"])
            )
            # Transient server errors (5xx) and request timeouts (408) are
            # worth retrying with a short exponential backoff.
            is_transient = (500 <= e.code < 600) or e.code == 408
            rl_remaining = _parse_rate_limit_header(e.headers, "RateLimit-Remaining")

            if (is_rate_limit or is_transient) and attempt < max_retries:
                retry_after = _get_header(e.headers, "Retry-After")
                if retry_after:
                    try:
                        delay = float(retry_after)
                    except ValueError:
                        delay = retry_base_delay * (2 ** attempt)
                elif is_transient:
                    delay = transient_base_delay * (2 ** attempt)
                else:
                    delay = retry_base_delay * (2 ** attempt)
                delay = min(delay, max_retry_delay)
                delay = delay * (0.75 + random.random() * 0.5)  # ±25% jitter
                rl_info = ""
                if rl_remaining is not None:
                    rl_info = " (RateLimit-Remaining: %s)" % rl_remaining
                reason = "rate limit" if is_rate_limit else "transient error (HTTP %d)" % e.code
                log("%s hit for %s, retrying in %.1fs (attempt %d/%d)%s"
                    % (reason, endpoint, delay, attempt + 1, max_retries, rl_info))
                _sleep(delay)
            else:
                log("API error %d: %s" % (e.code, error_body))
                return {
                    "success": False,
                    "data": None,
                    "is_rate_limit_exhausted": is_rate_limit,
                    "rate_limit_remaining": rl_remaining,
                }
        except urllib.error.URLError as e:
            # Retry connection-level errors (DNS, refused, reset), but NOT
            # connection-phase timeouts (ambiguous — server may have
            # processed the request, so retrying risks a duplicate post).
            if isinstance(e.reason, (socket.timeout, TimeoutError)):
                log("Connection timeout for %s, not retrying" % endpoint)
                return {
                    "success": False,
                    "data": None,
                    "is_rate_limit_exhausted": False,
                    "rate_limit_remaining": None,
                }
            if attempt < max_retries:
                delay = transient_base_delay * (2 ** attempt)
                delay = min(delay, max_retry_delay)
                delay = delay * (0.75 + random.random() * 0.5)  # ±25% jitter
                log("Network error for %s, retrying in %.1fs (attempt %d/%d)"
                    % (endpoint, delay, attempt + 1, max_retries))
                _sleep(delay)
            else:
                log("Network error for %s after %d retries, giving up"
                    % (endpoint, max_retries))
                return {
                    "success": False,
                    "data": None,
                    "is_rate_limit_exhausted": False,
                    "rate_limit_remaining": None,
                }

    return {
        "success": False,
        "data": None,
        "is_rate_limit_exhausted": False,
        "rate_limit_remaining": None,
    }


def make_poster(api_base, token, auth_header, config):
    """Return a ``post(discussion)`` callable that POSTs to the GitLab API.

    Discussions with a ``position`` key are posted as inline comments via
    ``/discussions``; discussions with only ``body`` are posted as notes via
    ``/notes``.

    The returned callable returns::

        {'success': bool, 'rate_limit_remaining': int or None,
         'is_rate_limit_exhausted': bool}
    """

    def post(discussion):
        if "position" in discussion:
            resp = _api_request_with_retry(
                api_base, token, auth_header, config,
                "/discussions", data=discussion, method="POST",
            )
        else:
            resp = _api_request_with_retry(
                api_base, token, auth_header, config,
                "/notes", data={"body": discussion["body"]}, method="POST",
            )
        return {
            "success": resp["success"],
            "rate_limit_remaining": resp["rate_limit_remaining"],
            "is_rate_limit_exhausted": resp["is_rate_limit_exhausted"],
        }

    return post


def fetch_diff_refs(api_base, token, auth_header, config):
    """Fetch MR diff refs from the ``/versions`` endpoint.

    Returns a dict with ``base_sha``/``start_sha``/``head_sha`` on success,
    or ``None`` on failure.
    """
    resp = _api_request_with_retry(
        api_base, token, auth_header, config,
        "/versions", method="GET",
    )
    if resp and resp.get("success"):
        versions = resp.get("data", [])
        if versions:
            latest = versions[0]
            return {
                "base_sha": latest.get("base_commit_sha", ""),
                "start_sha": latest.get("start_commit_sha", ""),
                "head_sha": latest.get("head_commit_sha", ""),
            }
    return None


def make_dry_run_poster():
    """Return a ``post(discussion)`` callable that prints instead of posting."""

    def post(discussion):
        if "position" in discussion:
            pos = discussion["position"]
            location = "%s:%s" % (pos.get("new_path", ""), pos.get("new_line", ""))
        else:
            location = "general"
        print("--- dry-run discussion [%s] ---\n%s\n" % (location, discussion.get("body", "")))
        return {"success": True, "rate_limit_remaining": None, "is_rate_limit_exhausted": False}

    return post


# --------------------------------------------------------------------------- #
# Config
# --------------------------------------------------------------------------- #


def build_config(env):
    """Build the config dict from environment variables (with defaults).

    ``env`` is typically ``os.environ``; tests pass a plain dict.
    """
    return {
        # Pacing (used by publish)
        "success_delay": int(env.get("OCR_SUCCESS_DELAY", "2000")) / 1000,
        "failure_delay": int(env.get("OCR_FAILURE_DELAY", "1000")) / 1000,
        "rate_limit_threshold": int(env.get("OCR_RATE_LIMIT_THRESHOLD", "10")),
        # Retry (used by make_poster / fetch_diff_refs)
        "retry_base_delay": int(env.get("OCR_RETRY_BASE_DELAY", "2000")) / 1000,
        "max_retries": int(env.get("OCR_MAX_RETRIES", "3")),
        "max_retry_delay": int(env.get("OCR_MAX_RETRY_DELAY", "60000")) / 1000,
        "transient_base_delay": 2.0,  # hardcoded, matches heredoc
    }


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
    p.add_argument("input", nargs="?", default="/tmp/ocr-result.json",
                   help="review result JSON path (default: /tmp/ocr-result.json)")
    p.add_argument("--stderr-log", default="/tmp/ocr-stderr.log",
                   help="OCR stderr log path, read on parse failure (default: /tmp/ocr-stderr.log)")
    p.add_argument("--dry-run", action="store_true",
                   help="print discussions instead of posting them")
    return p.parse_args(argv)


def main(argv=None):
    args = parse_args(sys.argv[1:] if argv is None else argv)
    env = os.environ

    # Resolve CI environment
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
            log("error: missing required %s (set via CI environment)"
                % ", ".join(missing))
            return 1
        if not api_token:
            log("ERROR: No API token available (GITLAB_API_TOKEN or CI_JOB_TOKEN). Cannot post comments.")
            return 1

    api_base = "%s/api/v4/projects/%s/merge_requests/%s" % (gitlab_url, project_id, mr_iid)

    # Determine auth header: PRIVATE-TOKEN for personal/project tokens,
    # JOB-TOKEN for CI_JOB_TOKEN.
    auth_header = "JOB-TOKEN" if not env.get("GITLAB_API_TOKEN") else "PRIVATE-TOKEN"

    config = build_config(env)

    # Read OCR result
    try:
        result = load_review_result(args.input)
    except (FileNotFoundError, json.JSONDecodeError) as e:
        log("Failed to parse OCR output: %s" % e)
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
            post = make_poster(api_base, api_token, auth_header, config)
            post({"body": "⚠️ **OpenCodeReview** encountered an error:\n```\n%s\n```" % stderr_content})
        return 0

    comments = result.get("comments", [])

    # No comments — post LGTM note
    if not comments:
        message = result.get("message", "No comments generated. Looks good to me.")
        if args.dry_run:
            post = make_dry_run_poster()
        else:
            post = make_poster(api_base, api_token, auth_header, config)
        post({"body": "✅ **OpenCodeReview**: %s" % message})
        print("No review comments to post.")
        return 0

    if args.dry_run:
        # Dry-run: no API calls at all; comments go through publish() with
        # diff_refs=None so they appear as fallback (matching gerrit/gitflic
        # dry-run which also avoids all network).
        post = make_dry_run_poster()
        diff_refs = None
    else:
        post = make_poster(api_base, api_token, auth_header, config)
        # Fetch MR diff metadata for position calculation
        diff_refs = fetch_diff_refs(api_base, api_token, auth_header, config)
        if not diff_refs:
            log("Warning: Could not fetch MR versions. Inline comments will use fallback.")

    publish(result, diff_refs, post, config, sleep=_sleep)
    return 0


if __name__ == "__main__":
    sys.exit(main())

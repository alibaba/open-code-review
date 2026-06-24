#!/usr/bin/env python3
"""Post an OpenCodeReview result onto a GitFlic merge request.

This is the CI-layer "glue" for GitFlic, mirroring examples/gitlab_ci: it keeps
platform-specific publishing out of the `ocr` binary and lives entirely in the
pipeline. It reads the JSON emitted by `ocr review --format json` and posts it
onto the merge request as discussions:

  - one inline discussion per comment that maps onto the diff,
  - a single fallback note collecting the comments that do not,
  - a final summary note.

GitFlic's Discussions API needs an *old-side* line even for a comment on the new
side of the diff: an inline (code) discussion requires all four of
newLine/oldLine/newPath/oldPath, otherwise GitFlic silently records a plain
comment. `ocr review` only reports new-side positions, so this script computes
the old-side line itself by parsing the same merge-base diff the review ran on
(`git diff merge-base(from, to)..to`).

Standard library only (json, urllib, subprocess) so it runs on the stock
node:20 / python image used by the pipeline.
"""

import argparse
import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.request
from urllib.parse import quote

# GitFlic SaaS REST API endpoint; override with --api-url / $GITFLIC_API_URL for
# self-hosted instances (e.g. http://gitflic.example/rest-api).
DEFAULT_API_URL = "https://api.gitflic.ru"

# Context lines around each hunk; must match what `ocr review` diffs with so the
# new-side line numbers in the comments align with the hunks parsed here.
DIFF_CONTEXT_LINES = 3


def log(msg):
    print(msg, file=sys.stderr)


# --------------------------------------------------------------------------- #
# Diff parsing
# --------------------------------------------------------------------------- #

HUNK_CONTEXT, HUNK_ADDED, HUNK_DELETED = range(3)

_HUNK_HEADER_RE = re.compile(r"^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@")
_DIFF_HEADER_RE = re.compile(r"^diff --git a/(.+?) b/(.+)$")


class Hunk:
    """One @@ ... @@ block of a unified diff."""

    __slots__ = ("old_start", "old_count", "new_start", "new_count", "lines")

    def __init__(self, old_start, old_count, new_start, new_count):
        self.old_start = old_start
        self.old_count = old_count
        self.new_start = new_start
        self.new_count = new_count
        self.lines = []  # list of (type, content)


class FileDiff:
    """A single file's section of a unified diff."""

    __slots__ = ("old_path", "new_path", "is_new", "is_deleted", "is_binary", "text")

    def __init__(self, old_path="", new_path=""):
        self.old_path = old_path
        self.new_path = new_path
        self.is_new = False
        self.is_deleted = False
        self.is_binary = False
        self.text = ""  # raw diff body, fed to parse_hunks() on demand


def parse_hunks(raw):
    """Parse one file's unified diff text into a list of Hunks.

    Lines before the first @@ header (diff --git, ---, +++) are ignored.
    """
    hunks = []
    current = None
    for line in raw.split("\n"):
        m = _HUNK_HEADER_RE.match(line)
        if m:
            if current is not None:
                hunks.append(current)
            old_start = int(m.group(1))
            old_count = int(m.group(2)) if m.group(2) else 1
            new_start = int(m.group(3))
            new_count = int(m.group(4)) if m.group(4) else 1
            current = Hunk(old_start, old_count, new_start, new_count)
            continue
        if current is None:
            continue
        if line.startswith("\\ No newline at end of file"):
            continue
        if line.startswith("diff --git "):
            break
        if line.startswith("+"):
            current.lines.append((HUNK_ADDED, line[1:]))
        elif line.startswith("-"):
            current.lines.append((HUNK_DELETED, line[1:]))
        else:
            content = line[1:] if line[:1] == " " else line
            current.lines.append((HUNK_CONTEXT, content))
    if current is not None:
        hunks.append(current)
    return hunks


def parse_diff(diff_text):
    """Split combined unified diff text into per-file FileDiff sections."""
    files = []
    current = None
    buf = []

    def flush():
        if current is not None:
            current.text = "\n".join(buf)
            files.append(current)

    for line in diff_text.split("\n"):
        m = _DIFF_HEADER_RE.match(line)
        if m:
            flush()
            buf = []
            current = FileDiff(old_path=m.group(1), new_path=m.group(2))
        if current is None:
            continue
        if line.startswith("Binary files ") or line.startswith("GIT binary patch"):
            current.is_binary = True
        elif line.startswith("new file mode"):
            current.is_new = True
        elif line.startswith("deleted file mode"):
            current.is_deleted = True
        elif line.startswith("--- "):
            path = line[4:]
            if path == "/dev/null":
                current.is_new = True
                current.old_path = "/dev/null"
            elif path.startswith("a/"):
                current.old_path = path[2:]
        elif line.startswith("+++ "):
            path = line[4:]
            if path == "/dev/null":
                current.is_deleted = True
                current.new_path = "/dev/null"
            elif path.startswith("b/"):
                current.new_path = path[2:]
        buf.append(line)

    flush()
    return files


# --------------------------------------------------------------------------- #
# Line mapping (new file side -> old file side)
# --------------------------------------------------------------------------- #


def clamp_line(n):
    return 1 if n < 1 else n


def old_line_for(hunks, new_line):
    """Map a new-side line number to the corresponding old-side line.

    Lines added by the diff have no old counterpart, so they are anchored to the
    closest preceding old line -- GitFlic only needs a plausible old-side
    position to render the code comment next to the insertion point. The result
    is always >= 1.
    """
    delta = 0  # cumulative (new - old) line-count shift from preceding hunks
    for h in hunks:
        if new_line < h.new_start:
            break
        if new_line < h.new_start + h.new_count:
            return _old_line_in_hunk(h, new_line)
        delta += h.new_count - h.old_count
    return clamp_line(new_line - delta)


def _old_line_in_hunk(h, new_line):
    """Walk a hunk's lines tracking both counters until reaching new_line."""
    old_ln, new_ln = h.old_start, h.new_start
    last_old = h.old_start - 1  # last old line seen before the current position
    for line_type, _content in h.lines:
        if line_type == HUNK_CONTEXT:
            if new_ln == new_line:
                return clamp_line(old_ln)
            last_old = old_ln
            old_ln += 1
            new_ln += 1
        elif line_type == HUNK_DELETED:
            last_old = old_ln
            old_ln += 1
        elif line_type == HUNK_ADDED:
            if new_ln == new_line:
                return clamp_line(last_old)
            new_ln += 1
    return clamp_line(last_old)


# --------------------------------------------------------------------------- #
# Comment formatting
# --------------------------------------------------------------------------- #


def format_comment(c):
    """Render an inline discussion body."""
    body = c.get("content", "")
    suggestion = c.get("suggestion_code", "")
    existing = c.get("existing_code", "")
    if suggestion and existing:
        body += "\n\n**Suggestion:**\n```\n" + suggestion + "\n```"
    return body


def format_comment_fallback(c):
    """Render a comment for the fallback (non-inline) note."""
    md = "### 📄 `%s`" % c.get("path", "")
    start_line = c.get("start_line", 0)
    end_line = c.get("end_line", 0)
    if start_line and end_line:
        md += " (L%d-L%d)" % (start_line, end_line)
    md += "\n\n" + c.get("content", "")
    suggestion = c.get("suggestion_code", "")
    existing = c.get("existing_code", "")
    if suggestion and existing:
        md += "\n\n**Before:**\n```\n" + existing + "\n```\n\n**After:**\n```\n" + suggestion + "\n```"
    return md


# --------------------------------------------------------------------------- #
# Publishing (transport-agnostic; `post` does the actual API call)
# --------------------------------------------------------------------------- #


def publish(result, diffs_by_path, post):
    """Post the review result via the `post(discussion)` callable.

    `post` receives a discussion dict and must raise on failure. A general
    comment carries only "message"; an inline comment also carries
    newLine/oldLine/newPath/oldPath. Returns {"inline": int, "fallback": int}.
    """
    comments = result.get("comments") or []
    if not comments:
        message = result.get("message") or "No comments generated. Looks good to me."
        post({"message": "✅ **OpenCodeReview**: " + message})
        return {"inline": 0, "fallback": 0}

    inline = 0
    failed = []
    hunks_cache = {}

    for c in comments:
        path = c.get("path", "")
        end_line = c.get("end_line", 0) or 0
        fd = diffs_by_path.get(path)
        if fd is None:
            log("no diff for %s; folding comment into the summary note" % path)
            failed.append(c)
            continue
        if fd.is_binary or fd.is_deleted or end_line <= 0:
            failed.append(c)
            continue

        old_path = fd.old_path
        old_line = 1
        if fd.is_new or old_path == "" or old_path == "/dev/null":
            # GitFlic has no old side for a new file; anchor to the new path.
            old_path = fd.new_path
        else:
            hunks = hunks_cache.get(path)
            if hunks is None:
                hunks = parse_hunks(fd.text)
                hunks_cache[path] = hunks
            old_line = old_line_for(hunks, end_line)

        discussion = {
            "message": format_comment(c),
            "newLine": end_line,
            "oldLine": old_line,
            "newPath": path,
            "oldPath": old_path,
        }
        try:
            post(discussion)
        except Exception as e:  # noqa: BLE001 - any transport error falls back
            log("inline comment failed for %s:%d: %s" % (path, end_line, e))
            failed.append(c)
            continue
        inline += 1

    if failed:
        note = "🔍 **OpenCodeReview** found issues that could not be posted inline:\n\n---\n\n"
        for c in failed:
            note += format_comment_fallback(c) + "\n\n---\n\n"
        post({"message": note})

    summary = "🔍 **OpenCodeReview** found **%d** issue(s) in this MR." % len(comments)
    summary += "\n- ✅ %d posted as inline comment(s)" % inline
    summary += "\n- 📝 %d posted as summary (could not be placed inline)" % len(failed)
    warnings = result.get("warnings") or []
    if warnings:
        summary += "\n\n⚠️ %d warning(s) occurred during review." % len(warnings)
    post({"message": summary})

    return {"inline": inline, "fallback": len(failed)}


# --------------------------------------------------------------------------- #
# GitFlic REST transport
# --------------------------------------------------------------------------- #


def make_poster(api_url, token, owner, project, mr):
    """Return a post(discussion) that POSTs to the GitFlic Discussions API."""
    endpoint = "%s/project/%s/%s/merge-request/%s/discussions/create" % (
        api_url.rstrip("/"),
        quote(owner, safe=""),
        quote(project, safe=""),
        quote(mr, safe=""),
    )

    def post(discussion):
        body = json.dumps(discussion).encode("utf-8")
        req = urllib.request.Request(endpoint, data=body, method="POST")
        req.add_header("Authorization", "token " + token)
        req.add_header("Content-Type", "application/json")
        try:
            with urllib.request.urlopen(req) as resp:
                resp.read()
        except urllib.error.HTTPError as e:
            snippet = e.read(512).decode("utf-8", "replace").strip()
            # Some APIs echo request details back in error bodies; never let the
            # token reach the CI log if GitFlic does that.
            if token:
                snippet = snippet.replace(token, "***")
            raise RuntimeError("gitflic API %s %s: %s" % (e.code, e.reason, snippet))

    return post


def make_dry_run_poster():
    """Return a post(discussion) that prints instead of calling the API."""

    def post(discussion):
        if discussion.get("newPath") and "newLine" in discussion and "oldLine" in discussion:
            position = "%s:%d (old %s:%d)" % (
                discussion["newPath"],
                discussion["newLine"],
                discussion.get("oldPath", ""),
                discussion["oldLine"],
            )
        else:
            position = "general"
        print("--- dry-run discussion [%s] ---\n%s\n" % (position, discussion["message"]))

    return post


# --------------------------------------------------------------------------- #
# git / IO
# --------------------------------------------------------------------------- #


def _git(repo, *args):
    return subprocess.run(
        ["git", *args], cwd=repo, check=True, capture_output=True, text=True
    ).stdout


def load_diffs_by_path(repo, from_ref, to_ref):
    """Build {new_path: FileDiff} for the merge-base diff `ocr review` ran on."""
    base = _git(repo, "merge-base", from_ref, to_ref).strip()
    out = _git(
        repo, "diff", "--no-ext-diff", "--no-textconv",
        "--src-prefix=a/", "--dst-prefix=b/", "--no-color",
        "-U%d" % DIFF_CONTEXT_LINES, base, to_ref, "--",
    )
    return {fd.new_path: fd for fd in parse_diff(out)}


def load_review_result(path):
    """Read the JSON produced by `ocr review --format json` (path '-' = stdin)."""
    if path == "-":
        data = sys.stdin.read()
    else:
        with open(path, encoding="utf-8") as f:
            data = f.read()
    return json.loads(data)


# --------------------------------------------------------------------------- #
# CLI
# --------------------------------------------------------------------------- #


def parse_args(argv):
    target = os.environ.get("CI_MERGE_REQUEST_TARGET_BRANCH_NAME", "")
    default_from = "origin/" + target if target else ""

    p = argparse.ArgumentParser(
        description="Post `ocr review --format json` output onto a GitFlic merge request."
    )
    p.add_argument("file", nargs="?", default="-",
                   help="review result JSON ('-' = stdin, default)")
    p.add_argument("--owner", default=os.environ.get("CI_PROJECT_NAMESPACE", ""),
                   help="project owner alias (default: $CI_PROJECT_NAMESPACE)")
    p.add_argument("--project", default=os.environ.get("CI_PROJECT_NAME", ""),
                   help="project alias (default: $CI_PROJECT_NAME)")
    p.add_argument("--mr", default=os.environ.get("CI_MERGE_REQUEST_LOCAL_ID", ""),
                   help="merge request local id (default: $CI_MERGE_REQUEST_LOCAL_ID)")
    p.add_argument("--api-url", default=os.environ.get("GITFLIC_API_URL", "") or DEFAULT_API_URL,
                   help="GitFlic REST API base URL (default: $GITFLIC_API_URL or %s)" % DEFAULT_API_URL)
    p.add_argument("--from", dest="from_ref", default=default_from,
                   help="base ref of the reviewed range (default: origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME)")
    p.add_argument("--to", dest="to_ref", default=os.environ.get("CI_COMMIT_SHA", ""),
                   help="head ref of the reviewed range (default: $CI_COMMIT_SHA)")
    p.add_argument("--repo", default=".", help="git repository root (default: .)")
    p.add_argument("--dry-run", action="store_true",
                   help="print discussions instead of posting them")
    return p.parse_args(argv)


def main(argv=None):
    args = parse_args(sys.argv[1:] if argv is None else argv)

    missing = [name for name, value in (
        ("--owner", args.owner), ("--project", args.project), ("--mr", args.mr),
        ("--from", args.from_ref), ("--to", args.to_ref),
    ) if not value]
    if missing:
        log("error: %s required (set via flag or CI environment)" % ", ".join(missing))
        return 2

    token = os.environ.get("GITFLIC_TOKEN", "")
    if not token and not args.dry_run:
        log("error: GITFLIC_TOKEN environment variable is required")
        return 2

    try:
        result = load_review_result(args.file)
    except (OSError, ValueError) as e:
        log("error: cannot read review result %s: %s" % (args.file, e))
        return 1

    try:
        diffs_by_path = load_diffs_by_path(args.repo, args.from_ref, args.to_ref)
    except (subprocess.CalledProcessError, OSError) as e:
        # Without the diff, inline positions cannot be computed; comments still
        # go out via the fallback note.
        log("warning: cannot read diff %s..%s, posting all comments as fallback: %s"
            % (args.from_ref, args.to_ref, e))
        diffs_by_path = {}

    if args.dry_run:
        post = make_dry_run_poster()
    else:
        post = make_poster(args.api_url, token, args.owner, args.project, args.mr)

    stats = publish(result, diffs_by_path, post)
    total = len(result.get("comments") or [])
    print("Posted %d inline comment(s), %d via fallback note (%d total)."
          % (stats["inline"], stats["fallback"], total))
    return 0


if __name__ == "__main__":
    sys.exit(main())

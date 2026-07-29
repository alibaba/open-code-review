#!/usr/bin/env python3
"""post_review.py

Runs Open Code Review (`ocr review --format json`) against the current
CodeUp change request and posts the results as a single summary comment
via the CodeUp CreateChangeRequestComment OpenAPI:

    POST https://{domain}/oapi/v1/codeup/organizations/{organizationId}/repositories/{repositoryId}/changeRequests/{localId}/comments
    POST https://{domain}/oapi/v1/codeup/repositories/{repositoryId}/changeRequests/{localId}/comments

v1 scope: one GLOBAL_COMMENT per review run, not true per-line inline
comments. Inline comments require the change request's current patchset
biz id (from/to_patchset_biz_id), which is a natural follow-up once this
lands.

Required environment variables (set by codeup-flow.yml from the merge
request trigger context):
    CODEUP_DOMAIN           e.g. codeup.aliyun.com
    CODEUP_TOKEN            personal access token (sent as x-yunxiao-token)
    CODEUP_ORG_ID           Yunxiao organization ID
    CODEUP_REPO_ID          numeric ID of the CodeUp repository
    CODEUP_MR_LOCAL_ID      the merge request's local id (integer)
    CODEUP_TARGET_BRANCH    merge request target branch (review base)
    CODEUP_SOURCE_BRANCH    merge request source branch (review head)

`ocr review` is run over `origin/<target>..origin/<source>`, so the
pipeline step needs at least a fetch of both branches (see README's
"non-shallow clone" note).

Optional:
    OCR_REVIEW_EXTRA_ARGS   extra flags to pass to `ocr review`, shell-quoted,
                            e.g. "--concurrency 4"
    OCR_MAX_ITEMS           cap on issues rendered in the comment (default 50)
    OCR_FAIL_ON_POST_ERROR  "1"/"true" to fail the pipeline step if
                            posting the comment fails (default: don't fail)

--from-ref/--to-ref and --extra-args CLI flags are provided mainly so this
script can be exercised locally without needing the full Flow env.
"""
from __future__ import annotations

import argparse
import json
import os
import shlex
import subprocess
import sys
import urllib.error
import urllib.request

SEVERITY_ORDER = {"critical": 0, "high": 1, "medium": 2, "low": 3, "info": 4}
SEVERITY_EMOJI = {
    "critical": "\U0001F534",  # red circle
    "high": "\U0001F7E0",      # orange circle
    "medium": "\U0001F7E1",    # yellow circle
    "low": "\U0001F535",       # blue circle
    "info": "\u26AA",          # white circle
}


class OcrReviewError(RuntimeError):
    """Raised when the ocr CLI fails to run or produce usable output."""


class CodeupApiError(RuntimeError):
    """Raised when the CodeUp OpenAPI call fails."""


def run_ocr_review(from_ref: str | None = None, to_ref: str | None = None,
                    extra_args: list[str] | None = None,
                    timeout: int = 1800) -> dict:
    """Run `ocr review --format json` and return the parsed JSON output."""
    cmd = ["ocr", "review", "--format", "json"]
    if from_ref and to_ref:
        cmd += ["--from", from_ref, "--to", to_ref]
    if extra_args:
        cmd += extra_args

    try:
        proc = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
    except FileNotFoundError as exc:
        raise OcrReviewError(
            "`ocr` CLI not found on PATH. Install it first "
            "(npm install -g @alibaba-group/open-code-review)."
        ) from exc
    except subprocess.TimeoutExpired as exc:
        raise OcrReviewError(f"`ocr review` timed out after {timeout}s.") from exc

    if proc.returncode != 0:
        raise OcrReviewError(
            f"`ocr review` exited with code {proc.returncode}.\n"
            f"stderr:\n{proc.stderr.strip()}"
        )

    stdout = proc.stdout.strip()
    if not stdout:
        raise OcrReviewError("`ocr review` produced no output on stdout.")

    try:
        return json.loads(stdout)
    except json.JSONDecodeError as exc:
        raise OcrReviewError(f"Could not parse ocr JSON output: {exc}") from exc


def _get_comments(review_result) -> list:
    """Normalize the top-level shape of ocr's JSON output to a list of comments."""
    if isinstance(review_result, list):
        return review_result
    if isinstance(review_result, dict):
        for key in ("comments", "results", "issues"):
            if key in review_result and isinstance(review_result[key], list):
                return review_result[key]
    return []


def _field(comment: dict, *names, default=""):
    """Return the first present, non-empty field among several possible key names."""
    for name in names:
        value = comment.get(name)
        if value not in (None, ""):
            return value
    return default


def format_summary_comment(review_result, max_items: int = 50) -> str:
    """Render ocr's JSON review output as a single Markdown summary comment."""
    comments = _get_comments(review_result)

    if not comments:
        return "**Open Code Review**: No issues found. \u2705"

    def sort_key(c):
        severity = str(_field(c, "severity", default="info")).lower()
        return SEVERITY_ORDER.get(severity, len(SEVERITY_ORDER))

    comments = sorted(comments, key=sort_key)
    shown = comments[:max_items] if max_items > 0 else comments

    lines = [f"**Open Code Review** found {len(comments)} issue(s):", ""]

    for c in shown:
        path = _field(c, "path", "file", default="(unknown file)")
        line = _field(c, "start_line", "line", default="")
        severity = str(_field(c, "severity", default="info")).lower()
        category = _field(c, "category", default="")
        content = str(_field(c, "content", "body", default="")).strip()

        emoji = SEVERITY_EMOJI.get(severity, "\u26AA")
        location = f"`{path}:{line}`" if line != "" else f"`{path}`"
        badge = f"[{category} \u00b7 {severity}]" if category else f"[{severity}]"

        lines.append(f"- {emoji} {location} {badge}")
        if content:
            lines.append(f"  {content}")

    remaining = len(comments) - len(shown)
    if remaining > 0:
        lines.append("")
        lines.append(
            f"...and {remaining} more issue(s). Run `ocr review` locally for the full report."
        )

    lines.append("")
    lines.append("_Posted automatically by Open Code Review._")
    return "\n".join(lines)


def post_global_comment(domain: str, repo_id: str, local_id: str, token: str,
                         content: str, organization_id: str | None = None,
                         timeout: int = 30):
    """POST a GLOBAL_COMMENT to a CodeUp change request. Returns (status, body)."""
    if organization_id:
        path = (
            f"/oapi/v1/codeup/organizations/{organization_id}"
            f"/repositories/{repo_id}/changeRequests/{local_id}/comments"
        )
    else:
        path = f"/oapi/v1/codeup/repositories/{repo_id}/changeRequests/{local_id}/comments"

    url = f"https://{domain}{path}"
    payload = json.dumps(
        {
            "comment_type": "GLOBAL_COMMENT",
            "content": content,
            "draft": False,
            "resolved": False,
        }
    ).encode("utf-8")

    req = urllib.request.Request(
        url,
        data=payload,
        method="POST",
        headers={
            "Content-Type": "application/json",
            "x-yunxiao-token": token,
        },
    )

    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.read().decode("utf-8")
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        raise CodeupApiError(f"CodeUp API returned {exc.code}: {body}") from exc
    except urllib.error.URLError as exc:
        raise CodeupApiError(f"Failed to reach CodeUp API at {url}: {exc.reason}") from exc


def _require_env(name: str) -> str:
    value = os.environ.get(name)
    if not value:
        raise SystemExit(f"Missing required environment variable: {name}")
    return value


def _env_flag(name: str, default: bool = False) -> bool:
    value = os.environ.get(name)
    if value is None:
        return default
    return value.strip().lower() in ("1", "true", "yes")


def _default_ref(branch_env_var: str) -> str | None:
    """Build an `origin/<branch>` ref from a CODEUP_*_BRANCH env var, if set."""
    branch = os.environ.get(branch_env_var)
    return f"origin/{branch}" if branch else None


def parse_args(argv=None):
    parser = argparse.ArgumentParser(
        description="Run Open Code Review and post a summary comment to CodeUp."
    )
    parser.add_argument(
        "--from-ref", dest="from_ref",
        default=_default_ref("CODEUP_TARGET_BRANCH"),
        help="Defaults to origin/<CODEUP_TARGET_BRANCH>.",
    )
    parser.add_argument(
        "--to-ref", dest="to_ref",
        default=_default_ref("CODEUP_SOURCE_BRANCH"),
        help="Defaults to origin/<CODEUP_SOURCE_BRANCH>.",
    )
    parser.add_argument(
        "--extra-args", dest="extra_args",
        default=os.environ.get("OCR_REVIEW_EXTRA_ARGS", ""),
        help="Extra shell-quoted flags to pass to `ocr review`.",
    )
    parser.add_argument(
        "--max-items", type=int, default=int(os.environ.get("OCR_MAX_ITEMS", "50"))
    )
    parser.add_argument(
        "--fail-on-post-error",
        action="store_true",
        default=_env_flag("OCR_FAIL_ON_POST_ERROR"),
    )
    return parser.parse_args(argv)


def main(argv=None) -> int:
    args = parse_args(argv)
    extra_args = shlex.split(args.extra_args) if args.extra_args else None

    try:
        review_result = run_ocr_review(
            from_ref=args.from_ref, to_ref=args.to_ref, extra_args=extra_args
        )
    except OcrReviewError as exc:
        print(f"::error:: {exc}", file=sys.stderr)
        return 1

    comment = format_summary_comment(review_result, max_items=args.max_items)
    print(comment)

    try:
        domain = _require_env("CODEUP_DOMAIN")
        org_id = _require_env("CODEUP_ORG_ID")
        repo_id = _require_env("CODEUP_REPO_ID")
        local_id = _require_env("CODEUP_MR_LOCAL_ID")
        token = _require_env("CODEUP_TOKEN")
    except SystemExit as exc:
        print(str(exc), file=sys.stderr)
        return 1

    try:
        status, _body = post_global_comment(
            domain=domain,
            repo_id=repo_id,
            local_id=local_id,
            token=token,
            content=comment,
            organization_id=org_id,
        )
        print(f"Posted review comment to CodeUp (HTTP {status}).")
    except CodeupApiError as exc:
        print(f"::error:: {exc}", file=sys.stderr)
        return 1 if args.fail_on_post_error else 0

    return 0


if __name__ == "__main__":
    sys.exit(main())
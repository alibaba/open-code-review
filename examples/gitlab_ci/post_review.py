#!/usr/bin/env python3
"""Post an OpenCodeReview result onto a GitLab merge request.

The publishing flow is transport-agnostic and accepts an injectable ``post``
callable, while :class:`GitLabClient` owns the REST API, authentication, and
retry behavior. This keeps the CI example standard-library-only and allows the
subtle fallback and rate-limit paths to be tested without network access.
"""

import argparse
import json
import os
import random
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path


def log(message):
    """Write a diagnostic message to stderr."""
    print(message, file=sys.stderr)


@dataclass(frozen=True)
class RetrySettings:
    """Retry and pacing settings, expressed in seconds."""

    retry_base_delay: float = 2.0
    max_retries: int = 3
    max_retry_delay: float = 60.0
    success_delay: float = 2.0
    failure_delay: float = 1.0
    rate_limit_threshold: int = 10

    @classmethod
    def from_environ(cls, environ):
        """Load settings from the GitLab CI variables documented in README.md."""
        return cls(
            retry_base_delay=int(environ.get("OCR_RETRY_BASE_DELAY", "2000")) / 1000,
            max_retries=int(environ.get("OCR_MAX_RETRIES", "3")),
            max_retry_delay=int(environ.get("OCR_MAX_RETRY_DELAY", "60000")) / 1000,
            success_delay=int(environ.get("OCR_SUCCESS_DELAY", "2000")) / 1000,
            failure_delay=int(environ.get("OCR_FAILURE_DELAY", "1000")) / 1000,
            rate_limit_threshold=int(environ.get("OCR_RATE_LIMIT_THRESHOLD", "10")),
        )


class GitLabAPIError(RuntimeError):
    """An API request that still failed after any configured retries."""

    def __init__(self, message, *, is_rate_limit_exhausted=False):
        super().__init__(message)
        self.is_rate_limit_exhausted = is_rate_limit_exhausted


def _get_header(headers, name):
    """Return a response header using a case-insensitive lookup."""
    if headers is None:
        return None
    value = headers.get(name)
    if value is None:
        wanted = name.lower()
        for key, candidate in headers.items():
            if key.lower() == wanted:
                value = candidate
                break
    return str(value).strip() if value is not None else None


def _parse_int_header(headers, name):
    """Parse an integer response header, returning None when absent or invalid."""
    value = _get_header(headers, name)
    if value is None:
        return None
    try:
        return int(value)
    except (TypeError, ValueError):
        return None


def select_auth(environ):
    """Return ``(header_name, token)`` for the available GitLab credential."""
    private_token = environ.get("GITLAB_API_TOKEN")
    if private_token:
        return "PRIVATE-TOKEN", private_token
    job_token = environ.get("CI_JOB_TOKEN")
    if job_token:
        return "JOB-TOKEN", job_token
    raise ValueError(
        "No API token available (GITLAB_API_TOKEN or CI_JOB_TOKEN). "
        "Cannot post comments."
    )


class GitLabClient:
    """Small GitLab REST client with bounded retry and rate-limit handling."""

    def __init__(
        self,
        api_base,
        auth_header,
        token,
        settings,
        *,
        urlopen=urllib.request.urlopen,
        sleep=time.sleep,
        random_value=random.random,
    ):
        self.api_base = api_base.rstrip("/")
        self.auth_header = auth_header
        self.token = token
        self.settings = settings
        self.urlopen = urlopen
        self.sleep = sleep
        self.random_value = random_value

    def request(self, endpoint, data=None, method="POST"):
        """Make a request and return its JSON data and remaining quota."""
        for attempt in range(self.settings.max_retries + 1):
            url = self.api_base + endpoint
            headers = {
                self.auth_header: self.token,
                "Content-Type": "application/json",
            }
            body = json.dumps(data).encode("utf-8") if data else None
            request = urllib.request.Request(
                url, data=body, headers=headers, method=method
            )
            try:
                with self.urlopen(request) as response:
                    raw = response.read().decode("utf-8")
                    response_data = json.loads(raw) if raw else None
                    remaining = _parse_int_header(
                        response.headers, "RateLimit-Remaining"
                    )
                    limit = _parse_int_header(response.headers, "RateLimit-Limit")
                    if remaining is not None and limit is not None:
                        log(
                            "RateLimit: %d/%d remaining for %s"
                            % (remaining, limit, endpoint)
                        )
                    return {
                        "data": response_data,
                        "rate_limit_remaining": remaining,
                    }
            except urllib.error.HTTPError as error:
                error_body = error.read().decode("utf-8", "replace")
                is_rate_limit = error.code == 429 or (
                    error.code == 403
                    and any(
                        phrase in error_body.lower()
                        for phrase in (
                            "retry later",
                            "rate limit",
                            "too many requests",
                            "abuse",
                        )
                    )
                )
                is_transient = error.code == 408 or 500 <= error.code < 600
                remaining = _parse_int_header(error.headers, "RateLimit-Remaining")

                if (
                    is_rate_limit or is_transient
                ) and attempt < self.settings.max_retries:
                    delay = self._retry_delay(
                        error.headers,
                        attempt,
                        is_transient=is_transient,
                    )
                    quota = (
                        " (RateLimit-Remaining: %d)" % remaining
                        if remaining is not None
                        else ""
                    )
                    reason = (
                        "rate limit"
                        if is_rate_limit
                        else "transient error (HTTP %d)" % error.code
                    )
                    log(
                        "%s hit for %s, retrying in %.1fs (attempt %d/%d)%s"
                        % (
                            reason,
                            endpoint,
                            delay,
                            attempt + 1,
                            self.settings.max_retries,
                            quota,
                        )
                    )
                    self.sleep(delay)
                    continue

                raise GitLabAPIError(
                    "GitLab API error %d for %s: %s"
                    % (error.code, endpoint, error_body),
                    is_rate_limit_exhausted=is_rate_limit,
                ) from error

        raise GitLabAPIError("GitLab API request failed for %s" % endpoint)

    def _retry_delay(self, headers, attempt, *, is_transient):
        retry_after = _get_header(headers, "Retry-After")
        if retry_after:
            try:
                delay = float(retry_after)
            except ValueError:
                delay = self.settings.retry_base_delay * (2**attempt)
        else:
            delay = self.settings.retry_base_delay * (2**attempt)

        delay *= 0.75 + self.random_value() * 0.5
        return min(delay, self.settings.max_retry_delay)

    def post_note(self, body):
        """Post a general merge-request note."""
        return self.request("/notes", {"body": body})

    def post_discussion(self, path, line, body, diff_refs):
        """Post a discussion anchored to a line in the current MR diff."""
        position = {
            "position_type": "text",
            "new_path": path,
            "old_path": path,
            "new_line": line,
            "base_sha": diff_refs["base_sha"],
            "start_sha": diff_refs["start_sha"],
            "head_sha": diff_refs["head_sha"],
        }
        return self.request("/discussions", {"body": body, "position": position})

    def get_diff_refs(self):
        """Fetch the latest merge-request version's diff SHAs."""
        response = self.request("/versions", method="GET")
        versions = response.get("data") or []
        if not versions:
            return None
        latest = versions[0]
        refs = {
            "base_sha": latest.get("base_commit_sha", ""),
            "start_sha": latest.get("start_commit_sha", ""),
            "head_sha": latest.get("head_commit_sha", ""),
        }
        return refs if all(refs.values()) else None


def make_poster(client):
    """Return the injectable ``post(discussion)`` used by :func:`publish`."""

    def post(discussion):
        if "path" in discussion:
            return client.post_discussion(
                discussion["path"],
                discussion["line"],
                discussion["message"],
                discussion["diff_refs"],
            )
        return client.post_note(discussion["message"])

    return post


def format_comment(comment):
    """Format a review comment for an inline GitLab discussion."""
    body = comment.get("content", "")
    existing = comment.get("existing_code", "")
    suggestion = comment.get("suggestion_code", "")
    if suggestion and existing:
        body += "\n\n**Suggestion:**\n"
        body += "```suggestion:-0+0\n%s\n```" % suggestion
    return body


def format_comment_fallback(comment):
    """Format a review comment for a non-inline fallback note."""
    path = comment.get("path", "unknown")
    start_line = comment.get("start_line", 0)
    end_line = comment.get("end_line", 0)
    markdown = "### 📄 `%s`" % path
    if start_line and end_line:
        markdown += " (L%d-L%d)" % (start_line, end_line)
    markdown += "\n\n" + comment.get("content", "")

    existing = comment.get("existing_code", "")
    suggestion = comment.get("suggestion_code", "")
    if suggestion and existing:
        markdown += "\n\n<details><summary>💡 Suggested Change</summary>\n\n"
        markdown += "**Before:**\n```\n%s\n```\n\n" % existing
        markdown += "**After:**\n```\n%s\n```\n\n" % suggestion
        markdown += "</details>"
    return markdown


def _post_note_safely(post, message):
    """Post a general note without making a best-effort review fail the CI job."""
    try:
        post({"message": message})
    except Exception as error:  # noqa: BLE001 - preserve the heredoc's behavior
        log("general note failed: %s" % error)


def publish(
    result,
    diff_refs,
    post,
    *,
    sleep=time.sleep,
    success_delay=2.0,
    failure_delay=1.0,
    rate_limit_threshold=10,
):
    """Publish a review using an injectable, transport-agnostic poster.

    ``post`` receives a dict with ``message`` for a general note. Inline
    discussions additionally receive ``path``, ``line``, and ``diff_refs``.
    It returns optional response metadata and raises on failure.
    """
    comments = result.get("comments") or []
    if not comments:
        message = result.get("message") or "No comments generated. Looks good to me."
        _post_note_safely(post, "✅ **OpenCodeReview**: " + message)
        return {"inline": 0, "fallback": 0}

    inline_count = 0
    failed_comments = []
    for comment in comments:
        path = comment.get("path", "")
        end_line = comment.get("end_line", 0) or 0
        if not path or not end_line or not diff_refs:
            failed_comments.append(comment)
            continue

        discussion = {
            "message": format_comment(comment),
            "path": path,
            "line": end_line,
            "diff_refs": diff_refs,
        }
        try:
            response = post(discussion) or {}
        except Exception as error:  # noqa: BLE001 - any transport error falls back
            log("inline comment failed for %s:%d: %s" % (path, end_line, error))
            failed_comments.append(comment)
            pacing_delay = (
                success_delay
                if getattr(error, "is_rate_limit_exhausted", False)
                else failure_delay
            )
            # A fully exhausted rate limit warrants the longer configured
            # pacing interval before attempting the next inline comment.
            sleep(pacing_delay)
            continue

        inline_count += 1
        remaining = response.get("rate_limit_remaining")
        if (
            rate_limit_threshold > 0
            and remaining is not None
            and remaining <= rate_limit_threshold
        ):
            delay = success_delay * 2
            log(
                "Rate limit quota low (%d remaining), increasing pacing delay to %.1fs"
                % (remaining, delay)
            )
        else:
            delay = success_delay
        sleep(delay)

    if failed_comments:
        fallback = (
            "🔍 **OpenCodeReview** found issues that could not be posted inline:"
            "\n\n---\n\n"
        )
        for comment in failed_comments:
            fallback += format_comment_fallback(comment) + "\n\n---\n\n"
        _post_note_safely(post, fallback)

    summary = "🔍 **OpenCodeReview** found **%d** issue(s) in this MR." % len(comments)
    summary += "\n- ✅ %d posted as inline comment(s)" % inline_count
    summary += "\n- 📝 %d posted as summary (missing line info)" % len(failed_comments)
    warnings = result.get("warnings") or []
    if warnings:
        summary += "\n\n⚠️ %d warning(s) occurred during review." % len(warnings)
    _post_note_safely(post, summary)
    return {"inline": inline_count, "fallback": len(failed_comments)}


def load_result(path):
    """Read the JSON result emitted by ``ocr review``."""
    with Path(path).open("r", encoding="utf-8") as result_file:
        return json.load(result_file)


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("result_file", help="JSON file emitted by ocr review")
    parser.add_argument(
        "--stderr-file",
        default="/tmp/ocr-stderr.log",
        help="OCR stderr log used when the JSON result cannot be parsed",
    )
    return parser.parse_args(argv)


def main(argv=None, environ=None):
    """CLI entry point used by the GitLab pipeline."""
    args = parse_args(argv)
    environ = os.environ if environ is None else environ
    try:
        auth_header, token = select_auth(environ)
        project_id = environ["CI_PROJECT_ID"]
        mr_iid = environ["CI_MERGE_REQUEST_IID"]
    except (KeyError, ValueError) as error:
        log("ERROR: %s" % error)
        return 1

    gitlab_url = environ.get("CI_SERVER_URL", "https://gitlab.com")
    api_base = "%s/api/v4/projects/%s/merge_requests/%s" % (
        gitlab_url.rstrip("/"),
        project_id,
        mr_iid,
    )
    settings = RetrySettings.from_environ(environ)
    client = GitLabClient(api_base, auth_header, token, settings)
    post = make_poster(client)

    try:
        result = load_result(args.result_file)
    except (FileNotFoundError, json.JSONDecodeError) as error:
        log("Failed to parse OCR output: %s" % error)
        try:
            stderr_content = Path(args.stderr_file).read_text(encoding="utf-8").strip()
        except FileNotFoundError:
            stderr_content = ""
        if stderr_content:
            try:
                post(
                    {
                        "message": (
                            "⚠️ **OpenCodeReview** encountered an error:\n```\n%s\n```"
                            % stderr_content
                        )
                    }
                )
            except GitLabAPIError as post_error:
                log(str(post_error))
        return 0

    comments = result.get("comments") or []
    diff_refs = None
    if comments:
        try:
            diff_refs = client.get_diff_refs()
        except GitLabAPIError as error:
            log("Warning: Could not fetch MR versions: %s" % error)
        if not diff_refs:
            log("Warning: Inline comments will use fallback.")

    try:
        stats = publish(
            result,
            diff_refs,
            post,
            sleep=time.sleep,
            success_delay=settings.success_delay,
            failure_delay=settings.failure_delay,
            rate_limit_threshold=settings.rate_limit_threshold,
        )
    except GitLabAPIError as error:
        log(str(error))
        return 1

    if comments:
        print(
            "Successfully posted %d/%d inline comments."
            % (stats["inline"], len(comments))
        )
    else:
        print("No review comments to post.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

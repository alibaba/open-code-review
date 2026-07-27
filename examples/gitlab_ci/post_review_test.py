#!/usr/bin/env python3
"""Offline tests for the GitLab review publisher.

Run from this directory with ``python3 post_review_test.py`` or from the
repository root with::

    python3 -m unittest discover -s examples/gitlab_ci -p '*_test.py'
"""

import io
import json
import unittest
import urllib.error

import post_review as pr

DIFF_REFS = {
    "base_sha": "base",
    "start_sha": "start",
    "head_sha": "head",
}


class Recorder:
    """Record publisher calls and optionally fail inline discussions."""

    def __init__(
        self,
        *,
        fail_inline=False,
        fail_notes=False,
        remaining=None,
        rate_limited=False,
    ):
        self.calls = []
        self.fail_inline = fail_inline
        self.fail_notes = fail_notes
        self.remaining = remaining
        self.rate_limited = rate_limited

    def __call__(self, discussion):
        if self.fail_inline and "path" in discussion:
            raise pr.GitLabAPIError(
                "simulated failure",
                is_rate_limit_exhausted=self.rate_limited,
            )
        if self.fail_notes and "path" not in discussion:
            raise pr.GitLabAPIError("simulated note failure")
        self.calls.append(discussion)
        return {"rate_limit_remaining": self.remaining}


class FakeResponse:
    """Minimal context-manager response used by GitLabClient tests."""

    def __init__(self, data, headers=None):
        self._body = json.dumps(data).encode("utf-8")
        self.headers = headers or {}

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        return False

    def read(self):
        return self._body


class FakeUrlOpen:
    """Return or raise queued responses while recording requests."""

    def __init__(self, *results):
        self.results = list(results)
        self.requests = []

    def __call__(self, request):
        self.requests.append(request)
        result = self.results.pop(0)
        if isinstance(result, BaseException):
            raise result
        return result


def http_error(code, body="error", headers=None):
    return urllib.error.HTTPError(
        "https://gitlab.example/api",
        code,
        "error",
        headers or {},
        io.BytesIO(body.encode("utf-8")),
    )


class FormattingTest(unittest.TestCase):
    def test_inline_without_suggestion(self):
        self.assertEqual(pr.format_comment({"content": "finding"}), "finding")

    def test_inline_with_suggestion(self):
        output = pr.format_comment(
            {
                "content": "finding",
                "existing_code": "old()",
                "suggestion_code": "new()",
            }
        )
        self.assertIn("finding", output)
        self.assertIn("```suggestion:-0+0\nnew()\n```", output)

    def test_fallback_with_location_and_suggestion(self):
        output = pr.format_comment_fallback(
            {
                "path": "main.py",
                "start_line": 2,
                "end_line": 4,
                "content": "finding",
                "existing_code": "old()",
                "suggestion_code": "new()",
            }
        )
        self.assertIn("`main.py` (L2-L4)", output)
        self.assertIn("**Before:**", output)
        self.assertIn("**After:**", output)


class PublishTest(unittest.TestCase):
    def test_inline_success_then_summary(self):
        result = {
            "comments": [
                {
                    "path": "main.py",
                    "start_line": 7,
                    "end_line": 8,
                    "content": "finding",
                }
            ]
        }
        sleeps = []
        recorder = Recorder(remaining=50)

        stats = pr.publish(
            result,
            DIFF_REFS,
            recorder,
            sleep=sleeps.append,
            success_delay=0.25,
        )

        self.assertEqual(stats, {"inline": 1, "fallback": 0})
        self.assertEqual(len(recorder.calls), 2)
        inline, summary = recorder.calls
        self.assertEqual(inline["path"], "main.py")
        self.assertEqual(inline["line"], 8)
        self.assertEqual(inline["diff_refs"], DIFF_REFS)
        self.assertIn("**1** issue(s)", summary["message"])
        self.assertEqual(sleeps, [0.25])

    def test_missing_diff_refs_uses_fallback_then_summary(self):
        result = {
            "comments": [
                {
                    "path": "main.py",
                    "start_line": 7,
                    "end_line": 8,
                    "content": "finding",
                }
            ],
            "warnings": [{"message": "partial review"}],
        }
        recorder = Recorder()

        stats = pr.publish(result, None, recorder, sleep=lambda _delay: None)

        self.assertEqual(stats, {"inline": 0, "fallback": 1})
        self.assertEqual(len(recorder.calls), 2)
        self.assertIn("could not be posted inline", recorder.calls[0]["message"])
        self.assertIn("1 warning(s)", recorder.calls[1]["message"])

    def test_inline_error_uses_failure_delay_and_fallback(self):
        result = {
            "comments": [
                {
                    "path": "main.py",
                    "start_line": 1,
                    "end_line": 1,
                    "content": "finding",
                }
            ]
        }
        sleeps = []
        recorder = Recorder(fail_inline=True)

        stats = pr.publish(
            result,
            DIFF_REFS,
            recorder,
            sleep=sleeps.append,
            failure_delay=0.5,
        )

        self.assertEqual(stats, {"inline": 0, "fallback": 1})
        self.assertEqual(sleeps, [0.5])
        self.assertEqual(len(recorder.calls), 2)

    def test_exhausted_rate_limit_uses_success_delay(self):
        result = {
            "comments": [
                {
                    "path": "main.py",
                    "end_line": 1,
                    "content": "finding",
                }
            ]
        }
        sleeps = []
        recorder = Recorder(fail_inline=True, rate_limited=True)

        pr.publish(
            result,
            DIFF_REFS,
            recorder,
            sleep=sleeps.append,
            success_delay=2.0,
            failure_delay=1.0,
        )

        self.assertEqual(sleeps, [2.0])

    def test_low_quota_doubles_success_delay(self):
        result = {
            "comments": [
                {
                    "path": "main.py",
                    "end_line": 1,
                    "content": "finding",
                }
            ]
        }
        sleeps = []

        pr.publish(
            result,
            DIFF_REFS,
            Recorder(remaining=3),
            sleep=sleeps.append,
            success_delay=1.5,
            rate_limit_threshold=10,
        )

        self.assertEqual(sleeps, [3.0])

    def test_no_comments_posts_only_success_note(self):
        recorder = Recorder()

        stats = pr.publish(
            {"message": "Looks good."},
            None,
            recorder,
            sleep=lambda _delay: None,
        )

        self.assertEqual(stats, {"inline": 0, "fallback": 0})
        self.assertEqual(len(recorder.calls), 1)
        self.assertIn("Looks good.", recorder.calls[0]["message"])

    def test_general_note_failure_remains_best_effort(self):
        stats = pr.publish(
            {"message": "Looks good."},
            None,
            Recorder(fail_notes=True),
            sleep=lambda _delay: None,
        )

        self.assertEqual(stats, {"inline": 0, "fallback": 0})


class AuthenticationTest(unittest.TestCase):
    def test_private_token_is_preferred(self):
        self.assertEqual(
            pr.select_auth({"GITLAB_API_TOKEN": "private", "CI_JOB_TOKEN": "job"}),
            ("PRIVATE-TOKEN", "private"),
        )

    def test_job_token_is_fallback(self):
        self.assertEqual(
            pr.select_auth({"CI_JOB_TOKEN": "job"}),
            ("JOB-TOKEN", "job"),
        )

    def test_missing_token_is_rejected(self):
        with self.assertRaises(ValueError):
            pr.select_auth({})


class RetrySettingsTest(unittest.TestCase):
    def test_all_documented_environment_overrides(self):
        settings = pr.RetrySettings.from_environ(
            {
                "OCR_RETRY_BASE_DELAY": "1250",
                "OCR_MAX_RETRIES": "7",
                "OCR_MAX_RETRY_DELAY": "9000",
                "OCR_SUCCESS_DELAY": "250",
                "OCR_FAILURE_DELAY": "500",
                "OCR_RATE_LIMIT_THRESHOLD": "4",
            }
        )

        self.assertEqual(settings.retry_base_delay, 1.25)
        self.assertEqual(settings.max_retries, 7)
        self.assertEqual(settings.max_retry_delay, 9.0)
        self.assertEqual(settings.success_delay, 0.25)
        self.assertEqual(settings.failure_delay, 0.5)
        self.assertEqual(settings.rate_limit_threshold, 4)


class GitLabClientTest(unittest.TestCase):
    def make_client(self, urlopen, sleeps, **setting_overrides):
        settings = pr.RetrySettings(
            retry_base_delay=setting_overrides.get("retry_base_delay", 2.0),
            max_retries=setting_overrides.get("max_retries", 2),
            max_retry_delay=setting_overrides.get("max_retry_delay", 5.0),
            success_delay=0,
            failure_delay=0,
            rate_limit_threshold=10,
        )
        return pr.GitLabClient(
            "https://gitlab.example/api/v4/projects/1/merge_requests/2",
            "PRIVATE-TOKEN",
            "secret",
            settings,
            urlopen=urlopen,
            sleep=sleeps.append,
            random_value=lambda: 0.5,
        )

    def test_success_parses_quota_and_auth_header(self):
        opener = FakeUrlOpen(
            FakeResponse(
                {"id": 1},
                {"ratelimit-remaining": "9", "RateLimit-Limit": "100"},
            )
        )
        client = self.make_client(opener, [])

        response = client.request("/notes", {"body": "hello"})

        self.assertEqual(response["data"], {"id": 1})
        self.assertEqual(response["rate_limit_remaining"], 9)
        request = opener.requests[0]
        self.assertEqual(request.get_header("Private-token"), "secret")
        self.assertEqual(request.get_method(), "POST")

    def test_retry_after_is_honored_and_capped(self):
        sleeps = []
        opener = FakeUrlOpen(
            http_error(429, headers={"Retry-After": "120"}),
            FakeResponse({"id": 1}),
        )
        client = self.make_client(opener, sleeps, max_retry_delay=5.0)

        client.request("/notes", {"body": "hello"})

        self.assertEqual(sleeps, [5.0])
        self.assertEqual(len(opener.requests), 2)

    def test_invalid_retry_after_uses_exponential_rate_limit_delay(self):
        sleeps = []
        opener = FakeUrlOpen(
            http_error(
                403,
                body="rate limit reached; retry later",
                headers={"Retry-After": "tomorrow"},
            ),
            FakeResponse({"id": 1}),
        )
        client = self.make_client(opener, sleeps, retry_base_delay=1.25)

        client.request("/notes", {"body": "hello"})

        self.assertEqual(sleeps, [1.25])

    def test_transient_statuses_use_short_backoff(self):
        for code in (408, 500, 503):
            with self.subTest(code=code):
                sleeps = []
                opener = FakeUrlOpen(
                    http_error(code),
                    FakeResponse({"id": 1}),
                )
                client = self.make_client(opener, sleeps)

                client.request("/versions", method="GET")

                self.assertEqual(sleeps, [2.0])
                self.assertEqual(opener.requests[0].get_method(), "GET")

    def test_transient_status_respects_configured_retry_base_delay(self):
        sleeps = []
        opener = FakeUrlOpen(
            http_error(503),
            FakeResponse({"id": 1}),
        )
        client = self.make_client(opener, sleeps, retry_base_delay=1.25)

        client.request("/versions", method="GET")

        self.assertEqual(sleeps, [1.25])

    def test_exhausted_rate_limit_is_classified(self):
        opener = FakeUrlOpen(http_error(429))
        client = self.make_client(opener, [], max_retries=0)

        with self.assertRaises(pr.GitLabAPIError) as raised:
            client.request("/notes", {"body": "hello"})

        self.assertTrue(raised.exception.is_rate_limit_exhausted)

    def test_non_retryable_error_fails_immediately(self):
        opener = FakeUrlOpen(http_error(400, body="bad request"))
        client = self.make_client(opener, [])

        with self.assertRaises(pr.GitLabAPIError) as raised:
            client.request("/notes", {"body": "hello"})

        self.assertFalse(raised.exception.is_rate_limit_exhausted)
        self.assertEqual(len(opener.requests), 1)

    def test_get_diff_refs_uses_latest_version(self):
        opener = FakeUrlOpen(
            FakeResponse(
                [
                    {
                        "base_commit_sha": "base",
                        "start_commit_sha": "start",
                        "head_commit_sha": "head",
                    }
                ]
            )
        )
        client = self.make_client(opener, [])

        self.assertEqual(client.get_diff_refs(), DIFF_REFS)


if __name__ == "__main__":
    unittest.main()

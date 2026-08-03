#!/usr/bin/env python3
"""Tests for post_review.py.

Standard-library unittest only (no pytest, no network, no real time.sleep): run with

    python3 post_review_test.py                                     # from examples/gitlab_ci/
    python3 -m unittest discover -s examples/gitlab_ci -p '*_test.py'

Two test seams:

  1. **publish()** driven through a ``Recorder`` fake poster — exercises the
     full inline→fallback→summary flow with no HTTP and no wall-clock cost.

  2. **make_poster()** / **fetch_diff_refs()** with mocked ``urlopen`` and
     ``_sleep`` — exercises the retry/backoff/jitter logic with canned
     ``HTTPError`` sequences.
"""

import io
import json
import os
import random
import socket
import sys
import tempfile
import unittest
import urllib.error
from unittest import mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import post_review as pr  # noqa: E402


# --------------------------------------------------------------------------- #
# Helpers
# --------------------------------------------------------------------------- #


def comment(**overrides):
    """One OCR comment as ``ocr review --format json`` emits it."""
    c = {
        "path": "main.py",
        "content": "possible issue",
        "start_line": 10,
        "end_line": 10,
    }
    c.update(overrides)
    return c


DIFF_REFS = {
    "base_sha": "abc123",
    "start_sha": "def456",
    "head_sha": "ghi789",
}


DEFAULT_CONFIG = {
    "success_delay": 2.0,
    "failure_delay": 1.0,
    "rate_limit_threshold": 10,
    "retry_base_delay": 2.0,
    "max_retries": 3,
    "max_retry_delay": 60.0,
    "transient_base_delay": 2.0,
    "route_severity_below": "",
    "route_categories": "",
}


NOOP_SLEEP = lambda _s: None  # noqa: E731


def http_error(code, body=b"", reason="error", headers=None):
    """Build an HTTPError suitable for mocking urlopen."""
    return urllib.error.HTTPError(
        "https://gitlab.example/api/v4", code, reason, headers or {}, io.BytesIO(body),
    )


class FakeResponse:
    """A fake urllib response object."""

    def __init__(self, body=b"{}", headers=None):
        self._body = body
        self.headers = headers or {}

    def read(self):
        return self._body

    def __enter__(self):
        return self

    def __exit__(self, *args):
        return False


# --------------------------------------------------------------------------- #
# Comment formatting
# --------------------------------------------------------------------------- #


class FormatCommentTest(unittest.TestCase):
    def test_plain_content(self):
        body = pr.format_comment({"content": "hello world"})
        self.assertEqual(body, "hello world")

    def test_badge_prefix(self):
        body = pr.format_comment(comment(content="issue", category="bug", severity="high"))
        self.assertTrue(body.startswith("[bug · high]\n"))
        self.assertIn("issue", body)

    def test_with_suggestion(self):
        body = pr.format_comment(comment(
            content="fix this",
            existing_code="x = 1",
            suggestion_code="x = 2",
        ))
        self.assertIn("fix this", body)
        self.assertIn("```suggestion:-0+0\nx = 2\n```", body)
        self.assertIn("**Suggestion:**", body)

    def test_suggestion_without_existing(self):
        body = pr.format_comment(comment(
            content="fix this",
            suggestion_code="x = 2",
        ))
        self.assertNotIn("```suggestion", body)

    def test_existing_without_suggestion(self):
        body = pr.format_comment(comment(
            content="fix this",
            existing_code="x = 1",
        ))
        self.assertNotIn("```suggestion", body)


class FormatCommentFallbackTest(unittest.TestCase):
    def test_plain_content(self):
        md = pr.format_comment_fallback({"content": "hello", "path": "a.py"})
        self.assertIn("### 📄 `a.py`", md)
        self.assertIn("hello", md)

    def test_with_line_range(self):
        md = pr.format_comment_fallback({
            "content": "issue",
            "path": "a.py",
            "start_line": 3,
            "end_line": 7,
        })
        self.assertIn("(L3-L7)", md)

    def test_with_suggestion(self):
        md = pr.format_comment_fallback(comment(
            content="fix this",
            existing_code="x = 1",
            suggestion_code="x = 2",
        ))
        self.assertIn("<details><summary>💡 Suggested Change</summary>", md)
        self.assertIn("**Before:**\n```\nx = 1\n```", md)
        self.assertIn("**After:**\n```\nx = 2\n```", md)
        self.assertIn("</details>", md)

    def test_badge_prefix(self):
        md = pr.format_comment_fallback(comment(
            content="issue",
            category="style",
            severity="low",
        ))
        self.assertTrue(md.startswith("[style · low]\n"))

    def test_reason_appended(self):
        md = pr.format_comment_fallback(comment(content="issue"), reason="Routed to summary (severity low)")
        self.assertIn("*Routed to summary (severity low)*", md)

    def test_suggestion_without_existing(self):
        md = pr.format_comment_fallback(comment(
            content="fix this",
            suggestion_code="x = 2",
        ))
        self.assertNotIn("<details>", md)


# --------------------------------------------------------------------------- #
# Badge + publication policy
# --------------------------------------------------------------------------- #


class BuildBadgeTest(unittest.TestCase):
    def test_both(self):
        self.assertEqual(pr.build_badge({"category": "bug", "severity": "high"}), "[bug · high]")

    def test_category_only(self):
        self.assertEqual(pr.build_badge({"category": "style"}), "[style]")

    def test_severity_only(self):
        self.assertEqual(pr.build_badge({"severity": "low"}), "[low]")

    def test_neither(self):
        self.assertEqual(pr.build_badge({}), "")

    def test_preserves_case(self):
        self.assertEqual(pr.build_badge({"category": "Bug", "severity": "High"}), "[Bug · High]")

    def test_strips_control_chars(self):
        self.assertEqual(pr.build_badge({"category": "bu\ng", "severity": "high"}), "[bug · high]")


class BuildPolicyTest(unittest.TestCase):
    def test_empty_is_no_routing(self):
        self.assertEqual(pr.build_policy(), pr.NO_ROUTING)
        self.assertEqual(pr.build_policy(severity_threshold="", categories=""), pr.NO_ROUTING)

    def test_unknown_severity_fails_open(self):
        self.assertEqual(pr.build_policy(severity_threshold="trivial"), pr.NO_ROUTING)

    def test_medium_threshold(self):
        policy = pr.build_policy(severity_threshold="medium")
        self.assertTrue(policy["route_by_severity"])
        self.assertEqual(policy["severity_rank"], pr.SEVERITY_RANK["medium"])

    def test_known_categories(self):
        policy = pr.build_policy(categories="style, UNKNOWN, documentation")
        self.assertTrue(policy["route_by_category"])
        self.assertIn("style", policy["categories"])
        self.assertIn("documentation", policy["categories"])
        self.assertNotIn("unknown", policy["categories"])


class RouteCommentTest(unittest.TestCase):
    def setUp(self):
        self.policy = pr.build_policy(severity_threshold="medium", categories="style,documentation")

    def test_severity_at_or_below_threshold(self):
        self.assertTrue(pr.route_comment({"severity": "medium"}, self.policy)["routed"])
        self.assertTrue(pr.route_comment({"severity": "low"}, self.policy)["routed"])
        self.assertFalse(pr.route_comment({"severity": "high"}, self.policy)["routed"])

    def test_unknown_severity_stays_inline(self):
        self.assertFalse(pr.route_comment({"severity": "trivial"}, self.policy)["routed"])
        self.assertFalse(pr.route_comment({"severity": ""}, self.policy)["routed"])

    def test_category_routing(self):
        self.assertTrue(pr.route_comment({"category": "style"}, self.policy)["routed"])
        self.assertFalse(pr.route_comment({"category": "bug"}, self.policy)["routed"])

    def test_unknown_category_stays_inline(self):
        self.assertFalse(pr.route_comment({"category": "unknown"}, self.policy)["routed"])

    def test_no_routing_sentinel(self):
        self.assertFalse(pr.route_comment({"severity": "low", "category": "style"}, pr.NO_ROUTING)["routed"])


# --------------------------------------------------------------------------- #
# publish() with Recorder fake poster (Seam 1)
# --------------------------------------------------------------------------- #


class Recorder:
    """A fake ``post(discussion)`` that records calls and simulates outcomes.

    Each outcome is either a dict (returned as-is) or an Exception (raised).
    If outcomes run out, returns a default success.
    """

    def __init__(self, outcomes=None):
        self.calls = []
        self.outcomes = list(outcomes or [])
        self.sleeps = []

    def __call__(self, discussion):
        self.calls.append(discussion)
        if self.outcomes:
            outcome = self.outcomes.pop(0)
            if isinstance(outcome, Exception):
                raise outcome
            return outcome
        return {"success": True, "rate_limit_remaining": None, "is_rate_limit_exhausted": False}


def run_publish(result, diff_refs=DIFF_REFS, config=None, outcomes=None):
    """Helper: run publish() with a Recorder and no-op sleep."""
    rec = Recorder(outcomes)
    cfg = config or DEFAULT_CONFIG
    stats = pr.publish(result, diff_refs, rec, cfg, sleep=rec.sleeps.append)
    return stats, rec


class PublishTest(unittest.TestCase):
    def test_inline_success_and_summary(self):
        result = {"comments": [comment()]}
        stats, rec = run_publish(result)

        self.assertEqual(stats, {"inline": 1, "fallback": 0, "routed": 0, "failed": 0})
        # 2 calls: inline discussion + summary note
        self.assertEqual(len(rec.calls), 2)

        inline = rec.calls[0]
        self.assertIn("position", inline)
        self.assertEqual(inline["position"]["new_path"], "main.py")
        self.assertEqual(inline["position"]["new_line"], 10)
        self.assertEqual(inline["position"]["base_sha"], "abc123")
        self.assertIn("possible issue", inline["body"])

        summary = rec.calls[1]
        self.assertNotIn("position", summary)
        self.assertIn("**1** issue(s)", summary["body"])
        self.assertIn("Successfully posted inline: 1", summary["body"])

    def test_fallback_when_diff_refs_none(self):
        result = {"comments": [comment()]}
        stats, rec = run_publish(result, diff_refs=None)

        self.assertEqual(stats, {"inline": 0, "fallback": 1, "routed": 0, "failed": 0})
        # 2 calls: no-line note + summary
        self.assertEqual(len(rec.calls), 2)
        self.assertIn("could not be posted inline", rec.calls[0]["body"])
        self.assertIn("In summary (no line info): 1", rec.calls[1]["body"])

    def test_inline_error_falls_back(self):
        result = {"comments": [comment()]}
        outcomes = [{"success": False, "rate_limit_remaining": None, "is_rate_limit_exhausted": False}]
        stats, rec = run_publish(result, outcomes=outcomes)

        self.assertEqual(stats, {"inline": 0, "fallback": 0, "routed": 0, "failed": 1})
        # 3 calls: failed inline attempt + failed note + summary
        self.assertEqual(len(rec.calls), 3)
        self.assertIn("failed to post inline", rec.calls[1]["body"].lower())

    def test_no_comments_lgtm(self):
        result = {"message": "Looks good to me."}
        stats, rec = run_publish(result)

        self.assertEqual(stats, {"inline": 0, "fallback": 0, "routed": 0, "failed": 0})
        self.assertEqual(len(rec.calls), 1)
        self.assertIn("Looks good to me", rec.calls[0]["body"])
        self.assertIn("✅", rec.calls[0]["body"])

    def test_warnings_in_summary(self):
        result = {
            "comments": [comment()],
            "warnings": [{"file": "a.py", "message": "skipped"}],
        }
        stats, rec = run_publish(result)
        summary = rec.calls[-1]["body"]
        self.assertIn("1 warning(s)", summary)

    def test_success_pacing(self):
        result = {"comments": [comment()]}
        outcomes = [{"success": True, "rate_limit_remaining": 100, "is_rate_limit_exhausted": False}]
        stats, rec = run_publish(result, outcomes=outcomes)
        # success_delay = 2.0, remaining=100 > threshold=10 → normal pace
        self.assertEqual(rec.sleeps, [2.0])

    def test_proactive_throttling(self):
        result = {"comments": [comment()]}
        outcomes = [{"success": True, "rate_limit_remaining": 5, "is_rate_limit_exhausted": False}]
        stats, rec = run_publish(result, outcomes=outcomes)
        # remaining=5 ≤ threshold=10 → doubled success_delay = 4.0
        self.assertEqual(rec.sleeps, [4.0])

    def test_failure_pacing_rate_limit(self):
        result = {"comments": [comment()]}
        outcomes = [{"success": False, "rate_limit_remaining": None, "is_rate_limit_exhausted": True}]
        stats, rec = run_publish(result, outcomes=outcomes)
        # rate_limit_exhausted → success_delay = 2.0
        self.assertEqual(rec.sleeps, [2.0])

    def test_failure_pacing_non_rate_limit(self):
        result = {"comments": [comment()]}
        outcomes = [{"success": False, "rate_limit_remaining": None, "is_rate_limit_exhausted": False}]
        stats, rec = run_publish(result, outcomes=outcomes)
        # not rate_limit → failure_delay = 1.0
        self.assertEqual(rec.sleeps, [1.0])

    def test_proactive_throttling_disabled(self):
        config = dict(DEFAULT_CONFIG)
        config["rate_limit_threshold"] = 0
        result = {"comments": [comment()]}
        outcomes = [{"success": True, "rate_limit_remaining": 1, "is_rate_limit_exhausted": False}]
        stats, rec = run_publish(result, config=config, outcomes=outcomes)
        # threshold=0 disables throttling → normal success_delay
        self.assertEqual(rec.sleeps, [2.0])

    def test_multiple_comments_some_fail(self):
        result = {"comments": [
            comment(path="a.py", end_line=1),
            comment(path="b.py", end_line=2),
            comment(path="c.py", end_line=3),
        ]}
        outcomes = [
            {"success": True, "rate_limit_remaining": 100, "is_rate_limit_exhausted": False},
            {"success": False, "rate_limit_remaining": None, "is_rate_limit_exhausted": False},
            {"success": True, "rate_limit_remaining": 100, "is_rate_limit_exhausted": False},
        ]
        stats, rec = run_publish(result, outcomes=outcomes)
        self.assertEqual(stats, {"inline": 2, "fallback": 0, "routed": 0, "failed": 1})
        # 5 calls: 3 inline attempts + failed note + summary
        self.assertEqual(len(rec.calls), 5)

    def test_comment_without_path_falls_back(self):
        result = {"comments": [comment(path="", end_line=0)]}
        stats, rec = run_publish(result)
        self.assertEqual(stats, {"inline": 0, "fallback": 1, "routed": 0, "failed": 0})

    def test_comment_without_end_line_falls_back(self):
        result = {"comments": [comment(end_line=0)]}
        stats, rec = run_publish(result)
        self.assertEqual(stats, {"inline": 0, "fallback": 1, "routed": 0, "failed": 0})

    def test_severity_routing_to_summary(self):
        config = dict(DEFAULT_CONFIG)
        config["route_severity_below"] = "medium"
        result = {"comments": [comment(severity="low", category="bug")]}
        stats, rec = run_publish(result, config=config)

        self.assertEqual(stats, {"inline": 0, "fallback": 0, "routed": 1, "failed": 0})
        self.assertEqual(len(rec.calls), 2)
        self.assertIn("routed to summary by policy", rec.calls[0]["body"].lower())
        self.assertIn("Routed to summary (severity low", rec.calls[0]["body"])
        self.assertIn("Routed to summary by policy: 1", rec.calls[1]["body"])

    def test_category_routing_to_summary(self):
        config = dict(DEFAULT_CONFIG)
        config["route_categories"] = "style"
        result = {"comments": [comment(category="style", severity="high")]}
        stats, rec = run_publish(result, config=config)

        self.assertEqual(stats, {"inline": 0, "fallback": 0, "routed": 1, "failed": 0})
        self.assertIn("Routed to summary (category style", rec.calls[0]["body"])

    def test_unknown_metadata_stays_inline(self):
        config = dict(DEFAULT_CONFIG)
        config["route_severity_below"] = "medium"
        config["route_categories"] = "style"
        result = {"comments": [comment(severity="trivial", category="unknown")]}
        stats, rec = run_publish(result, config=config)

        self.assertEqual(stats, {"inline": 1, "fallback": 0, "routed": 0, "failed": 0})
        self.assertIn("position", rec.calls[0])


# --------------------------------------------------------------------------- #
# make_poster() with mocked urlopen + _sleep (Seam 2)
# --------------------------------------------------------------------------- #


class MakePosterTest(unittest.TestCase):
    API_BASE = "https://gitlab.example/api/v4/projects/1/merge_requests/2"
    TOKEN = "test-token"
    AUTH_HEADER = "PRIVATE-TOKEN"

    def make_post(self):
        """Return a poster configured with DEFAULT_CONFIG."""
        return pr.make_poster(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG)

    def post_seq(self, discussion, outcomes, method="POST"):
        """Drive post() over a sequence of fake urlopen outcomes.

        Each outcome is either bytes (a response body), an HTTPError (raised),
        or a FakeResponse.  Returns (n_calls, result_or_None, raised_or_None).
        """
        calls = {"n": 0}
        outcomes = list(outcomes)

        def fake_urlopen(req, timeout=None):
            calls["n"] += 1
            outcome = outcomes.pop(0)
            if isinstance(outcome, Exception):
                raise outcome
            if isinstance(outcome, FakeResponse):
                return outcome
            return FakeResponse(outcome)

        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen), \
                mock.patch.object(pr, "_sleep", lambda _s: None), \
                mock.patch.object(pr.random, "random", lambda: 0.5):
            post = pr.make_poster(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG)
            try:
                result = post(discussion)
            except Exception as e:  # noqa: BLE001
                return calls["n"], None, e
            return calls["n"], result, None

    # ---- success path ----

    def test_post_note_success(self):
        discussion = {"body": "hello"}
        n, result, raised = self.post_seq(
            discussion, [b'{"id": 1}'],
        )
        self.assertIsNone(raised)
        self.assertTrue(result["success"])
        self.assertEqual(n, 1)

    def test_post_discussion_success(self):
        discussion = {
            "body": "inline comment",
            "position": {"new_path": "a.py", "new_line": 1},
        }
        n, result, raised = self.post_seq(
            discussion, [b'{"id": 1}'],
        )
        self.assertIsNone(raised)
        self.assertTrue(result["success"])

    def test_rate_limit_remaining_parsed(self):
        discussion = {"body": "hello"}
        n, result, raised = self.post_seq(
            discussion,
            [FakeResponse(b'{"id": 1}', headers={"RateLimit-Remaining": "42"})],
        )
        self.assertIsNone(raised)
        self.assertEqual(result["rate_limit_remaining"], 42)

    # ---- retry on rate-limit (429) ----

    def test_retry_429_then_success(self):
        discussion = {"body": "hello"}
        n, result, raised = self.post_seq(
            discussion,
            [http_error(429, b"rate limited"), b'{"id": 1}'],
        )
        self.assertIsNone(raised)
        self.assertTrue(result["success"])
        self.assertEqual(n, 2)

    def test_retry_403_rate_limit_keywords_then_success(self):
        discussion = {"body": "hello"}
        n, result, raised = self.post_seq(
            discussion,
            [http_error(403, b"Rate limit exceeded"), b'{"id": 1}'],
        )
        self.assertIsNone(raised)
        self.assertTrue(result["success"])
        self.assertEqual(n, 2)

    # ---- retry on transient (5xx, 408) ----

    def test_retry_500_then_success(self):
        discussion = {"body": "hello"}
        n, result, raised = self.post_seq(
            discussion,
            [http_error(500, b"server error"), b'{"id": 1}'],
        )
        self.assertIsNone(raised)
        self.assertTrue(result["success"])
        self.assertEqual(n, 2)

    def test_retry_408_then_success(self):
        discussion = {"body": "hello"}
        n, result, raised = self.post_seq(
            discussion,
            [http_error(408, b"timeout"), b'{"id": 1}'],
        )
        self.assertIsNone(raised)
        self.assertTrue(result["success"])
        self.assertEqual(n, 2)

    # ---- no retry on non-retryable errors ----

    def test_no_retry_404(self):
        discussion = {"body": "hello"}
        n, result, raised = self.post_seq(
            discussion,
            [http_error(404, b"not found")],
        )
        self.assertFalse(result["success"])
        self.assertFalse(result["is_rate_limit_exhausted"])
        self.assertEqual(n, 1)

    def test_no_retry_400(self):
        discussion = {"body": "hello"}
        n, result, raised = self.post_seq(
            discussion,
            [http_error(400, b"bad request")],
        )
        self.assertFalse(result["success"])
        self.assertEqual(n, 1)

    def test_no_retry_403_non_rate_limit(self):
        discussion = {"body": "hello"}
        n, result, raised = self.post_seq(
            discussion,
            [http_error(403, b"forbidden")],
        )
        self.assertFalse(result["success"])
        self.assertFalse(result["is_rate_limit_exhausted"])
        self.assertEqual(n, 1)

    # ---- is_rate_limit_exhausted classification ----

    def test_rate_limit_exhausted_on_429(self):
        discussion = {"body": "hello"}
        outcomes = [http_error(429, b"rate limited")] * 4  # max_retries=3 → 4 attempts
        n, result, raised = self.post_seq(discussion, outcomes)
        self.assertFalse(result["success"])
        self.assertTrue(result["is_rate_limit_exhausted"])
        self.assertEqual(n, 4)

    def test_rate_limit_not_exhausted_on_transient(self):
        discussion = {"body": "hello"}
        outcomes = [http_error(500, b"server error")] * 4
        n, result, raised = self.post_seq(discussion, outcomes)
        self.assertFalse(result["success"])
        self.assertFalse(result["is_rate_limit_exhausted"])
        self.assertEqual(n, 4)

    # ---- Retry-After header ----

    def test_honors_retry_after_header(self):
        discussion = {"body": "hello"}
        sleeps = []

        def fake_urlopen(req, timeout=None):
            if not hasattr(fake_urlopen, "_called"):
                fake_urlopen._called = True
                raise http_error(429, b"rate limited", headers={"Retry-After": "5"})
            return FakeResponse(b'{"id": 1}')

        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen), \
                mock.patch.object(pr, "_sleep", sleeps.append), \
                mock.patch.object(pr.random, "random", lambda: 0.5):
            post = pr.make_poster(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG)
            result = post(discussion)

        self.assertTrue(result["success"])
        # delay = 5.0 * (0.75 + 0.5 * 0.5) = 5.0 * 1.0 = 5.0
        self.assertEqual(sleeps, [5.0])

    def test_retry_after_non_numeric_falls_back_to_exponential(self):
        discussion = {"body": "hello"}
        sleeps = []

        def fake_urlopen(req, timeout=None):
            if not hasattr(fake_urlopen, "_called"):
                fake_urlopen._called = True
                raise http_error(429, b"rate limited", headers={"Retry-After": "soon"})
            return FakeResponse(b'{"id": 1}')

        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen), \
                mock.patch.object(pr, "_sleep", sleeps.append), \
                mock.patch.object(pr.random, "random", lambda: 0.5):
            post = pr.make_poster(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG)
            result = post(discussion)

        self.assertTrue(result["success"])
        # delay = retry_base_delay * 2^0 * (0.75 + 0.5*0.5) = 2.0 * 1.0 = 2.0
        self.assertEqual(sleeps, [2.0])

    # ---- delay cap ----

    def test_delay_capped_at_max_retry_delay(self):
        discussion = {"body": "hello"}
        sleeps = []

        config = dict(DEFAULT_CONFIG)
        config["max_retry_delay"] = 3.0  # low cap

        def fake_urlopen(req, timeout=None):
            if not hasattr(fake_urlopen, "_called"):
                fake_urlopen._called = True
                raise http_error(429, b"rate limited", headers={"Retry-After": "999"})
            return FakeResponse(b'{"id": 1}')

        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen), \
                mock.patch.object(pr, "_sleep", sleeps.append), \
                mock.patch.object(pr.random, "random", lambda: 0.5):
            post = pr.make_poster(self.API_BASE, self.TOKEN, self.AUTH_HEADER, config)
            result = post(discussion)

        self.assertTrue(result["success"])
        # delay = min(999, 3.0) * (0.75 + 0.5*0.5) = 3.0 * 1.0 = 3.0
        self.assertEqual(sleeps, [3.0])

    # ---- jitter ----

    def test_jitter_within_25_percent(self):
        """Jitter should produce delays within ±25% of the base delay."""
        discussion = {"body": "hello"}
        sleeps = []

        with mock.patch.object(pr.random, "random", lambda: 0.0):  # min jitter (-25%)
            def fake_urlopen(req, timeout=None):
                if not hasattr(fake_urlopen, "_called"):
                    fake_urlopen._called = True
                    raise http_error(429, b"rate limited")
                return FakeResponse(b'{"id": 1}')

            with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen), \
                    mock.patch.object(pr, "_sleep", sleeps.append):
                post = pr.make_poster(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG)
                post(discussion)

        # delay = 2.0 * 2^0 * (0.75 + 0.0 * 0.5) = 2.0 * 0.75 = 1.5
        self.assertAlmostEqual(sleeps[0], 1.5, places=2)

        # Reset and test max jitter (+25%)
        sleeps.clear()
        fake_urlopen._called = False

        with mock.patch.object(pr.random, "random", lambda: 1.0):  # max jitter (+25%)
            def fake_urlopen2(req, timeout=None):
                if not hasattr(fake_urlopen2, "_called"):
                    fake_urlopen2._called = True
                    raise http_error(429, b"rate limited")
                return FakeResponse(b'{"id": 1}')

            with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen2), \
                    mock.patch.object(pr, "_sleep", sleeps.append):
                post = pr.make_poster(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG)
                post(discussion)

        # delay = 2.0 * 2^0 * (0.75 + 1.0 * 0.5) = 2.0 * 1.25 = 2.5
        self.assertAlmostEqual(sleeps[0], 2.5, places=2)

    # ---- auth header ----

    def test_auth_header_private_token(self):
        captured = {}

        def fake_urlopen(req, timeout=None):
            captured["headers"] = dict(req.header_items())
            return FakeResponse(b'{"id": 1}')

        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen):
            post = pr.make_poster(self.API_BASE, self.TOKEN, "PRIVATE-TOKEN", DEFAULT_CONFIG)
            post({"body": "hello"})

        # urllib normalizes header names to title-case in header_items()
        headers = {k.lower(): v for k, v in captured["headers"].items()}
        self.assertEqual(headers.get("private-token"), self.TOKEN)
        self.assertNotIn("job-token", headers)

    def test_auth_header_job_token(self):
        captured = {}

        def fake_urlopen(req, timeout=None):
            captured["headers"] = dict(req.header_items())
            return FakeResponse(b'{"id": 1}')

        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen):
            post = pr.make_poster(self.API_BASE, self.TOKEN, "JOB-TOKEN", DEFAULT_CONFIG)
            post({"body": "hello"})

        headers = {k.lower(): v for k, v in captured["headers"].items()}
        self.assertEqual(headers.get("job-token"), self.TOKEN)
        self.assertNotIn("private-token", headers)

    # ---- URLError (network-level errors) ----

    def test_retry_urlerror_then_success(self):
        discussion = {"body": "hello"}
        conn_err = urllib.error.URLError(ConnectionRefusedError("refused"))
        n, result, raised = self.post_seq(
            discussion,
            [conn_err, b'{"id": 1}'],
        )
        self.assertIsNone(raised)
        self.assertTrue(result["success"])
        self.assertEqual(n, 2)

    def test_urlerror_exhausts_retries(self):
        discussion = {"body": "hello"}
        conn_err = urllib.error.URLError(ConnectionRefusedError("refused"))
        n, result, raised = self.post_seq(
            discussion,
            [conn_err] * 4,  # max_retries=3 → 4 attempts
        )
        self.assertFalse(result["success"])
        self.assertFalse(result["is_rate_limit_exhausted"])
        self.assertEqual(n, 4)

    def test_urlerror_timeout_not_retried(self):
        discussion = {"body": "hello"}
        timeout_err = urllib.error.URLError(socket.timeout("timed out"))
        n, result, raised = self.post_seq(
            discussion,
            [timeout_err, b'{"id": 1}'],
        )
        self.assertFalse(result["success"])
        self.assertEqual(n, 1)  # no retry on timeout

    # ---- non-UTF-8 error body ----

    def test_non_utf8_error_body_does_not_crash(self):
        """HTTPError with non-UTF-8 body must not raise UnicodeDecodeError."""
        discussion = {"body": "hello"}
        # latin-1 bytes that are invalid UTF-8
        bad_body = b"\xff\xfe<html>Server Error</html>"
        n, result, raised = self.post_seq(
            discussion,
            [http_error(404, bad_body)],
        )
        self.assertFalse(result["success"])
        self.assertEqual(n, 1)
        self.assertIsNone(raised)


# --------------------------------------------------------------------------- #
# fetch_diff_refs() with mocked urlopen (Seam 2 extension)
# --------------------------------------------------------------------------- #


class FetchDiffRefsTest(unittest.TestCase):
    API_BASE = "https://gitlab.example/api/v4/projects/1/merge_requests/2"
    TOKEN = "test-token"
    AUTH_HEADER = "PRIVATE-TOKEN"

    def test_success(self):
        body = json.dumps([{
            "base_commit_sha": "aaa",
            "start_commit_sha": "bbb",
            "head_commit_sha": "ccc",
        }]).encode()

        with mock.patch.object(pr.urllib.request, "urlopen",
                               lambda req, timeout=None: FakeResponse(body)), \
                mock.patch.object(pr, "_sleep", lambda _s: None):
            refs = pr.fetch_diff_refs(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG)

        self.assertEqual(refs, {"base_sha": "aaa", "start_sha": "bbb", "head_sha": "ccc"})

    def test_returns_none_on_failure(self):
        with mock.patch.object(pr.urllib.request, "urlopen",
                               lambda req, timeout=None: (_ for _ in ()).throw(http_error(404, b"not found"))), \
                mock.patch.object(pr, "_sleep", lambda _s: None):
            refs = pr.fetch_diff_refs(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG)

        self.assertIsNone(refs)

    def test_returns_none_on_empty_versions(self):
        with mock.patch.object(pr.urllib.request, "urlopen",
                               lambda req, timeout=None: FakeResponse(b"[]")), \
                mock.patch.object(pr, "_sleep", lambda _s: None):
            refs = pr.fetch_diff_refs(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG)

        self.assertIsNone(refs)

    def test_uses_retry_on_transient(self):
        body = json.dumps([{
            "base_commit_sha": "aaa",
            "start_commit_sha": "bbb",
            "head_commit_sha": "ccc",
        }]).encode()
        calls = {"n": 0}

        def fake_urlopen(req, timeout=None):
            calls["n"] += 1
            if calls["n"] == 1:
                raise http_error(500, b"server error")
            return FakeResponse(body)

        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen), \
                mock.patch.object(pr, "_sleep", lambda _s: None), \
                mock.patch.object(pr.random, "random", lambda: 0.5):
            refs = pr.fetch_diff_refs(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG)

        self.assertEqual(calls["n"], 2)
        self.assertIsNotNone(refs)


# --------------------------------------------------------------------------- #
# Config
# --------------------------------------------------------------------------- #


class BuildConfigTest(unittest.TestCase):
    def test_defaults(self):
        config = pr.build_config({})
        self.assertEqual(config["success_delay"], 2.0)
        self.assertEqual(config["failure_delay"], 1.0)
        self.assertEqual(config["rate_limit_threshold"], 10)
        self.assertEqual(config["retry_base_delay"], 2.0)
        self.assertEqual(config["max_retries"], 3)
        self.assertEqual(config["max_retry_delay"], 60.0)
        self.assertEqual(config["transient_base_delay"], 2)
        self.assertEqual(config["route_severity_below"], "")
        self.assertEqual(config["route_categories"], "")

    def test_env_overrides(self):
        env = {
            "OCR_SUCCESS_DELAY": "5000",
            "OCR_FAILURE_DELAY": "2500",
            "OCR_RATE_LIMIT_THRESHOLD": "5",
            "OCR_RETRY_BASE_DELAY": "3000",
            "OCR_MAX_RETRIES": "5",
            "OCR_MAX_RETRY_DELAY": "120000",
            "OCR_ROUTE_SEVERITY_BELOW": "low",
            "OCR_ROUTE_CATEGORIES": "style,documentation",
        }
        config = pr.build_config(env)
        self.assertEqual(config["success_delay"], 5.0)
        self.assertEqual(config["failure_delay"], 2.5)
        self.assertEqual(config["rate_limit_threshold"], 5)
        self.assertEqual(config["retry_base_delay"], 3.0)
        self.assertEqual(config["max_retries"], 5)
        self.assertEqual(config["max_retry_delay"], 120.0)
        self.assertEqual(config["route_severity_below"], "low")
        self.assertEqual(config["route_categories"], "style,documentation")
        # transient_base_delay is always hardcoded
        self.assertEqual(config["transient_base_delay"], 2)


# --------------------------------------------------------------------------- #
# Dry-run poster
# --------------------------------------------------------------------------- #


class DryRunPosterTest(unittest.TestCase):
    def test_prints_note(self):
        poster = pr.make_dry_run_poster()
        stdout = io.StringIO()
        import contextlib
        with contextlib.redirect_stdout(stdout):
            result = poster({"body": "hello world"})
        self.assertTrue(result["success"])
        self.assertIn("hello world", stdout.getvalue())
        self.assertIn("dry-run", stdout.getvalue())

    def test_prints_inline_discussion(self):
        poster = pr.make_dry_run_poster()
        stdout = io.StringIO()
        import contextlib
        with contextlib.redirect_stdout(stdout):
            result = poster({
                "body": "inline finding",
                "position": {"new_path": "a.py", "new_line": 5},
            })
        self.assertTrue(result["success"])
        output = stdout.getvalue()
        self.assertIn("a.py:5", output)
        self.assertIn("inline finding", output)

    def test_no_http_calls(self):
        """Dry-run poster must not touch urlopen."""
        poster = pr.make_dry_run_poster()
        with mock.patch.object(pr.urllib.request, "urlopen",
                               side_effect=AssertionError("urlopen should not be called")):
            poster({"body": "test"})
            poster({"body": "test2", "position": {"new_path": "x.py", "new_line": 1}})


# --------------------------------------------------------------------------- #
# main() auth-header resolution (end-to-end)
# --------------------------------------------------------------------------- #


class MainAuthHeaderTest(unittest.TestCase):
    """Verify main() resolves PRIVATE-TOKEN vs JOB-TOKEN from env vars."""

    BASE_ENV = {
        "CI_SERVER_URL": "https://gitlab.example",
        "CI_PROJECT_ID": "1",
        "CI_MERGE_REQUEST_IID": "2",
    }

    def run_main_with_captured_poster(self, env_overrides):
        """Run main() with make_poster mocked; return (rc, captured_auth_header)."""
        env = dict(self.BASE_ENV)
        env.update(env_overrides)
        captured = {}

        def fake_make_poster(api_base, token, auth_header, config):
            captured["auth_header"] = auth_header
            captured["token"] = token
            def post(discussion):
                return {"success": True, "rate_limit_remaining": None, "is_rate_limit_exhausted": False}
            return post

        import tempfile as tf
        f = tf.NamedTemporaryFile("w", suffix=".json", delete=False)
        json.dump({"comments": [comment()]}, f)
        f.close()
        self.addCleanup(os.unlink, f.name)

        with mock.patch.dict(os.environ, env, clear=True), \
                mock.patch.object(pr, "make_poster", fake_make_poster), \
                mock.patch.object(pr, "fetch_diff_refs", lambda *a, **kw: DIFF_REFS), \
                mock.patch.object(pr, "_sleep", lambda _s: None):
            rc = pr.main([f.name])
        return rc, captured

    def test_private_token_when_gitlab_api_token_set(self):
        rc, captured = self.run_main_with_captured_poster({
            "GITLAB_API_TOKEN": "glpat-xxxx",
        })
        self.assertEqual(rc, 0)
        self.assertEqual(captured["auth_header"], "PRIVATE-TOKEN")
        self.assertEqual(captured["token"], "glpat-xxxx")

    def test_job_token_when_only_ci_job_token_set(self):
        rc, captured = self.run_main_with_captured_poster({
            "CI_JOB_TOKEN": "ci-job-xxxx",
        })
        self.assertEqual(rc, 0)
        self.assertEqual(captured["auth_header"], "JOB-TOKEN")
        self.assertEqual(captured["token"], "ci-job-xxxx")

    def test_private_token_wins_when_both_set(self):
        rc, captured = self.run_main_with_captured_poster({
            "GITLAB_API_TOKEN": "glpat-xxxx",
            "CI_JOB_TOKEN": "ci-job-xxxx",
        })
        self.assertEqual(rc, 0)
        self.assertEqual(captured["auth_header"], "PRIVATE-TOKEN")
        self.assertEqual(captured["token"], "glpat-xxxx")

    def test_missing_ci_vars_fails_fast(self):
        env = {"GITLAB_API_TOKEN": "glpat-xxxx"}  # missing CI_PROJECT_ID, CI_MERGE_REQUEST_IID
        with mock.patch.dict(os.environ, env, clear=True):
            rc = pr.main(["/tmp/nonexistent.json"])
        self.assertEqual(rc, 1)

    def test_missing_token_fails_fast(self):
        env = dict(self.BASE_ENV)  # has CI_PROJECT_ID and CI_MERGE_REQUEST_IID but no token
        with mock.patch.dict(os.environ, env, clear=True):
            rc = pr.main(["/tmp/nonexistent.json"])
        self.assertEqual(rc, 1)


if __name__ == "__main__":
    unittest.main()

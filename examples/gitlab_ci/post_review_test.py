#!/usr/bin/env python3
"""Tests for post_review.py.

Standard-library unittest only (no pytest, no network, no real time.sleep): run with

    python3 post_review_test.py                                     # from examples/gitlab_ci/
    python3 -m unittest discover -s examples/gitlab_ci -p '*_test.py'

Test seams:

  1. **publish()** driven through a ``Recorder`` fake poster object — exercises
     the full partition→inline→sticky-summary flow with no HTTP and no wall-clock.

  2. **GitLabPoster** (post_note / post_discussion / list_* / retry) with mocked
     ``urlopen`` and ``_sleep`` — exercises retry/backoff/jitter/idempotency with
     canned ``HTTPError`` sequences.
"""

import hashlib
import io
import json
import os
import socket
import sys
import tempfile
import time
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
    # Publication-policy / summary / incremental defaults: legacy-friendly.
    "sticky_summary": False,
    "incremental": False,
    "incremental_overlap_threshold": 0.6,
    "route_severity_below": "",
    "route_categories": "",
    "fail_on_severity": "",
    "run_tag": "42-7",
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
        self.status = 200

    def read(self):
        return self._body

    def __enter__(self):
        return self

    def __exit__(self, *args):
        return False


# --------------------------------------------------------------------------- #
# Fake Poster (Seam 1)
# --------------------------------------------------------------------------- #


class Recorder:
    """A fake Poster that records calls and simulates outcomes.

    - ``post_note``/``post_discussion`` pop configured outcomes; when outcomes
      run out, they return a default success.
    - ``list_notes``/``list_discussions`` return the configured payload (or []
      by default). Set ``notes=None`` to simulate a read-API failure.
    """

    def __init__(self, note_outcomes=None, disc_outcomes=None,
                 notes=None, notes_read_failure=False,
                 discussions=None, disc_read_failure=False, diffs=None,
                 update_outcomes=None):
        self.note_calls = []
        self.disc_calls = []
        self.update_calls = []
        self.list_notes_calls = 0
        self.list_disc_calls = 0
        self.diff_calls = 0
        self.sleeps = []
        self._note_outcomes = list(note_outcomes or [])
        self._disc_outcomes = list(disc_outcomes or [])
        self._update_outcomes = list(update_outcomes or [])
        self._notes = notes if notes is not None else []
        self._notes_fail = notes_read_failure
        self._discussions = discussions if discussions is not None else []
        self._disc_fail = disc_read_failure
        self._diffs = diffs
        self._note_id = 1000

    def post_note(self, body):
        self.note_calls.append(body)
        if self._note_outcomes:
            o = self._note_outcomes.pop(0)
            if isinstance(o, Exception):
                raise o
            return o
        self._note_id += 1
        return {"success": True, "rate_limit_remaining": None,
                "is_rate_limit_exhausted": False,
                "url": "https://gitlab.example/note/%d" % self._note_id,
                "id": self._note_id}

    def update_note(self, note_id, body):
        self.update_calls.append((note_id, body))
        if self._update_outcomes:
            return self._update_outcomes.pop(0)
        return {"success": True, "url": "https://gitlab.example/note/%s" % note_id}

    def list_notes(self):
        self.list_notes_calls += 1
        return None if self._notes_fail else self._notes

    def list_discussions(self):
        self.list_disc_calls += 1
        return None if self._disc_fail else self._discussions

    @property
    def final_summary_body(self):
        """The final summary body across the anchor→finalize two-phase flow.

        With the per-run anchor, the pre-review body lands in ``post_note``
        (note_calls) and the final body lands in ``update_note`` (update_calls).
        LGTM / read-unavailable paths may put the final body directly in
        ``post_note``. This helper returns whichever carries the final body.
        """
        if self.update_calls:
            return self.update_calls[-1][1]
        if self.note_calls:
            return self.note_calls[-1]
        return ""

    @property
    def summary_call_count(self):
        """Total summary note operations (anchor post + finalize update)."""
        return len(self.note_calls) + len(self.update_calls)

    def post_discussion(self, discussion, comment_id=None):
        self.disc_calls.append(discussion)
        if self._disc_outcomes:
            o = self._disc_outcomes.pop(0)
            if isinstance(o, Exception):
                raise o
            return o
        return {"success": True, "reconciled": False,
                "rate_limit_remaining": None, "is_rate_limit_exhausted": False}

    def get_mr_diffs(self):
        self.diff_calls += 1
        return self._diffs if self._diffs is not None else \
            {"files": {}, "known": set(), "complete": False}

    def mr_url(self):
        return None


def run_publish(result, diff_refs=DIFF_REFS, config=None, poster=None):
    """Helper: run publish() with a Recorder and no-op sleep."""
    rec = poster or Recorder()
    cfg = config or DEFAULT_CONFIG
    stats = pr.publish(result, diff_refs, rec, cfg, sleep=rec.sleeps.append)
    return stats, rec


# --------------------------------------------------------------------------- #
# Comment formatting (Group A)
# --------------------------------------------------------------------------- #


class FormatCommentTest(unittest.TestCase):
    def test_plain_content(self):
        self.assertEqual(pr.format_comment({"content": "hello world"}), "hello world")

    def test_badge_prefix(self):
        body = pr.format_comment(comment(content="issue", category="bug", severity="high"))
        self.assertTrue(body.startswith("[bug · high]\n"))
        self.assertIn("issue", body)

    def test_with_suggestion(self):
        body = pr.format_comment(comment(content="fix this", existing_code="x = 1", suggestion_code="x = 2"))
        self.assertIn("fix this", body)
        self.assertIn("```suggestion:-0+0\nx = 2\n```", body)
        self.assertIn("**Suggestion:**", body)

    def test_with_multiline_suggestion(self):
        body = pr.format_comment(comment(content="fix this", existing_code="x = 1\ny = 2", suggestion_code="x = 2\ny = 3", start_line=5, end_line=6))
        self.assertIn("fix this", body)
        self.assertIn("```suggestion:-1+0\nx = 2\ny = 3\n```", body)
        self.assertIn("**Suggestion:**", body)

    def test_with_multiline_suggestion_no_start_line(self):
        body = pr.format_comment(comment(content="fix this", existing_code="x = 1\ny = 2", suggestion_code="x = 2\ny = 3", start_line=None, end_line=5))
        self.assertIn("fix this", body)
        self.assertIn("```suggestion:-0+0\nx = 2\ny = 3\n```", body)
        self.assertIn("**Suggestion:**", body)

    def test_with_multiline_suggestion_no_end_line(self):
        body = pr.format_comment(comment(content="fix this", existing_code="x = 1\ny = 2", suggestion_code="x = 2\ny = 3", start_line=5, end_line=None))
        self.assertIn("fix this", body)
        self.assertIn("```suggestion:-0+0\nx = 2\ny = 3\n```", body)
        self.assertIn("**Suggestion:**", body)

    def test_suggestion_without_existing(self):
        body = pr.format_comment(comment(content="fix this", suggestion_code="x = 2"))
        self.assertNotIn("```suggestion", body)

    def test_with_id_tag(self):
        body = pr.format_comment(comment(content="hi"), "ocr-1-2-deadbeef")
        self.assertTrue(body.startswith("<!-- ocr-1-2-deadbeef -->\n"))
        self.assertIn("hi", body)

    def test_with_badge(self):
        body = pr.format_comment(comment(content="hi", category="bug", severity="high"))
        self.assertIn("[bug · high]\n", body)


class FormatCommentFallbackTest(unittest.TestCase):
    def test_plain_content(self):
        md = pr.format_comment_fallback({"content": "hello", "path": "a.py"})
        self.assertIn("### 📄 `a.py`", md)
        self.assertIn("hello", md)

    def test_with_line_range(self):
        md = pr.format_comment_fallback({"content": "issue", "path": "a.py", "start_line": 3, "end_line": 7})
        self.assertIn("(L3-L7)", md)

    def test_with_suggestion(self):
        md = pr.format_comment_fallback(comment(content="fix this", existing_code="x = 1", suggestion_code="x = 2"))
        self.assertIn("<details><summary>💡 Suggested Change</summary>", md)
        self.assertIn("**Before:**\n```\nx = 1\n```", md)
        self.assertIn("**After:**\n```\nx = 2\n```", md)
        self.assertIn("</details>", md)

    def test_with_reason(self):
        md = pr.format_comment_fallback(comment(), reason="out of diff")
        self.assertIn("⚠️ Could not be posted inline: out of diff", md)


class BadgeTest(unittest.TestCase):
    def test_both(self):
        self.assertEqual(pr.build_badge({"category": "Bug", "severity": "HIGH"}), "[Bug · HIGH]")

    def test_category_only(self):
        self.assertEqual(pr.build_badge({"category": "security"}), "[security]")

    def test_severity_only(self):
        self.assertEqual(pr.build_badge({"severity": "low"}), "[low]")

    def test_neither(self):
        self.assertEqual(pr.build_badge({}), "")
        self.assertEqual(pr.build_badge(None), "")

    def test_control_chars_stripped(self):
        self.assertEqual(pr.build_badge({"category": "a\nb"}), "[ab]")


class PolicyTest(unittest.TestCase):
    def test_no_routing_when_empty(self):
        self.assertEqual(pr.build_policy("", ""), pr.NO_ROUTING)

    def test_no_routing_when_unknown(self):
        self.assertEqual(pr.build_policy("bogus", "nope"), pr.NO_ROUTING)

    def test_severity_threshold(self):
        p = pr.build_policy("medium", "")
        self.assertTrue(p["route_by_severity"])
        self.assertEqual(p["severity_rank"], 2)

    def test_categories_set(self):
        p = pr.build_policy("", "Style, documentation ,bogus")
        self.assertTrue(p["route_by_category"])
        self.assertEqual(p["categories"], {"style", "documentation"})

    def test_route_severity_at_or_below(self):
        p = pr.build_policy("medium", "")
        self.assertTrue(pr.route_comment(comment(severity="low"), p)["routed"])
        self.assertTrue(pr.route_comment(comment(severity="medium"), p)["routed"])
        self.assertFalse(pr.route_comment(comment(severity="high"), p)["routed"])

    def test_route_unknown_severity_never_matches(self):
        p = pr.build_policy("critical", "")
        self.assertFalse(pr.route_comment(comment(severity="bogus"), p)["routed"])

    def test_route_category(self):
        p = pr.build_policy("", "style")
        self.assertTrue(pr.route_comment(comment(category="style"), p)["routed"])
        self.assertFalse(pr.route_comment(comment(category="bug"), p)["routed"])

    def test_route_reason_text(self):
        p = pr.build_policy("low", "")
        r = pr.route_comment(comment(severity="low", category="style"), p)
        self.assertIn("severity low", r["reason"])
        self.assertIn("category style", r["reason"])


class SortToSendTest(unittest.TestCase):
    def test_orders_by_path_then_line(self):
        items = [
            {"comment": {"path": "b.py", "start_line": 1, "end_line": 1}},
            {"comment": {"path": "a.py", "start_line": 5, "end_line": 5}},
            {"comment": {"path": "a.py", "start_line": 2, "end_line": 2}},
        ]
        out = pr.sort_to_send(items)
        self.assertEqual([i["comment"]["path"] for i in out], ["a.py", "a.py", "b.py"])
        self.assertEqual([out[0]["comment"]["start_line"], out[1]["comment"]["start_line"]], [2, 5])

    def test_stable_index_tiebreak(self):
        items = [
            {"comment": {"path": "a.py", "start_line": 1, "end_line": 1}, "tag": "first"},
            {"comment": {"path": "a.py", "start_line": 1, "end_line": 1}, "tag": "second"},
        ]
        out = pr.sort_to_send(items)
        self.assertEqual([i["tag"] for i in out], ["first", "second"])


class WarningsTest(unittest.TestCase):
    def test_empty(self):
        self.assertEqual(pr.format_warnings([]), "")

    def test_full_entry(self):
        body = pr.format_warnings([{"file": "a.py", "type": "skip", "message": "no go"}])
        self.assertIn("⚠️ **Warnings:**", body)
        self.assertIn("`a.py` (`skip`): no go", body)

    def test_partial_entry(self):
        self.assertIn("`a.py`: boom", pr.format_warnings([{"file": "a.py", "message": "boom"}]))

    def test_string_warning(self):
        self.assertIn("- plain string", pr.format_warnings(["plain string"]))


class SafeFenceTest(unittest.TestCase):
    def test_min_three_backticks(self):
        self.assertEqual(pr.safe_fence("no backticks"), "```")

    def test_grows_past_longest_run(self):
        self.assertEqual(pr.safe_fence("a ``` b"), "````")
        self.assertEqual(pr.safe_fence("a ```` b"), "`````")

    def test_fenced_block_ensures_trailing_newline(self):
        block = pr.fenced_block("x")
        self.assertTrue(block.startswith("```\n"))
        self.assertTrue(block.endswith("\n```"))
        self.assertIn("x\n", block)


# --------------------------------------------------------------------------- #
# publish() (Seam 1) — legacy-friendly (sticky_summary=False)
# --------------------------------------------------------------------------- #


class PublishTest(unittest.TestCase):
    def test_inline_success_and_summary(self):
        stats, rec = run_publish({"comments": [comment(start_line=5, end_line=10)]})
        self.assertEqual(stats["inline"], 1)
        self.assertEqual(stats["failed"], 0)
        self.assertEqual(len(rec.disc_calls), 1)
        # Two-phase summary: anchor (post_note) + finalize (update_note).
        self.assertEqual(rec.summary_call_count, 2)
        inline = rec.disc_calls[0]
        self.assertEqual(inline["position"]["new_path"], "main.py")
        self.assertEqual(inline["position"]["new_line"], 10)
        line_range = inline["position"]["line_range"]
        expected_path_sha1 = hashlib.sha1(b"main.py").hexdigest()
        self.assertEqual(line_range["start"]["line_code"], f"{expected_path_sha1}_0_5")
        self.assertEqual(line_range["start"]["new_line"], 5)
        self.assertEqual(line_range["end"]["line_code"], f"{expected_path_sha1}_0_10")
        self.assertEqual(line_range["end"]["new_line"], 10)

        self.assertIn("possible issue", inline["body"])
        self.assertIn("**1** issue(s)", rec.final_summary_body)
        self.assertIn("Successfully posted inline: 1 comment(s)", rec.final_summary_body)

    def test_fallback_when_diff_refs_none(self):
        stats, rec = run_publish({"comments": [comment()]}, diff_refs=None)
        self.assertEqual(stats["inline"], 0)
        self.assertEqual(stats["failed"], 1)
        # No discussions; summary still anchors first, then finalizes with the
        # failed-comment rendering.
        self.assertEqual(len(rec.disc_calls), 0)
        self.assertEqual(rec.summary_call_count, 2)
        self.assertIn("Could not be posted inline", rec.final_summary_body)

    def test_inline_error_falls_back(self):
        stats, rec = run_publish(
            {"comments": [comment()]},
            poster=Recorder(disc_outcomes=[
                {"success": False, "rate_limit_remaining": None, "is_rate_limit_exhausted": False,
                 "failed_reason": "boom", "http_status": 500, "error_body": "server error"}]),
        )
        self.assertEqual(stats["inline"], 0)
        self.assertEqual(stats["failed"], 1)
        self.assertEqual(len(rec.disc_calls), 1)
        self.assertEqual(rec.summary_call_count, 2)

    def test_no_comments_lgtm(self):
        stats, rec = run_publish({"message": "Looks good to me."})
        self.assertEqual(stats["inline"], 0)
        # LGTM path goes straight through upsert_summary (no anchor phase), so
        # the final body lands in post_note when no prior summary note exists.
        self.assertEqual(rec.final_summary_body,
                         "<!-- ocr-summary -->\n<!-- ocr-summary-run:42-7 -->\n"
                         "✅ **OpenCodeReview**: Looks good to me.")

    def test_warnings_in_summary(self):
        stats, rec = run_publish({"comments": [comment()], "warnings": [{"file": "a.py", "message": "skipped"}]})
        self.assertIn("1 warning(s)", rec.final_summary_body)
        self.assertIn("`a.py`: skipped", rec.final_summary_body)

    def test_success_pacing(self):
        stats, rec = run_publish(
            {"comments": [comment()]},
            poster=Recorder(disc_outcomes=[
                {"success": True, "rate_limit_remaining": 100, "is_rate_limit_exhausted": False}]),
        )
        # success_delay=2.0, remaining=100 > threshold=10 -> normal pace.
        self.assertEqual(rec.sleeps, [2.0])

    def test_proactive_throttling(self):
        stats, rec = run_publish(
            {"comments": [comment()]},
            poster=Recorder(disc_outcomes=[
                {"success": True, "rate_limit_remaining": 5, "is_rate_limit_exhausted": False}]),
        )
        self.assertEqual(rec.sleeps, [4.0])  # doubled

    def test_failure_pacing_rate_limit(self):
        stats, rec = run_publish(
            {"comments": [comment()]},
            poster=Recorder(disc_outcomes=[
                {"success": False, "rate_limit_remaining": None, "is_rate_limit_exhausted": True,
                 "failed_reason": "rl", "http_status": 429}]),
        )
        self.assertEqual(rec.sleeps, [2.0])

    def test_failure_pacing_non_rate_limit(self):
        stats, rec = run_publish(
            {"comments": [comment()]},
            poster=Recorder(disc_outcomes=[
                {"success": False, "rate_limit_remaining": None, "is_rate_limit_exhausted": False,
                 "failed_reason": "x", "http_status": 500}]),
        )
        self.assertEqual(rec.sleeps, [1.0])

    def test_proactive_throttling_disabled(self):
        config = dict(DEFAULT_CONFIG)
        config["rate_limit_threshold"] = 0
        stats, rec = run_publish(
            {"comments": [comment()]},
            config=config,
            poster=Recorder(disc_outcomes=[
                {"success": True, "rate_limit_remaining": 1, "is_rate_limit_exhausted": False}]),
        )
        self.assertEqual(rec.sleeps, [2.0])

    def test_multiple_comments_some_fail(self):
        result = {"comments": [
            comment(path="a.py", end_line=1),
            comment(path="b.py", end_line=2),
            comment(path="c.py", end_line=3),
        ]}
        stats, rec = run_publish(
            result,
            poster=Recorder(disc_outcomes=[
                {"success": True, "rate_limit_remaining": 100, "is_rate_limit_exhausted": False},
                {"success": False, "rate_limit_remaining": None, "is_rate_limit_exhausted": False,
                 "failed_reason": "x", "http_status": 500},
                {"success": True, "rate_limit_remaining": 100, "is_rate_limit_exhausted": False},
            ]),
        )
        self.assertEqual(stats["inline"], 2)
        self.assertEqual(stats["failed"], 1)
        self.assertEqual(len(rec.disc_calls), 3)
        self.assertEqual(len(rec.note_calls), 1)

    def test_comment_without_path_falls_back(self):
        stats, rec = run_publish({"comments": [comment(path="", start_line=0, end_line=0)]})
        self.assertEqual(stats["summary"], 1)
        self.assertEqual(stats["inline"], 0)

    def test_comment_without_end_line_falls_back(self):
        stats, rec = run_publish({"comments": [comment(start_line=0, end_line=0)]})
        self.assertEqual(stats["summary"], 1)
        self.assertEqual(stats["inline"], 0)

    def test_start_line_only_without_end_line_is_no_line(self):
        # A comment with start_line but a falsy end_line cannot be positioned
        # (end_line is the GitLab position's new_line), so it must land in
        # the summary (no_line), not be misclassified as a posting failure.
        stats, rec = run_publish({"comments": [comment(start_line=10, end_line=0)]})
        self.assertEqual(stats["summary"], 1)
        self.assertEqual(stats["inline"], 0)
        self.assertEqual(stats["failed"], 0)
        self.assertEqual(len(rec.disc_calls), 0)

    def test_route_policy_moves_to_summary(self):
        config = dict(DEFAULT_CONFIG)
        config["route_severity_below"] = "low"
        result = {"comments": [comment(severity="low")]}
        stats, rec = run_publish(result, config=config)
        self.assertEqual(stats["routed"], 1)
        self.assertEqual(stats["inline"], 0)
        self.assertEqual(len(rec.disc_calls), 0)
        self.assertIn("Routed to summary", rec.note_calls[0])


# --------------------------------------------------------------------------- #
# Sticky summary (Group B1)
# --------------------------------------------------------------------------- #


class StickySummaryTest(unittest.TestCase):
    def config(self):
        c = dict(DEFAULT_CONFIG)
        c["sticky_summary"] = True
        return c

    def test_cold_start_creates_anchor_then_updates(self):
        stats, rec = run_publish({"comments": [comment()]}, config=self.config())
        # list_notes once (anchor), post_note once (create anchor), update_note once (finalize).
        self.assertEqual(rec.list_notes_calls, 1)
        self.assertEqual(len(rec.note_calls), 1)  # pre-review anchor body
        self.assertEqual(len(rec.update_calls), 1)  # final body
        self.assertIn(pr.SUMMARY_MARKER, rec.note_calls[0])
        self.assertEqual(stats["inline"], 1)

    def test_existing_summary_is_updated_not_duplicated(self):
        existing = [{"id": 42, "body": "%s\nold" % pr.SUMMARY_MARKER, "web_url": "https://x/42"}]
        stats, rec = run_publish(
            {"comments": [comment()]},
            config=self.config(),
            poster=Recorder(notes=existing),
        )
        # Anchor finds existing -> no new note created; finalize updates id 42.
        self.assertEqual(len(rec.note_calls), 0)
        self.assertEqual(len(rec.update_calls), 1)
        self.assertEqual(rec.update_calls[0][0], 42)

    def test_upsert_update_failure_keeps_stale_url(self):
        existing = [{"id": 42, "body": "%s\nold" % pr.SUMMARY_MARKER, "web_url": "https://x/42"}]
        rec = Recorder(notes=existing, update_outcomes=[{"success": False, "url": None}])
        url = pr.upsert_summary(rec, "%s\nnew" % pr.SUMMARY_MARKER, sticky=True)
        # Update failed -> fall back to the existing note's URL (not None).
        self.assertEqual(url, "https://x/42")
        self.assertEqual(len(rec.update_calls), 1)

    def test_finalize_anchor_update_failure_falls_back_to_upsert(self):
        # Anchor created (id 1001) but the finalize PUT fails -> fall back to
        # upsert, which (sticky, no existing) posts a fresh note.
        rec = Recorder(update_outcomes=[{"success": False, "url": None}])
        url = pr.finalize_summary(rec, "%s\nfinal" % pr.SUMMARY_MARKER,
                                 sticky=True, anchor_id=1001)
        # update_note failed, then upsert posted a fresh note (id 1001).
        self.assertEqual(len(rec.update_calls), 1)
        self.assertEqual(len(rec.note_calls), 1)
        self.assertEqual(url, "https://gitlab.example/note/1001")

    def test_lgtm_sticky_upsert(self):
        existing = [{"id": 7, "body": "%s\nold" % pr.SUMMARY_MARKER, "web_url": "https://x/7"}]
        stats, rec = run_publish(
            {"message": "all good"}, config=self.config(), poster=Recorder(notes=existing))
        self.assertEqual(len(rec.update_calls), 1)
        self.assertEqual(rec.update_calls[0][0], 7)

    def test_read_failure_skips_summary(self):
        rec = Recorder(notes_read_failure=True)  # list_notes returns None
        cfg = self.config()
        stats = pr.publish({"comments": [comment()]}, DIFF_REFS, rec, cfg, sleep=NOOP_SLEEP)
        self.assertIsNone(stats["summary_url"])
        # Discussions still attempted.
        self.assertEqual(len(rec.disc_calls), 1)


class NonStickySummaryTest(unittest.TestCase):
    """Non-sticky mode now uses a per-run tag so the pre-review anchor and the
    finalize phase reuse the same note within a run (instead of always creating
    a fresh note), mirroring the GitHub Action's SUMMARY_TAG mechanism."""

    def test_cold_start_creates_anchor_then_updates_same_note(self):
        # Non-sticky: anchor creates note N, finalize updates N (not a new note).
        stats, rec = run_publish({"comments": [comment()]})  # DEFAULT_CONFIG is non-sticky
        self.assertEqual(rec.summary_call_count, 2)  # 1 post + 1 update
        self.assertEqual(len(rec.note_calls), 1)  # anchor (pre-review body)
        self.assertEqual(len(rec.update_calls), 1)  # finalize (final body)
        # The anchor body carries the per-run tag; the final body updates the
        # same note id the anchor created (Recorder's first post_note -> 1001).
        self.assertIn(pr.summary_tag_for("42-7"), rec.note_calls[0])
        self.assertEqual(rec.update_calls[0][0], 1001)

    def test_existing_same_run_note_is_reused_not_duplicated(self):
        # Simulate a prior call in THIS run that already created the anchor note
        # (carrying this run's tag). The anchor phase must find and reuse it.
        tag = pr.summary_tag_for("42-7")
        existing = [{"id": 55, "body": "%s\n%s\nold" % (pr.SUMMARY_MARKER, tag),
                     "web_url": "https://x/55"}]
        stats, rec = run_publish({"comments": [comment()]}, poster=Recorder(notes=existing))
        # Anchor found the existing note -> no new post; finalize updates id 55.
        self.assertEqual(len(rec.note_calls), 0)
        self.assertEqual(len(rec.update_calls), 1)
        self.assertEqual(rec.update_calls[0][0], 55)

    def test_existing_different_run_note_is_not_reused(self):
        # A note from a DIFFERENT run carries a different tag; non-sticky must
        # NOT reuse it (it should create a fresh note for this run).
        other_tag = pr.summary_tag_for("99-1")
        existing = [{"id": 55, "body": "%s\n%s\nold" % (pr.SUMMARY_MARKER, other_tag),
                     "web_url": "https://x/55"}]
        stats, rec = run_publish({"comments": [comment()]}, poster=Recorder(notes=existing))
        # No same-run note found -> anchor posts a new note; finalize updates it.
        self.assertEqual(len(rec.note_calls), 1)
        self.assertEqual(len(rec.update_calls), 1)


class SummaryTagPureTest(unittest.TestCase):
    def test_summary_tag_format(self):
        self.assertEqual(pr.summary_tag_for("42-7"), "<!-- ocr-summary-run:42-7 -->")

    def test_wrap_summary_body_embeds_both_markers(self):
        body = pr.wrap_summary_body("content here", "42-7")
        self.assertIn(pr.SUMMARY_MARKER, body)
        self.assertIn(pr.summary_tag_for("42-7"), body)
        self.assertIn("content here", body)

    def test_find_summary_note_by_tag_newest_first(self):
        tag = pr.summary_tag_for("1-1")
        old = {"id": 1, "body": "no marker"}
        stale = {"id": 2, "body": "%s\n%s\nstale" % (pr.SUMMARY_MARKER, tag)}
        fresh = {"id": 3, "body": "%s\n%s\nfresh" % (pr.SUMMARY_MARKER, tag)}
        # list_notes returns oldest-first; find_summary_note must return newest.
        self.assertEqual(pr.find_summary_note([old, stale, fresh], tag=tag)["id"], 3)

    def test_find_summary_note_marker_when_no_tag(self):
        # Sticky path: no tag -> match the cross-run SUMMARY_MARKER.
        n = {"id": 9, "body": "%s\nx" % pr.SUMMARY_MARKER}
        self.assertEqual(pr.find_summary_note([{"id": 1, "body": "x"}, n])["id"], 9)

    def test_find_summary_note_returns_none_when_no_match(self):
        self.assertIsNone(pr.find_summary_note([], tag="<!-- ocr-summary-run:1-1 -->"))
        self.assertIsNone(pr.find_summary_note([{"id": 1, "body": "nope"}]))


# --------------------------------------------------------------------------- #
# Incremental overlap (Group B2)
# --------------------------------------------------------------------------- #


class IncrementalPureTest(unittest.TestCase):
    def test_resolve_threshold_invalid(self):
        self.assertEqual(pr.resolve_threshold("x"), pr.DEFAULT_OVERLAP_THRESHOLD)
        self.assertEqual(pr.resolve_threshold(0), pr.DEFAULT_OVERLAP_THRESHOLD)
        self.assertEqual(pr.resolve_threshold(2), pr.DEFAULT_OVERLAP_THRESHOLD)
        self.assertEqual(pr.resolve_threshold(0.7), 0.7)

    def test_comment_span_single(self):
        s = pr.comment_span(comment(start_line=5, end_line=5))
        self.assertEqual(s, {"start": 5, "end": 5, "multiline": False})

    def test_comment_span_multi(self):
        s = pr.comment_span(comment(start_line=5, end_line=8))
        self.assertEqual(s, {"start": 5, "end": 8, "multiline": True})

    def test_same_comment_span_single(self):
        a = {"start": 5, "end": 5, "multiline": False}
        b = {"start": 5, "end": 5, "multiline": False}
        self.assertTrue(pr.same_comment_span(a, b, 0.6))
        c = {"start": 6, "end": 6, "multiline": False}
        self.assertFalse(pr.same_comment_span(a, c, 0.6))

    def test_single_vs_multi_never_match(self):
        single = {"start": 5, "end": 5, "multiline": False}
        multi = {"start": 5, "end": 5, "multiline": True}
        self.assertFalse(pr.same_comment_span(single, multi, 0.6))

    def test_multi_iou(self):
        # cur 5-10, other 8-13 -> overlap 3 (8,9,10), union 9 -> 0.33 -> below 0.6.
        cur = {"start": 5, "end": 10, "multiline": True}
        other = {"start": 8, "end": 13, "multiline": True}
        self.assertFalse(pr.same_comment_span(cur, other, 0.6))
        # cur 5-10, other 5-9 -> overlap 5, union 6 -> 0.83 -> above 0.6.
        other2 = {"start": 5, "end": 9, "multiline": True}
        self.assertTrue(pr.same_comment_span(cur, other2, 0.6))

    def test_position_span_single_line(self):
        self.assertEqual(pr.position_span({"new_line": 7}),
                         {"start": 7, "end": 7, "multiline": False})

    def test_position_span_line_range(self):
        pos = {"line_range": {"start": {"new_line": 3}, "end": {"new_line": 6}}}
        self.assertEqual(pr.position_span(pos), {"start": 3, "end": 6, "multiline": True})


class IncrementalPublishTest(unittest.TestCase):
    def config(self):
        c = dict(DEFAULT_CONFIG)
        c["incremental"] = True
        return c

    def test_overlapping_comment_skipped(self):
        body_with_marker = "<!-- ocr-1-1-aabb -->\nold finding"
        discussions = [{
            "notes": [{
                "body": body_with_marker,
                "position": {"new_path": "main.py", "new_line": 10},
            }],
        }]
        rec = Recorder(discussions=discussions)
        stats = pr.publish({"comments": [comment()]},  # also main.py:10
                           DIFF_REFS, rec, self.config(), sleep=NOOP_SLEEP)
        self.assertEqual(stats["skipped"], 1)
        self.assertEqual(stats["inline"], 0)
        self.assertEqual(len(rec.disc_calls), 0)

    def test_non_overlapping_comment_posted(self):
        body_with_marker = "<!-- ocr-1-1-aabb -->\nold finding"
        discussions = [{
            "notes": [{"body": body_with_marker,
                       "position": {"new_path": "main.py", "new_line": 99}}],
        }]
        rec = Recorder(discussions=discussions)
        stats = pr.publish({"comments": [comment()]},  # main.py:10, no overlap
                           DIFF_REFS, rec, self.config(), sleep=NOOP_SLEEP)
        self.assertEqual(stats["skipped"], 0)
        self.assertEqual(stats["inline"], 1)

    def test_non_bot_discussion_ignored(self):
        discussions = [{"notes": [{"body": "human comment", "position": {"new_path": "main.py", "new_line": 10}}]}]
        rec = Recorder(discussions=discussions)
        stats = pr.publish({"comments": [comment()]}, DIFF_REFS, rec, self.config(), sleep=NOOP_SLEEP)
        self.assertEqual(stats["skipped"], 0)  # not a bot discussion

    def test_read_failure_disables_filter(self):
        rec = Recorder(disc_read_failure=True)  # read failure
        stats = pr.publish({"comments": [comment()]}, DIFF_REFS, rec, self.config(), sleep=NOOP_SLEEP)
        self.assertEqual(stats["skipped"], 0)
        self.assertEqual(stats["inline"], 1)  # posted anyway


# --------------------------------------------------------------------------- #
# GitLabPoster transport (Seam 2)
# --------------------------------------------------------------------------- #


class GitLabPosterTest(unittest.TestCase):
    API_BASE = "https://gitlab.example/api/v4/projects/1/merge_requests/2"
    TOKEN = "test-token"
    AUTH_HEADER = "PRIVATE-TOKEN"

    def call(self, discussion, outcomes, config=None, sleep_obj=None):
        """Drive a poster method over a sequence of fake urlopen outcomes."""
        outcomes = list(outcomes)
        state = {"n": 0}

        def fake_urlopen(req, timeout=None):
            state["n"] += 1
            o = outcomes.pop(0)
            if isinstance(o, Exception):
                raise o
            if isinstance(o, FakeResponse):
                return o
            return FakeResponse(o)

        poster = pr.make_poster(self.API_BASE, self.TOKEN, self.AUTH_HEADER,
                                config or DEFAULT_CONFIG)
        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen), \
                mock.patch.object(pr, "_sleep", sleep_obj or (lambda _s: None)), \
                mock.patch.object(pr.random, "random", lambda: 0.5):
            try:
                if "position" in discussion:
                    result = poster.post_discussion(discussion)
                else:
                    result = poster.post_note(discussion["body"])
            except Exception as e:  # noqa: BLE001
                return state["n"], None, e
        return state["n"], result, None

    # ---- success path ----

    def test_post_note_success(self):
        n, result, raised = self.call({"body": "hello"}, [b'{"id": 1}'])
        self.assertIsNone(raised)
        self.assertTrue(result["success"])
        self.assertEqual(n, 1)

    def test_post_discussion_success(self):
        discussion = {"body": "inline", "position": {"new_path": "a.py", "new_line": 1}}
        n, result, raised = self.call(discussion, [b'{"id": 1}'])
        self.assertIsNone(raised)
        self.assertTrue(result["success"])
        self.assertEqual(n, 1)

    def test_rate_limit_remaining_parsed(self):
        n, result, raised = self.call(
            {"body": "hello"},
            [FakeResponse(b'{"id": 1}', headers={"RateLimit-Remaining": "42"})],
        )
        self.assertEqual(result["rate_limit_remaining"], 42)

    # ---- retry on rate-limit (429) ----

    def test_retry_429_then_success(self):
        n, result, raised = self.call({"body": "hello"}, [http_error(429, b"rate limited"), b'{"id": 1}'])
        self.assertTrue(result["success"])
        self.assertEqual(n, 2)

    def test_retry_403_rate_limit_keywords_then_success(self):
        n, result, raised = self.call({"body": "hello"}, [http_error(403, b"Rate limit exceeded"), b'{"id": 1}'])
        self.assertTrue(result["success"])
        self.assertEqual(n, 2)

    # ---- retry on transient (5xx, 408) ----

    def test_retry_500_then_success(self):
        n, result, raised = self.call({"body": "hello"}, [http_error(500, b"server error"), b'{"id": 1}'])
        self.assertTrue(result["success"])
        self.assertEqual(n, 2)

    def test_retry_408_then_success(self):
        n, result, raised = self.call({"body": "hello"}, [http_error(408, b"timeout"), b'{"id": 1}'])
        self.assertTrue(result["success"])
        self.assertEqual(n, 2)

    # ---- no retry on non-retryable errors ----

    def test_no_retry_404(self):
        n, result, raised = self.call({"body": "hello"}, [http_error(404, b"not found")])
        self.assertFalse(result["success"])
        self.assertFalse(result["is_rate_limit_exhausted"])
        self.assertEqual(n, 1)

    def test_no_retry_400(self):
        n, result, raised = self.call({"body": "hello"}, [http_error(400, b"bad request")])
        self.assertFalse(result["success"])
        self.assertEqual(n, 1)

    def test_no_retry_403_non_rate_limit(self):
        n, result, raised = self.call({"body": "hello"}, [http_error(403, b"forbidden")])
        self.assertFalse(result["success"])
        self.assertFalse(result["is_rate_limit_exhausted"])
        self.assertEqual(n, 1)

    # ---- is_rate_limit_exhausted classification ----

    def test_rate_limit_exhausted_on_429(self):
        outcomes = [http_error(429, b"rate limited")] * 4  # max_retries=3 -> 4 attempts
        n, result, raised = self.call({"body": "hello"}, outcomes)
        self.assertFalse(result["success"])
        self.assertTrue(result["is_rate_limit_exhausted"])
        self.assertEqual(n, 4)

    def test_rate_limit_not_exhausted_on_transient(self):
        outcomes = [http_error(500, b"server error")] * 4
        n, result, raised = self.call({"body": "hello"}, outcomes)
        self.assertFalse(result["success"])
        self.assertFalse(result["is_rate_limit_exhausted"])
        self.assertEqual(n, 4)

    # ---- Retry-After header ----

    def test_honors_retry_after_header(self):
        sleeps = []

        def fake_urlopen(req, timeout=None):
            if not hasattr(fake_urlopen, "_called"):
                fake_urlopen._called = True
                raise http_error(429, b"rate limited", headers={"Retry-After": "5"})
            return FakeResponse(b'{"id": 1}')

        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen), \
                mock.patch.object(pr, "_sleep", sleeps.append), \
                mock.patch.object(pr.random, "random", lambda: 0.5):
            poster = pr.make_poster(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG)
            result = poster.post_note("hello")
        self.assertTrue(result["success"])
        self.assertEqual(sleeps, [5.0])

    def test_retry_after_non_numeric_falls_back_to_exponential(self):
        sleeps = []

        def fake_urlopen(req, timeout=None):
            if not hasattr(fake_urlopen, "_called"):
                fake_urlopen._called = True
                raise http_error(429, b"rate limited", headers={"Retry-After": "soon"})
            return FakeResponse(b'{"id": 1}')

        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen), \
                mock.patch.object(pr, "_sleep", sleeps.append), \
                mock.patch.object(pr.random, "random", lambda: 0.5):
            poster = pr.make_poster(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG)
            result = poster.post_note("hello")
        self.assertTrue(result["success"])
        self.assertEqual(sleeps, [2.0])

    # ---- delay cap ----

    def test_delay_capped_at_max_retry_delay(self):
        sleeps = []
        config = dict(DEFAULT_CONFIG)
        config["max_retry_delay"] = 3.0

        def fake_urlopen(req, timeout=None):
            if not hasattr(fake_urlopen, "_called"):
                fake_urlopen._called = True
                raise http_error(429, b"rate limited", headers={"Retry-After": "999"})
            return FakeResponse(b'{"id": 1}')

        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen), \
                mock.patch.object(pr, "_sleep", sleeps.append), \
                mock.patch.object(pr.random, "random", lambda: 0.5):
            poster = pr.make_poster(self.API_BASE, self.TOKEN, self.AUTH_HEADER, config)
            result = poster.post_note("hello")
        self.assertTrue(result["success"])
        self.assertEqual(sleeps, [3.0])

    # ---- jitter ----

    def test_jitter_within_25_percent(self):
        """Jitter should produce delays within +-25% of the base delay."""
        sleeps = []

        def make_urlopen():
            state = {"n": 0}

            def fake_urlopen(req, timeout=None):
                state["n"] += 1
                if state["n"] == 1:
                    raise http_error(429, b"rate limited")
                return FakeResponse(b'{"id": 1}')
            return fake_urlopen

        with mock.patch.object(pr.random, "random", lambda: 0.0):  # min jitter (-25%)
            with mock.patch.object(pr.urllib.request, "urlopen", make_urlopen()), \
                    mock.patch.object(pr, "_sleep", sleeps.append):
                pr.make_poster(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG).post_note("hello")
        # delay = 2.0 * 2^0 * (0.75 + 0.0 * 0.5) = 2.0 * 0.75 = 1.5
        self.assertAlmostEqual(sleeps[0], 1.5, places=2)

        sleeps.clear()
        with mock.patch.object(pr.random, "random", lambda: 1.0):  # max jitter (+25%)
            with mock.patch.object(pr.urllib.request, "urlopen", make_urlopen()), \
                    mock.patch.object(pr, "_sleep", sleeps.append):
                pr.make_poster(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG).post_note("hello")
        # delay = 2.0 * 2^0 * (0.75 + 1.0 * 0.5) = 2.0 * 1.25 = 2.5
        self.assertAlmostEqual(sleeps[0], 2.5, places=2)

    # ---- auth header ----

    def test_auth_header_private_token(self):
        captured = {}

        def fake_urlopen(req, timeout=None):
            captured["headers"] = dict(req.header_items())
            return FakeResponse(b'{"id": 1}')

        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen):
            pr.make_poster(self.API_BASE, self.TOKEN, "PRIVATE-TOKEN", DEFAULT_CONFIG).post_note("hello")
        headers = {k.lower(): v for k, v in captured["headers"].items()}
        self.assertEqual(headers.get("private-token"), self.TOKEN)
        self.assertNotIn("job-token", headers)

    def test_auth_header_job_token(self):
        captured = {}

        def fake_urlopen(req, timeout=None):
            captured["headers"] = dict(req.header_items())
            return FakeResponse(b'{"id": 1}')

        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen):
            pr.make_poster(self.API_BASE, self.TOKEN, "JOB-TOKEN", DEFAULT_CONFIG).post_note("hello")
        headers = {k.lower(): v for k, v in captured["headers"].items()}
        self.assertEqual(headers.get("job-token"), self.TOKEN)

    # ---- URLError (network-level errors) ----

    def test_retry_urlerror_then_success(self):
        conn_err = urllib.error.URLError(ConnectionRefusedError("refused"))
        n, result, raised = self.call({"body": "hello"}, [conn_err, b'{"id": 1}'])
        self.assertTrue(result["success"])
        self.assertEqual(n, 2)

    def test_urlerror_exhausts_retries(self):
        conn_err = urllib.error.URLError(ConnectionRefusedError("refused"))
        n, result, raised = self.call({"body": "hello"}, [conn_err] * 4)
        self.assertFalse(result["success"])
        self.assertFalse(result["is_rate_limit_exhausted"])
        self.assertEqual(n, 4)

    def test_urlerror_timeout_not_retried(self):
        timeout_err = urllib.error.URLError(socket.timeout("timed out"))
        n, result, raised = self.call({"body": "hello"}, [timeout_err, b'{"id": 1}'])
        self.assertFalse(result["success"])
        self.assertEqual(n, 1)

    def test_non_utf8_error_body_does_not_crash(self):
        n, result, raised = self.call({"body": "hello"}, [http_error(404, b"\xff\xfe<html>")])
        self.assertFalse(result["success"])
        self.assertIsNone(raised)


# --------------------------------------------------------------------------- #
# Read-API pacing (mirrors the GitHub Action's readWithPacing)
# --------------------------------------------------------------------------- #


class ReadPacingTest(unittest.TestCase):
    API_BASE = "https://gitlab.example/api/v4/projects/1/merge_requests/2"
    TOKEN = "test-token"
    AUTH_HEADER = "PRIVATE-TOKEN"

    def _poster(self, config_overrides=None):
        cfg = dict(DEFAULT_CONFIG)
        cfg.update(config_overrides or {})
        return pr.make_poster(self.API_BASE, self.TOKEN, self.AUTH_HEADER, cfg)

    def test_paces_with_read_success_delay_after_successful_read(self):
        sleeps = []
        poster = self._poster({"read_success_delay": 0.4,
                               "read_low_remaining_spacing": 9.0,
                               "rate_limit_threshold": 3})
        resp = FakeResponse(b'[]', headers={})  # no RateLimit-Remaining header
        with mock.patch.object(pr.urllib.request, "urlopen",
                               lambda req, timeout=None: resp), \
                mock.patch.object(pr, "_sleep", sleeps.append):
            out = poster.list_notes()
        self.assertEqual(out, [])  # empty list, not None
        # The read succeeded with no quota info -> normal read_success_delay.
        self.assertEqual(sleeps, [0.4])

    def test_paces_with_long_spacing_when_quota_low(self):
        sleeps = []
        poster = self._poster({"read_success_delay": 0.4,
                               "read_low_remaining_spacing": 9.0,
                               "rate_limit_threshold": 10})
        # remaining=2 <= threshold 10 -> long spacing.
        resp = FakeResponse(b'[]', headers={"RateLimit-Remaining": "2"})
        with mock.patch.object(pr.urllib.request, "urlopen",
                               lambda req, timeout=None: resp), \
                mock.patch.object(pr, "_sleep", sleeps.append):
            poster.list_notes()
        self.assertEqual(sleeps, [9.0])

    def test_no_pacing_sleep_on_failed_read(self):
        sleeps = []
        poster = self._poster({"read_success_delay": 0.4,
                               "rate_limit_threshold": 3})
        with mock.patch.object(pr.urllib.request, "urlopen",
                               lambda req, timeout=None:
                                   (_ for _ in ()).throw(http_error(500, b"boom"))), \
                mock.patch.object(pr, "_sleep", sleeps.append), \
                mock.patch.object(pr.random, "random", lambda: 0.5):
            out = poster.list_notes()
        # Read failed after retries -> None; only retry backoff sleeps, no
        # read_success_delay (the pacing sleep only runs on success).
        self.assertIsNone(out)
        self.assertTrue(all(s != 0.4 for s in sleeps),
                        "read_success_delay must not fire on a failed read; got %r" % sleeps)


# --------------------------------------------------------------------------- #
# Idempotent post_discussion (Group B3)
# --------------------------------------------------------------------------- #


class IdempotencyTest(unittest.TestCase):
    API_BASE = "https://gitlab.example/api/v4/projects/1/merge_requests/2"
    CID = "ocr-42-7-deadbeefdeadbeef"

    def run_with_urlopen(self, fake_urlopen, config=None, discussion=None):
        poster = pr.make_poster(self.API_BASE, "tok", "PRIVATE-TOKEN", config or DEFAULT_CONFIG)
        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen), \
                mock.patch.object(pr, "_sleep", lambda _s: None):
            return poster.post_discussion(
                discussion or {"body": "<!-- %s -->\nhi" % self.CID,
                               "position": {"new_path": "a.py", "new_line": 1}},
                comment_id=self.CID)

    def test_reconciles_when_already_posted(self):
        calls = {"n": 0}

        def fake_urlopen(req, timeout=None):
            calls["n"] += 1
            if req.method == "POST":
                raise http_error(500, b"server error")
            # GET /discussions
            return FakeResponse(json.dumps([{
                "notes": [{"body": "<!-- %s -->\nlanded" % self.CID}]
            }]).encode())

        result = self.run_with_urlopen(fake_urlopen)
        self.assertTrue(result["success"])
        self.assertTrue(result["reconciled"])
        self.assertEqual(calls["n"], 2)  # POST then GET

    def test_read_unavailable_returns_failed_no_duplicate(self):
        def fake_urlopen(req, timeout=None):
            if req.method == "POST":
                raise http_error(500, b"server error")
            raise http_error(500, b"read broken")  # list_discussions fails

        result = self.run_with_urlopen(fake_urlopen)
        self.assertFalse(result["success"])
        self.assertIn("idempotency check unavailable", result["failed_reason"])

    def test_not_posted_then_retry_succeeds(self):
        seq = {"i": 0}

        def fake_urlopen(req, timeout=None):
            seq["i"] += 1
            if req.method == "POST":
                if seq["i"] == 1:
                    raise http_error(500, b"server error")
                return FakeResponse(b'{"id": 1}')
            return FakeResponse(b"[]")  # GET /discussions: not posted

        result = self.run_with_urlopen(fake_urlopen)
        self.assertTrue(result["success"])

    def test_400_does_not_reconcile(self):
        calls = {"n": 0}

        def fake_urlopen(req, timeout=None):
            calls["n"] += 1
            return FakeResponse(b'{"id":1}') if req.method == "GET" else \
                (_ for _ in ()).throw(http_error(400, b'{"message":"new_line is invalid"}'))

        result = self.run_with_urlopen(fake_urlopen)
        self.assertFalse(result["success"])
        self.assertEqual(result["http_status"], 400)
        self.assertEqual(calls["n"], 1)  # no read attempted


# --------------------------------------------------------------------------- #
# 400 line-resolution fallback (Group C1)
# --------------------------------------------------------------------------- #


class LineResolutionTest(unittest.TestCase):
    def test_is_line_resolution_failure_positive(self):
        self.assertTrue(pr.is_line_resolution_failure('{"message":"new_line is invalid"}'))
        self.assertTrue(pr.is_line_resolution_failure("Position out of diff"))
        self.assertTrue(pr.is_line_resolution_failure("the new_line is out of the diff"))

    def test_is_line_resolution_failure_line_code_pattern(self):
        # GitLab's Note-model form of the same line-resolution failure: the
        # position's new_line cannot be resolved, so line_code is blank.
        body = ('{"message":"400 Bad request - Note '
                '{:line_code=>["can\'t be blank", "must be a valid line code"]}"}')
        self.assertTrue(pr.is_line_resolution_failure(body))

    def test_is_line_resolution_failure_negative(self):
        self.assertFalse(pr.is_line_resolution_failure(""))
        self.assertFalse(pr.is_line_resolution_failure("something unrelated"))

    def test_parse_hunks_basic(self):
        patch = "@@ -1,3 +5,3 @@\n ctx\n-old\n+new\n ctx2\n"
        ranges, complete = pr.parse_diff_hunk_inventory(patch)
        self.assertTrue(complete)
        self.assertEqual(ranges, [{"start": 5, "end": 7}])

    def test_parse_hunks_truncated_is_incomplete(self):
        patch = "@@ -1,3 +5,5 @@\n ctx\n"  # declares 5 lines, shows 1
        ranges, complete = pr.parse_diff_hunk_inventory(patch)
        self.assertFalse(complete)

    def test_classify_unknown_when_no_inventory(self):
        self.assertEqual(pr.classify_comment_against_diff(comment(), None), "unknown")
        self.assertEqual(pr.classify_comment_against_diff(
            comment(), {"known": set(), "files": {}, "complete": False}), "unknown")

    def test_classify_invalid_path(self):
        diff = {"known": {"other.py"}, "files": {}, "complete": True}
        self.assertEqual(pr.classify_comment_against_diff(comment(path="main.py"), diff), "invalid")

    def test_classify_invalid_line_outside_hunk(self):
        diff = {"known": {"main.py"}, "files": {"main.py": [{"start": 5, "end": 7}]}, "complete": True}
        self.assertEqual(pr.classify_comment_against_diff(comment(start_line=10, end_line=10), diff), "invalid")

    def test_classify_valid(self):
        diff = {"known": {"main.py"}, "files": {"main.py": [{"start": 5, "end": 15}]}, "complete": True}
        self.assertEqual(pr.classify_comment_against_diff(comment(start_line=10, end_line=10), diff), "valid")


class FallbackPublishTest(unittest.TestCase):
    def test_400_line_failure_classifies_invalid(self):
        config = dict(DEFAULT_CONFIG)
        diff = {"known": set(), "files": {}, "complete": True}  # main.py not in diff -> invalid
        rec = Recorder(
            disc_outcomes=[{"success": False, "failed_reason": "new_line invalid",
                            "http_status": 400, "error_body": '{"message":"new_line is invalid"}',
                            "is_rate_limit_exhausted": False, "rate_limit_remaining": None}],
            diffs=diff,
        )
        stats = pr.publish({"comments": [comment()]}, DIFF_REFS, rec, config, sleep=NOOP_SLEEP)
        self.assertEqual(rec.diff_calls, 1)
        self.assertEqual(stats["failed"], 1)
        self.assertIn("out of diff", rec.final_summary_body)


# --------------------------------------------------------------------------- #
# wait-until-reset (Group C2)
# --------------------------------------------------------------------------- #


class WaitUntilResetTest(unittest.TestCase):
    def config(self):
        return DEFAULT_CONFIG

    def test_seconds_until_reset(self):
        resp = {"is_rate_limit_exhausted": True, "is_transient": False,
                "headers": {"RateLimit-Remaining": "0", "RateLimit-Reset": "5"}}
        with mock.patch.object(pr.random, "random", lambda: 0.5):
            self.assertEqual(pr._compute_retry_delay(resp, 0, self.config()), 5.0)

    def test_epoch_reset(self):
        now = 1700000000  # realistic Unix epoch (> 1e9)
        reset_epoch = now + 10
        resp = {"is_rate_limit_exhausted": True, "is_transient": False,
                "headers": {"RateLimit-Remaining": "0", "RateLimit-Reset": str(reset_epoch)}}
        with mock.patch.object(pr.time, "time", lambda: now), \
                mock.patch.object(pr.random, "random", lambda: 0.5):
            self.assertAlmostEqual(pr._compute_retry_delay(resp, 0, self.config()), 10.0, places=1)

    def test_remaining_nonzero_skips_wait_until_reset(self):
        resp = {"is_rate_limit_exhausted": True, "is_transient": False,
                "headers": {"RateLimit-Remaining": "5", "RateLimit-Reset": "5"}}
        with mock.patch.object(pr.random, "random", lambda: 0.5):
            # Falls through to exponential: retry_base_delay(2.0)*2^0 = 2.0
            self.assertEqual(pr._compute_retry_delay(resp, 0, self.config()), 2.0)


# --------------------------------------------------------------------------- #
# fetch_diff_refs() with mocked urlopen
# --------------------------------------------------------------------------- #


class FetchDiffRefsTest(unittest.TestCase):
    API_BASE = "https://gitlab.example/api/v4/projects/1/merge_requests/2"
    TOKEN = "test-token"
    AUTH_HEADER = "PRIVATE-TOKEN"

    def test_success(self):
        body = json.dumps([{
            "base_commit_sha": "aaa", "start_commit_sha": "bbb", "head_commit_sha": "ccc",
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
            "base_commit_sha": "aaa", "start_commit_sha": "bbb", "head_commit_sha": "ccc",
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

    def _versions(self, *triples):
        # triples: (base, start, head, [created_at])
        return [{
            "base_commit_sha": t[0], "start_commit_sha": t[1],
            "head_commit_sha": t[2],
            **({"created_at": t[3]} if len(t) > 3 else {}),
        } for t in triples]

    def test_picks_version_matching_expected_head(self):
        versions = self._versions(
            ("aaa", "bbb", "old-head", "2024-01-01T00:00:00Z"),
            ("ccc", "ddd", "ocr-head", "2024-01-02T00:00:00Z"),
        )
        body = json.dumps(versions).encode()
        with mock.patch.object(pr.urllib.request, "urlopen",
                               lambda req, timeout=None: FakeResponse(body)), \
                mock.patch.object(pr, "_sleep", lambda _s: None):
            refs = pr.fetch_diff_refs(self.API_BASE, self.TOKEN, self.AUTH_HEADER,
                                      DEFAULT_CONFIG, {"head": "ocr-head"})
        self.assertEqual(refs, {"base_sha": "ccc", "start_sha": "ddd", "head_sha": "ocr-head"})

    def test_picks_version_matching_head_and_base(self):
        # Head matches on both versions but base differs: must pick the one
        # whose base also matches, not just the first head match.
        versions = self._versions(
            ("wrong-base", "bbb", "ocr-head", "2024-01-02T00:00:00Z"),
            ("ocr-base", "ddd", "ocr-head", "2024-01-01T00:00:00Z"),
        )
        body = json.dumps(versions).encode()
        with mock.patch.object(pr.urllib.request, "urlopen",
                               lambda req, timeout=None: FakeResponse(body)), \
                mock.patch.object(pr, "_sleep", lambda _s: None):
            refs = pr.fetch_diff_refs(self.API_BASE, self.TOKEN, self.AUTH_HEADER,
                                      DEFAULT_CONFIG, {"head": "ocr-head", "base": "ocr-base"})
        self.assertEqual(refs["base_sha"], "ocr-base")

    def test_falls_back_to_newest_when_no_sha_match(self):
        versions = self._versions(
            ("aaa", "bbb", "v1-head", "2024-01-01T00:00:00Z"),
            ("ccc", "ddd", "v2-head", "2024-01-02T00:00:00Z"),
        )
        body = json.dumps(versions).encode()
        with mock.patch.object(pr.urllib.request, "urlopen",
                               lambda req, timeout=None: FakeResponse(body)), \
                mock.patch.object(pr, "_sleep", lambda _s: None), \
                mock.patch.object(pr, "log", lambda m: None):
            refs = pr.fetch_diff_refs(self.API_BASE, self.TOKEN, self.AUTH_HEADER,
                                      DEFAULT_CONFIG, {"head": "stale-head"})
        # No version matches "stale-head"; falls back to newest by created_at.
        self.assertEqual(refs, {"base_sha": "ccc", "start_sha": "ddd", "head_sha": "v2-head"})

    def test_without_expected_shas_picks_newest_by_created_at(self):
        # API returns oldest-first here; the sort must surface the newest.
        versions = self._versions(
            ("aaa", "bbb", "old-head", "2024-01-01T00:00:00Z"),
            ("ccc", "ddd", "new-head", "2024-01-02T00:00:00Z"),
        )
        body = json.dumps(versions).encode()
        with mock.patch.object(pr.urllib.request, "urlopen",
                               lambda req, timeout=None: FakeResponse(body)), \
                mock.patch.object(pr, "_sleep", lambda _s: None):
            refs = pr.fetch_diff_refs(self.API_BASE, self.TOKEN, self.AUTH_HEADER, DEFAULT_CONFIG)
        self.assertEqual(refs["head_sha"], "new-head")

    def test_extract_expected_shas_from_manifest(self):
        result = {"manifest": {"input": {
            "resolved_base": "  base123  ", "resolved_head": "head456"}}}
        self.assertEqual(pr._extract_expected_shas(result),
                         {"head": "head456", "base": "base123"})

    def test_extract_expected_shas_empty_when_no_manifest(self):
        self.assertEqual(pr._extract_expected_shas({}), {})
        self.assertEqual(pr._extract_expected_shas({"manifest": {}}), {})


# --------------------------------------------------------------------------- #
# Config / stats / gating (Group D)
# --------------------------------------------------------------------------- #


class BuildConfigTest(unittest.TestCase):
    def test_defaults(self):
        config = pr.build_config({})
        self.assertEqual(config["success_delay"], 2.0)
        self.assertEqual(config["failure_delay"], 1.0)
        self.assertEqual(config["rate_limit_threshold"], 10)
        self.assertEqual(config["transient_base_delay"], 2)
        self.assertTrue(config["sticky_summary"])  # default ON
        self.assertFalse(config["incremental"])
        self.assertEqual(config["incremental_overlap_threshold"], 0.6)
        self.assertEqual(config["route_severity_below"], "")
        self.assertEqual(config["fail_on_severity"], "")

    def test_env_overrides(self):
        env = {
            "OCR_SUCCESS_DELAY": "5000", "OCR_FAILURE_DELAY": "2500",
            "OCR_RATE_LIMIT_THRESHOLD": "5", "OCR_RETRY_BASE_DELAY": "3000",
            "OCR_MAX_RETRIES": "5", "OCR_MAX_RETRY_DELAY": "120000",
            "OCR_STICKY_SUMMARY": "false", "OCR_INCREMENTAL": "true",
            "OCR_INCREMENTAL_OVERLAP_THRESHOLD": "0.8",
            "OCR_ROUTE_SEVERITY_BELOW": "low", "OCR_ROUTE_CATEGORIES": "style,doc",
            "OCR_FAIL_ON_SEVERITY": "critical", "CI_PIPELINE_ID": "9", "CI_JOB_ID": "3",
        }
        config = pr.build_config(env)
        self.assertEqual(config["success_delay"], 5.0)
        self.assertEqual(config["max_retries"], 5)
        self.assertFalse(config["sticky_summary"])
        self.assertTrue(config["incremental"])
        self.assertEqual(config["incremental_overlap_threshold"], 0.8)
        self.assertEqual(config["route_severity_below"], "low")
        self.assertEqual(config["fail_on_severity"], "critical")
        self.assertEqual(config["run_tag"], "9-3")

    def test_read_pacing_defaults_and_overrides(self):
        # Defaults mirror the GitHub Action's readWithPacing knobs.
        config = pr.build_config({})
        self.assertEqual(config["read_success_delay"], 0.5)
        self.assertEqual(config["read_low_remaining_spacing"], 5.0)
        # Overridable via env.
        config = pr.build_config({"OCR_READ_SUCCESS_DELAY": "250",
                                  "OCR_READ_LOW_REMAINING_SPACING": "8000"})
        self.assertEqual(config["read_success_delay"], 0.25)
        self.assertEqual(config["read_low_remaining_spacing"], 8.0)

    def test_incremental_overlap_threshold_non_numeric_falls_back(self):
        # A malformed value must not crash build_config; it falls back to the
        # default via resolve_threshold.
        config = pr.build_config({"OCR_INCREMENTAL_OVERLAP_THRESHOLD": "bogus"})
        self.assertEqual(config["incremental_overlap_threshold"], pr.DEFAULT_OVERLAP_THRESHOLD)

    def test_incremental_overlap_threshold_out_of_range_falls_back(self):
        config = pr.build_config({"OCR_INCREMENTAL_OVERLAP_THRESHOLD": "2.5"})
        self.assertEqual(config["incremental_overlap_threshold"], pr.DEFAULT_OVERLAP_THRESHOLD)


class StatsGatingTest(unittest.TestCase):
    def test_write_stats_file(self):
        stats = {"total": 5, "inline": 3, "summary": 1, "routed": 0, "skipped": 1,
                 "failed": 0, "summary_url": "https://x/1"}
        with tempfile.NamedTemporaryFile("w", suffix=".env", delete=False) as f:
            path = f.name
        self.addCleanup(os.unlink, path)
        pr.write_stats_file(path, stats)
        with open(path) as f:
            content = f.read()
        self.assertIn("OCR_COMMENTS_TOTAL=5", content)
        self.assertIn("OCR_COMMENTS_INLINE=3", content)
        self.assertIn("OCR_COMMENTS_SKIPPED=1", content)
        self.assertIn("OCR_SUMMARY_URL=https://x/1", content)

    def test_check_fail_on_severity(self):
        comments = [{"severity": "high"}, {"severity": "low"}]
        self.assertTrue(pr.check_fail_on_severity(comments, "high"))
        self.assertTrue(pr.check_fail_on_severity(comments, "medium"))  # high,low >= medium
        self.assertFalse(pr.check_fail_on_severity(comments, "critical"))
        self.assertFalse(pr.check_fail_on_severity(comments, ""))

    def test_check_fail_unknown_threshold_disabled(self):
        self.assertFalse(pr.check_fail_on_severity([{"severity": "critical"}], "bogus"))


# --------------------------------------------------------------------------- #
# Dry-run poster
# --------------------------------------------------------------------------- #


class DryRunPosterTest(unittest.TestCase):
    def test_prints_note(self):
        poster = pr.make_dry_run_poster()
        stdout = io.StringIO()
        import contextlib
        with contextlib.redirect_stdout(stdout):
            result = poster.post_note("hello world")
        self.assertTrue(result["success"])
        self.assertIn("hello world", stdout.getvalue())
        self.assertIn("dry-run", stdout.getvalue())

    def test_prints_inline_discussion(self):
        poster = pr.make_dry_run_poster()
        stdout = io.StringIO()
        import contextlib
        with contextlib.redirect_stdout(stdout):
            result = poster.post_discussion({"body": "inline finding",
                                             "position": {"new_path": "a.py", "new_line": 5}})
        self.assertTrue(result["success"])
        output = stdout.getvalue()
        self.assertIn("a.py:5", output)
        self.assertIn("inline finding", output)

    def test_no_http_calls(self):
        poster = pr.make_dry_run_poster()
        with mock.patch.object(pr.urllib.request, "urlopen",
                               side_effect=AssertionError("urlopen should not be called")):
            poster.post_note("test")
            poster.post_discussion({"body": "test2", "position": {"new_path": "x.py", "new_line": 1}})


# --------------------------------------------------------------------------- #
# main() auth-header resolution + gating (end-to-end)
# --------------------------------------------------------------------------- #


class MainAuthHeaderTest(unittest.TestCase):
    BASE_ENV = {
        "CI_SERVER_URL": "https://gitlab.example",
        "CI_PROJECT_ID": "1",
        "CI_MERGE_REQUEST_IID": "2",
    }

    def run_main_with_captured_poster(self, env_overrides, result=None):
        env = dict(self.BASE_ENV)
        env.update(env_overrides)
        captured = {}

        class CapturingPoster(pr.GitLabPoster):
            def __init__(_self, api_base, token, auth_header, config):
                super().__init__(api_base, token, auth_header, config)
                captured["auth_header"] = auth_header
                captured["token"] = token

        import tempfile as tf
        f = tf.NamedTemporaryFile("w", suffix=".json", delete=False)
        json.dump(result or {"comments": [comment()]}, f)
        f.close()
        self.addCleanup(os.unlink, f.name)

        stats_path = tf.NamedTemporaryFile("w", suffix=".env", delete=False)
        stats_path.close()
        self.addCleanup(os.unlink, stats_path.name)

        # Canned urlopen so the real GitLabPoster never hits the network (the
        # host "gitlab.example" has no DNS and would stall on every read/write).
        # GETs return an empty list (no notes/discussions/diffs); writes return
        # a minimal note object. The auth-header selection under test happens
        # before any of these bodies matter.
        def fake_urlopen(req, timeout=None):
            if req.method == "GET":
                return FakeResponse(b"[]")
            return FakeResponse(b'{"id": 1, "web_url": "https://gitlab.example/note/1"}')

        with mock.patch.dict(os.environ, env, clear=True), \
                mock.patch.object(pr, "make_poster", CapturingPoster), \
                mock.patch.object(pr, "fetch_diff_refs", lambda *a, **kw: DIFF_REFS), \
                mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen), \
                mock.patch.object(pr, "_sleep", lambda _s: None):
            rc = pr.main([f.name, "--stats-file", stats_path.name])
        return rc, captured

    def test_private_token_when_gitlab_api_token_set(self):
        rc, captured = self.run_main_with_captured_poster({"GITLAB_API_TOKEN": "glpat-xxxx"})
        self.assertEqual(rc, 0)
        self.assertEqual(captured["auth_header"], "PRIVATE-TOKEN")
        self.assertEqual(captured["token"], "glpat-xxxx")

    def test_job_token_when_only_ci_job_token_set(self):
        rc, captured = self.run_main_with_captured_poster({"CI_JOB_TOKEN": "ci-job-xxxx"})
        self.assertEqual(rc, 0)
        self.assertEqual(captured["auth_header"], "JOB-TOKEN")

    def test_private_token_wins_when_both_set(self):
        rc, captured = self.run_main_with_captured_poster({
            "GITLAB_API_TOKEN": "glpat-xxxx", "CI_JOB_TOKEN": "ci-job-xxxx"})
        self.assertEqual(captured["auth_header"], "PRIVATE-TOKEN")

    def test_missing_ci_vars_fails_fast(self):
        env = {"GITLAB_API_TOKEN": "glpat-xxxx"}
        stats_path = tempfile.NamedTemporaryFile("w", suffix=".env", delete=False)
        stats_path.close()
        self.addCleanup(os.unlink, stats_path.name)
        with mock.patch.dict(os.environ, env, clear=True):
            rc = pr.main(["/tmp/nonexistent.json", "--stats-file", stats_path.name])
        self.assertEqual(rc, 1)
        # Early exit must still write a zero-filled stats file so the dotenv
        # artifact is present for downstream jobs.
        with open(stats_path.name) as f:
            content = f.read()
        self.assertIn("OCR_COMMENTS_TOTAL=0", content)
        self.assertIn("OCR_COMMENTS_INLINE=0", content)

    def test_missing_token_fails_fast(self):
        env = dict(self.BASE_ENV)
        stats_path = tempfile.NamedTemporaryFile("w", suffix=".env", delete=False)
        stats_path.close()
        self.addCleanup(os.unlink, stats_path.name)
        with mock.patch.dict(os.environ, env, clear=True):
            rc = pr.main(["/tmp/nonexistent.json", "--stats-file", stats_path.name])
        self.assertEqual(rc, 1)
        with open(stats_path.name) as f:
            content = f.read()
        self.assertIn("OCR_COMMENTS_TOTAL=0", content)

    def test_fail_on_severity_returns_nonzero(self):
        rc, captured = self.run_main_with_captured_poster(
            {"GITLAB_API_TOKEN": "glpat-xxxx", "OCR_FAIL_ON_SEVERITY": "critical"},
            result={"comments": [comment(severity="critical")]},
        )
        self.assertEqual(rc, 1)


if __name__ == "__main__":
    unittest.main()

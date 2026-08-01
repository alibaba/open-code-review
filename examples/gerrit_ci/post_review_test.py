#!/usr/bin/env python3
"""Tests for post_review.py.

Standard-library unittest only (no pytest, no network, no live Gerrit): run with

    python3 post_review_test.py                                     # from examples/gerrit_ci/
    python3 -m unittest examples/gerrit_ci/post_review_test.py -v   # from the repo root
    python3 -m unittest discover -s examples/gerrit_ci -p '*_test.py'

The transport tests drive main() through a Recorder fake poster, so the whole
flag/env/error-handling flow is exercised without any HTTP.
"""

import base64
import contextlib
import io
import json
import os
import socket
import sys
import tempfile
import unittest
import urllib.error
from unittest import mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import post_review as pr  # noqa: E402


def comment(**overrides):
    """One OCR comment as `ocr review --format json` emits it."""
    c = {
        "path": "main.go",
        "content": "possible nil dereference",
        "start_line": 6,
        "end_line": 6,
    }
    c.update(overrides)
    return c


def build(comments, **extra):
    result = dict(extra)
    result["comments"] = comments
    return pr.build_review_input(result)


def entry_of(ri, path="main.go"):
    return ri["comments"][path][0]


class BuildReviewInputTest(unittest.TestCase):
    # ---- line mapping (never a "range" key in v1) ----

    def assert_line(self, c, want_line):
        entry = entry_of(build([c]))
        self.assertNotIn("range", entry)
        if want_line is None:
            self.assertNotIn("line", entry)
        else:
            self.assertEqual(entry["line"], want_line)

    def test_single_line_comment(self):
        self.assert_line(comment(start_line=6, end_line=6), 6)

    def test_multi_line_range(self):
        self.assert_line(comment(start_line=3, end_line=6), 6)

    def test_file_level_comment(self):
        self.assert_line(comment(start_line=0, end_line=0), None)

    def test_missing_line_keys(self):
        self.assert_line({"path": "main.go", "content": "no lines"}, None)

    def test_start_gt_end_degenerate(self):
        self.assert_line(comment(start_line=8, end_line=3), 3)

    def test_negative_start_line(self):
        self.assert_line(comment(start_line=-1, end_line=6), 6)

    def test_missing_path(self):
        ri = build([comment(path="", content="orphan finding")])
        self.assertNotIn("comments", ri)
        self.assertIn("orphan finding", ri["message"])

    # ---- severity -> unresolved ----

    def assert_unresolved(self, severity, want):
        c = comment()
        if severity is not None:
            c["severity"] = severity
        self.assertIs(entry_of(build([c]))["unresolved"], want)

    def test_severity_critical_unresolved(self):
        self.assert_unresolved("critical", True)

    def test_severity_high_unresolved(self):
        self.assert_unresolved("high", True)

    def test_severity_medium_resolved(self):
        self.assert_unresolved("medium", False)

    def test_severity_low_resolved(self):
        self.assert_unresolved("low", False)

    def test_severity_missing_resolved(self):
        self.assert_unresolved(None, False)

    # ---- message formatting ----

    def test_severity_category_line(self):
        msg = entry_of(build([comment(severity="high", category="security")]))["message"]
        lines = msg.split("\n")
        self.assertEqual(lines[0], "**Severity:** high · **Category:** security")
        self.assertEqual(lines[1], "")
        self.assertIn("possible nil dereference", msg)

    def test_severity_line_omitted_when_unset(self):
        msg = entry_of(build([comment()]))["message"]
        self.assertTrue(msg.startswith("possible nil dereference"))
        self.assertNotIn("**Severity:**", msg)
        self.assertNotIn("**Category:**", msg)

    def test_suggestion_block(self):
        msg = entry_of(build([comment(
            existing_code="x := y.Field",
            suggestion_code="if y != nil { x = y.Field }",
        )]))["message"]
        self.assertIn("**Suggestion:**\n```\nif y != nil { x = y.Field }\n```", msg)

    def test_suggestion_without_existing(self):
        msg = entry_of(build([comment(suggestion_code="if y != nil {}")]))["message"]
        self.assertNotIn("**Suggestion:**", msg)

    def test_unicode_comment_roundtrip(self):
        content = "空指针解引用：y 可能为 nil"
        ri = build([comment(path="pkg/服务.go", content=content)])
        self.assertIn("pkg/服务.go", ri["comments"])
        self.assertIn(content, entry_of(ri, "pkg/服务.go")["message"])
        self.assertEqual(json.loads(json.dumps(ri, ensure_ascii=False)), ri)

    def test_path_with_spaces(self):
        ri = build([comment(path="docs/my file.go")])
        self.assertIn("docs/my file.go", ri["comments"])
        self.assertNotIn("docs/my%20file.go", ri["comments"])

    def test_very_long_message_truncated(self):
        msg = entry_of(build([comment(content="x" * 20000)]))["message"]
        self.assertLessEqual(len(msg), 16000)
        self.assertIn("[truncated]", msg)

    # ---- batching & summary ----

    def test_multiple_files_grouped(self):
        ri = build([
            comment(path="a.go", end_line=1),
            comment(path="a.go", end_line=2),
            comment(path="b.go", end_line=3),
        ])
        self.assertEqual(len(ri["comments"]["a.go"]), 2)
        self.assertEqual(len(ri["comments"]["b.go"]), 1)

    def test_batch_metadata(self):
        ri = build([comment(path="a.go"), comment(path="b.go"), comment(path="c.go")])
        self.assertEqual(ri["tag"], "autogenerated:opencodereview")
        self.assertEqual(ri["notify"], "OWNER")
        self.assertIs(ri["omit_duplicate_comments"], True)
        self.assertEqual(ri["drafts"], "KEEP")
        self.assertIn("3", ri["message"])

    def test_empty_comments_summary_only(self):
        ri = build([])
        self.assertNotIn("comments", ri)
        self.assertIn("Looks good", ri["message"])

    def test_result_message_only(self):
        ri = pr.build_review_input({"message": "No comments generated. Looks good to me."})
        self.assertNotIn("comments", ri)
        self.assertIn("Looks good to me", ri["message"])

    def test_warnings_in_summary(self):
        ri = pr.build_review_input({
            "comments": [comment()],
            "warnings": [{"file": "a.go", "message": "skipped", "type": "subtask_error"}],
        })
        self.assertIn("1 warning(s)", ri["message"])
        self.assertIn("comments", ri)

    def test_warnings_only_no_comments(self):
        ri = pr.build_review_input({
            "comments": [],
            "warnings": [{"file": "a.go", "message": "skipped", "type": "subtask_error"}],
        })
        self.assertNotIn("comments", ri)
        self.assertIn("1 warning(s)", ri["message"])


class StripXssiTest(unittest.TestCase):
    def test_strip_xssi(self):
        cases = [
            ("prefix_with_newline", ")]}'\n{\"a\": 1}"),
            ("bare_prefix", ")]}'{\"a\": 1}"),
            ("no_prefix", "{\"a\": 1}"),
            ("stacked_prefix", ")]}'\n)]}'\n{\"a\": 1}"),
        ]
        for name, raw in cases:
            with self.subTest(name):
                self.assertEqual(json.loads(pr.strip_xssi(raw)), {"a": 1})


class BuildEndpointTest(unittest.TestCase):
    def test_trailing_slash_base(self):
        self.assertEqual(
            pr.build_endpoint("https://g.example/", 42, "abc"),
            "https://g.example/a/changes/42/revisions/abc/review",
        )

    def test_context_path_base(self):
        self.assertEqual(
            pr.build_endpoint("https://host/gerrit", "42", "abc"),
            "https://host/gerrit/a/changes/42/revisions/abc/review",
        )

    def test_revision_default_current(self):
        for revision in ("", None):
            with self.subTest(revision=revision):
                self.assertEqual(
                    pr.build_endpoint("https://g.example", "42", revision),
                    "https://g.example/a/changes/42/revisions/current/review",
                )

    def test_url_from_change_url_modern(self):
        self.assertEqual(pr.derive_base_url("https://host/c/proj/+/42"), "https://host")

    def test_url_from_change_url_path_prefix(self):
        self.assertEqual(
            pr.derive_base_url("https://host/gerrit/c/proj/+/42"), "https://host/gerrit"
        )

    def test_url_from_change_url_legacy(self):
        self.assertEqual(pr.derive_base_url("https://g.example/42"), "https://g.example")


# --------------------------------------------------------------------------- #
# main() driven through a Recorder fake poster
# --------------------------------------------------------------------------- #

PASSWORD = "s3cret-pass"

BASE_ENV = {
    "GERRIT_URL": "https://gerrit.example",
    "GERRIT_CHANGE_NUMBER": "42",
    "GERRIT_PATCHSET_REVISION": "abc",
    "GERRIT_HTTP_USER": "review-bot",
    "GERRIT_HTTP_PASSWORD": PASSWORD,
}

SAMPLE_RESULT = {
    "comments": [
        comment(path="a.go", content="nil deref", start_line=3, end_line=6,
                severity="high", category="bug"),
        comment(path="a.go", content="shadowed err", start_line=10, end_line=10,
                severity="low"),
        comment(path="b.go", content="missing test", start_line=1, end_line=2,
                category="test"),
    ],
}


def http_error(code, body=b"", reason="error"):
    return urllib.error.HTTPError(
        "https://gerrit.example", code, reason, {}, io.BytesIO(body)
    )


class Recorder:
    """Stands in for make_poster(); replays canned outcomes per call."""

    def __init__(self, outcomes=None):
        self.calls = []
        self.outcomes = list(outcomes or [])
        self.url = None
        self.user = None
        self.password = None

    def factory(self, url, user, password, timeout):
        self.url = url
        self.user = user
        self.password = password
        return self

    def __call__(self, review_input):
        self.calls.append(review_input)
        if self.outcomes:
            outcome = self.outcomes.pop(0)
            if isinstance(outcome, Exception):
                raise outcome
            return outcome
        return {}


class PostTest(unittest.TestCase):
    def run_main(self, argv=(), env=BASE_ENV, result=SAMPLE_RESULT, outcomes=None,
                 input_args=None):
        rec = Recorder(outcomes)
        f = tempfile.NamedTemporaryFile("w", suffix=".json", delete=False)
        json.dump(result, f)
        f.close()
        self.addCleanup(os.unlink, f.name)
        self.input_path = f.name
        if input_args is None:
            input_args = ["--input", f.name]
        else:  # "{path}" in input_args stands for the temp result file
            input_args = [a.format(path=f.name) for a in input_args]
        stdout, stderr = io.StringIO(), io.StringIO()
        with mock.patch.dict(os.environ, env, clear=True), \
                mock.patch.object(pr, "make_poster", rec.factory), \
                contextlib.redirect_stdout(stdout), \
                contextlib.redirect_stderr(stderr):
            rc = pr.main(list(argv) + input_args)
        return rc, rec, stdout.getvalue(), stderr.getvalue()

    def test_single_post_batching(self):
        rc, rec, _out, _err = self.run_main()
        self.assertEqual(rc, 0)
        self.assertEqual(len(rec.calls), 1)
        ri = rec.calls[0]
        self.assertEqual(sorted(ri["comments"]), ["a.go", "b.go"])
        self.assertEqual(len(ri["comments"]["a.go"]), 2)
        dumped = json.dumps(ri, ensure_ascii=False)
        for text in ("nil deref", "shadowed err", "missing test"):
            self.assertIn(text, dumped)

    def test_auth_failure_401(self):
        rc, _rec, out, err = self.run_main(outcomes=[http_error(401, b"Unauthorized")])
        self.assertEqual(rc, 2)
        self.assertIn("HTTP password", err)
        self.assertNotIn(PASSWORD, out)
        self.assertNotIn(PASSWORD, err)

    def test_password_never_in_error_body(self):
        body = ("bad credentials: " + PASSWORD).encode("utf-8")
        rc, _rec, out, err = self.run_main(outcomes=[http_error(401, body)])
        self.assertEqual(rc, 2)
        self.assertIn("***", err)
        self.assertNotIn(PASSWORD, out)
        self.assertNotIn(PASSWORD, err)

    def test_b64_credentials_never_in_error_body(self):
        # Echoed Authorization header: base64(user:password) decodes trivially.
        creds = base64.b64encode(
            ("review-bot:" + PASSWORD).encode("utf-8")
        ).decode("ascii")
        body = ("echoed header: Basic " + creds).encode("utf-8")
        rc, _rec, out, err = self.run_main(outcomes=[http_error(401, body)])
        self.assertEqual(rc, 2)
        self.assertIn("***", err)
        self.assertNotIn(creds, out)
        self.assertNotIn(creds, err)

    def test_not_found_404(self):
        rc, _rec, _out, err = self.run_main(outcomes=[http_error(404, b"Not found")])
        self.assertEqual(rc, 2)
        self.assertIn("42", err)
        self.assertIn("/a/", err)

    def test_change_closed_409(self):
        rc, _rec, _out, err = self.run_main(outcomes=[http_error(409, b"change is closed")])
        self.assertEqual(rc, 0)
        self.assertIn("warning", err.lower())

    def test_bad_request_400_falls_back(self):
        rc, rec, _out, _err = self.run_main(
            outcomes=[http_error(400, b"invalid line"), {}]
        )
        self.assertEqual(rc, 0)
        self.assertEqual(len(rec.calls), 2)
        fallback = rec.calls[1]
        self.assertNotIn("comments", fallback)
        self.assertIn("nil deref", fallback["message"])
        # The folded message must not claim comments were posted inline while
        # explaining they couldn't be placed (see fold_comments).
        self.assertNotIn("posted as inline comment(s)", fallback["message"])

    def test_non_gerrit_response_body(self):
        rc, _rec, _out, err = self.run_main(
            outcomes=[ValueError("body was '<html>Sign in</html>'")]
        )
        self.assertEqual(rc, 2)
        self.assertIn("GERRIT_URL", err)
        self.assertIn("scheme", err)

    def test_timeout_urlerror(self):
        rc, _rec, _out, err = self.run_main(outcomes=[urllib.error.URLError("timed out")])
        self.assertEqual(rc, 2)
        self.assertIn("GERRIT_URL", err)
        self.assertNotIn("Traceback", err)

    def test_dry_run_no_credentials(self):
        rc, rec, out, _err = self.run_main(argv=["--dry-run"], env={})
        self.assertEqual(rc, 0)
        self.assertEqual(rec.calls, [])
        self.assertIsNone(rec.url)
        ri = json.loads(out)
        self.assertEqual(ri["tag"], "autogenerated:opencodereview")

    def test_env_resolution(self):
        rc, rec, _out, _err = self.run_main()
        self.assertEqual(rc, 0)
        self.assertEqual(rec.url, "https://gerrit.example/a/changes/42/revisions/abc/review")

    def test_missing_required_env(self):
        env = dict(BASE_ENV)
        del env["GERRIT_CHANGE_NUMBER"]
        rc, rec, _out, err = self.run_main(env=env)
        self.assertEqual(rc, 2)
        self.assertIn("GERRIT_CHANGE_NUMBER", err)
        self.assertEqual(rec.calls, [])

    def test_stdin_input(self):
        rec = Recorder()
        stdout, stderr = io.StringIO(), io.StringIO()
        with mock.patch.dict(os.environ, BASE_ENV, clear=True), \
                mock.patch.object(pr, "make_poster", rec.factory), \
                mock.patch.object(sys, "stdin", io.StringIO(json.dumps(SAMPLE_RESULT))), \
                contextlib.redirect_stdout(stdout), \
                contextlib.redirect_stderr(stderr):
            rc = pr.main(["--input", "-"])
        self.assertEqual(rc, 0)
        self.assertEqual(len(rec.calls), 1)
        self.assertIn("Posted review", stdout.getvalue())

    def test_flags_override_env(self):
        rc, rec, _out, _err = self.run_main(argv=[
            "--gerrit-url", "https://flag.example",
            "--change", "7",
            "--revision", "def",
            "--user", "flag-user",
            "--password", "flag-pass",
        ])
        self.assertEqual(rc, 0)
        self.assertEqual(rec.url, "https://flag.example/a/changes/7/revisions/def/review")
        self.assertEqual(rec.user, "flag-user")
        self.assertEqual(rec.password, "flag-pass")

    def test_fold_retry_also_fails(self):
        rc, rec, _out, err = self.run_main(
            outcomes=[http_error(400, b"invalid line"), http_error(500, b"boom")]
        )
        self.assertEqual(rc, 2)
        self.assertEqual(len(rec.calls), 2)
        self.assertIn("fallback post failed", err)

    def test_fold_truncation_and_message(self):
        result = {"comments": [comment(path="a.go", content="x" * 20000)]}
        rc, rec, out, err = self.run_main(
            result=result, outcomes=[http_error(400, b"invalid line"), {}]
        )
        self.assertEqual(rc, 0)
        self.assertEqual(len(rec.calls), 2)
        self.assertLessEqual(len(rec.calls[1]["message"]), pr.MAX_MESSAGE_LEN)
        self.assertIn("folded summary truncated", err)
        self.assertIn("folded into summary", out)
        self.assertNotIn("Posted review with", out)

    def test_change_url_derivation(self):
        env = {k: v for k, v in BASE_ENV.items() if k != "GERRIT_URL"}
        env["GERRIT_CHANGE_URL"] = "https://host/gerrit/c/proj/+/42"
        rc, rec, _out, _err = self.run_main(env=env)
        self.assertEqual(rc, 0)
        self.assertEqual(rec.url, "https://host/gerrit/a/changes/42/revisions/abc/review")

    def test_non_dict_json_input(self):
        rc, rec, _out, err = self.run_main(result=[])
        self.assertEqual(rc, 1)
        self.assertEqual(rec.calls, [])
        self.assertIn("cannot read review result", err)
        self.assertIn("expected a JSON object", err)
        self.assertNotIn("Traceback", err)

    def test_positional_input(self):
        rc, rec, _out, _err = self.run_main(input_args=["{path}"])
        self.assertEqual(rc, 0)
        self.assertEqual(len(rec.calls), 1)

    def test_positional_input_wins_over_flag(self):
        rc, rec, _out, _err = self.run_main(
            input_args=["{path}", "--input", "/nonexistent.json"]
        )
        self.assertEqual(rc, 0)
        self.assertEqual(len(rec.calls), 1)


class ParseArgsTest(unittest.TestCase):
    def test_timeout_must_be_positive(self):
        for value in ("0", "-5"):
            with self.subTest(value=value):
                stderr = io.StringIO()
                with contextlib.redirect_stderr(stderr), \
                        self.assertRaises(SystemExit) as cm:
                    pr.parse_args(["--timeout", value])
                self.assertEqual(cm.exception.code, 2)
                self.assertIn("timeout must be > 0", stderr.getvalue())

    def test_timeout_float_still_parses(self):
        self.assertEqual(pr.parse_args(["--timeout", "2.5"]).timeout, 2.5)


class FakeResponse:
    def __init__(self, body):
        self._body = body

    def read(self):
        return self._body

    def __enter__(self):
        return self

    def __exit__(self, *args):
        return False


class MakePosterTest(unittest.TestCase):
    ENDPOINT = "https://gerrit.example/a/changes/42/revisions/current/review"

    def post(self, review_input, body=b")]}'\n{}"):
        captured = {}

        def fake_urlopen(req, timeout=None):
            captured["req"] = req
            return FakeResponse(body)

        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen):
            post = pr.make_poster(self.ENDPOINT, "review-bot", "s3cret-pass", 30)
            parsed = post(review_input)
        return captured["req"], parsed

    def test_preemptive_basic_auth_and_utf8_body(self):
        import base64

        req, _parsed = self.post({"message": "空指针解引用：y 可能为 nil"})
        auth = req.get_header("Authorization")
        self.assertIsNotNone(auth, "Authorization header must be set preemptively")
        self.assertTrue(auth.startswith("Basic "))
        self.assertEqual(
            base64.b64decode(auth[len("Basic "):]).decode("utf-8"),
            "review-bot:s3cret-pass",
        )
        self.assertIn("空指针解引用".encode("utf-8"), req.data)
        self.assertIn("application/json", req.get_header("Content-type"))

    def test_xssi_response_parses(self):
        _req, parsed = self.post(
            {"message": "hi"}, body=b")]}'\n{\"labels\": {\"Code-Review\": 0}}"
        )
        self.assertEqual(parsed, {"labels": {"Code-Review": 0}})

    # ---- bounded retry on transient failures ----

    def post_seq(self, outcomes):
        """Drive post() over a sequence of per-call fake_urlopen outcomes.

        Each outcome is either the raw response body (bytes, returned) or an
        Exception (raised). Sleeps are stubbed to a no-op so retries are fast.
        Returns (calls, result_or_None, raised_or_None).
        """
        calls = {"n": 0}
        outcomes = list(outcomes)

        def fake_urlopen(req, timeout=None):
            calls["n"] += 1
            outcome = outcomes.pop(0)
            if isinstance(outcome, Exception):
                raise outcome
            return FakeResponse(outcome)

        with mock.patch.object(pr.urllib.request, "urlopen", fake_urlopen), \
                mock.patch.object(pr, "_sleep", lambda _s: None):
            post = pr.make_poster(self.ENDPOINT, "review-bot", "s3cret-pass", 30)
            try:
                result = post({"message": "hi"})
            except Exception as e:  # noqa: BLE001 - counted and asserted below
                return calls["n"], None, e
            return calls["n"], result, None

    def test_retry_503_then_200(self):
        n, result, raised = self.post_seq([http_error(503, b"boom"), b")]}'\n{}"])
        self.assertIsNone(raised)
        self.assertEqual(result, {})
        self.assertEqual(n, 2)

    def test_retry_connection_urlerror_then_200(self):
        conn_err = urllib.error.URLError(ConnectionRefusedError("refused"))
        n, result, raised = self.post_seq([conn_err, b")]}'\n{}"])
        self.assertIsNone(raised)
        self.assertEqual(result, {})
        self.assertEqual(n, 2)

    def test_retry_503_exhausts_and_propagates(self):
        n, _result, raised = self.post_seq([http_error(503, b"boom")] * pr.MAX_ATTEMPTS)
        self.assertIsInstance(raised, urllib.error.HTTPError)
        self.assertEqual(raised.code, 503)
        self.assertEqual(n, pr.MAX_ATTEMPTS)

    def test_read_timeout_not_retried(self):
        # A urlopen read-timeout raises a bare socket.timeout/TimeoutError,
        # which is ambiguous and must NOT be retried.
        n, _result, raised = self.post_seq([socket.timeout("timed out"), b")]}'\n{}"])
        self.assertIsInstance(raised, socket.timeout)
        self.assertEqual(n, 1)

    def test_4xx_not_retried(self):
        for code in (400, 409):
            with self.subTest(code=code):
                n, _result, raised = self.post_seq([http_error(code, b"nope")])
                self.assertIsInstance(raised, urllib.error.HTTPError)
                self.assertEqual(raised.code, code)
                self.assertEqual(n, 1)


if __name__ == "__main__":
    unittest.main()

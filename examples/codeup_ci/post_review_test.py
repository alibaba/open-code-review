#!/usr/bin/env python3
"""Unit tests for post_review.py.

Run with:
    python3 -m unittest post_review_test.py -v

Everything is mocked (subprocess, urllib) so these tests need neither a
real `ocr` install nor network access / a real CodeUp instance.
"""
from __future__ import annotations

import json
import subprocess
import unittest
import urllib.error
from unittest import mock

import post_review as pr


class RunOcrReviewTests(unittest.TestCase):
    def test_success_parses_json(self):
        fake_proc = mock.Mock(returncode=0, stdout='{"comments": []}', stderr="")
        with mock.patch("subprocess.run", return_value=fake_proc) as run_mock:
            result = pr.run_ocr_review()
        self.assertEqual(result, {"comments": []})
        run_mock.assert_called_once()
        cmd = run_mock.call_args[0][0]
        self.assertEqual(cmd, ["ocr", "review", "--audience", "agent", "--format", "json"])

    def test_from_to_ref_are_passed_through(self):
        fake_proc = mock.Mock(returncode=0, stdout='{"comments": []}', stderr="")
        with mock.patch("subprocess.run", return_value=fake_proc) as run_mock:
            pr.run_ocr_review(from_ref="main", to_ref="feature-x")
        cmd = run_mock.call_args[0][0]
        self.assertIn("--from", cmd)
        self.assertIn("main", cmd)
        self.assertIn("--to", cmd)
        self.assertIn("feature-x", cmd)

    def test_missing_cli_raises(self):
        with mock.patch("subprocess.run", side_effect=FileNotFoundError()):
            with self.assertRaises(pr.OcrReviewError):
                pr.run_ocr_review()

    def test_timeout_raises(self):
        with mock.patch(
            "subprocess.run",
            side_effect=subprocess.TimeoutExpired(cmd="ocr", timeout=1800),
        ):
            with self.assertRaises(pr.OcrReviewError):
                pr.run_ocr_review()

    def test_nonzero_exit_raises(self):
        fake_proc = mock.Mock(returncode=2, stdout="", stderr="boom")
        with mock.patch("subprocess.run", return_value=fake_proc):
            with self.assertRaises(pr.OcrReviewError) as ctx:
                pr.run_ocr_review()
        self.assertIn("boom", str(ctx.exception))

    def test_empty_stdout_raises(self):
        fake_proc = mock.Mock(returncode=0, stdout="   ", stderr="")
        with mock.patch("subprocess.run", return_value=fake_proc):
            with self.assertRaises(pr.OcrReviewError):
                pr.run_ocr_review()

    def test_invalid_json_raises(self):
        fake_proc = mock.Mock(returncode=0, stdout="not json", stderr="")
        with mock.patch("subprocess.run", return_value=fake_proc):
            with self.assertRaises(pr.OcrReviewError):
                pr.run_ocr_review()


class FormatSummaryCommentTests(unittest.TestCase):
    def test_no_issues(self):
        comment = pr.format_summary_comment({"comments": []})
        self.assertIn("No issues found", comment)

    def test_accepts_bare_list_shape(self):
        comment = pr.format_summary_comment(
            [{"path": "a.py", "start_line": 1, "severity": "low", "content": "nit"}]
        )
        self.assertIn("a.py:1", comment)

    def test_sorts_by_severity_high_first(self):
        review = {
            "comments": [
                {"path": "a.py", "start_line": 1, "severity": "low", "content": "minor"},
                {"path": "b.py", "start_line": 2, "severity": "critical", "content": "danger"},
                {"path": "c.py", "start_line": 3, "severity": "medium", "content": "meh"},
            ]
        }
        comment = pr.format_summary_comment(review)
        self.assertLess(comment.index("b.py"), comment.index("c.py"))
        self.assertLess(comment.index("c.py"), comment.index("a.py"))

    def test_handles_alternate_field_names(self):
        review = {"comments": [{"file": "x.py", "line": 10, "body": "uses body/line/file"}]}
        comment = pr.format_summary_comment(review)
        self.assertIn("x.py:10", comment)
        self.assertIn("uses body/line/file", comment)

    def test_missing_line_omits_colon(self):
        review = {"comments": [{"path": "y.py", "content": "no line number"}]}
        comment = pr.format_summary_comment(review)
        self.assertIn("`y.py`", comment)
        self.assertNotIn("y.py:", comment)

    def test_category_included_in_badge(self):
        review = {
            "comments": [
                {"path": "z.py", "start_line": 1, "severity": "high",
                 "category": "security", "content": "sql injection risk"}
            ]
        }
        comment = pr.format_summary_comment(review)
        self.assertIn("[security", comment)
        self.assertIn("high]", comment)

    def test_max_items_truncates_and_notes_remainder(self):
        review = {
            "comments": [
                {"path": f"f{i}.py", "start_line": i, "severity": "low", "content": "x"}
                for i in range(5)
            ]
        }
        comment = pr.format_summary_comment(review, max_items=2)
        self.assertIn("and 3 more issue(s)", comment)
        self.assertIn("f0.py", comment)
        self.assertNotIn("f4.py", comment)

    def test_unknown_top_level_shape_treated_as_no_comments(self):
        comment = pr.format_summary_comment({"unexpected": "shape"})
        self.assertIn("No issues found", comment)


class PostGlobalCommentTests(unittest.TestCase):
    def _mock_response(self, status=200, body=b'{"comment_biz_id": "abc"}'):
        cm = mock.MagicMock()
        cm.__enter__.return_value.status = status
        cm.__enter__.return_value.read.return_value = body
        return cm

    def test_builds_url_without_organization_id(self):
        with mock.patch("urllib.request.urlopen", return_value=self._mock_response()) as urlopen_mock:
            pr.post_global_comment(
                domain="codeup.aliyun.com",
                repo_id="123",
                local_id="7",
                token="pt-xxx",
                content="hello",
            )
        request = urlopen_mock.call_args[0][0]
        self.assertEqual(
            request.full_url,
            "https://codeup.aliyun.com/oapi/v1/codeup/repositories/123/changeRequests/7/comments",
        )
        self.assertEqual(request.get_header("X-yunxiao-token"), "pt-xxx")
        sent_body = json.loads(request.data.decode("utf-8"))
        self.assertEqual(sent_body["comment_type"], "GLOBAL_COMMENT")
        self.assertEqual(sent_body["content"], "hello")

    def test_builds_url_with_organization_id(self):
        with mock.patch("urllib.request.urlopen", return_value=self._mock_response()) as urlopen_mock:
            pr.post_global_comment(
                domain="codeup.aliyun.com",
                repo_id="123",
                local_id="7",
                token="pt-xxx",
                content="hello",
                organization_id="org-1",
            )
        request = urlopen_mock.call_args[0][0]
        self.assertIn("/organizations/org-1/", request.full_url)

    def test_returns_status_and_body(self):
        with mock.patch(
            "urllib.request.urlopen",
            return_value=self._mock_response(status=201, body=b'{"ok": true}'),
        ):
            status, body = pr.post_global_comment(
                domain="d", repo_id="1", local_id="2", token="t", content="c"
            )
        self.assertEqual(status, 201)
        self.assertEqual(body, '{"ok": true}')

    def test_http_error_raises_codeup_api_error(self):
        http_error = urllib.error.HTTPError(
            url="https://d/x", code=403, msg="Forbidden",
            hdrs=None, fp=mock.MagicMock(read=lambda: b"denied"),
        )
        with mock.patch("urllib.request.urlopen", side_effect=http_error):
            with self.assertRaises(pr.CodeupApiError) as ctx:
                pr.post_global_comment(
                    domain="d", repo_id="1", local_id="2", token="t", content="c"
                )
        self.assertIn("403", str(ctx.exception))

    def test_url_error_raises_codeup_api_error(self):
        with mock.patch(
            "urllib.request.urlopen",
            side_effect=urllib.error.URLError("connection refused"),
        ):
            with self.assertRaises(pr.CodeupApiError):
                pr.post_global_comment(
                    domain="d", repo_id="1", local_id="2", token="t", content="c"
                )


class MainEndToEndTests(unittest.TestCase):
    REQUIRED_ENV = {
        "CODEUP_DOMAIN": "codeup.aliyun.com",
        "CODEUP_ORG_ID": "60d54f3daccf2bbd6659f3ad",
        "CODEUP_REPO_ID": "123",
        "CODEUP_MR_LOCAL_ID": "7",
        "CODEUP_TOKEN": "pt-xxx",
        "CODEUP_TARGET_BRANCH": "master",
        "CODEUP_SOURCE_BRANCH": "feature-x",
    }

    def test_happy_path_posts_comment_and_returns_zero(self):
        with mock.patch.dict("os.environ", self.REQUIRED_ENV, clear=True), \
             mock.patch("post_review.run_ocr_review", return_value={"comments": []}) as run_mock, \
             mock.patch("post_review.post_global_comment", return_value=(200, "{}")) as post_mock:
            rc = pr.main([])
        self.assertEqual(rc, 0)
        post_mock.assert_called_once()
        # Branches should be turned into origin/<branch> refs automatically.
        self.assertEqual(run_mock.call_args.kwargs["from_ref"], "origin/master")
        self.assertEqual(run_mock.call_args.kwargs["to_ref"], "origin/feature-x")
        # organization_id is now always required/passed (no "center edition" opt-out).
        self.assertEqual(post_mock.call_args.kwargs["organization_id"], self.REQUIRED_ENV["CODEUP_ORG_ID"])

    def test_extra_args_are_split_and_forwarded(self):
        env = dict(self.REQUIRED_ENV)
        env["OCR_REVIEW_EXTRA_ARGS"] = "--concurrency 4"
        with mock.patch.dict("os.environ", env, clear=True), \
             mock.patch("post_review.run_ocr_review", return_value={"comments": []}) as run_mock, \
             mock.patch("post_review.post_global_comment", return_value=(200, "{}")):
            rc = pr.main([])
        self.assertEqual(rc, 0)
        self.assertEqual(run_mock.call_args.kwargs["extra_args"], ["--concurrency", "4"])

    def test_ocr_failure_returns_nonzero_and_skips_post(self):
        with mock.patch.dict("os.environ", self.REQUIRED_ENV, clear=True), \
             mock.patch("post_review.run_ocr_review", side_effect=pr.OcrReviewError("boom")), \
             mock.patch("post_review.post_global_comment") as post_mock:
            rc = pr.main([])
        self.assertEqual(rc, 1)
        post_mock.assert_not_called()

    def test_missing_env_var_returns_nonzero(self):
        env = dict(self.REQUIRED_ENV)
        del env["CODEUP_TOKEN"]
        with mock.patch.dict("os.environ", env, clear=True), \
             mock.patch("post_review.run_ocr_review", return_value={"comments": []}):
            rc = pr.main([])
        self.assertEqual(rc, 1)

    def test_missing_org_id_returns_nonzero(self):
        env = dict(self.REQUIRED_ENV)
        del env["CODEUP_ORG_ID"]
        with mock.patch.dict("os.environ", env, clear=True), \
             mock.patch("post_review.run_ocr_review", return_value={"comments": []}):
            rc = pr.main([])
        self.assertEqual(rc, 1)

    def test_post_failure_defaults_to_non_blocking(self):
        with mock.patch.dict("os.environ", self.REQUIRED_ENV, clear=True), \
             mock.patch("post_review.run_ocr_review", return_value={"comments": []}), \
             mock.patch(
                 "post_review.post_global_comment",
                 side_effect=pr.CodeupApiError("network blip"),
             ):
            rc = pr.main([])
        self.assertEqual(rc, 0)

    def test_post_failure_can_be_made_blocking(self):
        with mock.patch.dict("os.environ", self.REQUIRED_ENV, clear=True), \
             mock.patch("post_review.run_ocr_review", return_value={"comments": []}), \
             mock.patch(
                 "post_review.post_global_comment",
                 side_effect=pr.CodeupApiError("network blip"),
             ):
            rc = pr.main(["--fail-on-post-error"])
        self.assertEqual(rc, 1)


if __name__ == "__main__":
    unittest.main()

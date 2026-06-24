#!/usr/bin/env python3
"""Tests for post_review.py.

Standard-library unittest only (no pytest, no network, no git): run with

    python3 post_review_test.py            # from examples/gitflic_ci/
    python3 -m unittest discover examples/gitflic_ci

The line-mapping cases are ported 1:1 from the upstream Go test
internal/publish/gitflic/linemap_test.go and publisher_test.go, so the script
keeps proven parity with the binary it replaces.
"""

import unittest

import post_review as pr


# old file lines 1..10; line 3 modified, a line inserted after old line 5,
# old line 8 deleted. (from linemap_test.go)
SAMPLE_DIFF = """diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,10 +1,10 @@
 line1
 line2
-line3 old
+line3 new
 line4
 line5
+inserted after5
 line6
 line7
-line8
 line9
 line10
"""

NEW_FILE_DIFF = """diff --git a/added.go b/added.go
new file mode 100644
--- /dev/null
+++ b/added.go
@@ -0,0 +1,3 @@
+package main
+
+func main() {}
"""

DELETED_FILE_DIFF = """diff --git a/gone.go b/gone.go
deleted file mode 100644
--- a/gone.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package main
-
"""

BINARY_DIFF = """diff --git a/logo.png b/logo.png
index 1111111..2222222 100644
Binary files a/logo.png and b/logo.png differ
"""


class OldLineForTest(unittest.TestCase):
    def setUp(self):
        self.hunks = pr.parse_hunks(SAMPLE_DIFF)

    def test_single_hunk_positions(self):
        self.assertEqual(len(self.hunks), 1)
        cases = [
            ("context before changes", 1, 1),
            ("modified line maps to deleted position anchor", 3, 3),
            ("context after modification", 4, 4),
            ("added line anchors to preceding old line", 6, 5),
            ("context shifted by insertion", 7, 6),
            ("context after deletion", 9, 9),
            ("last context line", 10, 10),
        ]
        for name, new_line, want in cases:
            with self.subTest(name):
                self.assertEqual(pr.old_line_for(self.hunks, new_line), want)

    def test_outside_hunks(self):
        # one line added and one deleted -> cumulative delta 0
        self.assertEqual(pr.old_line_for(self.hunks, 42), 42)

    def test_multiple_hunks(self):
        multi = (
            "@@ -1,2 +1,4 @@\n"
            " line1\n"
            "+added2\n"
            "+added3\n"
            " line2\n"
            "@@ -10,3 +12,3 @@\n"
            " line10\n"
            "-line11 old\n"
            "+line11 new\n"
            " line12\n"
        )
        hunks = pr.parse_hunks(multi)
        self.assertEqual(len(hunks), 2)
        # between hunks: new 8 = old 6 (two lines added by hunk 1)
        self.assertEqual(pr.old_line_for(hunks, 8), 6)
        # inside second hunk: modified new 13 anchors to old 11
        self.assertEqual(pr.old_line_for(hunks, 13), 11)

    def test_pure_addition_at_top(self):
        hunks = pr.parse_hunks("@@ -0,0 +1,2 @@\n+first\n+second\n")
        self.assertEqual(pr.old_line_for(hunks, 1), 1)


class ParseDiffTest(unittest.TestCase):
    def test_modified_file(self):
        fd = pr.parse_diff(SAMPLE_DIFF)[0]
        self.assertEqual((fd.old_path, fd.new_path), ("main.go", "main.go"))
        self.assertFalse(fd.is_new or fd.is_deleted or fd.is_binary)

    def test_new_file(self):
        fd = pr.parse_diff(NEW_FILE_DIFF)[0]
        self.assertTrue(fd.is_new)
        self.assertEqual(fd.new_path, "added.go")

    def test_deleted_file(self):
        fd = pr.parse_diff(DELETED_FILE_DIFF)[0]
        self.assertTrue(fd.is_deleted)
        self.assertEqual(fd.new_path, "/dev/null")

    def test_binary_file(self):
        fd = pr.parse_diff(BINARY_DIFF)[0]
        self.assertTrue(fd.is_binary)

    def test_multiple_files(self):
        files = pr.parse_diff(SAMPLE_DIFF + NEW_FILE_DIFF)
        self.assertEqual([f.new_path for f in files], ["main.go", "added.go"])


class Recorder:
    """A post() that records discussions; optionally fails the first inline."""

    def __init__(self, fail_first_inline=False):
        self.calls = []
        self.fail_first_inline = fail_first_inline
        self._inline_seen = 0

    def __call__(self, discussion):
        if self.fail_first_inline and "newPath" in discussion:
            self._inline_seen += 1
            if self._inline_seen == 1:
                raise RuntimeError("simulated 403")
        self.calls.append(discussion)


def diffs_from(diff_text):
    return {fd.new_path: fd for fd in pr.parse_diff(diff_text)}


class PublishTest(unittest.TestCase):
    def test_inline_and_summary(self):
        result = {
            "comments": [{
                "path": "main.go", "content": "possible nil dereference",
                "start_line": 6, "end_line": 6,
                "existing_code": "x := y.Field",
                "suggestion_code": "if y != nil { x = y.Field }",
            }],
        }
        rec = Recorder()
        stats = pr.publish(result, diffs_from(SAMPLE_DIFF), rec)

        self.assertEqual(stats, {"inline": 1, "fallback": 0})
        self.assertEqual(len(rec.calls), 2)  # inline + summary

        inline = rec.calls[0]
        self.assertEqual(inline["newLine"], 6)
        self.assertEqual(inline["oldLine"], 5)
        self.assertEqual((inline["newPath"], inline["oldPath"]), ("main.go", "main.go"))
        self.assertIn("possible nil dereference", inline["message"])
        self.assertIn("**Suggestion:**", inline["message"])

        summary = rec.calls[1]
        self.assertNotIn("newPath", summary)
        self.assertIn("**1** issue(s)", summary["message"])

    def test_fallback_for_unmapped_comment(self):
        result = {
            "comments": [{
                "path": "missing.go", "content": "issue in file absent from diff",
                "start_line": 1, "end_line": 1,
            }],
            "warnings": [{"file": "a.go", "message": "skipped", "type": "subtask_error"}],
        }
        rec = Recorder()
        stats = pr.publish(result, {}, rec)

        self.assertEqual(stats, {"inline": 0, "fallback": 1})
        self.assertEqual(len(rec.calls), 2)  # fallback + summary
        self.assertIn("could not be posted inline", rec.calls[0]["message"])
        self.assertIn("`missing.go`", rec.calls[0]["message"])
        self.assertIn("1 warning(s)", rec.calls[1]["message"])

    def test_inline_error_falls_back(self):
        result = {
            "comments": [{
                "path": "main.go", "content": "finding",
                "start_line": 1, "end_line": 1,
            }],
        }
        rec = Recorder(fail_first_inline=True)
        stats = pr.publish(result, diffs_from(SAMPLE_DIFF), rec)

        self.assertEqual(stats, {"inline": 0, "fallback": 1})
        self.assertEqual(len(rec.calls), 2)  # fallback + summary after inline failure

    def test_no_comments(self):
        rec = Recorder()
        stats = pr.publish({"message": "No comments generated. Looks good to me."}, {}, rec)

        self.assertEqual(stats, {"inline": 0, "fallback": 0})
        self.assertEqual(len(rec.calls), 1)
        self.assertIn("Looks good to me", rec.calls[0]["message"])

    def test_new_file_anchors_to_new_path(self):
        result = {
            "comments": [{
                "path": "added.go", "content": "empty main",
                "start_line": 3, "end_line": 3,
            }],
        }
        rec = Recorder()
        stats = pr.publish(result, diffs_from(NEW_FILE_DIFF), rec)

        self.assertEqual(stats["inline"], 1)
        inline = rec.calls[0]
        self.assertEqual(inline["oldPath"], "added.go")
        self.assertEqual(inline["oldLine"], 1)
        self.assertEqual(inline["newLine"], 3)


if __name__ == "__main__":
    unittest.main()

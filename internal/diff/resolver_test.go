// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

const testDiff = `diff --git a/pkg/example/handler.go b/pkg/example/handler.go
--- a/pkg/example/handler.go
+++ b/pkg/example/handler.go
@@ -10,7 +10,7 @@ func HandleRequest(w http.ResponseWriter, r *http.Request) {
     ctx := r.Context()
-    log.Print("handling request")
+    log.Printf("handling request: %s", r.URL.Path)
     err := process(ctx)`

func TestResolveLineNumbers_DeletedLineIsNotAnchored(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "pkg/example/handler.go", Diff: testDiff},
	}
	comments := []model.LlmComment{
		{Path: "pkg/example/handler.go", ExistingCode: `    log.Print("handling request")`},
	}

	result := ResolveLineNumbers(comments, diffs)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(result))
	}
	cm := result[0]
	// The quoted line is deleted, so the hunk's only candidate number is its
	// old-file 11. New-file line 11 holds the replacement log.Printf call, so
	// publishing that number would anchor the comment to code the quote does
	// not describe. A deleted line has no new-file position, and StartLine/
	// EndLine carry no side, so the comment must stay unanchored instead.
	if cm.StartLine != 0 || cm.EndLine != 0 {
		t.Errorf("deleted-line match: expected 0..0 (unanchored), got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveLineNumbers_WhitespaceTolerant(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "pkg/example/handler.go", Diff: testDiff},
	}
	// LLM may return indented or differently formatted code. The replacement
	// line is added, so it does have a new-file position and the tolerance stays
	// observable: a match that failed outright would leave the comment at 0.
	comments := []model.LlmComment{
		{Path: "pkg/example/handler.go", ExistingCode: `log.Printf("handling request: %s", r.URL.Path)`},
	}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	if cm.StartLine != 11 || cm.EndLine != 11 {
		t.Errorf("whitespace-tolerant match: expected 11..11, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveLineNumbers_MultiLineHunkMatch(t *testing.T) {
	rawMulti := `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -5,4 +5,4 @@ import "fmt"
 func foo() {
-    x := 1
-    y := 2
+    x := 10
+    y := 20
 }`

	diffs := []model.Diff{
		{NewPath: "test.go", Diff: rawMulti},
	}
	// The quote is the replacement pair, not the pair it replaced: the deleted
	// lines number 6..7 in the old file and have no position in the new one, so
	// only the added lines can be anchored.
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: `    x := 10
    y := 20`},
	}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	if cm.StartLine == 0 || cm.EndLine == 0 {
		t.Errorf("multiline hunk match: expected non-zero lines, got StartLine=%d EndLine=%d", cm.StartLine, cm.EndLine)
	}
	if cm.StartLine != 6 || cm.EndLine != 7 {
		t.Errorf("expected 6..7, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveLineNumbers_FallbackToFileContent(t *testing.T) {
	// Code that doesn't appear in diff hunks but exists in file content
	raw := `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
 func foo() {}`

	diffs := []model.Diff{
		{NewPath: "test.go", Diff: raw, NewFileContent: `package main
import "fmt"
func foo() {}`},
	}
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: `package main
import "fmt"`},
	}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	// Fallback should find these consecutive lines starting at line 1
	if cm.StartLine != 1 || cm.EndLine != 2 {
		t.Errorf("fallback: expected 1..2, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveLineNumbers_FallbackToFileContent_BlankLines(t *testing.T) {
	// Hunk match fails; fallback must match across blank lines in NewFileContent.
	diffs := []model.Diff{{
		NewPath:        "main.go",
		NewFileContent: "package main\n\nfunc foo() {\n\n\treturn 1\n}",
		Diff:           "@@ -1,2 +1,2 @@\n-old\n+new",
	}}
	comments := []model.LlmComment{{
		Path:         "main.go",
		ExistingCode: "func foo() {\n\treturn 1\n}",
	}}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	if cm.StartLine != 3 || cm.EndLine != 6 {
		t.Errorf("fallback with blank lines: expected 3..6, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveLineNumbers_FallbackToFileContent_MultipleBlankLines(t *testing.T) {
	diffs := []model.Diff{{
		NewPath:        "main.go",
		NewFileContent: "a\n\n\nb\n\nc\n",
		Diff:           "@@ -1,2 +1,2 @@\n-old\n+new",
	}}
	comments := []model.LlmComment{{
		Path:         "main.go",
		ExistingCode: "a\nb\nc",
	}}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	if cm.StartLine != 1 || cm.EndLine != 6 {
		t.Errorf("multiple blank lines: expected 1..6, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveLineNumbers_FallbackToFileContent_LeadingBlanks(t *testing.T) {
	diffs := []model.Diff{{
		NewPath:        "main.go",
		NewFileContent: "\n\nfoo\nbar\n",
		Diff:           "@@ -1,2 +1,2 @@\n-old\n+new",
	}}
	comments := []model.LlmComment{{
		Path:         "main.go",
		ExistingCode: "foo\nbar",
	}}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	if cm.StartLine != 3 || cm.EndLine != 4 {
		t.Errorf("leading blanks: expected 3..4, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveLineNumbers_FallbackToFileContent_CRLF(t *testing.T) {
	diffs := []model.Diff{{
		NewPath:        "main.go",
		NewFileContent: "alpha\r\nbeta\r\ngamma\r\n",
		Diff:           "@@ -1,2 +1,2 @@\n-old\n+new",
	}}
	comments := []model.LlmComment{{
		Path:         "main.go",
		ExistingCode: "beta\ngamma",
	}}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	if cm.StartLine != 2 || cm.EndLine != 3 {
		t.Errorf("CRLF: expected 2..3, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveLineNumbers_FallbackToFileContent_FirstMatchWins(t *testing.T) {
	diffs := []model.Diff{{
		NewPath:        "main.go",
		NewFileContent: "x\ny\nx\ny\n",
		Diff:           "@@ -1,2 +1,2 @@\n-old\n+new",
	}}
	comments := []model.LlmComment{{
		Path:         "main.go",
		ExistingCode: "x\ny",
	}}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	if cm.StartLine != 1 || cm.EndLine != 2 {
		t.Errorf("first match wins: expected 1..2, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveLineNumbers_FallbackToFileContent_AllBlankExistingCode(t *testing.T) {
	diffs := []model.Diff{{
		NewPath:        "main.go",
		NewFileContent: "a\nb\nc\n",
		Diff:           "@@ -1,2 +1,2 @@\n-old\n+new",
	}}
	comments := []model.LlmComment{{
		Path:         "main.go",
		ExistingCode: "\n\n\n",
	}}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	if cm.StartLine != 0 || cm.EndLine != 0 {
		t.Errorf("all-blank existing code: expected 0..0, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveLineNumbers_NoMatchKeepsZero(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "test.go", Diff: testDiff},
	}
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: `totally unrelated code`},
	}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	if cm.StartLine != 0 || cm.EndLine != 0 {
		t.Errorf("no match: expected 0..0, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveLineNumbers_NoExistingCode(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "test.go", Diff: testDiff},
	}
	comments := []model.LlmComment{
		{Path: "test.go", Content: "some comment without existing_code"},
	}

	result := ResolveLineNumbers(comments, diffs)
	if result[0].StartLine != 0 {
		t.Errorf("empty ExistingCode: expected 0, got %d", result[0].StartLine)
	}
}

func TestResolveLineNumbers_PathNotFound(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "other.go", Diff: testDiff},
	}
	comments := []model.LlmComment{
		{Path: "missing.go", ExistingCode: `some code`},
	}

	result := ResolveLineNumbers(comments, diffs)
	if result[0].StartLine != 0 {
		t.Errorf("path not found: expected 0, got %d", result[0].StartLine)
	}
}

func TestResolveLineNumbers_EmptyInputs(t *testing.T) {
	// No comments
	r1 := ResolveLineNumbers([]model.LlmComment{}, []model.Diff{{}})
	if len(r1) != 0 {
		t.Errorf("empty comments: expected 0 results, got %d", len(r1))
	}

	// No diffs — returns comments unchanged (line numbers stay at 0)
	r2 := ResolveLineNumbers([]model.LlmComment{{}}, []model.Diff{})
	if len(r2) != 1 || r2[0].StartLine != 0 {
		t.Errorf("empty diffs: expected 1 result with StartLine=0, got %d", len(r2))
	}
}

func TestNormalizeLine(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"  hello  ", "hello"},
		{"+added line", "added line"},
		{"-deleted line", "deleted line"},
		{"\tindented\t", "indented"},
		{"", ""},
	}

	for _, tt := range tests {
		got := normalizeLine(tt.input)
		if got != tt.want {
			t.Errorf("normalizeLine(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSplitAndNormalize_SkipsEmptyLines(t *testing.T) {
	lines := splitAndNormalize(`line1

line2`)

	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "line1" || lines[1] != "line2" {
		t.Errorf("got %v", lines)
	}
}

// ---------------------------------------------------------------------------
// extractSideLines unit tests
// ---------------------------------------------------------------------------

func TestExtractSideLines_NewSide(t *testing.T) {
	hunk := Hunk{
		OldStart: 10, OldCount: 3,
		NewStart: 10, NewCount: 4,
		Lines: []HunkLine{
			{HunkContext, "    ctx := r.Context()"},
			{HunkDeleted, `    log.Print("old")`},
			{HunkAdded, `    log.Printf("new: %s", r.URL)`},
			{HunkContext, "    err := process(ctx)"},
		},
	}

	got := extractSideLines(&hunk, true)

	want := []indexedLine{
		{10, 10, `ctx := r.Context()`},
		{11, 11, `log.Printf("new: %s", r.URL)`},
		{12, 12, `err := process(ctx)`},
	}

	if len(got) != len(want) {
		t.Fatalf("new-side: expected %d lines, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i].lineNum != want[i].lineNum || got[i].content != want[i].content {
			t.Errorf("new-side[%d]: got {%d, %q}, want {%d, %q}",
				i, got[i].lineNum, got[i].content, want[i].lineNum, want[i].content)
		}
	}
}

func TestExtractSideLines_OldSide(t *testing.T) {
	hunk := Hunk{
		OldStart: 10, OldCount: 3,
		NewStart: 10, NewCount: 4,
		Lines: []HunkLine{
			{HunkContext, "    ctx := r.Context()"},
			{HunkDeleted, `    log.Print("old")`},
			{HunkAdded, `    log.Printf("new: %s", r.URL)`},
			{HunkContext, "    err := process(ctx)"},
		},
	}

	got := extractSideLines(&hunk, false)

	// The old side numbers lines in the old file, and each line also records
	// where it landed in the new file: the deleted line has no position there,
	// which is what lets the resolver refuse to publish an old-file number.
	want := []indexedLine{
		{lineNum: 10, newLineNum: 10, content: `ctx := r.Context()`},
		{lineNum: 11, newLineNum: 0, content: `log.Print("old")`},
		{lineNum: 12, newLineNum: 12, content: `err := process(ctx)`},
	}

	if len(got) != len(want) {
		t.Fatalf("old-side: expected %d lines, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("old-side[%d]: got {%d, %d, %q}, want {%d, %d, %q}",
				i, got[i].lineNum, got[i].newLineNum, got[i].content,
				want[i].lineNum, want[i].newLineNum, want[i].content)
		}
	}
}

func TestExtractSideLines_DivergentStartLines(t *testing.T) {
	hunk := Hunk{
		OldStart: 5, OldCount: 2,
		NewStart: 8, NewCount: 3,
		Lines: []HunkLine{
			{HunkContext, "A"},
			{HunkAdded, "B"},
			{HunkContext, "C"},
		},
	}

	newSide := extractSideLines(&hunk, true)
	if len(newSide) != 3 {
		t.Fatalf("expected 3 new-side lines, got %d", len(newSide))
	}
	// new-side: A(8), B(9), C(10)
	wantNew := []int{8, 9, 10}
	for i, w := range wantNew {
		if newSide[i].lineNum != w {
			t.Errorf("new-side[%d].lineNum = %d, want %d", i, newSide[i].lineNum, w)
		}
	}

	oldSide := extractSideLines(&hunk, false)
	if len(oldSide) != 2 {
		t.Fatalf("expected 2 old-side lines, got %d", len(oldSide))
	}
	// old-side: A(5), C(6)
	wantOld := []int{5, 6}
	for i, w := range wantOld {
		if oldSide[i].lineNum != w {
			t.Errorf("old-side[%d].lineNum = %d, want %d", i, oldSide[i].lineNum, w)
		}
	}
	// The same two lines are A(new 8) and C(new 10); the insertion at B is what
	// pulls the numberings apart, and it is why an old-side match has to be
	// re-read on the new side before any number is published.
	wantOldNew := []int{8, 10}
	for i, w := range wantOldNew {
		if oldSide[i].newLineNum != w {
			t.Errorf("old-side[%d].newLineNum = %d, want %d", i, oldSide[i].newLineNum, w)
		}
	}
}

func TestExtractSideLines_OnlyAdded(t *testing.T) {
	hunk := Hunk{
		OldStart: 1, OldCount: 0,
		NewStart: 1, NewCount: 2,
		Lines: []HunkLine{
			{HunkAdded, "line1"},
			{HunkAdded, "line2"},
		},
	}

	newSide := extractSideLines(&hunk, true)
	if len(newSide) != 2 {
		t.Fatalf("expected 2 new-side lines, got %d", len(newSide))
	}

	oldSide := extractSideLines(&hunk, false)
	if len(oldSide) != 0 {
		t.Errorf("expected 0 old-side lines, got %d", len(oldSide))
	}
}

func TestExtractSideLines_OnlyDeleted(t *testing.T) {
	hunk := Hunk{
		OldStart: 3, OldCount: 2,
		NewStart: 3, NewCount: 0,
		Lines: []HunkLine{
			{HunkDeleted, "old1"},
			{HunkDeleted, "old2"},
		},
	}

	oldSide := extractSideLines(&hunk, false)
	if len(oldSide) != 2 {
		t.Fatalf("expected 2 old-side lines, got %d", len(oldSide))
	}
	if oldSide[0].lineNum != 3 || oldSide[1].lineNum != 4 {
		t.Errorf("expected line nums 3,4, got %d,%d", oldSide[0].lineNum, oldSide[1].lineNum)
	}

	newSide := extractSideLines(&hunk, true)
	if len(newSide) != 0 {
		t.Errorf("expected 0 new-side lines, got %d", len(newSide))
	}
}

// ---------------------------------------------------------------------------
// matchConsecutive unit tests
// ---------------------------------------------------------------------------

func TestMatchConsecutive_SingleLine(t *testing.T) {
	lines := []indexedLine{{5, 5, "hello"}, {6, 6, "world"}, {7, 7, "foo"}}
	start, end, ok := matchConsecutive(lines, []string{"world"})
	if !ok || start != 6 || end != 6 {
		t.Errorf("single-line: got (%d, %d, %v), want (6, 6, true)", start, end, ok)
	}
}

func TestMatchConsecutive_MultiLine(t *testing.T) {
	lines := []indexedLine{{1, 1, "a"}, {2, 2, "b"}, {3, 3, "c"}, {4, 4, "d"}}
	start, end, ok := matchConsecutive(lines, []string{"b", "c"})
	if !ok || start != 2 || end != 3 {
		t.Errorf("multi-line: got (%d, %d, %v), want (2, 3, true)", start, end, ok)
	}
}

func TestMatchConsecutive_NoMatch(t *testing.T) {
	lines := []indexedLine{{1, 1, "a"}, {2, 2, "b"}}
	_, _, ok := matchConsecutive(lines, []string{"x"})
	if ok {
		t.Errorf("expected no match")
	}
}

func TestMatchConsecutive_FirstMatchWins(t *testing.T) {
	lines := []indexedLine{{10, 10, "x"}, {11, 11, "y"}, {20, 20, "x"}, {21, 21, "y"}}
	start, end, ok := matchConsecutive(lines, []string{"x", "y"})
	if !ok || start != 10 || end != 11 {
		t.Errorf("first match: got (%d, %d, %v), want (10, 11, true)", start, end, ok)
	}
}

func TestMatchConsecutive_TargetLongerThanLines(t *testing.T) {
	lines := []indexedLine{{1, 1, "a"}}
	_, _, ok := matchConsecutive(lines, []string{"a", "b"})
	if ok {
		t.Errorf("expected no match when target is longer")
	}
}

func TestMatchConsecutive_EmptySideLines(t *testing.T) {
	_, _, ok := matchConsecutive(nil, []string{"a"})
	if ok {
		t.Errorf("expected no match on empty side lines")
	}
}

func TestMatchConsecutive_MatchAtEnd(t *testing.T) {
	lines := []indexedLine{{1, 1, "a"}, {2, 2, "b"}, {3, 3, "c"}}
	start, end, ok := matchConsecutive(lines, []string{"b", "c"})
	if !ok || start != 2 || end != 3 {
		t.Errorf("match-at-end: got (%d, %d, %v), want (2, 3, true)", start, end, ok)
	}
}

func TestMatchConsecutive_MatchAtStart(t *testing.T) {
	lines := []indexedLine{{1, 1, "a"}, {2, 2, "b"}, {3, 3, "c"}}
	start, end, ok := matchConsecutive(lines, []string{"a", "b"})
	if !ok || start != 1 || end != 2 {
		t.Errorf("match-at-start: got (%d, %d, %v), want (1, 2, true)", start, end, ok)
	}
}

func TestMatchConsecutive_ExactFull(t *testing.T) {
	lines := []indexedLine{{1, 1, "a"}, {2, 2, "b"}}
	start, end, ok := matchConsecutive(lines, []string{"a", "b"})
	if !ok || start != 1 || end != 2 {
		t.Errorf("exact-full: got (%d, %d, %v), want (1, 2, true)", start, end, ok)
	}
}

// ---------------------------------------------------------------------------
// newSideSpan unit tests
// ---------------------------------------------------------------------------

func TestNewSideSpan_SurvivingLines(t *testing.T) {
	// Old-file 5 and 6, new-file 5 and 7: the insertion between them is absent
	// from the old-side view, so the matched run is wider on the new side.
	matched := []indexedLine{
		{lineNum: 5, newLineNum: 5, content: "x := 1"},
		{lineNum: 6, newLineNum: 7, content: "y := 2"},
	}
	start, end, anchored := newSideSpan(matched)
	if !anchored || start != 5 || end != 7 {
		t.Errorf("got (%d, %d, %v), want (5, 7, true)", start, end, anchored)
	}
}

func TestNewSideSpan_DeletedLine(t *testing.T) {
	matched := []indexedLine{
		{lineNum: 5, newLineNum: 5, content: "x := 1"},
		{lineNum: 6, newLineNum: 0, content: "removed()"},
	}
	start, end, anchored := newSideSpan(matched)
	if anchored || start != 0 || end != 0 {
		t.Errorf("got (%d, %d, %v), want (0, 0, false)", start, end, anchored)
	}
}

func TestNewSideSpan_SingleDeletedLine(t *testing.T) {
	start, end, anchored := newSideSpan([]indexedLine{
		{lineNum: 2, newLineNum: 0, content: "legacyCall()"},
	})
	if anchored || start != 0 || end != 0 {
		t.Errorf("got (%d, %d, %v), want (0, 0, false)", start, end, anchored)
	}
}

// ---------------------------------------------------------------------------
// resolveFromHunk integration tests
// ---------------------------------------------------------------------------

func TestResolveFromHunk_AddedLines(t *testing.T) {
	raw := `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -3,3 +3,5 @@
 func main() {
+    x := 1
+    y := 2
     fmt.Println("hello")
 }`

	diffs := []model.Diff{{NewPath: "test.go", Diff: raw}}
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: `    x := 1
    y := 2`},
	}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	if cm.StartLine != 4 || cm.EndLine != 5 {
		t.Errorf("added lines: expected 4..5, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveFromHunk_OldSideAcrossAddedLines(t *testing.T) {
	raw := `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -5,3 +5,4 @@
     x := 1
+    z := 99
     y := 2
 }`

	diffs := []model.Diff{{NewPath: "test.go", Diff: raw}}
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: `    x := 1
    y := 2`},
	}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	// Both quoted lines survive, but the insertion splits them in the new file:
	// y := 2 moves from old-file 6 to new-file 7. Old-side extracts
	// [x:=1(5), y:=2(6), }(7)] and matches at 5..6, which is only correct for the
	// old file. Every matched line has a new-file number, so the span is
	// reported as 5..7; the anchor at 5 is exact, since that is where the quote
	// starts in the new file.
	if cm.StartLine != 5 || cm.EndLine != 7 {
		t.Errorf("old-side across added: expected new-file 5..7, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveLineNumbers_DeletedCodeBelowInsertions(t *testing.T) {
	// The report in #1486. Ten lines are inserted above a deleted legacyCall(),
	// and a comment quotes the deleted code. Old-side matching returned old-file
	// line 2, which it published as StartLine; new-file line 2 is added1(), so
	// the comment landed on unrelated inserted code with no field recording that
	// the number was an old-file one. Old-file 2 is not "line 2 of the file
	// being reviewed" - the deletion makes the two numberings disagree.
	raw := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,4 +1,13 @@
 package main
+    added1()
+    added2()
+    added3()
+    added4()
+    added5()
+    added6()
+    added7()
+    added8()
+    added9()
+    added10()
-    legacyCall()
 func foo() {
 }`

	// The two cases are the paths a caller can actually reach: hunks alone, and
	// the new-file content that the fallback scans when the hunks decline.
	cases := []struct {
		name    string
		content string
	}{
		{name: "hunks only"},
		{
			name: "with new file content",
			content: `package main
    added1()
    added2()
    added3()
    added4()
    added5()
    added6()
    added7()
    added8()
    added9()
    added10()
func foo() {
}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diffs := []model.Diff{{NewPath: "main.go", Diff: raw, NewFileContent: tc.content}}
			comments := []model.LlmComment{
				{Path: "main.go", ExistingCode: `    legacyCall()`},
			}

			result := ResolveLineNumbers(comments, diffs)
			cm := result[0]
			if cm.StartLine != 0 || cm.EndLine != 0 {
				t.Errorf("deleted legacyCall(): expected 0..0 (unanchored), got %d..%d", cm.StartLine, cm.EndLine)
			}
		})
	}
}

func TestResolveFromHunk_ContextLinesOnly(t *testing.T) {
	// When existing_code matches context (unchanged) lines rather than deleted ones
	raw := `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -3,3 +3,4 @@
 func main() {
     fmt.Println("hello")
+    fmt.Println("world")
 }`

	diffs := []model.Diff{{NewPath: "test.go", Diff: raw}}
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: `    fmt.Println("hello")`},
	}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	if cm.StartLine == 0 {
		t.Errorf("context-line match: expected non-zero start, got 0")
	}
	if cm.StartLine != 4 {
		t.Errorf("expected line 4, got %d", cm.StartLine)
	}
}

func TestResolveFromHunk_SingleAddedLine(t *testing.T) {
	raw := `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -1,2 +1,3 @@
 package main
+import "fmt"
 func main() {}`

	diffs := []model.Diff{{NewPath: "test.go", Diff: raw}}
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: `import "fmt"`},
	}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	if cm.StartLine != 2 || cm.EndLine != 2 {
		t.Errorf("single added line: expected 2..2, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveFromHunk_NewSidePriority(t *testing.T) {
	// "fmt.Println" appears on both sides (context line).
	// OldStart differs from NewStart → verifies new-side wins with new-file line number.
	raw := `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -5,3 +8,4 @@
 func main() {
     fmt.Println("hello")
+    fmt.Println("world")
 }`

	diffs := []model.Diff{{NewPath: "test.go", Diff: raw}}
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: `    fmt.Println("hello")`},
	}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	// new-side: func main(8), fmt.Println("hello")(9), fmt.Println("world")(10), }(11)
	// old-side: func main(5), fmt.Println("hello")(6), }(7)
	// new-side wins → line 9
	if cm.StartLine != 9 {
		t.Errorf("new-side priority: expected 9, got %d", cm.StartLine)
	}
}

func TestResolveFromHunk_MultiHunkMatchInSecond(t *testing.T) {
	raw := `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -2,3 +2,3 @@
 func foo() {
-    old1()
+    new1()
 }
@@ -20,3 +20,4 @@
 func bar() {
+    added_in_bar()
     existing()
 }`

	diffs := []model.Diff{{NewPath: "test.go", Diff: raw}}
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: "    added_in_bar()"},
	}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	// Second hunk @@ -20,3 +20,4: new-side → func bar(20), added_in_bar(21), existing(22), }(23)
	if cm.StartLine != 21 || cm.EndLine != 21 {
		t.Errorf("multi-hunk second: expected 21..21, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveFromHunk_AddedWithContext(t *testing.T) {
	raw := `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -10,3 +10,5 @@
 func process() {
+    validate()
+    transform()
     save()
 }`

	diffs := []model.Diff{{NewPath: "test.go", Diff: raw}}
	// LLM provides added line + surrounding context
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: `    validate()
    transform()
    save()`},
	}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	// new-side: process(10), validate(11), transform(12), save(13), }(14)
	if cm.StartLine != 11 || cm.EndLine != 13 {
		t.Errorf("added+context: expected 11..13, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveFromHunk_NewSideAcrossDeletedLines(t *testing.T) {
	raw := `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -5,4 +5,3 @@
     a := 1
-    unused := 0
     b := 2
 }`

	diffs := []model.Diff{{NewPath: "test.go", Diff: raw}}
	// Target spans two context lines that have a deleted line between them in the hunk.
	// In the new file they are consecutive.
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: `    a := 1
    b := 2`},
	}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	// new-side: a:=1(5), b:=2(6), }(7) — deleted line skipped, consecutive
	if cm.StartLine != 5 || cm.EndLine != 6 {
		t.Errorf("new-side across deleted: expected 5..6, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

// ---------------------------------------------------------------------------
// ResolveLineNumbers integration tests (additional scenarios)
// ---------------------------------------------------------------------------

func TestResolveLineNumbers_AlreadyResolved(t *testing.T) {
	diffs := []model.Diff{{NewPath: "test.go", Diff: testDiff}}
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: `log.Print("handling request")`, StartLine: 99, EndLine: 99},
	}

	result := ResolveLineNumbers(comments, diffs)
	if result[0].StartLine != 99 || result[0].EndLine != 99 {
		t.Errorf("already resolved: expected 99..99, got %d..%d", result[0].StartLine, result[0].EndLine)
	}
}

func TestResolveLineNumbers_MultipleCommentsOnSameFile(t *testing.T) {
	raw := `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -1,4 +1,6 @@
 package main
+import "fmt"
+import "os"
 func main() {
-    old()
+    new()
 }`

	diffs := []model.Diff{{NewPath: "test.go", Diff: raw}}
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: `import "fmt"`},
		{Path: "test.go", ExistingCode: `import "os"`},
		{Path: "test.go", ExistingCode: "    old()"},
	}

	result := ResolveLineNumbers(comments, diffs)
	// comment 0: added import "fmt" → new-side line 2
	if result[0].StartLine != 2 || result[0].EndLine != 2 {
		t.Errorf("comment[0]: expected 2..2, got %d..%d", result[0].StartLine, result[0].EndLine)
	}
	// comment 1: added import "os" → new-side line 3
	if result[1].StartLine != 3 || result[1].EndLine != 3 {
		t.Errorf("comment[1]: expected 3..3, got %d..%d", result[1].StartLine, result[1].EndLine)
	}
	// comment 2: deleted old() has no new-file position, so it stays unanchored
	// rather than reporting old-file line 3, which is import "os" in the new file.
	if result[2].StartLine != 0 || result[2].EndLine != 0 {
		t.Errorf("comment[2]: expected 0..0 (unanchored), got %d..%d", result[2].StartLine, result[2].EndLine)
	}
}

func TestResolveLineNumbers_OldPathMapping(t *testing.T) {
	raw := `diff --git a/old_name.go b/new_name.go
--- a/old_name.go
+++ b/new_name.go
@@ -1,3 +1,3 @@
 package main
-func oldFunc() {}
+func newFunc() {}`

	diffs := []model.Diff{{OldPath: "old_name.go", NewPath: "new_name.go", Diff: raw}}
	// Comment references the old path — it should still resolve through
	// diffByPath[oldPath]. The quote is the replacement line: the deleted
	// func oldFunc() has no new-file position, so quoting it would leave the
	// comment unanchored and prove nothing about the path lookup.
	comments := []model.LlmComment{
		{Path: "old_name.go", ExistingCode: "func newFunc() {}"},
	}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	if cm.StartLine != 2 || cm.EndLine != 2 {
		t.Errorf("old path mapping: expected 2..2, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveLineNumbers_MixedStrategies(t *testing.T) {
	raw := `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -5,3 +5,4 @@
 func foo() {
+    newLine()
     bar()
 }`

	diffs := []model.Diff{
		{
			NewPath: "test.go", Diff: raw,
			NewFileContent: "package main\nimport \"fmt\"\n\nfunc helper() {}\nfunc foo() {\n    newLine()\n    bar()\n}",
		},
	}
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: "    newLine()"},                  // hunk new-side
		{Path: "test.go", ExistingCode: "func helper() {}"},               // not in hunk, file content fallback
		{Path: "test.go", ExistingCode: "this_does_not_exist_anywhere()"}, // no match
	}

	result := ResolveLineNumbers(comments, diffs)

	if result[0].StartLine != 6 || result[0].EndLine != 6 {
		t.Errorf("hunk match: expected 6..6, got %d..%d", result[0].StartLine, result[0].EndLine)
	}
	if result[1].StartLine != 4 || result[1].EndLine != 4 {
		t.Errorf("file content fallback: expected 4..4, got %d..%d", result[1].StartLine, result[1].EndLine)
	}
	if result[2].StartLine != 0 || result[2].EndLine != 0 {
		t.Errorf("no match: expected 0..0, got %d..%d", result[2].StartLine, result[2].EndLine)
	}
}

func TestResolveLineNumbers_DiffMarkerInExistingCode(t *testing.T) {
	raw := `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -1,2 +1,3 @@
 x := 1
+y := 2
 z := 3`

	diffs := []model.Diff{{NewPath: "test.go", Diff: raw}}
	// LLM may include the '+' marker from diff output
	comments := []model.LlmComment{
		{Path: "test.go", ExistingCode: "+y := 2"},
	}

	result := ResolveLineNumbers(comments, diffs)
	cm := result[0]
	// normalizeLine strips leading '+', so "+y := 2" → "y := 2" matches
	if cm.StartLine != 2 || cm.EndLine != 2 {
		t.Errorf("diff marker in existing_code: expected 2..2, got %d..%d", cm.StartLine, cm.EndLine)
	}
}

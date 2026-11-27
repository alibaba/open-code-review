// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"reflect"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestLineBlockContains(t *testing.T) {
	long := "rows, err := db.Query(query)\nif err != nil {\n    return err\n}"
	short := "rows, err := db.Query(query)"

	tests := []struct {
		name  string
		outer string
		inner string
		want  bool
	}{
		{name: "whole line at start", outer: long, inner: short, want: true},
		{name: "whole line in the middle", outer: "a := 1\n" + short + "\nb := 2", inner: short, want: true},
		{name: "blank lines are ignored", outer: "a := 1\n\n" + short + "\nb := 2", inner: short, want: true},
		{name: "inner is longer", outer: short, inner: long, want: false},
		{name: "short inner", outer: "if err != nil {\n    return nil\n}", inner: "return nil", want: false},
		{name: "partial line", outer: "if err != nil { return err }", inner: "err != nil { return err }", want: false},
		{name: "not contiguous", outer: "alpha line here\nbeta line here\ngamma line here", inner: "alpha line here\ngamma line here", want: false},
		{name: "empty inner", outer: long, inner: "", want: false},
		{name: "empty outer", outer: "", inner: long, want: false},
		{name: "same line count", outer: long, inner: long, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lineBlockContains(tt.outer, tt.inner); got != tt.want {
				t.Fatalf("lineBlockContains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizedLines(t *testing.T) {
	got := normalizedLines("  a := 1\r\n\r\n\tb := 2  \n")
	want := []string{"a := 1", "b := 2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizedLines() = %v, want %v", got, want)
	}
}

func TestMatchContainedSnippetPicksClosestLength(t *testing.T) {
	before := testComment("a.go", 10, "rows, err := db.Query(query)")
	closer := testComment("a.go", 30, "rows, err := db.Query(query)\nif err != nil {\n    return err\n}")
	farther := testComment("a.go", 20, "rows, err := db.Query(query)\nif err != nil {\n    return err\n}\nlog.Error(err)\nreturn nil")
	after := []model.LlmComment{farther, closer}

	if got := matchContainedSnippet(before, after, []int{0, 1}, []bool{false, false}); got != 1 {
		t.Fatalf("matchContainedSnippet() = %d, want 1 (the closest length)", got)
	}
	if got := matchContainedSnippet(before, after, []int{0, 1}, []bool{true, true}); got != -1 {
		t.Fatalf("matchContainedSnippet() = %d, want -1 when all candidates are used", got)
	}
	if got := matchContainedSnippet(before, after, nil, nil); got != -1 {
		t.Fatalf("matchContainedSnippet() = %d, want -1 with no candidates", got)
	}
}

func TestFindingBodyFallsBackToContent(t *testing.T) {
	withSnippet := cmt("a.go", 10, "bug", "  x := 1  ", "ignored")
	if got := findingBody(withSnippet); got != "x := 1" {
		t.Fatalf("findingBody() = %q, want the normalized snippet", got)
	}
	withoutSnippet := cmt("a.go", 10, "bug", "", "  missing  error check ")
	if got := findingBody(withoutSnippet); got != "missing error check" {
		t.Fatalf("findingBody() = %q, want the normalized content", got)
	}
}

func testComment(path string, line int, snippet string) model.LlmComment {
	return model.LlmComment{
		Path:         path,
		StartLine:    line,
		EndLine:      line,
		Category:     "bug",
		ExistingCode: snippet,
		Content:      "test",
	}
}

func TestFirstUnused(t *testing.T) {
	if got := firstUnused([]int{0, 1}, []bool{false, false}); got != 0 {
		t.Fatalf("firstUnused() = %d, want 0", got)
	}
	if got := firstUnused([]int{1, 2}, []bool{false, true, false}); got != 2 {
		t.Fatalf("firstUnused() = %d, want 2", got)
	}
	if got := firstUnused(nil, nil); got != -1 {
		t.Fatalf("firstUnused() = %d, want -1", got)
	}
}

func TestAbsInt(t *testing.T) {
	for _, tc := range []struct {
		in, want int
	}{
		{in: -3, want: 3},
		{in: 0, want: 0},
		{in: 4, want: 4},
	} {
		if got := absInt(tc.in); got != tc.want {
			t.Fatalf("absInt(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

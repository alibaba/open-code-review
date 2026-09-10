// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

// cmt builds a finding compactly: path, start line, category, snippet, content.
func cmt(path string, line int, category, snippet, content string) model.LlmComment {
	return model.LlmComment{
		Path:         path,
		StartLine:    line,
		EndLine:      line,
		Category:     category,
		ExistingCode: snippet,
		Content:      content,
	}
}

// ids renders a bucket as "path:startline" entries, so a case pins both the
// membership and the order Compare promises.
func ids(comments []model.LlmComment) []string {
	out := []string{}
	for _, c := range comments {
		out = append(out, c.Path+":"+strconv.Itoa(c.StartLine))
	}
	return out
}

func TestCompare(t *testing.T) {
	tests := []struct {
		name            string
		before          []model.LlmComment
		after           []model.LlmComment
		reviewed        map[string]bool
		wantNew         []string
		wantPersisting  []string
		wantResolved    []string
		wantNotReviewed []string
	}{
		{
			name:           "line drift keeps the finding persisting",
			before:         []model.LlmComment{cmt("a.go", 40, "bug", "x := 1", "unused")},
			after:          []model.LlmComment{cmt("a.go", 52, "bug", "x := 1", "unused")},
			wantNew:        []string{},
			wantPersisting: []string{"a.go:52"},
			wantResolved:   []string{},
		},
		{
			name:           "reindented snippet keeps the finding persisting",
			before:         []model.LlmComment{cmt("a.go", 40, "bug", "x := 1\ny := 2", "unused")},
			after:          []model.LlmComment{cmt("a.go", 41, "bug", "\t\tx := 1\n\t\ty := 2", "unused")},
			wantNew:        []string{},
			wantPersisting: []string{"a.go:41"},
			wantResolved:   []string{},
		},
		{
			name:           "reworded content with the same snippet persists",
			before:         []model.LlmComment{cmt("a.go", 40, "bug", "x := 1", "this variable is unused")},
			after:          []model.LlmComment{cmt("a.go", 40, "bug", "x := 1", "x is never read")},
			wantNew:        []string{},
			wantPersisting: []string{"a.go:40"},
			wantResolved:   []string{},
		},
		{
			name:           "category is matched case insensitively",
			before:         []model.LlmComment{cmt("a.go", 40, "Bug", "x := 1", "unused")},
			after:          []model.LlmComment{cmt("a.go", 40, "bug", "x := 1", "unused")},
			wantNew:        []string{},
			wantPersisting: []string{"a.go:40"},
			wantResolved:   []string{},
		},
		{
			name:           "changed category splits into new and resolved",
			before:         []model.LlmComment{cmt("a.go", 40, "bug", "x := 1", "unused")},
			after:          []model.LlmComment{cmt("a.go", 40, "style", "x := 1", "unused")},
			wantNew:        []string{"a.go:40"},
			wantPersisting: []string{},
			wantResolved:   []string{"a.go:40"},
		},
		{
			name:           "changed snippet splits into new and resolved",
			before:         []model.LlmComment{cmt("a.go", 40, "bug", "x := 1", "unused")},
			after:          []model.LlmComment{cmt("a.go", 40, "bug", "x := 2", "unused")},
			wantNew:        []string{"a.go:40"},
			wantPersisting: []string{},
			wantResolved:   []string{"a.go:40"},
		},
		{
			name: "two before and one after leaves one resolved",
			before: []model.LlmComment{
				cmt("a.go", 10, "bug", "x := 1", "unused"),
				cmt("a.go", 20, "bug", "x := 1", "unused"),
			},
			after:          []model.LlmComment{cmt("a.go", 11, "bug", "x := 1", "unused")},
			wantNew:        []string{},
			wantPersisting: []string{"a.go:11"},
			wantResolved:   []string{"a.go:20"},
		},
		{
			name: "two before and three after leaves one new",
			before: []model.LlmComment{
				cmt("a.go", 10, "bug", "x := 1", "unused"),
				cmt("a.go", 20, "bug", "x := 1", "unused"),
			},
			after: []model.LlmComment{
				cmt("a.go", 11, "bug", "x := 1", "unused"),
				cmt("a.go", 21, "bug", "x := 1", "unused"),
				cmt("a.go", 31, "bug", "x := 1", "unused"),
			},
			wantNew:        []string{"a.go:31"},
			wantPersisting: []string{"a.go:11", "a.go:21"},
			wantResolved:   []string{},
		},
		{
			name:           "blank snippet falls back to content",
			before:         []model.LlmComment{cmt("a.go", 40, "bug", "", "missing error check")},
			after:          []model.LlmComment{cmt("a.go", 77, "bug", "", "missing   error check")},
			wantNew:        []string{},
			wantPersisting: []string{"a.go:77"},
			wantResolved:   []string{},
		},
		{
			name:           "blank snippet reworded splits into new and resolved",
			before:         []model.LlmComment{cmt("a.go", 40, "bug", "", "missing error check")},
			after:          []model.LlmComment{cmt("a.go", 40, "bug", "", "the error is dropped")},
			wantNew:        []string{"a.go:40"},
			wantPersisting: []string{},
			wantResolved:   []string{"a.go:40"},
		},
		{
			name:           "empty before makes everything new",
			after:          []model.LlmComment{cmt("a.go", 5, "bug", "x := 1", "unused")},
			wantNew:        []string{"a.go:5"},
			wantPersisting: []string{},
			wantResolved:   []string{},
		},
		{
			name:           "empty after makes everything resolved",
			before:         []model.LlmComment{cmt("a.go", 5, "bug", "x := 1", "unused")},
			wantNew:        []string{},
			wantPersisting: []string{},
			wantResolved:   []string{"a.go:5"},
		},
		{
			name:            "two empty sessions produce four empty buckets",
			wantNew:         []string{},
			wantPersisting:  []string{},
			wantResolved:    []string{},
			wantNotReviewed: []string{},
		},
		{
			name: "a file the after run skipped is not resolved",
			before: []model.LlmComment{
				cmt("a.go", 5, "bug", "x := 1", "unused"),
				cmt("skipped.go", 9, "bug", "y := 2", "unused"),
			},
			after:           []model.LlmComment{},
			reviewed:        map[string]bool{"a.go": true},
			wantNew:         []string{},
			wantPersisting:  []string{},
			wantResolved:    []string{"a.go:5"},
			wantNotReviewed: []string{"skipped.go:9"},
		},
		{
			name: "a nil reviewed set restores naive resolved",
			before: []model.LlmComment{
				cmt("a.go", 5, "bug", "x := 1", "unused"),
				cmt("skipped.go", 9, "bug", "y := 2", "unused"),
			},
			reviewed:        nil,
			wantNew:         []string{},
			wantPersisting:  []string{},
			wantResolved:    []string{"a.go:5", "skipped.go:9"},
			wantNotReviewed: []string{},
		},
		{
			name:           "reviewed paths match across path spellings",
			before:         []model.LlmComment{cmt("./pkg/../pkg/a.go", 5, "bug", "x := 1", "unused")},
			reviewed:       map[string]bool{"pkg/a.go": true},
			wantNew:        []string{},
			wantPersisting: []string{},
			wantResolved:   []string{"./pkg/../pkg/a.go:5"},
		},
		{
			name:            "a path-less finding is never claimed as resolved",
			before:          []model.LlmComment{cmt("", 5, "bug", "x := 1", "unplaceable")},
			reviewed:        map[string]bool{"a.go": true},
			wantNew:         []string{},
			wantPersisting:  []string{},
			wantResolved:    []string{},
			wantNotReviewed: []string{":5"},
		},
		{
			name: "buckets are sorted regardless of input order",
			before: []model.LlmComment{
				cmt("z.go", 3, "bug", "gone-z", "unused"),
				cmt("a.go", 90, "style", "gone-a90", "unused"),
				cmt("a.go", 7, "bug", "gone-a7", "unused"),
				cmt("a.go", 7, "aaa", "gone-a7-aaa", "unused"),
			},
			after: []model.LlmComment{
				cmt("m.go", 4, "bug", "fresh-m", "unused"),
				cmt("b.go", 12, "bug", "fresh-b12", "unused"),
				cmt("b.go", 2, "bug", "fresh-b2", "unused"),
			},
			wantNew:        []string{"b.go:2", "b.go:12", "m.go:4"},
			wantPersisting: []string{},
			wantResolved:   []string{"a.go:7", "a.go:7", "a.go:90", "z.go:3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compare(tt.before, tt.after, tt.reviewed)
			want := map[string][]string{
				"new":          tt.wantNew,
				"persisting":   tt.wantPersisting,
				"resolved":     tt.wantResolved,
				"not_reviewed": tt.wantNotReviewed,
			}
			have := map[string][]string{
				"new":          ids(got.New),
				"persisting":   ids(got.Persisting),
				"resolved":     ids(got.Resolved),
				"not_reviewed": ids(got.NotReviewed),
			}
			for _, bucket := range []string{"new", "persisting", "resolved", "not_reviewed"} {
				expected := want[bucket]
				if expected == nil {
					expected = []string{}
				}
				if !reflect.DeepEqual(have[bucket], expected) {
					t.Errorf("%s = %v, want %v", bucket, have[bucket], expected)
				}
			}
			for name, bucket := range map[string][]model.LlmComment{
				"new": got.New, "persisting": got.Persisting, "resolved": got.Resolved, "not_reviewed": got.NotReviewed,
			} {
				if bucket == nil {
					t.Errorf("%s bucket is nil, want an empty slice", name)
				}
			}
		})
	}
}

// TestCompare_SortedOrderIsStableAcrossInputOrder pins that shuffling the
// inputs cannot change the output, which is what keeps --json diffable.
func TestCompare_SortedOrderIsStableAcrossInputOrder(t *testing.T) {
	before := []model.LlmComment{
		cmt("z.go", 3, "bug", "s1", "one"),
		cmt("a.go", 90, "bug", "s2", "two"),
		cmt("a.go", 7, "bug", "s3", "three"),
	}
	shuffled := []model.LlmComment{before[1], before[2], before[0]}

	first := ids(Compare(before, nil, nil).Resolved)
	second := ids(Compare(shuffled, nil, nil).Resolved)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("order depends on input: %v vs %v", first, second)
	}
	if want := []string{"a.go:7", "a.go:90", "z.go:3"}; !reflect.DeepEqual(first, want) {
		t.Fatalf("resolved = %v, want %v", first, want)
	}
}

// TestCompare_SortBreaksTiesOnSnippet pins that two findings differing only in
// their snippet - and therefore living under different keys in the map the New
// bucket is drained from - come out in the same order every run.
func TestCompare_SortBreaksTiesOnSnippet(t *testing.T) {
	after := []model.LlmComment{
		cmt("a.go", 7, "bug", "zzz := 1", "same prose"),
		cmt("a.go", 7, "bug", "aaa := 1", "same prose"),
	}
	want := []string{"aaa := 1", "zzz := 1"}
	// Map iteration order is randomized per range statement, so a missing
	// tie-break shows up within a handful of repetitions.
	for i := 0; i < 50; i++ {
		var got []string
		for _, c := range Compare(nil, after, nil).New {
			got = append(got, c.ExistingCode)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: new = %v, want %v", i, got, want)
		}
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package ghpost

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/github"
	"github.com/alibaba/open-code-review/internal/model"
)

func TestClassifyLocationUsesIndependentDiffSides(t *testing.T) {
	files := []github.ChangedFile{{
		Filename: "main.go",
		Patch:    "@@ -8,3 +10,4 @@\n old-eight\n-old-nine\n+new-eleven\n shared-ten\n+new-thirteen",
	}}
	inventory := buildDiffInventory(files)
	tests := []struct {
		name string
		line int
		want locationClass
	}{
		{name: "left only", line: 8, want: locationLeftOnly},
		{name: "ambiguous numeric line", line: 10, want: locationAmbiguous},
		{name: "right only", line: 12, want: locationRightOnly},
		{name: "outside", line: 40, want: locationUnverified},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comment := model.LlmComment{Path: "main.go", StartLine: tc.line, EndLine: tc.line}
			if got := classifyLocation(comment, inventory); got != tc.want {
				t.Fatalf("classifyLocation(line %d) = %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}

func TestClassifyLocationRejectsInvalidAndIncompleteRanges(t *testing.T) {
	inventory := buildDiffInventory([]github.ChangedFile{
		{Filename: "complete.go", Patch: "@@ -1,1 +1,2 @@\n context\n+added"},
		{Filename: "truncated.go", Patch: "@@ -1,2 +1,2 @@\n context"},
		{Filename: "missing.go", Patch: ""},
	})
	tests := []struct {
		name    string
		comment model.LlmComment
		want    locationClass
	}{
		{name: "complete right only", comment: model.LlmComment{Path: "complete.go", StartLine: 2, EndLine: 2}, want: locationRightOnly},
		{name: "truncated", comment: model.LlmComment{Path: "truncated.go", StartLine: 1, EndLine: 1}, want: locationUnverified},
		{name: "missing patch", comment: model.LlmComment{Path: "missing.go", StartLine: 1, EndLine: 1}, want: locationUnverified},
		{name: "zero", comment: model.LlmComment{Path: "complete.go"}, want: locationInvalid},
		{name: "reversed", comment: model.LlmComment{Path: "complete.go", StartLine: 3, EndLine: 2}, want: locationInvalid},
		{name: "range crosses boundary", comment: model.LlmComment{Path: "complete.go", StartLine: 1, EndLine: 2}, want: locationAmbiguous},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyLocation(tc.comment, inventory); got != tc.want {
				t.Fatalf("classifyLocation(%+v) = %v, want %v", tc.comment, got, tc.want)
			}
		})
	}
}

func TestBuildDiffInventorySupportsMultipleHunksAndZeroCounts(t *testing.T) {
	inventory := buildDiffInventory([]github.ChangedFile{{
		Filename: "new.go",
		Patch:    "@@ -0,0 +1,2 @@\n+first\n+second\n@@ -20,2 +22,0 @@\n-old\n-lines",
	}})
	if got := classifyLocation(model.LlmComment{Path: "new.go", StartLine: 1, EndLine: 2}, inventory); got != locationRightOnly {
		t.Fatalf("new-file range = %v, want right only", got)
	}
	if got := classifyLocation(model.LlmComment{Path: "new.go", StartLine: 20, EndLine: 21}, inventory); got != locationLeftOnly {
		t.Fatalf("deleted range = %v, want left only", got)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestToSingleFileGroups(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "a.go"},
		{NewPath: "b.go"},
	}
	groups := toSingleFileGroups(diffs)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if groups[0].Label != "a.go" || groups[1].Label != "b.go" {
		t.Errorf("labels = [%q, %q]", groups[0].Label, groups[1].Label)
	}
}

func TestParseGroupingResponse_Valid(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "internal/auth/handler.go"},
		{NewPath: "internal/auth/handler_test.go"},
		{NewPath: "docs/README.md"},
	}
	content := `[
		{"label": "auth handler", "files": ["internal/auth/handler.go", "internal/auth/handler_test.go"]},
		{"label": "docs", "files": ["docs/README.md"]}
	]`
	groups, err := parseGroupingResponse(content, diffs)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if len(groups[0].Diffs) != 2 {
		t.Errorf("group 0 has %d diffs, want 2", len(groups[0].Diffs))
	}
	if groups[0].Label != "auth handler" {
		t.Errorf("group 0 label = %q", groups[0].Label)
	}
}

func TestParseGroupingResponse_MarkdownFenced(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "a.go"},
		{NewPath: "b.go"},
	}
	content := "```json\n" + `[{"label":"all","files":["a.go","b.go"]}]` + "\n```"
	groups, err := parseGroupingResponse(content, diffs)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
}

func TestParseGroupingResponse_DuplicateFile(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "a.go"},
	}
	content := `[{"label":"g1","files":["a.go"]},{"label":"g2","files":["a.go"]}]`
	groups, err := parseGroupingResponse(content, diffs)
	if err != nil {
		t.Fatal(err)
	}
	// Duplicate is skipped; file stays in first group only
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1 (duplicate skipped)", len(groups))
	}
	if len(groups[0].Diffs) != 1 || groups[0].Diffs[0].NewPath != "a.go" {
		t.Errorf("unexpected group content: %+v", groups[0])
	}
}

func TestParseGroupingResponse_MissingFile(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "a.go"},
		{NewPath: "b.go"},
	}
	content := `[{"label":"g1","files":["a.go"]}]`
	groups, err := parseGroupingResponse(content, diffs)
	if err != nil {
		t.Fatal(err)
	}
	// b.go not covered by LLM response, gets its own single-file group
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2 (one from LLM + one fallback)", len(groups))
	}
	if groups[1].Diffs[0].NewPath != "b.go" {
		t.Errorf("uncovered file group: got %q, want b.go", groups[1].Diffs[0].NewPath)
	}
}

func TestParseGroupingResponse_UnknownFile(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "a.go"},
	}
	content := `[{"label":"g1","files":["a.go","unknown.go"]}]`
	groups, err := parseGroupingResponse(content, diffs)
	if err != nil {
		t.Fatal(err)
	}
	// unknown.go is skipped; a.go still forms the group
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	if len(groups[0].Diffs) != 1 || groups[0].Diffs[0].NewPath != "a.go" {
		t.Errorf("unexpected group: %+v", groups[0])
	}
}

func TestParseGroupingResponse_InvalidJSON(t *testing.T) {
	diffs := []model.Diff{{NewPath: "a.go"}}
	_, err := parseGroupingResponse("not json", diffs)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestEnforceGroupTokenBudget_NoSplit(t *testing.T) {
	groups := []FileGroup{
		{Label: "small", Diffs: []model.Diff{{NewPath: "a.go", Diff: "short"}}},
	}
	result := enforceGroupTokenBudget(groups, 10000)
	if len(result) != 1 {
		t.Fatalf("got %d groups, want 1", len(result))
	}
}

func TestEnforceGroupTokenBudget_Split(t *testing.T) {
	largeDiff := make([]byte, 50000)
	for i := range largeDiff {
		largeDiff[i] = 'x'
	}
	groups := []FileGroup{
		{Label: "big", Diffs: []model.Diff{
			{NewPath: "a.go", Diff: string(largeDiff)},
			{NewPath: "b.go", Diff: string(largeDiff)},
		}},
	}
	result := enforceGroupTokenBudget(groups, 100)
	if len(result) != 2 {
		t.Fatalf("got %d groups, want 2 (split)", len(result))
	}
}

func TestBuildFileMetadataTable(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "a.go", IsNew: true, Insertions: 10, Deletions: 0},
		{NewPath: "b.go", IsDeleted: true, Insertions: 0, Deletions: 5},
	}
	table := buildFileMetadataTable(diffs)
	if !contains(table, "ADDED") || !contains(table, "DELETED") {
		t.Errorf("table missing status:\n%s", table)
	}
	if !contains(table, "a.go") || !contains(table, "b.go") {
		t.Errorf("table missing paths:\n%s", table)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

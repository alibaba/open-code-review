// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

type fakeGroupingClient struct {
	response string
	err      error
}

func (f *fakeGroupingClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	content := f.response
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{Role: "assistant", Content: &content},
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{TotalTokens: 100},
	}, nil
}

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

func TestEnforceMaxFilesPerGroup_NoSplit(t *testing.T) {
	groups := []FileGroup{
		{Label: "small", Diffs: []model.Diff{{NewPath: "a.go"}, {NewPath: "b.go"}}},
	}
	result := enforceMaxFilesPerGroup(groups)
	if len(result) != 1 {
		t.Fatalf("got %d groups, want 1", len(result))
	}
}

func TestEnforceMaxFilesPerGroup_Split(t *testing.T) {
	diffs := make([]model.Diff, 25)
	for i := range diffs {
		diffs[i] = model.Diff{NewPath: "file" + string(rune('a'+i)) + ".go"}
	}
	groups := []FileGroup{{Label: "big", Diffs: diffs}}
	result := enforceMaxFilesPerGroup(groups)
	if len(result) != 3 {
		t.Fatalf("got %d groups, want 3 (25 files / 10 max = 3 chunks)", len(result))
	}
	if len(result[0].Diffs) != 10 || len(result[1].Diffs) != 10 || len(result[2].Diffs) != 5 {
		t.Errorf("chunk sizes: %d, %d, %d", len(result[0].Diffs), len(result[1].Diffs), len(result[2].Diffs))
	}
}

func TestFileGroupKey_Single(t *testing.T) {
	key := fileGroupKey([]model.Diff{{NewPath: "a.go"}})
	if key != "a.go" {
		t.Errorf("got %q, want %q", key, "a.go")
	}
}

func TestFileGroupKey_Multiple(t *testing.T) {
	key := fileGroupKey([]model.Diff{{NewPath: "b.go"}, {NewPath: "a.go"}})
	if key != "a.go,b.go" {
		t.Errorf("got %q, want %q (sorted)", key, "a.go,b.go")
	}
}

func TestGroupDiffs_SingleFile(t *testing.T) {
	diffs := []model.Diff{{NewPath: "a.go"}}
	result := groupDiffs(nil, diffs, nil, "", template.Template{}, 0)
	if len(result.groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(result.groups))
	}
}

func TestGroupDiffs_NoGroupingTask(t *testing.T) {
	diffs := []model.Diff{{NewPath: "a.go"}, {NewPath: "b.go"}}
	result := groupDiffs(nil, diffs, nil, "", template.Template{}, 0)
	if len(result.groups) != 2 {
		t.Fatalf("got %d groups, want 2 (fallback to per-file)", len(result.groups))
	}
}

func TestGroupDiffs_LLMError_Fallback(t *testing.T) {
	diffs := []model.Diff{{NewPath: "a.go"}, {NewPath: "b.go"}}
	client := &fakeGroupingClient{err: fmt.Errorf("connection refused")}
	tpl := template.Template{
		GroupingTask: &template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "{{file_metadata_table}}"}},
		},
	}
	result := groupDiffs(context.Background(), diffs, client, "fake", tpl, 0)
	if len(result.groups) != 2 {
		t.Fatalf("got %d groups, want 2 (fallback on error)", len(result.groups))
	}
}

func TestGroupDiffs_LLMSuccess(t *testing.T) {
	diffs := []model.Diff{{NewPath: "a.go"}, {NewPath: "b.go"}, {NewPath: "c.go"}}
	client := &fakeGroupingClient{
		response: `[{"label":"ab","files":["a.go","b.go"]},{"label":"c","files":["c.go"]}]`,
	}
	tpl := template.Template{
		GroupingTask: &template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "{{file_metadata_table}}"}},
		},
	}
	result := groupDiffs(context.Background(), diffs, client, "fake", tpl, 0)
	if len(result.groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(result.groups))
	}
	if result.groups[0].Label != "ab" {
		t.Errorf("group 0 label = %q, want %q", result.groups[0].Label, "ab")
	}
	if len(result.groups[0].Diffs) != 2 {
		t.Errorf("group 0 has %d diffs, want 2", len(result.groups[0].Diffs))
	}
}

func TestCallGroupingLLM_EmptyResponse(t *testing.T) {
	diffs := []model.Diff{{NewPath: "a.go"}}
	client := &fakeGroupingClient{response: ""}
	task := &template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "{{file_metadata_table}}"}},
	}
	_, _, err := callGroupingLLM(context.Background(), diffs, client, "fake", task)
	if err == nil {
		t.Fatal("expected error for empty response")
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

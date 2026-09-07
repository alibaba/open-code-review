// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/stdout"
)

type fakeGroupingClient struct {
	response string
	err      error
	gotReq   llm.ChatRequest
	called   bool
}

func (f *fakeGroupingClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	f.gotReq = req
	f.called = true
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
	result := groupDiffs(nil, diffs, nil, "", template.Template{}, 0, nil)
	if len(result.groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(result.groups))
	}
}

func TestGroupDiffs_NoGroupingTask(t *testing.T) {
	diffs := []model.Diff{{NewPath: "a.go"}, {NewPath: "b.go"}}
	result := groupDiffs(nil, diffs, nil, "", template.Template{}, 0, nil)
	if len(result.groups) != 2 {
		t.Fatalf("got %d groups, want 2 (fallback to per-file)", len(result.groups))
	}
}

func TestGroupDiffs_LLMError_Fallback(t *testing.T) {
	diffs := []model.Diff{{NewPath: "a.go"}, {NewPath: "b.go"}}
	client := &fakeGroupingClient{err: fmt.Errorf("connection refused")}
	tpl := template.Template{
		GroupingTask: &template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "{{file_list}}"}},
		},
	}
	result := groupDiffs(context.Background(), diffs, client, "fake", tpl, 0, nil)
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
			Messages: []template.ChatMessage{{Role: "user", Content: "{{file_list}}"}},
		},
	}
	result := groupDiffs(context.Background(), diffs, client, "fake", tpl, 0, nil)
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

// groupingSkipTemplate is a template whose GROUPING_TASK is configured, so that
// a test asserting client.called == false proves the fast path skipped the call
// rather than the nil-task fallback having short-circuited it.
func groupingSkipTemplate(minFiles, bundleLines int) template.Template {
	return template.Template{
		GroupingMinFiles:            minFiles,
		GroupingBundleLineThreshold: bundleLines,
		GroupingTask: &template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "{{file_list}}"}},
		},
	}
}

func TestGroupDiffs_SmallLowChurnBundles(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "a.go", Insertions: 10, Deletions: 5, Diff: "diff a"},
		{NewPath: "b.go", Insertions: 8, Deletions: 2, Diff: "diff b"},
		{NewPath: "c.go", Insertions: 4, Deletions: 1, Diff: "diff c"},
	}
	client := &fakeGroupingClient{response: `[{"label":"x","files":["a.go"]}]`}
	result := groupDiffs(context.Background(), diffs, client, "fake", groupingSkipTemplate(4, 200), 0, nil)
	if client.called {
		t.Error("grouping LLM was called for a below-threshold change set")
	}
	if len(result.groups) != 1 {
		t.Fatalf("got %d groups, want 1 bundled group", len(result.groups))
	}
	if len(result.groups[0].Diffs) != 3 {
		t.Errorf("bundled group has %d diffs, want 3", len(result.groups[0].Diffs))
	}
	if result.groups[0].Label != smallChangeSetLabel {
		t.Errorf("label = %q, want %q", result.groups[0].Label, smallChangeSetLabel)
	}
	if result.usage != nil {
		t.Errorf("usage = %v, want nil (no LLM call was made)", result.usage)
	}
}

func TestGroupDiffs_SmallHighChurnPerFile(t *testing.T) {
	// Few enough files to skip the LLM, but too much churn for the three to
	// share one group's review rounds.
	diffs := []model.Diff{
		{NewPath: "a.go", Insertions: 100, Deletions: 50, Diff: "diff a"},
		{NewPath: "b.go", Insertions: 40, Deletions: 20, Diff: "diff b"},
		{NewPath: "c.go", Insertions: 10, Deletions: 5, Diff: "diff c"},
	}
	client := &fakeGroupingClient{response: `[{"label":"x","files":["a.go"]}]`}
	result := groupDiffs(context.Background(), diffs, client, "fake", groupingSkipTemplate(4, 200), 0, nil)
	if client.called {
		t.Error("grouping LLM was called for a below-threshold change set")
	}
	if len(result.groups) != 3 {
		t.Fatalf("got %d groups, want 3 (per-file)", len(result.groups))
	}
	for i, g := range result.groups {
		if len(g.Diffs) != 1 {
			t.Errorf("group %d has %d diffs, want 1", i, len(g.Diffs))
		}
	}
}

func TestGroupDiffs_BundleTokenBudgetSplit(t *testing.T) {
	// Low churn admits the bundle, but a tiny prompt limit makes it unusable, so
	// the token valve degrades it to the per-file shape.
	diffs := []model.Diff{
		{NewPath: "a.go", Insertions: 3, Deletions: 1, Diff: "some diff content for a"},
		{NewPath: "b.go", Insertions: 2, Deletions: 1, Diff: "some diff content for b"},
	}
	client := &fakeGroupingClient{response: `[{"label":"x","files":["a.go"]}]`}
	result := groupDiffs(context.Background(), diffs, client, "fake", groupingSkipTemplate(4, 200), 1, nil)
	if client.called {
		t.Error("grouping LLM was called for a below-threshold change set")
	}
	if len(result.groups) != 2 {
		t.Fatalf("got %d groups, want 2 (bundle split by token budget)", len(result.groups))
	}
	for i, g := range result.groups {
		if len(g.Diffs) != 1 {
			t.Errorf("group %d has %d diffs, want 1", i, len(g.Diffs))
		}
		if !strings.Contains(g.Label, "split:") {
			t.Errorf("group %d label = %q, want a split marker", i, g.Label)
		}
	}
}

func TestGroupDiffs_SkipLogReportsActualShape(t *testing.T) {
	// The log line names the partition that came out, so a bundle degraded by
	// the token valve must not still be announced as one group.
	diffs := []model.Diff{
		{NewPath: "a.go", Insertions: 3, Deletions: 1, Diff: "some diff content for a"},
		{NewPath: "b.go", Insertions: 2, Deletions: 1, Diff: "some diff content for b"},
	}
	tests := []struct {
		name       string
		tpl        template.Template
		tokenLimit int
		want       string
	}{
		{"bundle survives", groupingSkipTemplate(4, 200), 0, "reviewing as one group"},
		{"bundle split by token budget", groupingSkipTemplate(4, 200), 1, "reviewing per file"},
		{"bundle disabled", groupingSkipTemplate(4, 0), 0, "reviewing per file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			restore := stdout.Swap(&buf)
			groupDiffs(context.Background(), diffs, &fakeGroupingClient{}, "fake", tt.tpl, tt.tokenLimit, nil)
			restore()
			if got := buf.String(); !strings.Contains(got, tt.want) {
				t.Errorf("log = %q, want it to mention %q", got, tt.want)
			}
		})
	}
}

func TestGroupDiffs_AtFileThresholdCallsLLM(t *testing.T) {
	// Exactly at GroupingMinFiles: the LLM decides, and churn is not consulted.
	diffs := []model.Diff{
		{NewPath: "a.go", Insertions: 1},
		{NewPath: "b.go", Insertions: 1},
		{NewPath: "c.go", Insertions: 1},
		{NewPath: "d.go", Insertions: 1},
	}
	client := &fakeGroupingClient{
		response: `[{"label":"ab","files":["a.go","b.go"]},{"label":"cd","files":["c.go","d.go"]}]`,
	}
	result := groupDiffs(context.Background(), diffs, client, "fake", groupingSkipTemplate(4, 200), 0, nil)
	if !client.called {
		t.Fatal("grouping LLM was not called at the file threshold")
	}
	if len(result.groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(result.groups))
	}
}

func TestGroupDiffs_BundleDisabledFallsToPerFile(t *testing.T) {
	// GroupingBundleLineThreshold of 0 turns the second step off: small change
	// sets still skip the LLM, but never bundle.
	diffs := []model.Diff{
		{NewPath: "a.go", Insertions: 1, Diff: "diff a"},
		{NewPath: "b.go", Insertions: 1, Diff: "diff b"},
	}
	client := &fakeGroupingClient{response: `[{"label":"x","files":["a.go"]}]`}
	result := groupDiffs(context.Background(), diffs, client, "fake", groupingSkipTemplate(4, 0), 0, nil)
	if client.called {
		t.Error("grouping LLM was called for a below-threshold change set")
	}
	if len(result.groups) != 2 {
		t.Fatalf("got %d groups, want 2 (per-file)", len(result.groups))
	}
}

func TestGroupDiffs_SingleFileSkipsQuietly(t *testing.T) {
	// A single file reports the skip through telemetry but prints nothing: the
	// terminal reader gains nothing from being told that one file needs no
	// partition. The group keeps the file path as its label, not
	// smallChangeSetLabel — this path must not fall through to the bundle shape.
	diffs := []model.Diff{{NewPath: "a.go", Insertions: 3, Deletions: 1, Diff: "diff a"}}
	client := &fakeGroupingClient{response: `[{"label":"x","files":["a.go"]}]`}

	var buf bytes.Buffer
	restore := stdout.Swap(&buf)
	result := groupDiffs(context.Background(), diffs, client, "fake", groupingSkipTemplate(4, 200), 0, nil)
	restore()

	if client.called {
		t.Error("grouping LLM was called for a single-file change set")
	}
	if len(result.groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(result.groups))
	}
	if result.groups[0].Label != "a.go" {
		t.Errorf("label = %q, want the file path", result.groups[0].Label)
	}
	if got := buf.String(); got != "" {
		t.Errorf("single-file skip printed %q, want no output", got)
	}
}

func TestGroupDiffs_SingleFileSkipsWithGroupingDisabled(t *testing.T) {
	// GroupingMinFiles of 0 makes GroupingPlan return GroupingViaLLM, so only the
	// unconditional single-file short-circuit stops a pointless grouping call for
	// one file. Guards the ordering of that check against GroupingPlan.
	diffs := []model.Diff{{NewPath: "a.go", Insertions: 3, Diff: "diff a"}}
	client := &fakeGroupingClient{response: `[{"label":"x","files":["a.go"]}]`}
	result := groupDiffs(context.Background(), diffs, client, "fake", groupingSkipTemplate(0, 200), 0, nil)
	if client.called {
		t.Error("grouping LLM was called for a single file with GroupingMinFiles disabled")
	}
	if len(result.groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(result.groups))
	}
}

func TestGroupDiffs_BundleSplitByMaxFilesPerGroup(t *testing.T) {
	// GroupingMinFiles raised past maxFilesPerGroup, which is the case the bundle's
	// enforceMaxFilesPerGroup call exists to guard: the bundle is chopped into
	// 10-file chunks, so the shape is neither one group nor one per file.
	diffs := make([]model.Diff, 12)
	for i := range diffs {
		diffs[i] = model.Diff{NewPath: fmt.Sprintf("f%d.go", i), Insertions: 1, Diff: "d"}
	}
	client := &fakeGroupingClient{response: `[{"label":"x","files":["f0.go"]}]`}

	var buf bytes.Buffer
	restore := stdout.Swap(&buf)
	result := groupDiffs(context.Background(), diffs, client, "fake", groupingSkipTemplate(13, 200), 0, nil)
	restore()

	if client.called {
		t.Error("grouping LLM was called for a below-threshold change set")
	}
	if len(result.groups) != 2 {
		t.Fatalf("got %d groups, want 2 (10 + 2)", len(result.groups))
	}
	if !strings.Contains(buf.String(), "reviewing as 2 groups") {
		t.Errorf("log = %q, want it to name the 2-group shape", buf.String())
	}
}

func TestCallGroupingLLM_EmptyResponse(t *testing.T) {
	diffs := []model.Diff{{NewPath: "a.go"}}
	client := &fakeGroupingClient{response: ""}
	task := &template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "{{file_list}}"}},
	}
	_, _, err := callGroupingLLM(context.Background(), diffs, client, "fake", task, 4096, nil)
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

// TestCallGroupingLLM_UsesTemplateMaxTokens guards against reintroducing the
// hardcoded MaxTokens: 4096 this replaced: a small, task-specific cap left no
// room for a provider config that enables Anthropic extended thinking via
// extra_body.thinking with a larger budget_tokens, so the grouping call
// failed with "max_tokens must be greater than thinking.budget_tokens" even
// though the main review loop's own MAX_TOKENS was large enough.
func TestCallGroupingLLM_UsesTemplateMaxTokens(t *testing.T) {
	diffs := []model.Diff{{NewPath: "a.go"}}
	task := &template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "{{file_list}}"}},
	}

	client := &fakeGroupingClient{response: `[{"label":"a","files":["a.go"]}]`}
	if _, _, err := callGroupingLLM(context.Background(), diffs, client, "fake", task, 32000, nil); err != nil {
		t.Fatalf("callGroupingLLM: %v", err)
	}
	if client.gotReq.MaxTokens != 32000 {
		t.Errorf("MaxTokens = %d, want 32000 (the template's own limit)", client.gotReq.MaxTokens)
	}

	client = &fakeGroupingClient{response: `[{"label":"a","files":["a.go"]}]`}
	if _, _, err := callGroupingLLM(context.Background(), diffs, client, "fake", task, 0, nil); err != nil {
		t.Fatalf("callGroupingLLM: %v", err)
	}
	if client.gotReq.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want fallback 4096 when the template leaves MAX_TOKENS unset", client.gotReq.MaxTokens)
	}
}

func TestBuildFileMetadataTable(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "a.go", IsNew: true, Insertions: 10},
		{NewPath: "b.go", IsDeleted: true, Deletions: 5},
		{NewPath: "c.go", OldPath: "old_c.go", IsRenamed: true, Insertions: 2, Deletions: 1},
		{NewPath: "d.go", OldPath: "d.go", Insertions: 3, Deletions: 4},
	}
	// The grouping file list shares formatDiffEntry with the other-changed-files
	// block, so both prompts enumerate files the same way. Pin the exact shape,
	// including the per-entry trailing newline the grouping template relies on.
	want := "ADDED   a.go (+10/-0)\n" +
		"DELETED   b.go (+0/-5)\n" +
		"RENAMED   c.go (+2/-1)\n" +
		"MODIFIED   d.go (+3/-4)\n"
	if got := buildFileList(diffs); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveGroupSystemRule(t *testing.T) {
	const (
		goRule  = "GO CHECKLIST"
		xmlRule = "XML CHECKLIST"
	)
	// DefaultRule is deliberately empty so a path matching neither pattern
	// resolves to no rule at all, which is the skip case below.
	resolver := &rules.SystemRule{
		PathRules: []rules.PathRule{
			{Pattern: "*.go", Rule: goRule},
			{Pattern: "*.xml", Rule: xmlRule},
		},
	}
	diff := func(path string) model.Diff { return model.Diff{NewPath: path} }

	t.Run("nil resolver returns empty", func(t *testing.T) {
		a := New(Args{})
		if got := a.resolveGroupSystemRule([]model.Diff{diff("a.go")}); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("single file returns the bare rule", func(t *testing.T) {
		a := New(Args{SystemRule: resolver})
		if got := a.resolveGroupSystemRule([]model.Diff{diff("a.go")}); got != goRule {
			t.Errorf("got %q, want %q", got, goRule)
		}
	})

	// The untagged form is what every same-language group renders, so it must stay
	// byte-identical to the pre-tagging output — no wrapper, no duplicated rule.
	t.Run("one rule set across many files stays untagged", func(t *testing.T) {
		a := New(Args{SystemRule: resolver})
		got := a.resolveGroupSystemRule([]model.Diff{diff("b.go"), diff("a.go")})
		if got != goRule {
			t.Errorf("got %q, want the bare rule %q", got, goRule)
		}
	})

	t.Run("mixed rule sets are tagged with their files", func(t *testing.T) {
		a := New(Args{SystemRule: resolver})
		got := a.resolveGroupSystemRule([]model.Diff{diff("b.go"), diff("m.xml"), diff("a.go")})
		want := "<rules for=\"a.go, b.go\">\n" + goRule + "\n</rules>\n" +
			"<rules for=\"m.xml\">\n" + xmlRule + "\n</rules>"
		if got != want {
			t.Errorf("got:\n%s\n\nwant:\n%s", got, want)
		}
	})

	t.Run("output does not depend on input order", func(t *testing.T) {
		a := New(Args{SystemRule: resolver})
		forward := a.resolveGroupSystemRule([]model.Diff{diff("a.go"), diff("m.xml")})
		reverse := a.resolveGroupSystemRule([]model.Diff{diff("m.xml"), diff("a.go")})
		if forward != reverse {
			t.Errorf("input order changed the output:\n%s\n---\n%s", forward, reverse)
		}
	})

	t.Run("caller slice is not reordered", func(t *testing.T) {
		a := New(Args{SystemRule: resolver})
		in := []model.Diff{diff("z.go"), diff("a.go")}
		a.resolveGroupSystemRule(in)
		if in[0].NewPath != "z.go" || in[1].NewPath != "a.go" {
			t.Errorf("input slice was reordered: %q, %q", in[0].NewPath, in[1].NewPath)
		}
	})

	t.Run("files resolving to no rule are skipped", func(t *testing.T) {
		a := New(Args{SystemRule: resolver})
		got := a.resolveGroupSystemRule([]model.Diff{diff("a.go"), diff("README.md")})
		if got != goRule {
			t.Errorf("got %q, want the bare Go rule %q", got, goRule)
		}
	})

	t.Run("no file resolves to a rule", func(t *testing.T) {
		a := New(Args{SystemRule: resolver})
		if got := a.resolveGroupSystemRule([]model.Diff{diff("README.md")}); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	// Smoke test against the real shipped rule set rather than a hand-built one:
	// the glob patterns in system_rules.json must actually resolve for a plain
	// source path, which a synthetic resolver cannot show.
	t.Run("default rule set resolves a real path", func(t *testing.T) {
		real, err := rules.LoadDefault()
		if err != nil {
			t.Skipf("cannot load default rules: %v", err)
		}
		a := New(Args{SystemRule: real})
		if got := a.resolveGroupSystemRule([]model.Diff{diff("main.go")}); got == "" {
			t.Error("expected a non-empty rule for a .go file")
		}
	})

	// Regression test: resolveGroupSystemRule must pass the path through
	// verbatim, not lowercased. The sniffer-wrapped resolver does its own
	// internal lowercasing for glob matching, but also does real file I/O
	// (disk read or `git show`) to sniff .m content — lowercasing the path
	// before that call breaks the read for any mixed-case path (e.g.
	// ios/ViewController.m -> ios/viewcontroller.m doesn't exist), silently
	// falling back to the wrong rule instead of erroring loudly.
	t.Run("mixed-case .m path still sniffs as Objective-C", func(t *testing.T) {
		dir := t.TempDir()
		git := func(args ...string) {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		const objcHeader = "#import \"ViewController.h\"\n\n@implementation ViewController\n@end\n"
		full := filepath.Join(dir, "ios", "ViewController.m")
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(objcHeader), 0o644); err != nil {
			t.Fatal(err)
		}
		git("init")
		git("config", "user.email", "t@t.co")
		git("config", "user.name", "t")
		git("add", "-A")
		git("commit", "-m", "init")

		t.Setenv("HOME", t.TempDir())
		resolver, _, err := rules.NewResolver(dir, "", rules.ResolverOptions{})
		if err != nil {
			t.Fatalf("NewResolver: %v", err)
		}

		a := New(Args{SystemRule: resolver})
		got := a.resolveGroupSystemRule([]model.Diff{diff("ios/ViewController.m")})
		if strings.Contains(got, "Indexing, Shapes, and Implicit Expansion") {
			t.Errorf("resolved the MATLAB rule for a real ObjC file — mixed-case path broke the content sniff:\n%s", got)
		}
	})
}

func TestGroupChurn(t *testing.T) {
	tests := []struct {
		name    string
		group   FileGroup
		total   int64
		maxFile int64
	}{
		{
			name: "single file",
			group: FileGroup{Diffs: []model.Diff{
				{Insertions: 30, Deletions: 10},
			}},
			total: 40, maxFile: 40,
		},
		{
			name: "multiple files, max is first",
			group: FileGroup{Diffs: []model.Diff{
				{Insertions: 50, Deletions: 10},
				{Insertions: 20, Deletions: 5},
				{Insertions: 10, Deletions: 3},
			}},
			total: 98, maxFile: 60,
		},
		{
			name: "multiple files, max is last",
			group: FileGroup{Diffs: []model.Diff{
				{Insertions: 10, Deletions: 5},
				{Insertions: 20, Deletions: 10},
				{Insertions: 40, Deletions: 40},
			}},
			total: 125, maxFile: 80,
		},
		{
			name:    "empty group",
			group:   FileGroup{Diffs: nil},
			total:   0,
			maxFile: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, maxFile := diffsChurn(tt.group.Diffs)
			if total != tt.total {
				t.Errorf("total = %d, want %d", total, tt.total)
			}
			if maxFile != tt.maxFile {
				t.Errorf("maxFile = %d, want %d", maxFile, tt.maxFile)
			}
		})
	}
}

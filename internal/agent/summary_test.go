// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// summaryFakeClient returns a fixed response for every LLM call.
type summaryFakeClient struct {
	content string
	err     error
	calls   int
}

func (f *summaryFakeClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	content := f.content
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
		Model:   "fake",
		Usage:   &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5},
	}, nil
}

func newSummaryAgent(t *testing.T, tpl template.Template, client *summaryFakeClient) *Agent {
	t.Helper()
	a := New(Args{
		RepoDir:          t.TempDir(),
		Template:         tpl,
		LLMClient:        client,
		Model:            "fake",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		Session: session.New(t.TempDir(), "main", "fake", session.SessionOptions{
			ReviewMode: session.ReviewModeWorkspace,
			Operation:  session.OperationReview,
		}),
	})
	return a
}

func summaryTask(content string) *template.LlmConversation {
	return &template.LlmConversation{
		Messages: []template.ChatMessage{
			{Role: "system", Content: "You are a summary assistant."},
			{Role: "user", Content: content},
		},
	}
}

func baseSummaryTemplate() template.Template {
	return template.Template{
		MaxTokens:             4000,
		MaxToolRequestTimes:   10,
		MainTask:              template.LlmConversation{Messages: []template.ChatMessage{{Role: "system", Content: "x"}}},
		MemoryCompressionTask: template.LlmConversation{Messages: []template.ChatMessage{{Role: "system", Content: "x"}}},
	}
}

func TestMaybeRunChangeSummary_Success(t *testing.T) {
	client := &summaryFakeClient{content: "## Change Overview\n\nThis change adds a new feature."}
	tpl := baseSummaryTemplate()
	tpl.ChangeSummaryTask = summaryTask("{{diff_summary}}\n{{background}}\n{{commit_message}}")
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}
	a.args.Background = "add rate limiting"

	a.maybeRunChangeSummary(context.Background())

	if a.changeSummary == "" {
		t.Fatal("changeSummary should be set")
	}
	if !strings.Contains(a.changeSummary, "Change Overview") {
		t.Errorf("unexpected changeSummary: %q", a.changeSummary)
	}
	if client.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", client.calls)
	}
}

func TestMaybeRunChangeSummary_SkipWhenDisabled(t *testing.T) {
	client := &summaryFakeClient{content: "summary"}
	tpl := baseSummaryTemplate()
	tpl.ChangeSummaryTask = summaryTask("{{diff_summary}}")
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}
	a.args.SkipSummary = true

	a.maybeRunChangeSummary(context.Background())

	if a.changeSummary != "" {
		t.Error("changeSummary should be empty when SkipSummary is true")
	}
	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls, got %d", client.calls)
	}
}

func TestMaybeRunChangeSummary_SkipWhenNoTemplate(t *testing.T) {
	client := &summaryFakeClient{content: "summary"}
	tpl := baseSummaryTemplate()
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}

	a.maybeRunChangeSummary(context.Background())

	if a.changeSummary != "" {
		t.Error("changeSummary should be empty without template")
	}
	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls, got %d", client.calls)
	}
}

func TestMaybeRunChangeSummary_SkipWhenNoDiffs(t *testing.T) {
	client := &summaryFakeClient{content: "summary"}
	tpl := baseSummaryTemplate()
	tpl.ChangeSummaryTask = summaryTask("{{diff_summary}}")
	a := newSummaryAgent(t, tpl, client)

	a.maybeRunChangeSummary(context.Background())

	if a.changeSummary != "" {
		t.Error("changeSummary should be empty without diffs")
	}
	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls, got %d", client.calls)
	}
}

func TestMaybeRunChangeSummary_LLMError(t *testing.T) {
	client := &summaryFakeClient{err: errors.New("LLM unavailable")}
	tpl := baseSummaryTemplate()
	tpl.ChangeSummaryTask = summaryTask("{{diff_summary}}")
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}

	a.maybeRunChangeSummary(context.Background())

	if a.changeSummary != "" {
		t.Error("changeSummary should be empty on LLM error")
	}
	if client.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", client.calls)
	}
}

func TestMaybeRunChangeSummary_EmptyResponse(t *testing.T) {
	client := &summaryFakeClient{content: "   "}
	tpl := baseSummaryTemplate()
	tpl.ChangeSummaryTask = summaryTask("{{diff_summary}}")
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}

	a.maybeRunChangeSummary(context.Background())

	if a.changeSummary != "" {
		t.Error("changeSummary should be empty when LLM returns whitespace-only response")
	}
}

func TestMaybeRunImpactAnalysis_Success(t *testing.T) {
	client := &summaryFakeClient{content: "## Risk Assessment\n\nLow risk change."}
	tpl := baseSummaryTemplate()
	tpl.ImpactAnalysisTask = summaryTask("{{diff_summary}}\n{{all_comments}}\n{{change_summary}}")
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}
	a.changeSummary = "Previous summary"
	comments := []model.LlmComment{{Path: "a.go", Content: "issue found"}}

	a.maybeRunImpactAnalysis(context.Background(), comments)

	if a.impactAnalysis == "" {
		t.Fatal("impactAnalysis should be set")
	}
	if !strings.Contains(a.impactAnalysis, "Risk Assessment") {
		t.Errorf("unexpected impactAnalysis: %q", a.impactAnalysis)
	}
	if client.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", client.calls)
	}
}

func TestMaybeRunImpactAnalysis_SkipWhenDisabled(t *testing.T) {
	client := &summaryFakeClient{content: "analysis"}
	tpl := baseSummaryTemplate()
	tpl.ImpactAnalysisTask = summaryTask("{{diff_summary}}")
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}
	a.args.SkipSummary = true

	a.maybeRunImpactAnalysis(context.Background(), nil)

	if a.impactAnalysis != "" {
		t.Error("impactAnalysis should be empty when SkipSummary is true")
	}
	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls, got %d", client.calls)
	}
}

func TestMaybeRunImpactAnalysis_SkipWhenNoTemplate(t *testing.T) {
	client := &summaryFakeClient{content: "analysis"}
	tpl := baseSummaryTemplate()
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}

	a.maybeRunImpactAnalysis(context.Background(), nil)

	if a.impactAnalysis != "" {
		t.Error("impactAnalysis should be empty without template")
	}
	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls, got %d", client.calls)
	}
}

func TestMaybeRunImpactAnalysis_SkipWhenNoDiffs(t *testing.T) {
	client := &summaryFakeClient{content: "analysis"}
	tpl := baseSummaryTemplate()
	tpl.ImpactAnalysisTask = summaryTask("{{diff_summary}}")
	a := newSummaryAgent(t, tpl, client)

	a.maybeRunImpactAnalysis(context.Background(), nil)

	if a.impactAnalysis != "" {
		t.Error("impactAnalysis should be empty without diffs")
	}
	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls, got %d", client.calls)
	}
}

func TestMaybeRunImpactAnalysis_LLMError(t *testing.T) {
	client := &summaryFakeClient{err: errors.New("LLM unavailable")}
	tpl := baseSummaryTemplate()
	tpl.ImpactAnalysisTask = summaryTask("{{diff_summary}}")
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}

	a.maybeRunImpactAnalysis(context.Background(), []model.LlmComment{{Path: "a.go", Content: "issue"}})

	if a.impactAnalysis != "" {
		t.Error("impactAnalysis should be empty on LLM error")
	}
	if client.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", client.calls)
	}
}

func TestMaybeRunImpactAnalysis_EmptyResponse(t *testing.T) {
	client := &summaryFakeClient{content: "   "}
	tpl := baseSummaryTemplate()
	tpl.ImpactAnalysisTask = summaryTask("{{diff_summary}}")
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}

	a.maybeRunImpactAnalysis(context.Background(), nil)

	if a.impactAnalysis != "" {
		t.Error("impactAnalysis should be empty when LLM returns whitespace-only response")
	}
}

func TestMaybeRunFlowDiagram_Success(t *testing.T) {
	client := &summaryFakeClient{content: "```mermaid\nflowchart TD\n  A --> B\n```"}
	tpl := baseSummaryTemplate()
	tpl.FlowDiagramTask = summaryTask("{{diff_summary}}\n{{all_comments}}")
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}
	comments := []model.LlmComment{{Path: "a.go", Content: "issue found"}}

	a.maybeRunFlowDiagram(context.Background(), comments)

	if a.flowDiagram == "" {
		t.Fatal("flowDiagram should be set")
	}
	if !strings.Contains(a.flowDiagram, "mermaid") {
		t.Errorf("unexpected flowDiagram: %q", a.flowDiagram)
	}
	if client.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", client.calls)
	}
}

func TestMaybeRunFlowDiagram_SkipWhenDisabled(t *testing.T) {
	client := &summaryFakeClient{content: "diagram"}
	tpl := baseSummaryTemplate()
	tpl.FlowDiagramTask = summaryTask("{{diff_summary}}")
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}
	a.args.SkipSummary = true

	a.maybeRunFlowDiagram(context.Background(), nil)

	if a.flowDiagram != "" {
		t.Error("flowDiagram should be empty when SkipSummary is true")
	}
	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls, got %d", client.calls)
	}
}

func TestMaybeRunFlowDiagram_SkipWhenNoTemplate(t *testing.T) {
	client := &summaryFakeClient{content: "diagram"}
	tpl := baseSummaryTemplate()
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}

	a.maybeRunFlowDiagram(context.Background(), nil)

	if a.flowDiagram != "" {
		t.Error("flowDiagram should be empty without template")
	}
	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls, got %d", client.calls)
	}
}

func TestMaybeRunFlowDiagram_SkipWhenNoDiffs(t *testing.T) {
	client := &summaryFakeClient{content: "diagram"}
	tpl := baseSummaryTemplate()
	tpl.FlowDiagramTask = summaryTask("{{diff_summary}}")
	a := newSummaryAgent(t, tpl, client)

	a.maybeRunFlowDiagram(context.Background(), nil)

	if a.flowDiagram != "" {
		t.Error("flowDiagram should be empty without diffs")
	}
	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls, got %d", client.calls)
	}
}

func TestMaybeRunFlowDiagram_LLMError(t *testing.T) {
	client := &summaryFakeClient{err: errors.New("LLM unavailable")}
	tpl := baseSummaryTemplate()
	tpl.FlowDiagramTask = summaryTask("{{diff_summary}}")
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}

	a.maybeRunFlowDiagram(context.Background(), []model.LlmComment{{Path: "a.go", Content: "issue"}})

	if a.flowDiagram != "" {
		t.Error("flowDiagram should be empty on LLM error")
	}
	if client.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", client.calls)
	}
}

func TestMaybeRunFlowDiagram_EmptyResponse(t *testing.T) {
	client := &summaryFakeClient{content: "   "}
	tpl := baseSummaryTemplate()
	tpl.FlowDiagramTask = summaryTask("{{diff_summary}}")
	a := newSummaryAgent(t, tpl, client)
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: "+package main", Insertions: 1}}

	a.maybeRunFlowDiagram(context.Background(), nil)

	if a.flowDiagram != "" {
		t.Error("flowDiagram should be empty when LLM returns whitespace-only response")
	}
}

func TestBuildDiffSummary(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "a.go", Insertions: 10, Deletions: 2, IsNew: true},
		{NewPath: "b.go", Insertions: 5, Deletions: 8},
	}
	got := buildDiffSummary(diffs)
	if !strings.Contains(got, "2 file(s) changed") {
		t.Errorf("expected file count, got: %s", got)
	}
	if !strings.Contains(got, "ADDED") {
		t.Errorf("expected ADDED status, got: %s", got)
	}
	if !strings.Contains(got, "MODIFIED") {
		t.Errorf("expected MODIFIED status, got: %s", got)
	}
}

func TestBuildDiffSummary_Empty(t *testing.T) {
	got := buildDiffSummary(nil)
	if got != "(no files changed)" {
		t.Errorf("expected empty message, got: %s", got)
	}
}

func TestBuildReviewCommentsList(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "a.go", Content: "first issue"},
		{Path: "b.go", Content: "second issue"},
	}
	got := buildReviewCommentsList(comments)
	if !strings.Contains(got, "a.go") {
		t.Errorf("expected path a.go, got: %s", got)
	}
	if !strings.Contains(got, "first issue") {
		t.Errorf("expected content, got: %s", got)
	}
}

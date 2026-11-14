// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/alibaba/open-code-review/internal/workflow"
)

type sharedBudgetUsageClient struct {
	inner              groupedBudgetPartialClient
	completeFirstRound bool
}

func (client *sharedBudgetUsageClient) CompletionsWithCtx(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	response, err := client.inner.CompletionsWithCtx(ctx, request)
	if response != nil {
		response.Usage = &llm.UsageInfo{PromptTokens: 50, TotalTokens: 50}
		calls := response.ToolCalls()
		if client.completeFirstRound && len(calls) > 0 && calls[0].Function.Name == "code_comment" {
			response.Choices[0].Message.ToolCalls = append(calls, llm.ToolCall{ID: "done", Type: "function", Function: llm.FunctionCall{Name: "task_done", Arguments: "{}"}})
		}
	}
	return response, err
}

func newSharedBudgetAgent(test *testing.T, completeFirstRound bool) (*Agent, *workflow.Budget) {
	test.Helper()
	test.Setenv("HOME", test.TempDir())
	repo := test.TempDir()
	collector := tool.NewCommentCollector()
	registry := tool.NewRegistry()
	registry.Register(&tool.CodeCommentProvider{Collector: collector})
	budget := workflow.NewBudget(100, 1)
	history := session.New(repo, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeRange, DiffFrom: "main", DiffTo: "feature", Operation: session.OperationReview,
	})
	agent := New(Args{
		RepoDir: repo, From: "main", To: "feature", ReviewMode: session.ReviewModeRange,
		LLMClient: budget.Wrap(&sharedBudgetUsageClient{completeFirstRound: completeFirstRound}), Model: "fake", Session: history,
		CommentCollector: collector, Tools: registry, MaxConcurrency: 1, MaxTokensBudget: 0,
		Template: template.Template{
			MaxTokens: 100000, MaxToolRequestTimes: 5, MaxReviewRounds: 2,
			GroupingTask: &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "{{file_list}}"}}},
			MainTask:     template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}}},
		},
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "finish"}},
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment", Description: "comment"}},
		},
	})
	agent.diffs = []model.Diff{
		{OldPath: "a.go", NewPath: "a.go", Diff: "+a", Insertions: 1},
		{OldPath: "b.go", NewPath: "b.go", Diff: "+b", Insertions: 1},
	}
	agent.currentDate = "2026-11-15 00:00"
	return agent, budget
}

func TestSharedBudgetAfterCommentPreservesCoverage(test *testing.T) {
	agent, budget := newSharedBudgetAgent(test, false)
	comments, runErr := agent.dispatchSubtasks(test.Context())
	manifest := finishManifestFlow(test, agent)
	usage, exhausted := budget.Snapshot()
	hasBudgetWarning := false
	for _, warning := range agent.Warnings() {
		if warning.Type == "token_budget_reached" {
			hasBudgetWarning = true
		}
	}
	test.Logf("comments=%d error=%v state=%s completed=%d failed=%d shared_exhausted=%v usage=%+v agent_budget_exceeded=%v budget_warning=%v",
		len(comments), runErr, manifest.TerminalState, len(manifest.Coverage.Completed), len(manifest.Coverage.Failed), exhausted, usage, agent.BudgetExceeded(), hasBudgetWarning)
	if len(comments) != 1 || !exhausted {
		test.Fatal("fixture did not reach shared budget after producing a comment")
	}
	if manifest.TerminalState != session.StatePartial || len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 1 || !hasBudgetWarning {
		test.Fatal("shared budget must preserve partial coverage and surface a controlled budget stop")
	}
}

func TestSharedBudgetPreservesCompletedRoundWithoutCommentsOnEveryFile(test *testing.T) {
	agent, budget := newSharedBudgetAgent(test, true)
	comments, err := agent.dispatchSubtasks(test.Context())
	manifest := finishManifestFlow(test, agent)
	_, exhausted := budget.Snapshot()
	if err != nil || len(comments) != 1 || !exhausted || !agent.BudgetExceeded() {
		test.Fatalf("comments=%d error=%v exhausted=%v budget_exceeded=%v", len(comments), err, exhausted, agent.BudgetExceeded())
	}
	if manifest.TerminalState != session.StateComplete || len(manifest.Coverage.Completed) != 2 || len(manifest.Coverage.Failed) != 0 {
		test.Fatalf("completed first round lost coverage: %+v", manifest)
	}
	for _, warning := range agent.Warnings() {
		if warning.Type == "token_budget_reached" {
			return
		}
	}
	test.Fatal("missing budget warning")
}

package agent

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/tool"
)

// fakeBudgetAgentClient returns a task_done tool call on every request and
// reports a fixed token usage, so each file completes in exactly one round
// and consumes a predictable number of tokens. Used to drive the diff-path
// token-budget gate deterministically. Mirrors scan/budget_test.go's
// fakeBudgetClient.
type fakeBudgetAgentClient struct {
	perCallTokens int64
	calls         int64 // atomic
}

func (f *fakeBudgetAgentClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	atomic.AddInt64(&f.calls, 1)
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{{
					ID:       "1",
					Type:     "function",
					Function: llm.FunctionCall{Name: "task_done", Arguments: "{}"},
				}},
			},
			FinishReason: "tool_calls",
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{
			PromptTokens:     f.perCallTokens,
			CompletionTokens: 0,
			TotalTokens:      f.perCallTokens,
		},
	}, nil
}

// budgetAgentTestTemplate returns a minimal template sufficient to drive
// dispatchSubtasks without plan/dedup/summary phases.
func budgetAgentTestTemplate() template.Template {
	return template.Template{
		MaxTokens:           100000,
		MaxToolRequestTimes: 5,
		MainTask: template.LlmConversation{
			Messages: []template.ChatMessage{
				{Role: "system", Content: "review"},
				{Role: "user", Content: "review {{diff}} for {{current_file_path}}"},
			},
		},
	}
}

// makeBudgetDiffs returns n small, non-deleted diffs that survive filterDiffs
// and filterLargeDiffs.
func makeBudgetDiffs(n int) []model.Diff {
	diffs := make([]model.Diff, n)
	for i := range diffs {
		name := "f" + string(rune('0'+i)) + ".go"
		diffs[i] = model.Diff{
			NewPath:    name,
			OldPath:    name,
			Diff:       "+package x\n",
			Insertions: 1,
		}
	}
	return diffs
}

// TestDispatchSubtasks_TokenBudgetStopsDispatch verifies the per-file gate
// stops dispatch once the running token total + next-file look-ahead would
// blow the budget (INV-1), that a token_budget_reached warning is recorded,
// and that BudgetExceeded()==true with partial results returned (INV-3).
// Overrun is bounded by at most (concurrency) in-flight files — here 1.
func TestDispatchSubtasks_TokenBudgetStopsDispatch(t *testing.T) {
	const perCall = 50_000
	fake := &fakeBudgetAgentClient{perCallTokens: perCall}
	a := New(Args{
		LLMClient:        fake,
		Model:            "fake",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MaxConcurrency:   1, // serialize so the gate is deterministic
		MaxTokensBudget:  120_000,
		Template:         budgetAgentTestTemplate(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
		},
	})
	a.diffs = makeBudgetDiffs(10)
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}

	// Budget 120K, each file ~50K actual. The look-ahead adds a per-file
	// estimate so the gate should stop well before all 10 files run.
	calls := atomic.LoadInt64(&fake.calls)
	if calls == 0 {
		t.Fatal("expected at least one file to be dispatched")
	}
	if calls >= 10 {
		t.Errorf("budget gate did not stop dispatch: all %d files ran (budget should have cut it short)", calls)
	}

	// A token_budget_reached warning must be recorded.
	var found bool
	for _, w := range a.Warnings() {
		if w.Type == "token_budget_reached" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a token_budget_reached warning")
	}

	// Budget exhaustion must signal out-of-band, not via an error (INV-3).
	if !a.BudgetExceeded() {
		t.Error("expected BudgetExceeded()==true after token budget trip")
	}

	// Partial comments returned as a non-nil slice (INV-3 edge — partial
	// findings). Even when empty it must not be nil-with-error.
	if comments == nil {
		t.Error("expected partial comments slice (non-nil), got nil")
	}
}

// TestDispatchSubtasks_UnlimitedBudget verifies MaxTokensBudget=0 runs every
// file (default behavior unchanged — regression guard).
func TestDispatchSubtasks_UnlimitedBudget(t *testing.T) {
	fake := &fakeBudgetAgentClient{perCallTokens: 50_000}
	a := New(Args{
		LLMClient:        fake,
		Model:            "fake",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MaxConcurrency:   1,
		MaxTokensBudget:  0, // unlimited
		Template:         budgetAgentTestTemplate(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
		},
	})
	a.diffs = makeBudgetDiffs(5)
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	if calls := atomic.LoadInt64(&fake.calls); calls != 5 {
		t.Errorf("unlimited budget should run all 5 files, ran %d", calls)
	}
	if a.BudgetExceeded() {
		t.Error("unlimited budget must not set BudgetExceeded")
	}
}

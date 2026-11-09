// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/estimate"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// fakeBudgetClient returns a task_done tool call on every request and
// reports a fixed token usage, so each file completes in exactly one round
// and consumes a predictable number of tokens. Used to drive the budget
// gate deterministically.
type fakeBudgetClient struct {
	perCallTokens int64
	calls         int64 // atomic
}

func (f *fakeBudgetClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
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
		Usage: &llm.UsageInfo{
			PromptTokens:     f.perCallTokens,
			CompletionTokens: 0,
			TotalTokens:      f.perCallTokens,
		},
	}, nil
}

func budgetTestTemplate() template.ScanTemplate {
	return template.ScanTemplate{
		MaxTokens:           100000,
		MaxToolRequestTimes: 5,
		MainTask: template.LlmConversation{
			Messages: []template.ChatMessage{
				{Role: "system", Content: "scan"},
				{Role: "user", Content: "review {{file_content}}"},
			},
		},
	}
}

func makeScanItems(n int) []model.ScanItem {
	items := make([]model.ScanItem, n)
	for i := range items {
		items[i] = model.ScanItem{
			Path:      "f" + string(rune('0'+i)) + ".go",
			Content:   "package x\n",
			LineCount: 1,
		}
	}
	return items
}

func TestBudgetGate_CustomEstimation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params estimate.Parameters
		calls  int64
	}{
		{"defaults", estimate.Parameters{}, 1},
		{"overhead override", estimate.Parameters{PromptOverheadTokens: 8000}, 0},
		{"output override", estimate.Parameters{OutputTokensPerRound: 3000}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeBudgetClient{perCallTokens: 10}
			a := NewAgent(Args{
				Template: budgetTestTemplate(), LLMClient: fake, MaxConcurrency: 1,
				MaxTokensBudget: 30_000, Estimation: tc.params,
				Session:  session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: session.ReviewModeFullScan}),
				SkipPlan: true, SkipDedup: true, SkipSummary: true,
			})
			t.Cleanup(func() { _ = a.Session().Finalize() })
			a.items = makeScanItems(1)
			a.args.Tools.Freeze()
			if _, err := a.dispatchSubtasks(context.Background()); err != nil {
				t.Fatal(err)
			}
			if calls := atomic.LoadInt64(&fake.calls); calls != tc.calls {
				t.Fatalf("LLM calls = %d, want %d", calls, tc.calls)
			}
			if a.BudgetExceeded() != (tc.calls == 0) {
				t.Fatalf("BudgetExceeded = %v, expected stopped=%v", a.BudgetExceeded(), tc.calls == 0)
			}
		})
	}
}

// TestBudgetGate_StopsBeforeExceeding verifies the per-file gate stops
// dispatch once the running token total + next-file look-ahead would blow
// the budget — overrun is bounded by at most (concurrency) in-flight files,
// not a whole batch.
func TestBudgetGate_StopsBeforeExceeding(t *testing.T) {
	const perCall = 50_000
	fake := &fakeBudgetClient{perCallTokens: perCall}

	a := NewAgent(Args{
		Template:         budgetTestTemplate(),
		LLMClient:        fake,
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MaxConcurrency:   1, // serialize so the gate is deterministic
		MaxTokensBudget:  120_000,
		Session:          session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: session.ReviewModeFullScan}),
		SkipPlan:         true,
		SkipDedup:        true,
		SkipSummary:      true,
	})
	a.items = makeScanItems(10)
	a.args.Tools.Freeze()

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}

	// Budget 120K, each file ~50K actual. estimateFileTokens for a 1-line
	// file is dominated by promptOverhead×rounds, so the look-ahead will be
	// large; the gate should stop well before all 10 files run.
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
}

// TestBudgetGate_Unlimited verifies MaxTokensBudget=0 runs every file.
func TestBudgetGate_Unlimited(t *testing.T) {
	fake := &fakeBudgetClient{perCallTokens: 50_000}
	a := NewAgent(Args{
		Template:         budgetTestTemplate(),
		LLMClient:        fake,
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MaxConcurrency:   1,
		MaxTokensBudget:  0, // unlimited
		Session:          session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: session.ReviewModeFullScan}),
		SkipPlan:         true,
		SkipDedup:        true,
		SkipSummary:      true,
	})
	a.items = makeScanItems(5)
	a.args.Tools.Freeze()

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	if calls := atomic.LoadInt64(&fake.calls); calls != 5 {
		t.Errorf("unlimited budget should run all 5 files, ran %d", calls)
	}
}

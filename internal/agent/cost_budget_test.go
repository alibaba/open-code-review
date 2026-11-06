// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// onePerMillion bills every token kind at $1 per million, so N tokens cost N/1e6 USD.
var onePerMillion = &llm.Price{Input: 1, CacheRead: 1, CacheWrite: 1, Output: 1}

func countWarnings(a *Agent, warningType string) int {
	n := 0
	for _, w := range a.Warnings() {
		if w.Type == warningType {
			n++
		}
	}
	return n
}

func TestDispatchSubtasks_CostBudgetStopsDispatch(t *testing.T) {
	setTestHome(t, t.TempDir())
	fake := &fakeBudgetAgentClient{perCallTokens: 50_000} // $0.05 per file
	a := New(Args{
		LLMClient:        fake,
		Model:            "fake",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MaxConcurrency:   1,
		MaxCostBudget:    0.12,
		Price:            onePerMillion,
		Template:         budgetAgentTestTemplate(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
		},
	})
	t.Cleanup(func() { _ = a.Session().Finalize() })
	a.diffs = makeBudgetDiffs(10)
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	// $0.00, $0.05 and $0.10 are within the $0.12 budget; $0.15 is not.
	if calls := atomic.LoadInt64(&fake.calls); calls != 3 {
		t.Fatalf("LLM calls = %d, want 3", calls)
	}
	if !a.BudgetExceeded() || !a.CostBudgetExceeded() {
		t.Fatalf("BudgetExceeded=%v CostBudgetExceeded=%v, want both true", a.BudgetExceeded(), a.CostBudgetExceeded())
	}
	if n := countWarnings(a, "cost_budget_reached"); n != 1 {
		t.Errorf("cost_budget_reached warnings = %d, want 1", n)
	}
	if n := countWarnings(a, "token_budget_reached"); n != 0 {
		t.Errorf("token_budget_reached warnings = %d, want 0", n)
	}
	if cost, ok := a.TotalCost(); !ok || cost < 0.149 || cost > 0.151 {
		t.Errorf("TotalCost = %v, %v; want 0.15, true", cost, ok)
	}

	if err := a.finalizeManifest(); err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	m := a.RunManifest()
	if m == nil || m.TerminalState != session.StatePartial || m.RunFailure != nil {
		t.Fatalf("manifest = %+v, want partial without run_failure", m)
	}
	if len(m.Coverage.Completed) != 3 || len(m.Coverage.Failed) != 7 {
		t.Fatalf("coverage completed=%d failed=%d, want 3 and 7", len(m.Coverage.Completed), len(m.Coverage.Failed))
	}
	for _, item := range m.Coverage.Failed {
		if item.Classification != session.FailureBudget {
			t.Fatalf("%s classification = %q, want budget", item.Path, item.Classification)
		}
	}
}

func TestDispatchSubtasks_CostBudgetStopsInFlightGroup(t *testing.T) {
	setTestHome(t, t.TempDir())
	// The first group completes for 10 tokens. The second spends $0.60 a round,
	// so its third round is refused against the $1.00 budget and it gets the
	// grace round.
	fake := &fakeFirstDoneThenNeverClient{perCallTokens: 600_000}
	a := New(Args{
		LLMClient:        fake,
		Model:            "fake",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MaxConcurrency:   1,
		MaxCostBudget:    1,
		Price:            onePerMillion,
		Template:         budgetAgentTestTemplate(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
		},
	})
	t.Cleanup(func() { _ = a.Session().Finalize() })
	a.diffs = makeBudgetDiffs(2)
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	if calls := atomic.LoadInt64(&fake.calls); calls != 4 {
		t.Fatalf("LLM calls = %d, want 4 (1 done + 2 rounds + grace)", calls)
	}
	if !a.CostBudgetExceeded() || !a.BudgetExceeded() {
		t.Fatal("an in-flight cost stop must set CostBudgetExceeded and BudgetExceeded")
	}
	if n := countWarnings(a, "cost_budget_reached"); n != 1 {
		t.Errorf("cost_budget_reached warnings = %d, want 1", n)
	}
	if err := a.finalizeManifest(); err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	m := a.RunManifest()
	if m == nil || len(m.Coverage.Completed) != 1 || len(m.Coverage.Failed) != 1 ||
		m.Coverage.Failed[0].Classification != session.FailureBudget {
		t.Fatalf("coverage = %+v, want one completed and one failed(budget)", m)
	}
}

func TestDispatchSubtasks_CostBudgetSkipsNextRound(t *testing.T) {
	setTestHome(t, t.TempDir())
	diffs := makeBudgetDiffs(1)
	fake := &fakeCommentAndDoneClient{perCallTokens: 2_000_000, path: diffs[0].NewPath} // $2.00
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	tpl := budgetAgentTestTemplate()
	tpl.MaxReviewRounds = 2
	a := New(Args{
		LLMClient:        fake,
		Model:            "fake",
		CommentCollector: collector,
		Tools:            reg,
		MaxConcurrency:   1,
		MaxCostBudget:    1,
		Price:            onePerMillion,
		SkipFilter:       true,
		Template:         tpl,
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment", Description: "comment"}},
		},
	})
	t.Cleanup(func() { _ = a.Session().Finalize() })
	a.diffs = diffs
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	if calls := atomic.LoadInt64(&fake.calls); calls != 1 {
		t.Fatalf("LLM calls = %d, want 1: round 2 must be skipped", calls)
	}
	if len(comments) != 1 || !a.CostBudgetExceeded() {
		t.Fatalf("comments=%d CostBudgetExceeded=%v, want the round 1 finding and a cost stop", len(comments), a.CostBudgetExceeded())
	}
}

// A price without a budget only reports the cost: nothing is stopped.
func TestDispatchSubtasks_PriceWithoutCostBudgetNeverStops(t *testing.T) {
	setTestHome(t, t.TempDir())
	fake := &fakeBudgetAgentClient{perCallTokens: 5_000_000}
	a := New(Args{
		LLMClient:        fake,
		Model:            "fake",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MaxConcurrency:   1,
		Price:            onePerMillion,
		Template:         budgetAgentTestTemplate(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
		},
	})
	t.Cleanup(func() { _ = a.Session().Finalize() })
	a.diffs = makeBudgetDiffs(3)
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	if calls := atomic.LoadInt64(&fake.calls); calls != 3 || a.BudgetExceeded() {
		t.Fatalf("calls=%d BudgetExceeded=%v, want 3 and false", calls, a.BudgetExceeded())
	}
	if cost, ok := a.TotalCost(); !ok || cost != 15 {
		t.Errorf("TotalCost = %v, %v; want 15, true", cost, ok)
	}
}

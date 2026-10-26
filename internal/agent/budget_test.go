// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
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
				{Role: "user", Content: "review {{diffs}}"},
			},
		},
	}
}

// makeBudgetDiffs returns n small, non-deleted diffs that survive every
// selection gate.
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
// blow the budget, that a token_budget_reached warning is recorded, and that
// BudgetExceeded()==true with partial results returned.
// Overrun is bounded by at most (concurrency) in-flight files — here 1.
func TestDispatchSubtasks_TokenBudgetStopsDispatch(t *testing.T) {
	setTestHome(t, t.TempDir())
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
	t.Cleanup(func() { _ = a.Session().Finalize() })
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

	// Budget exhaustion must signal out-of-band, not via an error.
	if !a.BudgetExceeded() {
		t.Error("expected BudgetExceeded()==true after token budget trip")
	}

	// Partial comments are returned as a non-nil slice. Even when empty, the
	// result must not be represented as nil-with-error.
	if comments == nil {
		t.Error("expected partial comments slice (non-nil), got nil")
	}

	if err := a.finalizeManifest(); err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	t.Cleanup(func() { _ = a.session.Finalize() })
	manifest := a.RunManifest()
	// A budget stop that still covered files is a controlled coverage truncation:
	// coverage alone derives the terminal state, so it reads partial (not failed)
	// and `ocr review` exits 0.
	if manifest == nil || manifest.TerminalState != session.StatePartial {
		t.Fatalf("manifest terminal = %v, want partial", manifest)
	}
	// Crucially it must NOT claim the single run_failure slot: that would force the
	// run to failed regardless of coverage, and would block a genuine run-level
	// cause raised later from being recorded at all.
	if manifest.RunFailure != nil {
		t.Fatalf("run_failure = %+v, want nil for a controlled budget stop", manifest.RunFailure)
	}
	if got, want := len(manifest.Coverage.Completed), int(calls); got != want {
		t.Fatalf("completed coverage = %d, want %d", got, want)
	}
	if got, want := len(manifest.Coverage.Failed), len(a.diffs)-int(calls); got != want {
		t.Fatalf("failed coverage = %d, want %d", got, want)
	}
	for _, item := range manifest.Coverage.Failed {
		if item.Classification != session.FailureBudget {
			t.Fatalf("undispatched item %s classification = %q, want budget", item.Path, item.Classification)
		}
	}
}

// TestDispatchSubtasks_TokenBudgetBeforeFirstFileIsFailed pins the other side of
// the exit-code boundary: when the budget is too small to admit even the first
// file, nothing is covered, every selected item is swept to failed(budget), and
// the manifest reads failed so `ocr review` exits non-zero. One covered file is
// the entire difference between this and the partial/exit-0 case above — the
// boundary is deliberate, so it gets its own guard.
func TestDispatchSubtasks_TokenBudgetBeforeFirstFileIsFailed(t *testing.T) {
	setTestHome(t, t.TempDir())
	fake := &fakeBudgetAgentClient{perCallTokens: 50_000}
	a := New(Args{
		LLMClient:        fake,
		Model:            "fake",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MaxConcurrency:   1,
		MaxTokensBudget:  1, // smaller than any single file's look-ahead estimate
		Template:         budgetAgentTestTemplate(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
		},
	})
	t.Cleanup(func() { _ = a.Session().Finalize() })
	a.diffs = makeBudgetDiffs(3)
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatchSubtasks must not return an error for a controlled stop: %v", err)
	}
	if comments == nil {
		t.Error("expected a non-nil (empty) comments slice, got nil")
	}
	if got := atomic.LoadInt64(&fake.calls); got != 0 {
		t.Fatalf("expected zero LLM calls, got %d", got)
	}
	if !a.BudgetExceeded() {
		t.Error("expected BudgetExceeded()==true")
	}

	if err := a.finalizeManifest(); err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	t.Cleanup(func() { _ = a.session.Finalize() })
	manifest := a.RunManifest()
	if manifest == nil || manifest.TerminalState != session.StateFailed {
		t.Fatalf("manifest terminal = %v, want failed", manifest)
	}
	// Still no run_failure: the exit code comes from coverage (all selected items
	// failed), not from a run-level failure classification.
	if manifest.RunFailure != nil {
		t.Fatalf("run_failure = %+v, want nil", manifest.RunFailure)
	}
	if len(manifest.Coverage.Completed) != 0 {
		t.Fatalf("completed coverage = %d, want 0", len(manifest.Coverage.Completed))
	}
	if got, want := len(manifest.Coverage.Failed), len(a.diffs); got != want {
		t.Fatalf("failed coverage = %d, want %d", got, want)
	}
	for _, item := range manifest.Coverage.Failed {
		if item.Classification != session.FailureBudget {
			t.Fatalf("item %s classification = %q, want budget", item.Path, item.Classification)
		}
		if item.Reason == "" {
			t.Fatalf("item %s must carry the truncation reason", item.Path)
		}
	}
}

// TestDispatchSubtasks_UnlimitedBudget verifies MaxTokensBudget=0 runs every
// file (default behavior unchanged — regression guard).
func TestDispatchSubtasks_UnlimitedBudget(t *testing.T) {
	setTestHome(t, t.TempDir())
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
	t.Cleanup(func() { _ = a.Session().Finalize() })
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

// fakeFirstDoneThenNeverClient completes the first conversation it sees with
// task_done at negligible cost, then never calls task_done again: every later
// response is an empty assistant turn that still reports perCallTokens of
// usage. It drives the in-conversation budget stop against a run that also has
// one healthy, completed group, so the result is a partial run rather than an
// all-failed one.
type fakeFirstDoneThenNeverClient struct {
	perCallTokens int64
	calls         int64 // atomic
}

func (f *fakeFirstDoneThenNeverClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	n := atomic.AddInt64(&f.calls, 1)
	if n == 1 {
		return &llm.ChatResponse{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{
					ID: "1", Type: "function", Function: llm.FunctionCall{Name: "task_done", Arguments: "{}"},
				}}},
				FinishReason: "tool_calls",
			}},
			Model: "fake",
			Usage: &llm.UsageInfo{PromptTokens: 10, TotalTokens: 10},
		}, nil
	}
	content := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Role: "assistant", Content: &content}}},
		Model:   "fake",
		Usage:   &llm.UsageInfo{PromptTokens: f.perCallTokens, TotalTokens: f.perCallTokens},
	}, nil
}

// TestDispatchSubtasks_TokenBudgetStopsInFlightGroup pins the gap the dispatch
// look-ahead cannot close: a group admitted while usage is still low can outgrow
// the budget on its own, and only an in-conversation check can stop it. The
// stopped group must read as failed(budget) while the completed one stays
// completed, BudgetExceeded() must be true, the warning must be recorded once,
// and the run must be a controlled partial stop rather than an error.
func TestDispatchSubtasks_TokenBudgetStopsInFlightGroup(t *testing.T) {
	setTestHome(t, t.TempDir())
	diffs := makeBudgetDiffs(2)
	// The gate admits both groups (estimate <= budget). The second group's
	// second round pushes usage past the budget, so its third round is refused.
	budget := estimateDiffFileTokens(diffs[1]) * 4
	perCall := budget/2 + 1
	fake := &fakeFirstDoneThenNeverClient{perCallTokens: perCall}
	a := New(Args{
		LLMClient:        fake,
		Model:            "fake",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MaxConcurrency:   1, // serialize so the call sequence is deterministic
		MaxTokensBudget:  budget,
		Template:         budgetAgentTestTemplate(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
		},
	})
	t.Cleanup(func() { _ = a.Session().Finalize() })
	a.diffs = diffs
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatchSubtasks must not return an error for a controlled stop: %v", err)
	}
	if comments == nil {
		t.Error("expected a non-nil (empty) comments slice, got nil")
	}

	// One call for the completed group, then two admitted rounds and one grace
	// round for the stopped group. Without the in-loop check the template's
	// five rounds would all run.
	if calls := atomic.LoadInt64(&fake.calls); calls != 4 {
		t.Fatalf("LLM calls = %d, want 4 (1 done + 2 rounds + grace)", calls)
	}
	if !a.BudgetExceeded() {
		t.Error("expected BudgetExceeded()==true after an in-flight budget stop")
	}
	var warnings int
	for _, w := range a.Warnings() {
		if w.Type == "token_budget_reached" {
			warnings++
		}
	}
	if warnings != 1 {
		t.Errorf("token_budget_reached warnings = %d, want 1", warnings)
	}

	if err := a.finalizeManifest(); err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	manifest := a.RunManifest()
	if manifest == nil || manifest.TerminalState != session.StatePartial {
		t.Fatalf("manifest terminal = %v, want partial", manifest)
	}
	if manifest.RunFailure != nil {
		t.Fatalf("run_failure = %+v, want nil for a controlled budget stop", manifest.RunFailure)
	}
	if got := len(manifest.Coverage.Completed); got != 1 {
		t.Fatalf("completed coverage = %d, want 1", got)
	}
	if got := len(manifest.Coverage.Failed); got != 1 {
		t.Fatalf("failed coverage = %d, want 1", got)
	}
	if got := manifest.Coverage.Failed[0].Classification; got != session.FailureBudget {
		t.Fatalf("stopped group classification = %q, want budget", got)
	}
}

// fakeCommentAndDoneClient answers the first request with a code_comment and a
// task_done in the same turn, reporting perCallTokens of usage. Its round
// therefore completes with a new finding, which is what lets the round loop
// reach round 2 at all, while leaving the run over budget.
type fakeCommentAndDoneClient struct {
	perCallTokens int64
	path          string
	calls         int64 // atomic
}

func (f *fakeCommentAndDoneClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	atomic.AddInt64(&f.calls, 1)
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
				{ID: "1", Type: "function", Function: llm.FunctionCall{
					Name:      "code_comment",
					Arguments: `{"comments":[{"path":"` + f.path + `","content":"missing a nil check here"}]}`,
				}},
				{ID: "2", Type: "function", Function: llm.FunctionCall{Name: "task_done", Arguments: "{}"}},
			}},
			FinishReason: "tool_calls",
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: f.perCallTokens, TotalTokens: f.perCallTokens},
	}, nil
}

// TestDispatchSubtasks_TokenBudgetSkipsNextRoundAfterCompletedRound covers the
// round gate rather than the in-conversation check: a group whose round 1 ends
// in task_done already over budget must not start round 2, must not get a grace
// round (it has nothing unreported), and must stay completed, while the run
// still reports that the budget was exceeded.
func TestDispatchSubtasks_TokenBudgetSkipsNextRoundAfterCompletedRound(t *testing.T) {
	setTestHome(t, t.TempDir())
	diffs := makeBudgetDiffs(1)
	budget := estimateDiffFileTokens(diffs[0]) * 4
	fake := &fakeCommentAndDoneClient{perCallTokens: budget + 1, path: diffs[0].NewPath}
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
		MaxTokensBudget:  budget,
		SkipFilter:       true, // the filter is an LLM call of its own
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
		t.Fatalf("LLM calls = %d, want 1: round 2 and any grace round must be skipped", calls)
	}
	if len(comments) != 1 {
		t.Errorf("comments = %d, want the round 1 finding", len(comments))
	}
	if !a.BudgetExceeded() {
		t.Error("expected BudgetExceeded()==true when the round gate skips a round")
	}

	if err := a.finalizeManifest(); err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	manifest := a.RunManifest()
	if manifest == nil || len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 0 {
		t.Fatalf("coverage = %+v, want the group completed", manifest)
	}
}

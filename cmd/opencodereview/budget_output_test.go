// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

// TestEmitRunResult_JSONBudgetStopIsPartial pins the unified terminal-state
// contract for a controlled budget stop that still covered something: the
// top-level status comes solely from the manifest's coverage-derived
// terminal_state ("partial"), and budgetExceeded never overrides it. The budget
// reason stays observable through three independent outlets — summary.
// budget_exceeded, the token_budget_reached warning, and the failed items'
// classification — so nothing is lost by dropping the typed status.
func TestEmitRunResult_JSONBudgetStopIsPartial(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed:  3,
		inputTokens:    100,
		outputTokens:   50,
		totalTokens:    150,
		warnings:       []agent.AgentWarning{{Type: "token_budget_reached", File: "big.go", Message: "stopped"}},
		toolCalls:      map[string]int64{"file_read": 2},
		budgetExceeded: true,
		manifest: &session.RunManifest{
			TerminalState: session.StatePartial,
			Coverage: session.Coverage{
				Selected:  []session.CoverageItem{{ItemID: "a"}, {ItemID: "b"}},
				Completed: []session.CoverageItem{{ItemID: "a"}},
				Failed: []session.CoverageItem{{
					ItemID:         "b",
					Classification: session.FailureBudget,
					Reason:         "aggregate token budget reached before dispatch completed",
				}},
			},
		},
	}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != string(session.StatePartial) {
		t.Errorf("status = %q, want %q (must mirror manifest.terminal_state, not a typed budget status)",
			out.Status, session.StatePartial)
	}
	if out.Summary == nil || !out.Summary.BudgetExceeded {
		t.Errorf("summary.budget_exceeded = %v, want true", out.Summary)
	}
	if out.Manifest == nil {
		t.Fatal("manifest must be published")
	}
	if out.Manifest.RunFailure != nil {
		t.Errorf("a controlled budget stop must record no run_failure, got %+v", out.Manifest.RunFailure)
	}
	if len(out.Manifest.Coverage.Failed) != 1 || out.Manifest.Coverage.Failed[0].Classification != session.FailureBudget {
		t.Errorf("coverage.failed = %+v, want one item classified budget", out.Manifest.Coverage.Failed)
	}
	// The token_budget_reached warning must still be present in the output so
	// the reason is observable alongside the coverage-derived status.
	var foundBudgetWarn bool
	for _, w := range out.Warnings {
		if w.Type == "token_budget_reached" {
			foundBudgetWarn = true
			break
		}
	}
	if !foundBudgetWarn {
		t.Error("expected token_budget_reached warning preserved in output")
	}
}

// TestEmitRunResult_JSONBudgetDoesNotOverrideLegacyStatus guards the legacy
// (manifest-less) path: budgetExceeded must not rewrite the warning-derived
// status there either. Before the unified contract a budget trip forced
// status=="budget_exceeded" and masked the fact that a subtask had errored;
// now the error status survives and the budget shows up only in the summary.
func TestEmitRunResult_JSONBudgetDoesNotOverrideLegacyStatus(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed:  1,
		warnings:       []agent.AgentWarning{{Type: "subtask_error", File: "x.go", Message: "boom"}},
		budgetExceeded: true,
	}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != "completed_with_errors" {
		t.Errorf("status = %q, want completed_with_errors (budgetExceeded must not override it)", out.Status)
	}
	if out.Summary == nil || !out.Summary.BudgetExceeded {
		t.Errorf("summary.budget_exceeded = %v, want true", out.Summary)
	}
}

// TestEmitRunResult_JSONNoBudgetIsSuccess verifies the default path is
// unchanged: BudgetExceeded()==false yields the normal status (regression
// guard that the new field/param didn't disturb existing behavior).
func TestEmitRunResult_JSONNoBudgetIsSuccess(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed: 1,
		inputTokens:   10,
		outputTokens:  5,
		totalTokens:   15,
	}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != "success" {
		t.Errorf("status = %q, want success", out.Status)
	}
	if out.Summary != nil && out.Summary.BudgetExceeded {
		t.Error("budget_exceeded should be false/omitted on a normal success run")
	}
}

// TestEmitFailureUsage_TextEmitsStructuredRecord verifies the non-budget
// failure path emits a structured usage record to stderr with the token totals
// and budget_exceeded=false in text format.
func TestEmitFailureUsage_TextEmitsStructuredRecord(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed: 4,
		inputTokens:   1000,
		outputTokens:  500,
		totalTokens:   1500,
		toolCalls:     map[string]int64{"file_read": 3, "code_comment": 2},
		toolFailures: []llmloop.ToolFailureDetail{
			{ToolName: "file_read"},
			{ToolName: "code_comment"},
		},
		sessionID: "sess-fail-1",
	}
	got := captureStderr(t, func() {
		emitFailureUsage(ag, 42*time.Second, "text", nil, nil)
	})
	for _, want := range []string{"usage on failure", "1500 total tokens", "5 tool calls, 2 failed", "budget_exceeded=false", "sess-fail-1"} {
		if !strings.Contains(got, want) {
			t.Errorf("stderr missing %q; got %q", want, got)
		}
	}
}

func TestEmitFailureUsage_TextOmitsZeroToolFailures(t *testing.T) {
	ag := &mockResultProvider{toolCalls: map[string]int64{"file_read": 2}}
	got := captureStderr(t, func() {
		emitFailureUsage(ag, time.Second, "text", nil, nil)
	})
	if !strings.Contains(got, "2 tool calls, elapsed") {
		t.Errorf("stderr missing unchanged zero-failure summary; got %q", got)
	}
	if strings.Contains(got, "0 failed") {
		t.Errorf("stderr should omit zero tool failures; got %q", got)
	}
}

// TestEmitFailureUsage_JSONEmitsStructuredRecord verifies the JSON form emits a
// parseable record to stderr with budget_exceeded=false.
func TestEmitFailureUsage_JSONEmitsStructuredRecord(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed: 2,
		inputTokens:   200,
		outputTokens:  80,
		totalTokens:   280,
		toolCalls:     map[string]int64{"file_read": 1},
		toolFailures: []llmloop.ToolFailureDetail{{
			ToolCallNumber: 1,
			ToolName:       "file_read",
			FilePath:       "missing.go",
			Arguments:      `{"file_path":"missing.go"}`,
			Error:          "file not found",
		}},
	}
	identity := &jsonLLMIdentity{Provider: "openai", Model: "gpt-5.4"}
	got := captureStderr(t, func() {
		emitFailureUsage(ag, 5*time.Second, "json", identity, nil)
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal stderr json: %v\ngot: %q", err, got)
	}
	if out.Status != "failed" {
		t.Errorf("status = %q, want failed", out.Status)
	}
	if out.Summary == nil {
		t.Fatal("expected summary on failure usage record")
	}
	if out.Summary.BudgetExceeded {
		t.Error("budget_exceeded must be false on the non-budget failure path")
	}
	if out.Summary.TotalTokens != 280 {
		t.Errorf("total_tokens = %d, want 280", out.Summary.TotalTokens)
	}
	if out.ToolCalls == nil || out.ToolCalls.Total != 1 {
		t.Errorf("tool_calls.total = %v, want 1", out.ToolCalls)
	}
	if out.ToolCalls.Failure != 1 || len(out.ToolCalls.FailureDetails) != 1 {
		t.Errorf("tool call failures = %+v, want one", out.ToolCalls)
	}
	if out.ToolCalls.FailureByTool["file_read"] != 1 {
		t.Errorf("failure_by_tool = %+v, want file_read=1", out.ToolCalls.FailureByTool)
	}
	if got := out.ToolCalls.FailureDetails[0].Arguments; got != `{"file_path":"missing.go"}` {
		t.Errorf("failure arguments = %q, want raw tool arguments", got)
	}
	if out.LLM == nil || out.LLM.Provider != "openai" || out.LLM.Model != "gpt-5.4" {
		t.Fatalf("llm = %+v", out.LLM)
	}
}

// TestEmitFailureUsage_BudgetExceededPropagated closes the residual edge the
// OCR review flagged: a budget gate can trip after dispatching N files, and if
// every dispatched file then fails, dispatchSubtasks returns (nil, error) — so
// the failure path is reached with BudgetExceeded()==true. The failure usage
// record must report the agent's ACTUAL budget state, never a hardcoded false,
// so it cannot contradict the agent's typed status.
func TestEmitFailureUsage_BudgetExceededPropagated(t *testing.T) {
	// Text form.
	ag := &mockResultProvider{
		filesReviewed:  1,
		totalTokens:    100,
		budgetExceeded: true,
	}
	got := captureStderr(t, func() {
		emitFailureUsage(ag, 3*time.Second, "text", nil, nil)
	})
	if !strings.Contains(got, "budget_exceeded=true") {
		t.Errorf("text failure record must reflect budget_exceeded=true; got %q", got)
	}

	// JSON form.
	ag2 := &mockResultProvider{
		filesReviewed:  1,
		totalTokens:    100,
		budgetExceeded: true,
	}
	gotJSON := captureStderr(t, func() {
		emitFailureUsage(ag2, 3*time.Second, "json", nil, nil)
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(gotJSON), &out); err != nil {
		t.Fatalf("unmarshal stderr json: %v\ngot: %q", err, gotJSON)
	}
	if out.Summary == nil || !out.Summary.BudgetExceeded {
		t.Errorf("JSON failure record must carry summary.budget_exceeded=true; got %+v", out.Summary)
	}
}

// TestEmitRunResult_BudgetExceededFalseOmittedFromJSON is a regression guard
// that the omitempty tag on BudgetExceeded keeps the success JSON free of the
// key (so old parsers see no diff on the common path).
func TestEmitRunResult_BudgetExceededFalseOmittedFromJSON(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed: 1,
		inputTokens:   10,
		totalTokens:   10,
	}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, []model.LlmComment{}, time.Now(), "json", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if strings.Contains(got, "budget_exceeded") {
		t.Errorf("success JSON should omit budget_exceeded (omitempty), got %q", got)
	}
}

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/model"
)

// TestEmitRunResult_JSONBudgetExceededStatus verifies that a provider signaling
// BudgetExceeded()==true produces JSON with status=="budget_exceeded" AND
// summary.budget_exceeded==true (INV-3 typed status), and that it takes
// precedence over completed_with_warnings.
func TestEmitRunResult_JSONBudgetExceededStatus(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed:  3,
		inputTokens:    100,
		outputTokens:   50,
		totalTokens:    150,
		warnings:       []agent.AgentWarning{{Type: "token_budget_reached", File: "big.go", Message: "stopped"}},
		toolCalls:      map[string]int64{"file_read": 2},
		budgetExceeded: true,
	}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != "budget_exceeded" {
		t.Errorf("status = %q, want budget_exceeded (must take precedence over completed_with_warnings)", out.Status)
	}
	if out.Summary == nil || !out.Summary.BudgetExceeded {
		t.Errorf("summary.budget_exceeded = %v, want true", out.Summary)
	}
	// The token_budget_reached warning must still be present in the output so
	// the reason is observable alongside the typed status.
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

// TestEmitRunResult_JSONBudgetExceededPrecedenceOverErrors verifies that
// budget_exceeded takes precedence over completed_with_errors too — a budget
// trip is a distinct typed terminal state (INV-3 lists completed_with_errors
// as a status it must be distinct from).
func TestEmitRunResult_JSONBudgetExceededPrecedenceOverErrors(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed:  1,
		warnings:       []agent.AgentWarning{{Type: "subtask_error", File: "x.go", Message: "boom"}},
		budgetExceeded: true,
	}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != "budget_exceeded" {
		t.Errorf("status = %q, want budget_exceeded (must take precedence over completed_with_errors)", out.Status)
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
		err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil)
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
// and budget_exceeded=false (INV-4). Text format.
func TestEmitFailureUsage_TextEmitsStructuredRecord(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed: 4,
		inputTokens:   1000,
		outputTokens:  500,
		totalTokens:   1500,
		toolCalls:     map[string]int64{"file_read": 3, "code_comment": 2},
		sessionID:     "sess-fail-1",
	}
	got := captureStderr(t, func() {
		emitFailureUsage(ag, 42*time.Second, "text")
	})
	for _, want := range []string{"usage on failure", "1500 total tokens", "5 tool calls", "budget_exceeded=false", "sess-fail-1"} {
		if !strings.Contains(got, want) {
			t.Errorf("stderr missing %q; got %q", want, got)
		}
	}
}

// TestEmitFailureUsage_JSONEmitsStructuredRecord verifies the JSON form emits a
// parseable record to stderr with budget_exceeded=false (INV-4).
func TestEmitFailureUsage_JSONEmitsStructuredRecord(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed: 2,
		inputTokens:   200,
		outputTokens:  80,
		totalTokens:   280,
		toolCalls:     map[string]int64{"file_read": 1},
	}
	got := captureStderr(t, func() {
		emitFailureUsage(ag, 5*time.Second, "json")
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
		emitFailureUsage(ag, 3*time.Second, "text")
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
		emitFailureUsage(ag2, 3*time.Second, "json")
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
		err := emitRunResult(context.Background(), ag, []model.LlmComment{}, time.Now(), "json", "developer", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if strings.Contains(got, "budget_exceeded") {
		t.Errorf("success JSON should omit budget_exceeded (omitempty), got %q", got)
	}
}

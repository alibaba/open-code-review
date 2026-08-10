// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

type accountingTestClient struct {
	response *ChatResponse
	calls    atomic.Int64
}

func (c *accountingTestClient) CompletionsWithCtx(_ context.Context, _ ChatRequest) (*ChatResponse, error) {
	c.calls.Add(1)
	return c.response, nil
}

func accountingRequest() ChatRequest {
	return ChatRequest{
		Model: "gpt-5.6-luna",
		Messages: []Message{
			{Role: "system", Content: "instructions", Phase: "analysis"},
			{Role: "user", Content: "review this"},
			{
				Role:      "assistant",
				Content:   "calling a tool",
				ToolCalls: []ToolCall{{ID: "call_1", Type: "function", Function: FunctionCall{Name: "file_read", Arguments: `{"path":"main.go"}`}}},
				ResponseItems: []json.RawMessage{
					json.RawMessage(`{"type":"reasoning","id":"rs_1"}`),
				},
			},
			{Role: "tool", ToolCallID: "call_1", Content: "package main"},
		},
		Tools: []ToolDef{{
			Type: "function",
			Function: FunctionDef{
				Name:        "file_read",
				Description: "read a file",
				Parameters:  map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
			},
		}},
		MaxTokens: 16384,
	}
}

func TestEstimateChatRequestIncludesSemanticPayload(t *testing.T) {
	base := accountingRequest()
	base.Messages = []Message{{Role: "user", Content: "review this"}}
	base.Tools = nil
	withoutContext := EstimateChatRequest(base)
	withContext := EstimateChatRequest(accountingRequest())

	if withContext.Status != UsageStatusEstimated {
		t.Fatalf("status = %q, want %q", withContext.Status, UsageStatusEstimated)
	}
	if withContext.InputTokens <= withoutContext.InputTokens {
		t.Fatalf("semantic payload estimate = %d, base = %d; roles/tools/replay data must count", withContext.InputTokens, withoutContext.InputTokens)
	}
	for _, component := range []string{"messages", "roles", "tool_calls", "tool_results", "tools", "response_items", "phase_metadata"} {
		if withContext.Components[component] <= 0 {
			t.Errorf("component %q = %d, want positive", component, withContext.Components[component])
		}
	}

	withoutOutputLimit := accountingRequest()
	withoutOutputLimit.MaxTokens = 1
	if got, want := EstimateChatRequest(withoutOutputLimit).InputTokens, withContext.InputTokens; got != want {
		t.Errorf("MaxTokens changed input estimate: got %d, want %d", got, want)
	}
}

func TestEstimateChatRequestMarksLunaTokenizerFallback(t *testing.T) {
	est := EstimateChatRequest(ChatRequest{Model: "gpt-5.6-luna", Messages: []Message{{Role: "user", Content: "hello"}}})
	if est.Encoding == "" {
		t.Fatal("Luna estimate must expose the selected fallback encoding")
	}
	if est.ModelMappingKnown {
		t.Error("gpt-5.6-luna must not claim an exact local tokenizer mapping")
	}
	if est.Status != UsageStatusEstimated {
		t.Errorf("status = %q, want estimated fallback", est.Status)
	}
}

func TestEffectivePreflightCeiling(t *testing.T) {
	if got, want := EffectivePreflightCeiling(262144), int64(222822); got != want {
		t.Fatalf("EffectivePreflightCeiling(262144) = %d, want %d", got, want)
	}
}

func TestAccountingClientUsesEstimatedFallbackAndActualUsage(t *testing.T) {
	request := accountingRequest()
	content := "done"
	fake := &accountingTestClient{response: &ChatResponse{Choices: []Choice{{Message: ResponseMessage{Content: &content}}}}}
	accounting := NewTokenAccounting(TokenAccountingOptions{ContextBudget: 262144})
	client := NewAccountingClient(fake, accounting)

	resp, err := client.CompletionsWithCtx(context.Background(), request)
	if err != nil {
		t.Fatalf("estimated completion: %v", err)
	}
	if resp.UsageStatus != UsageStatusEstimated {
		t.Fatalf("UsageStatus = %q, want estimated", resp.UsageStatus)
	}
	est := EstimateChatRequest(request)
	if resp.Usage == nil || resp.Usage.PromptTokens != est.InputTokens {
		t.Fatalf("fallback usage = %+v, want prompt tokens %d", resp.Usage, est.InputTokens)
	}
	snapshot := accounting.Snapshot()
	if snapshot.EstimatedRequests != 1 || snapshot.ActualRequests != 0 || snapshot.UnknownRequests != 0 {
		t.Fatalf("snapshot statuses = %+v", snapshot)
	}

	actual := &UsageInfo{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10}
	fake.response = &ChatResponse{Choices: []Choice{{Message: ResponseMessage{Content: &content}}}, Usage: actual}
	if _, err := client.CompletionsWithCtx(context.Background(), request); err != nil {
		t.Fatalf("actual completion: %v", err)
	}
	snapshot = accounting.Snapshot()
	fallbackOutput := EstimateChatResponseTokens(&ChatResponse{Choices: []Choice{{Message: ResponseMessage{Content: &content}}}}, request.Model)
	if snapshot.InputTokens != est.InputTokens+7 || snapshot.OutputTokens != fallbackOutput+3 {
		t.Fatalf("actual/fallback totals = %+v", snapshot)
	}
	if snapshot.EstimatedRequests != 1 || snapshot.ActualRequests != 1 {
		t.Fatalf("actual reconciliation statuses = %+v", snapshot)
	}
}

func TestTokenAccountingReconcileReplacesEstimate(t *testing.T) {
	accounting := NewTokenAccounting(TokenAccountingOptions{ContextBudget: 262144})
	request := ChatRequest{Model: "gpt-5.6-luna", Messages: []Message{{Role: "user", Content: "hello"}}}
	handle, err := accounting.Begin(request)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	content := "estimated response"
	record := accounting.Finish(handle, &ChatResponse{Choices: []Choice{{Message: ResponseMessage{Content: &content}}}})
	if record.Status != UsageStatusEstimated {
		t.Fatalf("initial status = %q, want estimated", record.Status)
	}

	if !accounting.Reconcile(handle, &UsageInfo{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10}) {
		t.Fatal("Reconcile returned false")
	}
	snapshot := accounting.Snapshot()
	if snapshot.InputTokens != 7 || snapshot.OutputTokens != 3 || snapshot.TotalTokens != 10 {
		t.Fatalf("reconciled totals = %+v, want input=7 output=3 total=10", snapshot)
	}
	if snapshot.Requests != 1 || snapshot.EstimatedRequests != 0 || snapshot.ActualRequests != 1 {
		t.Fatalf("reconciled statuses = %+v", snapshot)
	}
	if accounting.Reconcile(handle, &UsageInfo{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}) {
		t.Fatal("an actual record must not be reconciled twice")
	}
}

func TestTokenAccountingFinishCanReconcileLaterUsage(t *testing.T) {
	accounting := NewTokenAccounting(TokenAccountingOptions{ContextBudget: 262144})
	handle, err := accounting.Begin(ChatRequest{Model: "gpt-5.6-luna", Messages: []Message{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	content := "estimated response"
	accounting.Finish(handle, &ChatResponse{Choices: []Choice{{Message: ResponseMessage{Content: &content}}}})

	resp := &ChatResponse{Usage: &UsageInfo{PromptTokens: 4, CompletionTokens: 2, TotalTokens: 6}}
	record := accounting.Finish(handle, resp)
	if record.Status != UsageStatusActual || resp.UsageStatus != UsageStatusActual {
		t.Fatalf("late finish status = %q, response status = %q", record.Status, resp.UsageStatus)
	}
	snapshot := accounting.Snapshot()
	if snapshot.TotalTokens != 6 || snapshot.EstimatedRequests != 0 || snapshot.ActualRequests != 1 {
		t.Fatalf("late finish totals = %+v, want one actual record totaling 6", snapshot)
	}
}

func TestAccountingClientUnknownUsageIsNotZeroStatus(t *testing.T) {
	fake := &accountingTestClient{response: &ChatResponse{Choices: []Choice{{Message: ResponseMessage{Role: "assistant"}}}}}
	accounting := NewTokenAccounting(TokenAccountingOptions{
		Estimator: func(ChatRequest) RequestEstimate { return RequestEstimate{Status: UsageStatusUnknown} },
	})
	client := NewAccountingClient(fake, accounting)
	resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{Model: "unsupported"})
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	if resp.UsageStatus != UsageStatusUnknown || resp.Usage != nil {
		t.Fatalf("response usage = %+v status=%q, want nil/unknown", resp.Usage, resp.UsageStatus)
	}
	snapshot := accounting.Snapshot()
	if snapshot.UnknownRequests != 1 || snapshot.Requests != 1 {
		t.Fatalf("unknown snapshot = %+v", snapshot)
	}
	if snapshot.TotalTokens != 0 {
		t.Errorf("unknown request must not be represented as a precise token total: %d", snapshot.TotalTokens)
	}
}

func TestAccountingClientStopsAfterPositiveBudget(t *testing.T) {
	content := "done"
	fake := &accountingTestClient{response: &ChatResponse{
		Choices: []Choice{{Message: ResponseMessage{Content: &content}}},
		Usage:   &UsageInfo{PromptTokens: 6, CompletionTokens: 4, TotalTokens: 10},
	}}
	accounting := NewTokenAccounting(TokenAccountingOptions{AggregateBudget: 10})
	client := NewAccountingClient(fake, accounting)
	request := ChatRequest{Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hello"}}}
	if _, err := client.CompletionsWithCtx(context.Background(), request); err != nil {
		t.Fatalf("first completion: %v", err)
	}
	if accounting.Snapshot().BudgetExceeded {
		t.Fatal("reaching the cap must not report a rejected dispatch")
	}
	if _, err := client.CompletionsWithCtx(context.Background(), request); !errors.Is(err, ErrTokenBudgetExceeded) {
		t.Fatalf("second completion error = %v, want ErrTokenBudgetExceeded", err)
	}
	if got := fake.calls.Load(); got != 1 {
		t.Fatalf("provider calls = %d, want 1", got)
	}
	if !accounting.Snapshot().BudgetExceeded {
		t.Error("budget stop was not recorded")
	}
}

func TestTokenAccountingTracksProviderTotalWithoutSplit(t *testing.T) {
	accounting := NewTokenAccounting(TokenAccountingOptions{AggregateBudget: 10})
	accounting.RecordExternal(&UsageInfo{TotalTokens: 10})
	snapshot := accounting.Snapshot()
	if snapshot.TotalTokens != 10 || snapshot.UnattributedTokens != 10 {
		t.Fatalf("total-only usage = %+v, want total/unattributed 10", snapshot)
	}
	if snapshot.InputTokens != 0 || snapshot.OutputTokens != 0 {
		t.Fatalf("total-only usage was assigned to a split bucket: %+v", snapshot)
	}

	if _, err := NewAccountingClient(&accountingTestClient{}, accounting).CompletionsWithCtx(context.Background(), ChatRequest{Model: "gpt-4"}); !errors.Is(err, ErrTokenBudgetExceeded) {
		t.Fatalf("next completion error = %v, want ErrTokenBudgetExceeded", err)
	}
}

func TestAccountingClientZeroBudgetIsUnlimited(t *testing.T) {
	content := "done"
	fake := &accountingTestClient{response: &ChatResponse{Choices: []Choice{{Message: ResponseMessage{Content: &content}}}}}
	accounting := NewTokenAccounting(TokenAccountingOptions{AggregateBudget: 0})
	client := NewAccountingClient(fake, accounting)
	request := ChatRequest{Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hello"}}}
	for range 2 {
		if _, err := client.CompletionsWithCtx(context.Background(), request); err != nil {
			t.Fatalf("unlimited completion: %v", err)
		}
	}
	if got := fake.calls.Load(); got != 2 {
		t.Fatalf("provider calls = %d, want 2", got)
	}
}

func TestAccountingClientContextPreflightUsesSingleSafetyMargin(t *testing.T) {
	content := "done"
	fake := &accountingTestClient{response: &ChatResponse{Choices: []Choice{{Message: ResponseMessage{Content: &content}}}}}
	accounting := NewTokenAccounting(TokenAccountingOptions{ContextBudget: 100})
	client := NewAccountingClient(fake, accounting)
	request := ChatRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: strings.Repeat("word ", 100)}},
	}
	if _, err := client.CompletionsWithCtx(context.Background(), request); !errors.Is(err, ErrContextBudgetExceeded) {
		t.Fatalf("completion error = %v, want ErrContextBudgetExceeded", err)
	}
	if got := fake.calls.Load(); got != 0 {
		t.Fatalf("provider calls = %d, want 0 after preflight rejection", got)
	}
}

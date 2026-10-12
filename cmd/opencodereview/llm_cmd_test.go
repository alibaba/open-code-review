// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/testconnection"
	"github.com/alibaba/open-code-review/internal/llm"
)

// TestToolRoundTripNote guards the reporting contract for #1357: a provider that
// never calls the test tool leaves the round trip unproven, and saying so is the
// whole point — a plain success line is what let a broken provider look healthy.
func TestToolRoundTripNote(t *testing.T) {
	tests := []struct {
		name    string
		offered bool
		called  bool
		want    string
	}{
		{name: "no tool offered says nothing", offered: false, called: false, want: ""},
		{name: "tool called reports verified", offered: true, called: true, want: "verified"},
		{name: "tool not called reports unverified", offered: true, called: false, want: "unverified"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := toolRoundTripNote(tc.offered, tc.called)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("note = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("note = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

// TestToolRoundTripNote_UnverifiedIsNotMistakenForSuccess keeps the two states
// textually distinct, so neither reads as the other.
func TestToolRoundTripNote_UnverifiedIsNotMistakenForSuccess(t *testing.T) {
	unverified := toolRoundTripNote(true, false)
	if strings.Contains(unverified, "✓") {
		t.Errorf("unverified note must not carry a success mark: %q", unverified)
	}
}

// TestTestToolDef maps the configured tool onto the shape the LLM clients take.
func TestTestToolDef(t *testing.T) {
	spec := &testconnection.ToolSpec{
		Name:        "ocr_selftest",
		Description: "echoes",
		Parameters:  map[string]any{"type": "object"},
	}
	tools := testToolDefs(spec)
	if len(tools) != 1 {
		t.Fatalf("tools = %d, want 1", len(tools))
	}
	if tools[0].Type != "function" {
		t.Errorf("type = %q, want function", tools[0].Type)
	}
	if tools[0].Function.Name != "ocr_selftest" {
		t.Errorf("name = %q", tools[0].Function.Name)
	}
	if tools[0].Function.Parameters["type"] != "object" {
		t.Errorf("parameters not carried through: %v", tools[0].Function.Parameters)
	}
	if got := testToolDefs(nil); got != nil {
		t.Errorf("a task without a tool must offer none, got %v", got)
	}
}

// TestFindTestToolCall picks out the configured tool from a response, ignoring
// anything else the model may have asked for.
func TestFindTestToolCall(t *testing.T) {
	resp := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{
		{ID: "call_other", Function: llm.FunctionCall{Name: "something_else"}},
		{ID: "call_1", Function: llm.FunctionCall{Name: "ocr_selftest"}},
	}}}}}

	tc := findTestToolCall(resp, "ocr_selftest")
	if tc == nil {
		t.Fatal("expected the test tool call to be found")
	}
	if tc.ID != "call_1" {
		t.Errorf("ID = %q, want call_1", tc.ID)
	}
	if findTestToolCall(resp, "absent") != nil {
		t.Error("expected nil when the tool was not called")
	}
	if findTestToolCall(&llm.ChatResponse{}, "ocr_selftest") != nil {
		t.Error("expected nil for a response with no choices")
	}
}

// TestFindTestToolCall_EmptyNameMatchesNothing guards the task-without-a-tool
// path: an empty name must not match a provider's unnamed tool call, or the
// caller would go on to dereference a tool spec that does not exist.
func TestFindTestToolCall_EmptyNameMatchesNothing(t *testing.T) {
	resp := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{
		{ID: "call_unnamed", Function: llm.FunctionCall{Name: ""}},
	}}}}}

	if tc := findTestToolCall(resp, ""); tc != nil {
		t.Fatalf("empty name matched %q; a task with no tool must find nothing", tc.ID)
	}
}

// TestToolResultMessages_AnswersEveryCall guards the protocol rule both
// supported families share: an assistant turn carrying tool calls must be
// followed by a result for each one. Leaving any unanswered makes the next
// request fail for a reason that has nothing to do with the provider's handling
// of tool-call metadata, which is the only thing this test is trying to learn.
func TestToolResultMessages_AnswersEveryCall(t *testing.T) {
	spec := &testconnection.ToolSpec{Name: "ocr_selftest", Result: "ocr_selftest ok"}
	resp := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{
		{ID: "call_1", Function: llm.FunctionCall{Name: "file_read"}},
		{ID: "call_2", Function: llm.FunctionCall{Name: "ocr_selftest"}},
		{ID: "call_3", Function: llm.FunctionCall{Name: "code_search"}},
	}}}}}

	got := toolResultMessages(resp, spec)
	if len(got) != 3 {
		t.Fatalf("produced %d results for 3 tool calls, want 3", len(got))
	}
	for i, want := range []string{"call_1", "call_2", "call_3"} {
		if got[i].ToolCallID != want {
			t.Errorf("result %d answers %q, want %q", i, got[i].ToolCallID, want)
		}
		if got[i].Role != "tool" {
			t.Errorf("result %d role = %q, want tool", i, got[i].Role)
		}
	}
	if got[1].ExtractText() != "ocr_selftest ok" {
		t.Errorf("self-test call got %q, want the configured result", got[1].ExtractText())
	}
	if got[0].ExtractText() == "ocr_selftest ok" {
		t.Error("a tool the test never offered must not receive the self-test result")
	}
	if got[0].ExtractText() == "" {
		t.Error("every result needs content; an empty one is rejected by some providers")
	}
}

// TestToolResultMessages_NoCalls returns nothing when the model called nothing.
func TestToolResultMessages_NoCalls(t *testing.T) {
	if got := toolResultMessages(&llm.ChatResponse{}, nil); len(got) != 0 {
		t.Fatalf("got %d results for a response with no tool calls", len(got))
	}
}

// TestSecondTurnPairsEveryToolCall is the regression test for the review on
// #1394: the second request replays the whole assistant turn, so every tool
// call in it must be answered exactly once. An unanswered call makes a working
// provider reject the request, which is indistinguishable from the provider
// rejection this command exists to detect.
func TestSecondTurnPairsEveryToolCall(t *testing.T) {
	spec := &testconnection.ToolSpec{Name: "ocr_selftest", Result: "ocr_selftest ok"}
	resp := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{
		{ID: "call_1", Function: llm.FunctionCall{Name: "ocr_selftest"}},
		{ID: "call_2", Function: llm.FunctionCall{Name: "file_read"}},
		{ID: "call_3", Function: llm.FunctionCall{Name: "ocr_selftest"}},
	}}}}}

	// The same construction runLLMTest performs for the second request.
	assistant := llm.NewToolCallMessage(resp.VisibleContent(), resp.ToolCalls(), resp.Native(), resp.ReasoningContent())
	turn := append([]llm.Message{assistant}, toolResultMessages(resp, spec)...)

	answered := map[string]int{}
	for _, m := range turn[1:] {
		answered[m.ToolCallID]++
	}
	for _, tc := range assistant.ToolCalls {
		switch answered[tc.ID] {
		case 1:
		case 0:
			t.Errorf("tool call %q (%s) is replayed but never answered", tc.ID, tc.Function.Name)
		default:
			t.Errorf("tool call %q answered %d times, want exactly 1", tc.ID, answered[tc.ID])
		}
	}
	if len(answered) != len(assistant.ToolCalls) {
		t.Errorf("%d results for %d replayed calls", len(answered), len(assistant.ToolCalls))
	}
	// A repeated call to the offered tool is still that tool, not an unknown one.
	if got := toolResultMessages(resp, spec)[2].ExtractText(); got != "ocr_selftest ok" {
		t.Errorf("repeated self-test call got %q, want the configured result", got)
	}
}

func TestLLMTestCommand_UsesDefaultConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	original := runLLMTestPath
	t.Cleanup(func() { runLLMTestPath = original })
	var gotPath string
	runLLMTestPath = func(configPath string) error {
		gotPath = configPath
		return nil
	}

	if err := llmTestCmd.RunE(llmTestCmd, nil); err != nil {
		t.Fatalf("llm test: %v", err)
	}
	if want := filepath.Join(home, ".opencodereview", "config.json"); gotPath != want {
		t.Fatalf("config path = %q, want %q", gotPath, want)
	}
}

type connectionTestClient struct {
	requests  []llm.ChatRequest
	responses []*llm.ChatResponse
	err       error
}

func (c *connectionTestClient) CompletionsWithCtx(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	c.requests = append(c.requests, request)
	if c.err != nil {
		return nil, c.err
	}
	i := len(c.requests) - 1
	if i >= len(c.responses) {
		return nil, nil
	}
	return c.responses[i], nil
}

func TestChatGPTConnectionTestRequiresToolAndVisibleAnswer(t *testing.T) {
	text := "Self-test verified."
	tool := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{{ID: "call_1", Function: llm.FunctionCall{Name: "ocr_selftest", Arguments: `{"note":"hello"}`}}}}}}}
	answer := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &text}}}}
	for _, test := range []struct {
		name      string
		responses []*llm.ChatResponse
		err       error
		success   bool
	}{
		{"verified", []*llm.ChatResponse{tool, answer}, nil, true},
		{"no tool", []*llm.ChatResponse{answer}, nil, false},
		{"empty final answer", []*llm.ChatResponse{tool, {}}, nil, false},
		{"reasoning is not an answer", []*llm.ChatResponse{tool, {Choices: []llm.Choice{{Message: llm.ResponseMessage{ReasoningContent: "reasoning only"}}}}}, nil, false},
		{"nil response", nil, nil, false},
		{"request failure", nil, errors.New("request failed"), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, key := range []string{"OCR_LLM_URL", "OCR_LLM_TOKEN", "OCR_LLM_MODEL", "OCR_LLM_PROTOCOL"} {
				t.Setenv(key, "")
			}
			path := filepath.Join(t.TempDir(), "config.json")
			data, _ := json.Marshal(map[string]any{"provider": "openai-chatgpt", "providers": map[string]any{"openai-chatgpt": map[string]any{"model": "account-model"}}})
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			client := &connectionTestClient{responses: test.responses, err: test.err}
			previous := newLLMTestClient
			newLLMTestClient = func(ep llm.ResolvedEndpoint, _ *llm.RetryCollector, _ *llm.RawHolder) llm.LLMClient {
				if !ep.ChatGPT {
					t.Fatal("wrong provider")
				}
				return client
			}
			t.Cleanup(func() { newLLMTestClient = previous })
			var err error
			output := captureStdout(t, func() { err = runLLMTestWithConfigPath(path) })
			if (err == nil) != test.success {
				t.Fatalf("test result: %v, output: %s", err, output)
			}
			if !test.success && strings.Contains(output, "Connection test successful") {
				t.Fatal("failure was reported as successful")
			}
			if client.requests[0].ToolChoice != "required" {
				t.Fatal("self-test must request a tool call")
			}
			if test.success {
				if len(client.requests) != 2 || client.requests[1].ToolChoice != "none" || !strings.Contains(output, "Tool-call round trip verified") || !strings.Contains(output, text) {
					t.Fatalf("incomplete probe: %s", output)
				}
			}
		})
	}
}

func TestConnectionTestKeepsLenientAnswerCheckForOtherProviders(t *testing.T) {
	for _, key := range []string{"OCR_LLM_URL", "OCR_LLM_TOKEN", "OCR_LLM_MODEL", "OCR_LLM_PROTOCOL"} {
		t.Setenv(key, "")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	data, _ := json.Marshal(map[string]any{"provider": "openai", "providers": map[string]any{"openai": map[string]any{"model": "gpt-test", "api_key": "sk-test"}}})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	reasoningOnly := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ReasoningContent: "thinking"}}}}
	client := &connectionTestClient{responses: []*llm.ChatResponse{reasoningOnly}}
	previous := newLLMTestClient
	newLLMTestClient = func(llm.ResolvedEndpoint, *llm.RetryCollector, *llm.RawHolder) llm.LLMClient { return client }
	t.Cleanup(func() { newLLMTestClient = previous })
	var err error
	output := captureStdout(t, func() { err = runLLMTestWithConfigPath(path) })
	if err != nil || !strings.Contains(output, "Connection test successful") || !strings.Contains(output, "thinking") {
		t.Fatalf("reasoning-only reply must still pass for non-ChatGPT providers: %v\n%s", err, output)
	}
	if client.requests[0].ToolChoice != "" {
		t.Fatal("only the ChatGPT route forces a tool call")
	}
}

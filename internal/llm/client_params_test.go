// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildOpenAIParams_AllRoles exercises every message-role branch plus the
// tools/max-tokens/temperature options in buildOpenAIParams.
func TestBuildOpenAIParams_AllRoles(t *testing.T) {
	c := &OpenAIClient{}
	temp := 0.5
	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hi"},
			{Role: "tool", Content: "result", ToolCallID: "call-1"},
			{Role: "assistant", Content: "plain"},
			{Role: "assistant", Content: "with tools", ToolCalls: []ToolCall{
				{ID: "call-2", Function: FunctionCall{Name: "f", Arguments: `{"a":1}`}},
			}},
			{Role: "assistant", Content: "", ToolCalls: []ToolCall{
				{ID: "call-3", Function: FunctionCall{Name: "g", Arguments: `{}`}},
			}},
			{Role: "unknown", Content: "fallback"},
		},
		Tools: []ToolDef{
			{Function: FunctionDef{Name: "f", Description: "d", Parameters: map[string]any{"type": "object"}}},
		},
		MaxTokens:   256,
		Temperature: &temp,
	}

	params := c.buildOpenAIParams("gpt-x", req)

	if string(params.Model) != "gpt-x" {
		t.Errorf("model = %q, want gpt-x", params.Model)
	}
	if len(params.Messages) != len(req.Messages) {
		t.Errorf("messages = %d, want %d", len(params.Messages), len(req.Messages))
	}
	if len(params.Tools) != 1 {
		t.Errorf("tools = %d, want 1", len(params.Tools))
	}
	if params.MaxCompletionTokens.Value != 256 {
		t.Errorf("max completion tokens = %d, want 256", params.MaxCompletionTokens.Value)
	}
	if params.Temperature.Value != 0.5 {
		t.Errorf("temperature = %v, want 0.5", params.Temperature.Value)
	}
}

// TestBuildOpenAIParams_Minimal verifies that optional fields stay unset when
// the request omits tools, max tokens, and temperature.
func TestBuildOpenAIParams_Minimal(t *testing.T) {
	c := &OpenAIClient{}
	params := c.buildOpenAIParams("m", ChatRequest{
		Messages: []Message{{Role: "user", Content: "x"}},
	})
	if len(params.Tools) != 0 {
		t.Errorf("tools = %d, want 0", len(params.Tools))
	}
	if params.MaxCompletionTokens.Valid() {
		t.Error("expected max completion tokens unset")
	}
	if params.Temperature.Valid() {
		t.Error("expected temperature unset")
	}
}

// TestBuildAnthropicParams_AllRoles exercises every message-role branch (system,
// tool-result flushing, assistant with/without tool calls, user string and
// content-block content) plus tools/system cache-control and temperature.
func TestBuildAnthropicParams_AllRoles(t *testing.T) {
	c := &AnthropicClient{}
	temp := 0.3
	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "sys"},
			{Role: "tool", Content: "tool-result", ToolCallID: "call-1"},
			{Role: "assistant", Content: "with tools", ToolCalls: []ToolCall{
				{ID: "call-2", Function: FunctionCall{Name: "f", Arguments: `{"a":1}`}},
			}},
			{Role: "assistant", Content: "plain-no-tools"},
			{Role: "user", Content: "hi"},
			{Role: "user", Content: []ContentBlock{
				{Type: "text", Text: "block-text"},
				{Type: "tool_result", ToolUseID: "call-3", Text: "tr"},
			}},
		},
		Tools: []ToolDef{
			{Function: FunctionDef{Name: "f", Description: "d", Parameters: map[string]any{"type": "object"}}},
		},
		MaxTokens:   512,
		Temperature: &temp,
	}

	params, err := c.buildAnthropicParams("claude-x", req)
	if err != nil {
		t.Fatalf("buildAnthropicParams returned error: %v", err)
	}
	if string(params.Model) != "claude-x" {
		t.Errorf("model = %q, want claude-x", params.Model)
	}
	if params.MaxTokens != 512 {
		t.Errorf("max tokens = %d, want 512", params.MaxTokens)
	}
	if len(params.System) == 0 {
		t.Error("expected system blocks to be set")
	}
	if len(params.Tools) != 1 {
		t.Errorf("tools = %d, want 1", len(params.Tools))
	}
	if !params.Temperature.Valid() || params.Temperature.Value != 0.3 {
		t.Errorf("temperature = %v, want 0.3", params.Temperature)
	}
	if len(params.Messages) == 0 {
		t.Error("expected messages to be built")
	}
}

// TestBuildAnthropicParams_InvalidToolArgs covers the error branch where an
// assistant tool call carries malformed JSON arguments.
func TestBuildAnthropicParams_InvalidToolArgs(t *testing.T) {
	c := &AnthropicClient{}
	_, err := c.buildAnthropicParams("claude-x", ChatRequest{
		Messages: []Message{
			{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "call-1", Function: FunctionCall{Name: "bad", Arguments: `{not-json`}},
			}},
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid tool call arguments")
	}
}

// TestBuildAnthropicParams_DefaultMaxTokens verifies the fallback to 8192 when
// the request omits MaxTokens, and that optional fields stay unset.
func TestBuildAnthropicParams_DefaultMaxTokens(t *testing.T) {
	c := &AnthropicClient{}
	params, err := c.buildAnthropicParams("m", ChatRequest{
		Messages: []Message{{Role: "user", Content: "x"}},
	})
	if err != nil {
		t.Fatalf("buildAnthropicParams returned error: %v", err)
	}
	if params.MaxTokens != 8192 {
		t.Errorf("max tokens = %d, want default 8192", params.MaxTokens)
	}
	if len(params.Tools) != 0 {
		t.Errorf("tools = %d, want 0", len(params.Tools))
	}
	if len(params.System) != 0 {
		t.Errorf("system = %d, want 0", len(params.System))
	}
	if params.Temperature.Valid() {
		t.Error("expected temperature unset")
	}
}

// TestBuildToolInputSchema covers properties, required filtering (non-string
// entries dropped), and extra-field passthrough.
func TestBuildToolInputSchema(t *testing.T) {
	props := map[string]any{"a": map[string]any{"type": "string"}}
	schema := buildToolInputSchema(map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             []any{"a", 42},
		"additionalProperties": false,
	})

	if schema.Properties == nil {
		t.Error("properties not set")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "a" {
		t.Errorf("required = %v, want [a] (non-string dropped)", schema.Required)
	}
	if schema.ExtraFields == nil || schema.ExtraFields["additionalProperties"] != false {
		t.Errorf("extra field additionalProperties not preserved: %v", schema.ExtraFields)
	}
	if _, ok := schema.ExtraFields["type"]; ok {
		t.Error("reserved key 'type' should not appear in ExtraFields")
	}
}

// TestBuildToolInputSchema_Empty ensures a bare schema stays empty.
func TestBuildToolInputSchema_Empty(t *testing.T) {
	schema := buildToolInputSchema(map[string]any{})
	if schema.Properties != nil || len(schema.Required) != 0 || schema.ExtraFields != nil {
		t.Errorf("empty input produced non-empty schema: %+v", schema)
	}
}

// TestBuildOpenAIParams_EmptyAssistantContentKeepsContentField pins the wire
// contract for assistant messages without tool calls: content must be present
// even when empty ({"role":"assistant"} alone is rejected by strict servers).
func TestBuildOpenAIParams_EmptyAssistantContentKeepsContentField(t *testing.T) {
	client := NewOpenAIClient(ClientConfig{URL: "https://api.example.com/v1"})
	params := client.buildOpenAIParams("m", ChatRequest{Messages: []Message{
		{Role: "assistant", Content: ""},
	}})
	if len(params.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(params.Messages))
	}
	payload, err := json.Marshal(params.Messages[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"content":""`) {
		t.Fatalf("empty assistant content field missing from wire message: %s", payload)
	}
}

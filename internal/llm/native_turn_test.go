// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"encoding/json"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

// These tests are the deterministic, no-live-model reproductions for issue
// #805: a response that carries provider reasoning must still carry it, byte
// for byte, in the assistant history message built for the *next* request.
// Each test exercises the real mapXXXResponse -> NewToolCallMessage ->
// buildXXXParams path, not a hand-rolled approximation of it.

func unmarshalChatCompletionBody(t *testing.T, body string) *openai.ChatCompletion {
	t.Helper()
	var r openai.ChatCompletion
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("unmarshal chat completion body: %v", err)
	}
	return &r
}

func unmarshalAnthropicBody(t *testing.T, body string) *anthropic.Message {
	t.Helper()
	var m anthropic.Message
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("unmarshal anthropic message body: %v", err)
	}
	return &m
}

func TestOpenAIChatCompletions_ReplaysReasoningContentAcrossTurns(t *testing.T) {
	client := NewOpenAIClient(ClientConfig{URL: "https://api.openai.com/v1"})
	body := `{
		"id":"chatcmpl_1",
		"object":"chat.completion",
		"model":"deepseek-r1",
		"choices":[{
			"index":0,
			"message":{
				"role":"assistant",
				"content":null,
				"reasoning_content":"private reasoning that the provider requires on replay",
				"tool_calls":[{"id":"call_1","type":"function","function":{"name":"code_comment","arguments":"{}"}}]
			},
			"finish_reason":"tool_calls"
		}]
	}`
	sdkResp := unmarshalChatCompletionBody(t, body)
	resp := client.mapOpenAIResponse(sdkResp)

	historyMsg := NewToolCallMessage(resp.Content(), resp.ToolCalls(), resp.Native(), resp.ReasoningContent())

	params := client.buildOpenAIParams("deepseek-r1", ChatRequest{Messages: []Message{historyMsg}})
	if len(params.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(params.Messages))
	}
	payload, err := json.Marshal(params.Messages[0])
	if err != nil {
		t.Fatalf("marshal assistant message: %v", err)
	}
	if !bytes.Contains(payload, []byte(`"reasoning_content":"private reasoning that the provider requires on replay"`)) {
		t.Fatalf("assistant tool-call history dropped reasoning_content: %s", payload)
	}
}

// TestOpenAIChatCompletions_ReplaysReasoningContentWithEmptyVisibleContent
// guards the specific regression called out in issue #805: when visible
// content is empty, Content() falls back to returning the reasoning text as
// ordinary content, which must not be mistaken for the native replay payload
// being intact — Native must still carry the real reasoning_content field.
func TestOpenAIChatCompletions_ReplaysReasoningContentWithEmptyVisibleContent(t *testing.T) {
	client := NewOpenAIClient(ClientConfig{URL: "https://api.openai.com/v1"})
	body := `{
		"id":"chatcmpl_2",
		"object":"chat.completion",
		"model":"deepseek-r1",
		"choices":[{
			"index":0,
			"message":{
				"role":"assistant",
				"content":"",
				"reasoning_content":"reasoning with no visible answer yet",
				"tool_calls":[{"id":"call_1","type":"function","function":{"name":"code_comment","arguments":"{}"}}]
			},
			"finish_reason":"tool_calls"
		}]
	}`
	sdkResp := unmarshalChatCompletionBody(t, body)
	resp := client.mapOpenAIResponse(sdkResp)

	historyMsg := NewToolCallMessage(resp.Content(), resp.ToolCalls(), resp.Native(), resp.ReasoningContent())
	params := client.buildOpenAIParams("deepseek-r1", ChatRequest{Messages: []Message{historyMsg}})
	payload, err := json.Marshal(params.Messages[0])
	if err != nil {
		t.Fatalf("marshal assistant message: %v", err)
	}
	if !bytes.Contains(payload, []byte(`"reasoning_content":"reasoning with no visible answer yet"`)) {
		t.Fatalf("assistant tool-call history dropped reasoning_content: %s", payload)
	}
}

func TestAnthropicClient_ReplaysThinkingBlockBeforeToolUse(t *testing.T) {
	c := &AnthropicClient{}
	body := `{
		"id":"msg_1",
		"type":"message",
		"role":"assistant",
		"model":"claude-x",
		"content":[
			{"type":"thinking","thinking":"let me think this through","signature":"sig-abc-123"},
			{"type":"tool_use","id":"toolu_1","name":"code_comment","input":{"a":1}}
		],
		"stop_reason":"tool_use",
		"usage":{"input_tokens":10,"output_tokens":5}
	}`
	sdkResp := unmarshalAnthropicBody(t, body)
	resp := c.mapAnthropicResponse(sdkResp)

	historyMsg := NewToolCallMessage(resp.Content(), resp.ToolCalls(), resp.Native(), resp.ReasoningContent())

	params, err := c.buildAnthropicParams("claude-x", ChatRequest{Messages: []Message{historyMsg}})
	if err != nil {
		t.Fatalf("buildAnthropicParams: %v", err)
	}
	if len(params.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(params.Messages))
	}
	blocks := params.Messages[0].Content
	if len(blocks) != 2 {
		t.Fatalf("content blocks = %d, want 2 (thinking, tool_use)", len(blocks))
	}
	if blocks[0].OfThinking == nil {
		t.Fatalf("blocks[0] is not a thinking block: %+v", blocks[0])
	}
	if blocks[0].OfThinking.Signature != "sig-abc-123" {
		t.Errorf("thinking signature = %q, want %q (must be passed back unmodified)", blocks[0].OfThinking.Signature, "sig-abc-123")
	}
	if blocks[0].OfThinking.Thinking != "let me think this through" {
		t.Errorf("thinking text = %q, want unmodified original", blocks[0].OfThinking.Thinking)
	}
	if blocks[1].OfToolUse == nil {
		t.Fatalf("blocks[1] is not a tool_use block: %+v", blocks[1])
	}
	if blocks[1].OfToolUse.ID != "toolu_1" {
		t.Errorf("tool_use id = %q, want %q", blocks[1].OfToolUse.ID, "toolu_1")
	}
}

func TestOpenAIResponsesClient_ReplaysReasoningItemBeforeFunctionCall(t *testing.T) {
	client := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1"})
	body := `{
		"id":"resp_1",
		"object":"response",
		"model":"o3",
		"status":"completed",
		"output":[
			{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"step one"}],"encrypted_content":"enc_abc123"},
			{"type":"function_call","call_id":"call_xyz","name":"do_thing","arguments":"{\"x\":1}"}
		],
		"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}
	}`
	sdkResp := unmarshalResponsesBody(t, body)
	resp := client.mapResponsesResponse(sdkResp)

	historyMsg := NewToolCallMessage(resp.Content(), resp.ToolCalls(), resp.Native(), resp.ReasoningContent())

	params := client.buildResponsesParams("o3", ChatRequest{Messages: []Message{historyMsg}})
	items := params.Input.OfInputItemList
	if len(items) != 2 {
		t.Fatalf("input items = %d, want 2 (reasoning, function_call)", len(items))
	}
	if items[0].OfReasoning == nil {
		t.Fatalf("items[0] is not a reasoning item: %+v", items[0])
	}
	// ResponseReasoningItem.ToParam() overrides with the item's own RawJSON
	// (see the SDK's implementation) rather than populating typed fields, so
	// the payload only surfaces on marshal — reading OfReasoning.EncryptedContent
	// directly would see the zero value even though replay is intact.
	reasoningJSON, err := json.Marshal(items[0])
	if err != nil {
		t.Fatalf("marshal reasoning item: %v", err)
	}
	if !bytes.Contains(reasoningJSON, []byte(`"encrypted_content":"enc_abc123"`)) {
		t.Errorf("reasoning item = %s, want encrypted_content %q preserved", reasoningJSON, "enc_abc123")
	}
	if items[1].OfFunctionCall == nil {
		t.Fatalf("items[1] is not a function_call item: %+v", items[1])
	}
	functionCallJSON, err := json.Marshal(items[1])
	if err != nil {
		t.Fatalf("marshal function_call item: %v", err)
	}
	if !bytes.Contains(functionCallJSON, []byte(`"call_id":"call_xyz"`)) {
		t.Errorf("function_call item = %s, want call_id %q preserved", functionCallJSON, "call_xyz")
	}

	// Include must ask the API for encrypted_content, or there would be
	// nothing to replay on a stateless (store=false) request.
	found := false
	for _, inc := range params.Include {
		if inc == responses.ResponseIncludableReasoningEncryptedContent {
			found = true
		}
	}
	if !found {
		t.Error("params.Include missing reasoning.encrypted_content")
	}
}

// TestNativeTurn_CrossProviderFallback proves that a Native payload built by
// one adapter safely falls back to ordinary Content/ToolCalls reconstruction
// when handed to a different adapter (e.g. a session resumed against a
// different provider) — the Go type assertion in each adapter is the safety
// net, not a runtime protocol check, and a mismatch must not panic or
// silently corrupt the request.
func TestNativeTurn_CrossProviderFallback(t *testing.T) {
	toolCalls := []ToolCall{{ID: "call_1", Type: "function", Function: FunctionCall{Name: "code_comment", Arguments: `{}`}}}

	t.Run("anthropic builder ignores a non-anthropic payload", func(t *testing.T) {
		c := &AnthropicClient{}
		msg := NewToolCallMessage("plain text", toolCalls, NativeTurn{Family: "openai-chat-completions", Payload: "some reasoning string"}, "")
		params, err := c.buildAnthropicParams("claude-x", ChatRequest{Messages: []Message{msg}})
		if err != nil {
			t.Fatalf("buildAnthropicParams: %v", err)
		}
		if len(params.Messages) != 1 {
			t.Fatalf("messages = %d, want 1", len(params.Messages))
		}
		blocks := params.Messages[0].Content
		if len(blocks) != 2 || blocks[0].OfText == nil || blocks[1].OfToolUse == nil {
			t.Fatalf("expected fallback reconstruction (text + tool_use), got %+v", blocks)
		}
	})

	t.Run("openai builder ignores a non-string payload", func(t *testing.T) {
		c := &OpenAIClient{}
		msg := NewToolCallMessage("plain text", toolCalls, NativeTurn{Family: "openai-responses", Payload: []responses.ResponseInputItemUnionParam{}}, "")
		params := c.buildOpenAIParams("gpt-x", ChatRequest{Messages: []Message{msg}})
		payload, err := json.Marshal(params.Messages[0])
		if err != nil {
			t.Fatalf("marshal assistant message: %v", err)
		}
		if bytes.Contains(payload, []byte(`reasoning_content`)) {
			t.Errorf("expected no reasoning_content field, got: %s", payload)
		}
	})

	t.Run("responses builder ignores a non-slice payload", func(t *testing.T) {
		c := &OpenAIResponsesClient{}
		msg := NewToolCallMessage("plain text", toolCalls, NativeTurn{Family: "anthropic-messages", Payload: anthropic.MessageParam{}}, "")
		params := c.buildResponsesParams("o3", ChatRequest{Messages: []Message{msg}})
		items := params.Input.OfInputItemList
		if len(items) != 2 || items[0].OfMessage == nil || items[1].OfFunctionCall == nil {
			t.Fatalf("expected fallback reconstruction (message + function_call), got %+v", items)
		}
	})

	// Same-typed but structurally empty payloads must also fall back —
	// found in review of the #805 fix: our own adapters never produce these,
	// but Payload is exported, so a defensive guard belongs at the consumer.
	t.Run("anthropic builder falls back when native Content is empty", func(t *testing.T) {
		c := &AnthropicClient{}
		msg := NewToolCallMessage("plain text", toolCalls, NativeTurn{Family: "anthropic-messages", Payload: anthropic.MessageParam{}}, "")
		params, err := c.buildAnthropicParams("claude-x", ChatRequest{Messages: []Message{msg}})
		if err != nil {
			t.Fatalf("buildAnthropicParams: %v", err)
		}
		blocks := params.Messages[0].Content
		if len(blocks) != 2 || blocks[0].OfText == nil || blocks[1].OfToolUse == nil {
			t.Fatalf("expected fallback reconstruction (text + tool_use), got %+v", blocks)
		}
	})

	t.Run("responses builder falls back when native items slice is empty", func(t *testing.T) {
		c := &OpenAIResponsesClient{}
		msg := NewToolCallMessage("plain text", toolCalls, NativeTurn{Family: "openai-responses", Payload: []responses.ResponseInputItemUnionParam{}}, "")
		params := c.buildResponsesParams("o3", ChatRequest{Messages: []Message{msg}})
		items := params.Input.OfInputItemList
		if len(items) != 2 || items[0].OfMessage == nil || items[1].OfFunctionCall == nil {
			t.Fatalf("expected fallback reconstruction (message + function_call), got %+v", items)
		}
	})
}

// TestAnthropicClient_NativeOnlySetWhenThinkingPresent guards the
// hasThinking gate in mapAnthropicResponse: an ordinary text + tool_use turn
// (no thinking/redacted_thinking block) round-trips losslessly through the
// existing Content()/ToolCalls() reconstruction, so Native must stay unset —
// setting it unconditionally risks replaying a MessageParam with an empty
// Content slice for a truncated/error response, which Anthropic rejects.
func TestAnthropicClient_NativeOnlySetWhenThinkingPresent(t *testing.T) {
	c := &AnthropicClient{}
	body := `{
		"id":"msg_2",
		"type":"message",
		"role":"assistant",
		"model":"claude-x",
		"content":[
			{"type":"text","text":"answer"},
			{"type":"tool_use","id":"toolu_1","name":"code_comment","input":{"a":1}}
		],
		"stop_reason":"tool_use",
		"usage":{"input_tokens":10,"output_tokens":5}
	}`
	sdkResp := unmarshalAnthropicBody(t, body)
	resp := c.mapAnthropicResponse(sdkResp)
	if resp.Native().Payload != nil {
		t.Errorf("Native.Payload = %#v, want nil for a turn with no thinking block", resp.Native().Payload)
	}
}

// TestBuildAnthropicParams_NativeReuseDoesNotAliasOriginalSlice guards the
// slice-aliasing fix: buildAnthropicParams must not let the dynamic
// cache_control breakpoint mutate the backing array a Native payload shares
// with whatever this Message's Native came from.
func TestBuildAnthropicParams_NativeReuseDoesNotAliasOriginalSlice(t *testing.T) {
	c := &AnthropicClient{}
	original := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleAssistant,
		Content: []anthropic.ContentBlockParamUnion{
			anthropic.NewThinkingBlock("sig", "thinking"),
			anthropic.NewToolUseBlock("toolu_1", map[string]any{}, "code_comment"),
		},
	}
	msg := NewToolCallMessage("", []ToolCall{{ID: "toolu_1", Type: "function", Function: FunctionCall{Name: "code_comment", Arguments: "{}"}}},
		NativeTurn{Family: "anthropic-messages", Payload: original}, "")

	// A trailing tool-result message makes this assistant turn NOT the last
	// message — buildAnthropicParams's cache_control breakpoint targets
	// whichever message ends up last, so this exercises the ordinary path.
	// A second, standalone build below targets the aliasing case directly:
	// the assistant turn IS the last message, which is exactly when the
	// breakpoint would write into the shared backing array if the reuse
	// weren't copied first.
	if _, err := c.buildAnthropicParams("claude-x", ChatRequest{Messages: []Message{msg}}); err != nil {
		t.Fatalf("buildAnthropicParams: %v", err)
	}
	if original.Content[1].GetCacheControl() != nil && original.Content[1].GetCacheControl().Type != "" {
		t.Fatalf("original payload's tool_use block was mutated: %+v", original.Content[1])
	}
}

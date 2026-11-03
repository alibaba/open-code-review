// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// These tests are the deterministic, no-live-model reproductions for issue
// #1357. Gemini 3 models reached through an OpenAI-compatible endpoint attach a
// thought signature to each tool call as tool_calls[].extra_content, and reject
// the next request unless it is echoed back unchanged. Each test exercises the
// real mapOpenAIResponse -> NewToolCallMessage -> buildOpenAIParams path rather
// than a hand-rolled approximation of it.

const vertexThoughtSignature = `"extra_content":{"google":{"thought_signature":"sig-abc-123"}}`

func TestOpenAIChatCompletions_ReplaysToolCallExtraContentAcrossTurns(t *testing.T) {
	client := NewOpenAIClient(ClientConfig{URL: "https://aiplatform.googleapis.com/v1/projects/p/locations/global/endpoints/openapi"})
	body := `{
		"id":"chatcmpl_1",
		"object":"chat.completion",
		"model":"google/gemini-3.8-flash",
		"choices":[{
			"index":0,
			"message":{
				"role":"assistant",
				"content":null,
				"tool_calls":[{
					"id":"call_1",
					"type":"function",
					"function":{"name":"file_read","arguments":"{}"},
					"extra_content":{"google":{"thought_signature":"sig-abc-123"}}
				}]
			},
			"finish_reason":"tool_calls"
		}]
	}`
	sdkResp := unmarshalChatCompletionBody(t, body)
	resp := client.mapOpenAIResponse(sdkResp)

	historyMsg := NewToolCallMessage(resp.Content(), resp.ToolCalls(), resp.Native(), resp.ReasoningContent())

	params := client.buildOpenAIParams("google/gemini-3.8-flash", ChatRequest{Messages: []Message{historyMsg}})
	if len(params.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(params.Messages))
	}
	payload, err := json.Marshal(params.Messages[0])
	if err != nil {
		t.Fatalf("marshal assistant message: %v", err)
	}
	if !bytes.Contains(payload, []byte(vertexThoughtSignature)) {
		t.Fatalf("assistant tool-call history dropped extra_content: %s", payload)
	}
}

// replayStreamedToolCall runs one streamed completion against a fixture server
// and returns the assistant history message rebuilt for the next request.
func replayStreamedToolCall(t *testing.T, events ...string) []byte {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOpenAISSE(t, w, events...)
	}))
	defer server.Close()

	client := NewOpenAIClient(ClientConfig{
		URL:       server.URL + "/v1",
		APIKey:    "test-key",
		Model:     "google/gemini-3.8-flash",
		ExtraBody: map[string]any{"stream": true},
	})

	resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}

	historyMsg := NewToolCallMessage(resp.Content(), resp.ToolCalls(), resp.Native(), resp.ReasoningContent())
	params := client.buildOpenAIParams("google/gemini-3.8-flash", ChatRequest{Messages: []Message{historyMsg}})
	if len(params.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(params.Messages))
	}
	payload, err := json.Marshal(params.Messages[0])
	if err != nil {
		t.Fatalf("marshal assistant message: %v", err)
	}
	return payload
}

// TestOpenAIChatCompletions_ReplaysToolCallExtraContentFromStream covers the
// streaming path, where ChatCompletionAccumulator synthesizes its result field
// by field and never sets raw JSON — so the value is unreachable after
// accumulation and must be captured from the deltas.
//
// None of these fixtures carry reasoning_content, which is the real Gemini
// shape: a capture placed after the reasoning_content lookup's `continue` would
// never run, and every subtest here would fail.
func TestOpenAIChatCompletions_ReplaysToolCallExtraContentFromStream(t *testing.T) {
	tests := []struct {
		name   string
		events []string
	}{
		{
			name: "metadata on the chunk that opens the call",
			events: []string{
				`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"google/gemini-3.8-flash","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"file_read","arguments":""},"extra_content":{"google":{"thought_signature":"sig-abc-123"}}}]},"finish_reason":null}]}`,
				`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"google/gemini-3.8-flash","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]},"finish_reason":null}]}`,
				`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"google/gemini-3.8-flash","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			},
		},
		{
			name: "metadata on a later chunk",
			events: []string{
				`{"id":"c2","object":"chat.completion.chunk","created":1,"model":"google/gemini-3.8-flash","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"file_read","arguments":""}}]},"finish_reason":null}]}`,
				`{"id":"c2","object":"chat.completion.chunk","created":1,"model":"google/gemini-3.8-flash","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"},"extra_content":{"google":{"thought_signature":"sig-abc-123"}}}]},"finish_reason":null}]}`,
				`{"id":"c2","object":"chat.completion.chunk","created":1,"model":"google/gemini-3.8-flash","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			},
		},
		{
			name: "negative delta index, as some gateways send for a single call",
			events: []string{
				`{"id":"c3","object":"chat.completion.chunk","created":1,"model":"google/gemini-3.8-flash","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":-1,"id":"call_1","type":"function","function":{"name":"file_read","arguments":"{}"},"extra_content":{"google":{"thought_signature":"sig-abc-123"}}}]},"finish_reason":null}]}`,
				`{"id":"c3","object":"chat.completion.chunk","created":1,"model":"google/gemini-3.8-flash","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := replayStreamedToolCall(t, tt.events...)
			if !bytes.Contains(payload, []byte(vertexThoughtSignature)) {
				t.Fatalf("streamed tool-call history dropped extra_content: %s", payload)
			}
		})
	}
}

// TestOpenAIChatCompletions_ToolCallExtraContentOmittedWhenAbsent guards the
// wire format for every provider that never sends the field.
func TestOpenAIChatCompletions_ToolCallExtraContentOmittedWhenAbsent(t *testing.T) {
	client := NewOpenAIClient(ClientConfig{URL: "https://api.openai.com/v1"})
	body := `{
		"id":"chatcmpl_1",
		"object":"chat.completion",
		"model":"gpt-5.5",
		"choices":[{
			"index":0,
			"message":{
				"role":"assistant",
				"content":null,
				"tool_calls":[{"id":"call_1","type":"function","function":{"name":"file_read","arguments":"{}"}}]
			},
			"finish_reason":"tool_calls"
		}]
	}`
	resp := client.mapOpenAIResponse(unmarshalChatCompletionBody(t, body))
	historyMsg := NewToolCallMessage(resp.Content(), resp.ToolCalls(), resp.Native(), resp.ReasoningContent())
	params := client.buildOpenAIParams("gpt-5.5", ChatRequest{Messages: []Message{historyMsg}})
	payload, err := json.Marshal(params.Messages[0])
	if err != nil {
		t.Fatalf("marshal assistant message: %v", err)
	}
	if bytes.Contains(payload, []byte("extra_content")) {
		t.Fatalf("extra_content leaked into a response that carried none: %s", payload)
	}
}

// TestOpenAIChatCompletions_StreamedExtraContentSurvivesLaterChunks guards the
// merge rule: a later chunk carrying no metadata must not erase what an earlier
// one captured, and a repeated value must not be concatenated.
func TestOpenAIChatCompletions_StreamedExtraContentSurvivesLaterChunks(t *testing.T) {
	payload := replayStreamedToolCall(t,
		`{"id":"c4","object":"chat.completion.chunk","created":1,"model":"google/gemini-3.8-flash","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"file_read","arguments":""},"extra_content":{"google":{"thought_signature":"sig-abc-123"}}}]},"finish_reason":null}]}`,
		`{"id":"c4","object":"chat.completion.chunk","created":1,"model":"google/gemini-3.8-flash","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]},"finish_reason":null}]}`,
		`{"id":"c4","object":"chat.completion.chunk","created":1,"model":"google/gemini-3.8-flash","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
	)
	if got := bytes.Count(payload, []byte("thought_signature")); got != 1 {
		t.Fatalf("thought_signature appears %d times, want 1: %s", got, payload)
	}
}

// TestOpenAIChatCompletions_StreamedExtraContentMapsToMatchingToolCall guards
// that interleaved tool calls each keep their own metadata.
func TestOpenAIChatCompletions_StreamedExtraContentMapsToMatchingToolCall(t *testing.T) {
	payload := replayStreamedToolCall(t,
		`{"id":"c5","object":"chat.completion.chunk","created":1,"model":"google/gemini-3.8-flash","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"file_read","arguments":"{}"},"extra_content":{"google":{"thought_signature":"sig-first"}}}]},"finish_reason":null}]}`,
		`{"id":"c5","object":"chat.completion.chunk","created":1,"model":"google/gemini-3.8-flash","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"code_search","arguments":"{}"},"extra_content":{"google":{"thought_signature":"sig-second"}}}]},"finish_reason":null}]}`,
		`{"id":"c5","object":"chat.completion.chunk","created":1,"model":"google/gemini-3.8-flash","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
	)
	first := bytes.Index(payload, []byte("sig-first"))
	second := bytes.Index(payload, []byte("sig-second"))
	if first < 0 || second < 0 {
		t.Fatalf("a signature was dropped: %s", payload)
	}
	if first > second {
		t.Fatalf("signatures attached to the wrong calls: %s", payload)
	}
}

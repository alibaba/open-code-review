// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"encoding/json"
	"strings"
	"testing"

	openai "github.com/openai/openai-go/v3"
)

func decodeReplayEnvelope(t *testing.T, envelope json.RawMessage) map[string]any {
	t.Helper()
	object, err := decodeOpenAIObject(string(envelope))
	if err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return object
}

// TestOpenAIReplayEnvelopeSurvivesMissingToolCallType covers gateways that
// omit the tool call "type" field (some Ollama/vLLM builds and proxies do):
// the SDK's typed request conversion yields a null union there, and the
// envelope must rebuild the item from the provider's own response JSON
// instead of replaying tool_calls:[null].
func TestOpenAIReplayEnvelopeSurvivesMissingToolCallType(t *testing.T) {
	raw := `{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","function":{"name":"file_read","arguments":"{\"path\":\"main.go\"}"}}]}`
	var message openai.ChatCompletionMessage
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		t.Fatal(err)
	}

	envelope, err := openAIReplayMessageFromResponse(message)
	if err != nil {
		t.Fatal(err)
	}
	object := decodeReplayEnvelope(t, envelope)
	items, _ := object["tool_calls"].([]any)
	if len(items) != 1 {
		t.Fatalf("tool_calls = %v, want 1 item", object["tool_calls"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("tool_calls[0] is not an object: %s", envelope)
	}
	if item["id"] != "call_1" || item["type"] != "function" {
		t.Fatalf("tool call must keep id and default the type: %s", envelope)
	}
	function, _ := item["function"].(map[string]any)
	if function["name"] != "file_read" || function["arguments"] != `{"path":"main.go"}` {
		t.Fatalf("function payload lost: %s", envelope)
	}
}

// TestNewOpenAIReplayRejectsCorruptToolCalls is the backstop for the same
// class of failure: an envelope whose tool_calls cannot be replayed verbatim
// must be discarded so the request falls back to the normalized rebuild,
// never sent broken.
func TestNewOpenAIReplayRejectsCorruptToolCalls(t *testing.T) {
	corrupt := json.RawMessage(`{"role":"assistant","content":null,"tool_calls":[null]}`)
	if replay := newOpenAIReplay(corrupt); replay != nil {
		t.Fatalf("corrupt envelope must be rejected, got %#v", replay)
	}
	missingType := json.RawMessage(`{"role":"assistant","content":null,"tool_calls":[{"id":"call_1"}]}`)
	if replay := newOpenAIReplay(missingType); replay != nil {
		t.Fatalf("envelope with untyped tool call must be rejected, got %#v", replay)
	}
}

// TestOpenAIReplayExtensionPolicy pins the extension policy: fields with a
// known request-side replay contract survive (reasoning_content,
// reasoning_details), while unknown vendor fields are never echoed back.
func TestOpenAIReplayExtensionPolicy(t *testing.T) {
	raw := `{"role":"assistant","content":"visible","reasoning_content":"hidden chain","reasoning_details":[{"type":"reasoning.text","text":"chain"}],"some_vendor_field":{"a":1}}`
	var message openai.ChatCompletionMessage
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		t.Fatal(err)
	}

	envelope, err := openAIReplayMessageFromResponse(message)
	if err != nil {
		t.Fatal(err)
	}
	object := decodeReplayEnvelope(t, envelope)
	if object["reasoning_content"] != "hidden chain" {
		t.Fatalf("reasoning_content must replay: %s", envelope)
	}
	if _, ok := object["reasoning_details"].([]any); !ok {
		t.Fatalf("reasoning_details must survive replay: %s", envelope)
	}
	if strings.Contains(string(envelope), "some_vendor_field") {
		t.Fatalf("unknown vendor fields must not be echoed on replay: %s", envelope)
	}
}

// TestOpenAIReplayKeepsReasoningForToolTurns pins the DeepSeek thinking-mode
// contract: reasoning_content on a tool-call turn must be passed back to the
// API verbatim (the API returns 400 when it is missing), so the envelope may
// never strip it.
func TestOpenAIReplayKeepsReasoningForToolTurns(t *testing.T) {
	raw := `{"role":"assistant","content":"","reasoning_content":"tool-turn chain","tool_calls":[{"id":"call-1","type":"function","function":{"name":"file_read","arguments":"{}"}}]}`
	var message openai.ChatCompletionMessage
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		t.Fatal(err)
	}
	envelope, err := openAIReplayMessageFromResponse(message)
	if err != nil {
		t.Fatal(err)
	}
	object := decodeReplayEnvelope(t, envelope)
	if object["reasoning_content"] != "tool-turn chain" {
		t.Fatalf("tool-turn reasoning_content must be passed back verbatim: %s", envelope)
	}
	if _, present := object["tool_calls"]; !present {
		t.Fatalf("tool calls must survive alongside reasoning: %s", envelope)
	}
}

// TestAssistantTurnFallbackSubstitutesReasoningForEmptyContent pins parity
// with the pre-turn history rebuild for custom LLMClient implementations:
// resp.Content() substituted reasoning for empty visible content, so the
// fallback turn must too — otherwise reasoning-only turns become IsEmpty and
// vanish from the retry path.
func TestAssistantTurnFallbackSubstitutesReasoningForEmptyContent(t *testing.T) {
	empty := ""
	resp := &ChatResponse{Choices: []Choice{{Message: ResponseMessage{
		Content:          &empty,
		ReasoningContent: "hidden plan",
	}}}}
	turn := resp.AssistantTurn()
	if turn.Content() != "hidden plan" {
		t.Fatalf("Content() = %q, want reasoning fallback", turn.Content())
	}
	if turn.IsEmpty() {
		t.Fatal("a reasoning-only turn must not be IsEmpty")
	}
}

// TestOpenAIAssistantTurnStripsThinkTagsFromNormalizedContent pins the
// normalization contract on the adapter path: the normalized view strips
// think tags exactly like the fallback path, while the opaque envelope keeps
// the provider's raw text for replay.
func TestOpenAIAssistantTurnStripsThinkTagsFromNormalizedContent(t *testing.T) {
	content := "<think>hidden</think>\nanswer"
	resp := &ChatResponse{Choices: []Choice{{Message: ResponseMessage{Content: &content}}}}
	raw := json.RawMessage(`{"role":"assistant","content":"<think>hidden</think>\nanswer"}`)
	turn := openAIAssistantTurnFromResponse(resp, raw)
	// Identical normalization to the generic fallback and resp.Content():
	// tag markers removed, text trimmed.
	if want := strings.TrimSpace(stripThinkTags(content)); turn.Content() != want {
		t.Fatalf("normalized content = %q, want %q (fallback-path normalization)", turn.Content(), want)
	}
	replay, ok := turn.replay.(openAIReplay)
	if !ok {
		t.Fatal("expected an opaque replay envelope")
	}
	if !strings.Contains(string(replay.assistantMessage), "<think>hidden</think>") {
		t.Fatalf("envelope must keep the provider's raw text: %s", replay.assistantMessage)
	}
}

// TestOpenAIAssistantTurnWithoutEnvelopeFallsBackToReasoning covers the
// adapter path when no envelope survives (or the feature is disabled): the
// turn behaves exactly like the pre-envelope history rebuild.
func TestOpenAIAssistantTurnWithoutEnvelopeFallsBackToReasoning(t *testing.T) {
	empty := ""
	resp := &ChatResponse{Choices: []Choice{{Message: ResponseMessage{
		Content:          &empty,
		ReasoningContent: "hidden plan",
	}}}}
	turn := openAIAssistantTurnFromResponse(resp, nil)
	if turn.replay != nil {
		t.Fatalf("no envelope expected, got %#v", turn.replay)
	}
	if turn.Content() != "hidden plan" {
		t.Fatalf("Content() = %q, want reasoning fallback without envelope", turn.Content())
	}
}

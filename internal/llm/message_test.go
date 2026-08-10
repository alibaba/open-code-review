// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewTextMessage(t *testing.T) {
	m := NewTextMessage("user", "hello")
	if m.Role != "user" {
		t.Errorf("Role = %q, want user", m.Role)
	}
	if m.Content != "hello" {
		t.Errorf("Content = %v, want hello", m.Content)
	}
	if m.ToolCallID != "" {
		t.Error("ToolCallID should be empty")
	}
	if len(m.ToolCalls) != 0 {
		t.Errorf("ToolCalls should be nil, got %v", m.ToolCalls)
	}
}

func TestNewToolCallMessage(t *testing.T) {
	calls := []ToolCall{
		{ID: "c1", Type: "function", Function: FunctionCall{Name: "tool_a", Arguments: `{}`}},
		{ID: "c2", Type: "function", Function: FunctionCall{Name: "tool_b", Arguments: `{"x":1}`}},
	}
	m := NewToolCallMessage("thinking", calls)
	if m.Role != "assistant" {
		t.Errorf("Role = %q, want assistant", m.Role)
	}
	if m.Content != "thinking" {
		t.Errorf("Content = %v, want thinking", m.Content)
	}
	if len(m.ToolCalls) != 2 {
		t.Fatalf("ToolCalls len = %d, want 2", len(m.ToolCalls))
	}
	if m.ToolCalls[0].ID != "c1" || m.ToolCalls[1].Function.Name != "tool_b" {
		t.Errorf("ToolCalls not copied correctly")
	}

	// Mutation of original must not affect the message.
	calls[0].ID = "mutated"
	if m.ToolCalls[0].ID == "mutated" {
		t.Error("NewToolCallMessage must copy ToolCalls")
	}
}

func TestNewToolCallMessage_NilCalls(t *testing.T) {
	m := NewToolCallMessage("text", nil)
	if m.ToolCalls != nil {
		t.Errorf("expected nil ToolCalls for nil input, got %v", m.ToolCalls)
	}
}

func TestAssistantTurnMessageKeepsReplayStateOutOfNormalizedJSON(t *testing.T) {
	empty := ""
	resp := &ChatResponse{Choices: []Choice{{Message: ResponseMessage{
		Content:   &empty,
		ToolCalls: []ToolCall{{ID: "call-1", Type: "function", Function: FunctionCall{Name: "file_read", Arguments: `{}`}}},
	}}}}
	rawMessage := json.RawMessage(`{"role":"assistant","content":null,"reasoning_details":[{"type":"reasoning.text","text":"private reasoning"}],"tool_calls":[{"id":"call-1","type":"function","function":{"name":"file_read","arguments":"{}"}}]}`)
	resp.turn = openAIAssistantTurnFromResponse(resp, rawMessage)

	turn := resp.AssistantTurn()
	if got := turn.Content(); got != "" {
		t.Fatalf("AssistantTurn.Content() = %q, want empty visible content", got)
	}
	history, err := json.Marshal(turn.Message())
	if err != nil {
		t.Fatalf("marshal normalized history: %v", err)
	}
	if bytes.Contains(history, []byte("reasoning_details")) {
		t.Fatalf("normalized history exposed provider replay state: %s", history)
	}

	params := (&OpenAIClient{}).buildOpenAIParams("fake", ChatRequest{Messages: []Message{turn.Message()}})
	wire, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal OpenAI params: %v", err)
	}
	if !bytes.Contains(wire, []byte(`"reasoning_details"`)) {
		t.Fatalf("OpenAI replay omitted reasoning_details: %s", wire)
	}
}

func TestMessageApproxTokenCountIncludesOpaqueReplay(t *testing.T) {
	visible := "visible"
	resp := &ChatResponse{Choices: []Choice{{Message: ResponseMessage{
		Content: &visible,
	}}}}
	rawMessage := json.RawMessage(`{"role":"assistant","content":"visible","reasoning_details":[{"type":"reasoning.text","text":"private reasoning private reasoning"}]}`)
	message := openAIAssistantTurnFromResponse(resp, rawMessage).Message()
	if got, want := message.ApproxTokenCount(), CountTokens(visible); got <= want {
		t.Fatalf("replay-aware token estimate = %d, want greater than visible-only estimate %d", got, want)
	}
}

func TestChatResponseApproxCompletionTokenCountIncludesOpaqueReplay(t *testing.T) {
	visible := "visible"
	resp := &ChatResponse{Choices: []Choice{{Message: ResponseMessage{
		Content:   &visible,
		ToolCalls: []ToolCall{{ID: "call-1", Type: "function", Function: FunctionCall{Name: "file_read", Arguments: `{}`}}},
	}}}}
	rawMessage := json.RawMessage(`{"role":"assistant","content":"visible","reasoning_content":"private reasoning private reasoning","vendor_state":{"nonce":"opaque"},"tool_calls":[{"id":"call-1","type":"function","function":{"name":"file_read","arguments":"{}"}}]}`)
	resp.turn = openAIAssistantTurnFromResponse(resp, rawMessage)

	if got, visibleOnly := resp.ApproxCompletionTokenCount(), CountTokens(visible); got <= visibleOnly {
		t.Fatalf("completion token estimate = %d, want greater than visible-only estimate %d", got, visibleOnly)
	}
}

func TestCloneMessagesRetainsAdapterReplayAndIsolatesNormalizedState(t *testing.T) {
	visible := "original visible"
	response := &ChatResponse{Choices: []Choice{{Message: ResponseMessage{
		Content:          &visible,
		ReasoningContent: "private reasoning",
		ToolCalls:        []ToolCall{{ID: "call-1", Type: "function", Function: FunctionCall{Name: "file_read", Arguments: `{}`}}},
	}}}}
	rawMessage := json.RawMessage(`{"role":"assistant","content":"original visible","reasoning_content":"private reasoning","vendor_state":{"nonce":"opaque"},"tool_calls":[{"id":"call-1","type":"function","function":{"name":"file_read","arguments":"{}"}}]}`)
	original := []Message{openAIAssistantTurnFromResponse(response, rawMessage).Message()}

	cloned := CloneMessages(original)
	cloned[0].Content = "mutated normalized content"
	cloned[0].ToolCalls[0].ID = "mutated-call"
	if original[0].ToolCalls[0].ID != "call-1" {
		t.Fatalf("CloneMessages aliased normalized tool calls: %+v", original[0].ToolCalls)
	}

	params := (&OpenAIClient{}).buildOpenAIParams("fake", ChatRequest{Messages: cloned})
	wire, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal replayed clone: %v", err)
	}
	if !bytes.Contains(wire, []byte(`"content":"original visible"`)) ||
		!bytes.Contains(wire, []byte(`"id":"call-1"`)) ||
		!bytes.Contains(wire, []byte(`"vendor_state":{"nonce":"opaque"}`)) {
		t.Fatalf("normalized mutation changed adapter-owned replay: %s", wire)
	}
}

func TestApproxMessagesTokenCountIncludesReplay(t *testing.T) {
	visible := "visible"
	response := &ChatResponse{Choices: []Choice{{Message: ResponseMessage{
		Content: &visible,
	}}}}
	rawMessage := json.RawMessage(`{"role":"assistant","content":"visible","reasoning_details":[{"type":"reasoning.text","text":"private reasoning"}]}`)
	message := openAIAssistantTurnFromResponse(response, rawMessage).Message()
	if got, visibleOnly := ApproxMessagesTokenCount([]Message{message}), CountTokens(visible); got <= visibleOnly {
		t.Fatalf("conversation token estimate = %d, want greater than visible-only estimate %d", got, visibleOnly)
	}
	if got := ApproxMessagesTokenCount(nil); got != 0 {
		t.Fatalf("empty conversation token estimate = %d, want 0", got)
	}
}

func TestChatResponseAssistantTurnFallbackDoesNotInventOpenAIReplay(t *testing.T) {
	empty := ""
	resp := &ChatResponse{Choices: []Choice{{Message: ResponseMessage{
		Content:          &empty,
		ReasoningContent: "provider-native reasoning",
	}}}}
	turn := resp.AssistantTurn()
	if turn.replay != nil {
		t.Fatalf("generic response fallback invented provider replay state: %+v", turn)
	}
	// The reasoning text itself still rides in normalized content, matching
	// the pre-turn resp.Content() history rebuild.
	if turn.Content() != "provider-native reasoning" {
		t.Fatalf("Content() = %q, want reasoning substitution without an envelope", turn.Content())
	}
}

func TestNewToolResultMessage(t *testing.T) {
	m := NewToolResultMessage("call-123", "result text")
	if m.Role != "tool" {
		t.Errorf("Role = %q, want tool", m.Role)
	}
	if m.Content != "result text" {
		t.Errorf("Content = %v, want result text", m.Content)
	}
	if m.ToolCallID != "call-123" {
		t.Errorf("ToolCallID = %q, want call-123", m.ToolCallID)
	}
}

func TestExtractText_String(t *testing.T) {
	m := Message{Role: "user", Content: "plain text"}
	if got := m.ExtractText(); got != "plain text" {
		t.Errorf("ExtractText() = %q, want plain text", got)
	}
}

func TestExtractText_ContentBlocks(t *testing.T) {
	m := Message{Role: "assistant", Content: []ContentBlock{
		{Type: "text", Text: "part1"},
		{Type: "text", Text: " part2"},
	}}
	if got := m.ExtractText(); got != "part1 part2" {
		t.Errorf("ExtractText() = %q, want 'part1 part2'", got)
	}
}

func TestExtractText_NestedContentBlocks(t *testing.T) {
	m := Message{Role: "tool", Content: []ContentBlock{
		{
			Type: "tool_result",
			Content: []ContentBlock{
				{Type: "text", Text: "inner1"},
				{Type: "text", Text: "inner2"},
			},
		},
		{Type: "text", Text: "outer"},
	}}
	got := m.ExtractText()
	if got != "inner1inner2outer" {
		t.Errorf("ExtractText() = %q, want inner1inner2outer", got)
	}
}

func TestExtractText_Default(t *testing.T) {
	m := Message{Role: "user", Content: 42}
	if got := m.ExtractText(); got != "" {
		t.Errorf("ExtractText() for non-string/non-block = %q, want empty", got)
	}
}

func TestExtractText_NilContent(t *testing.T) {
	m := Message{Role: "user", Content: nil}
	if got := m.ExtractText(); got != "" {
		t.Errorf("ExtractText() for nil = %q, want empty", got)
	}
}

func TestChatResponse_Content(t *testing.T) {
	text := "hello world"
	resp := &ChatResponse{
		Choices: []Choice{{
			Message: ResponseMessage{Content: &text},
		}},
	}
	if got := resp.Content(); got != "hello world" {
		t.Errorf("Content() = %q, want hello world", got)
	}
}

func TestChatResponse_Content_Empty(t *testing.T) {
	resp := &ChatResponse{}
	if got := resp.Content(); got != "" {
		t.Errorf("Content() with no choices = %q, want empty", got)
	}
}

func TestChatResponse_Content_FallbackToReasoning(t *testing.T) {
	empty := ""
	resp := &ChatResponse{
		Choices: []Choice{{
			Message: ResponseMessage{Content: &empty, ReasoningContent: "reasoning here"},
		}},
	}
	if got := resp.Content(); got != "reasoning here" {
		t.Errorf("Content() = %q, want reasoning here", got)
	}
}

func TestChatResponse_Content_NilContent(t *testing.T) {
	resp := &ChatResponse{
		Choices: []Choice{{
			Message: ResponseMessage{Content: nil, ReasoningContent: "fallback"},
		}},
	}
	if got := resp.Content(); got != "fallback" {
		t.Errorf("Content() = %q, want fallback", got)
	}
}

func TestChatResponse_Content_StripsThinkTags(t *testing.T) {
	text := "<think>internal</think>answer"
	resp := &ChatResponse{
		Choices: []Choice{{
			Message: ResponseMessage{Content: &text},
		}},
	}
	if got := resp.Content(); got != "internalanswer" {
		t.Errorf("Content() = %q, want internalanswer", got)
	}
}

func TestChatResponse_ToolCalls(t *testing.T) {
	resp := &ChatResponse{
		Choices: []Choice{{
			Message: ResponseMessage{
				ToolCalls: []ToolCall{
					{ID: "c1", Type: "function", Function: FunctionCall{Name: "tool_a"}},
					{ID: "c2", Type: "function", Function: FunctionCall{Name: "tool_b"}},
				},
			},
		}},
	}
	calls := resp.ToolCalls()
	if len(calls) != 2 {
		t.Fatalf("ToolCalls() len = %d, want 2", len(calls))
	}
	if calls[0].Function.Name != "tool_a" || calls[1].Function.Name != "tool_b" {
		t.Error("ToolCalls returned unexpected values")
	}
}

func TestChatResponse_ToolCalls_Empty(t *testing.T) {
	resp := &ChatResponse{}
	if got := resp.ToolCalls(); got != nil {
		t.Errorf("ToolCalls() with no choices = %v, want nil", got)
	}
}

func TestParseShellRC(t *testing.T) {
	tmp := t.TempDir()
	rcPath := filepath.Join(tmp, ".zshrc")

	content := `# some comment
export PATH="/usr/bin:$PATH"
export ANTHROPIC_BASE_URL="https://api.example.com"
export ANTHROPIC_AUTH_TOKEN='sk-test-token'
export ANTHROPIC_MODEL=claude-sonnet-4-20250514
`
	if err := os.WriteFile(rcPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ep, ok, err := parseShellRC(rcPath, "")
	if err != nil {
		t.Fatalf("parseShellRC: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if ep.Token != "sk-test-token" {
		t.Errorf("Token = %q, want sk-test-token", ep.Token)
	}
	if ep.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q", ep.Model)
	}
	if ep.Protocol != "anthropic" {
		t.Errorf("Protocol = %q, want anthropic", ep.Protocol)
	}
	if ep.AuthHeader != "authorization" {
		t.Errorf("AuthHeader = %q, want authorization", ep.AuthHeader)
	}
	if ep.Source != "Shell rc file" {
		t.Errorf("Source = %q", ep.Source)
	}
}

func TestParseShellRC_ModelOverride(t *testing.T) {
	tmp := t.TempDir()
	rcPath := filepath.Join(tmp, ".bashrc")

	content := `export ANTHROPIC_BASE_URL="https://api.example.com"
export ANTHROPIC_AUTH_TOKEN="token"
export ANTHROPIC_MODEL=claude-3-opus
`
	if err := os.WriteFile(rcPath, []byte(content), 0644); err != nil {
		t.Fatalf("write rc: %v", err)
	}

	ep, ok, err := parseShellRC(rcPath, "override-model")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if ep.Model != "override-model" {
		t.Errorf("Model = %q, want override-model", ep.Model)
	}
}

func TestParseShellRC_Incomplete(t *testing.T) {
	tmp := t.TempDir()
	rcPath := filepath.Join(tmp, ".zshrc")

	content := `export ANTHROPIC_BASE_URL="https://api.example.com"
export ANTHROPIC_AUTH_TOKEN="token"
# missing ANTHROPIC_MODEL
`
	if err := os.WriteFile(rcPath, []byte(content), 0644); err != nil {
		t.Fatalf("write rc: %v", err)
	}

	_, ok, err := parseShellRC(rcPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected ok=false when model is missing")
	}
}

func TestParseShellRC_NonexistentFile(t *testing.T) {
	_, ok, err := parseShellRC("/nonexistent/path/.zshrc", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected ok=false for missing file")
	}
}

func TestModelListContains(t *testing.T) {
	models := []string{"gpt-4", " claude-3-opus ", "gemini-pro"}
	if !ModelListContains(models, "claude-3-opus") {
		t.Error("expected true for claude-3-opus")
	}
	if !ModelListContains(models, "gpt-4") {
		t.Error("expected true for gpt-4")
	}
	if ModelListContains(models, "gpt-3.5") {
		t.Error("expected false for gpt-3.5")
	}
	if ModelListContains(nil, "anything") {
		t.Error("expected false for nil list")
	}
}

func TestDefaultAuthHeader(t *testing.T) {
	if got := defaultAuthHeader("anthropic"); got != "authorization" {
		t.Errorf("anthropic: got %q, want authorization", got)
	}
	if got := defaultAuthHeader("openai"); got != "" {
		t.Errorf("openai: got %q, want empty", got)
	}
	if got := defaultAuthHeader(""); got != "" {
		t.Errorf("empty: got %q, want empty", got)
	}
}

func TestUserAgent(t *testing.T) {
	got := userAgent("anthropic")
	if got != "open-code-review/dev | anthropic" {
		t.Errorf("userAgent(anthropic) = %q", got)
	}
	got2 := userAgent("")
	if got2 != "open-code-review/dev" {
		t.Errorf("userAgent('') = %q", got2)
	}
}

func TestCloneMessagesIsolatesContentBlocks(t *testing.T) {
	original := []Message{{
		Role: "user",
		Content: []ContentBlock{{
			Type:      "tool_result",
			ToolUseID: "call-1",
			Content:   []ContentBlock{{Type: "text", Text: "original nested"}},
		}},
	}}

	cloned := CloneMessages(original)
	blocks, ok := cloned[0].Content.([]ContentBlock)
	if !ok {
		t.Fatalf("cloned content type = %T, want []ContentBlock", cloned[0].Content)
	}
	blocks[0].ToolUseID = "mutated-call"
	blocks[0].Content[0].Text = "mutated nested"

	got := original[0].Content.([]ContentBlock)
	if got[0].ToolUseID != "call-1" || got[0].Content[0].Text != "original nested" {
		t.Fatalf("CloneMessages aliased content blocks: %+v", got)
	}
}

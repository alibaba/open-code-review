// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestSensitiveNativeTranscriptPreservesLiveReplay(t *testing.T) {
	const secret = "native-credential-sentinel"
	native := llm.NativeTurn{Family: "anthropic", Payload: map[string]any{"input": secret}}
	extra := json.RawMessage(`{"google":{"thought_signature":"` + secret + `"}}`)
	calls := []llm.ToolCall{
		{ID: "sensitive", Function: llm.FunctionCall{Name: "mcp__fixture__probe", Arguments: `{"token":"` + secret + `"}`}, ExtraContent: extra},
		{ID: "ordinary", Function: llm.FunctionCall{Name: "file_read", Arguments: `{}`}, ExtraContent: extra},
	}
	message := llm.NewToolCallMessage(secret, calls, native, secret)
	sh := New(t.TempDir(), "main", "test", SessionOptions{})
	fs := sh.GetOrCreateFileSession("main.go")
	sanitize := func(name, arguments string) string {
		if name == "mcp__fixture__probe" {
			return `{"redacted":true}`
		}
		return arguments
	}
	rec := fs.AppendTaskRecordSanitized(MainTask, []llm.Message{message}, sanitize)
	// NewToolCallMessage uses an interface content field; build the response
	// with its own string pointer and keep all native replay metadata intact.
	text := secret
	response := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &text, ToolCalls: calls, Native: native, ReasoningContent: secret}}}}
	rec.SetResponseSanitized(response, time.Second, sanitize)
	serialized, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), secret) {
		t.Fatal("sensitive transcript contains native or argument credentials")
	}
	if rec.Response.Native.Payload != nil || rec.RequestMessages[0].Native.Payload != nil {
		t.Fatal("native payload copied to sensitive history")
	}
	for i := range calls {
		if len(rec.Response.ToolCalls[i].ExtraContent) != 0 || len(rec.RequestMessages[0].ToolCalls[i].ExtraContent) != 0 {
			t.Fatal("opaque metadata copied to sensitive history")
		}
		if string(response.ToolCalls()[i].ExtraContent) != string(extra) || string(message.ToolCalls[i].ExtraContent) != string(extra) {
			t.Fatal("live replay metadata was modified")
		}
	}
	if response.Native().Payload == nil || message.Native.Payload == nil || !strings.Contains(calls[0].Function.Arguments, secret) {
		t.Fatal("live replay state was modified")
	}
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}
	records := readJSONLRecords(t, sessionJSONLPath(t, sh.RepoDir, sh.SessionID))
	encoded, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatal("secret persisted in JSONL")
	}
}

func TestResponseMetadataIsCopiedBeforePersistence(t *testing.T) {
	extra := json.RawMessage(`{"signature":"original"}`)
	response := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{
		ToolCalls: []llm.ToolCall{{Function: llm.FunctionCall{Name: "file_read", Arguments: `{}`}, ExtraContent: extra}},
	}}}}
	record := &TaskRecord{}
	record.SetResponse(response, time.Second)
	extra[2] = 'X'
	if string(record.Response.ToolCalls[0].ExtraContent) != `{"signature":"original"}` {
		t.Fatal("stored metadata aliases the live response")
	}
}

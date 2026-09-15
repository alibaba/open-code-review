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
	calls := []llm.ToolCall{{ID: "sensitive", Function: llm.FunctionCall{Name: "mcp__fixture__probe", Arguments: `{"token":"` + secret + `"}`}}}
	message := llm.NewToolCallMessage(secret, calls, native, secret)
	sh := New(t.TempDir(), "main", "test", SessionOptions{})
	fs := sh.GetOrCreateFileSession("main.go")
	sanitize := func(string, string) string { return `{"redacted":true}` }
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

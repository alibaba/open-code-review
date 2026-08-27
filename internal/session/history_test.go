// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	anthropic "github.com/anthropics/anthropic-sdk-go"
)

func TestNew(t *testing.T) {
	sh := New("/tmp/repo", "main", "gpt-4", SessionOptions{
		ReviewMode: ReviewModeWorkspace,
		DiffFrom:   "a",
		DiffTo:     "b",
		DiffCommit: "c",
	})
	if sh == nil {
		t.Fatal("New returned nil")
	}
	if sh.SessionID == "" {
		t.Error("SessionID should not be empty")
	}
	if sh.RepoDir != "/tmp/repo" {
		t.Errorf("RepoDir = %q", sh.RepoDir)
	}
	if sh.GitBranch != "main" {
		t.Errorf("GitBranch = %q", sh.GitBranch)
	}
	if sh.Model != "gpt-4" {
		t.Errorf("Model = %q", sh.Model)
	}
	if sh.ReviewMode != ReviewModeWorkspace {
		t.Errorf("ReviewMode = %q", sh.ReviewMode)
	}
	if sh.DiffFrom != "a" || sh.DiffTo != "b" || sh.DiffCommit != "c" {
		t.Errorf("Diff fields mismatch")
	}
	if sh.StartTime.IsZero() {
		t.Error("StartTime should be set")
	}
	if sh.FileSessions == nil {
		t.Error("FileSessions map should be initialized")
	}
}

func TestGetOrCreateFileSession(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})

	fs1 := sh.GetOrCreateFileSession("main.go")
	if fs1 == nil {
		t.Fatal("nil FileSession")
	}
	if fs1.FilePath != "main.go" {
		t.Errorf("FilePath = %q", fs1.FilePath)
	}

	fs2 := sh.GetOrCreateFileSession("main.go")
	if fs1 != fs2 {
		t.Error("expected same FileSession instance on second call")
	}

	fs3 := sh.GetOrCreateFileSession("other.go")
	if fs3 == fs1 {
		t.Error("different paths should yield different sessions")
	}
}

func TestAppendTaskRecord(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("file.go")

	msgs := []llm.Message{llm.NewTextMessage("user", "hello")}
	rec := fs.AppendTaskRecord(MainTask, msgs)
	if rec == nil {
		t.Fatal("nil TaskRecord")
	}
	if rec.Type != MainTask {
		t.Errorf("Type = %v", rec.Type)
	}
	if rec.RequestNo != 1 {
		t.Errorf("RequestNo = %d, want 1", rec.RequestNo)
	}

	rec2 := fs.AppendTaskRecord(MainTask, msgs)
	if rec2.RequestNo != 2 {
		t.Errorf("second RequestNo = %d, want 2", rec2.RequestNo)
	}

	rec3 := fs.AppendTaskRecord(PlanTask, msgs)
	if rec3.RequestNo != 1 {
		t.Errorf("PlanTask RequestNo = %d, want 1 (separate counter)", rec3.RequestNo)
	}
}

func TestAppendTaskRecord_DefensiveCopy(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("file.go")

	msgs := []llm.Message{llm.NewTextMessage("user", "original")}
	rec := fs.AppendTaskRecord(MainTask, msgs)
	msgs[0] = llm.NewTextMessage("user", "mutated")

	if rec.RequestMessages[0].ExtractText() == "mutated" {
		t.Error("AppendTaskRecord should store a copy of messages")
	}
}

// TestAppendTaskRecord_CopiesNativeAndReasoningContent guards copyMessages'
// own "deep copy" contract: an assistant history message carrying replay
// state must keep that state in TaskRecord.RequestMessages too, not just in
// the JSONL projection (copyMessagesForJSON) — otherwise anything reading the
// in-memory record (e.g. a future compression/export path) would silently see
// an incomplete turn.
func TestAppendTaskRecord_CopiesNativeAndReasoningContent(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("file.go")

	native := llm.NativeTurn{Family: "anthropic-messages", Payload: anthropic.MessageParam{
		Content: []anthropic.ContentBlockParamUnion{anthropic.NewThinkingBlock("sig-copy", "thinking")},
	}}
	msgs := []llm.Message{
		llm.NewTextMessage("user", "hi"),
		llm.NewToolCallMessage("", nil, native, "reasoning text"),
	}
	rec := fs.AppendTaskRecord(MainTask, msgs)

	got := rec.RequestMessages[1]
	if got.ReasoningContent != "reasoning text" {
		t.Errorf("ReasoningContent = %q, want %q", got.ReasoningContent, "reasoning text")
	}
	if got.Native.Family != "anthropic-messages" {
		t.Errorf("Native.Family = %q, want anthropic-messages", got.Native.Family)
	}
	payload, ok := got.Native.Payload.(anthropic.MessageParam)
	if !ok {
		t.Fatalf("Native.Payload type = %T, want anthropic.MessageParam", got.Native.Payload)
	}
	if len(payload.Content) != 1 || payload.Content[0].OfThinking == nil || payload.Content[0].OfThinking.Signature != "sig-copy" {
		t.Errorf("Native.Payload signature not preserved: %+v", payload)
	}
}

func TestSetResponse(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("file.go")
	rec := fs.AppendTaskRecord(MainTask, []llm.Message{llm.NewTextMessage("user", "hi")})

	content := "response text"
	resp := &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content:          &content,
				ReasoningContent: "because the diff touches auth code",
			},
		}},
		Model: "gpt-4",
		Usage: &llm.UsageInfo{
			PromptTokens:     100,
			CompletionTokens: 50,
		},
	}

	rec.SetResponse(resp, 2*time.Second)

	if rec.Response == nil {
		t.Fatal("Response should be set")
	}
	if rec.Response.Content != "response text" {
		t.Errorf("Content = %q", rec.Response.Content)
	}
	if rec.Response.ReasoningContent != "because the diff touches auth code" {
		t.Errorf("ReasoningContent = %q", rec.Response.ReasoningContent)
	}
	if rec.Response.Model != "gpt-4" {
		t.Errorf("Model = %q", rec.Response.Model)
	}
	if rec.Response.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d", rec.Response.Usage.PromptTokens)
	}
	if rec.Response.Usage.CompletionTokens != 50 {
		t.Errorf("CompletionTokens = %d", rec.Response.Usage.CompletionTokens)
	}
	if rec.Duration != 2*time.Second {
		t.Errorf("Duration = %v", rec.Duration)
	}
}

// TestSetResponse_PersistsReasoningContent guards the audit-transcript gap
// raised in review: the raw per-turn llm_response record must carry the
// model's reasoning text, not just Content/ToolCalls/Usage — otherwise a
// human auditing session.jsonl has no way to see why the model did what it
// did on a turn that produced no visible text.
func TestSetResponse_PersistsReasoningContent(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	fs := sh.GetOrCreateFileSession("file.go")
	rec := fs.AppendTaskRecord(MainTask, []llm.Message{llm.NewTextMessage("user", "hi")})

	content := ""
	resp := &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content:          &content,
				ReasoningContent: "auditable reasoning text",
			},
		}},
		Model: "test-model",
	}
	rec.SetResponse(resp, time.Second)
	sh.Finalize()

	records := readJSONLRecords(t, sessionJSONLPath(t, repoDir, sh.SessionID))
	var found bool
	for _, r := range records {
		if r["type"] != "llm_response" {
			continue
		}
		found = true
		if r["reasoning_content"] != "auditable reasoning text" {
			t.Errorf("reasoning_content = %v, want %q", r["reasoning_content"], "auditable reasoning text")
		}
	}
	if !found {
		t.Fatal("no llm_response record found in session JSONL")
	}
}

// TestSetResponse_PersistsNativePayload guards the training-reflow gap raised
// in review: the llm_response record must carry the opaque replay payload
// (e.g. an Anthropic thinking block's signature), not just the plain-text
// ReasoningContent projection — otherwise a session exported for training
// cannot reconstruct a request that replays byte for byte.
func TestSetResponse_PersistsNativePayload(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	fs := sh.GetOrCreateFileSession("file.go")
	rec := fs.AppendTaskRecord(MainTask, []llm.Message{llm.NewTextMessage("user", "hi")})

	native := llm.NativeTurn{
		Family: "anthropic-messages",
		Payload: anthropic.MessageParam{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewThinkingBlock("sig-abc-123", "let me think this through"),
			},
		},
	}
	resp := &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{Native: native},
		}},
		Model: "test-model",
	}
	rec.SetResponse(resp, time.Second)
	sh.Finalize()

	if rec.Response.Native.Family != "anthropic-messages" {
		t.Errorf("in-memory Native.Family = %q, want anthropic-messages", rec.Response.Native.Family)
	}

	records := readJSONLRecords(t, sessionJSONLPath(t, repoDir, sh.SessionID))
	var found bool
	for _, r := range records {
		if r["type"] != "llm_response" {
			continue
		}
		found = true
		np, ok := r["native_payload"].(map[string]any)
		if !ok {
			t.Fatalf("native_payload missing or wrong shape: %v", r["native_payload"])
		}
		if np["family"] != "anthropic-messages" {
			t.Errorf("native_payload.family = %v, want anthropic-messages", np["family"])
		}
		payload, err := json.Marshal(np["payload"])
		if err != nil {
			t.Fatalf("marshal native_payload.payload: %v", err)
		}
		if !bytes.Contains(payload, []byte(`"signature":"sig-abc-123"`)) {
			t.Errorf("thinking signature not preserved verbatim in native_payload: %s", payload)
		}
	}
	if !found {
		t.Fatal("no llm_response record found in session JSONL")
	}
}

// TestSetResponse_OmitsNativePayloadWhenAbsent guards the other half of the
// same contract: an ordinary turn with nothing to replay beyond
// Content/ToolCalls must not grow a native_payload key at all.
func TestSetResponse_OmitsNativePayloadWhenAbsent(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	fs := sh.GetOrCreateFileSession("file.go")
	rec := fs.AppendTaskRecord(MainTask, []llm.Message{llm.NewTextMessage("user", "hi")})

	content := "plain text answer"
	resp := &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
		Model:   "test-model",
	}
	rec.SetResponse(resp, time.Second)
	sh.Finalize()

	records := readJSONLRecords(t, sessionJSONLPath(t, repoDir, sh.SessionID))
	var found bool
	for _, r := range records {
		if r["type"] != "llm_response" {
			continue
		}
		found = true
		if _, ok := r["native_payload"]; ok {
			t.Errorf("native_payload should be omitted for a turn with no replay state, got %v", r["native_payload"])
		}
	}
	if !found {
		t.Fatal("no llm_response record found in session JSONL")
	}
}

// TestAppendTaskRecord_PersistsToolCallsAndNativePayload guards
// copyMessagesForJSON: the llm_request record's messages must be a
// self-contained, replayable projection of what was actually sent — including
// tool_calls (previously dropped) and, for an assistant history turn carrying
// replay state, its native_payload with fields like a thinking signature
// intact.
func TestAppendTaskRecord_PersistsToolCallsAndNativePayload(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	fs := sh.GetOrCreateFileSession("file.go")

	native := llm.NativeTurn{
		Family: "anthropic-messages",
		Payload: anthropic.MessageParam{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewThinkingBlock("sig-xyz", "thinking"),
			},
		},
	}
	toolCalls := []llm.ToolCall{{ID: "call_1", Type: "function", Function: llm.FunctionCall{Name: "code_comment", Arguments: "{}"}}}
	msgs := []llm.Message{
		llm.NewTextMessage("user", "hi"),
		llm.NewToolCallMessage("", toolCalls, native, "thinking text"),
	}
	fs.AppendTaskRecord(MainTask, msgs)
	sh.Finalize()

	records := readJSONLRecords(t, sessionJSONLPath(t, repoDir, sh.SessionID))
	var found bool
	for _, r := range records {
		if r["type"] != "llm_request" {
			continue
		}
		found = true
		reqMsgs, ok := r["messages"].([]any)
		if !ok || len(reqMsgs) != 2 {
			t.Fatalf("messages = %v", r["messages"])
		}
		assistantMsg, ok := reqMsgs[1].(map[string]any)
		if !ok {
			t.Fatalf("assistant message shape: %v", reqMsgs[1])
		}
		if _, ok := assistantMsg["tool_calls"]; !ok {
			t.Errorf("tool_calls missing from persisted request message: %v", assistantMsg)
		}
		np, ok := assistantMsg["native_payload"].(map[string]any)
		if !ok {
			t.Fatalf("native_payload missing from persisted request message: %v", assistantMsg)
		}
		payload, err := json.Marshal(np["payload"])
		if err != nil {
			t.Fatalf("marshal native_payload.payload: %v", err)
		}
		if !bytes.Contains(payload, []byte(`"signature":"sig-xyz"`)) {
			t.Errorf("thinking signature not preserved in request message: %s", payload)
		}
	}
	if !found {
		t.Fatal("no llm_request record found in session JSONL")
	}
}

func TestSetResponse_EmptyResponse(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("file.go")
	rec := fs.AppendTaskRecord(MainTask, []llm.Message{llm.NewTextMessage("user", "hi")})

	rec.SetResponse(nil, time.Second)
	if rec.Error == "" {
		t.Error("expected error for nil response")
	}
}

func TestSetError(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("file.go")
	rec := fs.AppendTaskRecord(MainTask, []llm.Message{llm.NewTextMessage("user", "hi")})

	rec.SetError(errors.New("timeout"), 5*time.Second)

	if rec.Error != "timeout" {
		t.Errorf("Error = %q, want %q", rec.Error, "timeout")
	}
	if rec.Duration != 5*time.Second {
		t.Errorf("Duration = %v", rec.Duration)
	}
}

func TestLLMFailures(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	if sh.LLMFailures() != 0 {
		t.Errorf("initial failures = %d", sh.LLMFailures())
	}

	fs := sh.GetOrCreateFileSession("a.go")
	rec := fs.AppendTaskRecord(MainTask, nil)
	rec.SetError(errors.New("fail1"), time.Second)

	rec2 := fs.AppendTaskRecord(MainTask, nil)
	rec2.SetError(errors.New("fail2"), time.Second)

	if sh.LLMFailures() != 2 {
		t.Errorf("failures = %d, want 2", sh.LLMFailures())
	}
}

func TestAddToolResult(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("file.go")
	rec := fs.AppendTaskRecord(MainTask, nil)

	rec.AddToolResult("file_read", `{"path":"main.go"}`, "package main")

	if len(rec.ToolResults) != 1 {
		t.Fatalf("len = %d", len(rec.ToolResults))
	}
	tr := rec.ToolResults[0]
	if tr.ToolName != "file_read" {
		t.Errorf("ToolName = %q", tr.ToolName)
	}
	if tr.Arguments != `{"path":"main.go"}` {
		t.Errorf("Arguments = %q", tr.Arguments)
	}
	if tr.Result != "package main" {
		t.Errorf("Result = %q", tr.Result)
	}
}

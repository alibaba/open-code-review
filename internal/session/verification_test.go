// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestPersistVerificationWithResponseAndSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()
	s := New(repo, "main", "model", SessionOptions{})
	record := &llm.VerificationRecord{CompletionID: "chat-1", Model: "model", Status: "verified", SignatureKind: "provider_tee", NearAISettings: llm.NearAISettings{E2EE: true, VerificationLevel: "model-tee"}}
	response := &llm.ChatResponse{Model: "model", Verification: record, Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5}}
	content := "verified answer"
	response.Choices = []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}}
	fs := s.GetOrCreateFileSession("main.go")
	tr := fs.AppendTaskRecord(MainTask, []llm.Message{llm.NewTextMessage("user", "review")})
	tr.SetResponse(response, time.Second)
	s.SetVerificationReport(&llm.VerificationReport{Status: "verified", Verified: 1, NearAISettings: record.NearAISettings, Records: []llm.VerificationRecord{*record}})
	if err := s.Finalize(); err != nil {
		t.Fatal(err)
	}
	path, err := SessionFilePath(repo, s.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	var replies, ends int
	for _, rec := range readJSONLRecords(t, path) {
		switch rec["type"] {
		case "llm_response":
			replies++
			v, ok := rec["verification"].(map[string]any)
			if !ok || v["completion_id"] != "chat-1" || v["signature_kind"] != "provider_tee" {
				t.Fatalf("response verification: %+v", rec)
			}
		case "session_end":
			ends++
			v, ok := rec["verification"].(map[string]any)
			if !ok || v["status"] != "verified" || v["verified"] != float64(1) {
				t.Fatalf("session verification: %+v", rec)
			}
		}
	}
	if replies != 1 || ends != 1 || tr.Response.Verification != record {
		t.Fatal("verification not persisted exactly once")
	}
}

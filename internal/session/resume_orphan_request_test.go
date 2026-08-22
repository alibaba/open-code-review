// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"os"
	"path/filepath"
	"testing"
)

// orphanLLMRequestLine is the shape WriteLLMRequest emits, with no llm_response
// after it. Since the task record is now created before the HTTP call rather
// than after it, a run killed mid-request leaves exactly this in the JSONL.
const orphanLLMRequestLine = `{"uuid":"u2","parentUuid":"u1","type":"llm_request","sessionId":"orphan-session",` +
	`"timestamp":"2026-08-07T02:00:00Z","filePath":"b.go","taskType":"main_task","request_no":1,` +
	`"messages":[{"role":"user","content":"review b.go"}]}`

// TestLoadResumeState_IgnoresOrphanLLMRequest is the regression guard for that
// reordering: resume must treat a request with no response as absent, neither
// erroring out nor counting b.go as completed. applyResumeLine has no case for
// llm_request, and this test is what keeps it that way.
func TestLoadResumeState_IgnoresOrphanLLMRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	repoDir := "/test/orphan"
	sessionID := "orphan-session"
	path, err := SessionFilePath(repoDir, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}

	// a.go finished before the kill; b.go only got as far as its request.
	content := `{"type":"session_start","sessionId":"orphan-session","cwd":"/test/orphan","reviewMode":"range"}` + "\n" +
		`{"type":"review_item_done","filePath":"a.go","fingerprint":"fp-a","comments":[{"content":"comment-a"}]}` + "\n" +
		orphanLLMRequestLine + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	state, err := LoadResumeState(repoDir, sessionID)
	if err != nil {
		t.Fatalf("LoadResumeState: %v", err)
	}
	if state.CompletedCount() != 1 {
		t.Errorf("CompletedCount = %d, want 1 (only a.go)", state.CompletedCount())
	}
	if _, ok := state.Item("fp-a"); !ok {
		t.Error("fp-a should have survived the orphan request line")
	}
	for fp := range state.Items {
		if state.Items[fp].FilePath == "b.go" {
			t.Errorf("b.go was resumed as completed from an orphan llm_request: %+v", state.Items[fp])
		}
	}
}

// TestApplyResumeLine_OrphanLLMRequestIsNoOp is the same guarantee at the unit
// level: the line must change nothing at all, not merely avoid an error.
func TestApplyResumeLine_OrphanLLMRequestIsNoOp(t *testing.T) {
	s := &ResumeState{Items: make(map[string]ResumeItem)}
	if err := s.applyResumeLine([]byte(orphanLLMRequestLine)); err != nil {
		t.Fatalf("applyResumeLine: %v", err)
	}
	if len(s.Items) != 0 {
		t.Errorf("Items = %+v, want empty", s.Items)
	}
	if s.SessionID != "" || s.ReviewMode != "" {
		t.Errorf("llm_request mutated state: SessionID=%q ReviewMode=%q", s.SessionID, s.ReviewMode)
	}
}

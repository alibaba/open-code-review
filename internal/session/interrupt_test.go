// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"testing"
)

// sessionEndRecord returns the single session_end record from a finalized
// session, failing the test when there is not exactly one.
func sessionEndRecord(t *testing.T, repoDir, sessionID string) map[string]any {
	t.Helper()
	var found map[string]any
	for _, rec := range readJSONLRecords(t, sessionJSONLPath(t, repoDir, sessionID)) {
		if rec["type"] != "session_end" {
			continue
		}
		if found != nil {
			t.Fatal("session_end written more than once")
		}
		found = rec
	}
	if found == nil {
		t.Fatal("no session_end record")
	}
	return found
}

// TestRecordInterrupted_ReachesSessionList is the point of the label: the reason
// the run recorded has to survive the replay that `ocr session list --json`
// performs, otherwise the status column still has nothing better to say than
// the catch-all "aborted".
func TestRecordInterrupted_ReachesSessionList(t *testing.T) {
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	sh.RecordInterrupted(InterruptReasonCancelled)
	if err := sh.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	if got, _ := sessionEndRecord(t, repoDir, sh.SessionID)["interrupted_by"]; got != InterruptReasonCancelled {
		t.Fatalf("session_end.interrupted_by = %v, want %q", got, InterruptReasonCancelled)
	}

	summaries, err := ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("got %d summaries, want 1", len(summaries))
	}
	if summaries[0].InterruptedBy != InterruptReasonCancelled {
		t.Fatalf("Summary.InterruptedBy = %q, want %q",
			summaries[0].InterruptedBy, InterruptReasonCancelled)
	}
	// A cancelled run still finalized, so it is not the unclosed-session case.
	if summaries[0].Aborted {
		t.Error("a finalized interrupted run must not report Aborted")
	}
}

// TestFinalize_OmitsInterruptedByWhenClean pins the record shape for the common
// path. Readers treat a missing key as "finished on its own", so writing it
// empty would be harmless — but every existing session would then differ from
// every new one by a field that says nothing.
func TestFinalize_OmitsInterruptedByWhenClean(t *testing.T) {
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	if err := sh.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if _, ok := sessionEndRecord(t, repoDir, sh.SessionID)["interrupted_by"]; ok {
		t.Fatal("session_end carried interrupted_by for a run that was not interrupted")
	}
}

// TestRecordInterrupted_IgnoredAfterFinalize guards the write-once contract:
// session_end has already reached disk and every reader has seen it, so a late
// call must not make the in-memory summary disagree with that record.
func TestRecordInterrupted_IgnoredAfterFinalize(t *testing.T) {
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	if err := sh.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	sh.RecordInterrupted(InterruptReasonCancelled)

	if _, ok := sessionEndRecord(t, repoDir, sh.SessionID)["interrupted_by"]; ok {
		t.Fatal("RecordInterrupted after Finalize changed the persisted session_end")
	}
	if err := sh.Finalize(); err != nil {
		t.Fatalf("second Finalize: %v", err)
	}
}

// TestRecordInterrupted_EmptyReasonIsNoop keeps an unset reason from turning
// into a label that claims an interruption happened for no stated cause.
func TestRecordInterrupted_EmptyReasonIsNoop(t *testing.T) {
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	sh.RecordInterrupted("")
	if err := sh.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if got, ok := sessionEndRecord(t, repoDir, sh.SessionID)["interrupted_by"]; ok {
		t.Fatalf("session_end.interrupted_by = %v, want the key absent", got)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/session"
)

// writeScanResumeSession persists a full-scan session with the given completed
// file checkpoints and returns its ID. HOME must already point at a temp dir.
func writeScanResumeSession(t *testing.T, repoDir string, files ...string) string {
	t.Helper()
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeFullScan,
	})
	for _, f := range files {
		sh.RecordReviewItemDone(f, "", f, "fp-"+f, nil)
	}
	if err := sh.Finalize(); err != nil {
		t.Fatalf("finalize session: %v", err)
	}
	return sh.SessionID
}

// appendTornTail writes the on-disk bytes a SIGINT-killed scan leaves: a final
// record whose closing bytes never reached disk. Returns the new session path.
func appendTornTail(t *testing.T, repoDir, sessionID string) string {
	t.Helper()
	path, err := session.SessionFilePath(repoDir, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open session for torn append: %v", err)
	}
	if _, err := f.Write([]byte(`{"type":"review_item_done","filePath":"torn.go","fingerprint":`)); err != nil {
		t.Fatalf("append torn record: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return path
}

// TestLoadScanResumeState_TornTailRecovered is the end-to-end rendering of the
// reported bug: a scan killed before its last checkpoint reached disk must
// resume from the last complete checkpoint and tell the user a record was
// recovered — not fail with "unexpected end of JSON input".
func TestLoadScanResumeState_TornTailRecovered(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	id := writeScanResumeSession(t, repoDir, "a.go")
	appendTornTail(t, repoDir, id)

	var stderr string
	var state *session.ResumeState
	stderr = captureStderr(t, func() {
		var err error
		state, err = loadScanResumeState(repoDir, scanOptions{resume: id}, nil)
		if err != nil {
			t.Fatalf("loadScanResumeState: %v", err)
		}
	})
	if state == nil || state.CompletedCount() != 1 {
		t.Fatalf("got state=%v completed=%v, want 1 completed item after torn-tail recovery", state, state.CompletedCount())
	}
	if !state.Recovered {
		t.Error("state.Recovered = false, want true")
	}
	for _, want := range []string{"ended mid-write", "last complete checkpoint"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr = %q, want recovery warning containing %q", stderr, want)
		}
	}
	if strings.Contains(stderr, "unexpected end of JSON input") {
		t.Errorf("stderr still reports the corruption after recovery: %q", stderr)
	}
}

// TestLoadScanResumeState_WithSession drives the fixture-backed branches of
// loadScanResumeState: a successful resume, a scan-mode mismatch, and a session
// that completed no items.
func TestLoadScanResumeState_WithSession(t *testing.T) {
	t.Run("success returns state with completed items", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		id := writeScanResumeSession(t, repoDir, "a.go", "b.go")

		state, err := loadScanResumeState(repoDir, scanOptions{resume: id}, nil)
		if err != nil {
			t.Fatalf("loadScanResumeState: %v", err)
		}
		if state == nil || state.CompletedCount() != 2 {
			t.Fatalf("got %v, want state with 2 completed items", state)
		}
	})

	t.Run("non-scan session rejected", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		// Persist a range-mode session, then try to resume it as a scan.
		id := writeRangeResumeSession(t, repoDir, "a.go")

		_, err := loadScanResumeState(repoDir, scanOptions{resume: id}, nil)
		if err == nil {
			t.Fatal("expected error resuming a non-scan session")
		}
	})

	t.Run("no completed items errors", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		id := writeScanResumeSession(t, repoDir) // no items recorded

		_, err := loadScanResumeState(repoDir, scanOptions{resume: id}, nil)
		if err == nil || !strings.Contains(err.Error(), "no completed scan items") {
			t.Fatalf("got %v, want no-completed-items error", err)
		}
	})
}

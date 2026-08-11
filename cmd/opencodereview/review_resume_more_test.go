// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/session"
)

// writeRangeResumeSession persists a range-mode session with the given completed
// file checkpoints and returns its ID. HOME must already point at a temp dir.
func writeRangeResumeSession(t *testing.T, repoDir string, files ...string) string {
	t.Helper()
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
	})
	for _, f := range files {
		sh.RecordReviewItemDone(f, "", f, "fp-"+f, nil)
	}
	if err := sh.Finalize(); err != nil {
		t.Fatalf("finalize session: %v", err)
	}
	return sh.SessionID
}

// TestLoadReviewResumeState_WithSession drives the fixture-backed branches of
// loadReviewResumeState: a successful resume, a review-mode mismatch, and a
// session that completed no items.
func TestLoadReviewResumeState_WithSession(t *testing.T) {
	t.Run("success returns state with completed items", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		id := writeRangeResumeSession(t, repoDir, "a.go", "b.go")

		state, err := loadReviewResumeState(repoDir, reviewOptions{resume: id, from: "main", to: "feature"})
		if err != nil {
			t.Fatalf("loadReviewResumeState: %v", err)
		}
		if state == nil || state.CompletedCount() != 2 {
			t.Fatalf("got %v, want state with 2 completed items", state)
		}
	})

	t.Run("review mode mismatch errors", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		id := writeRangeResumeSession(t, repoDir, "a.go")

		// Session was range-mode; request commit-mode resume.
		_, err := loadReviewResumeState(repoDir, reviewOptions{resume: id, commit: "HEAD"})
		if err == nil {
			t.Fatal("expected error for mode mismatch")
		}
	})

	t.Run("no completed items errors", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		id := writeRangeResumeSession(t, repoDir) // no items recorded

		_, err := loadReviewResumeState(repoDir, reviewOptions{resume: id, from: "main", to: "feature"})
		if err == nil || !strings.Contains(err.Error(), "no completed review items") {
			t.Fatalf("got %v, want no-completed-items error", err)
		}
	})
}

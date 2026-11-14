// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A failed workspace review must not recommend --resume, because
// loadReviewResumeState rejects workspace mode and the hint would steer the user
// down a recovery path that always fails. The session ID is still printed for
// diagnostics. Commit/range reviews keep their existing --resume guidance.

// dirtyWorkspaceRepo builds a repo with committed history plus one uncommitted
// change, so a review without --from/--to/--commit runs in workspace mode with
// at least one entry to review.
func dirtyWorkspaceRepo(t *testing.T) string {
	t.Helper()
	dir := retryTestRepo(t)
	// retryTestRepo leaves the tree clean; add an uncommitted edit to a.go.
	p := filepath.Join(dir, "a.go")
	if err := os.WriteFile(p, []byte("package p\n\nfunc a() int { return 3 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReviewE2E_WorkspaceFailureOmitsResumeHint(t *testing.T) {
	repoDir := dirtyWorkspaceRepo(t)
	srv := newFakeLLM()
	srv.failAll()
	startFakeLLM(t, srv)

	var errOut string
	_ = captureStdout(t, func() {
		errOut = captureStderr(t, func() {
			_ = runReview([]string{"--repo", repoDir, "--format", "text"})
		})
	})

	if !strings.Contains(errOut, "[ocr] Session:") {
		t.Fatalf("a failed workspace review must still print the session ID for diagnostics:\n%s", errOut)
	}
	if strings.Contains(errOut, "--resume") {
		t.Errorf("workspace review cannot be resumed, so no --resume hint may be printed:\n%s", errOut)
	}
	if !strings.Contains(errOut, "cannot be resumed") {
		t.Errorf("workspace failure guidance must explain that resume is unsupported:\n%s", errOut)
	}
}

func TestReviewE2E_RangeFailureKeepsResumeHint(t *testing.T) {
	repoDir := retryTestRepo(t)
	srv := newFakeLLM()
	srv.failAll()
	startFakeLLM(t, srv)

	var errOut string
	_ = captureStdout(t, func() {
		errOut = captureStderr(t, func() {
			_ = runReview([]string{"--repo", repoDir, "--from", "HEAD~1", "--to", "HEAD", "--format", "text"})
		})
	})

	if !strings.Contains(errOut, "--resume") {
		t.Errorf("a failed commit/range review must keep its --resume guidance:\n%s", errOut)
	}
}

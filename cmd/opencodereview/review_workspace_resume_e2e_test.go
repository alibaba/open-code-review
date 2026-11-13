// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/session"
)

func workspaceTestRepo(t *testing.T) string {
	t.Helper()
	dir := retryTestRepo(t)
	retryTestGit(t, dir, "reset", "HEAD~1")
	return dir
}

func executeReviewCLI(ctx context.Context, args []string) error {
	opts, err := parseReviewFlags(args)
	if err != nil {
		return err
	}
	ictx, stop := interruptContextWithForcedExit(ctx)
	defer stop()
	return executeReviewContext(ictx, opts)
}

func findNewSession(t *testing.T, repoDir string, seen map[string]bool) *session.Summary {
	t.Helper()
	summaries, err := session.ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	for i := range summaries {
		if !seen[summaries[i].SessionID] {
			seen[summaries[i].SessionID] = true
			return &summaries[i]
		}
	}
	t.Fatal("expected a new session on disk")
	return nil
}

func TestWorkspaceReviewResume_E2E(t *testing.T) {
	repoDir := workspaceTestRepo(t)
	srv := newFakeLLM()
	startFakeLLM(t, srv)

	var sigCh chan<- os.Signal
	origNotify := notifySignals
	notifySignals = func(c chan<- os.Signal) {
		sigCh = c
	}
	defer func() { notifySignals = origNotify }()

	runCLI := func(args ...string) (string, error) {
		srv.mu.Lock()
		srv.attemptsByFile = map[string]int{}
		srv.mu.Unlock()

		var runErr error
		stderr := captureStderr(t, func() {
			captureStdout(t, func() {
				runErr = executeReviewCLI(context.Background(), args)
			})
		})
		return stderr, runErr
	}

	seen := map[string]bool{}
	var parentSummary, resumedSummary *session.Summary

	t.Run("interrupt saves checkpoint and prints terminal feedback", func(t *testing.T) {
		srv.onAttempt = func(file string, n int, r *http.Request) {
			if file == "b.go" {
				if sigCh != nil {
					sigCh <- os.Interrupt
				}
				select {
				case <-r.Context().Done():
				case <-time.After(5 * time.Second):
				}
			}
		}

		stderr, runErr := runCLI("--repo", repoDir, "--concurrency", "1")
		if runErr == nil {
			t.Fatal("expected interrupted review to return error")
		}
		if !strings.Contains(stderr, "[ocr] Interrupted. Saving session checkpoint...") {
			t.Fatalf("expected stderr to contain interrupt checkpoint message, got:\n%s", stderr)
		}

		parentSummary = findNewSession(t, repoDir, seen)
		if parentSummary.RunManifest == nil || parentSummary.RunManifest.TerminalState != session.StateFailed {
			t.Fatalf("expected failed terminal state on disk, got %+v", parentSummary.RunManifest)
		}
		parentState, err := session.LoadResumeState(repoDir, parentSummary.SessionID)
		if err != nil {
			t.Fatalf("LoadResumeState: %v", err)
		}
		if parentState.CompletedCount() != 1 {
			t.Fatalf("expected 1 completed item in parent checkpoint, got %d", parentState.CompletedCount())
		}
		var completedFile string
		for _, item := range parentState.Items {
			completedFile = item.FilePath
		}
		if completedFile != "a.go" {
			t.Fatalf("expected a.go to be completed checkpoint, got %q", completedFile)
		}
	})

	t.Run("resume infers workspace mode and skips completed checkpoint", func(t *testing.T) {
		if parentSummary == nil {
			t.Fatal("missing parent session")
		}
		srv.mu.Lock()
		srv.onAttempt = nil
		srv.mu.Unlock()

		resumeStderr, resumeErr := runCLI("--repo", repoDir, "--resume", parentSummary.SessionID)
		if resumeErr != nil {
			t.Fatalf("resumed review must succeed: %v\nstderr: %s", resumeErr, resumeStderr)
		}

		attempts := srv.attemptCounts()
		if attempts["a.go"] != 0 {
			t.Errorf("a.go should be reused from parent session (0 attempts), got %d", attempts["a.go"])
		}
		if attempts["b.go"]+attempts["c.go"]+attempts["d.go"] == 0 {
			t.Errorf("expected remaining uncompleted files to be reviewed in resumed run, got 0 attempts")
		}

		resumedSummary = findNewSession(t, repoDir, seen)
		if resumedSummary.ResumeLineage == nil || resumedSummary.ResumeLineage.ParentRunID != parentSummary.SessionID {
			t.Fatalf("expected resumed session to record parent lineage %q, got %+v", parentSummary.SessionID, resumedSummary.ResumeLineage)
		}
		if resumedSummary.ReusedFiles != 1 {
			t.Errorf("expected 1 reused file, got %d", resumedSummary.ReusedFiles)
		}
	})

	t.Run("partial workspace edit re-reviews modified file only", func(t *testing.T) {
		if resumedSummary == nil {
			t.Fatal("missing resumed session")
		}
		modD := fmt.Sprintf("package p\n\n// changed %s\n// extra diff\nfunc %s() int {\n\treturn 99\n}\n",
			"MARKER_DELTA", strings.TrimSuffix("d.go", ".go"))
		if err := os.WriteFile(filepath.Join(repoDir, "d.go"), []byte(modD), 0o600); err != nil {
			t.Fatal(err)
		}

		if _, partialErr := runCLI("--repo", repoDir, "--resume", resumedSummary.SessionID); partialErr != nil {
			t.Fatalf("resume with partially modified file must succeed: %v", partialErr)
		}

		attempts := srv.attemptCounts()
		for _, f := range []string{"a.go", "b.go", "c.go"} {
			if attempts[f] != 0 {
				t.Errorf("%s was unmodified and should have 0 attempts, got %d", f, attempts[f])
			}
		}
		if attempts["d.go"] == 0 {
			t.Errorf("d.go was modified and should have been re-reviewed, got 0 attempts")
		}

		latestSummary := findNewSession(t, repoDir, seen)
		if latestSummary.ReusedFiles != 3 {
			t.Errorf("expected 3 reused files, got %d", latestSummary.ReusedFiles)
		}
	})
}

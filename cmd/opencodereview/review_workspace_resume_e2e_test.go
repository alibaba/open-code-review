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
	"github.com/spf13/cobra"
)

func workspaceTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	retryTestGit(t, dir, "init", "-q", "-b", "main")
	for _, name := range markers {
		body := fmt.Sprintf("package p\n\nfunc %s() int { return 1 }\n", strings.TrimSuffix(name, ".go"))
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	retryTestGit(t, dir, "add", ".")
	retryTestGit(t, dir, "commit", "-q", "-m", "base")

	for marker, name := range markers {
		body := fmt.Sprintf("package p\n\n// changed %s\nfunc %s() int {\n\treturn 2\n}\n",
			marker, strings.TrimSuffix(name, ".go"))
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func executeReviewCLI(ctx context.Context, args []string) error {
	var opts reviewOptions
	cmd := &cobra.Command{
		Use:           "review",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateReviewOptions(&opts); err != nil {
				return err
			}
			ictx, stop := interruptContextWithForcedExit(cmd.Context())
			defer stop()
			return executeReviewContext(ictx, opts)
		},
	}
	registerReviewFlags(cmd, &opts)
	cmd.SetContext(ctx)
	cmd.SetArgs(args)
	return cmd.Execute()
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

	// Interrupt the first review during b.go execution once a.go has completed.
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

	var runErr error
	var runOut string
	stderr := captureStderr(t, func() {
		runOut = captureStdout(t, func() {
			runErr = executeReviewCLI(context.Background(), []string{"--repo", repoDir, "--concurrency", "1"})
		})
	})
	_ = runOut

	if runErr == nil {
		t.Fatal("expected interrupted review to return error")
	}
	if !strings.Contains(stderr, "[ocr] Interrupted. Saving session checkpoint...") {
		t.Fatalf("expected stderr to contain interrupt checkpoint message, got:\n%s", stderr)
	}

	summaries, err := session.ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(summaries) == 0 {
		t.Fatal("expected session on disk")
	}
	parentSummary := summaries[0]
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

	// Resume without range/commit flags: verify workspace mode is auto-inferred
	// and uncompleted files are reviewed.
	srv.mu.Lock()
	srv.attemptsByFile = map[string]int{}
	srv.onAttempt = nil
	srv.mu.Unlock()

	var resumeErr error
	resumeStderr := captureStderr(t, func() {
		captureStdout(t, func() {
			resumeErr = executeReviewCLI(context.Background(), []string{"--repo", repoDir, "--resume", parentSummary.SessionID})
		})
	})
	if resumeErr != nil {
		t.Fatalf("resumed review must succeed: %v\nstderr: %s", resumeErr, resumeStderr)
	}

	attempts := srv.attemptCounts()
	if attempts["a.go"] != 0 {
		t.Errorf("a.go should be reused from parent session (0 attempts), got %d", attempts["a.go"])
	}
	totalRemainingAttempts := attempts["b.go"] + attempts["c.go"] + attempts["d.go"]
	if totalRemainingAttempts == 0 {
		t.Errorf("expected remaining uncompleted files to be reviewed in resumed run, got 0 attempts")
	}

	summaries, err = session.ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(summaries) < 2 {
		t.Fatalf("expected at least 2 sessions, got %d", len(summaries))
	}
	var resumedSummary *session.Summary
	for i := range summaries {
		if summaries[i].SessionID != parentSummary.SessionID {
			resumedSummary = &summaries[i]
			break
		}
	}
	if resumedSummary == nil {
		t.Fatal("expected to find resumed session")
	}
	if resumedSummary.ResumeLineage == nil || resumedSummary.ResumeLineage.ParentRunID != parentSummary.SessionID {
		t.Fatalf("expected resumed session to record parent lineage %q, got %+v", parentSummary.SessionID, resumedSummary.ResumeLineage)
	}
	if resumedSummary.ReusedFiles != 1 {
		t.Errorf("expected 1 reused file, got %d", resumedSummary.ReusedFiles)
	}

	// Partial modification in workspace: modify d.go while keeping a.go, b.go, c.go unchanged.
	modD := fmt.Sprintf("package p\n\n// changed %s\n// extra diff\nfunc %s() int {\n\treturn 99\n}\n",
		"MARKER_DELTA", strings.TrimSuffix("d.go", ".go"))
	if err := os.WriteFile(filepath.Join(repoDir, "d.go"), []byte(modD), 0o600); err != nil {
		t.Fatal(err)
	}

	srv.mu.Lock()
	srv.attemptsByFile = map[string]int{}
	srv.mu.Unlock()

	var partialErr error
	captureStderr(t, func() {
		captureStdout(t, func() {
			partialErr = executeReviewCLI(context.Background(), []string{"--repo", repoDir, "--resume", resumedSummary.SessionID})
		})
	})
	if partialErr != nil {
		t.Fatalf("resume with partially modified file must succeed: %v", partialErr)
	}

	attempts = srv.attemptCounts()
	for _, f := range []string{"a.go", "b.go", "c.go"} {
		if attempts[f] != 0 {
			t.Errorf("%s was unmodified and should have 0 attempts, got %d", f, attempts[f])
		}
	}
	if attempts["d.go"] == 0 {
		t.Errorf("d.go was modified and should have been re-reviewed, got 0 attempts")
	}

	summaries, err = session.ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	var latestSummary *session.Summary
	for i := range summaries {
		if summaries[i].SessionID != parentSummary.SessionID && summaries[i].SessionID != resumedSummary.SessionID {
			latestSummary = &summaries[i]
			break
		}
	}
	if latestSummary == nil {
		t.Fatal("expected to find latest resumed session")
	}
	if latestSummary.ReusedFiles != 3 {
		t.Errorf("expected 3 reused files, got %d", latestSummary.ReusedFiles)
	}

	// Git branch switch must be rejected.
	retryTestGit(t, repoDir, "checkout", "-b", "other-branch")
	var branchErr error
	captureStderr(t, func() {
		captureStdout(t, func() {
			branchErr = executeReviewCLI(context.Background(), []string{"--repo", repoDir, "--resume", latestSummary.SessionID})
		})
	})
	if branchErr == nil || !strings.Contains(branchErr.Error(), "does not match session branch") {
		t.Fatalf("expected branch mismatch error, got: %v", branchErr)
	}
	retryTestGit(t, repoDir, "checkout", "main")

	// Modifying all files triggers the zero-reusable items guard.
	for marker, name := range markers {
		body := fmt.Sprintf("package p\n\n// entirely different %s\nfunc %s() int {\n\treturn 12345\n}\n",
			marker, strings.TrimSuffix(name, ".go"))
		if err := os.WriteFile(filepath.Join(repoDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var zeroErr error
	captureStderr(t, func() {
		captureStdout(t, func() {
			zeroErr = executeReviewCLI(context.Background(), []string{"--repo", repoDir, "--resume", latestSummary.SessionID})
		})
	})
	if zeroErr == nil || !strings.Contains(zeroErr.Error(), "no completed review items from session") {
		t.Fatalf("expected zero-reusable items guard error, got: %v", zeroErr)
	}
}

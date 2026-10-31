// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/session"
)

// TestRunRecordsCancellationInSession drives a real cancellation through Run,
// not dispatchSubtasks. The label is written at Run's boundary, so only this
// path can show that a Ctrl-C leaves a reason behind instead of a session that
// reads like it was killed.
func TestRunRecordsCancellationInSession(t *testing.T) {
	setTestHome(t, t.TempDir())
	dir := initPreviewRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "blocked.go"), []byte("package blocked\n"), 0o644); err != nil {
		t.Fatalf("write blocked.go: %v", err)
	}

	client := &cancellationFlowClient{blocked: make(chan struct{})}
	a := New(Args{
		RepoDir:   dir,
		LLMClient: client,
		Model:     "fake",
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 3,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}},
			},
		},
	})
	a.args.MaxConcurrency = 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() {
		_, err := a.Run(ctx)
		runErr <- err
	}()

	select {
	case <-client.blocked:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("blocked review did not start")
	}
	if err := <-runErr; err == nil {
		t.Fatal("a cancelled run must report an error")
	}

	got := singleSessionSummary(t, dir)
	if got.InterruptedBy != session.InterruptReasonCancelled {
		t.Fatalf("InterruptedBy = %q, want %q", got.InterruptedBy, session.InterruptReasonCancelled)
	}
	// The run still finalized and still froze a manifest: the reason is a label
	// on top of the existing outcome, never a replacement for it.
	if got.Aborted {
		t.Error("a cancelled run that finalized must not report Aborted")
	}
	if got.RunManifest == nil {
		t.Fatal("cancelled run lost its manifest")
	}
	if got.RunManifest.TerminalState == session.StateComplete {
		t.Errorf("terminal state = %q, want the run's real coverage outcome", got.RunManifest.TerminalState)
	}
}

// TestRunLabelsCancellationBeforeDispatch covers the paths that finalize before
// any subtask starts. A cancel landing while the diffs are still being resolved
// ends in an input-failure session_end, and that record has to carry the reason
// too — otherwise the status reads like a plain load failure and the one thing
// the run knows for certain is missing again.
func TestRunLabelsCancellationBeforeDispatch(t *testing.T) {
	setTestHome(t, t.TempDir())
	dir := initPreviewRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "early.go"), []byte("package early\n"), 0o644); err != nil {
		t.Fatalf("write early.go: %v", err)
	}

	a := New(Args{
		RepoDir: dir,
		Model:   "fake",
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 3,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}},
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Run(ctx); err == nil {
		t.Fatal("a cancelled run must report an error")
	}

	got := singleSessionSummary(t, dir)
	if got.InterruptedBy != session.InterruptReasonCancelled {
		t.Fatalf("InterruptedBy = %q, want %q", got.InterruptedBy, session.InterruptReasonCancelled)
	}
	if got.Aborted {
		t.Error("the cancelled run still finalized, so it must not report Aborted")
	}
	// The cancel lands in diff resolution, so the manifest is the input failure
	// this path has always written — the reason is added to it, not substituted
	// for it.
	if got.RunManifest == nil || got.RunManifest.TerminalState != session.StateFailed {
		t.Fatalf("terminal state = %+v, want failed load", got.RunManifest)
	}
}

// singleSessionSummary reads back the one session a run left on disk.
func singleSessionSummary(t *testing.T, repo string) session.Summary {
	t.Helper()
	summaries, err := session.ListSessions(repo)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("got %d sessions, want 1", len(summaries))
	}
	return summaries[0]
}

// TestRunDoesNotLabelCompletedRun is the other half of the contract: a run that
// reaches its own terminal state stays unlabelled, so `interrupted_by` never
// shows up as boilerplate on healthy sessions.
func TestRunDoesNotLabelCompletedRun(t *testing.T) {
	setTestHome(t, t.TempDir())
	dir := initPreviewRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "note.go"), []byte("package note\n"), 0o644); err != nil {
		t.Fatalf("write note.go: %v", err)
	}

	a := New(Args{
		RepoDir:   dir,
		LLMClient: manifestFlowClient{},
		Model:     "fake",
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 3,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}},
			},
		},
	})
	if _, err := a.Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	if got := singleSessionSummary(t, dir).InterruptedBy; got != "" {
		t.Fatalf("InterruptedBy = %q, want empty for a completed run", got)
	}
}

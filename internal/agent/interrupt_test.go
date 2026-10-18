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

	summaries, err := session.ListSessions(dir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("got %d sessions, want 1", len(summaries))
	}
	got := summaries[0]
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

	summaries, err := session.ListSessions(dir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("got %d sessions, want 1", len(summaries))
	}
	if got := summaries[0].InterruptedBy; got != "" {
		t.Fatalf("InterruptedBy = %q, want empty for a completed run", got)
	}
}

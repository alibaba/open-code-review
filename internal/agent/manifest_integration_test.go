package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

type manifestFlowClient struct{}

func (manifestFlowClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	var prompt string
	for _, message := range req.Messages {
		if text, ok := message.Content.(string); ok {
			prompt += text
		}
	}
	switch {
	case strings.Contains(prompt, "panic.go"):
		panic("manifest integration panic")
	case strings.Contains(prompt, "bad.go"):
		return nil, errors.New("provider rejected api_key=LEAKED at /Users/example/private")
	case strings.Contains(prompt, "slow.go"):
		// A per-item deadline (wrapped, not a cancelled dispatch ctx) so the
		// timeout stays isolated to this file while its sibling succeeds.
		return nil, fmt.Errorf("provider call timed out: %w", context.DeadlineExceeded)
	default:
		return agentTaskDoneResponse(), nil
	}
}

func newManifestFlowAgent(t *testing.T, diffs []model.Diff, resume *session.ResumeState) *Agent {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode:  session.ReviewModeRange,
		DiffFrom:    "main",
		DiffTo:      "feature",
		ResumedFrom: resumedFromSession(resume),
		Operation:   session.OperationReview,
	})
	a := New(Args{
		RepoDir:    repoDir,
		From:       "main",
		To:         "feature",
		ReviewMode: session.ReviewModeRange,
		LLMClient:  manifestFlowClient{},
		Model:      "fake",
		Session:    sh,
		Resume:     resume,
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 5,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{current_file_path}} {{diff}}"}},
			},
		},
		MainToolDefs: []llm.ToolDef{{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "task_done",
				Description: "finish the review",
			},
		}},
	})
	a.diffs = diffs
	a.currentDate = "2026-07-26 12:00"
	return a
}

func finishManifestFlow(t *testing.T, a *Agent) *session.RunManifest {
	t.Helper()
	if err := a.finalizeManifest(); err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	if err := a.session.Finalize(); err != nil {
		t.Fatalf("finalize session: %v", err)
	}
	manifest := a.RunManifest()
	if manifest == nil {
		t.Fatal("expected frozen manifest")
	}
	return manifest
}

func TestManifestFlowCompleteAndPartial(t *testing.T) {
	t.Run("complete with zero findings", func(t *testing.T) {
		a := newManifestFlowAgent(t, []model.Diff{{OldPath: "good.go", NewPath: "good.go", Diff: "+ok", Insertions: 1}}, nil)
		comments, err := a.dispatchSubtasks(context.Background())
		if err != nil {
			t.Fatalf("dispatch: %v", err)
		}
		if len(comments) != 0 {
			t.Fatalf("comments = %d, want 0", len(comments))
		}
		manifest := finishManifestFlow(t, a)
		if manifest.TerminalState != session.StateComplete || len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 0 {
			t.Fatalf("manifest = %+v", manifest)
		}
	})

	t.Run("mixed provider result is partial", func(t *testing.T) {
		a := newManifestFlowAgent(t, []model.Diff{
			{OldPath: "good.go", NewPath: "good.go", Diff: "+ok", Insertions: 1},
			{OldPath: "bad.go", NewPath: "bad.go", Diff: "+bad", Insertions: 1},
		}, nil)
		if _, err := a.dispatchSubtasks(context.Background()); err != nil {
			t.Fatalf("mixed dispatch must continue: %v", err)
		}
		manifest := finishManifestFlow(t, a)
		if manifest.TerminalState != session.StatePartial || len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 1 {
			t.Fatalf("manifest = %+v", manifest)
		}
		if manifest.Coverage.Failed[0].Classification != session.FailureProvider {
			t.Fatalf("failure = %+v", manifest.Coverage.Failed[0])
		}
		if strings.Contains(manifest.Coverage.Failed[0].Reason, "LEAKED") || strings.Contains(manifest.Coverage.Failed[0].Reason, "/Users/") {
			t.Fatalf("manifest exposed raw provider error: %+v", manifest.Coverage.Failed[0])
		}
	})
}

func TestManifestFlowRunInputFailureIsPersisted(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "missing-base",
		DiffTo:     "missing-head",
		Operation:  session.OperationReview,
	})
	a := New(Args{
		RepoDir:    repoDir,
		From:       "missing-base",
		To:         "missing-head",
		ReviewMode: session.ReviewModeRange,
		GitRunner:  gitcmd.New(1),
		LLMClient:  manifestFlowClient{},
		Model:      "fake",
		Session:    sh,
	})

	if _, err := a.Run(context.Background()); err == nil {
		t.Fatal("invalid review input must fail")
	}
	manifest := a.RunManifest()
	if manifest == nil || manifest.TerminalState != session.StateFailed || manifest.RunFailure == nil {
		t.Fatalf("manifest = %+v", manifest)
	}
	if manifest.RunFailure.Classification != session.RunFailureInput || len(manifest.Coverage.Selected) != 0 {
		t.Fatalf("run failure = %+v, coverage = %+v", manifest.RunFailure, manifest.Coverage)
	}

	summary, err := session.LoadSummary(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("load persisted summary: %v", err)
	}
	if summary.RunManifest == nil || summary.RunManifest.TerminalState != session.StateFailed || summary.Aborted {
		t.Fatalf("persisted summary = %+v", summary)
	}
}

func TestManifestFlowAllFailedTimeoutAndPanic(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		ctx       func() context.Context
		wantClass session.FailureClass
	}{
		{name: "provider", path: "bad.go", ctx: context.Background, wantClass: session.FailureProvider},
		{name: "timeout", path: "timeout.go", ctx: func() context.Context {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			t.Cleanup(cancel)
			return ctx
		}, wantClass: session.FailureTimeout},
		{name: "panic", path: "panic.go", ctx: context.Background, wantClass: session.FailurePanic},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newManifestFlowAgent(t, []model.Diff{{OldPath: tc.path, NewPath: tc.path, Diff: "+x", Insertions: 1}}, nil)
			if _, err := a.dispatchSubtasks(tc.ctx()); err == nil {
				t.Fatal("all-failed dispatch must return an error")
			}
			manifest := finishManifestFlow(t, a)
			if manifest.TerminalState != session.StateFailed || manifest.RunFailure != nil || len(manifest.Coverage.Failed) != 1 {
				t.Fatalf("manifest = %+v", manifest)
			}
			if got := manifest.Coverage.Failed[0].Classification; got != tc.wantClass {
				t.Fatalf("classification = %q, want %q", got, tc.wantClass)
			}
		})
	}
}

// A single item failing by timeout, panic, or budget must stay isolated: the run
// is partial (not failed), the sibling still completes, and run_failure is nil —
// per-item outcomes never escalate to a run-level failure.
func TestManifestFlowMixedFailureIsIsolatedToPartial(t *testing.T) {
	// The budget item's diff stays under the diff-size pre-filter (80% of
	// MaxTokens), while its long path inflates the rendered prompt past the same
	// threshold — so only this file trips the budget stop, without touching the
	// shared template that its sibling also renders.
	longPath := strings.Repeat("nested/", 20) + "budget.go"
	for _, tc := range []struct {
		name      string
		failDiff  model.Diff
		setup     func(*Agent)
		wantClass session.FailureClass
	}{
		{
			name:      "timeout",
			failDiff:  model.Diff{OldPath: "slow.go", NewPath: "slow.go", Diff: "+slow", Insertions: 1},
			wantClass: session.FailureTimeout,
		},
		{
			name:      "panic",
			failDiff:  model.Diff{OldPath: "panic.go", NewPath: "panic.go", Diff: "+boom", Insertions: 1},
			wantClass: session.FailurePanic,
		},
		{
			name:      "budget",
			failDiff:  model.Diff{OldPath: longPath, NewPath: longPath, Diff: strings.Repeat("token ", 50), Insertions: 1},
			setup:     func(a *Agent) { a.args.Template.MaxTokens = 100 },
			wantClass: session.FailureBudget,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			good := model.Diff{OldPath: "good.go", NewPath: "good.go", Diff: "+ok", Insertions: 1}
			a := newManifestFlowAgent(t, []model.Diff{good, tc.failDiff}, nil)
			if tc.setup != nil {
				tc.setup(a)
			}
			// One item's failure must not fail the whole run: dispatch continues.
			if _, err := a.dispatchSubtasks(context.Background()); err != nil {
				t.Fatalf("mixed dispatch must continue past a single-item failure: %v", err)
			}
			manifest := finishManifestFlow(t, a)
			if manifest.TerminalState != session.StatePartial {
				t.Fatalf("terminal = %q, want partial; manifest = %+v", manifest.TerminalState, manifest)
			}
			if manifest.RunFailure != nil {
				t.Fatalf("item-level failure must not set run_failure: %+v", manifest.RunFailure)
			}
			if len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 1 {
				t.Fatalf("coverage = %+v", manifest.Coverage)
			}
			if got := manifest.Coverage.Failed[0].Classification; got != tc.wantClass {
				t.Fatalf("classification = %q, want %q", got, tc.wantClass)
			}
		})
	}
}

func TestManifestFlowBudgetAndSkipped(t *testing.T) {
	t.Run("single-item budget stop", func(t *testing.T) {
		a := newManifestFlowAgent(t, []model.Diff{{OldPath: "budget.go", NewPath: "budget.go", Diff: "+x", Insertions: 1}}, nil)
		a.args.Template.MaxTokens = 100
		a.args.Template.MainTask.Messages[0].Content = strings.Repeat("context ", 200) + "{{diff}}"
		if _, err := a.dispatchSubtasks(context.Background()); err != nil {
			t.Fatalf("business stop is represented by coverage, not a dispatch error: %v", err)
		}
		manifest := finishManifestFlow(t, a)
		if manifest.TerminalState != session.StateFailed || len(manifest.Coverage.Failed) != 1 || manifest.Coverage.Failed[0].Classification != session.FailureBudget {
			t.Fatalf("manifest = %+v", manifest)
		}
	})

	t.Run("all oversized is skipped", func(t *testing.T) {
		a := newManifestFlowAgent(t, []model.Diff{{OldPath: "large.go", NewPath: "large.go", Diff: strings.Repeat("word ", 500), Insertions: 1}}, nil)
		a.args.Template.MaxTokens = 10
		if _, err := a.dispatchSubtasks(context.Background()); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
		manifest := finishManifestFlow(t, a)
		if manifest.TerminalState != session.StateSkipped || len(manifest.Coverage.Selected) != 0 {
			t.Fatalf("manifest = %+v", manifest)
		}
	})
}

func TestManifestFlowResumeRecordsParentAndReusedItem(t *testing.T) {
	diffs := []model.Diff{
		{OldPath: "cached.go", NewPath: "cached.go", Diff: "+cached", Insertions: 1},
		{OldPath: "fresh.go", NewPath: "fresh.go", Diff: "+fresh", Insertions: 1},
	}
	fingerprint := reviewItemFingerprint(session.ReviewModeRange, diffs[0])
	resume := &session.ResumeState{
		SessionID:  "parent-run",
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Items: map[string]session.ResumeItem{
			fingerprint: {
				FilePath:    "cached.go",
				OldPath:     "cached.go",
				NewPath:     "cached.go",
				Fingerprint: fingerprint,
			},
		},
	}
	a := newManifestFlowAgent(t, diffs, resume)
	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	manifest := finishManifestFlow(t, a)
	if manifest.ParentRunID != "parent-run" || len(manifest.Coverage.Reused) != 1 || len(manifest.Coverage.Completed) != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}
	if manifest.Input.RequestedFrom != "main" || manifest.Input.RequestedHead != "feature" || manifest.Input.SourceArtifactSHA256 == "" {
		t.Fatalf("child input identity = %+v", manifest.Input)
	}
}

func TestManifestFlowResumeWithReusedAndAllRerunsFailedIsPartial(t *testing.T) {
	diffs := []model.Diff{
		{OldPath: "cached.go", NewPath: "cached.go", Diff: "+cached", Insertions: 1},
		{OldPath: "bad.go", NewPath: "bad.go", Diff: "+bad", Insertions: 1},
	}
	fingerprint := reviewItemFingerprint(session.ReviewModeRange, diffs[0])
	resume := &session.ResumeState{
		SessionID:  "parent-run",
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Items: map[string]session.ResumeItem{
			fingerprint: {
				FilePath:    "cached.go",
				OldPath:     "cached.go",
				NewPath:     "cached.go",
				Fingerprint: fingerprint,
			},
		},
	}

	a := newManifestFlowAgent(t, diffs, resume)
	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("partial resumed dispatch must not return an error: %v", err)
	}
	manifest := finishManifestFlow(t, a)
	if manifest.TerminalState != session.StatePartial || len(manifest.Coverage.Reused) != 1 || len(manifest.Coverage.Failed) != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}
}

// TestManifestFlowResumeWithProviderTransition verifies that a resumed run
// records the *current* provider/model in execution (not the parent's) while
// still linking to the parent via parent_run_id. Covers the issue #367
// acceptance criterion "provider transition": a consumer must be able to
// audit that a provider/model change happened across a resume chain.
func TestManifestFlowResumeWithProviderTransition(t *testing.T) {
	diffs := []model.Diff{
		{OldPath: "cached.go", NewPath: "cached.go", Diff: "+cached", Insertions: 1},
	}
	fingerprint := reviewItemFingerprint(session.ReviewModeRange, diffs[0])
	// The parent run used a different provider/model; ResumeState only carries
	// the session id and fingerprint, never the parent's execution metadata.
	resume := &session.ResumeState{
		SessionID:  "parent-run",
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Items: map[string]session.ResumeItem{
			fingerprint: {
				FilePath:    "cached.go",
				OldPath:     "cached.go",
				NewPath:     "cached.go",
				Fingerprint: fingerprint,
			},
		},
	}

	a := newManifestFlowAgent(t, diffs, resume)
	// Simulate a provider/model transition on the child run. New() already
	// called initManifest with the default "fake" model; re-seed execution so
	// the frozen manifest reflects the child's actual provider/model.
	a.args.Provider = "beta"
	a.args.Model = "model-b"
	a.initManifest()

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	manifest := finishManifestFlow(t, a)

	// The child manifest records the *current* provider/model, so a consumer
	// can audit that a transition occurred across the resume chain.
	if manifest.Execution.Provider != "beta" || manifest.Execution.Model != "model-b" {
		t.Fatalf("execution = %+v, want provider=beta model=model-b", manifest.Execution)
	}
	// parent_run_id links to the parent, making the transition traceable.
	if manifest.ParentRunID != "parent-run" {
		t.Fatalf("parent_run_id = %q, want parent-run", manifest.ParentRunID)
	}
	// The fingerprint hit is recorded as reused; the child recomputes its
	// own input identity (source_artifact_sha256).
	if len(manifest.Coverage.Reused) != 1 || len(manifest.Coverage.Completed) != 0 {
		t.Fatalf("coverage reused=%d completed=%d, want 1/0",
			len(manifest.Coverage.Reused), len(manifest.Coverage.Completed))
	}
	if manifest.Input.SourceArtifactSHA256 == "" {
		t.Fatal("child source_artifact_sha256 must be populated")
	}
}

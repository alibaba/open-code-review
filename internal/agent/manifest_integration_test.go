// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
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

type cancellationFlowClient struct {
	blocked chan struct{}
	once    sync.Once
}

func (c *cancellationFlowClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	var prompt string
	for _, message := range req.Messages {
		if text, ok := message.Content.(string); ok {
			prompt += text
		}
	}
	if strings.Contains(prompt, "blocked.go") {
		c.once.Do(func() { close(c.blocked) })
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return agentTaskDoneResponse(), nil
}

func newManifestFlowAgent(t *testing.T, diffs []model.Diff, resume *session.ResumeState) *Agent {
	t.Helper()
	return newManifestFlowAgentWithClient(t, diffs, resume, manifestFlowClient{})
}

func newManifestFlowAgentWithClient(t *testing.T, diffs []model.Diff, resume *session.ResumeState, client llm.LLMClient) *Agent {
	t.Helper()
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode:  session.ReviewModeRange,
		DiffFrom:    "main",
		DiffTo:      "feature",
		ResumedFrom: resumedFromSession(resume),
		Operation:   session.OperationReview,
	})
	t.Cleanup(func() { _ = sh.Finalize() })
	a := New(Args{
		RepoDir:    repoDir,
		From:       "main",
		To:         "feature",
		ReviewMode: session.ReviewModeRange,
		LLMClient:  client,
		Model:      "fake",
		Session:    sh,
		Resume:     resume,
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 5,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}},
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

// seedParentManifest gives a hand-built ResumeState the parent manifest a real
// resume always has, matching the identity a already resolved. Without one the
// agent reuses nothing at all: dispatch re-verifies the input against the parent
// manifest, and only fingerprints the parent's own coverage settled are eligible.
//
// It must be called once a.diffs is final — the identity hashes exactly that set.
func seedParentManifest(t *testing.T, a *Agent, fingerprints ...string) {
	t.Helper()
	resume := a.args.Resume
	if resume == nil {
		t.Fatal("seedParentManifest needs an agent built with a resume state")
	}
	id := a.runIdentity()
	items := make([]session.CoverageItem, 0, len(fingerprints))
	for _, fp := range fingerprints {
		items = append(items, session.CoverageItem{ItemID: fp, Fingerprint: fp})
	}
	resume.Closed = true
	resume.Manifest = &session.RunManifest{
		SchemaVersion: session.ManifestSchemaVersion,
		RunID:         resume.SessionID,
		Operation:     session.OperationReview,
		Repository:    session.ManifestRepository{IdentitySHA256: id.RepositorySHA256},
		Input:         session.ManifestInput{Mode: id.Mode, SourceArtifactSHA256: id.SourceArtifactSHA256},
		Execution:     session.ManifestExecution{RuleConfigSHA256: id.RuleConfigSHA256},
		Coverage:      session.Coverage{Selected: items, Completed: items},
	}
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

func TestManifestFlowCancellationPersistsResumableSession(t *testing.T) {
	done := model.Diff{OldPath: "done.go", NewPath: "done.go", Diff: "+done", Insertions: 1}
	blocked := model.Diff{OldPath: "blocked.go", NewPath: "blocked.go", Diff: "+blocked", Insertions: 1}
	pending := model.Diff{OldPath: "pending.go", NewPath: "pending.go", Diff: "+pending", Insertions: 1}
	client := &cancellationFlowClient{blocked: make(chan struct{})}
	a := newManifestFlowAgentWithClient(t, []model.Diff{done, blocked, pending}, nil, client)
	a.args.MaxConcurrency = 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dispatchErr := make(chan error, 1)
	go func() {
		_, err := a.dispatchSubtasks(ctx)
		dispatchErr <- err
	}()

	select {
	case <-client.blocked:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("blocked review did not start")
	}
	if err := <-dispatchErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("dispatch error = %v, want context.Canceled", err)
	}

	manifest := finishManifestFlow(t, a)
	if manifest.RunFailure == nil || manifest.RunFailure.Classification != session.RunFailureCancelled {
		t.Fatalf("run failure = %+v, want cancelled", manifest.RunFailure)
	}
	if len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 2 {
		t.Fatalf("coverage = %+v, want one completed and two failed", manifest.Coverage)
	}
	for _, item := range manifest.Coverage.Failed {
		if item.Classification != session.FailureCancelled {
			t.Fatalf("failed item = %+v, want cancelled", item)
		}
	}

	state, err := session.LoadReviewResumeState(a.args.RepoDir, a.session.SessionID)
	if err != nil {
		t.Fatalf("load cancelled session: %v", err)
	}
	identity := session.RunIdentity{
		Mode:                 manifest.Input.Mode,
		SourceArtifactSHA256: manifest.Input.SourceArtifactSHA256,
		RuleConfigSHA256:     manifest.Execution.RuleConfigSHA256,
		RepositorySHA256:     manifest.Repository.IdentitySHA256,
	}
	if err := state.ValidateResume(session.ResumeRequest{
		Identity: identity,
		Provider: manifest.Execution.Provider,
		Model:    manifest.Execution.Model,
	}); err != nil {
		t.Fatalf("cancelled session should remain resumable: %v", err)
	}
	if _, ok := state.ReusableItem(reviewItemFingerprint(a.reviewMode(), done)); !ok {
		t.Fatal("completed item is not reusable after cancellation")
	}
	if _, ok := state.ReusableItem(reviewItemFingerprint(a.reviewMode(), blocked)); ok {
		t.Fatal("cancelled item must be reviewed again")
	}
}

func TestManifestFlowCancellationBeforeDispatchStartsNoSubtask(t *testing.T) {
	pending := model.Diff{OldPath: "blocked.go", NewPath: "blocked.go", Diff: "+blocked", Insertions: 1}
	client := &cancellationFlowClient{blocked: make(chan struct{})}
	a := newManifestFlowAgentWithClient(t, []model.Diff{pending}, nil, client)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.dispatchSubtasks(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("dispatch error = %v, want context.Canceled", err)
	}
	if got := atomic.LoadInt64(&a.subtaskFailed); got != 0 {
		t.Fatalf("subtask failures = %d, want 0 because no subtask should start", got)
	}
	select {
	case <-client.blocked:
		t.Fatal("LLM client was called after cancellation")
	default:
	}

	manifest := finishManifestFlow(t, a)
	if manifest.RunFailure == nil || manifest.RunFailure.Classification != session.RunFailureCancelled {
		t.Fatalf("run failure = %+v, want cancelled", manifest.RunFailure)
	}
	if len(manifest.Coverage.Completed) != 0 || len(manifest.Coverage.Failed) != 1 {
		t.Fatalf("coverage = %+v, want one cancelled item and no completed items", manifest.Coverage)
	}
	if got := manifest.Coverage.Failed[0].Classification; got != session.FailureCancelled {
		t.Fatalf("failure classification = %q, want %q", got, session.FailureCancelled)
	}
}

func TestManifestFlowRunInputFailureIsPersisted(t *testing.T) {
	setTestHome(t, t.TempDir())
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
		a.args.Template.MainTask.Messages[0].Content = strings.Repeat("context ", 200) + "{{diffs}}"
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

// groupedBudgetPartialClient plays two roles for one grouped-review round trip:
// the grouping call (no Tools on the request) always merges every file into one
// group, and the main-task call (Tools present) files a comment for the first
// round then keeps returning an unregistered tool name — a valid, non-empty
// result that is neither task_done nor an empty round — until the round budget
// is exhausted by the caller's MaxToolRequestTimes.
type groupedBudgetPartialClient struct {
	round int32 // atomic
}

func (c *groupedBudgetPartialClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if len(req.Tools) == 0 {
		content := `[{"label":"pair","files":["a.go","b.go"]}]`
		return &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
			Model:   "fake",
		}, nil
	}
	if atomic.AddInt32(&c.round, 1) == 1 {
		return &llm.ChatResponse{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{
					ToolCalls: []llm.ToolCall{{
						ID:   "1",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "code_comment",
							Arguments: `{"comments":[{"path":"a.go","content":"missing a null check here"}]}`,
						},
					}},
				},
				FinishReason: "tool_calls",
			}},
			Model: "fake",
		}, nil
	}
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				ToolCalls: []llm.ToolCall{{
					ID:       "x",
					Type:     "function",
					Function: llm.FunctionCall{Name: "keep_exploring", Arguments: "{}"},
				}},
			},
			FinishReason: "tool_calls",
		}},
		Model: "fake",
	}, nil
}

// A group that never calls task_done before exhausting its round budget must
// classify coverage per file, not per group: a.go already has a comment from
// round 1, so it belongs in Completed even though the shared conversation
// stopped on StopMaxRounds; b.go never got a comment, so it stays Failed. The
// mixed outcome must read as partial (exit 0), not failed (exit 1) — the
// bitcoin__bitcoin regression this guards against had exactly one file with
// output and one without in the same stuck group, and the run reported "2 of 2
// selected item(s) failed" despite the real comment.
func TestManifestFlowGroupBudgetStopIsPartialWhenSomeFilesHaveComments(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	diffs := []model.Diff{
		{OldPath: "a.go", NewPath: "a.go", Diff: "+a", Insertions: 1},
		{OldPath: "b.go", NewPath: "b.go", Diff: "+b", Insertions: 1},
	}
	client := &groupedBudgetPartialClient{}
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Operation:  session.OperationReview,
	})
	// code_comment is only wired up if a CodeCommentProvider is registered in
	// Tools — the loop looks the provider up by name (loop.go:558) before it
	// ever reaches the code_comment-specific handling, even though it is a
	// built-in tool name.
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	// MaxToolRequestTimes and GroupingTask must be set before New(), since
	// llmloop.Runner snapshots Template by value at construction time — mutating
	// a.args.Template afterward (as other tests in this file do for MaxTokens,
	// which executeGroupSubtask re-reads from a.args.Template directly) would
	// not reach the runner's copy that RunMainTask actually consumes.
	a := New(Args{
		RepoDir:          repoDir,
		From:             "main",
		To:               "feature",
		ReviewMode:       session.ReviewModeRange,
		LLMClient:        client,
		Model:            "fake",
		Session:          sh,
		CommentCollector: collector,
		Tools:            reg,
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 3,
			GroupingTask: &template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "{{file_list}}"}},
			},
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}},
			},
		},
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "finish the review"}},
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment", Description: "file a review comment"}},
		},
	})
	a.diffs = diffs
	a.currentDate = "2026-07-26 12:00"

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("a stuck group with partial output must not be a dispatch error: %v", err)
	}
	manifest := finishManifestFlow(t, a)
	if manifest.TerminalState != session.StatePartial {
		t.Fatalf("terminal = %q, want partial; manifest = %+v", manifest.TerminalState, manifest)
	}
	if manifest.RunFailure != nil {
		t.Fatalf("run_failure = %+v, want nil for a per-item budget stop", manifest.RunFailure)
	}
	if len(manifest.Coverage.Completed) != 1 || manifest.Coverage.Completed[0].Path != "a.go" {
		t.Fatalf("completed coverage = %+v, want exactly a.go", manifest.Coverage.Completed)
	}
	if len(manifest.Coverage.Failed) != 1 || manifest.Coverage.Failed[0].Path != "b.go" {
		t.Fatalf("failed coverage = %+v, want exactly b.go", manifest.Coverage.Failed)
	}
	if manifest.Coverage.Failed[0].Classification != session.FailureBudget {
		t.Fatalf("b.go classification = %q, want budget", manifest.Coverage.Failed[0].Classification)
	}
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
	seedParentManifest(t, a, fingerprint)
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

// promptSpy answers every request the same way and keeps the prompts, so a test
// can assert what never reached the model.
type promptSpy struct {
	mu      sync.Mutex
	prompts []string
}

func (p *promptSpy) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	var prompt string
	for _, message := range req.Messages {
		if text, ok := message.Content.(string); ok {
			prompt += text
		}
	}
	p.mu.Lock()
	p.prompts = append(p.prompts, prompt)
	p.mu.Unlock()
	return agentTaskDoneResponse(), nil
}

// TestManifestFlowReusedCommentsStayOutOfPrompts pins the isolation half of
// reuse: a reused finding is merged into the output and nothing else. Feeding it
// back into a prompt would let the parent's conclusions steer the child's review
// of a different file, and the finding would also be paid for twice.
func TestManifestFlowReusedCommentsStayOutOfPrompts(t *testing.T) {
	diffs := []model.Diff{
		{OldPath: "cached.go", NewPath: "cached.go", Diff: "+cached", Insertions: 1},
		{OldPath: "fresh.go", NewPath: "fresh.go", Diff: "+fresh", Insertions: 1},
	}
	fingerprint := reviewItemFingerprint(session.ReviewModeRange, diffs[0])
	const reusedFinding = "PARENT_FINDING_ABOUT_CACHED_GO"
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
				Comments:    []model.LlmComment{{Path: "cached.go", Content: reusedFinding}},
			},
		},
	}
	spy := &promptSpy{}
	a := newManifestFlowAgentWithClient(t, diffs, resume, spy)
	seedParentManifest(t, a, fingerprint)

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	var merged bool
	for _, c := range comments {
		if c.Content == reusedFinding {
			merged = true
		}
	}
	if !merged {
		t.Fatalf("reused finding must be merged into the output, got %+v", comments)
	}
	// One dispatch, for fresh.go only — cached.go was reused, so it is neither
	// re-reviewed nor mentioned to the model.
	spy.mu.Lock()
	defer spy.mu.Unlock()
	if len(spy.prompts) != 1 {
		t.Fatalf("prompts = %d, want 1 (fresh.go only)", len(spy.prompts))
	}
	for _, prompt := range spy.prompts {
		if strings.Contains(prompt, reusedFinding) {
			t.Error("a reused finding must not enter a new LLM context")
		}
		if strings.Contains(prompt, "cached.go") {
			t.Error("a reused file must not be sent to the model again")
		}
	}
}

// TestManifestFlowResumeIgnoresCheckpointTheParentManifestDoesNotVouchFor covers
// a checkpoint line whose fingerprint the parent's coverage never settled. The
// manifest is the single source of truth for coverage, so the line is not
// evidence of anything and its file is reviewed again.
func TestManifestFlowResumeIgnoresCheckpointTheParentManifestDoesNotVouchFor(t *testing.T) {
	diffs := []model.Diff{{OldPath: "cached.go", NewPath: "cached.go", Diff: "+cached", Insertions: 1}}
	fingerprint := reviewItemFingerprint(session.ReviewModeRange, diffs[0])
	resume := &session.ResumeState{
		SessionID:  "parent-run",
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Items: map[string]session.ResumeItem{
			fingerprint: {FilePath: "cached.go", NewPath: "cached.go", Fingerprint: fingerprint},
		},
	}
	a := newManifestFlowAgent(t, diffs, resume)
	// A manifest that settled some *other* item: identity still matches, so the
	// resume is admitted, but this fingerprint is not in completed or reused.
	seedParentManifest(t, a, "fp-some-other-item")

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	manifest := finishManifestFlow(t, a)
	if len(manifest.Coverage.Reused) != 0 || len(manifest.Coverage.Completed) != 1 {
		t.Fatalf("coverage reused=%d completed=%d, want 0/1 — the unvouched checkpoint must be re-reviewed",
			len(manifest.Coverage.Reused), len(manifest.Coverage.Completed))
	}
}

// TestManifestFlowResumeOfFullyFailedParentRedispatchesEverything pins the case
// resume exists for. The parent's every item failed, so replay left no
// checkpoint behind and its coverage vouches for nothing: each selected item is
// reviewed again, and the child settles them itself.
func TestManifestFlowResumeOfFullyFailedParentRedispatchesEverything(t *testing.T) {
	diffs := []model.Diff{
		{OldPath: "one.go", NewPath: "one.go", Diff: "+one", Insertions: 1},
		{OldPath: "two.go", NewPath: "two.go", Diff: "+two", Insertions: 1},
	}
	// No Items: every review_item_done the parent wrote was retracted by the
	// review_item_failed that followed it.
	resume := &session.ResumeState{
		SessionID:  "parent-run",
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Items:      map[string]session.ResumeItem{},
	}

	a := newManifestFlowAgent(t, diffs, resume)
	fingerprints := make([]string, 0, len(diffs))
	for _, d := range diffs {
		fingerprints = append(fingerprints, reviewItemFingerprint(session.ReviewModeRange, d))
	}
	seedParentManifest(t, a, fingerprints...)
	// Same selection, but the parent completed none of it.
	cov := &a.args.Resume.Manifest.Coverage
	cov.Failed, cov.Completed = cov.Completed, nil

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	manifest := finishManifestFlow(t, a)
	if len(manifest.Coverage.Reused) != 0 || len(manifest.Coverage.Completed) != len(diffs) {
		t.Fatalf("coverage reused=%d completed=%d, want 0/%d — a fully failed parent must re-dispatch everything",
			len(manifest.Coverage.Reused), len(manifest.Coverage.Completed), len(diffs))
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
	seedParentManifest(t, a, fingerprint)
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
// still linking to the parent via parent_run_id, so a consumer can audit that
// the provider or model changed across a resume chain.
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
	seedParentManifest(t, a, fingerprint)
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

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// These tests cover grace-round completion propagation: a model that makes
// context-tool calls and then emits task_done in the grace round must be
// recorded as completed, mirroring the main loop's semantics.

// graceRoundTestDeps builds Deps for grace-round tests. Tests that need to
// cancel mid-round mutate the returned Deps before building the Runner, for
// example installing a DiffLookup hook that cancels the context while
// code_comment resolves its comments.
func graceRoundTestDeps(client llm.LLMClient, maxRounds int) Deps {
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&fakeFileReadProvider{result: "package main\n"})
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()
	return Deps{
		LLMClient:        client,
		Model:            "fake",
		Template:         template.Template{MaxTokens: 100000, MaxToolRequestTimes: maxRounds},
		Tools:            reg,
		CommentCollector: collector,
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
			{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
			{Type: "function", Function: llm.FunctionDef{Name: "file_read"}},
		},
		Session: session.New("/tmp/test-repo", "main", "fake", session.SessionOptions{}),
	}
}

// Scenario A (the regression): the model spends all its rounds on
// context tools (file_read), then — given one last chance in the grace
// round — correctly calls task_done with state DONE. The completion must
// propagate: RunPerFile reports completed=true with StopNone, so callers
// record a reusable checkpoint instead of translating the run into
// "main_task did not complete before stopping".
func TestRunPerFile_GraceRoundTaskDoneCompletes(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		// Round 1: context tool call (budget = 1, exhausted after this).
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		// Grace round: model calls task_done DONE.
		taskDoneResponseWithArguments(`{"state":"DONE"}`),
	}}
	runner := NewRunner(graceRoundTestDeps(client, 1))

	completed, stop, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"main.go",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	t.Logf("Scenario A: completed=%v stop=%v llmCalls=%d", completed, stop, client.calls)

	// Sanity: the grace round did run and task_done was issued there.
	if client.calls != 2 {
		t.Fatalf("LLM calls = %d, want 2 (1 main round + 1 grace round with task_done)", client.calls)
	}
	// Regression: an explicit task_done(DONE) in the grace round must
	// complete the run, mirroring the main loop's semantics.
	if !completed {
		t.Fatal("grace-round task_done(DONE) must propagate: RunPerFile returned completed=false")
	}
	if stop != StopNone {
		t.Fatalf("stop = %v, want StopNone on grace-round completion", stop)
	}
}

// graceRoundDoneThenCommentResponse builds a grace-round response whose
// task_done(DONE) arrives BEFORE a code_comment: the loop must keep
// executing the remaining calls in the round so the late comment still
// lands, mirroring the main loop's ordering semantics.
func graceRoundDoneThenCommentResponse() *llm.ChatResponse {
	content := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{
			Content: &content,
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call_done",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "task_done",
						Arguments: `{"state":"DONE"}`,
					},
				},
				{
					ID:   "call_comment",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "code_comment",
						Arguments: `{"comments":[{"content":"late finding","existing_code":"x"}]}`,
					},
				},
			},
		}}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 30, CompletionTokens: 10},
	}
}

// TestRunPerFile_GraceRoundCommentAfterTaskDoneStillLands locks the
// same-round ordering semantics: a code_comment issued in the same grace
// round as task_done(DONE) still lands, and the run still completes.
func TestRunPerFile_GraceRoundCommentAfterTaskDoneStillLands(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		// Round 1: context tool call (budget = 1, exhausted after this).
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		// Grace round: task_done first, then a late code_comment.
		graceRoundDoneThenCommentResponse(),
	}}
	runner := NewRunner(graceRoundTestDeps(client, 1))

	completed, _, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"main.go",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("LLM calls = %d, want 2 (1 main round + 1 grace round)", client.calls)
	}
	if !completed {
		t.Fatal("grace-round task_done(DONE) must complete the run")
	}
	comments := runner.CollectPendingComments()
	if len(comments) != 1 {
		t.Fatalf("comments = %d, want 1 (the late grace-round comment must land)", len(comments))
	}
	if comments[0].Content != "late finding" {
		t.Errorf("comment content = %q, want %q", comments[0].Content, "late finding")
	}
}

// graceRoundFailedThenCommentResponse builds a grace-round response whose
// task_done(FAILED) arrives BEFORE a code_comment: the failure must return
// immediately without executing the remaining calls, mirroring the main
// loop's ordering semantics.
func graceRoundFailedThenCommentResponse() *llm.ChatResponse {
	content := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{
			Content: &content,
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call_failed",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "task_done",
						Arguments: `{"state":"FAILED"}`,
					},
				},
				{
					ID:   "call_comment",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "code_comment",
						Arguments: `{"comments":[{"content":"must not land","existing_code":"x"}]}`,
					},
				},
			},
		}}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 30, CompletionTokens: 10},
	}
}

// TestRunPerFile_GraceRoundTaskDoneFailedSurfaces locks the failure
// semantics: a model-declared task_done(FAILED) in the grace round
// surfaces as a task failure error (not budget exhaustion), and the
// failure returns immediately so later calls in the round do not execute.
func TestRunPerFile_GraceRoundTaskDoneFailedSurfaces(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		// Round 1: context tool call (budget = 1, exhausted after this).
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		// Grace round: task_done(FAILED) first, then a comment that must
		// not execute.
		graceRoundFailedThenCommentResponse(),
	}}
	runner := NewRunner(graceRoundTestDeps(client, 1))

	completed, stop, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"main.go",
	)
	if client.calls != 2 {
		t.Fatalf("LLM calls = %d, want 2 (1 main round + 1 grace round)", client.calls)
	}
	if completed {
		t.Fatal("grace-round task_done(FAILED) must not complete the run")
	}
	if stop != StopNone {
		t.Fatalf("stop = %v, want StopNone on task failure", stop)
	}
	if err == nil {
		t.Fatal("grace-round task_done(FAILED) must surface as a task failure error")
	}
	if !errors.Is(err, ErrTaskFailed) {
		t.Errorf("err = %v, must wrap ErrTaskFailed", err)
	}
	if !strings.Contains(err.Error(), "task failed") {
		t.Errorf("err = %v, want a task failure error", err)
	}
	if comments := runner.CollectPendingComments(); len(comments) != 0 {
		t.Fatalf("comments = %d, want 0 (calls after FAILED must not execute)", len(comments))
	}
}

// graceErrorClient serves the main round normally, then fails the grace
// round with an LLM outage.
type graceErrorClient struct {
	calls int
}

func (c *graceErrorClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	c.calls++
	if c.calls == 1 {
		return fileReadToolCallResponse("call_1", `{"path":"main.go"}`), nil
	}
	return nil, errors.New("grace round outage")
}

// TestRunPerFile_GraceRoundLLMErrorSwallowed locks the asymmetric error
// handling: a grace-round LLM outage is logged and swallowed, so the file
// keeps its budget-exhaustion record instead of surfacing a transient
// network error as the primary failure cause.
func TestRunPerFile_GraceRoundLLMErrorSwallowed(t *testing.T) {
	client := &graceErrorClient{}
	runner := NewRunner(graceRoundTestDeps(client, 1))

	completed, stop, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"main.go",
	)
	if client.calls != 2 {
		t.Fatalf("LLM calls = %d, want 2 (1 main round + 1 grace round)", client.calls)
	}
	if completed {
		t.Fatal("grace-round LLM outage must not complete the run")
	}
	if err != nil {
		t.Fatalf("grace-round LLM outage must be swallowed, got err: %v", err)
	}
	if stop != StopMaxRounds {
		t.Fatalf("stop = %v, want StopMaxRounds (budget exhaustion stays the recorded cause)", stop)
	}
}

// cancellingErrorClient serves the main round normally, then cancels the
// run's context and fails the grace-round LLM call, simulating a Ctrl-C
// that interrupts the request in flight.
type cancellingErrorClient struct {
	cancel context.CancelFunc
	calls  int
}

func (c *cancellingErrorClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	c.calls++
	if c.calls == 1 {
		return fileReadToolCallResponse("call_1", `{"path":"main.go"}`), nil
	}
	c.cancel()
	return nil, errors.New("request interrupted")
}

// TestRunPerFile_GraceRoundCancelledDuringLLMCall closes the cancellation
// window inside the grace-round LLM call itself: the request fails because
// the context was cancelled mid-flight. The cancellation must surface as
// context.Canceled so callers classify the item as FailureCancelled — it
// must not be swallowed as a transient LLM outage.
func TestRunPerFile_GraceRoundCancelledDuringLLMCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &cancellingErrorClient{cancel: cancel}
	runner := NewRunner(graceRoundTestDeps(client, 1))

	completed, stop, err := runner.RunPerFile(
		ctx,
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"main.go",
	)
	if client.calls != 2 {
		t.Fatalf("LLM calls = %d, want 2 (1 main round + 1 grace round)", client.calls)
	}
	if completed {
		t.Fatal("a cancelled run must never be recorded as completed")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("grace-round cancellation during the LLM call must surface as context.Canceled, got: %v", err)
	}
	if stop != StopNone {
		t.Fatalf("stop = %v, want StopNone (an error carries no stop cause)", stop)
	}
}

// cancellingClient serves the main round normally, then cancels the run's
// context at the instant the grace-round response arrives, simulating a
// Ctrl-C that lands while the response is in flight.
type cancellingClient struct {
	cancel context.CancelFunc
	calls  int
}

func (c *cancellingClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	c.calls++
	if c.calls == 1 {
		return fileReadToolCallResponse("call_1", `{"path":"main.go"}`), nil
	}
	c.cancel()
	return taskDoneResponseWithArguments(`{"state":"DONE"}`), nil
}

// TestRunPerFile_GraceRoundCancelledAfterResponseDiscarded closes the last
// cancellation window around the grace round: the context is cancelled after
// the response arrives but before its tool calls execute. The response must
// be discarded without executing it, and the cancellation must surface as
// context.Canceled — mirroring the main loop, so callers classify the item
// as FailureCancelled instead of FailureBudget.
func TestRunPerFile_GraceRoundCancelledAfterResponseDiscarded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &cancellingClient{cancel: cancel}
	runner := NewRunner(graceRoundTestDeps(client, 1))

	completed, stop, err := runner.RunPerFile(
		ctx,
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"main.go",
	)
	if client.calls != 2 {
		t.Fatalf("LLM calls = %d, want 2 (1 main round + 1 grace round)", client.calls)
	}
	if completed {
		t.Fatal("a cancelled run must never be recorded as completed")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("grace-round cancellation must surface as context.Canceled, got: %v", err)
	}
	if stop != StopNone {
		t.Fatalf("stop = %v, want StopNone (an error carries no stop cause)", stop)
	}
}

// graceRoundCommentThenDoneResponse builds a grace-round response whose
// code_comment arrives BEFORE task_done(DONE), with distinct call IDs.
func graceRoundCommentThenDoneResponse() *llm.ChatResponse {
	content := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{
			Content: &content,
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call_comment",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "code_comment",
						Arguments: `{"comments":[{"content":"finding","existing_code":"x"}]}`,
					},
				},
				{
					ID:   "call_done",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "task_done",
						Arguments: `{"state":"DONE"}`,
					},
				},
			},
		}}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 30, CompletionTokens: 10},
	}
}

// cancellingDiffLookup returns a DiffLookup hook that cancels the run's
// context the moment code_comment resolves a comment, simulating a Ctrl-C
// that lands while a grace-round tool call executes.
func cancellingDiffLookup(cancel context.CancelFunc) func(string) *model.Diff {
	return func(string) *model.Diff {
		cancel()
		return nil
	}
}

// TestRunPerFile_GraceRoundCancelledBetweenToolCalls closes the cancellation
// window inside the grace-round tool loop: the context is cancelled while
// the round's first call (code_comment) executes, and the task_done(DONE)
// that follows must not complete the run. Without the per-call check, the
// completion would propagate and callers would checkpoint a cancelled file
// as reusable.
func TestRunPerFile_GraceRoundCancelledBetweenToolCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &fakeClient{responses: []*llm.ChatResponse{
		// Round 1: context tool call (budget = 1, exhausted after this).
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		// Grace round: code_comment (cancels the context), then task_done.
		graceRoundCommentThenDoneResponse(),
	}}
	deps := graceRoundTestDeps(client, 1)
	deps.DiffLookup = cancellingDiffLookup(cancel)
	runner := NewRunner(deps)

	completed, stop, err := runner.RunPerFile(
		ctx,
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"main.go",
	)
	if client.calls != 2 {
		t.Fatalf("LLM calls = %d, want 2 (1 main round + 1 grace round)", client.calls)
	}
	if completed {
		t.Fatal("task_done(DONE) after a mid-round cancellation must not complete the run")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("grace-round cancellation must surface as context.Canceled, got: %v", err)
	}
	if stop != StopNone {
		t.Fatalf("stop = %v, want StopNone (an error carries no stop cause)", stop)
	}
}

// TestRunPerFile_GraceRoundCancelledInFinalCallDiscardsCompletion closes the
// last cancellation window: task_done(DONE) executes, then the context is
// cancelled while the round's final call (code_comment) executes. The
// already-observed completion must be discarded, so a cancelled run is never
// recorded as completed.
func TestRunPerFile_GraceRoundCancelledInFinalCallDiscardsCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &fakeClient{responses: []*llm.ChatResponse{
		// Round 1: context tool call (budget = 1, exhausted after this).
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		// Grace round: task_done first, then code_comment (cancels the context).
		graceRoundDoneThenCommentResponse(),
	}}
	deps := graceRoundTestDeps(client, 1)
	deps.DiffLookup = cancellingDiffLookup(cancel)
	runner := NewRunner(deps)

	completed, stop, err := runner.RunPerFile(
		ctx,
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"main.go",
	)
	if client.calls != 2 {
		t.Fatalf("LLM calls = %d, want 2 (1 main round + 1 grace round)", client.calls)
	}
	if completed {
		t.Fatal("a completion observed before a mid-round cancellation must be discarded")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("grace-round cancellation must surface as context.Canceled, got: %v", err)
	}
	if stop != StopNone {
		t.Fatalf("stop = %v, want StopNone (an error carries no stop cause)", stop)
	}
}

// Scenario B (the "empty final response" case): the model answers with
// plain text and no tool call at all. Each such round burns budget with a
// retry nudge; once budget is gone the grace round gets a second text-only
// reply. Nothing ever completes the task.
func TestRunPerFile_EmptyResponsesNeverComplete(t *testing.T) {
	textOnly := func() *llm.ChatResponse {
		content := "I have finished reviewing this file."
		return &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
			Model:   "fake",
			Usage:   &llm.UsageInfo{PromptTokens: 5, CompletionTokens: 5},
		}
	}
	client := &fakeClient{responses: []*llm.ChatResponse{
		textOnly(), textOnly(), textOnly(), textOnly(),
	}}
	runner := NewRunner(graceRoundTestDeps(client, 2))

	completed, stop, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"main.go",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	t.Logf("Scenario B: completed=%v stop=%v llmCalls=%d", completed, stop, client.calls)

	if completed {
		t.Fatal("text-only run must never complete")
	}
	if stop != StopMaxRounds {
		t.Fatalf("stop = %v, want StopMaxRounds", stop)
	}
	t.Log("Scenario B OK: text-only model burns all rounds, grace round returns no tool call, run stays incomplete")
}

// Scenario C (control group, same shape as Scenario A but in a normal
// round): task_done(DONE) in a regular round completes the run. This
// isolates grace-round behavior from the normal-round completion path.
func TestRunPerFile_TaskDoneInNormalRoundCompletes(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		taskDoneResponseWithArguments(`{"state":"DONE"}`),
	}}
	runner := NewRunner(graceRoundTestDeps(client, 5))

	completed, _, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"main.go",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed {
		t.Fatal("control failed: task_done in a normal round must complete")
	}
	t.Log("Control OK: task_done(DONE) in a normal round completes the run")
}

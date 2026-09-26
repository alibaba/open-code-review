// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// fakeAdversarialAwareClient tells the standard pass, the adversarial pass and
// the review filter apart by the system-message marker each carries, and
// records what the adversarial conversation was actually sent. Every test
// using it serializes dispatch (MaxConcurrency 1 over one single-file group),
// so the counters need no locking.
type fakeAdversarialAwareClient struct {
	path       string
	mainTokens int64
	advTokens  int64
	advErr     error
	// advNoDone makes the adversarial branch omit task_done, so its
	// conversation keeps looping — the shape needed to reach a stop that fires
	// between requests rather than at task_done.
	advNoDone bool

	mainCalls          int
	advCalls           int
	advLastUserMessage string

	// Filter-call tracking for the baseline-isolation test: the first filter
	// call approves, every later one reports its first candidate incorrect, so
	// a removal verdict lands on the pass's own comment rather than an
	// earlier round's.
	filterCalls    int
	filterRequests []string
}

func (f *fakeAdversarialAwareClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if len(req.Messages) > 0 && strings.Contains(req.Messages[0].ExtractText(), "review filter") {
		f.filterCalls++
		f.filterRequests = append(f.filterRequests, req.Messages[len(req.Messages)-1].ExtractText())
		if f.filterCalls == 1 {
			return approveAllCommentsChatResponse(), nil
		}
		return reportIncorrectCommentsChatResponse("c-0"), nil
	}
	if len(req.Messages) > 0 && strings.Contains(req.Messages[0].ExtractText(), "adversarial review") {
		f.advCalls++
		f.advLastUserMessage = req.Messages[len(req.Messages)-1].ExtractText()
		if f.advErr != nil {
			return nil, f.advErr
		}
		if f.advNoDone {
			return codeCommentOnlyChatResponse(f.path, "potential race condition here", f.advTokens), nil
		}
		return commentAndDoneChatResponse(f.path, "potential race condition here", f.advTokens), nil
	}
	f.mainCalls++
	return commentAndDoneChatResponse(f.path, "missing a nil check here", f.mainTokens), nil
}

// codeCommentOnlyChatResponse files one code_comment and keeps the
// conversation open (no task_done).
func codeCommentOnlyChatResponse(path, content string, tokens int64) *llm.ChatResponse {
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
				{ID: "1", Type: "function", Function: llm.FunctionCall{
					Name:      "code_comment",
					Arguments: `{"comments":[{"path":"` + path + `","content":"` + content + `"}]}`,
				}},
			}},
			FinishReason: "tool_calls",
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: tokens, TotalTokens: tokens},
	}
}

// commentAndDoneChatResponse files one code_comment and completes the
// conversation in the same turn — the shape both passes need to finish in a
// single round while leaving a finding behind.
func commentAndDoneChatResponse(path, content string, tokens int64) *llm.ChatResponse {
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
				{ID: "1", Type: "function", Function: llm.FunctionCall{
					Name:      "code_comment",
					Arguments: `{"comments":[{"path":"` + path + `","content":"` + content + `"}]}`,
				}},
				{ID: "2", Type: "function", Function: llm.FunctionCall{Name: "task_done", Arguments: "{}"}},
			}},
			FinishReason: "tool_calls",
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: tokens, TotalTokens: tokens},
	}
}

// approveAllCommentsChatResponse is the filter's approve verdict.
func approveAllCommentsChatResponse() *llm.ChatResponse {
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
				{ID: "1", Type: "function", Function: llm.FunctionCall{Name: "approve_all_comments", Arguments: "{}"}},
			}},
			FinishReason: "tool_calls",
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{},
	}
}

// reportIncorrectCommentsChatResponse is the filter's removal verdict, naming
// candidates by their order in the filter prompt.
func reportIncorrectCommentsChatResponse(ids ...string) *llm.ChatResponse {
	args, _ := json.Marshal(struct {
		CommentIDs []string `json:"comment_ids"`
	}{CommentIDs: ids})
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
				{ID: "1", Type: "function", Function: llm.FunctionCall{Name: "report_incorrect_comments", Arguments: string(args)}},
			}},
			FinishReason: "tool_calls",
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{},
	}
}

// adversarialAgentTestTemplate extends the minimal budget template with an
// adversarial conversation whose system message carries the marker the fake
// client branches on.
func adversarialAgentTestTemplate() template.Template {
	tpl := budgetAgentTestTemplate()
	tpl.AdversarialTask = &template.LlmConversation{
		Messages: []template.ChatMessage{
			{Role: "system", Content: "adversarial review"},
			{Role: "user", Content: "challenge {{diffs}} confirmed: {{confirmed_comments}}"},
		},
	}
	return tpl
}

func newAdversarialTestAgent(t *testing.T, fake *fakeAdversarialAwareClient, tpl template.Template, maxTokensBudget int64) *Agent {
	t.Helper()
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	a := New(Args{
		LLMClient:        fake,
		Model:            "fake",
		CommentCollector: collector,
		Tools:            reg,
		MaxConcurrency:   1,
		MaxTokensBudget:  maxTokensBudget,
		SkipFilter:       true, // the filter is an LLM call of its own
		Template:         tpl,
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment", Description: "comment"}},
		},
	})
	t.Cleanup(func() { _ = a.Session().Finalize() })
	return a
}

// TestDispatchSubtasks_AdversarialPassRunsAfterStandard pins the happy path:
// after the standard pass completes, the group gets exactly one adversarial
// conversation that carries the standard pass's finding as do-not-repeat
// context, whose comment lands in the shared collector, and whose session
// records are bucketed under the adversarial task type.
func TestDispatchSubtasks_AdversarialPassRunsAfterStandard(t *testing.T) {
	setTestHome(t, t.TempDir())
	diffs := makeBudgetDiffs(1)
	fake := &fakeAdversarialAwareClient{path: diffs[0].NewPath}
	tpl := adversarialAgentTestTemplate()
	tpl.MaxReviewRounds = 1 // the standard pass ends after its finding, so the pass ordering is deterministic
	a := newAdversarialTestAgent(t, fake, tpl, 0)
	a.diffs = diffs
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}

	if fake.mainCalls != 1 || fake.advCalls != 1 {
		t.Fatalf("calls: standard=%d adversarial=%d, want 1 and 1", fake.mainCalls, fake.advCalls)
	}

	// The adversarial conversation must receive the standard pass's finding as
	// do-not-repeat context, with every placeholder replaced.
	user := fake.advLastUserMessage
	if !strings.Contains(user, "missing a nil check here") {
		t.Errorf("adversarial prompt does not carry the standard pass's confirmed finding: %q", user)
	}
	if !strings.Contains(user, "+package x") {
		t.Errorf("adversarial prompt does not carry the group diff: %q", user)
	}
	if strings.Contains(user, "{{") {
		t.Errorf("adversarial prompt has an unreplaced placeholder: %q", user)
	}

	if len(comments) != 2 {
		t.Fatalf("comments = %d, want the standard and the adversarial finding", len(comments))
	}

	// The adversarial conversation is recorded under its own session task type.
	fs := a.Session().GetOrCreateFileSession(diffs[0].NewPath)
	if n := len(fs.TaskRecords[session.AdversarialTask]); n != 1 {
		t.Fatalf("adversarial session records = %d, want 1", n)
	}
	if n := len(fs.TaskRecords[session.MainTask]); n != 1 {
		t.Fatalf("main session records = %d, want 1", n)
	}

	if err := a.finalizeManifest(); err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	manifest := a.RunManifest()
	if manifest == nil || len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 0 {
		t.Fatalf("coverage = %+v, want the group completed", manifest)
	}
}

// TestDispatchSubtasks_AdversarialPassFailureKeepsGroupComplete pins the
// best-effort contract: an adversarial conversation that errors is recorded as
// a warning while the group stays completed and only the standard finding is
// reported.
func TestDispatchSubtasks_AdversarialPassFailureKeepsGroupComplete(t *testing.T) {
	setTestHome(t, t.TempDir())
	diffs := makeBudgetDiffs(1)
	fake := &fakeAdversarialAwareClient{
		path:   diffs[0].NewPath,
		advErr: errors.New("adversarial backend exploded"),
	}
	tpl := adversarialAgentTestTemplate()
	tpl.MaxReviewRounds = 1
	a := newAdversarialTestAgent(t, fake, tpl, 0)
	a.diffs = diffs
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatchSubtasks must not fail the group on an adversarial error: %v", err)
	}
	if fake.advCalls != 1 {
		t.Fatalf("adversarial calls = %d, want 1", fake.advCalls)
	}
	if len(comments) != 1 || comments[0].Content != "missing a nil check here" {
		t.Fatalf("comments = %+v, want the standard finding only", comments)
	}
	var found bool
	for _, w := range a.Warnings() {
		if w.Type == "adversarial_pass_failed" {
			found = true
		}
	}
	if !found {
		t.Error("expected an adversarial_pass_failed warning")
	}

	if err := a.finalizeManifest(); err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	manifest := a.RunManifest()
	if manifest == nil || len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 0 {
		t.Fatalf("coverage = %+v, want the group completed despite the adversarial failure", manifest)
	}
}

// TestDispatchSubtasks_AdversarialPassSkipsWhenBudgetExhausted covers the pass
// entry gate: a group whose standard round already pushed the run over budget
// must not start an adversarial conversation, must stay completed, and must
// not double the token_budget_reached warning the round gate already recorded.
func TestDispatchSubtasks_AdversarialPassSkipsWhenBudgetExhausted(t *testing.T) {
	setTestHome(t, t.TempDir())
	diffs := makeBudgetDiffs(1)
	budget := estimateDiffFileTokens(diffs[0]) * 4
	fake := &fakeAdversarialAwareClient{path: diffs[0].NewPath, mainTokens: budget + 1}
	tpl := adversarialAgentTestTemplate()
	tpl.MaxReviewRounds = 2
	a := newAdversarialTestAgent(t, fake, tpl, budget)
	a.diffs = diffs
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	if fake.advCalls != 0 {
		t.Fatalf("adversarial calls = %d, want 0 once the budget is exhausted", fake.advCalls)
	}
	if !a.BudgetExceeded() {
		t.Error("expected BudgetExceeded()==true")
	}
	warnings := 0
	for _, w := range a.Warnings() {
		if w.Type == "token_budget_reached" {
			warnings++
		}
	}
	if warnings != 1 {
		t.Errorf("token_budget_reached warnings = %d, want 1 (the round gate's)", warnings)
	}
	if len(comments) != 1 || comments[0].Content != "missing a nil check here" {
		t.Fatalf("comments = %+v, want the standard finding only", comments)
	}

	if err := a.finalizeManifest(); err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	manifest := a.RunManifest()
	if manifest == nil || len(manifest.Coverage.Completed) != 1 {
		t.Fatalf("coverage = %+v, want the group completed", manifest)
	}
}

// TestDispatchSubtasks_AdversarialPassSkippedWithoutTemplate is the
// regression guard for templates without an adversarial conversation: exactly
// one standard call, no adversarial call, behavior identical to before.
func TestDispatchSubtasks_AdversarialPassSkippedWithoutTemplate(t *testing.T) {
	setTestHome(t, t.TempDir())
	diffs := makeBudgetDiffs(1)
	fake := &fakeAdversarialAwareClient{path: diffs[0].NewPath}
	a := newAdversarialTestAgent(t, fake, budgetAgentTestTemplate(), 0)
	a.diffs = diffs
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	if fake.mainCalls != 1 || fake.advCalls != 0 {
		t.Fatalf("calls: standard=%d adversarial=%d, want 1 and 0", fake.mainCalls, fake.advCalls)
	}
}

// TestDispatchSubtasks_AdversarialPassStoppedByMidConversationBudget pins the
// in-conversation budget trip during the adversarial pass: the stop surfaces
// as the same token_budget_reached warning the main loop uses, the pass's
// partial findings stay banked, and the group still completes.
func TestDispatchSubtasks_AdversarialPassStoppedByMidConversationBudget(t *testing.T) {
	setTestHome(t, t.TempDir())
	diffs := makeBudgetDiffs(1)
	budget := estimateDiffFileTokens(diffs[0]) * 4
	// The standard pass is cheap; the adversarial conversation's first request
	// alone pushes the run past the budget, so its next loop iteration stops.
	// advNoDone keeps that conversation open — a task_done on turn one would
	// complete it before the between-request budget check ever runs.
	fake := &fakeAdversarialAwareClient{path: diffs[0].NewPath, mainTokens: 10, advTokens: budget + 1, advNoDone: true}
	tpl := adversarialAgentTestTemplate()
	tpl.MaxReviewRounds = 1
	a := newAdversarialTestAgent(t, fake, tpl, budget)
	a.diffs = diffs
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatchSubtasks must not fail the group on an adversarial budget stop: %v", err)
	}
	// One adversarial request, then the budget-stop grace round.
	if fake.advCalls != 2 {
		t.Fatalf("adversarial calls = %d, want 2 (one round + grace)", fake.advCalls)
	}
	if !a.BudgetExceeded() {
		t.Error("expected BudgetExceeded()==true after the in-conversation budget trip")
	}
	warnings := 0
	for _, w := range a.Warnings() {
		if w.Type == "token_budget_reached" {
			warnings++
		}
	}
	if warnings != 1 {
		t.Errorf("token_budget_reached warnings = %d, want 1", warnings)
	}
	if len(comments) < 1 {
		t.Fatalf("comments = %d, want the standard and adversarial findings", len(comments))
	}

	if err := a.finalizeManifest(); err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	manifest := a.RunManifest()
	if manifest == nil || len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 0 {
		t.Fatalf("coverage = %+v, want the group completed despite the adversarial stop", manifest)
	}
}

// TestExecuteGroupAdversarialPass_SkipsOnCappedConfirmedFindings drives the
// confirmed-list cap gate directly: a group whose confirmed findings already
// reached the cap must not start an adversarial conversation, because the
// do-not-repeat block would dominate the prompt it is meant to focus.
func TestExecuteGroupAdversarialPass_SkipsOnCappedConfirmedFindings(t *testing.T) {
	setTestHome(t, t.TempDir())
	diffs := makeBudgetDiffs(1)
	fake := &fakeAdversarialAwareClient{path: diffs[0].NewPath}
	tpl := adversarialAgentTestTemplate()
	a := newAdversarialTestAgent(t, fake, tpl, 0)
	a.diffs = diffs
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	confirmed := make([]model.LlmComment, confirmedCap)
	baseline := map[string]int{diffs[0].NewPath: 0}
	a.executeGroupAdversarialPass(context.Background(), FileGroup{Diffs: diffs},
		diffs[0].NewPath, "", "", "", confirmed, baseline)
	if fake.advCalls != 0 {
		t.Fatalf("adversarial calls = %d, want 0 when the confirmed list is capped", fake.advCalls)
	}
}

// TestExecuteGroupAdversarialPass_SkipsWhenPromptOverBudget drives the
// prompt-size gate directly: an adversarial prompt that does not fit the token
// ceiling skips the pass with a recorded warning instead of dispatching a
// conversation that would be rejected anyway.
func TestExecuteGroupAdversarialPass_SkipsWhenPromptOverBudget(t *testing.T) {
	setTestHome(t, t.TempDir())
	diffs := makeBudgetDiffs(1)
	fake := &fakeAdversarialAwareClient{path: diffs[0].NewPath}
	tpl := adversarialAgentTestTemplate()
	tpl.MaxTokens = 1
	a := newAdversarialTestAgent(t, fake, tpl, 0)
	a.diffs = diffs
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	baseline := map[string]int{diffs[0].NewPath: 0}
	a.executeGroupAdversarialPass(context.Background(), FileGroup{Diffs: diffs},
		diffs[0].NewPath, "", "", "", nil, baseline)
	if fake.advCalls != 0 {
		t.Fatalf("adversarial calls = %d, want 0 when the prompt exceeds the token ceiling", fake.advCalls)
	}
	var found bool
	for _, w := range a.Warnings() {
		if w.Type == "token_threshold_exceeded" {
			found = true
		}
	}
	if !found {
		t.Error("expected a token_threshold_exceeded warning from the prompt-size gate")
	}
}

// TestDispatchSubtasks_AdversarialPassFiltersOnlyItsOwnComments drives the
// baseline isolation the SkipFilter tests cannot see: with the filter enabled,
// the standard round's already-filtered comment is not resubmitted after the
// adversarial pass, the pass's own comment is the filter's only candidate, and
// a removal verdict on it keeps it out of the final result.
func TestDispatchSubtasks_AdversarialPassFiltersOnlyItsOwnComments(t *testing.T) {
	setTestHome(t, t.TempDir())
	diffs := makeBudgetDiffs(1)
	fake := &fakeAdversarialAwareClient{path: diffs[0].NewPath}
	tpl := adversarialAgentTestTemplate()
	tpl.MaxReviewRounds = 1
	tpl.ReviewFilterTask = &template.LlmConversation{
		Messages: []template.ChatMessage{
			{Role: "system", Content: "review filter"},
			{Role: "user", Content: "comments: {{comments}}"},
		},
	}
	// Same construction as newAdversarialTestAgent but with SkipFilter left
	// false — the filter's LLM call is the subject of this test.
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	a := New(Args{
		LLMClient:        fake,
		Model:            "fake",
		CommentCollector: collector,
		Tools:            reg,
		MaxConcurrency:   1,
		Template:         tpl,
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment", Description: "comment"}},
		},
	})
	a.diffs = diffs
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}

	if fake.mainCalls != 1 || fake.advCalls != 1 {
		t.Fatalf("calls: standard=%d adversarial=%d, want 1 and 1", fake.mainCalls, fake.advCalls)
	}
	if len(fake.filterRequests) != 2 {
		t.Fatalf("filter calls = %d, want 2 (one after the standard round, one after the pass)", len(fake.filterRequests))
	}

	// The first filter run judges the standard round's comment.
	if !strings.Contains(fake.filterRequests[0], "missing a nil check here") {
		t.Errorf("standard round's filter run does not carry the standard comment: %q", fake.filterRequests[0])
	}
	// The second filter run judges only the pass's own comment: the standard
	// comment was already filtered once and must not be resubmitted.
	if !strings.Contains(fake.filterRequests[1], "potential race condition here") {
		t.Errorf("adversarial filter run does not carry the pass's comment: %q", fake.filterRequests[1])
	}
	if strings.Contains(fake.filterRequests[1], "missing a nil check here") {
		t.Errorf("standard comment was resubmitted to the filter after the adversarial pass: %q", fake.filterRequests[1])
	}

	// The removal verdict on the pass's comment stands: only the standard
	// finding survives into the final result.
	if len(comments) != 1 || comments[0].Content != "missing a nil check here" {
		t.Fatalf("comments = %+v, want the standard finding only after the filter removed the adversarial one", comments)
	}

	fs := a.Session().GetOrCreateFileSession(diffs[0].NewPath)
	if n := len(fs.TaskRecords[session.ReviewFilterTask]); n != 2 {
		t.Fatalf("review filter session records = %d, want 2", n)
	}
}

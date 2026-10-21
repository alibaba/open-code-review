// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/progress"
	"github.com/alibaba/open-code-review/internal/tool"
)

// The events this file publishes are the dashboard's only view of a running
// review: a group that never reaches a terminal state keeps its row spinning
// and never advances the bar, so a finished run reports less than 100%
// covered.

// captureProgress collects the events fn publishes.
func captureProgress(t *testing.T, fn func()) []progress.Event {
	t.Helper()
	var got []progress.Event
	restore := progress.SetSinkForTest(func(ev progress.Event) {
		got = append(got, ev)
	})
	defer restore()
	fn()
	return got
}

// fakeCommentOnlyClient answers every request with a code_comment and no
// task_done. The group's rounds therefore end without the task completing,
// which is the state a group stopped short of completion is left in.
type fakeCommentOnlyClient struct {
	path  string
	calls int64 // atomic
}

func (f *fakeCommentOnlyClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	atomic.AddInt64(&f.calls, 1)
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{
				ID: "1", Type: "function", Function: llm.FunctionCall{
					Name:      "code_comment",
					Arguments: `{"comments":[{"path":"` + f.path + `","content":"missing a nil check here"}]}`,
				},
			}}},
			FinishReason: "tool_calls",
		}},
		Model: "fake",
	}, nil
}

// TestDispatchSubtasks_StoppedGroupPublishesATerminalEvent covers a group
// that stops short of completion after it has already produced a finding. The
// dashboard needs a terminal event for it: without one the row keeps spinning
// and the group's file never advances the bar.
func TestDispatchSubtasks_StoppedGroupPublishesATerminalEvent(t *testing.T) {
	setTestHome(t, t.TempDir())
	diffs := makeBudgetDiffs(1)
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	tpl := budgetAgentTestTemplate()
	tpl.MaxReviewRounds = 2
	a := New(Args{
		LLMClient:        &fakeCommentOnlyClient{path: diffs[0].NewPath},
		Model:            "fake",
		CommentCollector: collector,
		Tools:            reg,
		MaxConcurrency:   1,
		SkipFilter:       true,
		Template:         tpl,
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment", Description: "comment"}},
		},
	})
	t.Cleanup(func() { _ = a.Session().Finalize() })
	a.diffs = diffs
	a.currentDate = "2025-06-26 10:00"
	a.args.Tools.Freeze()

	var events []progress.Event
	restore := progress.SetSinkForTest(func(ev progress.Event) {
		events = append(events, ev)
	})
	defer restore()

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}

	var terminal bool
	for _, ev := range events {
		if ev.Kind == progress.KindGroupDone || ev.Kind == progress.KindGroupFailed {
			terminal = true
		}
	}
	if !terminal {
		t.Fatalf("a group that stopped short must publish a terminal event, got %+v", events)
	}
}

// The budget event must not carry text: the run already printed the budget
// line where the fact became known, and the plain sink would print it again.
func TestPublishBudgetReachedCarriesNoText(t *testing.T) {
	got := captureProgress(t, func() { publishBudgetReached() })

	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Kind != progress.KindBudgetReached {
		t.Errorf("kind = %v, want KindBudgetReached", got[0].Kind)
	}
	if got[0].Text != "" {
		t.Errorf("the budget event must carry no text, got %q", got[0].Text)
	}
}

// A group that stopped short accounts for its files in two events: the ones
// that produced comments are done, the rest failed. The dashboard's bar
// needs both to reach the group's full file count.
func TestPublishGroupDoneFilesCountsOnlyTheFilesThatFinished(t *testing.T) {
	got := captureProgress(t, func() { publishGroupDoneFiles("a.go,b.go", 1, 3) })

	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Kind != progress.KindGroupDone {
		t.Errorf("kind = %v, want KindGroupDone", got[0].Kind)
	}
	if got[0].Group != "a.go,b.go" {
		t.Errorf("group = %q, want the group's key", got[0].Group)
	}
	if got[0].Files != 1 {
		t.Errorf("files = %d, want 1", got[0].Files)
	}
	if got[0].Findings != 3 {
		t.Errorf("findings = %d, want 3", got[0].Findings)
	}
}

// A group publishes its own paths, so the row's file list comes from the
// event rather than from a second lookup.
func TestPublishGroupStartCarriesTheGroupsPaths(t *testing.T) {
	g := FileGroup{Label: "a label", Diffs: []model.Diff{{NewPath: "a.go"}, {NewPath: "b.go"}}}

	got := captureProgress(t, func() { publishGroupStart(g) })

	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Kind != progress.KindGroupStart {
		t.Errorf("kind = %v, want KindGroupStart", got[0].Kind)
	}
	if got[0].Group != "a.go,b.go" {
		t.Errorf("group = %q, want the file key, not the label", got[0].Group)
	}
	if len(got[0].Paths) != 2 {
		t.Errorf("paths = %v, want both files", got[0].Paths)
	}
}

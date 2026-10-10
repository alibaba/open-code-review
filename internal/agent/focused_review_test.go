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

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type focusedClientFunc func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error)

func (f focusedClientFunc) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return f(ctx, req)
}

func focusedTestAgent(t *testing.T, count int, client llm.LLMClient, budgets ...int64) (*Agent, FileGroup) {
	t.Helper()
	collector := tool.NewCommentCollector()
	registry := tool.NewRegistry()
	registry.Register(&tool.CodeCommentProvider{Collector: collector})
	registry.Freeze()
	tpl := budgetAgentTestTemplate()
	tpl.MaxReviewRounds = 2
	tpl.MainTask.Messages[1].Content += "\n{{confirmed_comments}}"
	var budget int64
	if len(budgets) > 0 {
		budget = budgets[0]
	}
	a := New(Args{
		Template: tpl, LLMClient: client, Model: "fake", Tools: registry,
		CommentCollector: collector, IntraGroupConcurrency: count, SkipFilter: true,
		CommentWorkerPool: NewCommentWorkerPool(2),
		MaxTokensBudget:   budget,
	})
	t.Cleanup(func() { a.runner.WaitBackground(); _ = a.Session().Finalize() })
	a.diffs = makeBudgetDiffs(1)
	return a, FileGroup{Diffs: a.diffs}
}

func focusedCommentResponse(content string) *llm.ChatResponse {
	resp := agentTaskDoneResponse()
	// Omit path to exercise the real single-file fallback with a focus task key.
	resp.Choices[0].Message.ToolCalls = append([]llm.ToolCall{{
		ID: "comment", Type: "function", Function: llm.FunctionCall{
			Name: "code_comment", Arguments: fmt.Sprintf(`{"comments":[{"content":%q}]}`, content),
		},
	}}, resp.Choices[0].Message.ToolCalls...)
	return resp
}

func TestFocusedReviewOverlapsAndKeepsSequentialFollowup(t *testing.T) {
	for _, count := range []int{2, 3} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var started atomic.Int32
			var calls atomic.Int32
			allStarted := make(chan struct{})
			var mu sync.Mutex
			keys := map[string]bool{}
			focusPrompts := map[string]bool{}
			client := focusedClientFunc(func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				calls.Add(1)
				last := fmt.Sprint(req.Messages[len(req.Messages)-1].Content)
				if strings.Contains(last, "Your focus:") {
					mu.Lock()
					keys[llm.SessionKeyFromContext(ctx)] = true
					focusPrompts[last] = true
					mu.Unlock()
					if started.Add(1) == int32(count) {
						close(allStarted)
					}
					select {
					case <-allStarted:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
					return focusedCommentResponse("shared finding"), nil
				}
				if started.Load() != int32(count) || !strings.Contains(last, "shared finding") {
					t.Error("follow-up did not receive the merged first-round findings")
				}
				return agentTaskDoneResponse(), nil
			})
			a, g := focusedTestAgent(t, count, client)
			completed, stop, err := a.executeGroupSubtask(ctx, g)
			if err != nil || !completed || stop != nil {
				t.Fatalf("completed=%v stop=%+v err=%v", completed, stop, err)
			}
			if calls.Load() != int32(count+1) || len(keys) != count || len(focusPrompts) != count {
				t.Fatalf("calls=%d distinct cache keys=%d focuses=%d", calls.Load(), len(keys), len(focusPrompts))
			}
			comments := a.args.CommentCollector.Comments()
			if len(comments) != 1 || comments[0].Path != g.Diffs[0].NewPath {
				t.Fatalf("comments=%+v; want one deduplicated finding on the actual path", comments)
			}
			if got := a.runner.TotalTokensUsed(); got != int64(15*(count+1)) {
				t.Fatalf("total tokens=%d; all focused and sequential calls must share accounting", got)
			}
		})
	}
}

func TestFocusedReviewEmptyFirstRoundStillGetsFollowup(t *testing.T) {
	for _, count := range []int{0, 1, 2, 3, 20} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			client := &fakeBudgetAgentClient{perCallTokens: 1}
			a, g := focusedTestAgent(t, count, client)
			completed, stop, err := a.executeGroupSubtask(context.Background(), g)
			want := int64(1)
			if count > 1 {
				want = int64(min(count, 3) + 1)
			}
			if !completed || stop != nil || err != nil || atomic.LoadInt64(&client.calls) != want {
				t.Fatalf("completed=%v stop=%+v err=%v calls=%d want=%d", completed, stop, err, client.calls, want)
			}
		})
	}
}

func TestFocusedReviewCancellationJoinsConversations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var started, finished atomic.Int32
	a, g := focusedTestAgent(t, 3, focusedClientFunc(func(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
		if started.Add(1) == 3 {
			cancel()
		}
		<-ctx.Done()
		finished.Add(1)
		return nil, ctx.Err()
	}))
	completed, _, err := a.executeGroupSubtask(ctx, g)
	if completed || !errors.Is(err, context.Canceled) || finished.Load() != 3 {
		t.Fatalf("completed=%v err=%v joined=%d", completed, err, finished.Load())
	}
}

func TestFocusedReviewFailureCannotBeHiddenBySibling(t *testing.T) {
	for _, mode := range []string{"error", "panic", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			a, g := focusedTestAgent(t, 2, focusedClientFunc(func(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				calls.Add(1)
				if strings.Contains(fmt.Sprint(req.Messages[2].Content), "Your focus: Security") {
					switch mode {
					case "error":
						return nil, errors.New("provider unavailable")
					case "panic":
						panic("broken provider")
					case "incomplete":
						return &llm.ChatResponse{}, nil
					}
				}
				return agentTaskDoneResponse(), nil
			}))
			a.args.Template.MaxReviewRounds = 1
			completed, stop, err := a.executeGroupSubtask(context.Background(), g)
			if completed || (stop == nil && err == nil) || calls.Load() < 2 {
				t.Fatalf("completed=%v stop=%+v err=%v calls=%d", completed, stop, err, calls.Load())
			}
			if mode == "panic" {
				if class, _ := classifyItemError(err); class != session.FailurePanic {
					t.Fatalf("panic classified as %s", class)
				}
			}
		})
	}
}

func TestFocusedReviewPromptAndAggregateBudgets(t *testing.T) {
	t.Run("focused prompt", func(t *testing.T) {
		client := &fakeBudgetAgentClient{perCallTokens: 1}
		a, g := focusedTestAgent(t, 3, client)
		a.args.Template.MaxTokens = 80
		completed, stop, err := a.executeGroupSubtask(context.Background(), g)
		if completed || err != nil || stop == nil || stop.class != session.FailureBudget || client.calls != 0 {
			t.Fatalf("completed=%v stop=%+v err=%v calls=%d", completed, stop, err, client.calls)
		}
		if a.BudgetExceeded() {
			t.Fatal("prompt overflow must not report aggregate token exhaustion")
		}
	})
	t.Run("shared aggregate", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var calls atomic.Int32
		ready := make(chan struct{})
		a, g := focusedTestAgent(t, 3, focusedClientFunc(func(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
			if calls.Add(1) == 3 {
				close(ready)
			}
			select {
			case <-ready:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return agentTaskDoneResponse(), nil
		}), 1)
		completed, stop, err := a.executeGroupSubtask(ctx, g)
		if !completed || stop != nil || err != nil || !a.BudgetExceeded() || calls.Load() != 3 {
			t.Fatalf("completed=%v stop=%+v err=%v budget=%v calls=%d", completed, stop, err, a.BudgetExceeded(), calls.Load())
		}
	})
}

func TestFocusedReviewExactDedupPreservesDistinctFindingsAndOtherPaths(t *testing.T) {
	a, g := focusedTestAgent(t, 2, &fakeBudgetAgentClient{})
	first := model.LlmComment{Path: g.Diffs[0].NewPath, Content: "first", StartLine: 1}
	a.args.CommentCollector.Add(first)
	duplicate := first
	duplicate.Thinking = "different reasoning"
	a.args.CommentCollector.Add(duplicate)
	distinct := first
	distinct.Content = "different defect on the same line"
	a.args.CommentCollector.Add(distinct)
	other := model.LlmComment{Path: "other.go", Content: "first"}
	a.args.CommentCollector.Add(other)
	a.removeExactGroupDuplicates(g, map[string]int{first.Path: 1})
	got := a.args.CommentCollector.Comments()
	if len(got) != 3 || got[0] != first || got[1] != distinct || got[2] != other {
		t.Fatalf("comments=%+v", got)
	}
}

func TestFocusedReviewBudgetSignalSurvivesSiblingError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var started atomic.Int32
	ready := make(chan struct{})
	var mu sync.Mutex
	requests := map[string]int{}
	a, g := focusedTestAgent(t, 2, focusedClientFunc(func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
		mu.Lock()
		requests[req.SessionID]++
		n := requests[req.SessionID]
		mu.Unlock()
		if n > 1 {
			return agentTaskDoneResponse(), nil
		}
		if started.Add(1) == 2 {
			close(ready)
		}
		select {
		case <-ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if strings.Contains(fmt.Sprint(req.Messages[2].Content), "Your focus: Security") {
			return nil, errors.New("provider unavailable")
		}
		return &llm.ChatResponse{Usage: &llm.UsageInfo{PromptTokens: 100}}, nil
	}), 50)
	completed, _, err := a.executeGroupSubtask(ctx, g)
	if completed || err == nil || !a.BudgetExceeded() {
		t.Fatalf("completed=%v err=%v budget=%v", completed, err, a.BudgetExceeded())
	}
}

func TestFocusedReviewIncompleteFocusIsNotReusableCoverage(t *testing.T) {
	a, _ := focusedTestAgent(t, 2, focusedClientFunc(func(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
		if strings.Contains(fmt.Sprint(req.Messages[2].Content), "Your focus: Security") {
			return focusedCommentResponse("valid security finding"), nil
		}
		return &llm.ChatResponse{}, nil
	}))
	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil || len(comments) != 1 {
		t.Fatalf("comments=%+v err=%v; want the usable partial finding", comments, err)
	}
	if err := a.finalizeManifest(); err != nil {
		t.Fatal(err)
	}
	manifest := a.RunManifest()
	if len(manifest.Coverage.Failed) != 1 || len(manifest.Coverage.Completed) != 0 {
		t.Fatalf("coverage=%+v; a successful focus must not hide its incomplete sibling", manifest.Coverage)
	}
}

func TestFocusedReviewLowEffortHasNoFollowup(t *testing.T) {
	client := &fakeBudgetAgentClient{perCallTokens: 1}
	a, g := focusedTestAgent(t, 2, client)
	a.args.Template.MaxReviewRounds = 1
	completed, stop, err := a.executeGroupSubtask(context.Background(), g)
	if !completed || stop != nil || err != nil || client.calls != 2 {
		t.Fatalf("completed=%v stop=%+v err=%v calls=%d", completed, stop, err, client.calls)
	}
}

func TestFocusedReviewDeduplicatesPartialFindingsOnError(t *testing.T) {
	var mu sync.Mutex
	requests := map[string]int{}
	a, g := focusedTestAgent(t, 2, focusedClientFunc(func(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
		mu.Lock()
		requests[req.SessionID]++
		n := requests[req.SessionID]
		mu.Unlock()
		if n > 1 {
			return nil, errors.New("provider unavailable after finding")
		}
		resp := focusedCommentResponse("shared partial finding")
		if strings.Contains(fmt.Sprint(req.Messages[2].Content), "Your focus: Security") {
			resp.Choices[0].Message.ToolCalls = resp.Choices[0].Message.ToolCalls[:1]
		}
		return resp, nil
	}))
	completed, _, err := a.executeGroupSubtask(context.Background(), g)
	comments := a.args.CommentCollector.Comments()
	if completed || err == nil || len(comments) != 1 {
		t.Fatalf("completed=%v err=%v comments=%+v", completed, err, comments)
	}
}

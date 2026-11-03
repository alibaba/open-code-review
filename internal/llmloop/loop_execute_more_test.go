// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// scriptedLLMClient returns a fixed sequence of responses, one per call,
// letting tests drive the full main loop turn by turn. requests records the
// Messages of every call it received, in order, so a test can inspect what
// history the previous round's response actually produced.
type scriptedLLMClient struct {
	responses []*llm.ChatResponse
	calls     int
	requests  [][]llm.Message
}

func (s *scriptedLLMClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	s.requests = append(s.requests, req.Messages)
	if s.calls >= len(s.responses) {
		return s.responses[len(s.responses)-1], nil
	}
	resp := s.responses[s.calls]
	s.calls++
	return resp, nil
}

// newThinkingTestRunner builds a Runner wired for a full RunMainTask pass:
// a scripted LLM client, the code_comment tool, and a comment collector.
func newThinkingTestRunner(t *testing.T, client llm.LLMClient) (*Runner, *tool.CommentCollector) {
	t.Helper()
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()
	r := NewRunner(Deps{
		LLMClient:        client,
		Model:            "test-model",
		Template:         template.Template{MaxToolRequestTimes: 5, MaxTokens: 1000000},
		Tools:            reg,
		CommentCollector: collector,
		MainToolDefs:     []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: tool.CodeComment.Name()}}},
		Session:          session.New(t.TempDir(), "main", "test-model", session.SessionOptions{ReviewMode: "diff"}),
	})
	return r, collector
}

func codeCommentResponse(reasoning, content string) *llm.ChatResponse {
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{
			Content:          &content,
			ReasoningContent: reasoning,
			ToolCalls: []llm.ToolCall{{
				ID:   "call_comment",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      tool.CodeComment.Name(),
					Arguments: `{"comments":[{"content":"issue","existing_code":"x"}]}`,
				},
			}},
		}}},
	}
}

// TestRunMainTask_BackfillsThinkingFromReasoningContent verifies the full
// wiring: the model's native reasoning_content on a tool-calling turn is
// backfilled into the comment's thinking, while the turn's assistant
// content is ignored.
func TestRunMainTask_BackfillsThinkingFromReasoningContent(t *testing.T) {
	client := &scriptedLLMClient{responses: []*llm.ChatResponse{
		codeCommentResponse("native reasoning", "I'll now leave a comment on this file"),
		taskDoneResponse(),
	}}
	r, collector := newThinkingTestRunner(t, client)

	ok, _, err := r.RunMainTask(context.Background(), []llm.Message{msg("user", "review")}, "file.go")
	if err != nil || !ok {
		t.Fatalf("RunMainTask = (%v, err %v), want completed", ok, err)
	}

	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatalf("collected %d comments, want 1", len(comments))
	}
	if comments[0].Thinking != "native reasoning" {
		t.Errorf("comment Thinking = %q, want native reasoning_content", comments[0].Thinking)
	}
}

// TestRunMainTask_NoFallbackToContent is a regression test: when a turn has
// assistant content but no reasoning_content, the comment thinking must stay
// empty. It fails if the removed `thinking = content` fallback returns.
func TestRunMainTask_NoFallbackToContent(t *testing.T) {
	client := &scriptedLLMClient{responses: []*llm.ChatResponse{
		codeCommentResponse("", "I'll now leave a comment on this file"),
		taskDoneResponse(),
	}}
	r, collector := newThinkingTestRunner(t, client)

	ok, _, err := r.RunMainTask(context.Background(), []llm.Message{msg("user", "review")}, "file.go")
	if err != nil || !ok {
		t.Fatalf("RunMainTask = (%v, err %v), want completed", ok, err)
	}

	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatalf("collected %d comments, want 1", len(comments))
	}
	if comments[0].Thinking != "" {
		t.Errorf("comment Thinking = %q, want empty (no content fallback)", comments[0].Thinking)
	}
}

// TestRunMainTask_DoesNotDuplicateReasoningIntoVisibleContent is a regression
// test found in review of the #805 fix: ChatResponse.Content() falls back to
// ReasoningContent when there's no visible text — the common shape for an
// openai-chat-completions tool-calling turn (content: null, reasoning_content
// set). If the loop built history from Content() instead of VisibleContent(),
// that same reasoning text would land in both the ordinary content field and
// the reasoning_content native payload on replay, doubling it on the wire.
func TestRunMainTask_DoesNotDuplicateReasoningIntoVisibleContent(t *testing.T) {
	emptyContent := ""
	reasoning := "private reasoning that must not be duplicated"
	reasoningResp := &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{
			Content:          &emptyContent,
			ReasoningContent: reasoning,
			Native:           llm.NativeTurn{Family: "openai-chat-completions", Payload: llm.ReasoningPayload(reasoning)},
			ToolCalls: []llm.ToolCall{{
				ID:   "call_comment",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      tool.CodeComment.Name(),
					Arguments: `{"comments":[{"content":"issue","existing_code":"x"}]}`,
				},
			}},
		}}},
	}
	client := &scriptedLLMClient{responses: []*llm.ChatResponse{reasoningResp, taskDoneResponse()}}
	r, _ := newThinkingTestRunner(t, client)

	ok, _, err := r.RunMainTask(context.Background(), []llm.Message{msg("user", "review")}, "file.go")
	if err != nil || !ok {
		t.Fatalf("RunMainTask = (%v, err %v), want completed", ok, err)
	}

	// requests[1] is the second call: its Messages carry the history built
	// from reasoningResp. The assistant message in it must have empty
	// visible Content — the reasoning text belongs solely to Native.
	if len(r.deps.LLMClient.(*scriptedLLMClient).requests) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(r.deps.LLMClient.(*scriptedLLMClient).requests))
	}
	secondReq := r.deps.LLMClient.(*scriptedLLMClient).requests[1]
	var assistantMsg *llm.Message
	for i := range secondReq {
		if secondReq[i].Role == "assistant" {
			assistantMsg = &secondReq[i]
		}
	}
	if assistantMsg == nil {
		t.Fatal("no assistant message found in the replayed history")
	}
	if assistantMsg.Content != "" {
		t.Errorf("assistant Content = %q, want empty — reasoning must not be duplicated into visible content", assistantMsg.Content)
	}
	if reasoning, ok := assistantMsg.Native.Payload.(llm.ReasoningPayload); !ok || string(reasoning) != "private reasoning that must not be duplicated" {
		t.Errorf("assistant Native.Payload = %#v, want the reasoning text preserved for replay", assistantMsg.Native.Payload)
	}
}

// TestExecuteToolCall_TaskDone covers every branch of the task_done handling:
// argument parse error, missing state (implicit completion), non-string state,
// explicit DONE / FAILED, and an unrecognized state value.
func TestExecuteToolCall_TaskDone(t *testing.T) {
	newRunner := func() *Runner {
		reg := tool.NewRegistry()
		reg.Freeze()
		return NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})
	}

	call := func(args string) tool.TaskCheckpoint {
		return newRunner().executeToolCall(context.Background(), "file.go", llm.ToolCall{
			Function: llm.FunctionCall{Name: tool.TaskDone.Name(), Arguments: args},
		}, nil, "")
	}

	t.Run("parse error", func(t *testing.T) {
		cp := call(`{bad`)
		if !strings.Contains(cp.Data, "Error parsing tool arguments") {
			t.Errorf("cp.Data = %q, want parse-error message", cp.Data)
		}
	})

	t.Run("missing state completes", func(t *testing.T) {
		cp := call(`{}`)
		if !cp.Completed || cp.Failed {
			t.Errorf("cp = %+v, want Completed", cp)
		}
	})

	t.Run("non-string state", func(t *testing.T) {
		cp := call(`{"state":123}`)
		if !strings.Contains(cp.Data, "must be DONE or FAILED") {
			t.Errorf("cp.Data = %q, want non-string state message", cp.Data)
		}
	})

	t.Run("DONE completes", func(t *testing.T) {
		cp := call(`{"state":"DONE"}`)
		if !cp.Completed || cp.Failed {
			t.Errorf("cp = %+v, want Completed", cp)
		}
	})

	t.Run("FAILED fails", func(t *testing.T) {
		cp := call(`{"state":"FAILED"}`)
		if !cp.Failed {
			t.Errorf("cp = %+v, want Failed", cp)
		}
	})

	t.Run("invalid state", func(t *testing.T) {
		cp := call(`{"state":"MAYBE"}`)
		if !strings.Contains(cp.Data, "invalid task_done state") {
			t.Errorf("cp.Data = %q, want invalid-state message", cp.Data)
		}
	})
}

// TestExecuteToolCall_CodeCommentAsyncPool covers the async dispatch path where a
// CommentWorkerPool is present: the call returns immediately with a success
// checkpoint, records "(async)" on the task record, and the comment lands in the
// collector once the pool drains.
func TestExecuteToolCall_CodeCommentAsyncPool(t *testing.T) {
	collector := tool.NewCommentCollector()
	pool := NewCommentWorkerPool(2)
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	r := NewRunner(Deps{
		Tools:             reg,
		CommentCollector:  collector,
		CommentWorkerPool: pool,
	})

	rec := &session.TaskRecord{}
	cp := r.executeToolCall(context.Background(), "async.go", llm.ToolCall{
		Function: llm.FunctionCall{
			Name:      tool.CodeComment.Name(),
			Arguments: `{"comments":[{"content":"issue","existing_code":"foo","path":"async.go"}]}`,
		},
	}, rec, "")

	if cp.Data != tool.CommentSucceed {
		t.Fatalf("cp.Data = %q, want CommentSucceed", cp.Data)
	}
	if len(rec.ToolResults) != 1 || rec.ToolResults[0].Result != "(async)" {
		t.Errorf("recorded results = %+v, want one (async) entry", rec.ToolResults)
	}

	// Drain the pool and confirm the comment was collected with the injected path.
	comments := r.CollectPendingComments()
	if len(comments) != 1 {
		t.Fatalf("collected %d comments, want 1", len(comments))
	}
	if comments[0].Path != "async.go" {
		t.Errorf("comment path = %q, want async.go", comments[0].Path)
	}
}

// TestExecuteToolCall_CodeCommentDiffResolved covers the synchronous code_comment
// path where DiffLookup returns a diff and ResolveComment resolves the line
// numbers from file content (so the re-location LLM branch is skipped), with a
// non-nil task record so AddToolResult runs.
func TestExecuteToolCall_CodeCommentDiffResolved(t *testing.T) {
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	diffLookup := func(path string) *model.Diff {
		return &model.Diff{
			NewPath:        path,
			NewFileContent: "line one\nfoo bar\nline three\n",
		}
	}

	r := NewRunner(Deps{
		Tools:            reg,
		CommentCollector: collector,
		DiffLookup:       diffLookup,
	})

	rec := &session.TaskRecord{}
	cp := r.executeToolCall(context.Background(), "resolved.go", llm.ToolCall{
		Function: llm.FunctionCall{
			Name:      tool.CodeComment.Name(),
			Arguments: `{"comments":[{"content":"issue","existing_code":"foo bar","path":"resolved.go"}]}`,
		},
	}, rec, "")

	if cp.Data != tool.CommentSucceed {
		t.Fatalf("cp.Data = %q, want CommentSucceed", cp.Data)
	}
	if len(rec.ToolResults) != 1 || rec.ToolResults[0].Result != tool.CommentSucceed {
		t.Errorf("recorded results = %+v, want one success entry", rec.ToolResults)
	}

	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatalf("collected %d comments, want 1", len(comments))
	}
	// ResolveComment should have located "foo bar" on line 2 of NewFileContent.
	if comments[0].StartLine != 2 {
		t.Errorf("comment StartLine = %d, want 2 (resolved from file content)", comments[0].StartLine)
	}
}

// TestExecuteToolCall_CodeCommentThinkingBackfill covers the thinking backfill:
// when the current turn carries reasoning content, comments without an explicit
// thinking get the turn reasoning; explicit thinking wins.
func TestExecuteToolCall_CodeCommentThinkingBackfill(t *testing.T) {
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	r := NewRunner(Deps{Tools: reg, CommentCollector: collector})

	cp := r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
		Function: llm.FunctionCall{
			Name: tool.CodeComment.Name(),
			Arguments: `{"comments":[` +
				`{"content":"a","existing_code":"x"},` +
				`{"content":"b","existing_code":"y","thinking":"explicit"}]}`,
		},
	}, nil, "turn reasoning")

	if cp.Data != tool.CommentSucceed {
		t.Fatalf("cp.Data = %q, want CommentSucceed", cp.Data)
	}
	comments := collector.Comments()
	if len(comments) != 2 {
		t.Fatalf("collected %d comments, want 2", len(comments))
	}
	if comments[0].Thinking != "turn reasoning" {
		t.Errorf("comments[0].Thinking = %q, want backfilled turn reasoning", comments[0].Thinking)
	}
	if comments[1].Thinking != "explicit" {
		t.Errorf("comments[1].Thinking = %q, want explicit thinking preserved", comments[1].Thinking)
	}
}

// TestExecuteToolCall_CodeCommentNoReasoning verifies comments keep an empty
// thinking when the turn has no reasoning content.
func TestExecuteToolCall_CodeCommentNoReasoning(t *testing.T) {
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	r := NewRunner(Deps{Tools: reg, CommentCollector: collector})

	cp := r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
		Function: llm.FunctionCall{
			Name:      tool.CodeComment.Name(),
			Arguments: `{"comments":[{"content":"a","existing_code":"x"}]}`,
		},
	}, nil, "")

	if cp.Data != tool.CommentSucceed {
		t.Fatalf("cp.Data = %q, want CommentSucceed", cp.Data)
	}
	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatalf("collected %d comments, want 1", len(comments))
	}
	if comments[0].Thinking != "" {
		t.Errorf("comments[0].Thinking = %q, want empty", comments[0].Thinking)
	}
}

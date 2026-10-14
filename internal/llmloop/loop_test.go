// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type fakeClient struct {
	responses []*llm.ChatResponse
	requests  []llm.ChatRequest
	calls     int
	// sessionKeys records llm.SessionKeyFromContext for each call.
	sessionKeys []string
}

func (f *fakeClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	f.requests = append(f.requests, req)
	f.sessionKeys = append(f.sessionKeys, llm.SessionKeyFromContext(ctx))
	if f.calls >= len(f.responses) {
		content := ""
		return &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
			Model:   "fake",
		}, nil
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
}

func taskDoneResponseWithArguments(arguments string) *llm.ChatResponse {
	content := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &content,
				ToolCalls: []llm.ToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "task_done",
						Arguments: arguments,
					},
				}},
			},
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5},
	}
}

func taskDoneResponse() *llm.ChatResponse {
	return taskDoneResponseWithArguments(`{}`)
}

func fileReadToolCallResponse(callID, args string) *llm.ChatResponse {
	content := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &content,
				ToolCalls: []llm.ToolCall{{
					ID:   callID,
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "file_read",
						Arguments: args,
					},
				}},
			},
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 20, CompletionTokens: 10},
	}
}

type fakeFileReadProvider struct {
	result string
}

func (f *fakeFileReadProvider) Tool() tool.Tool { return tool.FileRead }
func (f *fakeFileReadProvider) Execute(_ context.Context, _ map[string]any) (string, error) {
	return f.result, nil
}

func newTestDeps(client llm.LLMClient) Deps {
	reg := tool.NewRegistry()
	reg.Register(&fakeFileReadProvider{result: "package main\n"})
	return Deps{
		LLMClient:        client,
		Model:            "fake",
		Template:         template.Template{MaxTokens: 100000, MaxToolRequestTimes: 10},
		Tools:            reg,
		CommentCollector: tool.NewCommentCollector(),
		Session:          session.New("/tmp/test-repo", "main", "fake", session.SessionOptions{}),
	}
}

func assertToolTurnHistory(t *testing.T, messages []llm.Message, resp *llm.ChatResponse, wantResults map[string]string) {
	t.Helper()
	calls := resp.ToolCalls()
	if len(messages) != 1+len(calls) {
		t.Fatalf("history has %d messages, want assistant plus %d tool results", len(messages), len(calls))
	}
	wantAssistant := llm.NewToolCallMessage(resp.VisibleContent(), calls, resp.Native(), resp.ReasoningContent())
	if !reflect.DeepEqual(messages[0], wantAssistant) {
		t.Fatalf("assistant history = %+v, want %+v", messages[0], wantAssistant)
	}
	for i, call := range calls {
		result := messages[i+1]
		if result.Role != "tool" || result.ToolCallID != call.ID {
			t.Errorf("result %d = %+v, want tool result for %s", i, result, call.ID)
		}
		want, ok := wantResults[call.ID]
		if !ok || !strings.Contains(result.ExtractText(), want) {
			t.Errorf("result for %s = %q, want %q", call.ID, result.ExtractText(), want)
		}
	}
}

func TestRunMainTask_TruncatedTaskDoneContinues(t *testing.T) {
	for _, reason := range []string{"length", "max_tokens"} {
		for _, tt := range []struct {
			args       string
			wantResult string
		}{
			{`{}`, "call task_done again in a complete response"},
			{`{"state":"DONE"}`, "call task_done again in a complete response"},
			{`{"state":`, "Error parsing tool arguments"},
		} {
			t.Run(reason+"/"+tt.args, func(t *testing.T) {
				resp := taskDoneResponseWithArguments(tt.args)
				resp.Choices[0].FinishReason = reason
				client := &fakeClient{responses: []*llm.ChatResponse{resp, taskDoneResponse()}}
				runner := NewRunner(newTestDeps(client))
				msgs := []llm.Message{msg("system", "review"), msg("user", "main.go")}
				completed, stop, err := runner.RunMainTask(context.Background(), msgs, "main.go")
				if err != nil || !completed || stop != StopNone {
					t.Fatalf("RunMainTask = (%v, %v, %v), want completion after continuation", completed, stop, err)
				}
				if len(client.requests) != 2 {
					t.Fatalf("LLM requests = %d, want 2", len(client.requests))
				}
				assertToolTurnHistory(t, client.requests[1].Messages[2:], resp, map[string]string{
					"call_1": tt.wantResult,
				})
				warnings := runner.Warnings()
				if len(warnings) != 1 || warnings[0].Type != "response_truncated" || warnings[0].File != "main.go" || !strings.Contains(warnings[0].Message, reason) {
					t.Fatalf("warnings = %+v, want one truncation warning", warnings)
				}
			})
		}
	}
}

func TestRunMainTask_TruncatedTaskDoneFailed(t *testing.T) {
	resp := taskDoneResponseWithArguments(`{"state":"FAILED"}`)
	resp.Choices[0].FinishReason = "length"
	client := &fakeClient{responses: []*llm.ChatResponse{resp}}
	runner := NewRunner(newTestDeps(client))
	completed, stop, err := runner.RunMainTask(context.Background(), []llm.Message{msg("user", "review")}, "main.go")
	if completed || stop != StopNone || err == nil || !strings.Contains(err.Error(), "task_done reported FAILED") {
		t.Fatalf("RunMainTask = (%v, %v, %v), want terminal failure", completed, stop, err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("LLM requests = %d, want no retry after FAILED", len(client.requests))
	}
}

func TestRunMainTask_TruncatedToolsExecuteOnce(t *testing.T) {
	for _, tt := range []struct {
		name      string
		doneFirst bool
		async     bool
	}{
		{name: "comment first"},
		{name: "done first", doneFirst: true},
		{name: "async comment first", async: true},
		{name: "async done first", doneFirst: true, async: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp := codeCommentResponse("review reasoning", "Submitting a finding")
			resp.Choices[0].FinishReason = "length"
			resp.Choices[0].Message.Native = llm.NativeTurn{Family: "openai-chat-completions", Payload: llm.ReasoningPayload("review reasoning")}
			calls := resp.ToolCalls()
			calls[0].ExtraContent = json.RawMessage(`{"signature":"preserved"}`)
			done := taskDoneResponse().ToolCalls()[0]
			if tt.doneFirst {
				calls = append([]llm.ToolCall{done}, calls...)
			} else {
				calls = append(calls, done)
			}
			done.ID = "call_done_2"
			calls = append(calls, done, fileReadToolCallResponse("call_read", `{"path":"main.go"}`).ToolCalls()[0])
			resp.Choices[0].Message.ToolCalls = calls
			client := &fakeClient{responses: []*llm.ChatResponse{resp, taskDoneResponse()}}
			deps := newTestDeps(client)
			deps.Tools.Register(&tool.CodeCommentProvider{Collector: deps.CommentCollector})
			if tt.async {
				deps.CommentWorkerPool = NewCommentWorkerPool(1)
				t.Cleanup(func() { deps.CommentWorkerPool.Await() })
			}
			runner := NewRunner(deps)
			msgs := []llm.Message{msg("system", "review"), msg("user", "main.go")}
			completed, stop, err := runner.RunMainTask(context.Background(), msgs, "main.go")
			if deps.CommentWorkerPool != nil {
				deps.CommentWorkerPool.AwaitKey("main.go")
			}
			if err != nil || !completed || stop != StopNone {
				t.Fatalf("RunMainTask = (%v, %v, %v), want completion after continuation", completed, stop, err)
			}
			if len(client.requests) != 2 {
				t.Fatalf("LLM requests = %d, want 2", len(client.requests))
			}
			assertToolTurnHistory(t, client.requests[1].Messages[2:], resp, map[string]string{
				"call_comment": tool.CommentSucceed,
				"call_read":    "package main",
				"call_1":       "without repeating successful tool calls",
				"call_done_2":  "without repeating successful tool calls",
			})
			if got := runner.ToolCalls(); got["code_comment"] != 1 || got["file_read"] != 1 {
				t.Fatalf("tool calls = %+v, want each ordinary tool executed once", got)
			}
			if comments := deps.CommentCollector.Comments(); len(comments) != 1 || comments[0].Content != "issue" {
				t.Fatalf("comments = %+v, want one saved finding", comments)
			}
			if warnings := runner.Warnings(); len(warnings) != 1 || warnings[0].Type != "response_truncated" {
				t.Fatalf("warnings = %+v, want one warning per response", warnings)
			}
		})
	}
}

func TestRunMainTask_TruncatedWithoutToolsContinues(t *testing.T) {
	for _, tt := range []struct {
		name      string
		content   string
		reasoning string
	}{
		{name: "text", content: "Partial review"},
		{name: "empty"},
		{name: "reasoning", reasoning: "Partial reasoning"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp := &llm.ChatResponse{Choices: []llm.Choice{{
				FinishReason: "max_tokens",
				Message:      llm.ResponseMessage{Content: &tt.content, ReasoningContent: tt.reasoning},
			}}}
			client := &fakeClient{responses: []*llm.ChatResponse{resp, taskDoneResponse()}}
			runner := NewRunner(newTestDeps(client))
			msgs := []llm.Message{msg("system", "review"), msg("user", "main.go")}
			completed, _, err := runner.RunMainTask(context.Background(), msgs, "main.go")
			if err != nil || !completed || len(client.requests) != 2 {
				t.Fatalf("completed=%v err=%v requests=%d, want continuation then completion", completed, err, len(client.requests))
			}
			history := client.requests[1].Messages[2:]
			if tt.content != "" || tt.reasoning != "" {
				assertToolTurnHistory(t, history[:len(history)-1], resp, nil)
			} else if len(history) != 1 {
				t.Fatalf("empty response history = %+v, want only continuation prompt", history)
			}
			prompt := history[len(history)-1]
			if prompt.Role != "user" || !strings.Contains(prompt.ExtractText(), "truncated") || !strings.Contains(prompt.ExtractText(), "Continue the review from where you stopped") {
				t.Fatalf("missing truncation continuation prompt: %+v", prompt)
			}
		})
	}
}

func TestRunMainTask_TruncatedResponsesRespectLimits(t *testing.T) {
	for _, tt := range []struct {
		name         string
		noTools      bool
		maxRounds    int
		budget       int64
		wantStop     MainLoopStop
		wantRequests int
	}{
		{name: "empty rounds", maxRounds: 10, wantStop: StopEmptyRounds, wantRequests: 3},
		{name: "round limit", maxRounds: 1, wantStop: StopMaxRounds, wantRequests: 2},
		{name: "token budget", maxRounds: 10, budget: 1, wantStop: StopTokenBudget, wantRequests: 2},
		{name: "no tools round limit", noTools: true, maxRounds: 2, wantStop: StopMaxRounds, wantRequests: 3},
		{name: "no tools token budget", noTools: true, maxRounds: 10, budget: 1, wantStop: StopTokenBudget, wantRequests: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp := taskDoneResponse()
			resp.Choices[0].FinishReason = "length"
			if tt.noTools {
				resp.Choices[0].Message.ToolCalls = nil
			}
			responses := make([]*llm.ChatResponse, tt.wantRequests)
			for i := range responses {
				responses[i] = resp
			}
			if tt.wantStop != StopEmptyRounds {
				responses[len(responses)-1] = taskDoneResponse()
			}
			client := &fakeClient{responses: responses}
			deps := newTestDeps(client)
			deps.Template.MaxToolRequestTimes = tt.maxRounds
			deps.MaxTokensBudget = tt.budget
			deps.MainToolDefs = []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "task_done"}}}
			runner := NewRunner(deps)
			completed, stop, err := runner.RunMainTask(context.Background(), []llm.Message{msg("system", "review"), msg("user", "main.go")}, "main.go")
			if err != nil || completed || stop != tt.wantStop {
				t.Fatalf("RunMainTask = (%v, %v, %v), want (false, %v, nil)", completed, stop, err, tt.wantStop)
			}
			if len(client.requests) != tt.wantRequests {
				t.Fatalf("LLM requests = %d, want %d including any grace round", len(client.requests), tt.wantRequests)
			}
		})
	}
}

func TestRunMainTask_TaskDoneImmediately(t *testing.T) {
	resp := taskDoneResponse()
	resp.Choices[0].FinishReason = "tool_calls"
	client := &fakeClient{responses: []*llm.ChatResponse{resp}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review this file")}
	completed, _, err := runner.RunMainTask(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done to complete RunMainTask")
	}
	if client.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", client.calls)
	}
	if runner.TotalInputTokens() != 10 {
		t.Errorf("TotalInputTokens = %d, want 10", runner.TotalInputTokens())
	}
	if runner.TotalOutputTokens() != 5 {
		t.Errorf("TotalOutputTokens = %d, want 5", runner.TotalOutputTokens())
	}
	if warnings := runner.Warnings(); len(warnings) != 0 {
		t.Errorf("unexpected warnings: %+v", warnings)
	}
}

func TestRunMainTask_UsesCompletionTokenLimit(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{taskDoneResponse()}}
	deps := newTestDeps(client)
	deps.Template.MaxTokens = 200000
	deps.Template.MaxCompletionTokens = 58888
	runner := NewRunner(deps)

	_, _, err := runner.RunMainTask(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"main.go",
	)
	if err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	if got := client.requests[0].MaxTokens; got != 58888 {
		t.Fatalf("request MaxTokens = %d, want 58888", got)
	}
}

func TestRunMainTask_TaskDoneExplicitDone(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		taskDoneResponseWithArguments(`{"state":"DONE"}`),
	}}
	runner := NewRunner(newTestDeps(client))

	completed, _, err := runner.RunMainTask(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review this file")},
		"main.go",
	)
	if err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done DONE to complete RunMainTask")
	}
}

func TestRunMainTask_TaskDoneFailed(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		taskDoneResponseWithArguments(`{"state":"FAILED"}`),
	}}
	runner := NewRunner(newTestDeps(client))

	completed, _, err := runner.RunMainTask(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review this file")},
		"main.go",
	)
	if err == nil || !strings.Contains(err.Error(), "task_done reported FAILED") {
		t.Fatalf("expected task_done FAILED error, got %v", err)
	}
	if completed {
		t.Fatal("task_done FAILED must not complete RunMainTask")
	}
	if client.calls != 1 {
		t.Fatalf("expected terminal failure after 1 LLM call, got %d", client.calls)
	}
}

func TestRunMainTask_InvalidTaskDoneStateRetries(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
	}{
		{name: "unknown state", arguments: `{"state":"UNKNOWN"}`},
		{name: "empty state", arguments: `{"state":""}`},
		{name: "non-string state", arguments: `{"state":1}`},
		{name: "malformed arguments", arguments: `{"state":`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeClient{responses: []*llm.ChatResponse{
				taskDoneResponseWithArguments(tt.arguments),
				taskDoneResponseWithArguments(`{"state":"DONE"}`),
			}}
			runner := NewRunner(newTestDeps(client))

			completed, _, err := runner.RunMainTask(
				context.Background(),
				[]llm.Message{llm.NewTextMessage("user", "review this file")},
				"main.go",
			)
			if err != nil {
				t.Fatalf("RunMainTask: %v", err)
			}
			if !completed {
				t.Fatal("expected retry to complete with task_done DONE")
			}
			if client.calls != 2 {
				t.Fatalf("expected invalid state to be retried, got %d LLM calls", client.calls)
			}
		})
	}
}

func TestRunMainTask_TagsRequestsWithTaskSessionKey(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		taskDoneResponse(),
	}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review this file")}
	if _, _, err := runner.RunMainTask(context.Background(), msgs, "main.go"); err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}

	want := llm.SessionTaskKey(deps.Session.SessionID, string(session.MainTask), "main.go")
	if len(client.sessionKeys) != 2 {
		t.Fatalf("expected 2 recorded session keys, got %d", len(client.sessionKeys))
	}
	for i, got := range client.sessionKeys {
		if got != want {
			t.Errorf("call %d session key = %q, want %q", i, got, want)
		}
	}
}

func TestRunMainTask_ToolCallThenDone(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		taskDoneResponse(),
	}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, _, err := runner.RunMainTask(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done to complete RunMainTask")
	}
	if client.calls != 2 {
		t.Errorf("expected 2 LLM calls, got %d", client.calls)
	}

	toolCalls := runner.ToolCalls()
	if toolCalls["file_read"] != 1 {
		t.Errorf("file_read calls = %d, want 1", toolCalls["file_read"])
	}
	if runner.TotalInputTokens() != 30 {
		t.Errorf("TotalInputTokens = %d, want 30", runner.TotalInputTokens())
	}
}

func TestRunMainTask_ContextCancelled(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{taskDoneResponse()}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, _, err := runner.RunMainTask(ctx, msgs, "main.go")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
	if completed {
		t.Fatal("cancelled context should not complete RunMainTask")
	}
}

func TestRunMainTask_UnknownTool(t *testing.T) {
	content := ""
	unknownToolResp := &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &content,
				ToolCalls: []llm.ToolCall{{
					ID:   "call_x",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "nonexistent_tool",
						Arguments: `{}`,
					},
				}},
			},
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 5, CompletionTokens: 5},
	}
	client := &fakeClient{responses: []*llm.ChatResponse{unknownToolResp, taskDoneResponse()}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, _, err := runner.RunMainTask(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done to complete RunMainTask")
	}
	if client.calls != 2 {
		t.Errorf("expected 2 calls, got %d", client.calls)
	}
}

func TestRunMainTask_MaxToolRequestsWithoutTaskDoneDoesNotComplete(t *testing.T) {
	content := ""
	client := &fakeClient{responses: []*llm.ChatResponse{{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
		Model:   "fake",
		Usage:   &llm.UsageInfo{PromptTokens: 5, CompletionTokens: 5},
	}}}
	deps := newTestDeps(client)
	deps.Template.MaxToolRequestTimes = 1
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, stop, err := runner.RunMainTask(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	if completed {
		t.Fatal("RunMainTask completed without task_done")
	}
	if stop != StopMaxRounds {
		t.Fatalf("expected StopMaxRounds, got %v", stop)
	}
}

func TestRunMainTask_EmptyToolResultsStopWithEmptyRounds(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		fileReadToolCallResponse("call_2", `{"path":"main.go"}`),
		fileReadToolCallResponse("call_3", `{"path":"main.go"}`),
	}}
	deps := newTestDeps(client)
	reg := tool.NewRegistry()
	reg.Register(&fakeFileReadProvider{result: ""})
	deps.Tools = reg
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, stop, err := runner.RunMainTask(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	if completed {
		t.Fatal("RunMainTask completed without task_done")
	}
	if stop != StopEmptyRounds {
		t.Fatalf("stop = %v, want StopEmptyRounds", stop)
	}
	if client.calls != 3 {
		t.Fatalf("LLM calls = %d, want 3 empty rounds", client.calls)
	}
}

func TestRunMainTask_UncompressibleContextStopsWithCompression(t *testing.T) {
	emptySummary := ""
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &emptySummary}}},
			Model:   "fake",
		},
	}}
	deps := newTestDeps(client)
	deps.Template.MaxTokens = 20
	deps.Template.MemoryCompressionTask = template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "Summarize: {{context}}"}},
	}
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", strings.Repeat("word ", 100))}
	completed, stop, err := runner.RunMainTask(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	if completed {
		t.Fatal("RunMainTask completed without task_done")
	}
	if stop != StopCompression {
		t.Fatalf("stop = %v, want StopCompression", stop)
	}
	if client.calls != 2 {
		t.Fatalf("LLM calls = %d, want one main call and one compression call", client.calls)
	}
}

func TestRunner_RecordWarning(t *testing.T) {
	deps := newTestDeps(&fakeClient{})
	runner := NewRunner(deps)

	runner.RecordWarning("token_limit", "a.go", "approaching token limit")
	runner.RecordWarning("parse_error", "b.go", "invalid JSON")

	warnings := runner.Warnings()
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d", len(warnings))
	}
	if warnings[0].Type != "token_limit" {
		t.Errorf("Type = %q", warnings[0].Type)
	}
	if warnings[1].File != "b.go" {
		t.Errorf("File = %q", warnings[1].File)
	}
}

func TestRunner_RecordUsage(t *testing.T) {
	deps := newTestDeps(&fakeClient{})
	runner := NewRunner(deps)

	runner.RecordUsage(&llm.UsageInfo{
		PromptTokens:     100,
		CompletionTokens: 50,
		CacheReadTokens:  20,
		CacheWriteTokens: 10,
	})
	runner.RecordUsage(nil)

	if runner.TotalInputTokens() != 100 {
		t.Errorf("input = %d", runner.TotalInputTokens())
	}
	if runner.TotalOutputTokens() != 50 {
		t.Errorf("output = %d", runner.TotalOutputTokens())
	}
	if runner.TotalCacheReadTokens() != 20 {
		t.Errorf("cache read = %d", runner.TotalCacheReadTokens())
	}
	if runner.TotalCacheWriteTokens() != 10 {
		t.Errorf("cache write = %d", runner.TotalCacheWriteTokens())
	}
	if runner.TotalTokensUsed() != 150 {
		t.Errorf("total = %d", runner.TotalTokensUsed())
	}
}

// argsCapturingProvider records the args map Execute receives, so tests can
// assert the runner never hands tools a nil map.
type argsCapturingProvider struct {
	tool     tool.Tool
	gotArgs  map[string]any
	captured bool
}

func (p *argsCapturingProvider) Tool() tool.Tool { return p.tool }
func (p *argsCapturingProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	p.gotArgs = args
	p.captured = true
	return "ok", nil
}

func TestExecuteToolCall_ArgumentsEdgeCases(t *testing.T) {
	// Regression for #382: some OpenAI-compatible gateways emit
	// "arguments": null; json.Unmarshal("null", &m) leaves m nil, and the
	// code_comment path override then panicked with "assignment to entry
	// in nil map".
	tests := []struct {
		name           string
		toolName       string
		arguments      string
		wantContains   string // substring expected in cp.Data ("" = skip)
		wantComment    string // if non-empty, expect one collected comment with this path
		wantNonNilArgs bool   // dynamic tool: Execute must receive a non-nil args map
		wantFailure    bool
	}{
		{
			name:         "null args on code_comment (issue #382)",
			toolName:     "code_comment",
			arguments:    `null`,
			wantContains: "'comments' array is required",
			wantFailure:  true,
		},
		{
			name:         "empty object on code_comment",
			toolName:     "code_comment",
			arguments:    `{}`,
			wantContains: "'comments' array is required",
			wantFailure:  true,
		},
		{
			name:        "valid args uses per-item path",
			toolName:    "code_comment",
			arguments:   `{"comments":[{"content":"issue","existing_code":"foo","path":"item.go"}]}`,
			wantComment: "item.go",
		},
		{
			name:         "empty string args",
			toolName:     "code_comment",
			arguments:    ``,
			wantContains: "Error parsing tool arguments",
			wantFailure:  true,
		},
		{
			name:         "malformed json args",
			toolName:     "code_comment",
			arguments:    `{"comments":`,
			wantContains: "Error parsing tool arguments",
			wantFailure:  true,
		},
		{
			name:           "null args on dynamic tool",
			toolName:       "dyn_echo",
			arguments:      `null`,
			wantNonNilArgs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := tool.NewCommentCollector()
			dyn := &argsCapturingProvider{tool: tool.Dynamic("dyn_echo")}
			reg := tool.NewRegistry()
			reg.Register(&tool.CodeCommentProvider{Collector: collector})
			reg.Register(dyn)
			reg.Freeze()

			r := NewRunner(Deps{
				Tools:            reg,
				CommentCollector: collector,
			})

			cp := r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
				Function: llm.FunctionCall{
					Name:      tt.toolName,
					Arguments: tt.arguments,
				},
			}, nil, "")

			if tt.wantContains != "" && !strings.Contains(cp.Data, tt.wantContains) {
				t.Errorf("cp.Data = %q, want substring %q", cp.Data, tt.wantContains)
			}
			if tt.wantComment != "" {
				comments := collector.Comments()
				if len(comments) != 1 {
					t.Fatalf("expected 1 comment, got %d", len(comments))
				}
				if comments[0].Path != tt.wantComment {
					t.Errorf("comment path = %q, want %q", comments[0].Path, tt.wantComment)
				}
			}
			if tt.wantNonNilArgs {
				if !dyn.captured {
					t.Fatal("dynamic tool Execute was not called")
				}
				if dyn.gotArgs == nil {
					t.Error("dynamic tool Execute received nil args map, want non-nil empty map")
				}
			}
			failures := r.ToolFailures()
			if tt.wantFailure {
				if len(failures) != 1 {
					t.Errorf("ToolFailures() = %+v, want one failure", failures)
				} else if failures[0].Arguments != tt.arguments {
					t.Errorf("failure arguments = %q, want %q", failures[0].Arguments, tt.arguments)
				}
			}
			if !tt.wantFailure && len(failures) != 0 {
				t.Errorf("ToolFailures() = %+v, want no failures", failures)
			}
		})
	}
}

func TestExecuteToolCall_CodeCommentUsesPerItemPath(t *testing.T) {
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	r := NewRunner(Deps{
		Tools:            reg,
		CommentCollector: collector,
	})

	args := map[string]any{
		"comments": []any{
			map[string]any{
				"content":       "issue",
				"existing_code": "foo",
				"path":          "item-level.go",
			},
		},
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}

	cp := r.executeToolCall(context.Background(), "group-key", llm.ToolCall{
		Function: llm.FunctionCall{
			Name:      "code_comment",
			Arguments: string(argsJSON),
		},
	}, nil, "")
	if cp.Data != tool.CommentSucceed {
		t.Fatalf("unexpected result: %+v", cp)
	}

	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Path != "item-level.go" {
		t.Errorf("comment path: got %q, want %q", comments[0].Path, "item-level.go")
	}
}

func graceRoundCommentResponse() *llm.ChatResponse {
	content := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &content,
				ToolCalls: []llm.ToolCall{{
					ID:   "call_grace",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "code_comment",
						Arguments: `{"comments":[{"content":"found a bug","existing_code":"x := 1"}]}`,
					},
				}},
			},
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 50, CompletionTokens: 20},
	}
}

func TestRunMainTask_GraceRoundSubmitsComment(t *testing.T) {
	// Round 1: file_read (exhausts budget with MaxToolRequestTimes=1)
	// Grace round: model calls code_comment
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		graceRoundCommentResponse(),
	}}
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&fakeFileReadProvider{result: "package main\n"})
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	deps := Deps{
		LLMClient:        client,
		Model:            "fake",
		Template:         template.Template{MaxTokens: 100000, MaxToolRequestTimes: 1},
		Tools:            reg,
		CommentCollector: collector,
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
			{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
			{Type: "function", Function: llm.FunctionDef{Name: "file_read"}},
		},
		Session: session.New("/tmp/test-repo", "main", "fake", session.SessionOptions{}),
	}
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, stop, err := runner.RunMainTask(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	if completed {
		t.Fatal("expected not completed (budget exhausted)")
	}
	if stop != StopMaxRounds {
		t.Fatalf("stop = %v, want StopMaxRounds", stop)
	}
	// Grace round should have been called (call 2)
	if client.calls != 2 {
		t.Fatalf("LLM calls = %d, want 2 (1 main + 1 grace)", client.calls)
	}
	// The grace round should only have code_comment + task_done tools
	graceReq := client.requests[1]
	if len(graceReq.Tools) != 2 {
		t.Fatalf("grace round tools = %d, want 2", len(graceReq.Tools))
	}
	// Comment should have been collected
	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatalf("comments = %d, want 1", len(comments))
	}
	if comments[0].Path != "main.go" {
		t.Errorf("comment path = %q, want main.go", comments[0].Path)
	}
	// Token usage should include grace round
	if runner.TotalInputTokens() != 70 {
		t.Errorf("TotalInputTokens = %d, want 70 (20+50)", runner.TotalInputTokens())
	}
}

func TestRunMainTask_GraceRoundSkippedWhenContextCancelled(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
	}}
	deps := newTestDeps(client)
	deps.Template.MaxToolRequestTimes = 1
	deps.MainToolDefs = []llm.ToolDef{
		{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
		{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
	}
	runner := NewRunner(deps)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after the main loop exits but before grace round runs.
	// We simulate this by using a client that cancels ctx after the first call.
	origClient := client
	client.responses = []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
	}
	_ = origClient

	// Use a wrapper that cancels after first call
	cancelClient := &cancelAfterNClient{inner: client, cancelAt: 1, cancel: cancel}
	deps.LLMClient = cancelClient
	runner = NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	_, stop, _ := runner.RunMainTask(ctx, msgs, "main.go")
	if stop != StopMaxRounds {
		t.Fatalf("stop = %v, want StopMaxRounds", stop)
	}
	// Grace round should have been skipped (only 1 LLM call total)
	if cancelClient.calls != 1 {
		t.Fatalf("LLM calls = %d, want 1 (grace skipped due to ctx cancel)", cancelClient.calls)
	}
}

type cancelAfterNClient struct {
	inner    *fakeClient
	cancelAt int
	cancel   context.CancelFunc
	calls    int
}

func (c *cancelAfterNClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.calls++
	resp, err := c.inner.CompletionsWithCtx(ctx, req)
	if c.calls >= c.cancelAt {
		c.cancel()
	}
	return resp, err
}

func TestRunMainTask_GraceRoundNotTriggeredOnEmptyRoundsStop(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		fileReadToolCallResponse("call_2", `{"path":"main.go"}`),
		fileReadToolCallResponse("call_3", `{"path":"main.go"}`),
	}}
	reg := tool.NewRegistry()
	reg.Register(&fakeFileReadProvider{result: ""})
	deps := Deps{
		LLMClient:        client,
		Model:            "fake",
		Template:         template.Template{MaxTokens: 100000, MaxToolRequestTimes: 10},
		Tools:            reg,
		CommentCollector: tool.NewCommentCollector(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
			{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
		},
		Session: session.New("/tmp/test-repo", "main", "fake", session.SessionOptions{}),
	}
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	_, stop, err := runner.RunMainTask(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	if stop != StopEmptyRounds {
		t.Fatalf("stop = %v, want StopEmptyRounds", stop)
	}
	// Should NOT trigger grace round — only 3 LLM calls (empty rounds)
	if client.calls != 3 {
		t.Fatalf("LLM calls = %d, want 3 (no grace round on empty-rounds stop)", client.calls)
	}
}

// MainLoopStop must be self-describing in both registers: String() for logs and
// telemetry, Reason() for the manifest reason and scan warning that are the only
// stop diagnostics surviving a --format json run. Neither may collide across
// stops, or a reader cannot tell which exit fired from the artifact alone.
func TestMainLoopStopStringAndReason(t *testing.T) {
	names := make(map[string]MainLoopStop)
	reasons := make(map[string]MainLoopStop)
	for _, tc := range []struct {
		stop     MainLoopStop
		wantName string
	}{
		{StopNone, "none"},
		{StopMaxRounds, "max_rounds"},
		{StopEmptyRounds, "empty_rounds"},
		{StopCompression, "compression"},
	} {
		t.Run(tc.wantName, func(t *testing.T) {
			name := tc.stop.String()
			if name != tc.wantName {
				t.Errorf("String() = %q, want %q", name, tc.wantName)
			}
			reason := tc.stop.Reason()
			if reason == "" {
				t.Fatal("Reason() is empty; every stop needs a diagnostic sentence")
			}
			if prev, dup := names[name]; dup {
				t.Errorf("String() %q is shared by %v and %v", name, prev, tc.stop)
			}
			if prev, dup := reasons[reason]; dup {
				t.Errorf("Reason() %q is shared by %v and %v; stops must stay distinguishable", reason, prev, tc.stop)
			}
			names[name] = tc.stop
			reasons[reason] = tc.stop
		})
	}
}

// A value outside the enum must name itself in both registers rather than borrow
// the StopNone catch-all. This also guards the enum's growth: adding a constant
// after StopCompression makes this test fail, which is the prompt to give the new
// stop its own String() and Reason() case instead of letting it fall through to
// a message that says nothing.
func TestMainLoopStopUnknownValue(t *testing.T) {
	unknown := StopTokenBudget + 1

	if got, want := unknown.String(), "MainLoopStop(5)"; got != want {
		t.Errorf("String() = %q, want %q; a new constant needs its own case in String() and Reason()", got, want)
	}
	if got, want := unknown.Reason(), "main task stopped for an unrecognized reason (stop=5)"; got != want {
		t.Errorf("Reason() = %q, want %q; a new constant needs its own case in String() and Reason()", got, want)
	}
	if unknown.Reason() == StopNone.Reason() {
		t.Error("an unrecognized stop reuses the StopNone catch-all; the collapsed message is back")
	}
}

// The model gets a plain success, so the run warning is the only record that its
// `comments` violated the array schema.
func TestExecuteToolCall_CodeCommentRepairedArgsWarns(t *testing.T) {
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	r := NewRunner(Deps{Tools: reg, CommentCollector: collector})

	// `comments` serialized into a string, with a prose quote left unescaped —
	// the observed failure shape.
	serialized := `[{"content":"the name suggests "a trusted proxy exists" here","existing_code":"foo","path":"Auth.java"}]`
	argsJSON, err := json.Marshal(map[string]any{"comments": serialized})
	if err != nil {
		t.Fatal(err)
	}

	cp := r.executeToolCall(context.Background(), "Auth.java", llm.ToolCall{
		Function: llm.FunctionCall{Name: "code_comment", Arguments: string(argsJSON)},
	}, nil, "")
	if cp.Data != tool.CommentSucceed {
		t.Fatalf("result = %+v, want the batch recovered", cp)
	}

	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatalf("collected %d comments, want 1", len(comments))
	}
	if !strings.Contains(comments[0].Content, `"a trusted proxy exists"`) {
		t.Errorf("quoted term lost: %q", comments[0].Content)
	}

	warnings := r.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v, want exactly one", warnings)
	}
	if warnings[0].Type != "comment_args_repaired" || warnings[0].File != "Auth.java" {
		t.Errorf("warning = %+v, want comment_args_repaired on Auth.java", warnings[0])
	}
	if !strings.Contains(warnings[0].Message, "serialized string") {
		t.Errorf("warning message = %q", warnings[0].Message)
	}
}

// The far side of the accept/decline decision: a refused batch must reach the
// model as a failure, since that error is what makes it resend. No warning
// either, because nothing was papered over.
func TestExecuteToolCall_CodeCommentSuspectRepairReportsTheError(t *testing.T) {
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	r := NewRunner(Deps{Tools: reg, CommentCollector: collector})

	// The suggestion's closing quote is missing, so the repair reads the comma
	// after it as a terminator.
	serialized := `[{"content":"use a literal","suggestion_code":"x = "y","existing_code":"z","path":"a.go"}]`
	argsJSON, err := json.Marshal(map[string]any{"comments": serialized})
	if err != nil {
		t.Fatal(err)
	}

	cp := r.executeToolCall(context.Background(), "a.go", llm.ToolCall{
		Function: llm.FunctionCall{Name: "code_comment", Arguments: string(argsJSON)},
	}, nil, "")
	if !strings.Contains(cp.Data, "invalid character") {
		t.Fatalf("result = %+v, want the parser wording that makes the model retry", cp)
	}

	if got := collector.Comments(); len(got) != 0 {
		t.Errorf("collected %+v, want nothing from a refused batch", got)
	}
	if w := r.Warnings(); len(w) != 0 {
		t.Errorf("warnings = %+v, want none — a refused repair papers over nothing", w)
	}
}

// The contrast that keeps the tests above honest: a conformant batch must warn
// nothing, so comment_args_repaired counts real violations only.
func TestExecuteToolCall_CodeCommentWellFormedArgsDoesNotWarn(t *testing.T) {
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	r := NewRunner(Deps{Tools: reg, CommentCollector: collector})

	argsJSON, err := json.Marshal(map[string]any{"comments": []any{
		map[string]any{"content": "issue", "existing_code": "foo", "path": "a.go"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	cp := r.executeToolCall(context.Background(), "a.go", llm.ToolCall{
		Function: llm.FunctionCall{Name: "code_comment", Arguments: string(argsJSON)},
	}, nil, "")
	if cp.Data != tool.CommentSucceed {
		t.Fatalf("result = %+v", cp)
	}
	if w := r.Warnings(); len(w) != 0 {
		t.Errorf("warnings = %+v, want none", w)
	}
}

// TestRunMainTask_TokenBudgetStopsBeforeNextRound pins the in-conversation
// budget check: once the Runner's aggregate usage is over Deps.MaxTokensBudget
// the next round is not sent, the stop is classified StopTokenBudget, and the
// model still gets its one grace round to submit findings.
func TestRunMainTask_TokenBudgetStopsBeforeNextRound(t *testing.T) {
	// Every round reads a file and reports 600 tokens; budget 1000 admits two
	// rounds (0 and 600 are both within budget) and refuses the third (1200).
	client := &fakeClient{responses: []*llm.ChatResponse{
		withUsage(fileReadToolCallResponse("call_1", `{"path":"main.go"}`), 600),
		withUsage(fileReadToolCallResponse("call_2", `{"path":"main.go"}`), 600),
		withUsage(fileReadToolCallResponse("call_3", `{"path":"main.go"}`), 600),
		withUsage(fileReadToolCallResponse("call_4", `{"path":"main.go"}`), 600),
	}}
	deps := newTestDeps(client)
	deps.MaxTokensBudget = 1000
	deps.MainToolDefs = []llm.ToolDef{
		{Type: "function", Function: llm.FunctionDef{Name: "file_read", Description: "read"}},
		{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
	}
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, stop, err := runner.RunMainTask(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	if completed {
		t.Fatal("RunMainTask completed without task_done")
	}
	if stop != StopTokenBudget {
		t.Fatalf("expected StopTokenBudget, got %v", stop)
	}
	// Two review rounds plus exactly one grace round; the budget must not be
	// spent on a third review round and the grace round must not be skipped.
	if got := len(client.requests); got != 3 {
		t.Fatalf("expected 3 LLM requests (2 rounds + grace), got %d", got)
	}
	last := client.requests[2]
	for _, def := range last.Tools {
		if def.Function.Name == "file_read" {
			t.Fatalf("grace round must not offer file_read, got tools %+v", last.Tools)
		}
	}
	if runner.TotalTokensUsed() <= deps.MaxTokensBudget {
		t.Fatalf("usage %d should exceed the budget %d after the stop", runner.TotalTokensUsed(), deps.MaxTokensBudget)
	}
}

// TestRunMainTask_ZeroTokenBudgetNeverStops guards the default: callers that
// never set a budget keep the pre-existing round-only behaviour.
func TestRunMainTask_ZeroTokenBudgetNeverStops(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		withUsage(fileReadToolCallResponse("call_1", `{"path":"main.go"}`), 5000),
		withUsage(fileReadToolCallResponse("call_2", `{"path":"main.go"}`), 5000),
		taskDoneResponse(),
	}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, stop, err := runner.RunMainTask(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	if !completed || stop != StopNone {
		t.Fatalf("expected completion with StopNone, got completed=%v stop=%v", completed, stop)
	}
}

func withUsage(resp *llm.ChatResponse, prompt int64) *llm.ChatResponse {
	resp.Usage = &llm.UsageInfo{PromptTokens: prompt, CompletionTokens: 0, TotalTokens: prompt}
	return resp
}

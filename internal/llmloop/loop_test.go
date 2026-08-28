// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"encoding/json"
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

func TestRunPerFile_TaskDoneImmediately(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{taskDoneResponse()}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review this file")}
	completed, _, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done to complete RunPerFile")
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
}

func TestRunPerFile_UsesCompletionTokenLimit(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{taskDoneResponse()}}
	deps := newTestDeps(client)
	deps.Template.MaxTokens = 200000
	deps.Template.MaxCompletionTokens = 58888
	runner := NewRunner(deps)

	_, _, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"main.go",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if got := client.requests[0].MaxTokens; got != 58888 {
		t.Fatalf("request MaxTokens = %d, want 58888", got)
	}
}

func TestRunPerFile_TaskDoneExplicitDone(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		taskDoneResponseWithArguments(`{"state":"DONE"}`),
	}}
	runner := NewRunner(newTestDeps(client))

	completed, _, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review this file")},
		"main.go",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done DONE to complete RunPerFile")
	}
}

func TestRunPerFile_TaskDoneFailed(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		taskDoneResponseWithArguments(`{"state":"FAILED"}`),
	}}
	runner := NewRunner(newTestDeps(client))

	completed, _, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review this file")},
		"main.go",
	)
	if err == nil || !strings.Contains(err.Error(), "task_done reported FAILED") {
		t.Fatalf("expected task_done FAILED error, got %v", err)
	}
	if completed {
		t.Fatal("task_done FAILED must not complete RunPerFile")
	}
	if client.calls != 1 {
		t.Fatalf("expected terminal failure after 1 LLM call, got %d", client.calls)
	}
}

func TestRunPerFile_InvalidTaskDoneStateRetries(t *testing.T) {
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

			completed, _, err := runner.RunPerFile(
				context.Background(),
				[]llm.Message{llm.NewTextMessage("user", "review this file")},
				"main.go",
			)
			if err != nil {
				t.Fatalf("RunPerFile: %v", err)
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

func TestRunPerFile_TagsRequestsWithTaskSessionKey(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		taskDoneResponse(),
	}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review this file")}
	if _, _, err := runner.RunPerFile(context.Background(), msgs, "main.go"); err != nil {
		t.Fatalf("RunPerFile: %v", err)
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

func TestRunPerFile_ToolCallThenDone(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		taskDoneResponse(),
	}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, _, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done to complete RunPerFile")
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

func TestRunPerFile_ContextCancelled(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{taskDoneResponse()}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, _, err := runner.RunPerFile(ctx, msgs, "main.go")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
	if completed {
		t.Fatal("cancelled context should not complete RunPerFile")
	}
}

func TestRunPerFile_UnknownTool(t *testing.T) {
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
	completed, _, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done to complete RunPerFile")
	}
	if client.calls != 2 {
		t.Errorf("expected 2 calls, got %d", client.calls)
	}
}

// TestRunPerFile_NoToolRetryBoundedByCompression is a regression test: the
// no-tool retry path must share the compression bounds of tool rounds. A
// model that keeps answering without tool calls used to grow the
// conversation without any token check; the loop must instead stop with
// StopCompression once the conversation exceeds the warning threshold and
// compression cannot shrink it.
func TestRunPerFile_NoToolRetryBoundedByCompression(t *testing.T) {
	content := strings.Repeat("chatter ", 200)
	client := &fakeClient{responses: []*llm.ChatResponse{
		{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}}},
	}}
	deps := newTestDeps(client)
	// No MemoryCompressionTask: compression cannot shrink the conversation,
	// so crossing the warning threshold has to stop the loop.
	deps.Template.MaxTokens = 100
	deps.Template.MaxToolRequestTimes = 10
	deps.Template.MemoryCompressionTask.Messages = nil
	runner := NewRunner(deps)

	completed, stop, err := runner.RunPerFile(context.Background(), []llm.Message{llm.NewTextMessage("user", "review")}, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if completed {
		t.Fatal("expected RunPerFile not to complete")
	}
	if stop != StopCompression {
		t.Errorf("stop = %v, want StopCompression", stop)
	}
	if client.calls != 1 {
		t.Errorf("LLM calls = %d, want 1 (loop must stop instead of retrying unbounded)", client.calls)
	}
}

func TestRunPerFile_MaxToolRequestsWithoutTaskDoneDoesNotComplete(t *testing.T) {
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
	completed, stop, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if completed {
		t.Fatal("RunPerFile completed without task_done")
	}
	if stop != StopMaxRounds {
		t.Fatalf("expected StopMaxRounds, got %v", stop)
	}
}

func TestRunPerFile_EmptyToolResultsStopWithEmptyRounds(t *testing.T) {
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
	completed, stop, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if completed {
		t.Fatal("RunPerFile completed without task_done")
	}
	if stop != StopEmptyRounds {
		t.Fatalf("stop = %v, want StopEmptyRounds", stop)
	}
	if client.calls != 3 {
		t.Fatalf("LLM calls = %d, want 3 empty rounds", client.calls)
	}
}

func TestRunPerFile_UncompressibleContextStopsWithCompression(t *testing.T) {
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
	completed, stop, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if completed {
		t.Fatal("RunPerFile completed without task_done")
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
	}{
		{
			name:         "null args on code_comment (issue #382)",
			toolName:     "code_comment",
			arguments:    `null`,
			wantContains: "'comments' array is required",
		},
		{
			name:         "empty object on code_comment",
			toolName:     "code_comment",
			arguments:    `{}`,
			wantContains: "'comments' array is required",
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
		},
		{
			name:         "malformed json args",
			toolName:     "code_comment",
			arguments:    `{"comments":`,
			wantContains: "Error parsing tool arguments",
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

func TestRunPerFile_GraceRoundSubmitsComment(t *testing.T) {
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
	completed, stop, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
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

func TestRunPerFile_GraceRoundSkippedWhenContextCancelled(t *testing.T) {
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
	_, stop, _ := runner.RunPerFile(ctx, msgs, "main.go")
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

func TestRunPerFile_GraceRoundNotTriggeredOnEmptyRoundsStop(t *testing.T) {
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
	_, stop, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
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
	unknown := StopCompression + 1

	if got, want := unknown.String(), "MainLoopStop(4)"; got != want {
		t.Errorf("String() = %q, want %q; a new constant needs its own case in String() and Reason()", got, want)
	}
	if got, want := unknown.Reason(), "main task stopped for an unrecognized reason (stop=4)"; got != want {
		t.Errorf("Reason() = %q, want %q; a new constant needs its own case in String() and Reason()", got, want)
	}
	if unknown.Reason() == StopNone.Reason() {
		t.Error("an unrecognized stop reuses the StopNone catch-all; the collapsed message is back")
	}
}

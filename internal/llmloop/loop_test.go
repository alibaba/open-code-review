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
	calls     int
}

func (f *fakeClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
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
	completed, err := runner.RunPerFile(context.Background(), msgs, "main.go")
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

func TestRunPerFile_TaskDoneExplicitDone(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		taskDoneResponseWithArguments(`{"state":"DONE"}`),
	}}
	runner := NewRunner(newTestDeps(client))

	completed, err := runner.RunPerFile(
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

	completed, err := runner.RunPerFile(
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

			completed, err := runner.RunPerFile(
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

func TestRunPerFile_ToolCallThenDone(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		taskDoneResponse(),
	}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, err := runner.RunPerFile(context.Background(), msgs, "main.go")
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
	completed, err := runner.RunPerFile(ctx, msgs, "main.go")
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
	completed, err := runner.RunPerFile(context.Background(), msgs, "main.go")
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
	completed, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if completed {
		t.Fatal("RunPerFile completed without task_done")
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
			name:        "valid args keeps path override",
			toolName:    "code_comment",
			arguments:   `{"path":"hallucinated.go","comments":[{"content":"issue","existing_code":"foo"}]}`,
			wantComment: "file.go",
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
			}, nil)

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

func TestExecuteToolCall_CodeCommentOverridesHallucinatedPath(t *testing.T) {
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	r := NewRunner(Deps{
		Tools:            reg,
		CommentCollector: collector,
	})

	args := map[string]any{
		"path": "wrong.go",
		"comments": []any{
			map[string]any{
				"content":       "issue",
				"existing_code": "foo",
			},
		},
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}

	cp := r.executeToolCall(context.Background(), "correct.go", llm.ToolCall{
		Function: llm.FunctionCall{
			Name:      "code_comment",
			Arguments: string(argsJSON),
		},
	}, nil)
	if cp.Data != tool.CommentSucceed {
		t.Fatalf("unexpected result: %+v", cp)
	}

	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Path != "correct.go" {
		t.Errorf("path override: got %q, want %q", comments[0].Path, "correct.go")
	}
}

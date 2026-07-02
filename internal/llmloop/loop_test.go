package llmloop

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/open-code-review/open-code-review/internal/config/template"
	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/session"
	"github.com/open-code-review/open-code-review/internal/tool"
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

func taskDoneResponse() *llm.ChatResponse {
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
						Arguments: `{}`,
					},
				}},
			},
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5},
	}
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
	err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
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

func TestRunPerFile_ToolCallThenDone(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		taskDoneResponse(),
	}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
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
	err := runner.RunPerFile(ctx, msgs, "main.go")
	if err == nil {
		t.Error("expected error for cancelled context")
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
	err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if client.calls != 2 {
		t.Errorf("expected 2 calls, got %d", client.calls)
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

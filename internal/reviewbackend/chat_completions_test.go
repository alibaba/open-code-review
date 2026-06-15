package reviewbackend

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/llm"
)

type mockLLMClient struct {
	responses []*llm.ChatResponse
	err       error
	calls     int
}

func (m *mockLLMClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.calls >= len(m.responses) {
		return &llm.ChatResponse{}, nil
	}
	resp := m.responses[m.calls]
	m.calls++
	return resp, nil
}

func toolCallResponse(name, id string) *llm.ChatResponse {
	content := "working"
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Role:    "assistant",
				Content: &content,
				ToolCalls: []llm.ToolCall{{
					ID:   id,
					Type: "function",
					Function: llm.FunctionCall{
						Name:      name,
						Arguments: `{}`,
					},
				}},
			},
		}},
	}
}

func TestChatCompletionsBackend_ReviewFile_TaskDone(t *testing.T) {
	mock := &mockLLMClient{
		responses: []*llm.ChatResponse{
			toolCallResponse("task_done", "call-1"),
		},
	}
	backend := &ChatCompletionsBackend{
		client: mock,
		ep:     llm.ResolvedEndpoint{Model: "test-model"},
	}

	var executed bool
	err := backend.ReviewFile(context.Background(), ReviewFileRequest{
		Model:         "test-model",
		Messages:      []llm.Message{llm.NewTextMessage("user", "review")},
		MaxToolRounds: 3,
		FilePath:      "foo.go",
	}, func(_ context.Context, call ToolCallInput) ToolCallOutput {
		executed = true
		if call.Name != "task_done" {
			t.Errorf("tool name = %q", call.Name)
		}
		return ToolCallOutput{Completed: true}
	}, nil)
	if err != nil {
		t.Fatalf("ReviewFile: %v", err)
	}
	if !executed {
		t.Fatal("executor was not called")
	}
	if mock.calls != 1 {
		t.Errorf("LLM calls = %d, want 1", mock.calls)
	}
}

func TestChatCompletionsBackend_ReviewFile_LLMError(t *testing.T) {
	mock := &mockLLMClient{err: errors.New("network down")}
	backend := &ChatCompletionsBackend{
		client: mock,
		ep:     llm.ResolvedEndpoint{Model: "test-model"},
	}

	err := backend.ReviewFile(context.Background(), ReviewFileRequest{
		Messages:      []llm.Message{llm.NewTextMessage("user", "review")},
		MaxToolRounds: 1,
		FilePath:      "foo.go",
	}, func(context.Context, ToolCallInput) ToolCallOutput {
		t.Fatal("executor should not run on LLM error")
		return ToolCallOutput{}
	}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "network down") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTextClient_CompleteAdapter(t *testing.T) {
	backend := &fakeTextBackend{content: "hello", model: "m1"}
	client := TextClient(backend)
	resp, err := client.CompletionsWithCtx(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{llm.NewTextMessage("user", "hi")},
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	if resp.Content() != "hello" {
		t.Errorf("content = %q", resp.Content())
	}
}

type fakeTextBackend struct {
	content string
	model   string
}

func (f *fakeTextBackend) Kind() Kind     { return KindCursorAgent }
func (f *fakeTextBackend) Model() string  { return f.model }
func (f *fakeTextBackend) Source() string { return "test" }
func (f *fakeTextBackend) Complete(_ context.Context, _ CompleteRequest) (*CompleteResponse, error) {
	return &CompleteResponse{Content: f.content, Model: f.model}, nil
}
func (f *fakeTextBackend) ReviewFile(context.Context, ReviewFileRequest, ToolExecutor, *ReviewHooks) error {
	return nil
}

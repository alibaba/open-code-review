// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
)

func TestTotalOnlyAndMultipleChoices(test *testing.T) {
	budget := newBudget(150)
	calls := 0
	client := budget.wrap(fakeClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		calls++
		return &llm.ChatResponse{Usage: &llm.UsageInfo{TotalTokens: 175}}, nil
	}))
	if _, err := client.CompletionsWithCtx(test.Context(), llm.ChatRequest{}); err != nil {
		test.Fatal(err)
	}
	usage := budget.snapshot()
	if _, err := client.CompletionsWithCtx(test.Context(), llm.ChatRequest{}); !errors.Is(err, llm.ErrTokenBudget) || calls != 1 || usage.TotalTokens != 175 || usage.InputTokens+usage.OutputTokens != 0 {
		test.Fatalf("calls=%d total=%d usage=%+v err=%v", calls, usage.TotalTokens, usage, err)
	}
	budget = newBudget(0)
	first, second, reason, args := "alpha", "beta", "reason", "arguments"
	client = budget.wrap(fakeClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &first}}, {Message: llm.ResponseMessage{Content: &second, ReasoningContent: reason, ToolCalls: []llm.ToolCall{{Function: llm.FunctionCall{Arguments: args}}}}}}}, nil
	}))
	if _, err := client.CompletionsWithCtx(test.Context(), llm.ChatRequest{}); err != nil {
		test.Fatal(err)
	}
	usage = budget.snapshot()
	if usage.OutputTokens != int64(llm.CountTokens(first)+llm.CountTokens(second)+llm.CountTokens(reason)+llm.CountTokens(args)) {
		test.Fatalf("incomplete candidate usage: %+v", usage)
	}
}

func TestRequestBudgetAndUsage(test *testing.T) {
	budget := newBudget(15)
	var calls int
	client := fakeClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		calls++
		return &llm.ChatResponse{Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 99, CacheReadTokens: 4, CacheWriteTokens: 2}}, nil
	})
	if _, err := budget.wrap(client).CompletionsWithCtx(test.Context(), llm.ChatRequest{}); err != nil {
		test.Fatal(err)
	}
	if _, err := budget.wrap(client).CompletionsWithCtx(test.Context(), llm.ChatRequest{}); !errors.Is(err, llm.ErrTokenBudget) {
		test.Fatalf("budget error: %v", err)
	}
	usage := budget.snapshot()
	if calls != 1 || usage != (Usage{TotalTokens: 99, InputTokens: 10, OutputTokens: 5, CacheReadTokens: 4, CacheWriteTokens: 2}) {
		test.Fatalf("usage: %+v calls: %d", usage, calls)
	}
}

func TestBudgetEstimationAndFailures(test *testing.T) {
	budget := newBudget(0)
	content := "response"
	response := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content, ToolCalls: []llm.ToolCall{{Function: llm.FunctionCall{Arguments: `{"result":"completed"}`}}}}}}}
	client := budget.wrap(fakeClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) { return response, nil }))
	request := llm.ChatRequest{Messages: []llm.Message{llm.NewTextMessage("user", "explain the behavior")}}
	if _, err := client.CompletionsWithCtx(test.Context(), request); err != nil {
		test.Fatal(err)
	}
	usage := budget.snapshot()
	if usage.InputTokens == 0 || usage.OutputTokens == 0 || response.Usage == nil || response.Usage.TotalTokens != usage.InputTokens+usage.OutputTokens {
		test.Fatalf("estimated usage: %+v", usage)
	}
	failure := errors.New("transport failed")
	broken := budget.wrap(fakeClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) { return nil, failure }))
	if _, err := broken.CompletionsWithCtx(test.Context(), request); !errors.Is(err, failure) {
		test.Fatal(err)
	}
	if after := budget.snapshot(); after != usage {
		test.Fatalf("failure added usage: %+v", after)
	}
	ctx, cancel := context.WithCancel(test.Context())
	cancel()
	if _, err := client.CompletionsWithCtx(ctx, request); !errors.Is(err, context.Canceled) {
		test.Fatal(err)
	}
}

func TestBudgetToolDefinitionEstimation(test *testing.T) {
	for _, fixture := range []struct {
		name  string
		tools []llm.ToolDef
	}{
		{name: "absent tools"},
		{name: "empty tools", tools: []llm.ToolDef{}},
		{name: "tool name", tools: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "read_repository_file"}}}},
		{name: "tool description", tools: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{
			Name:        "file_read",
			Description: "Read a repository file within an inclusive line range and return its contents with line numbers.",
		}}}},
		{name: "tool schema", tools: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{
			Name: "file_read",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path":  map[string]any{"type": "string", "description": "Repository-relative file path"},
					"start_line": map[string]any{"type": "integer", "minimum": 1},
				},
				"required":             []string{"file_path"},
				"additionalProperties": false,
			},
		}}}},
		{name: "multiple tools", tools: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "file_read", Description: "Read repository files", Parameters: map[string]any{"type": "object"}}},
			{Type: "function", Function: llm.FunctionDef{Name: "code_search", Description: "Search repository contents", Parameters: map[string]any{"type": "object"}}},
		}},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			request := llm.ChatRequest{Messages: []llm.Message{llm.NewTextMessage("user", "explain the behavior")}, Tools: fixture.tools}
			expectedInput := int64(llmloop.CountMessagesTokens(request.Messages))
			if len(fixture.tools) > 0 {
				definitions, err := json.Marshal(fixture.tools)
				if err != nil {
					test.Fatal(err)
				}
				expectedInput += int64(llm.CountTokens(string(definitions)))
			}
			budget := newBudget(2 * expectedInput)
			var calls int
			client := fakeClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
				calls++
				return &llm.ChatResponse{}, nil
			})
			for round := 0; round < 2; round++ {
				response, err := budget.wrap(client).CompletionsWithCtx(test.Context(), request)
				if err != nil {
					test.Fatal(err)
				}
				if response.Usage == nil || *response.Usage != (llm.UsageInfo{PromptTokens: expectedInput, TotalTokens: expectedInput}) {
					test.Fatalf("estimated usage: %+v, want input and total %d", response.Usage, expectedInput)
				}
			}
			if _, err := budget.wrap(client).CompletionsWithCtx(test.Context(), request); !errors.Is(err, llm.ErrTokenBudget) {
				test.Fatalf("budget error: %v", err)
			}
			usage := budget.snapshot()
			if calls != 2 || usage != (Usage{TotalTokens: 2 * expectedInput, InputTokens: 2 * expectedInput}) {
				test.Fatalf("usage: %+v calls: %d", usage, calls)
			}
		})
	}
}

func TestBudgetServerUsageWithTools(test *testing.T) {
	for _, fixture := range []struct {
		name    string
		usage   llm.UsageInfo
		charged int64
	}{
		{name: "reported usage", charged: 99, usage: llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 99, CacheReadTokens: 4, CacheWriteTokens: 2}},
		{name: "partial breakdown", charged: 175, usage: llm.UsageInfo{PromptTokens: 100, TotalTokens: 175}},
		{name: "missing total", charged: 15, usage: llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5}},
		{name: "inconsistent small total", charged: 15, usage: llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 8}},
		{name: "zero usage"},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			request := llm.ChatRequest{
				Messages: []llm.Message{llm.NewTextMessage("user", "explain the behavior")},
				Tools: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{
					Name: "file_read", Description: "Read a repository file", Parameters: map[string]any{"type": "object"},
				}}},
			}
			reported := fixture.usage
			total := fixture.charged
			budget := newBudget(max(1, total))
			var calls int
			client := budget.wrap(fakeClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
				calls++
				content := "The behavior is unchanged."
				return &llm.ChatResponse{Usage: &reported, Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}}}, nil
			}))
			response, err := client.CompletionsWithCtx(test.Context(), request)
			if err != nil {
				test.Fatal(err)
			}
			if response.Usage != &reported || *response.Usage != fixture.usage {
				test.Fatalf("server usage changed: %+v, want %+v", response.Usage, fixture.usage)
			}
			if total > 0 {
				if _, err := client.CompletionsWithCtx(test.Context(), request); !errors.Is(err, llm.ErrTokenBudget) {
					test.Fatalf("budget error: %v", err)
				}
			}
			usage := budget.snapshot()
			expected := Usage{TotalTokens: total,
				InputTokens: fixture.usage.PromptTokens, OutputTokens: fixture.usage.CompletionTokens,
				CacheReadTokens: fixture.usage.CacheReadTokens, CacheWriteTokens: fixture.usage.CacheWriteTokens,
			}
			if calls != 1 || usage.TotalTokens != total || usage != expected {
				test.Fatalf("usage: %+v calls: %d", usage, calls)
			}
		})
	}
}

func TestBudgetConcurrencyAndWaitingCancellation(test *testing.T) {
	budget := newBudget(0)
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	client := budget.wrap(fakeClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		calls.Add(1)
		close(entered)
		<-release
		return nil, nil
	}))
	done := make(chan struct{})
	go func() { defer close(done); _, _ = client.CompletionsWithCtx(test.Context(), llm.ChatRequest{}) }()
	<-entered
	ctx, cancel := context.WithTimeout(test.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := client.CompletionsWithCtx(ctx, llm.ChatRequest{}); !errors.Is(err, context.DeadlineExceeded) {
		test.Fatalf("queued call: %v", err)
	}
	finalCtx, cancelFinal := context.WithTimeout(test.Context(), 20*time.Millisecond)
	defer cancelFinal()
	if _, _, err := client.(llm.FinalSubmissionClient).FinalSubmission(finalCtx, llm.ChatRequest{}); !errors.Is(err, context.DeadlineExceeded) {
		test.Fatalf("queued final submission: %v", err)
	}
	close(release)
	<-done
	if calls.Load() != 1 {
		test.Fatalf("concurrency gate admitted %d calls", calls.Load())
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/behaviortask"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
)

type testClient func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error)

func (client testClient) CompletionsWithCtx(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	return client(ctx, request)
}

func TestRunIsolationAndJoin(test *testing.T) {
	for _, failReview := range []bool{false, true} {
		started := make(chan struct{})
		failed := errors.New("role failed")
		result := Run(test.Context(), func(ctx context.Context) ([]model.LlmComment, error) {
			select {
			case <-started:
			case <-time.After(time.Second):
				test.Error("roles did not run concurrently")
			}
			if ctx.Err() != nil {
				test.Error("sibling failure canceled review")
			}
			if failReview {
				return nil, failed
			}
			return []model.LlmComment{{}}, nil
		}, func(ctx context.Context) (behaviortask.Result, error) {
			close(started)
			if failReview {
				return behaviortask.Result{Completed: true}, nil
			}
			return behaviortask.Result{}, failed
		})
		if failReview {
			if !errors.Is(result.ReviewError, failed) || !result.Analysis.Completed || result.AnalysisError != nil {
				test.Fatalf("outcome: %+v", result)
			}
		} else if !errors.Is(result.AnalysisError, failed) || len(result.Comments) != 1 || result.ReviewError != nil {
			test.Fatalf("outcome: %+v", result)
		}
	}
}

func TestRunCancellation(test *testing.T) {
	ctx, cancel := context.WithCancel(test.Context())
	var ready sync.WaitGroup
	ready.Add(2)
	go func() { ready.Wait(); cancel() }()
	result := Run(ctx, func(ctx context.Context) ([]model.LlmComment, error) {
		ready.Done()
		<-ctx.Done()
		return nil, ctx.Err()
	}, func(ctx context.Context) (behaviortask.Result, error) {
		ready.Done()
		<-ctx.Done()
		return behaviortask.Result{}, ctx.Err()
	})
	if !errors.Is(result.ReviewError, context.Canceled) || !errors.Is(result.AnalysisError, context.Canceled) {
		test.Fatalf("outcome: %+v", result)
	}
}

func TestSharedBudgetAndUsage(test *testing.T) {
	budget := NewBudget(15, 1)
	var calls int
	client := testClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		calls++
		return &llm.ChatResponse{Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 99, CacheReadTokens: 4, CacheWriteTokens: 2}}, nil
	})
	if _, err := budget.Wrap(client).CompletionsWithCtx(test.Context(), llm.ChatRequest{}); err != nil {
		test.Fatal(err)
	}
	if _, err := budget.Wrap(client).CompletionsWithCtx(test.Context(), llm.ChatRequest{}); !errors.Is(err, llm.ErrTokenBudget) {
		test.Fatalf("budget error: %v", err)
	}
	usage, exhausted := budget.Snapshot()
	if calls != 1 || !exhausted || usage != (behaviortask.Usage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 4, CacheWriteTokens: 2}) {
		test.Fatalf("usage: %+v exhausted: %t calls: %d", usage, exhausted, calls)
	}
}

func TestBudgetEstimationAndFailures(test *testing.T) {
	budget := NewBudget(0, 0)
	content := "response"
	response := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content, ToolCalls: []llm.ToolCall{{Function: llm.FunctionCall{Arguments: `{"result":"completed"}`}}}}}}}
	client := budget.Wrap(testClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) { return response, nil }))
	request := llm.ChatRequest{Messages: []llm.Message{llm.NewTextMessage("user", "explain the behavior")}}
	if _, err := client.CompletionsWithCtx(test.Context(), request); err != nil {
		test.Fatal(err)
	}
	usage, exhausted := budget.Snapshot()
	if exhausted || usage.InputTokens == 0 || usage.OutputTokens == 0 || response.Usage == nil || response.Usage.TotalTokens != usage.InputTokens+usage.OutputTokens {
		test.Fatalf("estimated usage: %+v", usage)
	}
	failure := errors.New("transport failed")
	broken := budget.Wrap(testClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) { return nil, failure }))
	if _, err := broken.CompletionsWithCtx(test.Context(), request); !errors.Is(err, failure) {
		test.Fatal(err)
	}
	if after, _ := budget.Snapshot(); after != usage {
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
			budget := NewBudget(2*expectedInput, 1)
			var calls int
			client := testClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
				calls++
				return &llm.ChatResponse{}, nil
			})
			for round := 0; round < 2; round++ {
				response, err := budget.Wrap(client).CompletionsWithCtx(test.Context(), request)
				if err != nil {
					test.Fatal(err)
				}
				if response.Usage == nil || *response.Usage != (llm.UsageInfo{PromptTokens: expectedInput, TotalTokens: expectedInput}) {
					test.Fatalf("estimated usage: %+v, want input and total %d", response.Usage, expectedInput)
				}
			}
			if _, err := budget.Wrap(client).CompletionsWithCtx(test.Context(), request); !errors.Is(err, llm.ErrTokenBudget) {
				test.Fatalf("budget error: %v", err)
			}
			usage, exhausted := budget.Snapshot()
			if calls != 2 || !exhausted || usage != (behaviortask.Usage{InputTokens: 2 * expectedInput}) {
				test.Fatalf("usage: %+v exhausted: %t calls: %d", usage, exhausted, calls)
			}
		})
	}
}

func TestBudgetServerUsageWithTools(test *testing.T) {
	for _, fixture := range []struct {
		name  string
		usage llm.UsageInfo
	}{
		{name: "reported usage", usage: llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 99, CacheReadTokens: 4, CacheWriteTokens: 2}},
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
			total := reported.PromptTokens + reported.CompletionTokens
			budget := NewBudget(max(1, 2*total), 1)
			var calls int
			client := budget.Wrap(testClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
				calls++
				content := "The behavior is unchanged."
				return &llm.ChatResponse{Usage: &reported, Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}}}, nil
			}))
			for round := 0; round < 2; round++ {
				response, err := client.CompletionsWithCtx(test.Context(), request)
				if err != nil {
					test.Fatal(err)
				}
				if response.Usage != &reported || *response.Usage != fixture.usage {
					test.Fatalf("server usage changed: %+v, want %+v", response.Usage, fixture.usage)
				}
			}
			if total > 0 {
				if _, err := client.CompletionsWithCtx(test.Context(), request); !errors.Is(err, llm.ErrTokenBudget) {
					test.Fatalf("budget error: %v", err)
				}
			}
			usage, exhausted := budget.Snapshot()
			expected := behaviortask.Usage{
				InputTokens: 2 * fixture.usage.PromptTokens, OutputTokens: 2 * fixture.usage.CompletionTokens,
				CacheReadTokens: 2 * fixture.usage.CacheReadTokens, CacheWriteTokens: 2 * fixture.usage.CacheWriteTokens,
			}
			if calls != 2 || exhausted != (total > 0) || usage != expected {
				test.Fatalf("usage: %+v exhausted: %t calls: %d", usage, exhausted, calls)
			}
		})
	}
}

func TestBudgetConcurrencyAndWaitingCancellation(test *testing.T) {
	budget := NewBudget(0, 1)
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	client := budget.Wrap(testClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
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
	close(release)
	<-done
	if calls.Load() != 1 {
		test.Fatalf("concurrency gate admitted %d calls", calls.Load())
	}
}

func TestReportStatus(test *testing.T) {
	for _, fixture := range []struct {
		name   string
		result behaviortask.Result
		err    error
		status string
	}{
		{"no submission", behaviortask.Result{}, errors.New("provider failed"), "failed"},
		{"partial", behaviortask.Result{Analysis: &behaviortask.Analysis{DeclaredCompletion: "partial"}, Completed: true, StopReason: "none"}, nil, "completed"},
		{"complete", behaviortask.Result{Analysis: &behaviortask.Analysis{DeclaredCompletion: "complete"}, Completed: true, StopReason: "none"}, nil, "completed"},
		{"omitted completeness", behaviortask.Result{Analysis: &behaviortask.Analysis{Markdown: "Report"}, Completed: true, StopReason: "none"}, nil, "completed"},
		{"budget", behaviortask.Result{Analysis: &behaviortask.Analysis{DeclaredCompletion: "complete"}, Completed: true, StopReason: "token_budget"}, nil, "partial"},
		{"delivery", behaviortask.Result{Analysis: &behaviortask.Analysis{DeclaredCompletion: "complete"}, Completed: true, StopReason: "none", Errors: []string{"disk full"}}, errors.New("disk full"), "partial"},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			report := NewReport(fixture.result, fixture.err, "artifacts")
			if report.Status != fixture.status || report.ArtifactDir != "artifacts" || report.CitedObservationIDs == nil {
				test.Fatalf("report: %+v", report)
			}
			if fixture.err != nil && len(report.Errors) != 1 {
				test.Fatalf("errors: %v", report.Errors)
			}
			if fixture.result.Analysis != nil {
				declared := fixture.result.Analysis.DeclaredCompletion
				if declared == "" {
					declared = "unknown"
				}
				if report.Markdown != fixture.result.Analysis.Markdown || report.DeclaredCompletion != declared {
					test.Fatalf("model content changed: %+v", report)
				}
			}
		})
	}
}

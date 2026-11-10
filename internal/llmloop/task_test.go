// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

type taskClient func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error)

func (client taskClient) CompletionsWithCtx(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	return client(ctx, request)
}

func testTaskResponse(name, arguments string) *llm.ChatResponse {
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{
			ToolCalls: []llm.ToolCall{{ID: name, Type: "function", Function: llm.FunctionCall{Name: name, Arguments: arguments}}},
		}}},
		Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
}

func testTask() Task {
	submit := llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: "submit_test_result"}}
	return Task{
		ToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "read_test_data"}},
			submit,
		},
		MaxRounds:            8,
		MaxContextTokens:     10000,
		MaxCompletionTokens:  17,
		EmptyResponseMessage: "Read the data or submit_test_result.",
		Compression: CompressionPolicy{
			Messages:        []MessageTemplate{{Role: "user", Content: "Summarize the test data: {{context}}"}},
			SummaryTemplate: "\n<test_summary>{{summary}}</test_summary>",
		},
		Grace: GracePolicy{
			Messages: []llm.Message{llm.NewTextMessage("user", "Submit your test result now.")},
			Tools:    []llm.ToolDef{submit},
		},
		ExecuteTool: func(_ context.Context, _ string, call llm.ToolCall, _ *session.TaskRecord, _ string) ToolOutcome {
			if call.Function.Name == "read_test_data" {
				return ToolOutcome{Content: "42"}
			}
			if call.Function.Name != "submit_test_result" {
				return ToolOutcome{Content: "Tool unavailable; choose a test tool."}
			}
			var result struct {
				Value int `json:"value"`
			}
			if err := json.Unmarshal([]byte(call.Function.Arguments), &result); err != nil || result.Value != 42 {
				return ToolOutcome{Content: "Invalid result; submit value 42."}
			}
			return ToolOutcome{Content: "Result accepted.", Action: Complete}
		},
	}
}

func taskMessages() []llm.Message {
	return []llm.Message{llm.NewTextMessage("system", "Analyze test data."), llm.NewTextMessage("user", "Find the value.")}
}

func TestTaskStructuredSubmissionAndRetry(t *testing.T) {
	for _, invalid := range []struct {
		name, arguments, message string
	}{
		{"submit_test_result", `{"value":0}`, "Invalid result; submit value 42."},
		{"submit_test_result", `{`, "Invalid result; submit value 42."},
		{"unavailable", `{}`, "Tool unavailable; choose a test tool."},
	} {
		t.Run(invalid.name+invalid.arguments, func(t *testing.T) {
			var requests []llm.ChatRequest
			client := taskClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
				requests = append(requests, request)
				switch len(requests) {
				case 1:
					return testTaskResponse("read_test_data", `{}`), nil
				case 2:
					return testTaskResponse(invalid.name, invalid.arguments), nil
				default:
					return testTaskResponse("submit_test_result", `{"value":42}`), nil
				}
			})
			runner := NewRunner(Deps{LLMClient: client, Session: session.New(t.TempDir(), "main", "fake", session.SessionOptions{})}, testTask())
			result, err := runner.Run(context.Background(), taskMessages(), "data")
			if err != nil || !result.Completed || result.StopReason != StopNone {
				t.Fatalf("Run = %+v, %v", result, err)
			}
			if len(requests) != 3 {
				t.Fatalf("requests = %d, want 3", len(requests))
			}
			for index, expected := range []string{"42", invalid.message} {
				messages := requests[index+1].Messages
				if got := messages[len(messages)-1].ExtractText(); got != expected {
					t.Fatalf("tool result = %q, want %q", got, expected)
				}
			}
			if runner.TotalTokensUsed() != 45 {
				t.Fatalf("usage = %d, want 45", runner.TotalTokensUsed())
			}
		})
	}
}

func TestTaskBudgetUsesOwnSubmission(t *testing.T) {
	for _, tokenBudget := range []int64{0, 1} {
		t.Run(strconv.FormatInt(tokenBudget, 10), func(t *testing.T) {
			task := testTask()
			task.MaxRounds = 1
			wantStop := StopMaxRounds
			if tokenBudget > 0 {
				task.MaxRounds = 5
				wantStop = StopTokenBudget
			}
			requests := 0
			client := taskClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
				requests++
				if requests == 1 {
					return testTaskResponse("read_test_data", `{}`), nil
				}
				if !reflect.DeepEqual(request.Tools, task.Grace.Tools) || request.Messages[len(request.Messages)-1].ExtractText() != "Submit your test result now." {
					t.Fatalf("unexpected grace request: %+v", request)
				}
				return testTaskResponse("submit_test_result", `{"value":42}`), nil
			})
			runner := NewRunner(Deps{LLMClient: client, MaxTokensBudget: tokenBudget, Session: session.New(t.TempDir(), "main", "fake", session.SessionOptions{})}, task)
			result, err := runner.Run(context.Background(), taskMessages(), "data")
			if err != nil || !result.Completed || result.StopReason != wantStop || requests != 2 {
				t.Fatalf("Run = %+v, %v; requests = %d", result, err, requests)
			}
			if len(runner.Warnings()) != 0 {
				t.Fatalf("generic task acquired business warnings: %+v", runner.Warnings())
			}
		})
	}
}

func TestTaskFailureAndCancellation(t *testing.T) {
	client := taskClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		return testTaskResponse("submit_test_result", `{}`), nil
	})
	task := testTask()
	task.ExecuteTool = func(context.Context, string, llm.ToolCall, *session.TaskRecord, string) ToolOutcome {
		return ToolOutcome{Content: "test task failed", Action: Fail}
	}
	runner := NewRunner(Deps{LLMClient: client, Session: session.New(t.TempDir(), "main", "fake", session.SessionOptions{})}, task)
	result, err := runner.Run(context.Background(), taskMessages(), "data")
	if err == nil || err.Error() != "task failed: test task failed" || result.Completed || result.StopReason != StopNone {
		t.Fatalf("Run = %+v, %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = runner.Run(ctx, taskMessages(), "data")
	if !errors.Is(err, context.Canceled) || result.Completed {
		t.Fatalf("cancelled Run = %+v, %v", result, err)
	}
}

func TestTaskOwnCompressionAndEmptyResponse(t *testing.T) {
	task := testTask()
	task.MaxContextTokens = 100
	original := task.ExecuteTool
	task.ExecuteTool = func(ctx context.Context, key string, call llm.ToolCall, record *session.TaskRecord, thinking string) ToolOutcome {
		if call.Function.Name == "read_test_data" {
			return ToolOutcome{Content: strings.Repeat("sample data ", 500)}
		}
		return original(ctx, key, call, record, thinking)
	}
	mainRequests, compressions := 0, 0
	client := taskClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
		if request.MaxTokens != 17 {
			t.Fatalf("completion limit = %d, want 17", request.MaxTokens)
		}
		if len(request.Tools) == 0 {
			compressions++
			if !strings.HasPrefix(request.Messages[0].ExtractText(), "Summarize the test data:") || !strings.Contains(request.Messages[0].ExtractText(), "sample data") {
				t.Fatalf("unexpected compression prompt: %+v", request.Messages)
			}
			summary := "value 42"
			return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}, nil
		}
		mainRequests++
		switch mainRequests {
		case 1:
			return &llm.ChatResponse{}, nil
		case 2:
			if request.Messages[len(request.Messages)-1].ExtractText() != task.EmptyResponseMessage {
				t.Fatal("missing task-specific empty response guidance")
			}
			return testTaskResponse("read_test_data", `{}`), nil
		default:
			if !strings.Contains(request.Messages[1].ExtractText(), "<test_summary>value 42</test_summary>") {
				t.Fatalf("missing task-specific summary: %+v", request.Messages)
			}
			return testTaskResponse("submit_test_result", `{"value":42}`), nil
		}
	})
	runner := NewRunner(Deps{LLMClient: client, Session: session.New(t.TempDir(), "main", "fake", session.SessionOptions{})}, task)
	result, err := runner.Run(context.Background(), taskMessages(), "data")
	runner.WaitBackground()
	if err != nil || !result.Completed || compressions != 1 || mainRequests != 3 {
		t.Fatalf("Run = %+v, %v; compression/main = %d/%d", result, err, compressions, mainRequests)
	}
}

func TestTaskModelErrors(t *testing.T) {
	for _, grace := range []bool{false, true} {
		t.Run(strconv.FormatBool(grace), func(t *testing.T) {
			task := testTask()
			task.MaxRounds = 1
			requests := 0
			client := taskClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
				requests++
				if grace && requests == 1 {
					return testTaskResponse("read_test_data", `{}`), nil
				}
				return nil, errors.New("model unavailable")
			})
			history := session.New(t.TempDir(), "main", "fake", session.SessionOptions{})
			runner := NewRunner(Deps{LLMClient: client, Session: history}, task)
			result, err := runner.Run(context.Background(), taskMessages(), "data")
			if result.Completed || (err == nil) != grace {
				t.Fatalf("Run = %+v, %v", result, err)
			}
			if grace && result.StopReason != StopMaxRounds {
				t.Fatalf("grace stop = %v", result.StopReason)
			}
			records := history.GetOrCreateFileSession("data").TaskRecords[session.MainTask]
			if records[len(records)-1].Error != "model unavailable" {
				t.Fatal("model failure was not recorded")
			}
		})
	}
}

func TestTaskGraceOutcomes(t *testing.T) {
	for _, scenario := range []string{"no_calls", "failed", "mixed", "no_tools", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			task := testTask()
			task.MaxRounds = 1
			if scenario == "no_tools" {
				task.Grace.Tools = nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			requests := 0
			client := taskClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
				requests++
				if requests == 1 {
					if scenario == "cancelled" {
						cancel()
					}
					return testTaskResponse("read_test_data", `{}`), nil
				}
				if scenario == "no_calls" {
					return &llm.ChatResponse{}, nil
				}
				response := testTaskResponse("submit_test_result", `{"value":42}`)
				if scenario == "mixed" {
					response.Choices[0].Message.ToolCalls = append(response.Choices[0].Message.ToolCalls, llm.ToolCall{Function: llm.FunctionCall{Name: "abort"}})
				}
				return response, nil
			})
			original := task.ExecuteTool
			task.ExecuteTool = func(ctx context.Context, key string, call llm.ToolCall, record *session.TaskRecord, thinking string) ToolOutcome {
				if call.Function.Name == "abort" || (scenario == "failed" && call.Function.Name == "submit_test_result") {
					return ToolOutcome{Action: Fail, Content: "submission rejected"}
				}
				return original(ctx, key, call, record, thinking)
			}
			runner := NewRunner(Deps{LLMClient: client, Session: session.New(t.TempDir(), "main", "fake", session.SessionOptions{})}, task)
			result, err := runner.Run(ctx, taskMessages(), "data")
			if err != nil || result.Completed || result.StopReason != StopMaxRounds {
				t.Fatalf("Run = %+v, %v", result, err)
			}
			if (scenario == "no_tools" || scenario == "cancelled") && requests != 1 {
				t.Fatalf("disabled grace round issued a request: %d", requests)
			}
		})
	}
}

func TestTaskEmptyToolResultsStopWithoutGrace(t *testing.T) {
	task := testTask()
	task.ExecuteTool = func(context.Context, string, llm.ToolCall, *session.TaskRecord, string) ToolOutcome {
		return ToolOutcome{}
	}
	requests := 0
	client := taskClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		requests++
		return testTaskResponse("read_test_data", `{}`), nil
	})
	runner := NewRunner(Deps{LLMClient: client, Session: session.New(t.TempDir(), "main", "fake", session.SessionOptions{})}, task)
	result, err := runner.Run(context.Background(), taskMessages(), "data")
	if err != nil || result.Completed || result.StopReason != StopEmptyRounds || requests != 3 {
		t.Fatalf("Run = %+v, %v; requests = %d", result, err, requests)
	}
}

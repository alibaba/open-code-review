// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestProviderErrorSpansUseSafeSummary(test *testing.T) {
	for _, grace := range []bool{false, true} {
		test.Run(fmt.Sprintf("grace=%t", grace), func(test *testing.T) {
			const detail = "provider response fixture"
			const safe = "provider unavailable"
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			ctx, span := provider.Tracer("fixture").Start(test.Context(), "request")
			history := session.New(test.TempDir(), "main", "fake", session.SessionOptions{})
			test.Cleanup(func() { _ = history.Finalize() })
			runner := NewRunner(Deps{LLMClient: taskClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) { return nil, errors.New(detail) }), Session: history, ProgressError: func(error) string { return safe }}, testTask())
			if grace {
				runner.runGraceRound(ctx, taskMessages(), "fixture", "session", true, StopMaxRounds)
			} else {
				_, _ = runner.Run(ctx, taskMessages(), "fixture")
			}
			span.End()
			ended := recorder.Ended()
			if len(ended) != 1 || ended[0].Status().Description != safe || len(ended[0].Events()) == 0 {
				test.Fatal("expected sanitized error span")
			}
			if strings.Contains(fmt.Sprint(ended[0].Attributes(), ended[0].Events()), detail) {
				test.Fatal("provider detail leaked into telemetry")
			}
		})
	}
}

func TestRequestBudgetErrorStopsWithoutGraceRequest(test *testing.T) {
	for _, wrapped := range []bool{false, true} {
		test.Run(fmt.Sprint(wrapped), func(test *testing.T) {
			calls := 0
			client := taskClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
				calls++
				if wrapped {
					return nil, fmt.Errorf("request budget: %w", llm.ErrTokenBudget)
				}
				return nil, llm.ErrTokenBudget
			})
			history := session.New(test.TempDir(), "main", "fake", session.SessionOptions{})
			test.Cleanup(func() { _ = history.Finalize() })
			runner := NewRunner(Deps{LLMClient: client, Session: history}, testTask())
			result, err := runner.Run(test.Context(), taskMessages(), "data")
			if err != nil || result.Completed || result.StopReason != StopTokenBudget || calls != 1 {
				test.Fatalf("result=%+v error=%v calls=%d", result, err, calls)
			}
		})
	}
}

func TestGraceRoundRequestBudgetOverridesRoundLimit(test *testing.T) {
	calls := 0
	client := taskClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		calls++
		if calls == 1 {
			return testTaskResponse("read_test_data", "{}"), nil
		}
		return nil, fmt.Errorf("request budget: %w", llm.ErrTokenBudget)
	})
	history := session.New(test.TempDir(), "main", "fake", session.SessionOptions{})
	test.Cleanup(func() { _ = history.Finalize() })
	task := testTask()
	task.MaxRounds = 1
	runner := NewRunner(Deps{LLMClient: client, Session: history}, task)
	result, err := runner.Run(test.Context(), taskMessages(), "data")
	if err != nil || result.Completed || result.StopReason != StopTokenBudget || calls != 2 {
		test.Fatalf("result=%+v error=%v calls=%d", result, err, calls)
	}
}

func TestCompressionBudgetRejectionPreservesFinalSubmission(t *testing.T) {
	for _, phase := range []string{"before append", "after append"} {
		t.Run(phase, func(t *testing.T) {
			task := testTask()
			task.MaxContextTokens = 1000
			messages := taskMessages()
			evidence := strings.Repeat("fact ", 820)
			if phase == "before append" {
				messages = append(messages, llm.NewTextMessage("assistant", evidence))
				evidence = "latest observation"
			}
			execute := task.ExecuteTool
			task.ExecuteTool = func(ctx context.Context, key string, call llm.ToolCall, record *session.TaskRecord, thinking string) ToolOutcome {
				if call.Function.Name == "read_test_data" {
					return ToolOutcome{Content: evidence}
				}
				return execute(ctx, key, call, record, thinking)
			}
			calls := 0
			client := taskClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
				calls++
				switch calls {
				case 1:
					return testTaskResponse("read_test_data", "{}"), nil
				case 2:
					if len(request.Tools) != 0 {
						t.Fatal("expected a synchronous compression request")
					}
					return nil, fmt.Errorf("admission: %w", llm.ErrTokenBudget)
				case 3:
					if !reflect.DeepEqual(request.Tools, task.Grace.Tools) || len(request.Messages) != len(messages)+3 || !reflect.DeepEqual(request.Messages[:len(messages)], messages) {
						t.Fatal("expected the original conversation and submission-only tools")
					}
					result := request.Messages[len(messages)+1]
					if result.ToolCallID != "read_test_data" || result.ExtractText() != evidence {
						t.Fatal("final submission lost the latest tool result")
					}
					return testTaskResponse("submit_test_result", `{"value":42}`), nil
				default:
					t.Fatal("unexpected call after final submission")
					return nil, nil
				}
			})
			history := session.New(t.TempDir(), "main", "fake", session.SessionOptions{})
			t.Cleanup(func() { _ = history.Finalize() })
			runner := NewRunner(Deps{LLMClient: client, Session: history}, task)
			result, err := runner.Run(t.Context(), messages, "data")
			if err != nil || !result.Completed || result.StopReason != StopTokenBudget || calls != 3 {
				t.Fatalf("result=%+v error=%v calls=%d", result, err, calls)
			}
		})
	}
}

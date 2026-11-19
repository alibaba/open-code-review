// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"errors"
	"fmt"
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
				runner.runGraceRound(ctx, taskMessages(), "fixture", "session", true)
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

func TestSharedBudgetErrorStopsWithoutGraceRequest(test *testing.T) {
	for _, wrapped := range []bool{false, true} {
		test.Run(fmt.Sprint(wrapped), func(test *testing.T) {
			calls := 0
			client := taskClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
				calls++
				if wrapped {
					return nil, fmt.Errorf("shared client: %w", llm.ErrTokenBudget)
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

func TestGraceRoundSharedBudgetOverridesRoundLimit(test *testing.T) {
	calls := 0
	client := taskClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		calls++
		if calls == 1 {
			return testTaskResponse("read_test_data", "{}"), nil
		}
		return nil, fmt.Errorf("shared client: %w", llm.ErrTokenBudget)
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

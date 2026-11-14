// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"fmt"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

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

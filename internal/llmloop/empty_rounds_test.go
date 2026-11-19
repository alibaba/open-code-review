// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

// emptyRoundGuard mirrors maxConsecutiveEmptyRounds in loop.go, which is a
// function-local constant and therefore not visible to tests.
const emptyRoundGuard = 3

// A model that answers with visible content (e.g. reasoning) but never emits a
// tool call must not loop until the per-task deadline. The empty-round guard
// (maxConsecutiveEmptyRounds) has to see this reply shape too, not only rounds
// that produced tool calls with no usable result. See issue #2010.
func TestZeroToolCallRoundsHitEmptyRoundGuard(t *testing.T) {
	task := testTask()
	var requests atomic.Int64
	client := taskClient(func(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
		requests.Add(1)
		// Reasoning text, no tool calls: the shape that used to skip the counter.
		content := "thinking about the task"
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}}}, nil
	})
	runner := NewRunner(Deps{
		LLMClient: client,
		Session:   session.New(t.TempDir(), "main", "fake", session.SessionOptions{}),
	}, task)
	result, err := runner.Run(context.Background(), []llm.Message{msg("user", "go")}, "task")
	runner.WaitBackground()
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	if result.Completed {
		t.Fatalf("result.Completed = true; a model that never calls a tool cannot complete the task")
	}
	if result.StopReason != StopEmptyRounds {
		t.Errorf("StopReason = %v, want %v; the zero-tool-call path must feed the empty-round guard",
			result.StopReason, StopEmptyRounds)
	}
	// The guard trips after maxConsecutiveEmptyRounds rounds, so a small bound
	// proves the loop is not running until the deadline.
	if got := requests.Load(); got > int64(emptyRoundGuard)+1 {
		t.Errorf("%d model requests were made, want at most %d; the loop is unbounded",
			got, emptyRoundGuard+1)
	}
}

// The guard must not fire while the model is still producing usable tool
// results: a zero-tool-call round resets the streak only when a later round
// makes progress, so the counter has to be shared between both paths.
func TestZeroToolCallRoundResetsAfterProgress(t *testing.T) {
	task := testTask()
	var requests atomic.Int64
	client := taskClient(func(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
		n := requests.Add(1)
		if n == 2 {
			// One productive round in the middle of the empty streak.
			return testTaskResponse("read_test_data", `{}`), nil
		}
		content := "no tool call this round"
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}}}, nil
	})
	runner := NewRunner(Deps{
		LLMClient: client,
		Session:   session.New(t.TempDir(), "main", "fake", session.SessionOptions{}),
	}, task)
	result, err := runner.Run(context.Background(), []llm.Message{msg("user", "go")}, "task")
	runner.WaitBackground()
	if err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
	// 1 empty, 1 productive (resets), then the guard needs another full streak.
	if result.StopReason != StopEmptyRounds {
		t.Errorf("StopReason = %v, want %v", result.StopReason, StopEmptyRounds)
	}
	if got := requests.Load(); got < int64(emptyRoundGuard)+2 {
		t.Errorf("%d requests were made, want at least %d; the productive round should have reset the streak",
			got, emptyRoundGuard+2)
	}
}

// A zero-tool-call round must still put the model's reasoning back into the
// history so the next request can build on it.
func TestZeroToolCallRoundKeepsReasoningInHistory(t *testing.T) {
	task := testTask()
	var requests atomic.Int64
	var sawReasoning atomic.Bool
	const marker = "REASONING_MARKER"
	client := taskClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
		n := requests.Add(1)
		if n > 1 {
			for _, message := range request.Messages {
				if strings.Contains(message.ExtractText(), marker) {
					sawReasoning.Store(true)
				}
			}
			return testTaskResponse("submit_test_result", `{"value":42}`), nil
		}
		content := marker
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}}}, nil
	})
	runner := NewRunner(Deps{
		LLMClient: client,
		Session:   session.New(t.TempDir(), "main", "fake", session.SessionOptions{}),
	}, task)
	result, err := runner.Run(context.Background(), []llm.Message{msg("user", "go")}, "task")
	runner.WaitBackground()
	if err != nil || !result.Completed {
		t.Fatalf("Run did not complete: result=%+v err=%v", result, err)
	}
	if !sawReasoning.Load() {
		t.Error("the reasoning from the zero-tool-call round was dropped from the conversation history")
	}
}

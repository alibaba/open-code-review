// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestTaskEmptyResponseContextBudget(t *testing.T) {
	for _, scenario := range []struct {
		name           string
		compressionErr error
		wantStop       MainLoopStop
		wantCompleted  bool
	}{
		{name: "compressed retry", wantStop: StopNone, wantCompleted: true},
		{name: "failed compression", compressionErr: errors.New("compression unavailable"), wantStop: StopCompression},
		{name: "exhausted token budget", compressionErr: llm.ErrTokenBudget, wantStop: StopTokenBudget},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			task := testTask()
			task.MaxContextTokens = 4096
			task.MaxCompletionTokens = 4096
			task.Grace = GracePolicy{}
			text := strings.Repeat("Observation remains unverified. ", 700)
			response := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &text}}}}
			messages := taskMessages()
			retry := append(copyMessages(messages), llm.NewTextMessage("assistant", response.VisibleContent()), llm.NewTextMessage("user", task.EmptyResponseMessage))
			if count := CountMessagesTokens(retry); count < PromptTokenLimit(task.MaxContextTokens) || count >= task.MaxContextTokens {
				t.Fatalf("fixture must cross the compression threshold within the context window, got %d", count)
			}
			mainRequests, compressions := 0, 0
			var requestTokens []int
			client := taskClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
				if len(request.Tools) == 0 {
					compressions++
					if !strings.Contains(request.Messages[0].ExtractText(), "Observation remains unverified.") {
						t.Fatal("compression lost the assistant response")
					}
					if scenario.compressionErr != nil {
						return nil, scenario.compressionErr
					}
					summary := "Evidence remains unverified."
					return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}, nil
				}
				mainRequests++
				requestTokens = append(requestTokens, CountMessagesTokens(request.Messages))
				if count := CountMessagesTokens(request.Messages); count >= PromptTokenLimit(task.MaxContextTokens) {
					t.Errorf("main request %d sent %d tokens at or above the %d-token compression threshold", mainRequests, count, PromptTokenLimit(task.MaxContextTokens))
				}
				if mainRequests == 1 {
					return response, nil
				}
				if !strings.Contains(request.Messages[1].ExtractText(), "<test_summary>Evidence remains unverified.</test_summary>") {
					t.Error("retry did not retain the compression summary")
				}
				return testTaskResponse("submit_test_result", `{"value":42}`), nil
			})
			runner := NewRunner(Deps{LLMClient: client, Session: session.New(t.TempDir(), "main", "fake", session.SessionOptions{})}, task)
			result, err := runner.Run(t.Context(), messages, "data")
			runner.WaitBackground()
			t.Logf("main request tokens=%v; compressions=%d; stop=%s", requestTokens, compressions, result.StopReason)
			wantRequests := 1
			if scenario.wantCompleted {
				wantRequests++
			}
			if err != nil || result.Completed != scenario.wantCompleted || result.StopReason != scenario.wantStop || mainRequests != wantRequests || compressions != 1 {
				t.Fatalf("Run=%+v, err=%v, main/compression requests=%d/%d", result, err, mainRequests, compressions)
			}
		})
	}
}

func TestTaskEmptyResponsePreservesRetryMessages(t *testing.T) {
	for _, scenario := range []struct {
		name, content, reasoning string
		native                   llm.NativeTurn
	}{
		{name: "text", content: "Evidence is still inconclusive."},
		{name: "native reasoning", reasoning: "Inspect the evidence.", native: llm.NativeTurn{Family: "openai-chat-completions", Payload: llm.ReasoningPayload("Inspect the evidence.")}},
		{name: "reasoning without replay", reasoning: "Inspect the evidence."},
		{name: "empty"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			task := testTask()
			requests := 0
			client := taskClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
				requests++
				if len(request.Tools) == 0 {
					t.Fatal("short retry unexpectedly required compression")
				}
				if requests == 1 {
					return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &scenario.content, ReasoningContent: scenario.reasoning, Native: scenario.native}}}}, nil
				}
				messages := request.Messages
				last := messages[len(messages)-1]
				if last.Role != "user" || last.ExtractText() != task.EmptyResponseMessage {
					t.Fatal("retry guidance lost")
				}
				if scenario.name == "empty" {
					if len(messages) != 3 {
						t.Fatal("empty response created an assistant message")
					}
				} else {
					if len(messages) != 4 {
						t.Fatalf("retry message count=%d, want 4", len(messages))
					}
					assistant := messages[2]
					if assistant.Role != "assistant" || assistant.ExtractText() != scenario.content || assistant.ReasoningContent != scenario.reasoning || assistant.Native != scenario.native {
						t.Fatalf("assistant turn changed: %+v", assistant)
					}
				}
				return testTaskResponse("submit_test_result", `{"value":42}`), nil
			})
			runner := NewRunner(Deps{LLMClient: client, Session: session.New(t.TempDir(), "main", "fake", session.SessionOptions{})}, task)
			result, err := runner.Run(t.Context(), taskMessages(), "data")
			runner.WaitBackground()
			if err != nil || !result.Completed || result.StopReason != StopNone || requests != 2 {
				t.Fatalf("Run=%+v, err=%v, requests=%d", result, err, requests)
			}
		})
	}
}

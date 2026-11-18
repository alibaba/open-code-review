// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestCompressionBudgetPartition(test *testing.T) {
	for _, fixture := range []struct {
		name                                     string
		frozenWords, roundWords, rounds, reserve int
	}{
		{"all_fit", 10, 10, 3, 0},
		{"large_frozen", 450, 100, 5, 0},
		{"summary_reserved", 450, 100, 5, 150},
		{"oversized_latest", 450, 400, 2, 0},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			messages := []llm.Message{msg("system", "system"), msg("user", strings.Repeat("frozen ", fixture.frozenWords))}
			for roundIndex := 0; roundIndex < fixture.rounds; roundIndex++ {
				messages = append(messages, msg("assistant", "read"), msg("tool", strings.Repeat("data ", fixture.roundWords)))
			}
			part := partitionMessages(messages, 1000, fixture.reserve)
			kept := CountMessagesTokens(messages[:2]) + CountMessagesTokens(messages[part.compressEnd:]) + fixture.reserve
			if kept >= PromptTokenLimit(1000) {
				test.Fatalf("kept %d tokens, limit %d: %+v", kept, PromptTokenLimit(1000), part)
			}
			if fixture.name == "all_fit" && (part.compressEnd != 2 || part.activeCount != fixture.rounds) {
				test.Fatalf("fitting rounds must stay live: %+v", part)
			}
			if (fixture.name == "large_frozen" || fixture.name == "summary_reserved") && part.activeCount == 0 {
				test.Fatal("a fitting latest round must stay live")
			}
		})
	}
}

func TestCompressionBudgetPreservesUnchangedHistory(test *testing.T) {
	for _, name := range []string{"missing_template", "everything_fits", "single_live_round"} {
		test.Run(name, func(test *testing.T) {
			task := testTask()
			if name == "missing_template" {
				task.Compression.Messages = nil
			}
			client := taskClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
				test.Error("unchanged history must not request a summary")
				return &llm.ChatResponse{}, nil
			})
			messages := []llm.Message{msg("system", "sys"), msg("user", "prompt"), msg("assistant", "read"), msg("tool", "evidence")}
			if name == "single_live_round" {
				task.MaxContextTokens = 1000
				task.MaxCompletionTokens = 200
				messages[3] = msg("tool", strings.Repeat("data ", 700))
			}
			runner := NewRunner(Deps{LLMClient: client, Session: session.New(test.TempDir(), "main", "fake", session.SessionOptions{})}, task)
			got, err := runner.runCompression(test.Context(), messages, "task")
			if err != nil || !reflect.DeepEqual(got, messages) {
				test.Fatalf("history changed: %v, %v", got, err)
			}
		})
	}
}

func TestCompressionBudgetIncludesNewSummary(test *testing.T) {
	task := testTask()
	task.MaxContextTokens = 1000
	task.MaxCompletionTokens = 150
	messages := []llm.Message{msg("system", "system"), msg("user", strings.Repeat("frozen ", 100))}
	for roundIndex := 0; roundIndex < 8; roundIndex++ {
		messages = append(messages, msg("assistant", "round_"+strconv.Itoa(roundIndex)), msg("tool", strings.Repeat("data ", 100)))
	}
	original := copyMessages(messages)
	requests := 0
	client := taskClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
		requests++
		if request.MaxTokens != 150 || strings.Contains(request.Messages[0].ExtractText(), "round_7") {
			test.Errorf("wrong summary limit or latest round summarized: %+v", request)
		}
		summary := strings.Repeat("summary ", 140)
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}, nil
	})
	runner := NewRunner(Deps{LLMClient: client, Session: session.New(test.TempDir(), "main", "fake", session.SessionOptions{})}, task)
	got, err := runner.runCompression(test.Context(), messages, "task")
	if err != nil || requests != 1 {
		test.Fatalf("compression requests=%d error=%v", requests, err)
	}
	if CountMessagesTokens(got) >= PromptTokenLimit(task.MaxContextTokens) {
		test.Fatalf("new summary overflowed: %d >= %d", CountMessagesTokens(got), PromptTokenLimit(task.MaxContextTokens))
	}
	if len(got) <= 2 || !reflect.DeepEqual(got[len(got)-2:], messages[len(messages)-2:]) || !reflect.DeepEqual(messages, original) {
		test.Fatal("compression lost the latest round or mutated its input")
	}
}

func TestCompressionBudgetRejectsOversizedSummary(test *testing.T) {
	task := testTask()
	task.MaxContextTokens = 1000
	task.MaxCompletionTokens = 150
	summary := strings.Repeat("oversized ", 1000)
	client := taskClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}, nil
	})
	runner := NewRunner(Deps{LLMClient: client, Session: session.New(test.TempDir(), "main", "fake", session.SessionOptions{})}, task)
	messages := []llm.Message{msg("system", "sys"), msg("user", "prompt"), msg("assistant", "read"), msg("tool", strings.Repeat("data ", 900))}
	original := copyMessages(messages)
	got, err := runner.runCompression(test.Context(), messages, "task")
	if err == nil || !reflect.DeepEqual(got, original) || !reflect.DeepEqual(messages, original) {
		test.Fatalf("oversized summary must fail without discarding history: err=%v", err)
	}
}

func TestCompressionBudgetNoRoom(test *testing.T) {
	for _, name := range []string{"fixed_prompt", "missing_placeholder"} {
		test.Run(name, func(test *testing.T) {
			task := testTask()
			task.MaxContextTokens = 100
			messages := []llm.Message{msg("system", "sys"), msg("user", strings.Repeat("prompt ", 100)), msg("assistant", "read"), msg("tool", "data")}
			if name == "missing_placeholder" {
				task.Compression.SummaryTemplate = ""
			}
			client := taskClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
				test.Error("unusable compression must not issue a model request")
				return nil, nil
			})
			runner := NewRunner(Deps{LLMClient: client}, task)
			got, err := runner.runCompression(test.Context(), messages, "task")
			if err == nil || !reflect.DeepEqual(got, messages) {
				test.Fatalf("unusable compression changed history: %v", err)
			}
		})
	}
}

func TestCompressionBudgetStrictBoundary(test *testing.T) {
	messages := []llm.Message{msg("system", "sys"), msg("user", "prompt"), msg("assistant", "read"), msg("tool", strings.Repeat("data ", 100))}
	reserve := PromptTokenLimit(1000) - CountMessagesTokens(messages)
	part := partitionMessages(messages, 1000, reserve)
	if part.activeCount != 0 || part.compressEnd != len(messages) {
		test.Fatalf("equality is not below the continuation threshold: %+v", part)
	}
}

func TestCompressionBudgetMainLoopContinues(test *testing.T) {
	task := testTask()
	task.MaxContextTokens = 1000
	task.MaxCompletionTokens = 150
	latest := "LATEST_EVIDENCE " + strings.Repeat("data ", 120)
	execute := task.ExecuteTool
	task.ExecuteTool = func(ctx context.Context, key string, call llm.ToolCall, record *session.TaskRecord, thinking string) ToolOutcome {
		if call.Function.Name == "read_test_data" {
			return ToolOutcome{Content: latest}
		}
		return execute(ctx, key, call, record, thinking)
	}
	messages := []llm.Message{msg("system", "sys"), msg("user", strings.Repeat("prompt ", 100))}
	for roundIndex := 0; roundIndex < 6; roundIndex++ {
		messages = append(messages, msg("assistant", "read"), msg("tool", strings.Repeat("data ", 100)))
	}
	mainRequests := 0
	var compressionRequests atomic.Int64
	client := taskClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
		if len(request.Tools) == 0 {
			compressionRequests.Add(1)
			summary := strings.Repeat("summary ", 140)
			return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}, nil
		}
		mainRequests++
		if CountMessagesTokens(request.Messages) >= PromptTokenLimit(task.MaxContextTokens) {
			test.Error("main request exceeded the continuation threshold")
		}
		if mainRequests == 1 {
			return testTaskResponse("read_test_data", `{}`), nil
		}
		found := false
		for _, message := range request.Messages {
			found = found || message.ExtractText() == latest
		}
		if !found {
			test.Error("the latest tool result was lost before the model consumed it")
		}
		return testTaskResponse("submit_test_result", `{"value":42}`), nil
	})
	runner := NewRunner(Deps{LLMClient: client, Session: session.New(test.TempDir(), "main", "fake", session.SessionOptions{})}, task)
	result, err := runner.Run(test.Context(), messages, "task")
	runner.WaitBackground()
	if err != nil || !result.Completed || result.StopReason != StopNone || mainRequests != 2 || compressionRequests.Load() < 1 {
		test.Fatalf("loop did not continue after compression: result=%+v err=%v main=%d compression=%d", result, err, mainRequests, compressionRequests.Load())
	}
}

func TestCompressionBudgetOutputLimit(test *testing.T) {
	for _, fixture := range []struct {
		name                                      string
		completionLimit, frozenWords, occurrences int
	}{
		{"configured_cap", 50, 10, 1},
		{"context_margin", 400, 10, 1},
		{"unspecified_cap", 0, 10, 1},
		{"fixed_prompt_space", 400, 700, 1},
		{"repeated_summary", 400, 600, 2},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			task := testTask()
			task.MaxContextTokens = 1000
			task.MaxCompletionTokens = fixture.completionLimit
			task.Compression.SummaryTemplate = strings.Repeat("<summary>{{summary}}</summary>", fixture.occurrences)
			messages := []llm.Message{msg("system", "sys"), msg("user", strings.Repeat("prompt ", fixture.frozenWords)), msg("assistant", "read"), msg("tool", strings.Repeat("data ", 900))}
			calls := 0
			client := taskClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
				calls++
				if request.MaxTokens <= 0 || request.MaxTokens > 200 || (fixture.completionLimit > 0 && request.MaxTokens > fixture.completionLimit) {
					test.Errorf("invalid summary output limit: %d", request.MaxTokens)
				}
				frame := llm.CountTokens(strings.ReplaceAll(task.Compression.SummaryTemplate, "{{summary}}", ""))
				if CountMessagesTokens(messages[:2])+frame+request.MaxTokens*fixture.occurrences >= 800 {
					test.Error("summary cap does not fit next to the frozen zone")
				}
				summary := "short summary"
				return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}, nil
			})
			runner := NewRunner(Deps{LLMClient: client, Session: session.New(test.TempDir(), "main", "fake", session.SessionOptions{})}, task)
			got, err := runner.runCompression(test.Context(), messages, "task")
			if err != nil || calls != 1 || CountMessagesTokens(got) >= 800 {
				test.Fatalf("bounded summary failed: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestCompressionBudgetIgnoresNonShrinkingSummary(test *testing.T) {
	task := testTask()
	task.MaxContextTokens = 1000
	task.MaxCompletionTokens = 200
	summary := strings.Repeat("summary ", 400)
	client := &fakeLLMClient{response: &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}}
	runner := NewRunner(Deps{LLMClient: client, Session: session.New(test.TempDir(), "main", "fake", session.SessionOptions{})}, task)
	messages := []llm.Message{msg("system", "sys"), msg("user", "prompt")}
	for roundIndex := 0; roundIndex < 2; roundIndex++ {
		messages = append(messages, msg("assistant", "read"), msg("tool", strings.Repeat("data ", 350)))
	}
	got, err := runner.runCompression(test.Context(), messages, "task")
	if err != nil || !reflect.DeepEqual(got, messages) {
		test.Fatalf("a summary that expands the conversation must not replace it: %v", err)
	}
}

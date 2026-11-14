// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestBudgetEstimatesContentAndReasoningOnce(test *testing.T) {
	for _, fixture := range []struct {
		name      string
		content   string
		reasoning string
	}{
		{name: "both", content: "Short answer.", reasoning: "Consider all the possible execution paths before selecting the answer."},
		{name: "reasoning only", reasoning: "The only generated output is reasoning."},
		{name: "content only", content: "Answer without reasoning."},
		{name: "embedded reasoning", content: "<think>Investigate the paths.</think>Answer."},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			budget := NewBudget(1, 1)
			content := fixture.content
			arguments := `{"file_path":"main.go"}`
			response := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{
				Content: &content, ReasoningContent: fixture.reasoning,
				ToolCalls: []llm.ToolCall{{Function: llm.FunctionCall{Name: "file_read", Arguments: arguments}}},
			}}}}
			client := budget.Wrap(testClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) { return response, nil }))
			if _, err := client.CompletionsWithCtx(test.Context(), llm.ChatRequest{}); err != nil {
				test.Fatal(err)
			}
			expected := int64(llm.CountTokens(content) + llm.CountTokens(fixture.reasoning) + llm.CountTokens(arguments))
			usage, _ := budget.Snapshot()
			if usage.OutputTokens != expected || response.Usage.CompletionTokens != expected {
				test.Fatalf("usage=%+v response=%+v expected_output=%d", usage, response.Usage, expected)
			}
			if _, err := client.CompletionsWithCtx(test.Context(), llm.ChatRequest{}); !errors.Is(err, llm.ErrTokenBudget) {
				test.Fatalf("budget did not stop the next request: %v", err)
			}
		})
	}
}

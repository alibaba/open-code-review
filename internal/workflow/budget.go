// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package workflow

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/alibaba/open-code-review/internal/behaviortask"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
)

type Budget struct {
	mu        sync.Mutex
	maximum   int64
	usage     behaviortask.Usage
	exhausted bool
	slots     chan struct{}
}

func NewBudget(maximum int64, concurrency int) *Budget {
	if concurrency <= 0 {
		concurrency = 8
	}
	return &Budget{maximum: maximum, slots: make(chan struct{}, concurrency)}
}

func (budget *Budget) Wrap(client llm.LLMClient) llm.LLMClient {
	return &budgetClient{budget: budget, client: client}
}

func (budget *Budget) Snapshot() (behaviortask.Usage, bool) {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	return budget.usage, budget.exhausted
}

type budgetClient struct {
	budget *Budget
	client llm.LLMClient
}

func (client *budgetClient) CompletionsWithCtx(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	budget := client.budget
	select {
	case budget.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-budget.slots }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	budget.mu.Lock()
	if budget.maximum > 0 && budget.usage.InputTokens+budget.usage.OutputTokens >= budget.maximum {
		budget.exhausted = true
		budget.mu.Unlock()
		return nil, llm.ErrTokenBudget
	}
	budget.mu.Unlock()
	response, err := client.client.CompletionsWithCtx(ctx, request)
	if response != nil {
		usage := response.Usage
		if usage == nil {
			input := int64(llmloop.CountMessagesTokens(request.Messages))
			if len(request.Tools) > 0 {
				if definitions, marshalErr := json.Marshal(request.Tools); marshalErr == nil {
					input += int64(llm.CountTokens(string(definitions)))
				}
			}
			var output int64
			if len(response.Choices) > 0 {
				message := response.Choices[0].Message
				if message.Content != nil {
					output += int64(llm.CountTokens(*message.Content))
				}
				output += int64(llm.CountTokens(message.ReasoningContent))
			}
			for _, call := range response.ToolCalls() {
				output += int64(llm.CountTokens(call.Function.Arguments))
			}
			usage = &llm.UsageInfo{PromptTokens: input, CompletionTokens: output, TotalTokens: input + output}
			response.Usage = usage
		}
		budget.mu.Lock()
		budget.usage.InputTokens += usage.PromptTokens
		budget.usage.OutputTokens += usage.CompletionTokens
		budget.usage.CacheReadTokens += usage.CacheReadTokens
		budget.usage.CacheWriteTokens += usage.CacheWriteTokens
		budget.mu.Unlock()
	}
	return response, err
}

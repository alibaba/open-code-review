// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
)

type requestBudget struct {
	mu      sync.Mutex
	maximum int64
	usage   Usage
	slots   chan struct{}
}

func newBudget(maximum int64) *requestBudget {
	return &requestBudget{maximum: maximum, slots: make(chan struct{}, 1)}
}

func (budget *requestBudget) wrap(client llm.LLMClient) llm.LLMClient {
	return &budgetClient{budget: budget, client: client}
}

func (budget *requestBudget) snapshot() Usage {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	return budget.usage
}

type budgetClient struct {
	budget *requestBudget
	client llm.LLMClient
}

func (client *budgetClient) CompletionsWithCtx(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	response, _, err := client.complete(ctx, request, false)
	return response, err
}

func (client *budgetClient) FinalSubmission(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, bool, error) {
	return client.complete(ctx, request, true)
}

func (client *budgetClient) complete(ctx context.Context, request llm.ChatRequest, finalSubmission bool) (*llm.ChatResponse, bool, error) {
	budget := client.budget
	select {
	case budget.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
	defer func() { <-budget.slots }()
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	budget.mu.Lock()
	budgetReached := budget.maximum > 0 && budget.usage.TotalTokens >= budget.maximum
	budget.mu.Unlock()
	if budgetReached && !finalSubmission {
		return nil, true, llm.ErrTokenBudget
	}
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
			for _, choice := range response.Choices {
				message := choice.Message
				if message.Content != nil {
					output += int64(llm.CountTokens(*message.Content))
				}
				output += int64(llm.CountTokens(message.ReasoningContent))
				for _, call := range message.ToolCalls {
					output += int64(llm.CountTokens(call.Function.Arguments))
				}
			}
			usage = &llm.UsageInfo{PromptTokens: input, CompletionTokens: output, TotalTokens: input + output}
			response.Usage = usage
		}
		budget.mu.Lock()
		budget.usage.TotalTokens += max(usage.TotalTokens, usage.PromptTokens+usage.CompletionTokens)
		budget.usage.InputTokens += usage.PromptTokens
		budget.usage.OutputTokens += usage.CompletionTokens
		budget.usage.CacheReadTokens += usage.CacheReadTokens
		budget.usage.CacheWriteTokens += usage.CacheWriteTokens
		budget.mu.Unlock()
	}
	return response, budgetReached, err
}

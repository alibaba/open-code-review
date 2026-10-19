// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"encoding/json"
	"math"
	"testing"
)

func TestCompletionTokenBudget(t *testing.T) {
	for _, tc := range []struct {
		name      string
		client    LLMClient
		requested int
		want      int
	}{
		{"unknown client", nil, 16384, 16384},
		{"chat default", &OpenAIClient{}, 16384, 16384},
		{"chat legacy cap", &OpenAIClient{cfg: ClientConfig{ExtraBody: map[string]any{"max_tokens": float64(32768)}}}, 16384, 32768},
		{"chat modern cap", &OpenAIClient{cfg: ClientConfig{ExtraBody: map[string]any{"max_completion_tokens": json.Number("32768")}}}, 16384, 32768},
		{"ambiguous chat caps", &OpenAIClient{cfg: ClientConfig{ExtraBody: map[string]any{"max_tokens": 32768, "max_completion_tokens": 8192}}}, 16384, 32768},
		{"request larger", &OpenAIClient{cfg: ClientConfig{ExtraBody: map[string]any{"max_tokens": 8192}}}, 16384, 16384},
		{"responses cap", &OpenAIResponsesClient{cfg: ClientConfig{ExtraBody: map[string]any{"max_output_tokens": 32768}}}, 16384, 32768},
		{"responses ignores chat field", &OpenAIResponsesClient{cfg: ClientConfig{ExtraBody: map[string]any{"max_tokens": 32768}}}, 16384, 16384},
		{"anthropic cap", &AnthropicClient{cfg: ClientConfig{ExtraBody: map[string]any{"max_tokens": 32768}}}, 16384, 32768},
		{"anthropic default", &AnthropicClient{}, 0, defaultAnthropicMaxTokens},
		{"anthropic ignores chat field", &AnthropicClient{cfg: ClientConfig{ExtraBody: map[string]any{"max_completion_tokens": 32768}}}, 16384, 16384},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CompletionTokenBudget(tc.client, tc.requested); got != tc.want {
				t.Fatalf("budget = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCompletionTokenBudgetInvalidOverrideDoesNotInventLimit(t *testing.T) {
	for _, value := range []any{nil, "32768", 32768.5, -1, true, math.Inf(1), json.Number("999999999999999999999999999999")} {
		client := &OpenAIClient{cfg: ClientConfig{ExtraBody: map[string]any{"max_tokens": value}}}
		if got := CompletionTokenBudget(client, 16384); got != 16384 {
			t.Errorf("override %v: budget = %d, want the existing request estimate", value, got)
		}
	}
}

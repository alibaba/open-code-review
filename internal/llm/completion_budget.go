// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import "encoding/json"

// CompletionTokenBudget estimates output allowance from the request and the
// client's known extra_body cap fields. Keep the larger value when fields
// disagree: compatible servers can give different fields precedence. This
// does not change the request or certify server defaults, disabled limits,
// malformed overrides, or caps implemented by an unknown client wrapper.
func CompletionTokenBudget(client LLMClient, requested int) int {
	var body map[string]any
	var fields []string
	switch c := client.(type) {
	case *OpenAIClient:
		body, fields = c.cfg.ExtraBody, []string{"max_tokens", "max_completion_tokens"}
	case *OpenAIResponsesClient:
		body, fields = c.cfg.ExtraBody, []string{"max_output_tokens"}
	case *AnthropicClient:
		body, fields = c.cfg.ExtraBody, []string{"max_tokens"}
		if requested <= 0 {
			requested = defaultAnthropicMaxTokens
		}
	}
	for _, field := range fields {
		// Use JSON's integer representation so config-decoded floats and Go
		// integers agree, while strings, fractions and overflow stay invalid.
		raw, err := json.Marshal(body[field])
		if err != nil {
			continue
		}
		var value int
		if json.Unmarshal(raw, &value) == nil {
			requested = max(requested, value)
		}
	}
	return requested
}

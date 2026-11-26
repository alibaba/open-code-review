// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import "testing"

func TestResolveUsageOpenAICompatibleCachedTokens(t *testing.T) {
	usage := resolveUsage([]byte(`{
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 20,
			"total_tokens": 120,
			"prompt_tokens_details": {
				"cached_tokens": 75
			}
		}
	}`))

	if usage == nil {
		t.Fatal("resolveUsage returned nil")
	}
	if usage.CacheReadTokens != 75 {
		t.Errorf("CacheReadTokens = %d, want 75", usage.CacheReadTokens)
	}
	if usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", usage.PromptTokens)
	}
	if usage.CompletionTokens != 20 {
		t.Errorf("CompletionTokens = %d, want 20", usage.CompletionTokens)
	}
}

func TestResolveUsageWrappedCachedTokens(t *testing.T) {
	usage := resolveUsage([]byte(`{
		"data": {
			"usage": {
				"prompt_tokens": 100,
				"completion_tokens": 20,
				"prompt_tokens_details": {
					"cached_tokens": 75,
					"cache_creation_tokens": 10
				}
			}
		}
	}`))

	if usage == nil {
		t.Fatal("resolveUsage returned nil")
	}
	if usage.CacheReadTokens != 75 {
		t.Errorf("CacheReadTokens = %d, want 75", usage.CacheReadTokens)
	}
	if usage.CacheWriteTokens != 10 {
		t.Errorf("CacheWriteTokens = %d, want 10", usage.CacheWriteTokens)
	}
	if usage.TotalTokens != 120 {
		t.Errorf("TotalTokens = %d, want 120 (OpenAI cached tokens are included in prompt_tokens)", usage.TotalTokens)
	}
}

func TestResolveUsageWrappedAnthropicCompatibleCacheTokens(t *testing.T) {
	usage := resolveUsage([]byte(`{
		"data": {
			"usage": {
				"prompt_tokens": 100,
				"completion_tokens": 20,
				"cache_read_input_tokens": 40,
				"cache_creation_input_tokens": 15
			}
		}
	}`))

	if usage == nil {
		t.Fatal("resolveUsage returned nil")
	}
	if usage.CacheReadTokens != 40 {
		t.Errorf("CacheReadTokens = %d, want 40", usage.CacheReadTokens)
	}
	if usage.CacheWriteTokens != 15 {
		t.Errorf("CacheWriteTokens = %d, want 15", usage.CacheWriteTokens)
	}
	if usage.TotalTokens != 175 {
		t.Errorf("TotalTokens = %d, want 175", usage.TotalTokens)
	}
}

func TestResolveUsageCacheReadPathPriority(t *testing.T) {
	usage := resolveUsage([]byte(`{
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 20,
			"cache_read_input_tokens": 40,
			"prompt_tokens_details": {
				"cached_tokens": 75
			}
		}
	}`))

	if usage == nil {
		t.Fatal("resolveUsage returned nil")
	}
	if usage.CacheReadTokens != 40 {
		t.Errorf("CacheReadTokens = %d, want 40 (Anthropic path should win)", usage.CacheReadTokens)
	}
}

func TestResolveUsageCacheCreationTokensPriority(t *testing.T) {
	usage := resolveUsage([]byte(`{
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 20,
			"cache_creation_input_tokens": 30,
			"prompt_tokens_details": {
				"cache_creation_tokens": 15
			}
		}
	}`))

	if usage == nil {
		t.Fatal("resolveUsage returned nil")
	}
	if usage.CacheWriteTokens != 30 {
		t.Errorf("CacheWriteTokens = %d, want 30 (Anthropic top-level path should win over prompt_tokens_details)", usage.CacheWriteTokens)
	}
}

func TestResolveUsageResponsesAPIFieldNames(t *testing.T) {
	usage := resolveUsage([]byte(`{
		"usage": {
			"input_tokens": 100,
			"output_tokens": 50,
			"total_tokens": 150,
			"input_tokens_details": {
				"cached_tokens": 80
			}
		}
	}`))

	if usage == nil {
		t.Fatal("resolveUsage returned nil")
	}
	if usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100 (from input_tokens)", usage.PromptTokens)
	}
	if usage.CompletionTokens != 50 {
		t.Errorf("CompletionTokens = %d, want 50 (from output_tokens)", usage.CompletionTokens)
	}
	if usage.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", usage.TotalTokens)
	}
	if usage.CacheReadTokens != 80 {
		t.Errorf("CacheReadTokens = %d, want 80 (from input_tokens_details.cached_tokens)", usage.CacheReadTokens)
	}
}

func TestResolveUsageAnthropicPromptIncludesCache(t *testing.T) {
	for _, wrapper := range []struct{ name, prefix, suffix string }{
		{"usage", `{"usage":`, `}`},
		{"root", "", ""},
		{"data", `{"data":{"usage":`, `}}`},
	} {
		for _, fixture := range []struct {
			name, usage string
			want        UsageInfo
		}{
			{"input", `{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":40,"cache_creation_input_tokens":15}`,
				UsageInfo{PromptTokens: 155, CompletionTokens: 20, TotalTokens: 175, CacheReadTokens: 40, CacheWriteTokens: 15}},
			{"prompt", `{"prompt_tokens":100,"completion_tokens":20,"cache_read_input_tokens":40,"cache_creation_input_tokens":15}`,
				UsageInfo{PromptTokens: 155, CompletionTokens: 20, TotalTokens: 175, CacheReadTokens: 40, CacheWriteTokens: 15}},
			{"cache_only", `{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":40,"cache_creation_input_tokens":15}`,
				UsageInfo{PromptTokens: 55, TotalTokens: 55, CacheReadTokens: 40, CacheWriteTokens: 15}},
			{"explicit_total", `{"input_tokens":100,"output_tokens":20,"total_tokens":200,"cache_read_input_tokens":40,"cache_creation_input_tokens":15}`,
				UsageInfo{PromptTokens: 155, CompletionTokens: 20, TotalTokens: 200, CacheReadTokens: 40, CacheWriteTokens: 15}},
		} {
			t.Run(wrapper.name+"/"+fixture.name, func(t *testing.T) {
				got := resolveUsage([]byte(wrapper.prefix + fixture.usage + wrapper.suffix))
				if got == nil || *got != fixture.want {
					t.Fatalf("resolveUsage = %+v, want %+v", got, fixture.want)
				}
			})
		}
	}
}

func TestResolveUsageOpenAICacheAlreadyIncluded(t *testing.T) {
	for _, fixture := range []struct {
		name, raw string
		want      UsageInfo
	}{
		{"chat", `{"usage":{"prompt_tokens":155,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":40,"cache_creation_tokens":15}}}`,
			UsageInfo{PromptTokens: 155, CompletionTokens: 20, TotalTokens: 175, CacheReadTokens: 40, CacheWriteTokens: 15}},
		{"responses", `{"usage":{"input_tokens":155,"output_tokens":20,"input_tokens_details":{"cached_tokens":40}}}`,
			UsageInfo{PromptTokens: 155, CompletionTokens: 20, TotalTokens: 175, CacheReadTokens: 40}},
		{"mixed_details", `{"usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":40,"prompt_tokens_details":{"cached_tokens":75,"cache_creation_tokens":15}}}`,
			UsageInfo{PromptTokens: 140, CompletionTokens: 20, TotalTokens: 160, CacheReadTokens: 40, CacheWriteTokens: 15}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			got := resolveUsage([]byte(fixture.raw))
			if got == nil || *got != fixture.want {
				t.Fatalf("resolveUsage = %+v, want %+v", got, fixture.want)
			}
		})
	}
}

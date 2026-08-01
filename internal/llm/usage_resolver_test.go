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

package llm

import (
	"encoding/json"
	"strings"
)

// UsageInfo holds token usage extracted from an LLM API response.
type UsageInfo struct {
	TotalTokens      int64 `json:"total_tokens"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`
}

var promptTokensPaths = []string{
	"usage.prompt_tokens",      // OpenAI standard
	"prompt_tokens",            // flat at root
	"data.usage.prompt_tokens", // wrapped in data layer
	"usage.input_tokens",       // OpenAI Responses API
	"input_tokens",             // Responses flat at root
	"data.usage.input_tokens",  // wrapped Responses API
}

var completionTokensPaths = []string{
	"usage.completion_tokens",      // OpenAI standard
	"completion_tokens",            // flat at root
	"data.usage.completion_tokens", // wrapped in data layer
	"usage.output_tokens",          // OpenAI Responses API
	"output_tokens",                // Responses flat at root
	"data.usage.output_tokens",     // wrapped Responses API
}

var cacheReadTokensPaths = []string{
	"usage.cache_read_input_tokens",                  // Anthropic
	"cache_read_input_tokens",                        // flat at root
	"data.usage.cache_read_input_tokens",             // wrapped Anthropic-compatible proxy
	"usage.prompt_tokens_details.cached_tokens",      // OpenAI-compatible providers
	"data.usage.prompt_tokens_details.cached_tokens", // wrapped OpenAI-compatible providers
	"usage.input_tokens_details.cached_tokens",       // OpenAI Responses API
	"data.usage.input_tokens_details.cached_tokens",  // wrapped OpenAI Responses API
}

var cacheWriteTokensPaths = []string{
	"usage.cache_creation_input_tokens",                      // Anthropic / proxy
	"cache_creation_input_tokens",                            // flat at root
	"data.usage.cache_creation_input_tokens",                 // wrapped Anthropic-compatible proxy
	"usage.prompt_tokens_details.cache_creation_tokens",      // ApexRoute / LLM Gateway — proxy normalization of Anthropic cache_creation_input_tokens
	"data.usage.prompt_tokens_details.cache_creation_tokens", // wrapped proxy normalization
}

// anthropicCacheReadPathCount is the number of Anthropic-style cache read paths
// at the start of cacheReadTokensPaths. OpenAI-style paths follow; under OpenAI
// semantics cached tokens are already included in prompt_tokens.
const anthropicCacheReadPathCount = 3

// anthropicCacheWritePathCount is the number of Anthropic-style cache write paths
// at the start of cacheWriteTokensPaths.
const anthropicCacheWritePathCount = 3

// totalTokensPaths is an ordered list of JSON paths to try when extracting
// total token count from a response body. Paths are dot-separated keys that
// navigate through nested map[string]any objects. The first match wins.
var totalTokensPaths = []string{
	"usage.total_tokens",      // OpenAI standard
	"total_tokens",            // flat at root
	"data.usage.total_tokens", // wrapped in data layer
}

// resolveUsage parses raw JSON bytes into a map and extracts token usage
// by probing configured paths sequentially. Returns nil if no total_tokens found.
func resolveUsage(raw []byte) *UsageInfo {
	var rawBody map[string]any
	if err := json.Unmarshal(raw, &rawBody); err != nil {
		return nil
	}

	total, hasAny := probePath(rawBody, totalTokensPaths)
	prompt, _ := probePath(rawBody, promptTokensPaths)
	completion, _ := probePath(rawBody, completionTokensPaths)
	cacheRead, cacheReadIdx, _ := probePathIndex(rawBody, cacheReadTokensPaths)
	cacheWrite, cacheWriteIdx, _ := probePathIndex(rawBody, cacheWriteTokensPaths)

	if !hasAny && prompt == 0 && completion == 0 {
		return nil
	}

	ui := &UsageInfo{
		TotalTokens:      total,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		CacheReadTokens:  cacheRead,
		CacheWriteTokens: cacheWrite,
	}

	// If TotalTokens wasn't explicitly available but we have prompt+completion, compute it.
	// Anthropic reports cache tokens separately from input_tokens, so include them in the
	// fallback total. OpenAI prompt_tokens already includes cached_tokens, so only add cache
	// counts when they came from Anthropic-style top-level fields.
	if total == 0 && (prompt > 0 || completion > 0) {
		ui.TotalTokens = prompt + completion
		if cacheReadIdx >= 0 && cacheReadIdx < anthropicCacheReadPathCount {
			ui.TotalTokens += cacheRead
		}
		if cacheWriteIdx >= 0 && cacheWriteIdx < anthropicCacheWritePathCount {
			ui.TotalTokens += cacheWrite
		}
	}

	return ui
}

// probePath walks through each candidate path in order, returning the first
// int64 value found along with true. Returns (0, false) if none match.
func probePath(root map[string]any, paths []string) (int64, bool) {
	v, _, ok := probePathIndex(root, paths)
	return v, ok
}

// probePathIndex is like probePath but also returns the index of the matched path.
func probePathIndex(root map[string]any, paths []string) (int64, int, bool) {
	for i, p := range paths {
		parts := strings.Split(p, ".")

		var current any = root
		for _, part := range parts {
			obj, ok := current.(map[string]any)
			if !ok {
				goto next
			}
			current, ok = obj[part]
			if !ok {
				goto next
			}
		}

		switch v := current.(type) {
		case float64:
			return int64(v), i, true
		case int64:
			return v, i, true
		case int:
			return int64(v), i, true
		}
	next:
	}
	return 0, -1, false
}

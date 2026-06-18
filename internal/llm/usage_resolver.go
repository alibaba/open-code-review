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
	"usage.input_tokens",       // OpenAI Responses API
	"prompt_tokens",            // flat at root
	"input_tokens",             // flat Responses-style
	"data.usage.prompt_tokens", // wrapped in data layer
	"data.usage.input_tokens",  // wrapped Responses-style
}

var completionTokensPaths = []string{
	"usage.completion_tokens",      // OpenAI standard
	"usage.output_tokens",          // OpenAI Responses API
	"completion_tokens",            // flat at root
	"output_tokens",                // flat Responses-style
	"data.usage.completion_tokens", // wrapped in data layer
	"data.usage.output_tokens",     // wrapped Responses-style
}

var cacheReadTokensPaths = []string{
	"usage.cache_read_input_tokens",                // Anthropic
	"cache_read_input_tokens",                      // flat at root
	"usage.prompt_tokens_details.cache_tokens_hit", // some providers
	"usage.prompt_tokens_details.cache_tokens",     // some providers
}

var cacheWriteTokensPaths = []string{
	"usage.cache_creation_input_tokens", // Anthropic / proxy
	"cache_creation_input_tokens",       // flat at root
}

// totalTokensPaths is an ordered list of JSON paths to try when extracting
// total token count from a response body. Paths are dot-separated keys that
// navigate through nested map[string]any objects. The first match wins.
var totalTokensPaths = []string{
	"usage.total_tokens",      // OpenAI standard
	"total_tokens",            // flat at root
	"data.usage.total_tokens", // wrapped in data layer (some proxy APIs)
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
	cacheRead, _ := probePath(rawBody, cacheReadTokensPaths)
	cacheWrite, _ := probePath(rawBody, cacheWriteTokensPaths)

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
	if total == 0 && (prompt > 0 || completion > 0) {
		ui.TotalTokens = prompt + completion + cacheRead + cacheWrite
	}

	return ui
}

// probePath walks through each candidate path in order, returning the first
// int64 value found along with true. Returns (0, false) if none match.
func probePath(root map[string]any, paths []string) (int64, bool) {
	for _, p := range paths {
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
			return int64(v), true
		case int64:
			return v, true
		case int:
			return int64(v), true
		}
	next:
	}
	return 0, false
}

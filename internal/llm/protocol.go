package llm

import (
	"fmt"
	"strings"
)

// Canonical protocol identifiers understood by the LLM client factory and
// resolver. These are the only values produced by NormalizeProtocol for known
// protocols; downstream code (NewLLMClient switch, resolver branches) compares
// against these constants exclusively.
//
// Naming convention: <vendor>-<flavor>. New built-in protocols should add a
// constant here, extend ValidateProtocol's whitelist, and add a case to
// NewLLMClient.
const (
	// ProtocolAnthropic is the Anthropic Messages API spoken directly to
	// api.anthropic.com (or a compatible gateway).
	ProtocolAnthropic = "anthropic"
	// ProtocolOpenAIChatCompletions is the OpenAI Chat Completions API
	// (/v1/chat/completions). The value "openai" is kept for full backward
	// compatibility with existing config files.
	ProtocolOpenAIChatCompletions = "openai"
	// ProtocolOpenAIResponses is the OpenAI Responses API (/v1/responses),
	// used by GPT-5.x / o-series models.
	ProtocolOpenAIResponses = "openai-responses"
)

// NormalizeProtocol maps legacy protocol aliases to their canonical names.
// It is case-insensitive and returns canonical names for known aliases.
// Empty string is returned as-is (the caller decides the default). Unknown
// values are returned unchanged so that ValidateProtocol can surface a precise
// error message rather than silently swallowing a typo.
//
// The alias "openai-chat-completions" -> ProtocolOpenAIChatCompletions ("openai")
// exists for configs written during the short-lived naming experiment on this
// branch; all existing production configs already use "openai".
func NormalizeProtocol(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return ""
	case ProtocolAnthropic:
		return ProtocolAnthropic
	case ProtocolOpenAIChatCompletions, "openai-chat-completions":
		return ProtocolOpenAIChatCompletions
	case ProtocolOpenAIResponses:
		return ProtocolOpenAIResponses
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

// ValidateProtocol accepts the three canonical protocol names and rejects
// everything else. It intentionally does NOT accept the "openai-chat-completions"
// alias — callers must run the value through NormalizeProtocol first. This keeps
// alias mapping centralized in NormalizeProtocol and lets the error message
// enumerate the canonical names.
func ValidateProtocol(p string) error {
	switch p {
	case ProtocolAnthropic, ProtocolOpenAIChatCompletions, ProtocolOpenAIResponses:
		return nil
	case "anthropic-vertex":
		return fmt.Errorf("protocol %q is not yet implemented; supported protocols are %q, %q, %q", p, ProtocolAnthropic, ProtocolOpenAIChatCompletions, ProtocolOpenAIResponses)
	}
	return fmt.Errorf("unsupported protocol %q; supported protocols are %q, %q, %q", p, ProtocolAnthropic, ProtocolOpenAIChatCompletions, ProtocolOpenAIResponses)
}

// IsAnthropicProtocol reports whether p is the canonical Anthropic protocol
// name. It returns false for any other value (including the empty string and
// non-canonical aliases).
func IsAnthropicProtocol(p string) bool {
	return p == ProtocolAnthropic
}

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

// NormalizeProtocol canonicalizes protocol names. It is case-insensitive and
// trims whitespace. Empty string is returned as-is (the caller decides the
// default). Known protocol names are mapped to their canonical constants;
// unknown values are lowercased and trimmed so that ValidateProtocol can
// surface a precise error message rather than silently swallowing a typo.
func NormalizeProtocol(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "":
		return ""
	case ProtocolAnthropic:
		return ProtocolAnthropic
	case ProtocolOpenAIChatCompletions:
		return ProtocolOpenAIChatCompletions
	case ProtocolOpenAIResponses:
		return ProtocolOpenAIResponses
	default:
		return normalized
	}
}

// ValidateProtocol accepts the three canonical protocol names and rejects
// everything else.
func ValidateProtocol(p string) error {
	switch p {
	case ProtocolAnthropic, ProtocolOpenAIChatCompletions, ProtocolOpenAIResponses:
		return nil
	default:
		return fmt.Errorf("unsupported protocol %q; supported protocols are %q, %q, %q", p, ProtocolAnthropic, ProtocolOpenAIChatCompletions, ProtocolOpenAIResponses)
	}
}

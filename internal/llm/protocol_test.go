package llm

import (
	"strings"
	"testing"
)

func TestNormalizeProtocol(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty stays empty", "", ""},
		{"canonical anthropic is idempotent", ProtocolAnthropic, ProtocolAnthropic},
		{"canonical openai is idempotent", ProtocolOpenAIChatCompletions, ProtocolOpenAIChatCompletions},
		{"canonical openai-responses is idempotent", ProtocolOpenAIResponses, ProtocolOpenAIResponses},
		{"alias openai-chat-completions normalizes to openai", "openai-chat-completions", ProtocolOpenAIChatCompletions},
		{"alias OPENAI-CHAT-COMPLETIONS is case-insensitive", "OPENAI-CHAT-COMPLETIONS", ProtocolOpenAIChatCompletions},
		{"alias with whitespace is trimmed", "  openai-chat-completions  ", ProtocolOpenAIChatCompletions},
		{"anthropic case-insensitive", "ANTHROPIC", ProtocolAnthropic},
		{"openai-responses case-insensitive", "OpenAI-Responses", ProtocolOpenAIResponses},
		{"unknown passthrough lowercased", "gRPC", "grpc"},
		{"reserved-but-unimplemented anthropic-vertex preserved", "anthropic-vertex", "anthropic-vertex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeProtocol(tt.raw); got != tt.want {
				t.Errorf("NormalizeProtocol(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestValidateProtocol(t *testing.T) {
	tests := []struct {
		name    string
		p       string
		wantErr bool
		errSub  string
	}{
		{"anthropic ok", ProtocolAnthropic, false, ""},
		{"openai ok", ProtocolOpenAIChatCompletions, false, ""},
		{"openai-responses ok", ProtocolOpenAIResponses, false, ""},
		// ValidateProtocol does NOT accept the "openai-chat-completions" alias
		// — callers must run it through NormalizeProtocol first.
		{"alias openai-chat-completions rejected", "openai-chat-completions", true, "unsupported protocol"},
		{"empty rejected", "", true, "unsupported protocol"},
		{"grpc rejected", "grpc", true, "unsupported protocol"},
		{"anthropic-vertex rejected", "anthropic-vertex", true, "unsupported protocol"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProtocol(tt.p)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateProtocol(%q) returned nil, want error", tt.p)
				}
				if tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
					t.Errorf("ValidateProtocol(%q) error = %q, want substring %q", tt.p, err.Error(), tt.errSub)
				}
				return
			}
			if err != nil {
				t.Errorf("ValidateProtocol(%q) returned unexpected error: %v", tt.p, err)
			}
		})
	}
}

// TestValidateProtocol_AcceptsNormalizedAlias is the explicit contract check
// for the recommended call pattern ValidateProtocol(NormalizeProtocol(raw)).
func TestValidateProtocol_AcceptsNormalizedAlias(t *testing.T) {
	if err := ValidateProtocol(NormalizeProtocol("openai-chat-completions")); err != nil {
		t.Errorf("ValidateProtocol(NormalizeProtocol(\"openai-chat-completions\")) = %v, want nil", err)
	}
	if err := ValidateProtocol(NormalizeProtocol("OpenAI-Chat-Completions")); err != nil {
		t.Errorf("ValidateProtocol(NormalizeProtocol(\"OpenAI-Chat-Completions\")) = %v, want nil", err)
	}
}

// TestValidateProtocol_ErrorMessageListsAllProtocols makes sure the error
// message enumerates every canonical name so users discover openai-responses
// from any typo.
func TestValidateProtocol_ErrorMessageListsAllProtocols(t *testing.T) {
	err := ValidateProtocol("grpc")
	if err == nil {
		t.Fatal("expected error")
	}
	for _, sub := range []string{ProtocolAnthropic, ProtocolOpenAIChatCompletions, ProtocolOpenAIResponses} {
		if !strings.Contains(err.Error(), sub) {
			t.Errorf("error %q should mention %q", err.Error(), sub)
		}
	}
}

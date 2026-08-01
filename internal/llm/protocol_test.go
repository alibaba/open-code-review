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
		{"anthropic case-insensitive", "ANTHROPIC", ProtocolAnthropic},
		{"openai-responses case-insensitive", "OpenAI-Responses", ProtocolOpenAIResponses},
		{"unknown passthrough lowercased", "gRPC", "grpc"},
		{"unknown anthropic-vertex preserved", "anthropic-vertex", "anthropic-vertex"},
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

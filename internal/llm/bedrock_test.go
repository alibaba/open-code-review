// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes an OCR config file to a temp dir and returns its path.
func writeConfig(t *testing.T, cfg map[string]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// TestBedrockProtocolIsRecognized guards the three-part contract documented in
// protocol.go: a new protocol needs a constant, a NormalizeProtocol case, and a
// ValidateProtocol entry. Missing the last one turns a valid config into
// "unsupported protocol".
func TestBedrockProtocolIsRecognized(t *testing.T) {
	for _, raw := range []string{"anthropic-bedrock", "ANTHROPIC-BEDROCK", "  Anthropic-Bedrock  "} {
		if got := NormalizeProtocol(raw); got != ProtocolAnthropicBedrock {
			t.Errorf("NormalizeProtocol(%q) = %q, want %q", raw, got, ProtocolAnthropicBedrock)
		}
	}
	if err := ValidateProtocol(ProtocolAnthropicBedrock); err != nil {
		t.Errorf("ValidateProtocol(%q) = %v, want nil", ProtocolAnthropicBedrock, err)
	}
}

// TestBedrockProviderIsRegistered pins the preset's shape. An api_key or a
// BaseURL here would be wrong: credentials come from the AWS chain and the host
// is derived from the region.
func TestBedrockProviderIsRegistered(t *testing.T) {
	p, ok := LookupProvider("bedrock")
	if !ok {
		t.Fatal("LookupProvider(\"bedrock\") not found")
	}
	if p.Protocol != ProtocolAnthropicBedrock {
		t.Errorf("Protocol = %q, want %q", p.Protocol, ProtocolAnthropicBedrock)
	}
	if !p.AmbientAuth {
		t.Error("AmbientAuth = false, want true — bedrock signs with SigV4 and has no api_key")
	}
	if p.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty — the region determines the bedrock-runtime host", p.BaseURL)
	}
	if p.EnvVar != "" {
		t.Errorf("EnvVar = %q, want empty — there is no API key env var to fall back to", p.EnvVar)
	}
}

// TestResolveBedrockWithoutAPIKey is the regression test for the two gates that
// rejected a correct Bedrock config: the api_key requirement in
// tryProviderConfig, and the URL-and-Token completeness check in
// ResolveEndpointWithModelOverride. Either one turns a valid setup into
// "no valid LLM endpoint configured", which reads as "you forgot to configure
// anything".
func TestResolveBedrockWithoutAPIKey(t *testing.T) {
	path := writeConfig(t, map[string]any{
		"provider":  "bedrock",
		"model":     "us.anthropic.claude-sonnet-4-6",
		"providers": map[string]any{"bedrock": map[string]any{}},
	})

	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.Protocol != ProtocolAnthropicBedrock {
		t.Errorf("Protocol = %q, want %q", ep.Protocol, ProtocolAnthropicBedrock)
	}
	if !ep.AmbientAuth {
		t.Error("AmbientAuth = false, want true")
	}
	if ep.Token != "" {
		t.Errorf("Token = %q, want empty", ep.Token)
	}
	if ep.Model != "us.anthropic.claude-sonnet-4-6" {
		t.Errorf("Model = %q, want us.anthropic.claude-sonnet-4-6", ep.Model)
	}
}

// TestResolveBedrockPassesAWSSettings covers aws_profile / aws_region reaching
// the client, so a review run is reproducible without exporting AWS_PROFILE.
func TestResolveBedrockPassesAWSSettings(t *testing.T) {
	path := writeConfig(t, map[string]any{
		"provider": "bedrock",
		"model":    "us.anthropic.claude-sonnet-4-6",
		"providers": map[string]any{
			"bedrock": map[string]any{"aws_profile": "example-profile", "aws_region": "us-west-2"},
		},
	})

	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.AWSProfile != "example-profile" {
		t.Errorf("AWSProfile = %q, want example-profile", ep.AWSProfile)
	}
	if ep.AWSRegion != "us-west-2" {
		t.Errorf("AWSRegion = %q, want us-west-2", ep.AWSRegion)
	}

	cfg := ClientConfig{}
	if c, ok := NewLLMClient(ep, nil).(*AnthropicClient); ok {
		cfg = c.cfg
	} else {
		t.Fatal("NewLLMClient did not return *AnthropicClient for the bedrock protocol")
	}
	if cfg.AWSProfile != "example-profile" || cfg.AWSRegion != "us-west-2" {
		t.Errorf("ClientConfig AWS settings = %q/%q, want example-profile/us-west-2", cfg.AWSProfile, cfg.AWSRegion)
	}
}

// TestNonAmbientProviderStillRequiresAPIKey makes sure relaxing the gate for
// ambient auth did not relax it for everyone.
func TestNonAmbientProviderStillRequiresAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	path := writeConfig(t, map[string]any{
		"provider":  "anthropic",
		"model":     "claude-opus-4-6",
		"providers": map[string]any{"anthropic": map[string]any{}},
	})

	if _, err := ResolveEndpoint(path); err == nil {
		t.Fatal("ResolveEndpoint succeeded with no api_key for a non-ambient provider; want an error")
	}
}

// TestExplainErrorClassifiesBedrockFailures covers the diagnosis Bedrock's own
// wording does not give. Two of these are actively misleading: the API-key
// complaint names a credential the user cannot configure, and a model absent
// from the region reads as a malformed identifier.
func TestExplainErrorClassifiesBedrockFailures(t *testing.T) {
	client := &AnthropicClient{bedrock: true, awsRegion: "us-west-2", awsProfile: "example-profile"}

	tests := []struct {
		name     string
		err      error
		wantAll  []string
		wantNone []string
	}{
		{
			name:    "bearer token reached the request",
			err:     errors.New(`403 Forbidden {"Message":"Invalid API Key format: Must start with pre-defined prefix"}`),
			wantAll: []string{"no api_key applies to bedrock", "region us-west-2", "profile example-profile"},
		},
		{
			name:    "expired session",
			err:     errors.New("operation error: get credentials: ExpiredToken: the security token included in the request is expired"),
			wantAll: []string{"aws sso login --profile example-profile"},
		},
		{
			name:    "IAM gap, not a bad credential",
			err:     errors.New("operation error Bedrock Runtime: AccessDeniedException: user is not authorized to perform this action"),
			wantAll: []string{"bedrock:InvokeModel", "IAM policy gap"},
		},
		{
			name:    "model absent from the region",
			err:     errors.New("operation error Bedrock Runtime: ValidationException: The provided model identifier is invalid."),
			wantAll: []string{"aws bedrock list-inference-profiles --region us-west-2", "-v1:0"},
		},
		{
			name:     "anything else keeps its own wording and gains context",
			err:      errors.New("connection reset by peer"),
			wantAll:  []string{"connection reset by peer", "region us-west-2"},
			wantNone: []string{"IAM", "sso login"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := client.explainError("us.anthropic.claude-sonnet-4-6", tc.err)
			if got == nil {
				t.Fatal("explainError returned nil for a non-nil error")
			}
			if !errors.Is(got, tc.err) {
				t.Error("original error is not wrapped; callers lose the service's own message")
			}
			for _, want := range tc.wantAll {
				if !strings.Contains(got.Error(), want) {
					t.Errorf("message %q does not contain %q", got, want)
				}
			}
			for _, unwanted := range tc.wantNone {
				if strings.Contains(got.Error(), unwanted) {
					t.Errorf("message %q should not mention %q", got, unwanted)
				}
			}
		})
	}
}

// TestExplainErrorNamesTheBearerTokenVariable separates the two ways the same
// 403 arrives: an SSO token the SDK attached on its own, versus a token the user
// set deliberately. The fix differs, so the message has to.
func TestExplainErrorNamesTheBearerTokenVariable(t *testing.T) {
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "sk-not-a-real-token")
	client := &AnthropicClient{bedrock: true, awsRegion: "us-west-2"}
	err := client.explainError("m", errors.New(`{"Message":"Invalid API Key format: Must start with pre-defined prefix"}`))
	if !strings.Contains(err.Error(), "AWS_BEARER_TOKEN_BEDROCK") {
		t.Errorf("message %q does not name AWS_BEARER_TOKEN_BEDROCK", err)
	}
}

// TestExplainErrorLeavesNonBedrockErrorsAlone keeps the diagnosis scoped: every
// other protocol shares this client type.
func TestExplainErrorLeavesNonBedrockErrorsAlone(t *testing.T) {
	client := &AnthropicClient{}
	original := errors.New("401 Unauthorized")
	if got := client.explainError("m", original); got != original {
		t.Errorf("explainError rewrote a non-bedrock error: %q", got)
	}
	if got := client.explainError("m", nil); got != nil {
		t.Errorf("explainError(nil) = %v, want nil", got)
	}
}

// TestBedrockContextReportsResolvedRegion covers what `ocr llm test` prints:
// bedrock has no configured URL, so the resolved region is the only way to see
// where a request went.
func TestBedrockContextReportsResolvedRegion(t *testing.T) {
	client := &AnthropicClient{bedrock: true, awsRegion: "us-west-2", awsProfile: "example-profile"}
	region, profile, ok := client.BedrockContext()
	if !ok {
		t.Fatal("ok = false for a bedrock client")
	}
	if region != "us-west-2" || profile != "example-profile" {
		t.Errorf("BedrockContext() = %q/%q, want us-west-2/example-profile", region, profile)
	}

	if _, _, ok := (&AnthropicClient{}).BedrockContext(); ok {
		t.Error("ok = true for a non-bedrock client")
	}
}

// TestBedrockClientReportsAWSFailureAsError is the guard against the SDK's
// bedrock.WithLoadDefaultConfig, which panics when AWS config cannot be loaded.
// A CLI must not hand a user a stack trace because their session expired, so the
// failure is deferred to the first request instead.
func TestBedrockClientReportsAWSFailureAsError(t *testing.T) {
	// An unresolvable profile makes LoadDefaultConfig fail deterministically.
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "nonexistent-config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "nonexistent-creds"))

	client := NewAnthropicBedrockClient(ClientConfig{
		Model:      "us.anthropic.claude-sonnet-4-6",
		AWSProfile: "definitely-not-a-real-profile",
	})
	if client == nil {
		t.Fatal("NewAnthropicBedrockClient returned nil; it must always return a client so the error can surface per-request")
	}
	if client.initErr == nil {
		t.Skip("this environment resolved an AWS config for a bogus profile; nothing to assert")
	}
	if _, err := client.CompletionsWithCtx(t.Context(), ChatRequest{}); err == nil {
		t.Error("CompletionsWithCtx returned nil error despite a construction failure")
	}
}

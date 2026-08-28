// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/codexauth"
)

func codexResolverConfig(t *testing.T) string {
	t.Helper()
	path, _ := writeResolverConfig(t, configFile{
		Provider: "codex",
		Providers: map[string]providerEntryConfig{
			"codex": {Model: "gpt-5.6-luna"},
		},
	})
	return path
}

func TestResolveEndpoint_CodexUsesStoredCredential(t *testing.T) {
	clearAllEnv(t)
	setTestHome(t, t.TempDir())
	wantToken := "access-token-fixture"
	if err := codexauth.Save(&codexauth.CodexAuth{
		AccessToken:  wantToken,
		RefreshToken: "refresh-token-fixture",
		ExpiresAt:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save Codex auth: %v", err)
	}

	ep, err := ResolveEndpoint(codexResolverConfig(t))
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.Token != wantToken {
		t.Errorf("Token = %q, want stored access token", ep.Token)
	}
	if !ep.RequiresStreaming {
		t.Error("RequiresStreaming = false, want true")
	}
	if ep.Protocol != ProtocolOpenAIResponses {
		t.Errorf("Protocol = %q, want %q", ep.Protocol, ProtocolOpenAIResponses)
	}
}

func TestResolveEndpoint_CodexMissingCredentialIsActionable(t *testing.T) {
	clearAllEnv(t)
	setTestHome(t, t.TempDir())

	_, err := ResolveEndpoint(codexResolverConfig(t))
	if err == nil {
		t.Fatal("ResolveEndpoint returned nil error")
	}
	for _, want := range []string{"codex provider is selected but not signed in", "ocr auth login"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestResolveEndpoint_CodexStaticKeyTakesPrecedence(t *testing.T) {
	clearAllEnv(t)
	setTestHome(t, t.TempDir())
	path, _ := writeResolverConfig(t, configFile{
		Provider: "codex",
		Providers: map[string]providerEntryConfig{
			"codex": {APIKey: "configured-key", Model: "gpt-5.6-terra"},
		},
	})

	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.Token != "configured-key" {
		t.Errorf("Token = %q, want configured-key", ep.Token)
	}
}

func TestResolveEndpoint_CodexModelOverrideRemainsAllowlisted(t *testing.T) {
	clearAllEnv(t)
	setTestHome(t, t.TempDir())
	if err := codexauth.Save(&codexauth.CodexAuth{
		AccessToken:  "access-token-fixture",
		RefreshToken: "refresh-token-fixture",
		ExpiresAt:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save Codex auth: %v", err)
	}

	_, err := ResolveEndpointWithModelOverride(codexResolverConfig(t), "gpt-5.6-sol")
	if err == nil || !strings.Contains(err.Error(), "not available for provider \"codex\"") {
		t.Errorf("ResolveEndpointWithModelOverride error = %v", err)
	}
}

func TestNewLLMClient_CarriesRequiresStreaming(t *testing.T) {
	client, ok := NewLLMClient(ResolvedEndpoint{
		URL:               "https://example.com/v1",
		Token:             "token",
		Model:             "model",
		Protocol:          ProtocolOpenAIResponses,
		RequiresStreaming: true,
	}, nil).(*OpenAIResponsesClient)
	if !ok {
		t.Fatal("NewLLMClient did not return OpenAIResponsesClient")
	}
	if !client.cfg.RequiresStreaming {
		t.Error("client RequiresStreaming = false, want true")
	}
}

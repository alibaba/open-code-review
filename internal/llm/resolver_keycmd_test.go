// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigJSON(t *testing.T, cfg configFile) string {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

// (a) api_key_cmd resolves when no static key is present.
func TestResolveEndpoint_ProviderAPIKeyCmd(t *testing.T) {
	clearAllEnv(t)
	cfgPath := writeConfigJSON(t, configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKeyCmd: "printf 'sk-from-cmd\\n'", Model: "claude-sonnet-4-6"},
		},
	})
	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "sk-from-cmd" {
		t.Errorf("Token = %q, want %q", ep.Token, "sk-from-cmd")
	}
}

// (b) static api_key wins even when api_key_cmd is also set.
func TestResolveEndpoint_ProviderStaticKeyWinsOverCmd(t *testing.T) {
	clearAllEnv(t)
	cfgPath := writeConfigJSON(t, configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKey: "sk-static", APIKeyCmd: "printf 'sk-from-cmd\\n'", Model: "claude-sonnet-4-6"},
		},
	})
	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "sk-static" {
		t.Errorf("Token = %q, want %q (static api_key must win)", ep.Token, "sk-static")
	}
}

// (c) custom provider with api_key_cmd resolves (custom providers have no env fallback).
func TestResolveEndpoint_CustomProviderAPIKeyCmd(t *testing.T) {
	clearAllEnv(t)
	cfgPath := writeConfigJSON(t, configFile{
		Provider: "my-gateway",
		CustomProviders: map[string]providerEntryConfig{
			"my-gateway": {
				APIKeyCmd: "printf 'gw-token\\n'",
				URL:       "https://gateway.internal.com/v1",
				Protocol:  "openai",
				Model:     "llama-3-8b",
			},
		},
	})
	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "gw-token" {
		t.Errorf("Token = %q, want %q", ep.Token, "gw-token")
	}
}

// (d) a failing api_key_cmd is a hard error, not a silent fallback.
func TestResolveEndpoint_ProviderAPIKeyCmdFailsHard(t *testing.T) {
	clearAllEnv(t)
	cfgPath := writeConfigJSON(t, configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKeyCmd: "exit 7", Model: "claude-sonnet-4-6"},
		},
	})
	_, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatal("expected hard error from failing api_key_cmd, got nil")
	}
	if !strings.Contains(err.Error(), "api_key_cmd") {
		t.Errorf("error %q does not mention api_key_cmd", err.Error())
	}
}

// (e) legacy auth_token_cmd resolves on an otherwise-complete llm block.
func TestResolveEndpoint_LegacyAuthTokenCmd(t *testing.T) {
	clearAllEnv(t)
	cfgPath := writeConfigJSON(t, configFile{
		Llm: llmFileConfig{
			URL:          "https://api.example.com/v1/messages",
			AuthTokenCmd: "printf 'legacy-token\\n'",
			Model:        "claude-sonnet-4-6",
		},
	})
	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "legacy-token" {
		t.Errorf("Token = %q, want %q", ep.Token, "legacy-token")
	}
}

// (f) an incomplete legacy block (missing url) with auth_token_cmd set does NOT
// run the command and falls through to later strategies.
func TestResolveEndpoint_LegacyIncompleteDoesNotRunCmd(t *testing.T) {
	clearAllEnv(t)
	// Command would exit non-zero if ever executed; if it ran, we'd see that
	// error instead of the generic "no valid endpoint" fall-through error.
	cfgPath := writeConfigJSON(t, configFile{
		Llm: llmFileConfig{
			AuthTokenCmd: "exit 9",
			Model:        "claude-sonnet-4-6",
			// URL intentionally omitted -> incomplete
		},
	})
	_, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatal("expected no-endpoint error, got nil")
	}
	if strings.Contains(err.Error(), "auth_token_cmd") {
		t.Errorf("command should not have run for incomplete legacy config; error: %v", err)
	}
	if !strings.Contains(err.Error(), "no valid LLM endpoint") {
		t.Errorf("expected fall-through no-endpoint error, got: %v", err)
	}
}

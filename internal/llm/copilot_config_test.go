// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopilotPresetResolution(t *testing.T) {
	cfg := configFile{
		Provider: "copilot",
		Providers: map[string]providerEntryConfig{
			"copilot": {Model: "gpt-4.1"},
		},
	}
	ep, ok, err := tryProviderConfig(cfg, "account-specific-model")
	if err != nil || !ok {
		t.Fatalf("resolve Copilot preset = (%#v, %t, %v)", ep, ok, err)
	}
	if ep.Protocol != ProtocolCopilot || ep.URL != "" || ep.Token != "" || !ep.AmbientAuth || ep.Model != "account-specific-model" {
		t.Fatalf("Copilot endpoint = %#v", ep)
	}
	if _, ok := NewLLMClient(ep, nil, nil).(*CopilotClient); !ok {
		t.Fatal("Copilot endpoint did not create a CopilotClient")
	}
}

func TestCopilotExplicitProviderNeedsNoConfigEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"provider":"deepseek","providers":{"deepseek":{"model":"deepseek-chat","api_key":"unused"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, configPath := range []string{path, filepath.Join(t.TempDir(), "missing.json")} {
		ep, err := ResolveEndpointWithOptions(configPath, ResolveOptions{Provider: "copilot"})
		if err != nil {
			t.Fatalf("resolve Copilot without an entry (%s): %v", configPath, err)
		}
		if ep.Provider != "copilot" || ep.Protocol != ProtocolCopilot || ep.Model != "auto" || !ep.AmbientAuth {
			t.Fatalf("Copilot endpoint = %#v", ep)
		}
	}
}

func TestOtherExplicitProviderStillRequiresConfigEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	if _, err := ResolveEndpointWithOptions(path, ResolveOptions{Provider: "deepseek"}); err == nil {
		t.Fatal("non-Copilot provider resolved without an entry")
	}
}

func TestCopilotPresetRejectsUnusedEndpointFields(t *testing.T) {
	cases := []struct {
		name  string
		entry providerEntryConfig
	}{
		{"url", providerEntryConfig{Model: "gpt-4.1", URL: "https://example.com"}},
		{"key", providerEntryConfig{Model: "gpt-4.1", APIKey: "unused"}},
		{"key command", providerEntryConfig{Model: "gpt-4.1", APIKeyCmd: "echo unused"}},
		{"protocol override", providerEntryConfig{Model: "gpt-4.1", Protocol: ProtocolOpenAIChatCompletions}},
		{"headers", providerEntryConfig{Model: "gpt-4.1", ExtraHeaders: map[string]string{"X-Test": "unused"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := tryProviderConfig(configFile{Provider: "copilot", Providers: map[string]providerEntryConfig{"copilot": tc.entry}}, "")
			if err == nil {
				t.Fatal("unused endpoint field was accepted")
			}
		})
	}
}

func TestCopilotProtocolIsPresetOnly(t *testing.T) {
	_, _, err := tryProviderConfig(configFile{Provider: "other", CustomProviders: map[string]providerEntryConfig{
		"other": {Protocol: ProtocolCopilot, Model: "gpt-4.1"},
	}}, "")
	if err == nil || !strings.Contains(err.Error(), "copilot") {
		t.Fatalf("custom Copilot protocol error = %v", err)
	}
	if err := ValidateProtocol(ProtocolCopilot); err != nil {
		t.Fatal(err)
	}
	if NormalizeProtocol(" COPILOT ") != ProtocolCopilot {
		t.Fatal("Copilot protocol was not normalized")
	}
}

func TestCopilotURLTokenStrategiesRejected(t *testing.T) {
	t.Setenv(envOCRLLMURL, "https://example.com")
	t.Setenv(envOCRLLMToken, "unused")
	t.Setenv(envOCRLLMModel, "gpt-4.1")
	t.Setenv(envOCRLLMProtocol, ProtocolCopilot)
	if _, _, err := tryOCREnv(""); err == nil {
		t.Fatal("OCR URL/token environment accepted Copilot protocol")
	}
	_, _, err := tryLegacyLlmConfig(configFile{Llm: llmFileConfig{
		URL: "https://example.com", AuthToken: "unused", Model: "gpt-4.1", Protocol: ProtocolCopilot,
	}}, "")
	if err == nil {
		t.Fatal("legacy URL/token config accepted Copilot protocol")
	}
}

func TestCopilotRejectsGlobalHTTPHeaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"provider":"copilot","providers":{"copilot":{"model":"gpt-4.1"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envOCRLLMExtraHeaders, "X-Test=unused")
	if _, err := ResolveEndpointWithOptions(path, ResolveOptions{}); err == nil || !strings.Contains(err.Error(), envOCRLLMExtraHeaders) {
		t.Fatalf("global header error = %v", err)
	}
}

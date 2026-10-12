// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"net/http"
	"strings"
	"testing"
)

func TestResolveEndpoint_OpenCodeGoPresetHeaders(t *testing.T) {
	clearAllEnv(t)

	t.Run("preset session header applies by default", func(t *testing.T) {
		path, _ := writeResolverConfig(t, configFile{
			Provider:  "opencode-go",
			Providers: map[string]providerEntryConfig{"opencode-go": {APIKey: "sk-go", Model: "kimi-k3"}},
		})
		ep, err := ResolveEndpoint(path)
		if err != nil {
			t.Fatalf("ResolveEndpoint: %v", err)
		}
		if ep.URL != "https://opencode.ai/zen/go/v1" || ep.Protocol != ProtocolOpenAIChatCompletions {
			t.Errorf("endpoint = %s %s, want the Go chat completions endpoint", ep.Protocol, ep.URL)
		}
		if got := wireHeaders(ep.ExtraHeaders).Get("x-opencode-session"); got != SessionKeyTemplateVar {
			t.Errorf("x-opencode-session = %q, want %q", got, SessionKeyTemplateVar)
		}
	})

	t.Run("entry headers override the preset whatever their case", func(t *testing.T) {
		path, _ := writeResolverConfig(t, configFile{
			Provider: "opencode-go",
			Providers: map[string]providerEntryConfig{"opencode-go": {
				APIKey: "sk-go",
				Model:  "kimi-k3",
				ExtraHeaders: map[string]string{
					"X-OpenCode-Session": "fixed",
					"x-team":             "review",
				},
			}},
		})
		ep, err := ResolveEndpoint(path)
		if err != nil {
			t.Fatalf("ResolveEndpoint: %v", err)
		}
		// Two spellings of one header would race in map order when the
		// client applies them, so the merge must leave exactly one.
		h := wireHeaders(ep.ExtraHeaders)
		if got := h.Values("x-opencode-session"); len(got) != 1 || got[0] != "fixed" || h.Get("x-team") != "review" {
			t.Errorf("ExtraHeaders = %v, want one session header holding the entry value", ep.ExtraHeaders)
		}
		if p, _ := LookupProvider("opencode-go"); p.ExtraHeaders["x-opencode-session"] != SessionKeyTemplateVar {
			t.Error("resolving mutated the registry's preset headers")
		}
	})
}

// wireHeaders applies m with Add rather than the clients' Set, so a header name
// the merge left duplicated shows up as two values instead of racing.
func wireHeaders(m map[string]string) http.Header {
	h := http.Header{}
	for k, v := range m {
		h.Add(k, v)
	}
	return h
}

func TestResolveEndpoint_OpenCodeGoRoutesProtocolByModel(t *testing.T) {
	clearAllEnv(t)
	tests := []struct {
		model, entryProtocol, wantProtocol, wantURL, wantAuth string
	}{
		{"kimi-k3", "", ProtocolOpenAIChatCompletions, "https://opencode.ai/zen/go/v1", ""},
		{"grok-4.7", "", ProtocolOpenAIResponses, "https://opencode.ai/zen/go/v1", ""},
		{"qwen3.8-max", "", ProtocolAnthropic, "https://opencode.ai/zen/go/v1/messages", "x-api-key"},
		{"qwen3.8-max", "openai", ProtocolOpenAIChatCompletions, "https://opencode.ai/zen/go/v1", ""},
	}
	for _, tt := range tests {
		t.Run(tt.model+"/"+tt.entryProtocol, func(t *testing.T) {
			path, _ := writeResolverConfig(t, configFile{
				Provider: "opencode-go",
				Providers: map[string]providerEntryConfig{"opencode-go": {
					APIKey: "sk-go", Model: tt.model, Protocol: tt.entryProtocol,
				}},
			})
			ep, err := ResolveEndpoint(path)
			if err != nil {
				t.Fatalf("ResolveEndpoint: %v", err)
			}
			if ep.Protocol != tt.wantProtocol || ep.URL != tt.wantURL || ep.AuthHeader != tt.wantAuth {
				t.Errorf("endpoint = %s %s auth=%q, want %s %s auth=%q", ep.Protocol, ep.URL, ep.AuthHeader, tt.wantProtocol, tt.wantURL, tt.wantAuth)
			}
		})
	}

	t.Run("--model override picks its own protocol", func(t *testing.T) {
		path, _ := writeResolverConfig(t, configFile{
			Provider:  "opencode-go",
			Providers: map[string]providerEntryConfig{"opencode-go": {APIKey: "sk-go", Model: "kimi-k3"}},
		})
		ep, err := ResolveEndpointWithModelOverride(path, "gpt-6-luna")
		if err != nil {
			t.Fatalf("ResolveEndpoint: %v", err)
		}
		if ep.Protocol != ProtocolOpenAIResponses {
			t.Errorf("Protocol = %s, want %s", ep.Protocol, ProtocolOpenAIResponses)
		}
	})
}

// A routed model missing from Models could not be picked in the TUI or passed
// with --model, since the list gates overrides.
func TestProviders_ModelProtocolsAreListedAndValid(t *testing.T) {
	for _, p := range ListProviders() {
		for model, protocol := range p.ModelProtocols {
			if !ModelListContains(p.Models, model) {
				t.Errorf("%s: routed model %q is not in Models", p.Name, model)
			}
			if err := ValidateProtocol(protocol); err != nil {
				t.Errorf("%s: model %q: %v", p.Name, model, err)
			}
		}
	}
}

func TestResolveEndpoint_APIKeysOrderAndPromotion(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OPENCODE_API_KEY", "sk-env")
	tests := []struct {
		name         string
		entry        providerEntryConfig
		wantToken    string
		wantFallback []string
	}{
		{"api_key first, then api_keys without repeats or blanks",
			providerEntryConfig{APIKey: "a", APIKeys: []string{"b", "a", "  ", "c", "b"}}, "a", []string{"b", "c"}},
		{"api_keys alone promotes its first key",
			providerEntryConfig{APIKeys: []string{"x", "y"}}, "x", []string{"y"}},
		{"a single api_keys entry has nothing to fail over to",
			providerEntryConfig{APIKeys: []string{"x"}}, "x", nil},
		{"static keys shadow the env var",
			providerEntryConfig{APIKeys: []string{" ", "x"}}, "x", nil},
		{"no static keys falls back to the env var",
			providerEntryConfig{APIKeys: []string{" "}}, "sk-env", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.entry.Model = "kimi-k3"
			path, _ := writeResolverConfig(t, configFile{
				Provider:  "opencode-go",
				Providers: map[string]providerEntryConfig{"opencode-go": tt.entry},
			})
			ep, err := ResolveEndpoint(path)
			if err != nil {
				t.Fatalf("ResolveEndpoint: %v", err)
			}
			if ep.Token != tt.wantToken || strings.Join(ep.FallbackTokens, ",") != strings.Join(tt.wantFallback, ",") {
				t.Errorf("keys = %q + %q, want %q + %q", ep.Token, ep.FallbackTokens, tt.wantToken, tt.wantFallback)
			}
		})
	}
}

func TestResolveEndpoint_APIKeysOnCustomProvider(t *testing.T) {
	clearAllEnv(t)
	path, _ := writeResolverConfig(t, configFile{
		Provider: "gw",
		CustomProviders: map[string]providerEntryConfig{"gw": {
			URL: "https://gw.example.com/v1", Protocol: "openai", Model: "m", APIKeys: []string{"k1", "k2"},
		}},
	})
	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.Token != "k1" || len(ep.FallbackTokens) != 1 || ep.FallbackTokens[0] != "k2" {
		t.Errorf("keys = %q + %q, want k1 + [k2]", ep.Token, ep.FallbackTokens)
	}
}

// api_keys adds fallbacks behind api_key_cmd rather than displacing it: the
// failing command surfacing proves it ran instead of api_keys[0] being
// promoted over it.
func TestResolveEndpoint_APIKeysDoNotShadowAPIKeyCmd(t *testing.T) {
	clearAllEnv(t)
	path, _ := writeResolverConfig(t, configFile{
		Provider: "opencode-go",
		Providers: map[string]providerEntryConfig{"opencode-go": {
			APIKeyCmd: "exit 3", APIKeys: []string{"k1"}, Model: "kimi-k3",
		}},
	})
	if ep, err := ResolveEndpoint(path); err == nil {
		t.Fatalf("resolved with token %q, want the api_key_cmd failure", ep.Token)
	}
}

func TestResolveEndpoint_APIKeysDedupAgainstAPIKeyCmdOutput(t *testing.T) {
	clearAllEnv(t)
	path, _ := writeResolverConfig(t, configFile{
		Provider: "opencode-go",
		Providers: map[string]providerEntryConfig{"opencode-go": {
			APIKeyCmd: "echo k1", APIKeys: []string{"k1", "k2"}, Model: "kimi-k3",
		}},
	})
	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.Token != "k1" || strings.Join(ep.FallbackTokens, ",") != "k2" {
		t.Errorf("keys = %q + %q, want k1 + [k2]", ep.Token, ep.FallbackTokens)
	}
}

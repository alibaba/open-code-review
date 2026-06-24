package main

import (
	"testing"
)

func TestSetConfigValueAuthHeaderNormalizesKnownValues(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "llm.auth_header", " bearer "); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}

	if cfg.Llm.AuthHeader != "authorization" {
		t.Errorf("AuthHeader = %q, want %q", cfg.Llm.AuthHeader, "authorization")
	}
}

func TestSetConfigValueAuthHeaderRejectsCustomHeader(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "llm.auth_header", " X-Custom-Auth "); err == nil {
		t.Fatal("expected error for unsupported auth_header, got nil")
	}
}

func TestSetConfigValueProvider(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "provider", "anthropic"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", cfg.Provider, "anthropic")
	}
}

func TestSetConfigValueModel(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "model", "claude-opus-4-6"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Model != "claude-opus-4-6" {
		t.Errorf("Model = %q, want %q", cfg.Model, "claude-opus-4-6")
	}
}

func TestSetConfigValueModelWithProvider(t *testing.T) {
	cfg := &Config{
		Provider: "anthropic",
		Providers: map[string]ProviderEntry{
			"anthropic": {APIKey: "sk-test"},
		},
	}

	if err := setConfigValue(cfg, "model", "claude-opus-4-6"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Providers["anthropic"].Model != "claude-opus-4-6" {
		t.Errorf("entry Model = %q, want %q", cfg.Providers["anthropic"].Model, "claude-opus-4-6")
	}
	if cfg.Model != "" {
		t.Errorf("top-level Model = %q, want empty (should write to provider entry)", cfg.Model)
	}
}

func TestSetConfigValueProviderEntry(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "providers.anthropic.api_key", "sk-ant-test"); err != nil {
		t.Fatalf("setConfigValue api_key: %v", err)
	}
	if cfg.Providers["anthropic"].APIKey != "sk-ant-test" {
		t.Errorf("api_key = %q, want %q", cfg.Providers["anthropic"].APIKey, "sk-ant-test")
	}

	if err := setConfigValue(cfg, "providers.anthropic.model", "claude-opus-4-6"); err != nil {
		t.Fatalf("setConfigValue model: %v", err)
	}
	if cfg.Providers["anthropic"].Model != "claude-opus-4-6" {
		t.Errorf("model = %q, want %q", cfg.Providers["anthropic"].Model, "claude-opus-4-6")
	}
}

func TestSetConfigValueProviderEntryNonPresetWritesCustomProvider(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "providers.my-gateway.url", "https://gateway.internal.com/v1"); err != nil {
		t.Fatalf("setConfigValue url: %v", err)
	}

	if cfg.Providers != nil {
		if _, ok := cfg.Providers["my-gateway"]; ok {
			t.Fatal("non-preset providers.<name> should be stored in CustomProviders, not Providers")
		}
	}
	if cfg.CustomProviders["my-gateway"].URL != "https://gateway.internal.com/v1" {
		t.Errorf("custom provider URL = %q", cfg.CustomProviders["my-gateway"].URL)
	}
}

func TestSetConfigValueProviderEntryModelsJSON(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "custom_providers.my-gateway.models", `["llama-3-70b","llama-3-8b","llama-3-70b"]`); err != nil {
		t.Fatalf("setConfigValue models: %v", err)
	}

	got := cfg.CustomProviders["my-gateway"].Models
	want := []string{"llama-3-70b", "llama-3-8b"}
	if len(got) != len(want) {
		t.Fatalf("models length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("models[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSetConfigValueProviderEntryModelsCommaSeparated(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "custom_providers.my-gateway.models", " llama-3-70b, llama-3-8b ,, llama-3-70b "); err != nil {
		t.Fatalf("setConfigValue models: %v", err)
	}

	got := cfg.CustomProviders["my-gateway"].Models
	want := []string{"llama-3-70b", "llama-3-8b"}
	if len(got) != len(want) {
		t.Fatalf("models length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("models[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSetConfigValueProviderEntryModelsUnquotedBracketList(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "custom_providers.my-gateway.models", "[llama-3-70b,llama-3-8b]"); err != nil {
		t.Fatalf("setConfigValue models: %v", err)
	}

	got := cfg.CustomProviders["my-gateway"].Models
	want := []string{"llama-3-70b", "llama-3-8b"}
	if len(got) != len(want) {
		t.Fatalf("models length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("models[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSetConfigValueProviderEntryProtocol(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "custom_providers.custom.protocol", "openai"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.CustomProviders["custom"].Protocol != "openai" {
		t.Errorf("protocol = %q, want %q", cfg.CustomProviders["custom"].Protocol, "openai")
	}

	if err := setConfigValue(cfg, "custom_providers.custom.protocol", "invalid"); err == nil {
		t.Fatal("expected error for invalid protocol")
	}
}

func TestSetConfigValueProviderEntryInvalidKey(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "providers.anthropic.unknown_field", "value"); err == nil {
		t.Fatal("expected error for unknown provider field")
	}
}

func TestSetConfigValueProviderEntryInvalidPath(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "providers.anthropic", "value"); err == nil {
		t.Fatal("expected error for incomplete provider path")
	}
}

func TestSetConfigValueProviderEntryExtraBody(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "providers.anthropic.extra_body", `{"thinking":{"type":"disabled"}}`); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Providers["anthropic"].ExtraBody == nil {
		t.Fatal("extra_body should not be nil")
	}
	if _, ok := cfg.Providers["anthropic"].ExtraBody["thinking"]; !ok {
		t.Error("extra_body missing 'thinking' key")
	}
}

func TestSetConfigValueModelWithCustomProvider(t *testing.T) {
	cfg := &Config{
		Provider: "my-gateway",
		CustomProviders: map[string]ProviderEntry{
			"my-gateway": {URL: "https://gw.example.com/v1", Protocol: "openai"},
		},
	}

	if err := setConfigValue(cfg, "model", "llama-3-70b"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.CustomProviders["my-gateway"].Model != "llama-3-70b" {
		t.Errorf("entry Model = %q, want %q", cfg.CustomProviders["my-gateway"].Model, "llama-3-70b")
	}
	if cfg.Model != "" {
		t.Errorf("top-level Model = %q, want empty (should write to custom provider entry)", cfg.Model)
	}
}

// --- unset tests ---

func TestParseConfigArgsUnset(t *testing.T) {
	action, err := parseConfigArgs([]string{"unset", "custom_providers.my-gateway"})
	if err != nil {
		t.Fatalf("parseConfigArgs: %v", err)
	}
	if action.subCmd != "unset" {
		t.Errorf("subCmd = %q, want %q", action.subCmd, "unset")
	}
	if action.key != "custom_providers.my-gateway" {
		t.Errorf("key = %q, want %q", action.key, "custom_providers.my-gateway")
	}
}

func TestParseConfigArgsUnsetMissingKey(t *testing.T) {
	_, err := parseConfigArgs([]string{"unset"})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestUnsetCustomProvider(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.json"

	cfg := &Config{
		Provider: "anthropic",
		CustomProviders: map[string]ProviderEntry{
			"my-gateway": {URL: "https://gw.example.com/v1", Protocol: "openai", Model: "llama-3"},
		},
	}
	if err := saveConfig(configPath, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	if err := unsetCustomProvider(configPath, "my-gateway"); err != nil {
		t.Fatalf("unsetCustomProvider: %v", err)
	}

	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg.CustomProviders != nil {
		t.Errorf("CustomProviders should be nil after deleting the only entry, got %v", cfg.CustomProviders)
	}
	if cfg.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q (should be untouched)", cfg.Provider, "anthropic")
	}
}

func TestUnsetActiveCustomProvider(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.json"

	cfg := &Config{
		Provider: "my-gateway",
		Model:    "fallback-model",
		CustomProviders: map[string]ProviderEntry{
			"my-gateway":    {URL: "https://gw.example.com/v1", Protocol: "openai", Model: "llama-3"},
			"other-gateway": {URL: "https://other.example.com/v1", Protocol: "openai", Model: "other-model"},
		},
	}
	if err := saveConfig(configPath, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	if err := unsetCustomProvider(configPath, "my-gateway"); err != nil {
		t.Fatalf("unsetCustomProvider: %v", err)
	}

	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg.Provider != "" {
		t.Errorf("Provider = %q, want empty after deleting active provider", cfg.Provider)
	}
	if cfg.Model != "" {
		t.Errorf("Model = %q, want empty after deleting active provider", cfg.Model)
	}
	if _, exists := cfg.CustomProviders["my-gateway"]; exists {
		t.Error("my-gateway should have been deleted")
	}
	if _, exists := cfg.CustomProviders["other-gateway"]; !exists {
		t.Error("other-gateway should still exist")
	}
}

func TestUnsetInvalidKey(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"my-gateway", false},
		{"nonexistent", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := dir + "/config.json"
			cfg := &Config{
				CustomProviders: map[string]ProviderEntry{
					"my-gateway": {URL: "https://gw.example.com/v1"},
				},
			}
			if err := saveConfig(configPath, cfg); err != nil {
				t.Fatalf("saveConfig: %v", err)
			}
			err := unsetCustomProvider(configPath, tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("unsetCustomProvider(%q): err=%v, wantErr=%v", tt.name, err, tt.wantErr)
			}
		})
	}
}

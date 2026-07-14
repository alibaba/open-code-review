package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStripModelSuffix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"claude-opus-4-7[1m]", "claude-opus-4-7"},
		{"claude-sonnet-4-6[2m]", "claude-sonnet-4-6"},
		{"claude-opus-4-7[10m]", "claude-opus-4-7"},
		{"claude-opus-4-7", "claude-opus-4-7"},
		{"", ""},
		{"claude[1m]-extra", "claude[1m]-extra"},
		{"claude-opus-4-7[m]", "claude-opus-4-7[m]"},
		{"claude-opus-4-7[1M]", "claude-opus-4-7[1M]"},
		{"claude-opus-4-7[1]", "claude-opus-4-7[1]"},
	}

	for _, tt := range tests {
		got := stripModelSuffix(tt.input)
		if got != tt.want {
			t.Errorf("stripModelSuffix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveEndpoint_CCEnvStripsModelSuffix(t *testing.T) {
	t.Setenv("OCR_LLM_URL", "")
	t.Setenv("OCR_LLM_TOKEN", "")
	t.Setenv("OCR_LLM_MODEL", "")
	t.Setenv("ANTHROPIC_BASE_URL", "https://api.example.com")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "test-token")
	t.Setenv("ANTHROPIC_MODEL", "claude-opus-4-7[1m]")

	ep, err := ResolveEndpoint(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Model != "claude-opus-4-7" {
		t.Errorf("expected model %q, got %q", "claude-opus-4-7", ep.Model)
	}
	if ep.Source != "Claude Code environment" {
		t.Errorf("expected source %q, got %q", "Claude Code environment", ep.Source)
	}
}

func TestResolveEndpoint_CCEnvCleanModelUnchanged(t *testing.T) {
	t.Setenv("OCR_LLM_URL", "")
	t.Setenv("OCR_LLM_TOKEN", "")
	t.Setenv("OCR_LLM_MODEL", "")
	t.Setenv("ANTHROPIC_BASE_URL", "https://api.example.com")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "test-token")
	t.Setenv("ANTHROPIC_MODEL", "claude-opus-4-7")

	ep, err := ResolveEndpoint(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Model != "claude-opus-4-7" {
		t.Errorf("expected model %q, got %q", "claude-opus-4-7", ep.Model)
	}
}

func TestResolveEndpoint_OCREnvStripsModelSuffix(t *testing.T) {
	t.Setenv("OCR_LLM_URL", "https://api.example.com/v1/messages")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "claude-haiku[2m]")

	ep, err := ResolveEndpoint(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Model != "claude-haiku" {
		t.Errorf("expected model %q, got %q", "claude-haiku", ep.Model)
	}
	if ep.Source != "OCR environment" {
		t.Errorf("expected source %q, got %q", "OCR environment", ep.Source)
	}
}

func TestResolveEndpoint_ConfigFileStripsModelSuffix(t *testing.T) {
	t.Setenv("OCR_LLM_URL", "")
	t.Setenv("OCR_LLM_TOKEN", "")
	t.Setenv("OCR_LLM_MODEL", "")
	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_MODEL", "")

	cfg := configFile{
		Llm: llmFileConfig{
			URL:       "https://api.example.com/v1/messages",
			AuthToken: "test-token",
			Model:     "gpt-4[1m]",
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Model != "gpt-4" {
		t.Errorf("expected model %q, got %q", "gpt-4", ep.Model)
	}
	if ep.Source != "OCR config file" {
		t.Errorf("expected source %q, got %q", "OCR config file", ep.Source)
	}
}

func TestResolveEndpoint_ConfigAnthropicDefaultsToAuthorization(t *testing.T) {
	t.Setenv("OCR_LLM_URL", "")
	t.Setenv("OCR_LLM_TOKEN", "")
	t.Setenv("OCR_LLM_MODEL", "")
	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_MODEL", "")

	useAnthropic := true
	cfg := configFile{
		Llm: llmFileConfig{
			URL:          "https://api.anthropic.com",
			AuthToken:    "sk-ant-api03-test",
			Model:        "claude-opus-4-6",
			UseAnthropic: &useAnthropic,
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.AuthHeader != "authorization" {
		t.Errorf("expected auth header %q, got %q", "authorization", ep.AuthHeader)
	}
}

func TestResolveEndpoint_ConfigAuthHeaderOverrideToXAPIKey(t *testing.T) {
	t.Setenv("OCR_LLM_URL", "")
	t.Setenv("OCR_LLM_TOKEN", "")
	t.Setenv("OCR_LLM_MODEL", "")
	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_MODEL", "")

	useAnthropic := true
	cfg := configFile{
		Llm: llmFileConfig{
			URL:          "https://api.anthropic.com",
			AuthToken:    "sk-ant-api03-test",
			AuthHeader:   "x-api-key",
			Model:        "claude-opus-4-6",
			UseAnthropic: &useAnthropic,
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.AuthHeader != "x-api-key" {
		t.Errorf("expected auth header %q, got %q", "x-api-key", ep.AuthHeader)
	}
}

func TestResolveEndpoint_ConfigOpenAIIgnoresAuthHeader(t *testing.T) {
	t.Setenv("OCR_LLM_URL", "")
	t.Setenv("OCR_LLM_TOKEN", "")
	t.Setenv("OCR_LLM_MODEL", "")
	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_MODEL", "")

	useAnthropic := false
	cfg := configFile{
		Llm: llmFileConfig{
			URL:          "https://api.openai.com/v1",
			AuthToken:    "openai-token",
			AuthHeader:   "x-api-key",
			Model:        "gpt-4",
			UseAnthropic: &useAnthropic,
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != ProtocolOpenAIChatCompletions {
		t.Errorf("expected protocol %q, got %q", ProtocolOpenAIChatCompletions, ep.Protocol)
	}
	if ep.AuthHeader != "" {
		t.Errorf("expected empty auth header for OpenAI protocol, got %q", ep.AuthHeader)
	}
}

func TestResolveEndpoint_OCREnvAuthHeader(t *testing.T) {
	t.Setenv("OCR_LLM_URL", "https://api.anthropic.com")
	t.Setenv("OCR_LLM_TOKEN", "oauth-token")
	t.Setenv("OCR_LLM_MODEL", "claude-opus-4-6")
	t.Setenv("OCR_LLM_AUTH_HEADER", "authorization")

	ep, err := ResolveEndpoint(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.AuthHeader != "authorization" {
		t.Errorf("expected auth header %q, got %q", "authorization", ep.AuthHeader)
	}
}

func TestResolveEndpoint_OCREnvOpenAIIgnoresAuthHeader(t *testing.T) {
	t.Setenv("OCR_LLM_URL", "https://api.openai.com/v1")
	t.Setenv("OCR_LLM_TOKEN", "openai-token")
	t.Setenv("OCR_LLM_MODEL", "gpt-4")
	t.Setenv("OCR_LLM_AUTH_HEADER", "x-api-key")
	t.Setenv("OCR_USE_ANTHROPIC", "false")

	ep, err := ResolveEndpoint(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != ProtocolOpenAIChatCompletions {
		t.Errorf("expected protocol %q, got %q", ProtocolOpenAIChatCompletions, ep.Protocol)
	}
	if ep.AuthHeader != "" {
		t.Errorf("expected empty auth header for OpenAI protocol, got %q", ep.AuthHeader)
	}
}

// --- Provider-based resolution tests ---

func clearAllEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"OCR_LLM_URL", "OCR_LLM_TOKEN", "OCR_LLM_MODEL", "OCR_LLM_AUTH_HEADER", "OCR_USE_ANTHROPIC", "OCR_LLM_PROTOCOL",
		"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_MODEL",
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY",
	} {
		t.Setenv(k, "")
	}
}

func TestResolveEndpoint_ProviderAnthropic(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKey: "sk-ant-test", Model: "claude-sonnet-4-6"},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != "anthropic" {
		t.Errorf("Protocol = %q, want %q", ep.Protocol, "anthropic")
	}
	if ep.AuthHeader != "x-api-key" {
		t.Errorf("AuthHeader = %q, want %q", ep.AuthHeader, "x-api-key")
	}
	if ep.Token != "sk-ant-test" {
		t.Errorf("Token = %q, want %q", ep.Token, "sk-ant-test")
	}
	if ep.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want %q", ep.Model, "claude-sonnet-4-6")
	}
	if ep.Source != "provider:anthropic" {
		t.Errorf("Source = %q, want %q", ep.Source, "provider:anthropic")
	}
}

func TestResolveEndpoint_ProviderOpenAI(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "openai",
		Providers: map[string]providerEntryConfig{
			"openai": {APIKey: "sk-openai-test", Model: "gpt-4o"},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != ProtocolOpenAIChatCompletions {
		t.Errorf("Protocol = %q, want %q", ep.Protocol, ProtocolOpenAIChatCompletions)
	}
	if ep.AuthHeader != "" {
		t.Errorf("AuthHeader = %q, want empty", ep.AuthHeader)
	}
	if ep.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", ep.Model, "gpt-4o")
	}
}

func TestResolveEndpoint_ProviderModelOverride(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "anthropic",
		Model:    "claude-opus-4-6",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKey: "sk-ant-test", Model: "claude-haiku-4-5"},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Model != "claude-haiku-4-5" {
		t.Errorf("Model = %q, want %q (entry model should override top-level model)", ep.Model, "claude-haiku-4-5")
	}
}

func TestResolveEndpoint_ProviderEntryModelOverridesDefault(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKey: "sk-ant-test", Model: "claude-haiku-4-5"},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Model != "claude-haiku-4-5" {
		t.Errorf("Model = %q, want %q", ep.Model, "claude-haiku-4-5")
	}
}

func TestResolveEndpointWithModelOverride_CustomProviderWithoutConfiguredModel(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "my-gateway",
		CustomProviders: map[string]providerEntryConfig{
			"my-gateway": {
				APIKey:   "token",
				URL:      "https://gateway.internal.com/v1",
				Protocol: "openai",
				Models:   []string{"llama-3-70b", "llama-3-8b"},
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpointWithModelOverride(cfgPath, "llama-3-8b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Model != "llama-3-8b" {
		t.Errorf("Model = %q, want %q", ep.Model, "llama-3-8b")
	}
	if ep.Source != "provider:my-gateway" {
		t.Errorf("Source = %q, want %q", ep.Source, "provider:my-gateway")
	}
}

func TestResolveEndpoint_ProviderAPIKeyEnvFallback(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "env-api-key")

	cfg := configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {Model: "claude-sonnet-4-6"},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "env-api-key" {
		t.Errorf("Token = %q, want %q (should fall back to env var)", ep.Token, "env-api-key")
	}
}

func TestResolveEndpoint_ProviderMissingAPIKey(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestResolveEndpoint_ProviderNotConfigured(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider:  "anthropic",
		Providers: map[string]providerEntryConfig{},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatal("expected error for unconfigured provider")
	}
}

func TestResolveEndpoint_CustomProvider(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "my-gateway",
		CustomProviders: map[string]providerEntryConfig{
			"my-gateway": {
				APIKey:   "custom-token",
				URL:      "https://gateway.internal.com/v1",
				Protocol: "openai",
				Model:    "llama-3-70b",
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != ProtocolOpenAIChatCompletions {
		t.Errorf("Protocol = %q, want %q", ep.Protocol, ProtocolOpenAIChatCompletions)
	}
	if ep.URL != "https://gateway.internal.com/v1" {
		t.Errorf("URL = %q", ep.URL)
	}
	if ep.Model != "llama-3-70b" {
		t.Errorf("Model = %q, want %q", ep.Model, "llama-3-70b")
	}
	if ep.Source != "provider:my-gateway" {
		t.Errorf("Source = %q, want %q", ep.Source, "provider:my-gateway")
	}
}

func TestResolveEndpoint_CustomProviderInvalidProtocol(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "my-gateway",
		CustomProviders: map[string]providerEntryConfig{
			"my-gateway": {
				APIKey:   "token",
				URL:      "https://gateway.internal.com/v1",
				Protocol: "grpc",
				Model:    "some-model",
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatal("expected error for custom provider with invalid protocol")
	}
}

func TestResolveEndpoint_CustomProviderMissingFields(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "my-gateway",
		CustomProviders: map[string]providerEntryConfig{
			"my-gateway": {
				APIKey: "token",
				URL:    "https://gateway.internal.com/v1",
				// Missing protocol and model.
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatal("expected error for custom provider missing required fields")
	}
}

func TestResolveEndpoint_CustomProviderModelFromTopLevel(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "my-gateway",
		Model:    "top-level-model",
		CustomProviders: map[string]providerEntryConfig{
			"my-gateway": {
				APIKey:   "token",
				URL:      "https://gateway.internal.com/v1",
				Protocol: "openai",
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Model != "top-level-model" {
		t.Errorf("Model = %q, want %q", ep.Model, "top-level-model")
	}
}

func TestResolveEndpoint_LegacyLlmStillWorks(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Llm: llmFileConfig{
			URL:       "https://api.example.com/v1/messages",
			AuthToken: "legacy-token",
			Model:     "claude-opus-4-6",
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Source != "OCR config file" {
		t.Errorf("Source = %q, want %q", ep.Source, "OCR config file")
	}
	if ep.Token != "legacy-token" {
		t.Errorf("Token = %q, want %q", ep.Token, "legacy-token")
	}
}

func TestResolveEndpoint_ProviderAnthropicURLHasMessagesSuffix(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKey: "sk-ant-test", Model: "claude-sonnet-4-6"},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.URL != "https://api.anthropic.com/v1/messages" {
		t.Errorf("URL = %q, want %q", ep.URL, "https://api.anthropic.com/v1/messages")
	}
}

func TestResolveEndpoint_ProviderExtraBody(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {
				APIKey:    "sk-ant-test",
				Model:     "claude-sonnet-4-6",
				ExtraBody: map[string]any{"thinking": map[string]any{"type": "disabled"}},
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.ExtraBody == nil {
		t.Fatal("ExtraBody should not be nil")
	}
	if _, ok := ep.ExtraBody["thinking"]; !ok {
		t.Error("ExtraBody missing 'thinking' key")
	}
}

func TestResolveEndpointWithModelOverride_ValidModelInPresetList(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKey: "sk-ant-test", Model: "claude-sonnet-4-6"},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpointWithModelOverride(cfgPath, "claude-opus-4-8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Model != "claude-opus-4-8" {
		t.Errorf("Model = %q, want %q", ep.Model, "claude-opus-4-8")
	}
}

func TestResolveEndpointWithModelOverride_InvalidModelInPresetList(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKey: "sk-ant-test", Model: "claude-sonnet-4-6"},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := ResolveEndpointWithModelOverride(cfgPath, "claude-opsu-4-6")
	if err == nil {
		t.Fatal("expected error for invalid model override")
	}
	if !strings.Contains(err.Error(), "not available for provider") {
		t.Errorf("error message should mention model unavailability, got: %v", err)
	}
	if !strings.Contains(err.Error(), "available models:") {
		t.Errorf("error message should list available models, got: %v", err)
	}
}

func TestResolveEndpointWithModelOverride_ValidModelInCustomProviderList(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "my-gateway",
		CustomProviders: map[string]providerEntryConfig{
			"my-gateway": {
				APIKey:   "token",
				URL:      "https://gateway.internal.com/v1",
				Protocol: "openai",
				Models:   []string{"llama-3-70b", "llama-3-8b"},
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpointWithModelOverride(cfgPath, "llama-3-8b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Model != "llama-3-8b" {
		t.Errorf("Model = %q, want %q", ep.Model, "llama-3-8b")
	}
}

func TestResolveEndpointWithModelOverride_InvalidModelInCustomProviderList(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "my-gateway",
		CustomProviders: map[string]providerEntryConfig{
			"my-gateway": {
				APIKey:   "token",
				URL:      "https://gateway.internal.com/v1",
				Protocol: "openai",
				Models:   []string{"llama-3-70b", "llama-3-8b"},
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := ResolveEndpointWithModelOverride(cfgPath, "gpt-4")
	if err == nil {
		t.Fatal("expected error for invalid model override in custom provider")
	}
	if !strings.Contains(err.Error(), "not available for provider") {
		t.Errorf("error message should mention model unavailability, got: %v", err)
	}
}

func TestResolveEndpointWithModelOverride_NoValidationWhenNoModelList(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "my-gateway",
		CustomProviders: map[string]providerEntryConfig{
			"my-gateway": {
				APIKey:   "token",
				URL:      "https://gateway.internal.com/v1",
				Protocol: "openai",
				// No Models list, so any model override should be accepted.
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpointWithModelOverride(cfgPath, "any-model-name")
	if err != nil {
		t.Fatalf("unexpected error when no model list exists: %v", err)
	}
	if ep.Model != "any-model-name" {
		t.Errorf("Model = %q, want %q", ep.Model, "any-model-name")
	}
}

func TestResolveEndpointWithModelOverride_MergesPresetAndEntryModels(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {
				APIKey: "sk-ant-test",
				Models: []string{"custom-model-1", "custom-model-2"},
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Should accept both preset models and entry models.
	ep1, err := ResolveEndpointWithModelOverride(cfgPath, "claude-opus-4-8")
	if err != nil {
		t.Fatalf("unexpected error for preset model: %v", err)
	}
	if ep1.Model != "claude-opus-4-8" {
		t.Errorf("Model = %q, want %q", ep1.Model, "claude-opus-4-8")
	}

	ep2, err := ResolveEndpointWithModelOverride(cfgPath, "custom-model-1")
	if err != nil {
		t.Fatalf("unexpected error for entry model: %v", err)
	}
	if ep2.Model != "custom-model-1" {
		t.Errorf("Model = %q, want %q", ep2.Model, "custom-model-1")
	}

	// Should reject models not in either list.
	_, err = ResolveEndpointWithModelOverride(cfgPath, "invalid-model")
	if err == nil {
		t.Fatal("expected error for model not in preset or entry lists")
	}
}

func TestResolveEndpointWithModelOverride_LegacyConfigNoValidation(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Llm: llmFileConfig{
			URL:       "https://api.example.com/v1/messages",
			AuthToken: "legacy-token",
			Model:     "configured-model",
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Legacy config has no model list, so any override should be accepted.
	ep, err := ResolveEndpointWithModelOverride(cfgPath, "any-override-model")
	if err != nil {
		t.Fatalf("unexpected error for legacy config override: %v", err)
	}
	if ep.Model != "any-override-model" {
		t.Errorf("Model = %q, want %q", ep.Model, "any-override-model")
	}
}

func TestParseExtraHeaders(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "empty string returns nil",
			input: "",
			want:  nil,
		},
		{
			name:  "single pair",
			input: "X-Custom-Header=value1",
			want:  map[string]string{"X-Custom-Header": "value1"},
		},
		{
			name:  "multiple pairs",
			input: "X-Custom-Header=value1,X-Another=value2",
			want: map[string]string{
				"X-Custom-Header": "value1",
				"X-Another":       "value2",
			},
		},
		{
			name:  "pairs with whitespace are trimmed",
			input: "  X-Header  =  spaced-value  , X-Two = val ",
			want: map[string]string{
				"X-Header": "spaced-value",
				"X-Two":    "val",
			},
		},
		{
			name:  "trailing comma is ignored",
			input: "X-Header=value,",
			want:  map[string]string{"X-Header": "value"},
		},
		{
			name:  "empty pairs between commas are skipped",
			input: "X-Header=value,, ,X-Two=val2",
			want: map[string]string{
				"X-Header": "value",
				"X-Two":    "val2",
			},
		},
		{
			name:  "value can contain equals sign",
			input: "X-Header=a=b=c",
			want:  map[string]string{"X-Header": "a=b=c"},
		},
		{
			name:    "pair without equals sign is error",
			input:   "X-Header-no-equals",
			wantErr: true,
		},
		{
			name:    "empty key is error",
			input:   "=value",
			wantErr: true,
		},
		{
			name:    "empty key with whitespace is error",
			input:   "  =value",
			wantErr: true,
		},
		{
			name:    "reserved header authorization is rejected",
			input:   "Authorization=Bearer token",
			wantErr: true,
		},
		{
			name:    "reserved header x-api-key is rejected",
			input:   "x-api-key=secret",
			wantErr: true,
		},
		{
			name:    "reserved header content-type is rejected",
			input:   "Content-Type=text/plain",
			wantErr: true,
		},
		{
			name:    "reserved header user-agent is rejected",
			input:   "User-Agent=custom-agent",
			wantErr: true,
		},
		{
			name:    "reserved header rejected even when mixed with valid ones",
			input:   "X-Org=val,Authorization=bad",
			wantErr: true,
		},
		{
			name:  "quoted value with comma",
			input: `X-Forwarded-For="1.2.3.4,5.6.7.8"`,
			want:  map[string]string{"X-Forwarded-For": "1.2.3.4,5.6.7.8"},
		},
		{
			name:  "quoted value with comma followed by another pair",
			input: `X-Forwarded-For="1.2.3.4,5.6.7.8",X-Org=abc`,
			want: map[string]string{
				"X-Forwarded-For": "1.2.3.4,5.6.7.8",
				"X-Org":           "abc",
			},
		},
		{
			name:  "multiple quoted values with commas",
			input: `X-A="1,2",X-B="3,4"`,
			want: map[string]string{
				"X-A": "1,2",
				"X-B": "3,4",
			},
		},
		{
			name:  "quoted empty value",
			input: `X-Key=""`,
			want:  map[string]string{"X-Key": ""},
		},
		{
			name:  "quoted value preserves inner whitespace",
			input: `X-Key=" spaced "`,
			want:  map[string]string{"X-Key": " spaced "},
		},
		{
			name:  "mix of quoted and unquoted values",
			input: `X-A=plain,X-B="has,comma"`,
			want: map[string]string{
				"X-A": "plain",
				"X-B": "has,comma",
			},
		},
		{
			name:    "unclosed quote is error",
			input:   `X-Key="unterminated`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseExtraHeaders(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !mapsEqual(got, tt.want) {
				t.Errorf("ParseExtraHeaders(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func TestResolveEndpoint_OCREnvExtraHeaders(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OCR_LLM_URL", "https://api.example.com/v1/messages")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "claude-opus-4-6")
	t.Setenv("OCR_LLM_EXTRA_HEADERS", "X-Custom-Header=my-value,X-Another=second")

	ep, err := ResolveEndpoint(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.ExtraHeaders == nil {
		t.Fatal("ExtraHeaders should not be nil")
	}
	if v, ok := ep.ExtraHeaders["X-Custom-Header"]; !ok || v != "my-value" {
		t.Errorf("ExtraHeaders[\"X-Custom-Header\"] = %q, want %q", v, "my-value")
	}
	if v, ok := ep.ExtraHeaders["X-Another"]; !ok || v != "second" {
		t.Errorf("ExtraHeaders[\"X-Another\"] = %q, want %q", v, "second")
	}
}

func TestResolveEndpoint_OCREnvExtraHeadersEmpty(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OCR_LLM_URL", "https://api.example.com/v1/messages")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "claude-opus-4-6")

	ep, err := ResolveEndpoint(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ep.ExtraHeaders) != 0 {
		t.Errorf("ExtraHeaders should be empty, got %v", ep.ExtraHeaders)
	}
}

func TestResolveEndpoint_OCREnvExtraHeadersInvalid(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OCR_LLM_URL", "https://api.example.com/v1/messages")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "claude-opus-4-6")
	t.Setenv("OCR_LLM_EXTRA_HEADERS", "no-equals-sign")

	_, err := ResolveEndpoint(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err == nil {
		t.Fatal("expected error for invalid extra headers")
	}
	if !strings.Contains(err.Error(), "extra header") {
		t.Errorf("error should mention extra header, got: %v", err)
	}
}

func TestResolveEndpoint_OCREnvExtraHeadersReservedRejected(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OCR_LLM_URL", "https://api.example.com/v1/messages")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "claude-opus-4-6")
	t.Setenv("OCR_LLM_EXTRA_HEADERS", "Authorization=oops")

	_, err := ResolveEndpoint(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err == nil {
		t.Fatal("expected error for reserved extra header")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Errorf("error should mention reserved header, got: %v", err)
	}
}

func TestResolveEndpoint_ProviderExtraHeaders(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {
				APIKey:       "sk-ant-test",
				Model:        "claude-sonnet-4-6",
				ExtraHeaders: map[string]string{"X-Org-ID": "org-123"},
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.ExtraHeaders == nil {
		t.Fatal("ExtraHeaders should not be nil")
	}
	if v, ok := ep.ExtraHeaders["X-Org-ID"]; !ok || v != "org-123" {
		t.Errorf("ExtraHeaders[\"X-Org-ID\"] = %q, want %q", v, "org-123")
	}
}

func TestResolveEndpoint_EnvExtraHeadersMergedWithConfigFile(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OCR_LLM_EXTRA_HEADERS", "EagleEye-TraceId=trace-from-env,X-New=from-env")

	cfg := configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {
				APIKey:       "sk-ant-test",
				Model:        "claude-sonnet-4-6",
				ExtraHeaders: map[string]string{"X-Org-ID": "org-123"},
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := ep.ExtraHeaders["X-Org-ID"]; !ok || v != "org-123" {
		t.Errorf("config file header preserved: ExtraHeaders[\"X-Org-ID\"] = %q, want %q", v, "org-123")
	}
	if v, ok := ep.ExtraHeaders["EagleEye-TraceId"]; !ok || v != "trace-from-env" {
		t.Errorf("env header merged: ExtraHeaders[\"EagleEye-TraceId\"] = %q, want %q", v, "trace-from-env")
	}
	if v, ok := ep.ExtraHeaders["X-New"]; !ok || v != "from-env" {
		t.Errorf("env header merged: ExtraHeaders[\"X-New\"] = %q, want %q", v, "from-env")
	}
}

func TestResolveEndpoint_LegacyLlmExtraHeaders(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Llm: llmFileConfig{
			URL:          "https://api.example.com/v1/messages",
			AuthToken:    "legacy-token",
			Model:        "claude-opus-4-6",
			ExtraHeaders: map[string]string{"X-Legacy": "yes"},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := ep.ExtraHeaders["X-Legacy"]; !ok || v != "yes" {
		t.Errorf("ExtraHeaders[\"X-Legacy\"] = %q, want %q", v, "yes")
	}
}

func TestParseTimeoutEnv(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    time.Duration
		wantOK  bool
		wantErr bool
	}{
		{"empty", "", 0, false, false},
		{"valid 120", "120", 120 * time.Second, true, false},
		{"valid 60", "60", 60 * time.Second, true, false},
		{"zero", "0", 0, true, false},
		{"negative", "-5", 0, false, true},
		{"non-integer", "abc", 0, false, true},
		{"with spaces", " 90 ", 90 * time.Second, true, false},
		{"overflow", "99999999999999", 0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OCR_LLM_TIMEOUT", tt.value)
			got, ok, err := parseTimeoutEnv()
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTimeoutEnv() error = %v, wantErr %v", err, tt.wantErr)
			}
			if ok != tt.wantOK {
				t.Errorf("parseTimeoutEnv() ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("parseTimeoutEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateTimeoutSec(t *testing.T) {
	tests := []struct {
		name    string
		input   int
		want    time.Duration
		wantErr bool
	}{
		{"zero", 0, 0, false},
		{"positive 60", 60, 60 * time.Second, false},
		{"positive 300", 300, 300 * time.Second, false},
		{"negative -1", -1, 0, true},
		{"negative -100", -100, 0, true},
		{"max safe", 9223372036, 9223372036 * time.Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateTimeoutSec(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTimeoutSec(%d) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("validateTimeoutSec(%d) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveEndpoint_EnvTimeoutGlobalOverride(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OCR_LLM_URL", "https://api.example.com/v1")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "mimo-v2.5-pro")
	t.Setenv("OCR_USE_ANTHROPIC", "false")
	t.Setenv("OCR_LLM_TIMEOUT", "90")

	ep, err := ResolveEndpoint(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Timeout != 90*time.Second {
		t.Errorf("Timeout = %v, want %v", ep.Timeout, 90*time.Second)
	}
}

func TestResolveEndpoint_ConfigTimeoutSec(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Llm: llmFileConfig{
			URL:        "https://api.example.com/v1/messages",
			AuthToken:  "test-token",
			Model:      "claude-opus-4-6",
			TimeoutSec: 120,
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Timeout != 120*time.Second {
		t.Errorf("Timeout = %v, want %v", ep.Timeout, 120*time.Second)
	}
}

func TestResolveEndpoint_NegativeConfigTimeoutSec(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Llm: llmFileConfig{
			URL:        "https://api.example.com/v1/messages",
			AuthToken:  "test-token",
			Model:      "claude-opus-4-6",
			TimeoutSec: -5,
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatal("expected error for negative timeout_sec, got nil")
	}
	if !strings.Contains(err.Error(), "non-negative") {
		t.Errorf("error %q should mention non-negative", err.Error())
	}
}

func TestNewLLMClient_TimeoutForwarded(t *testing.T) {
	ep := ResolvedEndpoint{
		URL:     "https://api.example.com/v1",
		Token:   "test-token",
		Model:   "test-model",
		Timeout: 2 * time.Minute,
	}

	client := NewLLMClient(ep)
	if client == nil {
		t.Fatal("NewLLMClient returned nil")
	}

	// Verify the client was created (we can't easily inspect the internal timeout,
	// but we can verify the client is functional and was constructed without error).
	if oc, ok := client.(*OpenAIClient); ok {
		if oc.cfg.Timeout != 2*time.Minute {
			t.Errorf("OpenAIClient cfg.Timeout = %v, want %v", oc.cfg.Timeout, 2*time.Minute)
		}
	} else {
		t.Errorf("expected *OpenAIClient, got %T", client)
	}
}

func TestNewLLMClient_DefaultTimeout(t *testing.T) {
	ep := ResolvedEndpoint{
		URL:   "https://api.example.com/v1",
		Token: "test-token",
		Model: "test-model",
		// Timeout not set — should default to 5 minutes
	}

	client := NewLLMClient(ep)
	if oc, ok := client.(*OpenAIClient); ok {
		if oc.cfg.Timeout != 5*time.Minute {
			t.Errorf("OpenAIClient cfg.Timeout = %v, want default %v", oc.cfg.Timeout, 5*time.Minute)
		}
	}
}

func TestResolveEndpoint_ProviderConfigTimeoutSec(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {
				APIKey:     "sk-ant-test",
				Model:      "claude-sonnet-4-6",
				TimeoutSec: 180,
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Timeout != 180*time.Second {
		t.Errorf("Timeout = %v, want %v", ep.Timeout, 180*time.Second)
	}
}

func TestResolveEndpoint_ProviderConfigNegativeTimeoutSec(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {
				APIKey:     "sk-ant-test",
				Model:      "claude-sonnet-4-6",
				TimeoutSec: -10,
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatal("expected error for negative provider timeout_sec")
	}
	if !strings.Contains(err.Error(), "non-negative") {
		t.Errorf("error %q should mention non-negative", err.Error())
	}
}

func TestResolveEndpoint_EnvTimeoutOverridesConfigTimeout(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OCR_LLM_TIMEOUT", "60")

	cfg := configFile{
		Llm: llmFileConfig{
			URL:        "https://api.example.com/v1/messages",
			AuthToken:  "test-token",
			Model:      "claude-opus-4-6",
			TimeoutSec: 120,
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// OCR_LLM_TIMEOUT (60s) should override config timeout_sec (120s)
	if ep.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want %v (env should override config)", ep.Timeout, 60*time.Second)
	}
}

func TestResolveEndpoint_EnvTimeoutOverridesProviderTimeout(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OCR_LLM_TIMEOUT", "45")

	cfg := configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {
				APIKey:     "sk-ant-test",
				Model:      "claude-sonnet-4-6",
				TimeoutSec: 300,
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// OCR_LLM_TIMEOUT (45s) should override provider timeout_sec (300s)
	if ep.Timeout != 45*time.Second {
		t.Errorf("Timeout = %v, want %v (env should override provider config)", ep.Timeout, 45*time.Second)
	}
}

func TestResolveEndpoint_InvalidEnvTimeoutWithConfig(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OCR_LLM_TIMEOUT", "invalid")

	cfg := configFile{
		Llm: llmFileConfig{
			URL:       "https://api.example.com/v1/messages",
			AuthToken: "test-token",
			Model:     "claude-opus-4-6",
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid OCR_LLM_TIMEOUT")
	}
	if !strings.Contains(err.Error(), "OCR_LLM_TIMEOUT") {
		t.Errorf("error %q should mention OCR_LLM_TIMEOUT", err.Error())
	}
}

func TestResolveEndpoint_NegativeEnvTimeoutWithConfig(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OCR_LLM_TIMEOUT", "-30")

	cfg := configFile{
		Llm: llmFileConfig{
			URL:       "https://api.example.com/v1/messages",
			AuthToken: "test-token",
			Model:     "claude-opus-4-6",
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatal("expected error for negative OCR_LLM_TIMEOUT")
	}
	if !strings.Contains(err.Error(), "OCR_LLM_TIMEOUT") {
		t.Errorf("error %q should mention OCR_LLM_TIMEOUT", err.Error())
	}
}

// --- openai-responses / protocol normalization tests ---

func TestResolveEndpoint_CustomProviderResponsesProtocol(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "my-gateway",
		CustomProviders: map[string]providerEntryConfig{
			"my-gateway": {
				APIKey:   "token",
				URL:      "https://gateway.internal.com/v1",
				Protocol: ProtocolOpenAIResponses,
				Model:    "gpt-5.4",
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfgPath, data, 0644)

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != ProtocolOpenAIResponses {
		t.Errorf("Protocol = %q, want %q", ep.Protocol, ProtocolOpenAIResponses)
	}
	// Resolver must NOT append /v1/messages to a non-anthropic URL.
	if strings.Contains(ep.URL, "/v1/messages") {
		t.Errorf("URL = %q, must not have /v1/messages appended for openai-responses", ep.URL)
	}
	if strings.Contains(ep.URL, "/chat/completions") {
		t.Errorf("URL = %q, must not have /chat/completions appended (client handles its own suffix)", ep.URL)
	}
}

func TestResolveEndpoint_CustomProviderChatCompletionsProtocol(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "my-gateway",
		CustomProviders: map[string]providerEntryConfig{
			"my-gateway": {
				APIKey:   "token",
				URL:      "https://gateway.internal.com/v1",
				Protocol: ProtocolOpenAIChatCompletions,
				Model:    "gpt-4o",
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfgPath, data, 0644)

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != ProtocolOpenAIChatCompletions {
		t.Errorf("Protocol = %q, want %q", ep.Protocol, ProtocolOpenAIChatCompletions)
	}
}

func TestResolveEndpoint_CustomProviderOpenAIAlias(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "my-gateway",
		CustomProviders: map[string]providerEntryConfig{
			"my-gateway": {
				APIKey:   "token",
				URL:      "https://gateway.internal.com/v1",
				Protocol: "openai", // legacy alias
				Model:    "gpt-4o",
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfgPath, data, 0644)

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != ProtocolOpenAIChatCompletions {
		t.Errorf("Protocol = %q, want %q (alias should be normalized)", ep.Protocol, ProtocolOpenAIChatCompletions)
	}
}

func TestResolveEndpoint_CustomProviderAnthropicVertexRejected(t *testing.T) {
	clearAllEnv(t)

	cfg := configFile{
		Provider: "my-gateway",
		CustomProviders: map[string]providerEntryConfig{
			"my-gateway": {
				APIKey:   "token",
				URL:      "https://gateway.internal.com/v1",
				Protocol: "anthropic-vertex",
				Model:    "claude-3",
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfgPath, data, 0644)

	_, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatal("expected error for anthropic-vertex (unsupported)")
	}
	if !strings.Contains(err.Error(), "unsupported protocol") {
		t.Errorf("error %q should mention 'unsupported protocol'", err.Error())
	}
}

func TestResolveEndpoint_LegacyLlmProtocolTakesPriorityOverUseAnthropic(t *testing.T) {
	clearAllEnv(t)

	// llm.protocol says responses, use_anthropic says anthropic. protocol wins.
	useAnthropic := true
	cfg := configFile{
		Llm: llmFileConfig{
			URL:          "https://api.openai.com/v1",
			AuthToken:    "tok",
			Model:        "gpt-5.4",
			Protocol:     ProtocolOpenAIResponses,
			UseAnthropic: &useAnthropic,
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfgPath, data, 0644)

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != ProtocolOpenAIResponses {
		t.Errorf("Protocol = %q, want %q (llm.protocol should win over use_anthropic)", ep.Protocol, ProtocolOpenAIResponses)
	}
}

func TestResolveEndpoint_OCREnvProtocolTakesPriorityOverUseAnthropic(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OCR_LLM_URL", "https://api.openai.com/v1")
	t.Setenv("OCR_LLM_TOKEN", "tok")
	t.Setenv("OCR_LLM_MODEL", "gpt-5.4")
	t.Setenv("OCR_USE_ANTHROPIC", "true")
	t.Setenv("OCR_LLM_PROTOCOL", ProtocolOpenAIResponses)

	ep, err := ResolveEndpoint(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != ProtocolOpenAIResponses {
		t.Errorf("Protocol = %q, want %q (OCR_LLM_PROTOCOL should win over OCR_USE_ANTHROPIC)", ep.Protocol, ProtocolOpenAIResponses)
	}
}

func TestResolveEndpoint_OCREnvProtocolAlias(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OCR_LLM_URL", "https://api.openai.com/v1")
	t.Setenv("OCR_LLM_TOKEN", "tok")
	t.Setenv("OCR_LLM_MODEL", "gpt-4o")
	t.Setenv("OCR_LLM_PROTOCOL", "openai") // alias

	ep, err := ResolveEndpoint(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != ProtocolOpenAIChatCompletions {
		t.Errorf("Protocol = %q, want %q (alias should be normalized)", ep.Protocol, ProtocolOpenAIChatCompletions)
	}
}

func TestResolveEndpoint_OCREnvProtocolInvalid(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OCR_LLM_URL", "https://api.openai.com/v1")
	t.Setenv("OCR_LLM_TOKEN", "tok")
	t.Setenv("OCR_LLM_MODEL", "gpt-4o")
	t.Setenv("OCR_LLM_PROTOCOL", "grpc")

	_, err := ResolveEndpoint(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err == nil {
		t.Fatal("expected error for invalid OCR_LLM_PROTOCOL")
	}
	if !strings.Contains(err.Error(), "unsupported protocol") {
		t.Errorf("error %q should mention unsupported protocol", err.Error())
	}
}

// TestResolveEndpoint_ResponsesURLNotMutated makes sure the resolver leaves
// openai-responses URLs alone — only anthropic gets /v1/messages appended.
func TestResolveEndpoint_ResponsesURLNotMutated(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("OCR_LLM_URL", "https://api.openai.com/v1")
	t.Setenv("OCR_LLM_TOKEN", "tok")
	t.Setenv("OCR_LLM_MODEL", "gpt-5.4")
	t.Setenv("OCR_LLM_PROTOCOL", ProtocolOpenAIResponses)

	ep, err := ResolveEndpoint(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.URL != "https://api.openai.com/v1" {
		t.Errorf("URL = %q, want %q (resolver must not mutate openai-responses URLs)", ep.URL, "https://api.openai.com/v1")
	}
}

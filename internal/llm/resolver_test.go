package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != "openai" {
		t.Errorf("expected protocol %q, got %q", "openai", ep.Protocol)
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
	if ep.Protocol != "openai" {
		t.Errorf("expected protocol %q, got %q", "openai", ep.Protocol)
	}
	if ep.AuthHeader != "" {
		t.Errorf("expected empty auth header for OpenAI protocol, got %q", ep.AuthHeader)
	}
}

// --- Provider-based resolution tests ---

func clearAllEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"OCR_LLM_URL", "OCR_LLM_TOKEN", "OCR_LLM_MODEL", "OCR_LLM_AUTH_HEADER", "OCR_USE_ANTHROPIC",
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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != "openai" {
		t.Errorf("Protocol = %q, want %q", ep.Protocol, "openai")
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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Protocol != "openai" {
		t.Errorf("Protocol = %q, want %q", ep.Protocol, "openai")
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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

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
	os.WriteFile(cfgPath, data, 0644)

	// Legacy config has no model list, so any override should be accepted.
	ep, err := ResolveEndpointWithModelOverride(cfgPath, "any-override-model")
	if err != nil {
		t.Fatalf("unexpected error for legacy config override: %v", err)
	}
	if ep.Model != "any-override-model" {
		t.Errorf("Model = %q, want %q", ep.Model, "any-override-model")
	}
}

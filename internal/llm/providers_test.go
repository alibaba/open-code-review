// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"sort"
	"strings"
	"testing"
)

func TestLookupProvider_KnownProviders(t *testing.T) {
	names := []string{"anthropic", "openai", "openrouter", "dashscope", "edenai", "chatgpt"}
	for _, name := range names {
		p, ok := LookupProvider(name)
		if !ok {
			t.Errorf("LookupProvider(%q) returned false, want true", name)
			continue
		}
		if p.Name != name {
			t.Errorf("LookupProvider(%q).Name = %q", name, p.Name)
		}
		if p.Protocol == "" {
			t.Errorf("LookupProvider(%q).Protocol is empty", name)
		}
		if p.BaseURL == "" {
			t.Errorf("LookupProvider(%q).BaseURL is empty", name)
		}
		if len(p.Models) == 0 && name != "chatgpt" {
			t.Errorf("LookupProvider(%q).Models is empty", name)
		}
	}
}

func TestLookupProvider_ChatGPTDetails(t *testing.T) {
	if _, ok := LookupProvider("codex"); ok {
		t.Fatal("undocumented Codex preset must not ship")
	}
	p, ok := LookupProvider("chatgpt")
	if !ok {
		t.Fatal("chatgpt preset not found")
	}
	if p.Protocol != ProtocolOpenAIResponses || p.BaseURL != "https://api.openai.com/v1" || p.EnvVar != "" || !p.ExternalAuth || !p.RequiresStreaming || !p.RejectsSamplingParams || len(p.Models) != 0 {
		t.Fatalf("unexpected chatgpt preset: %+v", p)
	}
}

func TestLookupProvider_MiniMaxDetails(t *testing.T) {
	tests := []struct {
		name, wantURL, wantEnvVar string
	}{
		{
			name:       "minimax",
			wantURL:    "https://api.minimax.io/v1",
			wantEnvVar: "MINIMAX_GLOBAL_API_KEY",
		},
		{
			name:       "minimax-cn",
			wantURL:    "https://api.minimaxi.com/v1",
			wantEnvVar: "MINIMAX_API_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, ok := LookupProvider(tt.name)
			if !ok {
				t.Fatalf("LookupProvider(%q) returned false, want true", tt.name)
			}
			if len(provider.Models) != 2 || provider.Models[0] != "MiniMax-M3.1-Flash" || provider.Models[1] != "MiniMax-M3" {
				t.Errorf("LookupProvider(%q).Models = %v, want [MiniMax-M3.1-Flash MiniMax-M3]", tt.name, provider.Models)
			}
			if provider.BaseURL != tt.wantURL {
				t.Errorf("LookupProvider(%q).BaseURL = %q, want %q", tt.name, provider.BaseURL, tt.wantURL)
			}
			if provider.EnvVar != tt.wantEnvVar {
				t.Errorf("LookupProvider(%q).EnvVar = %q, want %q", tt.name, provider.EnvVar, tt.wantEnvVar)
			}
		})
	}
}

func TestMiniMaxSuggestedModels(t *testing.T) {
	for _, provider := range ListProviders() {
		for _, model := range provider.Models {
			if !strings.HasPrefix(strings.ToLower(model), "minimax-") {
				continue
			}
			if model != "MiniMax-M3.1-Flash" && model != "MiniMax-M3" && model != "minimax-m3.1-flash" {
				t.Errorf("provider %q suggests legacy MiniMax model %q", provider.Name, model)
			}
		}
	}
	for _, name := range []string{"tencent-tokenhub", "ollama-cloud"} {
		provider, _ := LookupProvider(name)
		if !ModelListContains(provider.Models, "minimax-m3.1-flash") {
			t.Errorf("provider %q is missing its M3.1 Flash route", name)
		}
	}
}

func TestLookupProvider_Unknown(t *testing.T) {
	_, ok := LookupProvider("nonexistent-provider")
	if ok {
		t.Error("LookupProvider(nonexistent) returned true, want false")
	}
}

func TestListProviders_Order(t *testing.T) {
	providers := ListProviders()
	if len(providers) < 3 {
		t.Fatalf("expected at least 3 providers, got %d", len(providers))
	}
	expected := []string{"anthropic", "baidu-qianfan", "bedrock", "chatgpt", "dashscope", "dashscope-tokenplan", "deepseek", "edenai", "gemini", "hy-tokenplan", "iflytek", "kimi", "kimi-global", "litellm", "mimo", "minimax", "minimax-cn", "mistral", "novita", "ollama-cloud", "openai", "openai-responses", "openrouter", "siliconflow", "siliconflow-cn", "tencent-tokenhub", "volcengine", "xai", "z-ai", "z-ai-coding"}
	if len(providers) != len(expected) {
		t.Fatalf("expected %d providers, got %d", len(expected), len(providers))
	}
	for i, name := range expected {
		if providers[i].Name != name {
			t.Errorf("providers[%d].Name = %q, want %q", i, providers[i].Name, name)
		}
	}
}

func TestListProviders_ReturnsCopy(t *testing.T) {
	p1 := ListProviders()
	p1[0].Name = "mutated"

	p2 := ListProviders()
	if p2[0].Name == "mutated" {
		t.Error("ListProviders returns a reference to the registry, should return a copy")
	}
}

func TestLookupProvider_ReturnsCopyOfModels(t *testing.T) {
	p1, _ := LookupProvider("anthropic")
	p1.Models[0] = "mutated"

	p2, _ := LookupProvider("anthropic")
	if p2.Models[0] == "mutated" {
		t.Error("LookupProvider returns a reference to Models slice, should return a copy")
	}
}

func TestLookupProvider_PreservesModelOrder(t *testing.T) {
	p, ok := LookupProvider("anthropic")
	if !ok {
		t.Fatal("anthropic not found")
	}
	expected := []string{
		"claude-fable-5-2",
		"claude-opus-5-5",
		"claude-sonnet-5-5",
		"claude-haiku-5-5",
	}
	if len(p.Models) != len(expected) {
		t.Fatalf("expected %d models, got %d", len(expected), len(p.Models))
	}
	for i, model := range expected {
		if p.Models[i] != model {
			t.Errorf("Models[%d] = %q, want %q", i, p.Models[i], model)
		}
	}
}

func TestListProviders_ReturnsSortedProviders(t *testing.T) {
	providers := ListProviders()
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("providers are not sorted: %v", names)
	}
}

func TestLookupProvider_AnthropicDetails(t *testing.T) {
	p, ok := LookupProvider("anthropic")
	if !ok {
		t.Fatal("anthropic not found")
	}
	if p.Protocol != "anthropic" {
		t.Errorf("Protocol = %q, want %q", p.Protocol, "anthropic")
	}
	if p.AuthHeader != "x-api-key" {
		t.Errorf("AuthHeader = %q, want %q", p.AuthHeader, "x-api-key")
	}
	if p.EnvVar != "ANTHROPIC_API_KEY" {
		t.Errorf("EnvVar = %q, want %q", p.EnvVar, "ANTHROPIC_API_KEY")
	}
}

func TestLookupProvider_OpenAIDetails(t *testing.T) {
	p, ok := LookupProvider("openai")
	if !ok {
		t.Fatal("openai not found")
	}
	if p.Protocol != ProtocolOpenAIChatCompletions {
		t.Errorf("Protocol = %q, want %q", p.Protocol, ProtocolOpenAIChatCompletions)
	}
	if p.AuthHeader != "" {
		t.Errorf("AuthHeader = %q, want empty", p.AuthHeader)
	}
	expectedModels := []string{
		"gpt-5.5",
	}
	if len(p.Models) != len(expectedModels) {
		t.Fatalf("Models length = %d, want %d", len(p.Models), len(expectedModels))
	}
	for i, model := range expectedModels {
		if p.Models[i] != model {
			t.Errorf("Models[%d] = %q, want %q", i, p.Models[i], model)
		}
	}
}

func TestLookupProvider_OpenRouterDetails(t *testing.T) {
	p, ok := LookupProvider("openrouter")
	if !ok {
		t.Fatal("openrouter not found")
	}
	if p.Protocol != ProtocolOpenAIChatCompletions {
		t.Errorf("Protocol = %q, want %q", p.Protocol, ProtocolOpenAIChatCompletions)
	}
	if p.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL, "https://openrouter.ai/api/v1")
	}
	if p.EnvVar != "OPENROUTER_API_KEY" {
		t.Errorf("EnvVar = %q, want %q", p.EnvVar, "OPENROUTER_API_KEY")
	}
}

func TestLookupProvider_DeepSeekFlash(t *testing.T) {
	p, ok := LookupProvider("deepseek")
	if !ok {
		t.Fatal("deepseek not found")
	}
	if !ModelListContains(p.Models, "deepseek-flash") {
		t.Error(`deepseek models do not contain "deepseek-flash"`)
	}
}

func TestLookupProvider_OpenAIResponsesDetails(t *testing.T) {
	const expectedEnvVar = "OPENAI_RESPONSES_API_KEY"

	p, ok := LookupProvider("openai-responses")
	if !ok {
		t.Fatal("openai-responses not found")
	}
	if p.Protocol != ProtocolOpenAIResponses {
		t.Errorf("Protocol = %q, want %q", p.Protocol, ProtocolOpenAIResponses)
	}
	if p.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL, "https://api.openai.com/v1")
	}
	if p.EnvVar != expectedEnvVar {
		t.Errorf("EnvVar = %q, want %q", p.EnvVar, expectedEnvVar)
	}
	if p.AuthHeader != "" {
		t.Errorf("AuthHeader = %q, want empty (OpenAI-compatible uses Bearer by default)", p.AuthHeader)
	}
	expectedModels := []string{
		"gpt-6.2-astra",
		"gpt-6.2-sol",
		"gpt-6.1-sol",
		"gpt-6.1-terra",
		"gpt-6-luna",
	}
	if len(p.Models) != len(expectedModels) {
		t.Fatalf("Models length = %d, want %d", len(p.Models), len(expectedModels))
	}
	for i, model := range expectedModels {
		if p.Models[i] != model {
			t.Errorf("Models[%d] = %q, want %q", i, p.Models[i], model)
		}
	}
}

func TestOpenAIToolCallingModelsUseResponses(t *testing.T) {
	for _, provider := range ListProviders() {
		if provider.BaseURL != "https://api.openai.com/v1" || provider.Protocol != ProtocolOpenAIChatCompletions {
			continue
		}
		for _, model := range provider.Models {
			if strings.HasPrefix(model, "gpt-6") || strings.HasPrefix(model, "gpt-5.6") {
				t.Errorf("provider %q suggests %q over Chat Completions, but OCR tool calls require Responses", provider.Name, model)
			}
		}
	}
}

func TestProviderSuggestionsStayShort(t *testing.T) {
	for _, provider := range ListProviders() {
		if len(provider.Models) > 6 {
			t.Errorf("provider %q suggests %d models, want at most 6", provider.Name, len(provider.Models))
		}
	}
}

func TestLookupProvider_OllamaCloudDetails(t *testing.T) {
	p, ok := LookupProvider("ollama-cloud")
	if !ok {
		t.Fatal("ollama-cloud not found")
	}
	if p.Protocol != ProtocolOpenAIChatCompletions {
		t.Errorf("Protocol = %q, want %q", p.Protocol, ProtocolOpenAIChatCompletions)
	}
	if p.BaseURL != "https://ollama.com/v1" {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL, "https://ollama.com/v1")
	}
	if p.EnvVar != "OLLAMA_API_KEY" {
		t.Errorf("EnvVar = %q, want %q", p.EnvVar, "OLLAMA_API_KEY")
	}
	if p.AuthHeader != "" {
		t.Errorf("AuthHeader = %q, want empty (OpenAI-compatible uses Bearer by default)", p.AuthHeader)
	}
	// Suggested models are a subset of the live /v1/models catalog.
	expectedModels := []string{
		"deepseek-v4.1-flash",
		"deepseek-v4-pro:0813",
		"kimi-k3.1-pro",
		"glm-5.3-flash",
		"minimax-m3.1-flash",
		"mistral-large-4",
	}
	if len(p.Models) != len(expectedModels) {
		t.Fatalf("Models length = %d, want %d", len(p.Models), len(expectedModels))
	}
	for i, model := range expectedModels {
		if p.Models[i] != model {
			t.Errorf("Models[%d] = %q, want %q", i, p.Models[i], model)
		}
	}
}

func TestLookupProvider_LiteLLMDetails(t *testing.T) {
	p, ok := LookupProvider("litellm")
	if !ok {
		t.Fatal("litellm not found")
	}
	if p.DisplayName != "LiteLLM AI Gateway" {
		t.Errorf("DisplayName = %q, want %q", p.DisplayName, "LiteLLM AI Gateway")
	}
	if p.Protocol != ProtocolOpenAIChatCompletions {
		t.Errorf("Protocol = %q, want %q", p.Protocol, ProtocolOpenAIChatCompletions)
	}
	if p.BaseURL != "http://localhost:4000/v1" {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL, "http://localhost:4000/v1")
	}
	if p.EnvVar != "LITELLM_API_KEY" {
		t.Errorf("EnvVar = %q, want %q", p.EnvVar, "LITELLM_API_KEY")
	}
	if p.AuthHeader != "" {
		t.Errorf("AuthHeader = %q, want empty (OpenAI-compatible uses Bearer by default)", p.AuthHeader)
	}
	// LiteLLM uses provider/model format for routing to backing providers.
	expectedModels := []string{
		"anthropic/claude-sonnet-5-5",
		"openai/gpt-5.5",
		"vertex_ai/gemini-4-neon",
		"mistral/mistral-large-latest",
		"deepseek/deepseek-flash",
	}
	if len(p.Models) != len(expectedModels) {
		t.Fatalf("Models length = %d, want %d", len(p.Models), len(expectedModels))
	}
	for i, model := range expectedModels {
		if p.Models[i] != model {
			t.Errorf("Models[%d] = %q, want %q", i, p.Models[i], model)
		}
	}
}

func TestLookupProvider_MistralDetails(t *testing.T) {
	p, ok := LookupProvider("mistral")
	if !ok {
		t.Fatal("mistral not found")
	}
	if p.DisplayName != "Mistral AI" {
		t.Errorf("DisplayName = %q, want %q", p.DisplayName, "Mistral AI")
	}
	if p.Protocol != ProtocolOpenAIChatCompletions {
		t.Errorf("Protocol = %q, want %q", p.Protocol, ProtocolOpenAIChatCompletions)
	}
	if p.BaseURL != "https://api.mistral.ai/v1" {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL, "https://api.mistral.ai/v1")
	}
	if p.EnvVar != "MISTRAL_API_KEY" {
		t.Errorf("EnvVar = %q, want %q", p.EnvVar, "MISTRAL_API_KEY")
	}
	if p.AuthHeader != "" {
		t.Errorf("AuthHeader = %q, want empty (OpenAI-compatible uses Bearer by default)", p.AuthHeader)
	}
	expectedModels := []string{
		"mistral-large-latest",
		"mistral-small-latest",
		"ministral-14b-latest",
		"zai-glm-5-3",
	}
	if len(p.Models) != len(expectedModels) {
		t.Fatalf("Models length = %d, want %d", len(p.Models), len(expectedModels))
	}
	for i, model := range expectedModels {
		if p.Models[i] != model {
			t.Errorf("Models[%d] = %q, want %q", i, p.Models[i], model)
		}
	}
}

func TestLookupProvider_XAIDetails(t *testing.T) {
	p, ok := LookupProvider("xai")
	if !ok {
		t.Fatal("xai not found")
	}
	if p.DisplayName != "xAI Grok API" {
		t.Errorf("DisplayName = %q, want %q", p.DisplayName, "xAI Grok API")
	}
	if p.Protocol != ProtocolOpenAIChatCompletions {
		t.Errorf("Protocol = %q, want %q", p.Protocol, ProtocolOpenAIChatCompletions)
	}
	if p.BaseURL != "https://api.x.ai/v1" {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL, "https://api.x.ai/v1")
	}
	if p.EnvVar != "XAI_API_KEY" {
		t.Errorf("EnvVar = %q, want %q", p.EnvVar, "XAI_API_KEY")
	}
	if p.AuthHeader != "" {
		t.Errorf("AuthHeader = %q, want empty (OpenAI-compatible uses Bearer by default)", p.AuthHeader)
	}
	expectedModels := []string{
		"grok-4.7",
	}
	if len(p.Models) != len(expectedModels) {
		t.Fatalf("Models length = %d, want %d", len(p.Models), len(expectedModels))
	}
	for i, model := range expectedModels {
		if p.Models[i] != model {
			t.Errorf("Models[%d] = %q, want %q", i, p.Models[i], model)
		}
	}
}

// TestProviders_AllProtocolsCanonical verifies every registry entry uses a
// canonical protocol constant — no stale "openai" / "anthropic" literals that
// would bypass NormalizeProtocol downstream.
func TestProviders_AllProtocolsCanonical(t *testing.T) {
	// Delegates to ValidateProtocol rather than re-listing the canonical names,
	// so adding a protocol does not silently leave this assertion behind.
	for _, p := range ListProviders() {
		if NormalizeProtocol(p.Protocol) != p.Protocol {
			t.Errorf("provider %q Protocol %q is not in canonical form", p.Name, p.Protocol)
		}
		if err := ValidateProtocol(p.Protocol); err != nil {
			t.Errorf("provider %q has non-canonical Protocol %q: %v", p.Name, p.Protocol, err)
		}
	}
}

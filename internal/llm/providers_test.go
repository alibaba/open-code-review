// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"sort"
	"strings"
	"testing"
)

func TestLookupProvider_KnownProviders(t *testing.T) {
	names := []string{"anthropic", "openai", "dashscope", "edenai"}
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
		if len(p.Models) == 0 {
			t.Errorf("LookupProvider(%q).Models is empty", name)
		}
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
			if provider.BaseURL != tt.wantURL {
				t.Errorf("LookupProvider(%q).BaseURL = %q, want %q", tt.name, provider.BaseURL, tt.wantURL)
			}
			if provider.EnvVar != tt.wantEnvVar {
				t.Errorf("LookupProvider(%q).EnvVar = %q, want %q", tt.name, provider.EnvVar, tt.wantEnvVar)
			}
		})
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
	expected := []string{"anthropic", "baidu-qianfan", "bedrock", "codex", "dashscope", "dashscope-tokenplan", "deepseek", "edenai", "gemini", "hy-tokenplan", "iflytek", "kimi", "kimi-global", "litellm", "mimo", "minimax", "minimax-cn", "mistral", "novita", "ollama-cloud", "openai", "openai-responses", "siliconflow", "siliconflow-cn", "tencent-tokenhub", "volcengine", "xai", "z-ai", "z-ai-coding"}
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
		"claude-opus-5",
		"claude-sonnet-5",
		"claude-opus-4-8",
		"claude-opus-4-7",
		"claude-opus-4-6",
		"claude-sonnet-4-6",
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
	// GPT-5.6 models are intentionally absent here: they require the Responses
	// API and live under the "openai-responses" preset (issue #559).
	expectedModels := []string{
		"gpt-5.5",
		"gpt-5.4",
		"gpt-5.4-mini",
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

func TestLookupProvider_CodexDetails(t *testing.T) {
	p, ok := LookupProvider("codex")
	if !ok {
		t.Fatal("codex not found")
	}
	if p.Protocol != ProtocolOpenAIResponses {
		t.Errorf("Protocol = %q, want %q", p.Protocol, ProtocolOpenAIResponses)
	}
	if p.BaseURL != "https://chatgpt.com/backend-api/codex" {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL, "https://chatgpt.com/backend-api/codex")
	}
	if p.EnvVar != "" {
		t.Errorf("EnvVar = %q, want empty", p.EnvVar)
	}
	if !p.ExternalAuth {
		t.Error("ExternalAuth = false, want true")
	}
	if !p.RequiresStreaming {
		t.Error("RequiresStreaming = false, want true")
	}
	if !p.RejectsSamplingParams {
		t.Error("RejectsSamplingParams = false, want true")
	}
	if !p.DetailErrorEnvelope {
		t.Error("DetailErrorEnvelope = false, want true")
	}
	expectedModels := []string{"gpt-5.6-luna", "gpt-5.6-terra"}
	if len(p.Models) != len(expectedModels) {
		t.Fatalf("Models length = %d, want %d", len(p.Models), len(expectedModels))
	}
	for i, model := range expectedModels {
		if p.Models[i] != model {
			t.Errorf("Models[%d] = %q, want %q", i, p.Models[i], model)
		}
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
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
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

// TestGPT56ModelsUseResponsesProtocol guards the fix for issue #559: GPT-5.6
// models require the OpenAI Responses API (/v1/responses). A built-in provider
// pointing at api.openai.com must never offer a gpt-5.6 model over Chat
// Completions, because ocr's tool-calling review workflow then fails with a
// 400 ("function tools with reasoning_effort are not supported ... in
// /v1/chat/completions").
func TestGPT56ModelsUseResponsesProtocol(t *testing.T) {
	const gpt56Prefix = "gpt-5.6"
	found := false
	for _, p := range ListProviders() {
		if !strings.Contains(p.BaseURL, "api.openai.com") {
			continue
		}
		for _, m := range p.Models {
			if !strings.HasPrefix(m, gpt56Prefix) {
				continue
			}
			found = true
			if p.Protocol != ProtocolOpenAIResponses {
				t.Errorf("provider %q offers %q over %q; GPT-5.6 needs %q", p.Name, m, p.Protocol, ProtocolOpenAIResponses)
			}
		}
	}
	if !found {
		t.Fatalf("no built-in OpenAI provider offers a %q model; expected the openai-responses preset to serve them", gpt56Prefix)
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
	// Models list mirrors the live /v1/models endpoint (as of Jul 2026).
	// Every entry was runtime-verified to support tool calling via /v1/chat/completions.
	expectedModels := []string{
		"deepseek-v4-flash",
		"deepseek-v4-pro",
		"gemma4:31b",
		"glm-5.1",
		"glm-5.2",
		"gpt-oss:120b",
		"gpt-oss:20b",
		"kimi-k2.5",
		"kimi-k2.6",
		"kimi-k2.7-code",
		"minimax-m2.5",
		"minimax-m2.7",
		"minimax-m3",
		"mistral-large-3:675b",
		"nemotron-3-nano:30b",
		"nemotron-3-super",
		"nemotron-3-ultra",
		"qwen3.5:397b",
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
		"anthropic/claude-sonnet-4-6",
		"anthropic/claude-opus-4-6",
		"anthropic/claude-haiku-4-5",
		"openai/gpt-4o",
		"openai/gpt-5.4",
		"openai/o3",
		"vertex_ai/gemini-2.5-flash",
		"vertex_ai/gemini-2.5-pro",
		"bedrock/anthropic.claude-sonnet-4-6-v1",
		"groq/llama-4-scout-17b-16e-instruct",
		"mistral/mistral-large-latest",
		"deepseek/deepseek-chat",
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
		"codestral-latest",
		"mistral-large-latest",
		"mistral-small-latest",
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
		"grok-4.6",
		"grok-4.5",
		"grok-4.3",
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

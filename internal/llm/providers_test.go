package llm

import (
	"sort"
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
	expected := []string{"anthropic", "baidu-qianfan", "dashscope", "dashscope-tokenplan", "deepseek", "edenai", "hy-tokenplan", "kimi", "mimo", "minimax", "ollama-cloud", "openai", "tencent-tokenhub", "volcengine", "z-ai", "z-ai-coding"}
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
	expected := []string{"claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6", "claude-sonnet-4-6"}
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

// TestProviders_AllProtocolsCanonical verifies every registry entry uses a
// canonical protocol constant — no stale "openai" / "anthropic" literals that
// would bypass NormalizeProtocol downstream.
func TestProviders_AllProtocolsCanonical(t *testing.T) {
	for _, p := range ListProviders() {
		switch p.Protocol {
		case ProtocolAnthropic, ProtocolOpenAIChatCompletions, ProtocolOpenAIResponses:
		default:
			t.Errorf("provider %q has non-canonical Protocol %q", p.Name, p.Protocol)
		}
	}
}

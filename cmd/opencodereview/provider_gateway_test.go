// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestProviderGatewayKeySurvivesModelSelection(t *testing.T) {
	t.Setenv("OCR_LLM_TIMEOUT", "")
	t.Setenv("OCR_LLM_EXTRA_HEADERS", "")
	setTestHome(t, t.TempDir())
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := &Config{
		Provider: "bedrock",
		Model:    "gateway-a",
		Providers: map[string]ProviderEntry{
			"bedrock": {Protocol: llm.ProtocolOpenAIChatCompletions, URL: "https://example.invalid/v1", APIKey: "dummy-gateway-key", Model: "gateway-a", Models: []string{"gateway-a", "gateway-b"}},
		},
	}
	if err := saveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	before, err := llm.ResolveEndpointWithOptions(path, llm.ResolveOptions{Provider: "bedrock"})
	if err != nil || before.Token != "dummy-gateway-key" || before.Protocol != llm.ProtocolOpenAIChatCompletions {
		t.Fatalf("initial gateway configuration must resolve: %v", err)
	}
	m := newProviderTUI(cfg, path)
	if m.currentProvider().Name != "bedrock" {
		t.Fatal("wrong provider")
	}
	updated, _ := m.Update(enterKey())
	m = updated.(providerTUIModel)
	found := false
	for i, model := range m.models() {
		if model == "gateway-b" {
			m.modelIdx = i
			found = true
		}
	}
	if !found {
		t.Fatal("target model missing")
	}
	updated, _ = m.Update(enterKey())
	m = updated.(providerTUIModel)
	if m.step == stepAPIKey {
		updated, _ = m.Update(enterKey())
		m = updated.(providerTUIModel)
	}
	if !m.confirmed {
		t.Fatalf("expected completed model selection, got step=%d", m.step)
	}
	result := m.result()
	if result.resolvedModel() != "gateway-b" {
		t.Fatalf("wrong selected model: %s", result.resolvedModel())
	}
	oldTest := runLLMTestPath
	t.Cleanup(func() { runLLMTestPath = oldTest })
	runLLMTestPath = func(path string) error {
		_, err := llm.ResolveEndpointWithOptions(path, llm.ResolveOptions{Provider: "bedrock"})
		return err
	}
	if err := applyOfficialProviderConfig(path, cfg, result); err != nil {
		t.Fatal(err)
	}
	after, resolveErr := llm.ResolveEndpointWithOptions(path, llm.ResolveOptions{Provider: "bedrock"})
	saved, err := loadOrCreateConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("selected_model=%s persisted_key_cleared=%t resolution_error=%v", saved.Providers["bedrock"].Model, saved.Providers["bedrock"].APIKey == "", resolveErr)
	if resolveErr != nil || after.Token != "dummy-gateway-key" {
		t.Fatalf("changing the model must preserve the existing gateway credential: %v", resolveErr)
	}
}

func TestProviderAmbientProtocolOverrideCanBeSaved(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	setTestHome(t, t.TempDir())
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := &Config{Provider: "anthropic", Providers: map[string]ProviderEntry{
		"anthropic": {Protocol: llm.ProtocolAnthropicBedrock, AWSRegion: "us-west-2", Model: "fixture-model", Models: []string{"fixture-model"}},
	}}
	m := newProviderTUI(cfg, path)
	updated, _ := m.Update(enterKey())
	m = updated.(providerTUIModel)
	updated, _ = m.Update(enterKey())
	m = updated.(providerTUIModel)
	if !m.confirmed || m.step == stepAPIKey {
		t.Fatal("ambient protocol unexpectedly requires a key")
	}
	previous := runLLMTestPath
	t.Cleanup(func() { runLLMTestPath = previous })
	runLLMTestPath = func(path string) error {
		ep, err := llm.ResolveEndpointWithOptions(path, llm.ResolveOptions{Provider: "anthropic"})
		if err == nil && (!ep.AmbientAuth || ep.Token != "") {
			t.Error("saved ambient protocol requires a token")
		}
		return err
	}
	if err := applyOfficialProviderConfig(path, cfg, m.result()); err != nil {
		t.Fatal(err)
	}
}

func TestProviderGatewayOverrideRequiresCredential(t *testing.T) {
	setTestHome(t, t.TempDir())
	cfg := &Config{Provider: "bedrock", Providers: map[string]ProviderEntry{
		"bedrock": {Protocol: llm.ProtocolOpenAIChatCompletions, URL: "https://example.invalid/v1", Model: "gateway", Models: []string{"gateway"}},
	}}
	m := newProviderTUI(cfg, "")
	updated, _ := m.Update(enterKey())
	m = updated.(providerTUIModel)
	updated, _ = m.Update(enterKey())
	m = updated.(providerTUIModel)
	if m.step != stepAPIKey {
		t.Fatal("token protocol skipped its credential step")
	}
	if ok, _ := m.apiKeyStepCanConfirm(); ok {
		t.Fatal("token protocol accepted an absent credential")
	}
	preset, _ := llm.LookupProvider("bedrock")
	if err := checkAPIKeyRequirement("bedrock", "", "", preset, true, cfg.Providers["bedrock"]); err == nil {
		t.Fatal("save validation accepted an absent credential")
	}
}

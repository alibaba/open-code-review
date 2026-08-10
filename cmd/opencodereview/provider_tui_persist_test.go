// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alibaba/open-code-review/internal/llm"
)

// TestPersistCustomModelName covers the providerTUIModel.persistCustomModelName
// branches: empty name, nil config, custom-tab success, official-tab success,
// and both save-failure rollbacks.
func TestPersistCustomModelName(t *testing.T) {
	t.Run("empty name errors", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		if _, err := m.persistCustomModelName(""); err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("nil config not persisted", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.existingCfg = nil
		persisted, err := m.persistCustomModelName("x")
		if err != nil || persisted {
			t.Errorf("got (%v, %v), want (false, nil)", persisted, err)
		}
	})

	t.Run("custom tab success", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		cfg := &Config{
			CustomProviders: map[string]ProviderEntry{
				"cp": {URL: "https://x.example", Protocol: "openai", Models: []string{"m1"}},
			},
		}
		m := newProviderTUI(cfg, path)
		m.activeTab = tabCustom
		m.customIdx = 0
		persisted, err := m.persistCustomModelName("m2")
		if err != nil || !persisted {
			t.Fatalf("got (%v, %v), want (true, nil)", persisted, err)
		}
		if !llm.ModelListContains(cfg.CustomProviders["cp"].Models, "m2") {
			t.Error("m2 not appended to custom provider")
		}
	})

	t.Run("official tab success", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		cfg := &Config{}
		m := newProviderTUI(cfg, path)
		m.activeTab = tabOfficial
		provider := m.currentProvider()
		if provider.Name == "" {
			t.Skip("no official provider available")
		}
		persisted, err := m.persistCustomModelName("my-model")
		if err != nil || !persisted {
			t.Fatalf("got (%v, %v), want (true, nil)", persisted, err)
		}
		if !llm.ModelListContains(cfg.Providers[provider.Name].Models, "my-model") {
			t.Error("my-model not appended to official provider entry")
		}
	})

	t.Run("custom tab save failure rolls back", func(t *testing.T) {
		cfg := &Config{
			CustomProviders: map[string]ProviderEntry{
				"cp": {URL: "https://x.example", Protocol: "openai", Models: []string{"m1"}},
			},
		}
		m := newProviderTUI(cfg, unwritableConfigPath(t))
		m.activeTab = tabCustom
		m.customIdx = 0
		persisted, err := m.persistCustomModelName("m2")
		if err == nil || persisted {
			t.Fatalf("got (%v, %v), want (false, error)", persisted, err)
		}
		if llm.ModelListContains(cfg.CustomProviders["cp"].Models, "m2") {
			t.Error("m2 should be rolled back after save failure")
		}
	})

	t.Run("official tab save failure rolls back", func(t *testing.T) {
		cfg := &Config{}
		m := newProviderTUI(cfg, unwritableConfigPath(t))
		m.activeTab = tabOfficial
		provider := m.currentProvider()
		if provider.Name == "" {
			t.Skip("no official provider available")
		}
		persisted, err := m.persistCustomModelName("my-model")
		if err == nil || persisted {
			t.Fatalf("got (%v, %v), want (false, error)", persisted, err)
		}
		if llm.ModelListContains(cfg.Providers[provider.Name].Models, "my-model") {
			t.Error("my-model should be rolled back after save failure")
		}
	})
}

// TestUpdateCustomModelInput drives the providerTUIModel custom-model input
// sub-mode: esc exit, empty enter, duplicate name, valid enter persists, and
// default key passthrough.
func TestUpdateCustomModelInput(t *testing.T) {
	newModel := func(t *testing.T) providerTUIModel {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.json")
		cfg := &Config{
			CustomProviders: map[string]ProviderEntry{
				"cp": {URL: "https://x.example", Protocol: "openai", Models: []string{"m1"}},
			},
		}
		m := newProviderTUI(cfg, path)
		m.activeTab = tabCustom
		m.customIdx = 0
		m.customModel = true
		return m
	}

	t.Run("esc exits", func(t *testing.T) {
		m := newModel(t)
		out, _ := m.updateCustomModelInput("esc", tea.KeyPressMsg{Code: tea.KeyEscape})
		if out.(providerTUIModel).customModel {
			t.Error("esc should exit custom model input")
		}
	})

	t.Run("empty enter stays", func(t *testing.T) {
		m := newModel(t)
		m.modelInput.SetValue("")
		out, _ := m.updateCustomModelInput("enter", enterKey())
		if !out.(providerTUIModel).customModel {
			t.Error("empty enter should stay in custom model input")
		}
	})

	t.Run("duplicate sets formError", func(t *testing.T) {
		m := newModel(t)
		m.modelInput.SetValue("m1")
		out, _ := m.updateCustomModelInput("enter", enterKey())
		if out.(providerTUIModel).formError == "" {
			t.Error("duplicate model should set formError")
		}
	})

	t.Run("valid enter persists", func(t *testing.T) {
		m := newModel(t)
		m.modelInput.SetValue("m2")
		out, _ := m.updateCustomModelInput("enter", enterKey())
		got := out.(providerTUIModel)
		if got.customModel {
			t.Error("valid enter should exit custom model input")
		}
		if !got.savedInSession {
			t.Errorf("valid enter should persist; formError=%q", got.formError)
		}
	})

	t.Run("default key passes through", func(t *testing.T) {
		m := newModel(t)
		out, _ := m.updateCustomModelInput("x", tea.KeyPressMsg{Code: 'x', Text: "x"})
		if !out.(providerTUIModel).customModel {
			t.Error("default key should stay in custom model input")
		}
	})
}

// TestApplyCreateCustomProvider covers the create-custom-provider handler:
// nil config guard, empty config path guard, empty name, duplicate name,
// success, and save failure.
func TestApplyCreateCustomProvider(t *testing.T) {
	setup := func(t *testing.T, cfg *Config, path string) providerTUIModel {
		t.Helper()
		m := newProviderTUI(cfg, path)
		m.activeTab = tabCustom
		m.creatingCustom = true
		m.cpProtocolIdx = 0
		return m
	}

	t.Run("nil config guard", func(t *testing.T) {
		m := setup(t, &Config{}, filepath.Join(t.TempDir(), "c.json"))
		m.existingCfg = nil
		out, _ := m.applyCreateCustomProvider()
		if out.(providerTUIModel).formError == "" {
			t.Error("nil config should set formError")
		}
	})

	t.Run("empty config path guard", func(t *testing.T) {
		m := setup(t, &Config{}, "")
		out, _ := m.applyCreateCustomProvider()
		if out.(providerTUIModel).formError == "" {
			t.Error("empty config path should set formError")
		}
	})

	t.Run("empty name", func(t *testing.T) {
		m := setup(t, &Config{}, filepath.Join(t.TempDir(), "c.json"))
		m.cpNameInput.SetValue("")
		out, _ := m.applyCreateCustomProvider()
		if out.(providerTUIModel).formError == "" {
			t.Error("empty name should set formError")
		}
	})

	t.Run("duplicate name", func(t *testing.T) {
		cfg := &Config{
			CustomProviders: map[string]ProviderEntry{
				"dup": {URL: "https://x.example", Protocol: "openai"},
			},
		}
		m := setup(t, cfg, filepath.Join(t.TempDir(), "c.json"))
		m.customProviders = collectCustomProviders(cfg)
		m.cpNameInput.SetValue("dup")
		m.cpURLInput.SetValue("https://y.example")
		out, _ := m.applyCreateCustomProvider()
		if out.(providerTUIModel).formError == "" {
			t.Error("duplicate name should set formError")
		}
	})

	t.Run("success", func(t *testing.T) {
		cfg := &Config{}
		m := setup(t, cfg, filepath.Join(t.TempDir(), "c.json"))
		m.cpNameInput.SetValue("brandnew")
		m.cpURLInput.SetValue("https://new.example/v1")
		out, _ := m.applyCreateCustomProvider()
		got := out.(providerTUIModel)
		if !got.savedInSession {
			t.Errorf("success should set savedInSession; formError=%q", got.formError)
		}
		if _, ok := cfg.CustomProviders["brandnew"]; !ok {
			t.Error("new provider not saved to config")
		}
		if got.step != stepModel {
			t.Error("success should advance to stepModel")
		}
	})

	t.Run("save failure", func(t *testing.T) {
		m := setup(t, &Config{}, unwritableConfigPath(t))
		m.cpNameInput.SetValue("brandnew")
		m.cpURLInput.SetValue("https://new.example/v1")
		out, _ := m.applyCreateCustomProvider()
		got := out.(providerTUIModel)
		if got.formError == "" {
			t.Error("save failure should set formError")
		}
		if got.savedInSession {
			t.Error("save failure must not set savedInSession")
		}
	})
}

// TestPersistAddedModelName covers modelTUIModel.persistAddedModelName: empty
// name, nil config, custom success, official success, and both save failures.
func TestPersistAddedModelName(t *testing.T) {
	t.Run("empty name errors", func(t *testing.T) {
		m, _, _ := newCustomModelTUI(t, []string{"m1"})
		if err := m.persistAddedModelName(""); err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("nil config errors", func(t *testing.T) {
		m, _, _ := newCustomModelTUI(t, []string{"m1"})
		m.existingCfg = nil
		if err := m.persistAddedModelName("x"); err == nil {
			t.Fatal("expected error for nil config")
		}
	})

	t.Run("custom success", func(t *testing.T) {
		m, cfg, _ := newCustomModelTUI(t, []string{"m1"})
		if err := m.persistAddedModelName("m2"); err != nil {
			t.Fatalf("persistAddedModelName: %v", err)
		}
		if !llm.ModelListContains(cfg.CustomProviders["myprov"].Models, "m2") {
			t.Error("m2 not added to custom provider")
		}
	})

	t.Run("custom save failure rolls back", func(t *testing.T) {
		cfg := &Config{
			CustomProviders: map[string]ProviderEntry{
				"myprov": {URL: "https://x.example", Protocol: "openai", Models: []string{"m1"}},
			},
		}
		m := newModelTUIConfig(modelTUIConfig{
			Provider:     llm.Provider{Name: "myprov", Models: []string{"m1"}},
			ProviderName: "myprov",
			ExistingCfg:  cfg,
			ConfigPath:   unwritableConfigPath(t),
			IsCustom:     true,
		})
		if err := m.persistAddedModelName("m2"); err == nil {
			t.Fatal("expected save failure error")
		}
		if llm.ModelListContains(cfg.CustomProviders["myprov"].Models, "m2") {
			t.Error("m2 should be rolled back after save failure")
		}
	})

	t.Run("official success", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		cfg := &Config{}
		m := newModelTUIConfig(modelTUIConfig{
			Provider:     llm.Provider{Name: "openai", Models: []string{"gpt-4"}},
			ProviderName: "openai",
			ExistingCfg:  cfg,
			ConfigPath:   path,
			IsCustom:     false,
		})
		if err := m.persistAddedModelName("gpt-x"); err != nil {
			t.Fatalf("persistAddedModelName: %v", err)
		}
		if !llm.ModelListContains(cfg.Providers["openai"].Models, "gpt-x") {
			t.Error("gpt-x not added to official provider entry")
		}
	})

	t.Run("official save failure rolls back", func(t *testing.T) {
		cfg := &Config{}
		m := newModelTUIConfig(modelTUIConfig{
			Provider:     llm.Provider{Name: "openai", Models: []string{"gpt-4"}},
			ProviderName: "openai",
			ExistingCfg:  cfg,
			ConfigPath:   unwritableConfigPath(t),
			IsCustom:     false,
		})
		if err := m.persistAddedModelName("gpt-x"); err == nil {
			t.Fatal("expected save failure error")
		}
		if llm.ModelListContains(cfg.Providers["openai"].Models, "gpt-x") {
			t.Error("gpt-x should be rolled back after save failure")
		}
	})
}

// TestConfirmDeleteOfficialModelActiveClear covers the modelTUIModel handler
// where the deleted model is also the active config model: deletion clears the
// active Model.
func TestConfirmDeleteOfficialModelActiveClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := &Config{
		Provider: "openai",
		Model:    "user-added",
		Providers: map[string]ProviderEntry{
			"openai": {Models: []string{"user-added"}, Model: "user-added"},
		},
	}
	m := newModelTUIConfig(modelTUIConfig{
		Provider:       llm.Provider{Name: "openai", DisplayName: "OpenAI", Models: []string{"gpt-4"}},
		ProviderName:   "openai",
		RegistryModels: []string{"gpt-4"},
		ExistingCfg:    cfg,
		ConfigPath:     path,
		IsCustom:       false,
	})
	m.confirmingDeleteModel = true
	m.deleteModelName = "user-added"

	out, _ := m.confirmDeleteOfficialModel()
	got := out.(modelTUIModel)
	if !got.savedInSession {
		t.Fatalf("should persist deletion; formError=%q", got.formError)
	}
	if cfg.Model == "user-added" {
		t.Error("active Model should be cleared after deleting the active model")
	}
}

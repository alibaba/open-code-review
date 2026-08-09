// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestRegistryModelsForProvider(t *testing.T) {
	t.Run("known preset ignores fallback", func(t *testing.T) {
		got := registryModelsForProvider("anthropic", []string{"junk"})
		if len(got) == 0 {
			t.Fatal("expected preset models for anthropic")
		}
		for _, m := range got {
			if m == "junk" {
				t.Error("fallback leaked into preset result")
			}
		}
	})
	t.Run("unknown uses fallback", func(t *testing.T) {
		got := registryModelsForProvider("no-such-provider", []string{"a", "b"})
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("got %v, want [a b]", got)
		}
	})
	t.Run("unknown without fallback is nil", func(t *testing.T) {
		if got := registryModelsForProvider("no-such-provider", nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

func TestApplyModelDeleteToEntry(t *testing.T) {
	entry := ProviderEntry{Models: []string{"m1", "m2"}, Model: "m2"}
	got := applyModelDeleteToEntry(entry, "m2")
	if llm.ModelListContains(got.Models, "m2") {
		t.Error("deleted model still present")
	}
	if got.Model != "" {
		t.Errorf("active model = %q, want cleared", got.Model)
	}
}

func TestClearCfgActiveModelIfDeleted(t *testing.T) {
	t.Run("clears matching", func(t *testing.T) {
		cfg := &Config{Provider: "p", Model: "m"}
		clearCfgActiveModelIfDeleted(cfg, "p", "m")
		if cfg.Model != "" {
			t.Errorf("model = %q, want cleared", cfg.Model)
		}
	})
	t.Run("keeps non-matching provider", func(t *testing.T) {
		cfg := &Config{Provider: "other", Model: "m"}
		clearCfgActiveModelIfDeleted(cfg, "p", "m")
		if cfg.Model != "m" {
			t.Errorf("model = %q, want unchanged", cfg.Model)
		}
	})
	t.Run("nil cfg is a no-op", func(t *testing.T) {
		clearCfgActiveModelIfDeleted(nil, "p", "m") // must not panic
	})
}

func TestRollbackCfgActiveModel(t *testing.T) {
	cfg := &Config{Provider: "p", Model: ""}
	rollbackCfgActiveModel(cfg, "p", "prev")
	if cfg.Model != "prev" {
		t.Errorf("model = %q, want prev", cfg.Model)
	}
	// Different provider: unchanged.
	cfg2 := &Config{Provider: "other", Model: "x"}
	rollbackCfgActiveModel(cfg2, "p", "prev")
	if cfg2.Model != "x" {
		t.Errorf("model = %q, want x", cfg2.Model)
	}
	rollbackCfgActiveModel(nil, "p", "prev") // must not panic
}

func TestRollbackModelDelete(t *testing.T) {
	t.Run("nil cfg is a no-op", func(t *testing.T) {
		m := &modelTUIModel{existingCfg: nil}
		m.rollbackModelDelete(ProviderEntry{}, "") // must not panic
	})

	t.Run("custom provider restores entry and active model", func(t *testing.T) {
		cfg := &Config{
			Provider:        "cp",
			Model:           "",
			CustomProviders: map[string]ProviderEntry{"cp": {Models: []string{"a"}}},
		}
		m := &modelTUIModel{
			existingCfg:      cfg,
			providerName:     "cp",
			isCustomProvider: true,
		}
		prev := ProviderEntry{Models: []string{"a", "b"}, Model: "b"}
		m.rollbackModelDelete(prev, "b")
		if got := cfg.CustomProviders["cp"]; len(got.Models) != 2 {
			t.Errorf("restored models = %v, want 2 entries", got.Models)
		}
		if cfg.Model != "b" {
			t.Errorf("active model = %q, want b", cfg.Model)
		}
	})

	t.Run("official provider restores entry", func(t *testing.T) {
		cfg := &Config{
			Provider:  "op",
			Providers: map[string]ProviderEntry{"op": {}},
		}
		m := &modelTUIModel{
			existingCfg:      cfg,
			providerName:     "op",
			isCustomProvider: false,
		}
		prev := ProviderEntry{Models: []string{"x"}, Model: "x"}
		m.rollbackModelDelete(prev, "x")
		if got := cfg.Providers["op"]; len(got.Models) != 1 {
			t.Errorf("restored models = %v, want 1 entry", got.Models)
		}
	})
}

func TestModelReloadConfigAfterSaveFailure(t *testing.T) {
	t.Run("empty path returns false", func(t *testing.T) {
		m := &modelTUIModel{configPath: ""}
		if m.reloadConfigAfterSaveFailure() {
			t.Error("expected false for empty configPath")
		}
	})

	t.Run("reloads from disk", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		if err := os.WriteFile(path, []byte(`{"provider":"cp","custom_providers":{"cp":{"models":["m1"]}}}`), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		m := &modelTUIModel{
			configPath:       path,
			providerName:     "cp",
			isCustomProvider: true,
		}
		if !m.reloadConfigAfterSaveFailure() {
			t.Fatal("expected reload to succeed")
		}
		if m.existingCfg == nil || m.existingCfg.Provider != "cp" {
			t.Errorf("reloaded cfg = %+v, want provider cp", m.existingCfg)
		}
	})

	t.Run("parse error returns false", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")
		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		m := &modelTUIModel{configPath: path}
		if m.reloadConfigAfterSaveFailure() {
			t.Error("expected false on parse error")
		}
	})
}

func TestProviderReloadConfigAfterSaveFailure(t *testing.T) {
	t.Run("empty path returns false", func(t *testing.T) {
		m := &providerTUIModel{configPath: ""}
		if m.reloadConfigAfterSaveFailure() {
			t.Error("expected false for empty configPath")
		}
	})

	t.Run("reloads and refreshes custom providers", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		if err := os.WriteFile(path, []byte(`{"custom_providers":{"cp":{"url":"http://x","models":["m1"]}}}`), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		m := &providerTUIModel{configPath: path}
		if !m.reloadConfigAfterSaveFailure() {
			t.Fatal("expected reload to succeed")
		}
		if m.existingCfg == nil {
			t.Fatal("existingCfg not set after reload")
		}
	})
}

func TestAdjustModelIdxAfterDelete(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{"cp": {Models: []string{"a"}}},
	}
	m := &modelTUIModel{
		existingCfg:      cfg,
		providerName:     "cp",
		isCustomProvider: true,
		models:           []string{"a", "b"},
		modelIdx:         5, // out of range
	}
	m.adjustModelIdxAfterDelete()
	if m.modelIdx != 0 {
		t.Errorf("modelIdx = %d, want clamped to 0 (single remaining model)", m.modelIdx)
	}
}

func TestRefreshModelSelectionAfterAdd(t *testing.T) {
	t.Run("selects matching name", func(t *testing.T) {
		m := &modelTUIModel{isCustomProvider: true, models: []string{"a", "b", "c"}}
		m.refreshModelSelectionAfterAdd("b")
		if m.modelIdx != 1 {
			t.Errorf("modelIdx = %d, want 1", m.modelIdx)
		}
	})
	t.Run("falls back to last when absent", func(t *testing.T) {
		m := &modelTUIModel{isCustomProvider: true, models: []string{"a", "b"}}
		m.refreshModelSelectionAfterAdd("missing")
		if m.modelIdx != 1 {
			t.Errorf("modelIdx = %d, want last index 1", m.modelIdx)
		}
	})
}

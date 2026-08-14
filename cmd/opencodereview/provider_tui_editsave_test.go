// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestApplyEditCustomProviderSave_Guards covers the two early-return guards:
// a nil config and an empty config path.
func TestApplyEditCustomProviderSave_Guards(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		m := &providerTUIModel{}
		if err := m.applyEditCustomProviderSave(); err == nil {
			t.Fatal("expected error when config is nil")
		}
		if m.formError == "" {
			t.Error("formError should be set when config is nil")
		}
	})

	t.Run("empty config path", func(t *testing.T) {
		m := &providerTUIModel{existingCfg: &Config{}}
		if err := m.applyEditCustomProviderSave(); err == nil {
			t.Fatal("expected error when config path is empty")
		}
		if m.formError == "" {
			t.Error("formError should be set when config path is empty")
		}
	})
}

// TestApplyEditCustomProviderSave_RenameReassignsActiveProvider covers the
// name-change branch where the edited provider is also the active provider: the
// old key is deleted, and the active Provider/Model are re-pointed at the new
// name.
func TestApplyEditCustomProviderSave_RenameReassignsActiveProvider(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Provider: "oldname",
		Model:    "some-model",
		CustomProviders: map[string]ProviderEntry{
			"oldname": {URL: "https://example.com/v1", Protocol: "openai"},
		},
	}
	m := newProviderTUI(cfg, configPath)
	m.activeTab = tabCustom
	m.editingCustom = true
	m.editTargetName = "oldname"
	m.cpProtocolIdx = 1 // openai
	m.cpNameInput.SetValue("newname")
	m.cpURLInput.SetValue("https://example.com/v1")

	if err := m.applyEditCustomProviderSave(); err != nil {
		t.Fatalf("applyEditCustomProviderSave: %v", err)
	}
	if _, ok := cfg.CustomProviders["oldname"]; ok {
		t.Error("old provider key should be deleted after rename")
	}
	if _, ok := cfg.CustomProviders["newname"]; !ok {
		t.Error("new provider key should exist after rename")
	}
	if cfg.Provider != "newname" {
		t.Errorf("active Provider = %q, want newname", cfg.Provider)
	}
	if cfg.Model != "" {
		t.Errorf("active Model = %q, want cleared", cfg.Model)
	}
}

// TestApplyEditCustomProviderSave_SaveFailureRestoresBackup covers the
// save-failure path where the reload also fails (config path is a directory, so
// both save and reload error) and the in-memory backup is restored.
func TestApplyEditCustomProviderSave_SaveFailureRestoresBackup(t *testing.T) {
	dir := t.TempDir()
	// A directory path makes both saveConfig and the reload fallback fail.
	blockPath := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blockPath, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"aaa": {URL: "https://example.com/v1", Protocol: "openai", Models: []string{"m1"}},
		},
	}
	m := newProviderTUI(cfg, blockPath)
	m.activeTab = tabCustom
	m.editingCustom = true
	m.editTargetName = "aaa"
	m.cpProtocolIdx = 1
	m.cpNameInput.SetValue("aaa")
	m.cpURLInput.SetValue("https://changed.example.com/v1")

	if err := m.applyEditCustomProviderSave(); err == nil {
		t.Fatal("expected save error when config path is a directory")
	}
	if m.formError == "" {
		t.Error("formError should be set on save failure")
	}
	if m.savedInSession {
		t.Error("savedInSession must stay false on save failure")
	}
	// Backup restored: the URL edit should not have stuck.
	if got := cfg.CustomProviders["aaa"].URL; got != "https://example.com/v1" {
		t.Errorf("URL = %q, want original restored", got)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unwritableConfigPath returns a config path that is itself a directory, so any
// saveConfig / loadOrCreateConfig against it fails. This is the lever used to
// drive the save-failure + rollback branches of the TUI handlers.
//
// A directory rather than the more obvious "parent is a regular file" trick:
// Windows reports a path below a non-directory parent as ERROR_PATH_NOT_FOUND,
// which os.IsNotExist accepts, so loadOrCreateConfig read that as "no config
// yet" and returned an empty Config instead of an error. The reload then looked
// like a success and the rollback branch never ran. A directory fails the write
// on every platform, and reading it yields either an error or empty bytes that
// fail to parse as JSON, so the reload fails everywhere too.
func unwritableConfigPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir blocking config dir: %v", err)
	}
	return path
}

// TestUpdateDeleteConfirm_SaveFailure drives the provider-delete confirm handler
// down its save-failure branch: the in-memory delete happens, saveConfig fails,
// and the handler surfaces a formError without marking savedInSession.
func TestUpdateDeleteConfirm_SaveFailure(t *testing.T) {
	cfg := &Config{
		Provider: "cp",
		Model:    "m1",
		CustomProviders: map[string]ProviderEntry{
			"cp": {URL: "https://x.example", Protocol: "openai", Models: []string{"m1"}},
		},
	}
	m := newProviderTUI(cfg, unwritableConfigPath(t))
	m.activeTab = tabCustom
	m.confirmingDelete = true
	m.deleteTargetIdx = 0
	m.deleteTargetName = "cp"

	out, _ := m.updateDeleteConfirm("y")
	got := out.(providerTUIModel)

	if !strings.Contains(got.formError, "failed to save") {
		t.Errorf("formError = %q, want save-failure message", got.formError)
	}
	if got.savedInSession {
		t.Error("savedInSession should be false after save failure")
	}
	if got.confirmingDelete {
		t.Error("confirmingDelete should be cleared after handling")
	}
}

// TestConfirmDeleteCustomModel_SaveFailureRollback drives the custom-model delete
// handler down its save-failure branch and asserts the entry is rolled back to
// its pre-delete state (reload fails, so the in-memory rollback runs).
func TestConfirmDeleteCustomModel_SaveFailureRollback(t *testing.T) {
	cfg := &Config{
		Provider: "cp",
		Model:    "m2",
		CustomProviders: map[string]ProviderEntry{
			"cp": {
				URL:      "https://x.example",
				Protocol: "openai",
				Models:   []string{"m1", "m2"},
				Model:    "m2",
			},
		},
	}
	m := newProviderTUI(cfg, unwritableConfigPath(t))
	m.activeTab = tabCustom
	m.customIdx = 0
	m.step = stepModel
	m.modelIdx = 1
	m.deleteModelName = "m2"
	m.confirmingDeleteModel = true

	out, _ := m.confirmDeleteCustomModel()
	got := out.(providerTUIModel)

	if !strings.Contains(got.formError, "failed to save") {
		t.Errorf("formError = %q, want save-failure message", got.formError)
	}
	if got.savedInSession {
		t.Error("savedInSession should be false after save failure")
	}
	// Rollback restored the model list in the in-memory config.
	entry := got.existingCfg.CustomProviders["cp"]
	if len(entry.Models) != 2 {
		t.Errorf("rolled-back models = %v, want [m1 m2]", entry.Models)
	}
	if entry.Model != "m2" {
		t.Errorf("rolled-back active model = %q, want m2", entry.Model)
	}
}

// TestConfirmDeleteOfficialModel_SaveFailureRollback drives the official-model
// delete handler down its save-failure branch for a user-added model and asserts
// the provider entry is rolled back.
func TestConfirmDeleteOfficialModel_SaveFailureRollback(t *testing.T) {
	m := newProviderTUI(&Config{}, unwritableConfigPath(t))
	m.activeTab = tabOfficial
	provider := m.currentProvider()
	if provider.Name == "" {
		t.Skip("no official provider available")
	}
	userModel := "user-added-model-xyz"
	cfg := &Config{
		Provider: provider.Name,
		Model:    userModel,
		Providers: map[string]ProviderEntry{
			provider.Name: {Models: []string{userModel}, Model: userModel},
		},
	}
	m.existingCfg = cfg
	m.step = stepModel
	m.deleteModelName = userModel
	m.confirmingDeleteModel = true

	if !m.isUserAddedOfficialModel(userModel) {
		t.Fatalf("test setup: %q not recognized as user-added official model", userModel)
	}

	out, _ := m.confirmDeleteOfficialModel()
	got := out.(providerTUIModel)

	if !strings.Contains(got.formError, "failed to save") {
		t.Errorf("formError = %q, want save-failure message", got.formError)
	}
	if got.savedInSession {
		t.Error("savedInSession should be false after save failure")
	}
	// Rollback restored the user-added model.
	entry := got.existingCfg.Providers[provider.Name]
	if !containsStr(entry.Models, userModel) {
		t.Errorf("rolled-back models = %v, want to contain %q", entry.Models, userModel)
	}
}

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

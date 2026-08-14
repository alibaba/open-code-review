// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alibaba/open-code-review/internal/llm"
)

// newCustomModelTUI builds a modelTUIModel backed by a custom provider with a
// writable config path, so add/delete/persist operations exercise the save path.
func newCustomModelTUI(t *testing.T, models []string) (modelTUIModel, *Config, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"myprov": {URL: "https://api.example.com", Protocol: "openai", Models: models},
		},
	}
	m := newModelTUIConfig(modelTUIConfig{
		Provider:     llm.Provider{Name: "myprov", DisplayName: "My Prov", Models: models},
		ProviderName: "myprov",
		ExistingCfg:  cfg,
		ConfigPath:   path,
		IsCustom:     true,
	})
	return m, cfg, path
}

// TestModelTUIUpdate_Navigation covers window resize, up/down wrap-around, and
// enter on both a real model and the custom "add" item.
func TestModelTUIUpdate_Navigation(t *testing.T) {
	m, _, _ := newCustomModelTUI(t, []string{"m1", "m2"})

	// Window resize records dimensions.
	out, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	got := out.(modelTUIModel)
	if got.width != 120 || got.height != 40 {
		t.Errorf("resize not recorded: %dx%d", got.width, got.height)
	}

	// down from 0 advances; up from 0 wraps to last item (the custom "add" row).
	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if out.(modelTUIModel).modelIdx != 1 {
		t.Errorf("down modelIdx = %d, want 1", out.(modelTUIModel).modelIdx)
	}
	m.modelIdx = 0
	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if out.(modelTUIModel).modelIdx != m.itemCount()-1 {
		t.Errorf("up-wrap modelIdx = %d, want %d", out.(modelTUIModel).modelIdx, m.itemCount()-1)
	}

	// enter on a real model confirms and quits.
	m.modelIdx = 0
	out, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !out.(modelTUIModel).confirmed || cmd == nil {
		t.Error("enter on model should confirm and quit")
	}

	// enter on the custom "add" item enters custom-model input mode.
	m.modelIdx = m.itemCount() - 1
	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !out.(modelTUIModel).customModel {
		t.Error("enter on custom item should enter customModel mode")
	}
}

// TestModelTUIUpdate_CancelAndDelete covers esc/ctrl+c cancel and the 'd' key
// entering delete-confirm on a user-added model.
func TestModelTUIUpdate_CancelAndDelete(t *testing.T) {
	m, _, _ := newCustomModelTUI(t, []string{"m1", "m2"})

	out, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !out.(modelTUIModel).cancelled || cmd == nil {
		t.Error("esc should cancel and quit")
	}

	m.modelIdx = 0
	out, _ = m.Update(dKey())
	got := out.(modelTUIModel)
	if !got.confirmingDeleteModel {
		t.Error("'d' on user-added model should begin delete confirm")
	}
	if got.deleteModelName != "m1" {
		t.Errorf("deleteModelName = %q, want m1", got.deleteModelName)
	}
}

// TestModelTUIUpdate_CustomModelInput covers the customModel input sub-mode:
// esc exit, empty enter stays, duplicate enter sets formError, valid enter
// persists and adds, and default keys pass through to the text input.
func TestModelTUIUpdate_CustomModelInput(t *testing.T) {
	t.Run("esc exits custom input", func(t *testing.T) {
		m, _, _ := newCustomModelTUI(t, []string{"m1"})
		m.customModel = true
		out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if out.(modelTUIModel).customModel {
			t.Error("esc should exit customModel mode")
		}
	})

	t.Run("empty enter stays", func(t *testing.T) {
		m, _, _ := newCustomModelTUI(t, []string{"m1"})
		m.customModel = true
		m.modelInput.SetValue("")
		out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if !out.(modelTUIModel).customModel {
			t.Error("empty enter should stay in customModel mode")
		}
	})

	t.Run("duplicate enter sets formError", func(t *testing.T) {
		m, _, _ := newCustomModelTUI(t, []string{"m1"})
		m.customModel = true
		m.modelInput.SetValue("m1")
		out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if out.(modelTUIModel).formError == "" {
			t.Error("duplicate model should set formError")
		}
	})

	t.Run("valid enter persists and adds", func(t *testing.T) {
		m, cfg, _ := newCustomModelTUI(t, []string{"m1"})
		m.customModel = true
		m.modelInput.SetValue("m2")
		out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		got := out.(modelTUIModel)
		if got.customModel {
			t.Error("valid enter should exit customModel mode")
		}
		if !got.savedInSession {
			t.Errorf("valid enter should persist; formError=%q", got.formError)
		}
		if !llm.ModelListContains(cfg.CustomProviders["myprov"].Models, "m2") {
			t.Error("new model should be added to config")
		}
	})

	t.Run("default key passes through", func(t *testing.T) {
		m, _, _ := newCustomModelTUI(t, []string{"m1"})
		m.customModel = true
		out, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		if !out.(modelTUIModel).customModel {
			t.Error("default key should stay in customModel mode")
		}
	})
}

// TestUpdateDeleteModelConfirm covers y (confirm), n/esc (cancel), and ctrl+c.
func TestUpdateDeleteModelConfirm(t *testing.T) {
	t.Run("n cancels", func(t *testing.T) {
		m, _, _ := newCustomModelTUI(t, []string{"m1"})
		m.confirmingDeleteModel = true
		out, _ := m.updateDeleteModelConfirm("n")
		if out.(modelTUIModel).confirmingDeleteModel {
			t.Error("'n' should cancel delete confirm")
		}
	})

	t.Run("ctrl+c quits", func(t *testing.T) {
		m, _, _ := newCustomModelTUI(t, []string{"m1"})
		m.confirmingDeleteModel = true
		out, cmd := m.updateDeleteModelConfirm("ctrl+c")
		if !out.(modelTUIModel).cancelled || cmd == nil {
			t.Error("ctrl+c should cancel and quit")
		}
	})

	t.Run("y confirms delete", func(t *testing.T) {
		m, cfg, _ := newCustomModelTUI(t, []string{"m1", "m2"})
		m.confirmingDeleteModel = true
		m.deleteModelName = "m2"
		out, _ := m.updateDeleteModelConfirm("y")
		got := out.(modelTUIModel)
		if got.confirmingDeleteModel {
			t.Error("'y' should end delete confirm")
		}
		if !got.savedInSession {
			t.Errorf("'y' should persist deletion; formError=%q", got.formError)
		}
		if llm.ModelListContains(cfg.CustomProviders["myprov"].Models, "m2") {
			t.Error("deleted model should be gone from config")
		}
	})
}

// TestConfirmDeleteCustomProviderModel_Guards covers the not-user-added and
// nil-cfg early returns.
func TestConfirmDeleteCustomProviderModel_Guards(t *testing.T) {
	t.Run("unknown model is a no-op", func(t *testing.T) {
		m, _, _ := newCustomModelTUI(t, []string{"m1"})
		m.confirmingDeleteModel = true
		m.deleteModelName = "not-in-list"
		out, _ := m.confirmDeleteCustomProviderModel()
		got := out.(modelTUIModel)
		if got.confirmingDeleteModel {
			t.Error("unknown model should end confirm without saving")
		}
		if got.savedInSession {
			t.Error("unknown model must not persist")
		}
	})

	t.Run("nil cfg is a no-op", func(t *testing.T) {
		m, _, _ := newCustomModelTUI(t, []string{"m1"})
		m.deleteModelName = "m1"
		m.existingCfg = nil
		m.confirmingDeleteModel = true
		out, _ := m.confirmDeleteCustomProviderModel()
		if out.(modelTUIModel).savedInSession {
			t.Error("nil cfg must not persist")
		}
	})
}

// TestConfirmDeleteOfficialModel covers deleting a user-added model from an
// official (preset) provider entry, plus the not-user-added guard.
func TestConfirmDeleteOfficialModel(t *testing.T) {
	newOfficial := func(t *testing.T) (modelTUIModel, *Config) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.json")
		cfg := &Config{
			Providers: map[string]ProviderEntry{
				"openai": {Models: []string{"user-added"}},
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
		return m, cfg
	}

	t.Run("deletes user-added model", func(t *testing.T) {
		m, cfg := newOfficial(t)
		m.confirmingDeleteModel = true
		m.deleteModelName = "user-added"
		out, _ := m.confirmDeleteOfficialModel()
		got := out.(modelTUIModel)
		if !got.savedInSession {
			t.Errorf("should persist deletion; formError=%q", got.formError)
		}
		if llm.ModelListContains(cfg.Providers["openai"].Models, "user-added") {
			t.Error("deleted model should be gone from config")
		}
	})

	t.Run("registry model is not deletable", func(t *testing.T) {
		m, _ := newOfficial(t)
		m.confirmingDeleteModel = true
		m.deleteModelName = "gpt-4" // in registry, not user-added
		out, _ := m.confirmDeleteOfficialModel()
		if out.(modelTUIModel).savedInSession {
			t.Error("registry model must not be deletable")
		}
	})
}

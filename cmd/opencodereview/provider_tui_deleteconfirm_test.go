// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestUpdateDeleteModelConfirm covers the key branches of the delete-model
// confirmation handler: cancel (n/esc), ctrl+c quit, and the default-tab
// no-op.
func TestUpdateDeleteModelConfirm_ProviderTUI(t *testing.T) {
	newModel := func() providerTUIModel {
		cfg := &Config{}
		m := newProviderTUI(cfg, filepath.Join(t.TempDir(), "config.json"))
		m.confirmingDeleteModel = true
		return m
	}

	t.Run("n cancels", func(t *testing.T) {
		m := newModel()
		out, _ := m.updateDeleteModelConfirm("n")
		if out.(providerTUIModel).confirmingDeleteModel {
			t.Error("n should cancel the delete confirmation")
		}
	})

	t.Run("esc cancels", func(t *testing.T) {
		m := newModel()
		out, _ := m.updateDeleteModelConfirm("esc")
		if out.(providerTUIModel).confirmingDeleteModel {
			t.Error("esc should cancel the delete confirmation")
		}
	})

	t.Run("ctrl+c quits", func(t *testing.T) {
		m := newModel()
		out, cmd := m.updateDeleteModelConfirm("ctrl+c")
		if !out.(providerTUIModel).cancelled {
			t.Error("ctrl+c should mark the model cancelled")
		}
		if cmd == nil {
			t.Error("ctrl+c should return a quit command")
		}
	})

	t.Run("y on manual tab is a no-op", func(t *testing.T) {
		m := newModel()
		m.activeTab = tabManual
		out, _ := m.updateDeleteModelConfirm("y")
		if out.(providerTUIModel).confirmingDeleteModel {
			t.Error("y on manual tab should clear the confirmation without deleting")
		}
	})

	t.Run("unhandled key is ignored", func(t *testing.T) {
		m := newModel()
		out, _ := m.updateDeleteModelConfirm("z")
		if !out.(providerTUIModel).confirmingDeleteModel {
			t.Error("unhandled key should leave the confirmation open")
		}
	})
}

// deleteFlowTUI adapts the two models that implement model deletion,
// providerTUIModel (ocr config provider) and modelTUIModel (ocr config model),
// so each delete scenario below is written once and run against both.
type deleteFlowTUI struct {
	name string
	// official returns a dashscope model whose extraModels are user-added.
	official func(t *testing.T, configPath string, extraModels []string) tea.Model
	// custom returns a custom provider whose models include "aaa".
	custom func(t *testing.T) tea.Model
	// selectModel moves the cursor to name, or to the custom-input row when
	// name is empty.
	selectModel func(t *testing.T, m tea.Model, name string) tea.Model
	confirming  func(m tea.Model) bool
	cfg         func(m tea.Model) *Config
}

var deleteFlowTUIs = []deleteFlowTUI{
	{
		name: "provider TUI",
		official: func(t *testing.T, configPath string, extraModels []string) tea.Model {
			return officialDashscopeModelTUI(t, configPath, extraModels)
		},
		custom: func(t *testing.T) tea.Model {
			return customStepfunModelTUI(t, "", []string{"step-3.5-flash", "aaa"})
		},
		selectModel: func(t *testing.T, m tea.Model, name string) tea.Model {
			pm := m.(providerTUIModel)
			if name == "" {
				pm.modelIdx = len(pm.models())
			} else {
				pm.modelIdx = modelIdxForName(t, pm, name)
			}
			return pm
		},
		confirming: func(m tea.Model) bool { return m.(providerTUIModel).confirmingDeleteModel },
		cfg:        func(m tea.Model) *Config { return m.(providerTUIModel).existingCfg },
	},
	{
		name: "model TUI",
		official: func(t *testing.T, configPath string, extraModels []string) tea.Model {
			return officialConfigModelTUI(t, configPath, extraModels)
		},
		custom: func(t *testing.T) tea.Model {
			return customConfigModelTUI(t, "", []string{"m1", "aaa"})
		},
		selectModel: func(t *testing.T, m tea.Model, name string) tea.Model {
			mm := m.(modelTUIModel)
			if name == "" {
				mm.modelIdx = len(mm.displayModels())
			} else {
				mm.modelIdx = modelTUIIdxForName(t, mm, name)
			}
			return mm
		},
		confirming: func(m tea.Model) bool { return m.(modelTUIModel).confirmingDeleteModel },
		cfg:        func(m tea.Model) *Config { return m.(modelTUIModel).existingCfg },
	},
}

func viewText(m tea.Model) string { return stripANSI(m.View().Content) }

func TestDeleteFlow_OfficialDeleteHintOnlyOnUserAddedModel(t *testing.T) {
	for _, tui := range deleteFlowTUIs {
		t.Run(tui.name, func(t *testing.T) {
			m := tui.official(t, "", []string{"my-custom-model"})
			if got := viewText(tui.selectModel(t, m, "qwen3.7-max")); strings.Contains(got, "d Delete") {
				t.Errorf("built-in model should not show d Delete hint; got:\n%s", got)
			}
			if got := viewText(tui.selectModel(t, m, "my-custom-model")); !strings.Contains(got, "d Delete") {
				t.Errorf("user-added model should show d Delete hint; got:\n%s", got)
			}
		})
	}
}

func TestDeleteFlow_CustomDeleteHintOnlyOnModelRows(t *testing.T) {
	for _, tui := range deleteFlowTUIs {
		t.Run(tui.name, func(t *testing.T) {
			m := tui.custom(t)
			if got := viewText(tui.selectModel(t, m, "")); strings.Contains(got, "d Delete") {
				t.Errorf("custom input row should not show d Delete hint; got:\n%s", got)
			}
			if got := viewText(tui.selectModel(t, m, "aaa")); !strings.Contains(got, "d Delete") {
				t.Errorf("custom model row should show d Delete hint; got:\n%s", got)
			}
		})
	}
}

func TestDeleteFlow_OfficialDeleteIgnoredOnNonDeletableRows(t *testing.T) {
	rows := []struct{ name, model string }{
		{"registry model", "qwen3.7-max"},
		{"custom input row", ""},
	}
	for _, tui := range deleteFlowTUIs {
		for _, row := range rows {
			t.Run(tui.name+"/"+row.name, func(t *testing.T) {
				m := tui.selectModel(t, tui.official(t, "", []string{"my-custom-model"}), row.model)
				m, _ = m.Update(dKey())
				if tui.confirming(m) {
					t.Error("d should not trigger delete confirmation")
				}
			})
		}
	}
}

func TestDeleteFlow_OfficialDeleteCancel(t *testing.T) {
	cancelKeys := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"n", nKey()},
		{"esc", escKey()},
	}
	for _, tui := range deleteFlowTUIs {
		for _, tc := range cancelKeys {
			t.Run(tui.name+"/"+tc.name, func(t *testing.T) {
				m := tui.selectModel(t, tui.official(t, "", []string{"my-custom-model"}), "my-custom-model")
				m, _ = m.Update(dKey())
				if !tui.confirming(m) {
					t.Fatal("expected confirmingDeleteModel after d")
				}
				m, _ = m.Update(tc.key)
				if tui.confirming(m) {
					t.Error("confirmingDeleteModel should be false after cancel")
				}
				got := tui.cfg(m).Providers["dashscope"].Models
				if len(got) != 2 || got[1] != "my-custom-model" {
					t.Errorf("Models = %v, want model unchanged", got)
				}
			})
		}
	}
}

func TestDeleteFlow_OfficialDeleteActiveUserModelClearsCfg(t *testing.T) {
	for _, tui := range deleteFlowTUIs {
		t.Run(tui.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.json")
			m := tui.official(t, configPath, []string{"my-custom-model"})
			cfg := tui.cfg(m)
			cfg.Model = "my-custom-model"
			cfg.Providers["dashscope"] = ProviderEntry{
				Model:  "my-custom-model",
				Models: []string{"qwen3.7-max", "my-custom-model"},
			}
			m = tui.selectModel(t, m, "my-custom-model")

			m, _ = m.Update(dKey())
			m, _ = m.Update(yKey())

			cfg = tui.cfg(m)
			if cfg.Providers["dashscope"].Model != "" {
				t.Errorf("entry.Model = %q, want empty", cfg.Providers["dashscope"].Model)
			}
			if cfg.Model != "" {
				t.Errorf("cfg.Model = %q, want empty", cfg.Model)
			}
		})
	}
}

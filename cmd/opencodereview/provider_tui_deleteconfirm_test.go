// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"path/filepath"
	"testing"
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

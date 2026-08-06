// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestHandleManualFormEnter_Steps drives handleManualFormEnter through every
// manualStep, covering both the guard (empty required field) and advance
// branches that the existing tests leave at ~28%.
func TestHandleManualFormEnter_Steps(t *testing.T) {
	t.Run("URL empty stays", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.manualStep = manualStepURL
		m.manualURLInput.SetValue("")
		out, _ := m.handleManualFormEnter()
		got := out.(providerTUIModel)
		if got.manualStep != manualStepURL {
			t.Errorf("empty URL should stay on URL step, got %d", got.manualStep)
		}
	})

	t.Run("URL non-empty advances to protocol", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.manualStep = manualStepURL
		m.manualURLInput.SetValue("https://api.example.com/v1")
		out, _ := m.handleManualFormEnter()
		got := out.(providerTUIModel)
		if got.manualStep != manualStepProtocol {
			t.Errorf("manualStep = %d, want manualStepProtocol", got.manualStep)
		}
	})

	t.Run("protocol advances to model", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.manualStep = manualStepProtocol
		out, _ := m.handleManualFormEnter()
		got := out.(providerTUIModel)
		if got.manualStep != manualStepModel {
			t.Errorf("manualStep = %d, want manualStepModel", got.manualStep)
		}
	})

	t.Run("model empty stays", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.manualStep = manualStepModel
		m.manualModelInput.SetValue("")
		out, _ := m.handleManualFormEnter()
		got := out.(providerTUIModel)
		if got.manualStep != manualStepModel {
			t.Errorf("empty model should stay on model step, got %d", got.manualStep)
		}
	})

	t.Run("model non-empty advances to auth token", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.manualStep = manualStepModel
		m.manualModelInput.SetValue("gpt-4")
		out, _ := m.handleManualFormEnter()
		got := out.(providerTUIModel)
		if got.manualStep != manualStepAuthToken {
			t.Errorf("manualStep = %d, want manualStepAuthToken", got.manualStep)
		}
	})

	t.Run("auth header invalid sets formError", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.manualStep = manualStepAuthHeader
		m.manualAuthHeaderInput.SetValue("bogus-header")
		out, _ := m.handleManualFormEnter()
		got := out.(providerTUIModel)
		if got.formError == "" {
			t.Error("invalid auth header should set formError")
		}
		if got.manualStep != manualStepAuthHeader {
			t.Errorf("invalid header should stay on auth header step, got %d", got.manualStep)
		}
		if got.confirmed {
			t.Error("invalid header must not confirm")
		}
	})

	t.Run("auth header valid confirms and quits", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.manualStep = manualStepAuthHeader
		m.manualAuthHeaderInput.SetValue("authorization")
		out, cmd := m.handleManualFormEnter()
		got := out.(providerTUIModel)
		if !got.confirmed {
			t.Error("valid auth header should confirm")
		}
		if cmd == nil {
			t.Error("valid auth header should return a quit command")
		}
	})
}

// TestUpdateManualForm_Esc covers the esc branches: on the URL step the form is
// dismissed and inputs reset (with and without an existing config), while later
// steps decrement to the previous step.
func TestUpdateManualForm_Esc(t *testing.T) {
	t.Run("esc on URL step with existingCfg restores values", func(t *testing.T) {
		cfg := &Config{}
		cfg.Llm.URL = "https://saved.example.com"
		cfg.Llm.Model = "saved-model"
		cfg.Llm.AuthToken = "secret"
		m := newProviderTUI(cfg, "")
		m.inManualForm = true
		m.manualStep = manualStepURL
		m.manualURLInput.SetValue("https://scratch")
		out, _ := m.updateManualForm("esc", escKey())
		got := out.(providerTUIModel)
		if got.inManualForm {
			t.Error("esc on URL step should exit manual form")
		}
		if got.manualURLInput.Value() != "https://saved.example.com" {
			t.Errorf("URL not restored: %q", got.manualURLInput.Value())
		}
		if !got.manualTokenMasked {
			t.Error("existing token should be masked after restore")
		}
	})

	t.Run("esc on URL step without existingCfg clears values", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.existingCfg = nil
		m.inManualForm = true
		m.manualStep = manualStepURL
		m.manualURLInput.SetValue("https://scratch")
		out, _ := m.updateManualForm("esc", escKey())
		got := out.(providerTUIModel)
		if got.inManualForm {
			t.Error("esc on URL step should exit manual form")
		}
		if got.manualURLInput.Value() != "" {
			t.Errorf("URL should be cleared, got %q", got.manualURLInput.Value())
		}
	})

	t.Run("esc on later step decrements", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.inManualForm = true
		m.manualStep = manualStepModel
		out, _ := m.updateManualForm("esc", escKey())
		got := out.(providerTUIModel)
		if got.manualStep != manualStepProtocol {
			t.Errorf("esc should decrement to protocol, got %d", got.manualStep)
		}
		if !got.inManualForm {
			t.Error("esc on later step should stay in manual form")
		}
	})
}

// TestUpdateManualForm_ProtocolNav covers the up/down protocol selection on the
// protocol step.
func TestUpdateManualForm_ProtocolNav(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.inManualForm = true
	m.manualStep = manualStepProtocol
	m.manualProtocolIdx = 0

	out, _ := m.updateManualForm("down", tea.KeyPressMsg{Code: tea.KeyDown})
	got := out.(providerTUIModel)
	if got.manualProtocolIdx != 1 {
		t.Errorf("down should advance protocol idx to 1, got %d", got.manualProtocolIdx)
	}

	out, _ = got.updateManualForm("up", tea.KeyPressMsg{Code: tea.KeyUp})
	got = out.(providerTUIModel)
	if got.manualProtocolIdx != 0 {
		t.Errorf("up should return protocol idx to 0, got %d", got.manualProtocolIdx)
	}
}

// TestUpdateManualForm_CtrlC covers the ctrl+c cancel branch.
func TestUpdateManualForm_CtrlC(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.inManualForm = true
	out, cmd := m.updateManualForm("ctrl+c", tea.KeyPressMsg{})
	got := out.(providerTUIModel)
	if !got.cancelled {
		t.Error("ctrl+c should mark cancelled")
	}
	if cmd == nil {
		t.Error("ctrl+c should return quit command")
	}
}

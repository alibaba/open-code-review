// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/alibaba/open-code-review/internal/llm"
)

// TestHandleCustomFormEnter_Steps drives handleCustomFormEnter through the create
// flow's every cpStep, covering both guard and advance branches.
func TestHandleCustomFormEnter_Steps(t *testing.T) {
	t.Run("name empty stays", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.creatingCustom = true
		m.cpStep = cpStepName
		m.cpNameInput.SetValue("")
		out, _ := m.handleCustomFormEnter()
		got := out.(providerTUIModel)
		if got.cpStep != cpStepName {
			t.Errorf("empty name should stay on name step, got %d", got.cpStep)
		}
	})

	t.Run("name taken sets formError", func(t *testing.T) {
		cfg := &Config{
			CustomProviders: map[string]ProviderEntry{
				"dup": {URL: "https://x", Protocol: "openai", Models: []string{"m"}},
			},
		}
		m := newProviderTUI(cfg, "")
		m.creatingCustom = true
		m.cpStep = cpStepName
		m.cpNameInput.SetValue("dup")
		out, _ := m.handleCustomFormEnter()
		got := out.(providerTUIModel)
		if got.formError == "" {
			t.Error("taken name should set formError")
		}
		if got.cpStep != cpStepName {
			t.Errorf("taken name should stay on name step, got %d", got.cpStep)
		}
	})

	t.Run("name valid advances to protocol", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.creatingCustom = true
		m.cpStep = cpStepName
		m.cpNameInput.SetValue("fresh")
		out, _ := m.handleCustomFormEnter()
		got := out.(providerTUIModel)
		if got.cpStep != cpStepProtocol {
			t.Errorf("cpStep = %d, want cpStepProtocol", got.cpStep)
		}
	})

	t.Run("protocol advances to baseURL", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.cpStep = cpStepProtocol
		out, _ := m.handleCustomFormEnter()
		got := out.(providerTUIModel)
		if got.cpStep != cpStepBaseURL {
			t.Errorf("cpStep = %d, want cpStepBaseURL", got.cpStep)
		}
	})

	t.Run("baseURL empty stays", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.cpStep = cpStepBaseURL
		m.cpURLInput.SetValue("")
		out, _ := m.handleCustomFormEnter()
		got := out.(providerTUIModel)
		if got.cpStep != cpStepBaseURL {
			t.Errorf("empty URL should stay on baseURL step, got %d", got.cpStep)
		}
	})

	t.Run("baseURL valid advances to APIKey", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.creatingCustom = true
		m.cpStep = cpStepBaseURL
		m.cpURLInput.SetValue("https://api.example.com")
		out, _ := m.handleCustomFormEnter()
		got := out.(providerTUIModel)
		if got.cpStep != cpStepAPIKey {
			t.Errorf("cpStep = %d, want cpStepAPIKey", got.cpStep)
		}
	})

	t.Run("APIKey advances to authHeader", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.cpStep = cpStepAPIKey
		out, _ := m.handleCustomFormEnter()
		got := out.(providerTUIModel)
		if got.cpStep != cpStepAuthHeader {
			t.Errorf("cpStep = %d, want cpStepAuthHeader", got.cpStep)
		}
	})

	t.Run("authHeader invalid sets formError", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.cpStep = cpStepAuthHeader
		m.cpAuthInput.SetValue("bogus")
		out, _ := m.handleCustomFormEnter()
		got := out.(providerTUIModel)
		if got.formError == "" {
			t.Error("invalid auth header should set formError")
		}
	})

	t.Run("authHeader valid creating saves and enters model step", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		m := newProviderTUI(&Config{}, path)
		m.creatingCustom = true
		m.cpStep = cpStepAuthHeader
		m.cpNameInput.SetValue("newprov")
		m.cpURLInput.SetValue("https://api.example.com")
		m.cpAuthInput.SetValue("authorization")
		out, _ := m.handleCustomFormEnter()
		got := out.(providerTUIModel)
		if !got.savedInSession {
			t.Errorf("create should mark savedInSession; formError=%q", got.formError)
		}
		if got.step != stepModel {
			t.Errorf("after create should enter model step, got %d", got.step)
		}
	})

	t.Run("authHeader valid non-create-non-edit confirms", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.cpStep = cpStepAuthHeader
		m.cpAuthInput.SetValue("")
		out, cmd := m.handleCustomFormEnter()
		got := out.(providerTUIModel)
		if !got.confirmed {
			t.Error("valid auth header (no create/edit) should confirm")
		}
		if cmd == nil {
			t.Error("should return quit command")
		}
	})
}

// TestUpdateCustomProviderForm_Esc covers esc on the name step (full reset) and
// on a later step (decrement / edit APIKey→BaseURL), plus ctrl+c.
func TestUpdateCustomProviderForm_Esc(t *testing.T) {
	t.Run("esc on name step resets", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.creatingCustom = true
		m.cpStep = cpStepName
		m.cpNameInput.SetValue("scratch")
		out, _ := m.updateCustomProviderForm("esc", escKey())
		got := out.(providerTUIModel)
		if got.creatingCustom {
			t.Error("esc on name step should cancel creatingCustom")
		}
		if got.cpNameInput.Value() != "" {
			t.Errorf("name input should be cleared, got %q", got.cpNameInput.Value())
		}
	})

	t.Run("esc on later step decrements", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.cpStep = cpStepBaseURL
		out, _ := m.updateCustomProviderForm("esc", escKey())
		got := out.(providerTUIModel)
		if got.cpStep != cpStepProtocol {
			t.Errorf("esc should decrement to protocol, got %d", got.cpStep)
		}
	})

	t.Run("esc on editing APIKey jumps to baseURL", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		m.editingCustom = true
		m.cpStep = cpStepAPIKey
		out, _ := m.updateCustomProviderForm("esc", escKey())
		got := out.(providerTUIModel)
		if got.cpStep != cpStepBaseURL {
			t.Errorf("editing esc on APIKey should go to baseURL, got %d", got.cpStep)
		}
	})

	t.Run("ctrl+c cancels", func(t *testing.T) {
		m := newProviderTUI(&Config{}, "")
		out, cmd := m.updateCustomProviderForm("ctrl+c", tea.KeyPressMsg{})
		got := out.(providerTUIModel)
		if !got.cancelled || cmd == nil {
			t.Error("ctrl+c should cancel and quit")
		}
	})
}

// TestCustomFormEndsAtProtocolForBedrock covers the Custom form's ambient
// branch. Bedrock has no url, no api key and no auth header to collect, so
// walking those three steps would ask for values the client never reads and
// store them as dead config. The form finishes at the protocol step instead.
func TestCustomFormEndsAtProtocolForBedrock(t *testing.T) {
	setup := func(t *testing.T) providerTUIModel {
		t.Helper()
		cfg := &Config{}
		m := newProviderTUI(cfg, filepath.Join(t.TempDir(), "c.json"))
		m.activeTab = tabCustom
		m.creatingCustom = true
		m.cpStep = cpStepProtocol
		m.cpProtocolIdx = cpProtocolIndex(llm.ProtocolAnthropicBedrock)
		m.cpNameInput.SetValue("bedrock-eu")
		return m
	}

	t.Run("protocol step saves instead of advancing", func(t *testing.T) {
		m := setup(t)
		out, _ := m.handleCustomFormEnter()
		got := out.(providerTUIModel)
		if got.creatingCustom {
			t.Error("still creating after the protocol step; the form did not finish")
		}
		entry, ok := got.existingCfg.CustomProviders["bedrock-eu"]
		if !ok {
			t.Fatal("provider was not written to config")
		}
		if entry.Protocol != llm.ProtocolAnthropicBedrock {
			t.Errorf("Protocol = %q, want %q", entry.Protocol, llm.ProtocolAnthropicBedrock)
		}
		if entry.URL != "" || entry.APIKey != "" || entry.AuthHeader != "" {
			t.Errorf("entry carries token-protocol fields: url=%q api_key=%q auth_header=%q", entry.URL, entry.APIKey, entry.AuthHeader)
		}
	})

	t.Run("a token protocol still walks to the url step", func(t *testing.T) {
		m := setup(t)
		m.cpProtocolIdx = cpProtocolIndex(llm.ProtocolOpenAIChatCompletions)
		out, _ := m.handleCustomFormEnter()
		got := out.(providerTUIModel)
		if got.cpStep != cpStepBaseURL {
			t.Errorf("cpStep = %d, want cpStepBaseURL", got.cpStep)
		}
	})

	t.Run("values left by a previous protocol are dropped", func(t *testing.T) {
		m := setup(t)
		m.cpURLInput.SetValue("https://stale.invalid/v1")
		m.cpAuthInput.SetValue("X-Api-Key")
		m.apiKeyInput.SetValue("sk-stale")
		r := m.result()
		if r.url != "" || r.apiKey != "" || r.authHeader != "" {
			t.Errorf("result kept stale fields: url=%q apiKey=%q authHeader=%q", r.url, r.apiKey, r.authHeader)
		}
	})
}

// TestUpdateCustomProviderForm_ProtocolNav covers up/down protocol selection.
func TestUpdateCustomProviderForm_ProtocolNav(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	m.cpStep = cpStepProtocol
	m.cpProtocolIdx = 0
	out, _ := m.updateCustomProviderForm("down", tea.KeyPressMsg{Code: tea.KeyDown})
	got := out.(providerTUIModel)
	if got.cpProtocolIdx != 1 {
		t.Errorf("down should advance protocol idx, got %d", got.cpProtocolIdx)
	}
	out, _ = got.updateCustomProviderForm("up", tea.KeyPressMsg{Code: tea.KeyUp})
	got = out.(providerTUIModel)
	if got.cpProtocolIdx != 0 {
		t.Errorf("up should decrement protocol idx, got %d", got.cpProtocolIdx)
	}
}

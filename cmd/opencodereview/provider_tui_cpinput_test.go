// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alibaba/open-code-review/internal/llm"
)

// TestIsUserEditMsg covers the key/paste true branches and the default false.
func TestIsUserEditMsg(t *testing.T) {
	if !isUserEditMsg(tea.KeyPressMsg{Code: 'a', Text: "a"}) {
		t.Error("KeyPressMsg should be a user edit")
	}
	if !isUserEditMsg(tea.PasteMsg{Content: "x"}) {
		t.Error("PasteMsg should be a user edit")
	}
	if isUserEditMsg(tea.WindowSizeMsg{Width: 10, Height: 10}) {
		t.Error("WindowSizeMsg should not be a user edit")
	}
}

// TestOfficialAPIKeyRequiredError covers the with-EnvVar and without branches.
func TestOfficialAPIKeyRequiredError(t *testing.T) {
	got := officialAPIKeyRequiredError(llm.Provider{EnvVar: "MY_KEY"})
	if got != "API key is required (or set $MY_KEY)" {
		t.Errorf("got %q, want mention of $MY_KEY", got)
	}
	if got := officialAPIKeyRequiredError(llm.Provider{}); got != "API key is required" {
		t.Errorf("got %q, want generic message", got)
	}
}

// TestOfficialProviderEnvKeySet covers empty EnvVar, unset var, and set var.
func TestOfficialProviderEnvKeySet(t *testing.T) {
	if officialProviderEnvKeySet(llm.Provider{}) {
		t.Error("empty EnvVar should report not set")
	}
	if officialProviderEnvKeySet(llm.Provider{EnvVar: "OCR_TEST_UNSET_ENVKEY_XYZ"}) {
		t.Error("unset env var should report not set")
	}
	t.Setenv("OCR_TEST_SET_ENVKEY", "value")
	if !officialProviderEnvKeySet(llm.Provider{EnvVar: "OCR_TEST_SET_ENVKEY"}) {
		t.Error("set env var should report set")
	}
}

// TestFindCustomIdx covers a hit and a miss.
func TestFindCustomIdx(t *testing.T) {
	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"cp": {URL: "https://x.example", Protocol: "openai"},
		},
	}
	m := newProviderTUI(cfg, filepath.Join(t.TempDir(), "config.json"))
	m.customProviders = collectCustomProviders(cfg)
	if idx := m.findCustomIdx("cp"); idx < 0 {
		t.Error("existing provider should be found")
	}
	if idx := m.findCustomIdx("nope"); idx != -1 {
		t.Errorf("missing provider should return -1, got %d", idx)
	}
}

// TestPassThroughCPInput drives each custom-provider input step and confirms a
// key press clears formError, including the masked-API-key replacement branch.
func TestPassThroughCPInput(t *testing.T) {
	newModel := func() providerTUIModel {
		cfg := &Config{}
		m := newProviderTUI(cfg, filepath.Join(t.TempDir(), "config.json"))
		m.formError = "stale error"
		return m
	}
	key := tea.KeyPressMsg{Code: 'a', Text: "a"}

	for _, step := range []struct {
		name string
		step customProviderStep
	}{
		{"name", cpStepName},
		{"baseURL", cpStepBaseURL},
		{"apiKey", cpStepAPIKey},
		{"authHeader", cpStepAuthHeader},
	} {
		t.Run(step.name, func(t *testing.T) {
			m := newModel()
			m.cpStep = step.step
			out, _ := m.passThroughCPInput(key)
			if out.(providerTUIModel).formError != "" {
				t.Error("a key press should clear formError")
			}
		})
	}

	t.Run("masked api key begins replace on edit", func(t *testing.T) {
		m := newModel()
		m.cpStep = cpStepAPIKey
		m.apiKeyMasked = true
		m.apiKeyOriginal = "secret"
		out, _ := m.passThroughCPInput(key)
		if out.(providerTUIModel).apiKeyMasked {
			t.Error("editing a masked API key should begin replacement")
		}
	})
}

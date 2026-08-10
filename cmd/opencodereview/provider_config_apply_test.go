// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestApplyOfficialProviderConfig_Validation covers the pre-save validation
// branches (empty provider, empty model, missing API key) that reject the
// request before any network connection test runs.
func TestApplyOfficialProviderConfig_Validation(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")

	t.Run("empty provider rejected", func(t *testing.T) {
		err := applyOfficialProviderConfig(configPath, &Config{}, providerTUIResult{})
		if err == nil || !strings.Contains(err.Error(), "required") {
			t.Fatalf("got %v, want provider/model required error", err)
		}
	})

	t.Run("empty model rejected", func(t *testing.T) {
		err := applyOfficialProviderConfig(configPath, &Config{}, providerTUIResult{provider: "openai"})
		if err == nil || !strings.Contains(err.Error(), "required") {
			t.Fatalf("got %v, want provider/model required error", err)
		}
	})

	t.Run("missing API key for non-preset provider rejected", func(t *testing.T) {
		err := applyOfficialProviderConfig(configPath, &Config{}, providerTUIResult{
			provider: "not-a-preset-provider",
			model:    "m",
		})
		if err == nil || !strings.Contains(err.Error(), "API key is required") {
			t.Fatalf("got %v, want API-key-required error", err)
		}
	})
}

// TestSetCustomProviderValue covers the malformed-key rejection and the
// success path that materializes a custom provider entry.
func TestSetCustomProviderValue(t *testing.T) {
	t.Run("malformed key rejected", func(t *testing.T) {
		if err := setCustomProviderValue(&Config{}, "custom_providers.onlyname", "v"); err == nil {
			t.Error("expected error for key missing a field segment")
		}
	})

	t.Run("success sets a custom provider field", func(t *testing.T) {
		cfg := &Config{}
		if err := setCustomProviderValue(cfg, "custom_providers.cp.url", "https://x.example"); err != nil {
			t.Fatalf("setCustomProviderValue: %v", err)
		}
		if cfg.CustomProviders["cp"].URL != "https://x.example" {
			t.Errorf("custom provider url not set: %+v", cfg.CustomProviders)
		}
	})
}

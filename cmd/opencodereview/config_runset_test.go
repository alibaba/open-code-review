// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"strings"
	"testing"
)

// TestRunConfigSetPersists drives runConfigSet end-to-end (HOME points at a temp
// dir) covering the success path and the API-key masking branch.
func TestRunConfigSetPersists(t *testing.T) {
	setTestHome(t, t.TempDir())

	t.Run("plain value success", func(t *testing.T) {
		out := captureStdout(t, func() {
			if err := runConfigSet("language", "zh"); err != nil {
				t.Fatalf("runConfigSet: %v", err)
			}
		})
		if !strings.Contains(out, "Set language = zh") {
			t.Errorf("stdout = %q, want confirmation", out)
		}
		configPath, err := defaultConfigPath()
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := loadOrCreateConfig(configPath)
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if cfg.Language != "zh" {
			t.Errorf("language = %q, want zh", cfg.Language)
		}
	})

	t.Run("api key value is masked in output", func(t *testing.T) {
		out := captureStdout(t, func() {
			if err := runConfigSet("providers.openai.api_key", "sk-supersecretvalue"); err != nil {
				t.Fatalf("runConfigSet: %v", err)
			}
		})
		if strings.Contains(out, "sk-supersecretvalue") {
			t.Errorf("stdout leaks the raw API key: %q", out)
		}
		if !strings.Contains(out, "Set providers.openai.api_key") {
			t.Errorf("stdout = %q, want confirmation line", out)
		}
	})

	t.Run("invalid key returns error", func(t *testing.T) {
		if err := runConfigSet("no_such_key", "x"); err == nil {
			t.Error("expected error for unknown config key")
		}
	})
}

// TestRunConfigUnsetPaths drives runConfigUnset across its dispatch branches.
func TestRunConfigUnsetPaths(t *testing.T) {
	t.Run("unset active provider clears provider and model", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		configPath, err := defaultConfigPath()
		if err != nil {
			t.Fatal(err)
		}
		if err := saveConfig(configPath, &Config{Provider: "openai", Model: "gpt-4"}); err != nil {
			t.Fatalf("save: %v", err)
		}
		out := captureStdout(t, func() {
			if err := runConfigUnset("provider"); err != nil {
				t.Fatalf("runConfigUnset: %v", err)
			}
		})
		if !strings.Contains(out, "Cleared active provider") {
			t.Errorf("stdout = %q", out)
		}
		cfg, err := loadOrCreateConfig(configPath)
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if cfg.Provider != "" || cfg.Model != "" {
			t.Errorf("provider/model not cleared: %q/%q", cfg.Provider, cfg.Model)
		}
	})

	t.Run("unset custom provider", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		configPath, err := defaultConfigPath()
		if err != nil {
			t.Fatal(err)
		}
		cfg := &Config{
			CustomProviders: map[string]ProviderEntry{
				"cp": {URL: "https://x.example", Protocol: "openai"},
			},
		}
		if err := saveConfig(configPath, cfg); err != nil {
			t.Fatalf("save: %v", err)
		}
		out := captureStdout(t, func() {
			if err := runConfigUnset("custom_providers.cp"); err != nil {
				t.Fatalf("runConfigUnset: %v", err)
			}
		})
		if !strings.Contains(out, "Deleted custom provider") {
			t.Errorf("stdout = %q", out)
		}
	})

	t.Run("unset mcp server", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		configPath, err := defaultConfigPath()
		if err != nil {
			t.Fatal(err)
		}
		cfg := &Config{
			MCPServers: map[string]MCPServerConfig{
				"srv": {Type: "stdio", Command: "echo"},
			},
		}
		if err := saveConfig(configPath, cfg); err != nil {
			t.Fatalf("save: %v", err)
		}
		out := captureStdout(t, func() {
			if err := runConfigUnset("mcp_servers.srv"); err != nil {
				t.Fatalf("runConfigUnset: %v", err)
			}
		})
		if !strings.Contains(out, "srv") {
			t.Errorf("stdout = %q", out)
		}
	})

	t.Run("malformed key returns error", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		if err := runConfigUnset("custom_providers"); err == nil {
			t.Error("expected error for key without a name segment")
		}
	})

	t.Run("unknown prefix returns error", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		if err := runConfigUnset("bogus.name"); err == nil {
			t.Error("expected error for unknown unset prefix")
		}
	})
}

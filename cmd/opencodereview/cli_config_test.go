// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

// TestConfigRoundTripKeepsCLISettings guards the same silent-loss failure the AWS
// fields had: config is unmarshalled into Config and marshalled back on every
// write, so a cli_path/cli_args missing from ProviderEntry would be dropped from
// a hand-written file on the first config command.
func TestConfigRoundTripKeepsCLISettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := `{
  "provider": "claude-code",
  "model": "claude-sonnet-5",
  "providers": {
    "claude-code": { "cli_path": "/opt/claude", "cli_args": ["--add-dir", "/x"] }
  }
}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadOrCreateConfig(path)
	if err != nil {
		t.Fatalf("loadOrCreateConfig: %v", err)
	}
	// A round trip through an unrelated set command must keep the CLI fields.
	if err := setConfigValue(cfg, "model", "claude-opus-5"); err != nil {
		t.Fatalf("set model: %v", err)
	}
	if err := saveConfig(path, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	reloaded, err := loadOrCreateConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	entry := reloaded.Providers["claude-code"]
	if entry.CLIPath != "/opt/claude" {
		t.Errorf("CLIPath = %q after round trip, want /opt/claude", entry.CLIPath)
	}
	if strings.Join(entry.CLIArgs, ",") != "--add-dir,/x" {
		t.Errorf("CLIArgs = %v after round trip, want [--add-dir /x]", entry.CLIArgs)
	}

	// The resolver reads the same file independently; assert the written JSON
	// still carries the keys it looks for.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal written config: %v", err)
	}
	providers, _ := raw["providers"].(map[string]any)
	entryRaw, _ := providers["claude-code"].(map[string]any)
	if entryRaw["cli_path"] != "/opt/claude" {
		t.Errorf("written JSON cli_path = %v, want /opt/claude", entryRaw["cli_path"])
	}
	if args, _ := entryRaw["cli_args"].([]any); len(args) != 2 {
		t.Errorf("written JSON cli_args = %v, want two elements", entryRaw["cli_args"])
	}
}

func TestSetProviderValueCLISettings(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr string
		check   func(*testing.T, *Config)
	}{
		{
			name:  "cli_path on a CLI provider is trimmed and stored",
			key:   "providers.claude-code.cli_path",
			value: "  /opt/claude  ",
			check: func(t *testing.T, cfg *Config) {
				if got := cfg.Providers["claude-code"].CLIPath; got != "/opt/claude" {
					t.Errorf("CLIPath = %q, want /opt/claude", got)
				}
			},
		},
		{
			name:  "cli_args accepts a JSON array of strings",
			key:   "providers.codex.cli_args",
			value: `["--add-dir","/x"]`,
			check: func(t *testing.T, cfg *Config) {
				if got := strings.Join(cfg.Providers["codex"].CLIArgs, ","); got != "--add-dir,/x" {
					t.Errorf("CLIArgs = %v, want [--add-dir /x]", cfg.Providers["codex"].CLIArgs)
				}
			},
		},
		{
			name:    "cli_args rejects a non-array string",
			key:     "providers.claude-code.cli_args",
			value:   "--add-dir /x",
			wantErr: "invalid JSON array",
		},
		{
			// Storing it would be dead config that reads as applied.
			name:    "cli_path rejected on a key-based provider",
			key:     "providers.anthropic.cli_path",
			value:   "/opt/claude",
			wantErr: "does not apply to provider",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{}
			err := setProviderValue(cfg, tc.key, tc.value)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("setProviderValue(%q, %q) = nil, want error containing %q", tc.key, tc.value, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("setProviderValue(%q, %q): %v", tc.key, tc.value, err)
			}
			tc.check(t, cfg)
		})
	}
}

// TestSetCustomProviderCLISettingsFollowProtocol covers the custom-provider path:
// cli_path/cli_args are meaningful only once the entry speaks a CLI protocol, so
// the order of the two set commands matters.
func TestSetCustomProviderCLISettingsFollowProtocol(t *testing.T) {
	cfg := &Config{}
	if err := setCustomProviderValue(cfg, "custom_providers.mine.cli_args", `["--x"]`); err == nil {
		t.Fatal("cli_args accepted before a protocol was set; want an error")
	}

	if err := setCustomProviderValue(cfg, "custom_providers.mine.protocol", llm.ProtocolCodexCLI); err != nil {
		t.Fatalf("set protocol: %v", err)
	}
	if err := setCustomProviderValue(cfg, "custom_providers.mine.cli_args", `["--x"]`); err != nil {
		t.Fatalf("set cli_args after protocol: %v", err)
	}
	if got := strings.Join(cfg.CustomProviders["mine"].CLIArgs, ","); got != "--x" {
		t.Errorf("CLIArgs = %v, want [--x]", cfg.CustomProviders["mine"].CLIArgs)
	}
}

// TestSetProtocolClearsStaleCLISettings mirrors the AWS reverse-order case:
// cli_path/cli_args set while the entry is a CLI protocol, then the entry switched
// to a protocol that does not read them must clear both and warn.
func TestSetProtocolClearsStaleCLISettings(t *testing.T) {
	cfg := &Config{}
	if err := setCustomProviderValue(cfg, "custom_providers.mine.protocol", llm.ProtocolCodexCLI); err != nil {
		t.Fatalf("set protocol: %v", err)
	}
	if err := setCustomProviderValue(cfg, "custom_providers.mine.cli_path", "/opt/codex"); err != nil {
		t.Fatalf("set cli_path: %v", err)
	}
	if err := setCustomProviderValue(cfg, "custom_providers.mine.cli_args", `["--x"]`); err != nil {
		t.Fatalf("set cli_args: %v", err)
	}

	stderr := captureStderr(t, func() {
		if err := setCustomProviderValue(cfg, "custom_providers.mine.protocol", "openai"); err != nil {
			t.Fatalf("set protocol: %v", err)
		}
	})

	entry := cfg.CustomProviders["mine"]
	if entry.CLIPath != "" || len(entry.CLIArgs) != 0 {
		t.Errorf("CLIPath/CLIArgs = %q/%v after switching to openai, want both cleared", entry.CLIPath, entry.CLIArgs)
	}
	if !strings.Contains(stderr, "WARNING") || !strings.Contains(stderr, "cli_path") {
		t.Errorf("stderr = %q, want a WARNING naming cli_path", stderr)
	}
}

// TestSetLlmProtocolRejectsCLIProtocols pins the other half of the contract: the
// llm block is one url plus one token, with nowhere to run a local binary, so a
// CLI protocol is refused where it is typed.
func TestSetLlmProtocolRejectsCLIProtocols(t *testing.T) {
	for _, proto := range []string{llm.ProtocolClaudeCLI, llm.ProtocolCodexCLI} {
		cfg := &Config{}
		err := setConfigValue(cfg, "llm.protocol", proto)
		if err == nil {
			t.Fatalf("llm.protocol accepted %q; want an error", proto)
		}
		if !strings.Contains(err.Error(), "claude-code") {
			t.Errorf("error %q does not point at `ocr config set provider claude-code`", err)
		}
		if cfg.Llm.Protocol != "" {
			t.Errorf("Llm.Protocol = %q, want it left unset after the rejection", cfg.Llm.Protocol)
		}
	}
}

// TestCheckAPIKeyRequirementCLIPresets: the claude-code and codex presets carry
// AmbientAuth, so a save with no api_key is allowed.
func TestCheckAPIKeyRequirementCLIPresets(t *testing.T) {
	for _, name := range []string{"claude-code", "codex"} {
		preset, ok := llm.LookupProvider(name)
		if !ok {
			t.Fatalf("%s preset not registered", name)
		}
		if err := checkAPIKeyRequirement(name, "", "", preset, true); err != nil {
			t.Errorf("ambient CLI provider %q with no api_key = %v, want nil", name, err)
		}
	}
}

// TestApplyOfficialProviderConfigCLINoKey: selecting claude-code and a model
// through the official path saves an entry with no api_key and sets the active
// provider/model.
func TestApplyOfficialProviderConfigCLINoKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := &Config{}
	res := providerTUIResult{provider: "claude-code", model: "claude-sonnet-5"}
	if err := applyOfficialProviderConfig(path, cfg, res); err != nil {
		t.Fatalf("applyOfficialProviderConfig: %v", err)
	}
	if cfg.Provider != "claude-code" {
		t.Errorf("Provider = %q, want claude-code", cfg.Provider)
	}
	if cfg.Model != "claude-sonnet-5" {
		t.Errorf("Model = %q, want claude-sonnet-5", cfg.Model)
	}
	if got := cfg.Providers["claude-code"].APIKey; got != "" {
		t.Errorf("APIKey = %q, want empty for an ambient CLI provider", got)
	}
}

// TestSetProtocolToCLIClearsStaleCredentials: switching a provider from an
// HTTP protocol to a CLI protocol must clear URL, APIKey, and AuthHeader
// so stale credentials do not persist in the config file.
func TestSetProtocolToCLIClearsStaleCredentials(t *testing.T) {
	for _, proto := range []string{llm.ProtocolClaudeCLI, llm.ProtocolCodexCLI} {
		t.Run(proto, func(t *testing.T) {
			cfg := &Config{}
			if err := setCustomProviderValue(cfg, "custom_providers.mine.protocol", "openai"); err != nil {
				t.Fatalf("set initial protocol: %v", err)
			}
			if err := setCustomProviderValue(cfg, "custom_providers.mine.url", "https://api.example.com/v1"); err != nil {
				t.Fatalf("set url: %v", err)
			}
			if err := setCustomProviderValue(cfg, "custom_providers.mine.api_key", "sk-secret-123"); err != nil {
				t.Fatalf("set api_key: %v", err)
			}
			if err := setCustomProviderValue(cfg, "custom_providers.mine.auth_header", "x-api-key"); err != nil {
				t.Fatalf("set auth_header: %v", err)
			}

			stderr := captureStderr(t, func() {
				if err := setCustomProviderValue(cfg, "custom_providers.mine.protocol", proto); err != nil {
					t.Fatalf("set protocol to %s: %v", proto, err)
				}
			})

			entry := cfg.CustomProviders["mine"]
			if entry.URL != "" {
				t.Errorf("URL = %q after switching to %s, want empty", entry.URL, proto)
			}
			if entry.APIKey != "" {
				t.Errorf("APIKey = %q after switching to %s, want empty", entry.APIKey, proto)
			}
			if entry.AuthHeader != "" {
				t.Errorf("AuthHeader = %q after switching to %s, want empty", entry.AuthHeader, proto)
			}
			if !strings.Contains(stderr, "Cleared") {
				t.Errorf("stderr = %q, want a message containing 'Cleared'", stderr)
			}
		})
	}
}

// TestSetProtocolToCLINoMessageWhenClean: switching to a CLI protocol when
// no credentials are set should not print a clearing message.
func TestSetProtocolToCLINoMessageWhenClean(t *testing.T) {
	cfg := &Config{}
	if err := setCustomProviderValue(cfg, "custom_providers.mine.protocol", "openai"); err != nil {
		t.Fatalf("set initial protocol: %v", err)
	}

	stderr := captureStderr(t, func() {
		if err := setCustomProviderValue(cfg, "custom_providers.mine.protocol", llm.ProtocolClaudeCLI); err != nil {
			t.Fatalf("set protocol: %v", err)
		}
	})

	if strings.Contains(stderr, "Cleared") {
		t.Errorf("stderr = %q, want no clearing message when no credentials were present", stderr)
	}
}

// TestProviderTUIClaudeCodeOfficialSkipsAPIKeyStep mirrors the bedrock official
// flow: claude-code has no key to collect, so the model step is the last one.
func TestProviderTUIClaudeCodeOfficialSkipsAPIKeyStep(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	idx := -1
	for i, p := range m.providers {
		if p.Name == "claude-code" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("claude-code not offered in the official provider list")
	}
	m.officialIdx = idx

	result, _ := m.Update(enterKey())
	atModel := result.(providerTUIModel)
	if atModel.step != stepModel {
		t.Fatalf("after Enter on provider, step = %d, want %d (stepModel)", atModel.step, stepModel)
	}

	result, cmd := atModel.Update(enterKey())
	done := result.(providerTUIModel)
	if done.step == stepAPIKey {
		t.Error("ambient CLI provider advanced to stepAPIKey; want the model step to be final")
	}
	if !done.confirmed {
		t.Error("confirmed = false; want the selection confirmed from the model step")
	}
	if cmd == nil {
		t.Error("no command returned; want tea.Quit")
	}
	res := done.result()
	if res.provider != "claude-code" {
		t.Errorf("result provider = %q, want claude-code", res.provider)
	}
	if res.apiKey != "" {
		t.Errorf("result apiKey = %q, want empty for an ambient CLI provider", res.apiKey)
	}
}

// TestProviderTUICustomCodexCLISkipsURLKeyAuth: choosing codex-cli in the custom
// form ends at the protocol step and saves an entry with empty url/api_key/auth.
func TestProviderTUICustomCodexCLISkipsURLKeyAuth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	m := newProviderTUI(&Config{}, path)
	m.activeTab = tabCustom
	m.creatingCustom = true
	m.cpStep = cpStepProtocol
	m.cpNameInput.SetValue("mycodex")
	m.cpProtocolIdx = cpProtocolIndex(llm.ProtocolCodexCLI)

	result, _ := m.Update(enterKey())
	done := result.(providerTUIModel)
	if done.cpStep == cpStepBaseURL || done.cpStep == cpStepAPIKey || done.cpStep == cpStepAuthHeader {
		t.Errorf("custom form advanced to a credential step (cpStep=%d) for a CLI protocol; want it to finish at the protocol step", done.cpStep)
	}

	reloaded, err := loadOrCreateConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	entry, ok := reloaded.CustomProviders["mycodex"]
	if !ok {
		t.Fatal("custom provider mycodex not saved")
	}
	if entry.Protocol != llm.ProtocolCodexCLI {
		t.Errorf("Protocol = %q, want codex-cli", entry.Protocol)
	}
	if entry.URL != "" || entry.APIKey != "" || entry.AuthHeader != "" {
		t.Errorf("entry url/key/auth = %q/%q/%q, want all empty for a CLI protocol", entry.URL, entry.APIKey, entry.AuthHeader)
	}
}

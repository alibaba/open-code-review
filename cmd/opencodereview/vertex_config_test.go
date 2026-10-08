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

// TestConfigRoundTripKeepsGCPSettings is the vertex counterpart to
// TestConfigRoundTripKeepsAWSSettings: config is unmarshalled into Config and
// marshalled back on every write, so a field missing from ProviderEntry is
// silently dropped from a hand-written file the first time any config command
// runs.
func TestConfigRoundTripKeepsGCPSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := `{
  "provider": "vertex",
  "model": "claude-sonnet-5",
  "providers": {
    "vertex": { "gcp_region": "us-east5", "gcp_project": "example-project" }
  }
}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadOrCreateConfig(path)
	if err != nil {
		t.Fatalf("loadOrCreateConfig: %v", err)
	}
	if err := saveConfig(path, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	reloaded, err := loadOrCreateConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	entry := reloaded.Providers["vertex"]
	if entry.GCPRegion != "us-east5" {
		t.Errorf("GCPRegion = %q after round trip, want us-east5", entry.GCPRegion)
	}
	if entry.GCPProject != "example-project" {
		t.Errorf("GCPProject = %q after round trip, want example-project", entry.GCPProject)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal written config: %v", err)
	}
	providers, _ := raw["providers"].(map[string]any)
	vertexEntry, _ := providers["vertex"].(map[string]any)
	if vertexEntry["gcp_region"] != "us-east5" || vertexEntry["gcp_project"] != "example-project" {
		t.Errorf("written JSON = %v, want gcp_region and gcp_project preserved", vertexEntry)
	}
}

func TestSetProviderValueGCPSettings(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr string
		check   func(*testing.T, *Config)
	}{
		{
			name:  "region on an ambient provider",
			key:   "providers.vertex.gcp_region",
			value: "us-east5",
			check: func(t *testing.T, cfg *Config) {
				if got := cfg.Providers["vertex"].GCPRegion; got != "us-east5" {
					t.Errorf("GCPRegion = %q, want us-east5", got)
				}
			},
		},
		{
			name:  "project is trimmed",
			key:   "providers.vertex.gcp_project",
			value: "  example-project  ",
			check: func(t *testing.T, cfg *Config) {
				if got := cfg.Providers["vertex"].GCPProject; got != "example-project" {
					t.Errorf("GCPProject = %q, want example-project", got)
				}
			},
		},
		{
			name:  "empty value hands the decision back to the ambient chain",
			key:   "providers.vertex.gcp_project",
			value: "",
			check: func(t *testing.T, cfg *Config) {
				if got := cfg.Providers["vertex"].GCPProject; got != "" {
					t.Errorf("GCPProject = %q, want empty", got)
				}
			},
		},
		{
			// Storing it would be dead config that reads as applied.
			name:    "rejected on a key-based provider",
			key:     "providers.anthropic.gcp_region",
			value:   "us-east5",
			wantErr: "does not apply to provider",
		},
		{
			// The other ambient-auth preset: gcp_region/gcp_project mean
			// nothing to bedrock, which reads aws_region/aws_profile instead.
			// This is the regression test for providerAcceptsGCPSettings
			// checking the preset's protocol, not just AmbientAuth — both
			// presets set AmbientAuth, and the field would otherwise leak
			// across them.
			name:    "rejected on the other ambient-auth preset",
			key:     "providers.bedrock.gcp_region",
			value:   "us-east5",
			wantErr: "does not apply to provider",
		},
		{
			name:    "whitespace inside the value is rejected",
			key:     "providers.vertex.gcp_region",
			value:   "us east 5",
			wantErr: "contains whitespace",
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

// TestAWSSettingsRejectedOnVertexPreset and
// TestGCPSettingsRejectedOnBedrockPreset are the two-directional regression
// test for the bug fixed alongside vertex support: providerAcceptsAWSSettings
// used to gate on AmbientAuth alone, which was a correct discriminator only
// while bedrock was the sole ambient-auth preset. With vertex also
// AmbientAuth, that check would have let aws_region apply to a vertex
// provider (and gcp_region apply to a bedrock provider) with no error and no
// effect.
func TestAWSSettingsRejectedOnVertexPreset(t *testing.T) {
	cfg := &Config{}
	err := setProviderValue(cfg, "providers.vertex.aws_region", "us-west-2")
	if err == nil {
		t.Fatal("aws_region accepted on the vertex preset; want an error")
	}
	if !strings.Contains(err.Error(), "does not apply to provider") {
		t.Errorf("error = %q, want it to explain the field does not apply", err)
	}
}

func TestGCPSettingsRejectedOnBedrockPreset(t *testing.T) {
	cfg := &Config{}
	err := setProviderValue(cfg, "providers.bedrock.gcp_region", "us-east5")
	if err == nil {
		t.Fatal("gcp_region accepted on the bedrock preset; want an error")
	}
	if !strings.Contains(err.Error(), "does not apply to provider") {
		t.Errorf("error = %q, want it to explain the field does not apply", err)
	}
}

// TestSetCustomProviderGCPSettingsFollowProtocol covers the custom-provider
// path: gcp_* is meaningful there only once the entry speaks the vertex
// protocol, so the order of the two set commands matters and the error has to
// say why.
func TestSetCustomProviderGCPSettingsFollowProtocol(t *testing.T) {
	cfg := &Config{}
	if err := setCustomProviderValue(cfg, "custom_providers.mine.gcp_region", "us-east5"); err == nil {
		t.Fatal("gcp_region accepted before a protocol was set; want an error")
	}

	if err := setCustomProviderValue(cfg, "custom_providers.mine.protocol", llm.ProtocolAnthropicVertex); err != nil {
		t.Fatalf("set protocol: %v", err)
	}
	if err := setCustomProviderValue(cfg, "custom_providers.mine.gcp_region", "us-east5"); err != nil {
		t.Fatalf("set gcp_region after protocol: %v", err)
	}
	if got := cfg.CustomProviders["mine"].GCPRegion; got != "us-east5" {
		t.Errorf("GCPRegion = %q, want us-east5", got)
	}
}

// TestSetProtocolClearsStaleGCPSettings mirrors TestSetProtocolClearsStaleAWSSettings:
// gcp_region/gcp_project set first while the entry is still vertex, then the
// entry switched to a protocol that does not read them.
func TestSetProtocolClearsStaleGCPSettings(t *testing.T) {
	cfg := &Config{}
	if err := setProviderValue(cfg, "providers.vertex.gcp_region", "us-east5"); err != nil {
		t.Fatalf("set gcp_region: %v", err)
	}
	if err := setProviderValue(cfg, "providers.vertex.gcp_project", "example-project"); err != nil {
		t.Fatalf("set gcp_project: %v", err)
	}

	stderr := captureStderr(t, func() {
		if err := setProviderValue(cfg, "providers.vertex.protocol", "openai"); err != nil {
			t.Fatalf("set protocol: %v", err)
		}
	})

	entry := cfg.Providers["vertex"]
	if entry.GCPRegion != "" || entry.GCPProject != "" {
		t.Errorf("GCPRegion/GCPProject = %q/%q after switching to openai, want both cleared", entry.GCPRegion, entry.GCPProject)
	}
	if !strings.Contains(stderr, "WARNING") || !strings.Contains(stderr, "gcp_region") {
		t.Errorf("stderr = %q, want a WARNING naming gcp_region", stderr)
	}
}

// TestSetLlmProtocolRejectsVertex is TestSetLlmProtocolRejectsBedrock for
// vertex: the llm block is one url plus one token, with nowhere to put a
// region or a project, so the value is refused where it is typed.
func TestSetLlmProtocolRejectsVertex(t *testing.T) {
	cfg := &Config{}
	err := setConfigValue(cfg, "llm.protocol", llm.ProtocolAnthropicVertex)
	if err == nil {
		t.Fatal("llm.protocol accepted anthropic-vertex; want an error")
	}
	for _, want := range []string{"gcp_region", "provider vertex"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if cfg.Llm.Protocol != "" {
		t.Errorf("Llm.Protocol = %q, want it left unset after the rejection", cfg.Llm.Protocol)
	}
}

// TestProviderTUIVertexSkipsAPIKeyStep is
// TestProviderTUIAmbientProviderSkipsAPIKeyStep for vertex: the Official tab's
// ambient-auth handling is generic over AmbientAuth, so a second preset
// exercises it isn't accidentally bedrock-specific.
func TestProviderTUIVertexSkipsAPIKeyStep(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	idx := -1
	for i, p := range m.providers {
		if p.Name == "vertex" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("vertex not offered in the official provider list")
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
		t.Error("ambient provider advanced to stepAPIKey; want the model step to be final")
	}
	if !done.confirmed {
		t.Error("confirmed = false; want the selection confirmed from the model step")
	}
	if cmd == nil {
		t.Error("no command returned; want tea.Quit")
	}
	res := done.result()
	if res.provider != "vertex" {
		t.Errorf("result provider = %q, want vertex", res.provider)
	}
	if res.apiKey != "" {
		t.Errorf("result apiKey = %q, want empty for an ambient provider", res.apiKey)
	}
}

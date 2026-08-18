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

// TestConfigRoundTripKeepsAWSSettings is the regression test for a silent loss:
// config is unmarshalled into Config and marshalled back on every write, so
// before aws_profile / aws_region existed on ProviderEntry, the first run of any
// config command deleted them from a hand-written file — with no error, and no
// way for the user to tell why Bedrock suddenly used the wrong region.
func TestConfigRoundTripKeepsAWSSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := `{
  "provider": "bedrock",
  "model": "us.anthropic.claude-sonnet-4-6",
  "providers": {
    "bedrock": { "aws_region": "us-west-2", "aws_profile": "example-profile" }
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
	entry := reloaded.Providers["bedrock"]
	if entry.AWSRegion != "us-west-2" {
		t.Errorf("AWSRegion = %q after round trip, want us-west-2", entry.AWSRegion)
	}
	if entry.AWSProfile != "example-profile" {
		t.Errorf("AWSProfile = %q after round trip, want example-profile", entry.AWSProfile)
	}

	// The resolver reads the same file independently; assert the written JSON
	// still carries the keys it looks for, not just that our struct held them.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal written config: %v", err)
	}
	providers, _ := raw["providers"].(map[string]any)
	bedrockEntry, _ := providers["bedrock"].(map[string]any)
	if bedrockEntry["aws_region"] != "us-west-2" || bedrockEntry["aws_profile"] != "example-profile" {
		t.Errorf("written JSON = %v, want aws_region and aws_profile preserved", bedrockEntry)
	}
}

func TestSetProviderValueAWSSettings(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr string
		check   func(*testing.T, *Config)
	}{
		{
			name:  "region on an ambient provider",
			key:   "providers.bedrock.aws_region",
			value: "us-west-2",
			check: func(t *testing.T, cfg *Config) {
				if got := cfg.Providers["bedrock"].AWSRegion; got != "us-west-2" {
					t.Errorf("AWSRegion = %q, want us-west-2", got)
				}
			},
		},
		{
			name:  "profile is trimmed",
			key:   "providers.bedrock.aws_profile",
			value: "  example-profile  ",
			check: func(t *testing.T, cfg *Config) {
				if got := cfg.Providers["bedrock"].AWSProfile; got != "example-profile" {
					t.Errorf("AWSProfile = %q, want example-profile", got)
				}
			},
		},
		{
			name:  "empty value hands the decision back to the AWS chain",
			key:   "providers.bedrock.aws_profile",
			value: "",
			check: func(t *testing.T, cfg *Config) {
				if got := cfg.Providers["bedrock"].AWSProfile; got != "" {
					t.Errorf("AWSProfile = %q, want empty", got)
				}
			},
		},
		{
			// Storing it would be dead config that reads as applied.
			name:    "rejected on a key-based provider",
			key:     "providers.anthropic.aws_region",
			value:   "us-west-2",
			wantErr: "does not apply to provider",
		},
		{
			name:    "whitespace inside the value is rejected",
			key:     "providers.bedrock.aws_region",
			value:   "us west 2",
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

// TestSetCustomProviderAWSSettingsFollowProtocol covers the custom-provider
// path: aws_* is meaningful there only once the entry speaks the Bedrock
// protocol, so the order of the two set commands matters and the error has to
// say why.
func TestSetCustomProviderAWSSettingsFollowProtocol(t *testing.T) {
	cfg := &Config{}
	if err := setCustomProviderValue(cfg, "custom_providers.mine.aws_region", "us-west-2"); err == nil {
		t.Fatal("aws_region accepted before a protocol was set; want an error")
	}

	if err := setCustomProviderValue(cfg, "custom_providers.mine.protocol", llm.ProtocolAnthropicBedrock); err != nil {
		t.Fatalf("set protocol: %v", err)
	}
	if err := setCustomProviderValue(cfg, "custom_providers.mine.aws_region", "us-west-2"); err != nil {
		t.Fatalf("set aws_region after protocol: %v", err)
	}
	if got := cfg.CustomProviders["mine"].AWSRegion; got != "us-west-2" {
		t.Errorf("AWSRegion = %q, want us-west-2", got)
	}
}

// TestAWSSettingsRejectedWhenEntryOverridesProtocol covers the same
// entry-level protocol override the resolver honours: a preset's protocol can be
// overridden per entry, so `protocol: openai` on the bedrock preset must stop
// accepting AWS settings that nothing would read.
func TestAWSSettingsRejectedWhenEntryOverridesProtocol(t *testing.T) {
	cfg := &Config{}
	if err := setProviderValue(cfg, "providers.bedrock.protocol", "openai"); err != nil {
		t.Fatalf("set protocol: %v", err)
	}
	err := setProviderValue(cfg, "providers.bedrock.aws_region", "us-west-2")
	if err == nil {
		t.Fatal("aws_region accepted on a bedrock entry overridden to protocol openai; want an error")
	}
	if !strings.Contains(err.Error(), "does not apply to provider") {
		t.Errorf("error = %q, want it to explain the field does not apply", err)
	}

	// Overriding back to the bedrock protocol makes them meaningful again.
	if err := setProviderValue(cfg, "providers.bedrock.protocol", llm.ProtocolAnthropicBedrock); err != nil {
		t.Fatalf("set protocol back: %v", err)
	}
	if err := setProviderValue(cfg, "providers.bedrock.aws_region", "us-west-2"); err != nil {
		t.Errorf("aws_region rejected for an explicit bedrock protocol: %v", err)
	}
}

// TestSetLlmProtocolRejectsBedrock pins the other half of the contract enforced
// in the resolver: the llm block is one url plus one token, with nowhere to put
// a region or a profile, so the value is refused where it is typed rather than
// stored and ignored until the next review run.
func TestSetLlmProtocolRejectsBedrock(t *testing.T) {
	cfg := &Config{}
	err := setConfigValue(cfg, "llm.protocol", llm.ProtocolAnthropicBedrock)
	if err == nil {
		t.Fatal("llm.protocol accepted anthropic-bedrock; want an error")
	}
	for _, want := range []string{"aws_region", "provider bedrock"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if cfg.Llm.Protocol != "" {
		t.Errorf("Llm.Protocol = %q, want it left unset after the rejection", cfg.Llm.Protocol)
	}

	// The neighbouring values still work — the rejection is one protocol, not
	// the whole key.
	if err := setConfigValue(cfg, "llm.protocol", llm.ProtocolOpenAIResponses); err != nil {
		t.Fatalf("set llm.protocol=openai-responses: %v", err)
	}
}

func TestCheckAPIKeyRequirement(t *testing.T) {
	bedrock, ok := llm.LookupProvider("bedrock")
	if !ok {
		t.Fatal("bedrock preset not registered")
	}
	anthropic, ok := llm.LookupProvider("anthropic")
	if !ok {
		t.Fatal("anthropic preset not registered")
	}

	if err := checkAPIKeyRequirement("bedrock", "", "", bedrock, true); err != nil {
		t.Errorf("ambient provider with no api_key = %v, want nil", err)
	}

	t.Setenv(anthropic.EnvVar, "")
	if err := checkAPIKeyRequirement("anthropic", "", "", anthropic, true); err == nil {
		t.Error("key-based provider with no api_key and no env var = nil, want an error")
	}
	if err := checkAPIKeyRequirement("anthropic", "", "op read op://vault/key", anthropic, true); err != nil {
		t.Errorf("key-based provider with api_key_cmd = %v, want nil", err)
	}
}

// TestProviderTUIAmbientProviderSkipsAPIKeyStep pins the wizard flow: the model
// step is the last one for a provider with no key to collect. An API-key prompt
// that must be left blank reads as a step the user failed to complete.
func TestProviderTUIAmbientProviderSkipsAPIKeyStep(t *testing.T) {
	m := newProviderTUI(&Config{}, "")
	idx := -1
	for i, p := range m.providers {
		if p.Name == "bedrock" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("bedrock not offered in the official provider list")
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
	if res.provider != "bedrock" {
		t.Errorf("result provider = %q, want bedrock", res.provider)
	}
	if res.apiKey != "" {
		t.Errorf("result apiKey = %q, want empty for an ambient provider", res.apiKey)
	}
	if got := res.resolvedModel(); got == "" {
		t.Error("resolvedModel is empty; want the model selected on the model step")
	}
}

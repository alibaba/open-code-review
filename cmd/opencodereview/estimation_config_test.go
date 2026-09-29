// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/estimate"
	"github.com/alibaba/open-code-review/internal/llm"
)

func TestEstimationConfigRoundTrip(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "estimation_overhead_tokens", "2147483647"); err != nil {
		t.Fatal(err)
	}
	if err := setConfigValue(cfg, "estimation_output_tokens_per_round", "950"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := saveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAppConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.EstimationOverheadTokens != 2147483647 || loaded.EstimationOutputTokensPerRound != 950 {
		t.Fatalf("estimation settings were not preserved: %+v", loaded)
	}
}

func TestSetEstimationConfigRejectsInvalidValues(t *testing.T) {
	for _, key := range []string{"estimation_overhead_tokens", "estimation_output_tokens_per_round"} {
		for _, value := range []string{"-1", "1.5", "", "abc", "2147483648", "999999999999999999999999999999"} {
			t.Run(key+"/"+value, func(t *testing.T) {
				cfg := &Config{EstimationOverheadTokens: 3000, EstimationOutputTokensPerRound: 950}
				before := *cfg
				err := setConfigValue(cfg, key, value)
				if err == nil || !strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "non-negative 32-bit integer") {
					t.Fatalf("setConfigValue error = %v, want an actionable error for %s", err, key)
				}
				if !reflect.DeepEqual(*cfg, before) {
					t.Fatal("invalid input changed the config")
				}
			})
		}
	}
}

func TestLoadAppConfigEstimationValidation(t *testing.T) {
	for _, key := range []string{"estimation_overhead_tokens", "estimation_output_tokens_per_round"} {
		for _, value := range []string{"-1", "2147483648", "999999999999999999999999999999", "1.5", `"700"`} {
			t.Run(key+"/"+value, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "config.json")
				data := fmt.Sprintf(`{"%s":%s}`, key, value)
				if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := LoadAppConfig(path); err == nil {
					t.Fatalf("LoadAppConfig accepted invalid %s=%s", key, value)
				}
			})
		}
	}
}

func TestResetEstimationConfig(t *testing.T) {
	defaults := map[string]int64{
		"estimation_overhead_tokens":         2000,
		"estimation_output_tokens_per_round": 700,
	}
	for _, key := range []string{"estimation_overhead_tokens", "estimation_output_tokens_per_round"} {
		for _, method := range []string{"zero", "unset"} {
			t.Run(key+"/"+method, func(t *testing.T) {
				setTestHome(t, t.TempDir())
				path, err := defaultConfigPath()
				if err != nil {
					t.Fatal(err)
				}
				cfg := &Config{Provider: "anthropic", EstimationOverheadTokens: 3000, EstimationOutputTokensPerRound: 950}
				if err := saveConfig(path, cfg); err != nil {
					t.Fatal(err)
				}
				if method == "zero" {
					out := captureStdout(t, func() { err = runConfigSet(key, "0") })
					wantMessage := fmt.Sprintf("Set %s = 0 (using default %d)", key, defaults[key])
					if !strings.Contains(out, wantMessage) {
						t.Errorf("config set output = %q, want %q", out, wantMessage)
					}
				} else {
					err = runConfigUnset(key)
				}
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(data), key) {
					t.Fatalf("reset key should be omitted from JSON: %s", data)
				}
				loaded, err := LoadAppConfig(path)
				if err != nil {
					t.Fatal(err)
				}
				want := estimate.Parameters{PromptOverheadTokens: 3000, OutputTokensPerRound: 950}
				if key == "estimation_overhead_tokens" {
					want.PromptOverheadTokens = 2000
				} else {
					want.OutputTokensPerRound = 700
				}
				if got := resolveEstimation(loaded); got != want {
					t.Errorf("resolved estimate = %+v, want %+v", got, want)
				}
				if loaded.Provider != "anthropic" {
					t.Error("reset changed an unrelated setting")
				}
			})
		}
	}
}

func TestLoadLLMRuntimeEstimation(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want estimate.Parameters
	}{
		{"missing config", nil, estimate.Parameters{PromptOverheadTokens: 2000, OutputTokensPerRound: 700}},
		{"zero values", &Config{}, estimate.Parameters{PromptOverheadTokens: 2000, OutputTokensPerRound: 700}},
		{"overhead only", &Config{EstimationOverheadTokens: 1}, estimate.Parameters{PromptOverheadTokens: 1, OutputTokensPerRound: 700}},
		{"output only", &Config{EstimationOutputTokensPerRound: 1}, estimate.Parameters{PromptOverheadTokens: 2000, OutputTokensPerRound: 1}},
		{"both custom", &Config{EstimationOverheadTokens: 3000, EstimationOutputTokensPerRound: 950}, estimate.Parameters{PromptOverheadTokens: 3000, OutputTokensPerRound: 950}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTestHome(t, t.TempDir())
			t.Setenv("OCR_LLM_URL", "https://api.example.test/v1")
			t.Setenv("OCR_LLM_TOKEN", "test-token")
			t.Setenv("OCR_LLM_MODEL", "test-model")
			if tt.cfg != nil {
				path, err := defaultConfigPath()
				if err != nil {
					t.Fatal(err)
				}
				if err := saveConfig(path, tt.cfg); err != nil {
					t.Fatal(err)
				}
			}
			rt, err := loadLLMRuntime(loadTestTemplate(t), "", llm.ResolveOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got := resolveEstimation(rt.AppCfg); got != tt.want {
				t.Errorf("resolved estimate = %+v, want %+v", got, tt.want)
			}
		})
	}
}

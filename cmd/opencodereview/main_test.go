// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRunConfigSetProviderNames(t *testing.T) {
	if os.Getenv("OCR_TEST_CONFIG_SET") == "1" {
		rootCmd.SetArgs(os.Args[3:])
		os.Exit(run())
	}
	setTestHome(t, t.TempDir())
	configPath, err := defaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(configPath, &Config{Provider: "openai", Model: "saved-model"}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		key   string
		value string
		code  int
	}{
		{"providers.openai.extra_body.model", "some-model", 1},
		{"custom_providers.openai.extra_body.model", "some-model", 1},
		{"providers.openai..url", "https://example.invalid/v1", 1},
		{"providers..a.url", "https://example.invalid/v1", 1},
		{"providers. .url", "https://example.invalid/v1", 1},
		{"custom_providers.anthropic..url", "https://example.invalid/v1", 1},
		{"provider", ".gateway", 1},
		{"provider", "gateway.", 1},
		{"provider", "gateway..v2", 1},
		{"provider", " ", 1},
		{"provider", "mimo-v2.6-flash", 0},
		{"providers.mimo-v2.6-flash.url", "https://example.invalid/v1", 0},
		{"custom_providers.mimo-v2.6-flash.protocol", "openai", 0},
		{"provider", "foo.api_key", 0},
		{"custom_providers.foo.api_key.model", "some-model", 0},
	}
	for _, tt := range tests {
		t.Run(tt.key+"/"+tt.value, func(t *testing.T) {
			before, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestRunConfigSetProviderNames$", "--", "config", "set", tt.key, tt.value)
			cmd.Env = append(os.Environ(), "OCR_TEST_CONFIG_SET=1", "OCR_NO_UPDATE=1", "OCR_ENABLE_TELEMETRY=0")
			output, err := cmd.CombinedOutput()
			if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != tt.code {
				t.Fatalf("exit error = %v, want code %d; output:\n%s", err, tt.code, output)
			}
			after, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if tt.code != 0 && !bytes.Equal(before, after) {
				t.Fatalf("configuration changed after rejected write: %s", after)
			}
		})
	}
	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CustomProviders["mimo-v2.6-flash"].URL != "https://example.invalid/v1" ||
		cfg.CustomProviders["mimo-v2.6-flash"].Protocol != "openai" ||
		cfg.CustomProviders["foo.api_key"].Model != "some-model" {
		t.Fatalf("dotted provider fields were not persisted: %#v", cfg.CustomProviders)
	}
}

func TestRunFlushesTelemetryOnError(t *testing.T) {
	if os.Getenv("OCR_TEST_RUN_ERROR") == "1" {
		rootCmd.SetArgs([]string{"--invalid-flag"})
		os.Exit(run())
	}

	home := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunFlushesTelemetryOnError$")
	cmd.Env = append(os.Environ(),
		"OCR_TEST_RUN_ERROR=1",
		"OCR_ENABLE_TELEMETRY=1",
		"OTEL_EXPORTER_OTLP_ENDPOINT=",
		"HOME="+home,
		"USERPROFILE="+home,
	)
	output, err := cmd.CombinedOutput()
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("exit error = %v, want exit code 1; output:\n%s", err, output)
	}
	if !strings.Contains(string(output), `"Resource"`) {
		t.Fatalf("telemetry was not flushed on error; output:\n%s", output)
	}
}

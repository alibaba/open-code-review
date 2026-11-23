// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunConfigSetProviderNames(t *testing.T) {
	if os.Getenv("OCR_TEST_CONFIG_SET") == "1" {
		os.Exit(runArgs(os.Args[3:]))
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

func TestTestMainProtectsInheritedOCRHome(t *testing.T) {
	storage := t.TempDir()
	config := filepath.Join(storage, "config.json")
	sentinel := []byte(`{"model":"sentinel"}`)
	if err := os.WriteFile(config, sentinel, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunConfigSetProviderNames$")
	cmd.Env = append(os.Environ(), testHomeIsolatedEnv+"=", "OCR_HOME="+storage)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("subprocess: %v\n%s", err, out)
	}
	got, err := os.ReadFile(config)
	if err != nil || !bytes.Equal(got, sentinel) {
		t.Fatalf("inherited config changed: %q, %v", got, err)
	}
}

func TestTestMainPreservesIsolatedHomeForSubprocess(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" || home != os.Getenv("USERPROFILE") {
		t.Fatalf("test home is not isolated: HOME=%q USERPROFILE=%q", home, os.Getenv("USERPROFILE"))
	}
	if os.Getenv("OCR_TEST_HOME_SUBPROCESS") == "1" {
		want := os.Getenv("OCR_TEST_HOME_PARENT")
		if home != want || os.Getenv("USERPROFILE") != want {
			t.Fatalf("subprocess home = HOME:%q USERPROFILE:%q, want %q", home, os.Getenv("USERPROFILE"), want)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestTestMainPreservesIsolatedHomeForSubprocess$")
	cmd.Env = append(os.Environ(), "OCR_TEST_HOME_SUBPROCESS=1", "OCR_TEST_HOME_PARENT="+home)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, output)
	}
}

func TestRunFlushesTelemetryOnError(t *testing.T) {
	if os.Getenv("OCR_TEST_RUN_ERROR") == "1" {
		os.Exit(runArgs([]string{"--invalid-flag"}))
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

func TestRunConfigPathWithConsoleTelemetry(t *testing.T) {
	if os.Getenv("OCR_TEST_CONFIG_PATH") == "1" {
		os.Exit(runArgs(os.Args[3:]))
	}
	for _, mode := range []string{"environment", "config"} {
		for _, args := range [][]string{
			{"config", "path", "--json"},
			{"--color", "never", "config", "path", "--json"},
		} {
			t.Run(mode+"/"+strings.Join(args, " "), func(t *testing.T) {
				home := t.TempDir()
				storage := filepath.Join(home, "custom storage")
				if err := os.MkdirAll(storage, 0o700); err != nil {
					t.Fatal(err)
				}
				enabled := "1"
				if mode == "config" {
					enabled = ""
					if err := os.WriteFile(filepath.Join(storage, "config.json"), []byte(`{"telemetry":{"enabled":true,"exporter":"console"}}`), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				childArgs := append([]string{"-test.run=^TestRunConfigPathWithConsoleTelemetry$", "--"}, args...)
				cmd := exec.Command(os.Args[0], childArgs...)
				cmd.Env = append(os.Environ(),
					"OCR_TEST_CONFIG_PATH=1", "OCR_ENABLE_TELEMETRY="+enabled,
					"OTEL_EXPORTER_OTLP_ENDPOINT=", "OCR_HOME="+storage,
					"HOME="+home, "USERPROFILE="+home,
				)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				stdout, err := cmd.Output()
				if err != nil || stderr.Len() != 0 {
					t.Fatalf("exit error = %v, stderr = %s", err, stderr.String())
				}
				var result struct {
					Path string `json:"path"`
				}
				if err := json.Unmarshal(stdout, &result); err != nil {
					t.Fatalf("stdout is not one JSON object: %v; output:\n%s", err, stdout)
				}
				if want := filepath.Join(storage, "config.json"); result.Path != want {
					t.Fatalf("path = %q, want %q", result.Path, want)
				}
			})
		}
	}
}

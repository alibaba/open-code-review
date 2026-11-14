// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/chatgptauth"
)

func inspectionEnv(t *testing.T) {
	t.Helper()
	clearAllEnv(t)
	t.Setenv(envOCRLLMTimeout, "")
	t.Setenv(envOCRLLMExtraHeaders, "")
}

func TestInspectEndpointMatchesRuntimeSelection(t *testing.T) {
	for _, mode := range []string{"provider", "legacy", "OCR environment", "Claude Code environment", "shell rc"} {
		t.Run(mode, func(t *testing.T) {
			inspectionEnv(t)
			path := filepath.Join(t.TempDir(), "missing.json")
			switch mode {
			case "provider":
				path = writeConfig(t, map[string]any{"provider": "anthropic", "providers": map[string]any{"anthropic": map[string]any{"api_key": "fixture-secret", "model": "fixture-model[1m]", "timeout_sec": 18}}})
				// The config strategy must win even over a complete OCR environment.
				t.Setenv(envOCRLLMURL, "https://env.invalid")
				t.Setenv(envOCRLLMToken, "env-secret")
				t.Setenv(envOCRLLMModel, "env-model")
			case "legacy":
				path = writeConfig(t, map[string]any{"llm": map[string]any{"url": "https://legacy.invalid", "auth_token": "fixture-secret", "model": "fixture-model", "timeout_sec": 18}})
			case "OCR environment":
				t.Setenv(envOCRLLMURL, "https://env.invalid")
				t.Setenv(envOCRLLMToken, "fixture-secret")
				t.Setenv(envOCRLLMModel, "fixture-model[1m]")
			case "Claude Code environment":
				t.Setenv(envCCBaseURL, "https://env.invalid")
				t.Setenv(envCCToken, "fixture-secret")
				t.Setenv(envCCModel, "fixture-model[1m]")
			case "shell rc":
				home, _ := os.UserHomeDir()
				if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export ANTHROPIC_BASE_URL=https://shell.invalid\nexport ANTHROPIC_AUTH_TOKEN=fixture-secret\nexport ANTHROPIC_MODEL=fixture-model[1m]\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for _, override := range []string{"", "37", "0"} {
				t.Setenv(envOCRLLMTimeout, override)
				want, err := ResolveEndpoint(path)
				if err != nil {
					t.Fatal(err)
				}
				got, err := InspectEndpoint(path)
				if err != nil {
					t.Fatal(err)
				}
				if got.Provider != want.Provider || got.Model != want.Model || got.Protocol != want.Protocol || got.Source != want.Source || got.TimeoutSec != int64(want.Timeout.Seconds()) || got.CredentialStatus != "configured" {
					t.Fatalf("inspection metadata differs from runtime: %+v", got)
				}
				if override != "" && got.TimeoutSource != envOCRLLMTimeout {
					t.Fatal("missing timeout override provenance")
				}
			}
		})
	}
}

func TestInspectEndpointDoesNotExecuteCredentialCommands(t *testing.T) {
	for _, mode := range []string{"provider", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			inspectionEnv(t)
			marker := filepath.Join(t.TempDir(), "executed")
			command := "echo must-not-execute > \"" + filepath.ToSlash(marker) + "\""
			entry := map[string]any{"model": "fixture-model", "url": "https://fixture.invalid", "protocol": "openai"}
			var cfg map[string]any
			if mode == "provider" {
				entry["api_key_cmd"] = command
				cfg = map[string]any{"provider": "gateway", "custom_providers": map[string]any{"gateway": entry}}
			} else {
				entry["auth_token_cmd"] = command
				cfg = map[string]any{"llm": entry}
			}
			path := writeConfig(t, cfg)
			before, _ := os.ReadFile(path)
			got, err := InspectEndpoint(path)
			if err != nil {
				t.Fatal(err)
			}
			if got.CredentialStatus != "command_not_executed" || got.Model != "fixture-model" {
				t.Fatalf("inspection = %+v", got)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("credential command executed")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("inspection changed config")
			}
		})
	}
}

func TestInspectEndpointOmitsCredentialBearingConfiguration(t *testing.T) {
	inspectionEnv(t)
	path := writeConfig(t, map[string]any{
		"provider": "gateway",
		"custom_providers": map[string]any{"gateway": map[string]any{
			"model": "fixture-model", "protocol": "openai", "api_key": "static-secret", "api_key_cmd": "command-secret",
			"url":           "https://user:password-secret@fixture.invalid/path-secret?token=query-secret",
			"extra_headers": map[string]string{"X-Secret": "header-secret"},
			"extra_body":    map[string]string{"credential": "body-secret"},
		}},
		"mcp_servers": map[string]any{"private": map[string]any{"env": map[string]string{"TOKEN": "mcp-secret"}}},
	})
	t.Setenv(envOCRLLMExtraHeaders, "X-Environment=environment-secret")
	got, err := InspectEndpoint(path)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"static-secret", "command-secret", "password-secret", "path-secret", "query-secret", "header-secret", "body-secret", "mcp-secret", "environment-secret"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("inspection leaked %s", secret)
		}
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 7 {
		t.Fatalf("unexpected output fields: %s", encoded)
	}
}

func TestInspectEndpointDoesNotResolveOAuthOrAmbientCredentials(t *testing.T) {
	inspectionEnv(t)
	credentials, models := chatGPTCredentials, chatGPTModels
	t.Cleanup(func() { chatGPTCredentials, chatGPTModels = credentials, models })
	chatGPTCredentials = func(context.Context, string) (*chatgptauth.Auth, error) {
		t.Fatal("OAuth credentials requested")
		return nil, nil
	}
	chatGPTModels = func(context.Context, *chatgptauth.Auth) ([]chatgptauth.Model, error) {
		t.Fatal("model catalog requested")
		return nil, nil
	}
	for _, model := range []string{"", "configured-model"} {
		path := writeConfig(t, map[string]any{"provider": "chatgpt", "model": model, "providers": map[string]any{"chatgpt": map[string]any{}}})
		got, err := InspectEndpoint(path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Model != model || got.CredentialStatus != "oauth_not_resolved" {
			t.Fatalf("inspection = %+v", got)
		}
	}
	path := writeConfig(t, map[string]any{"provider": "bedrock", "providers": map[string]any{"bedrock": map[string]any{"model": "fixture-model", "api_key_cmd": "exit 99"}}})
	got, err := InspectEndpoint(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.CredentialStatus != "ambient_not_resolved" {
		t.Fatalf("inspection = %+v", got)
	}
}

func TestInspectEndpointMissingAndInvalidConfiguration(t *testing.T) {
	inspectionEnv(t)
	path := filepath.Join(t.TempDir(), "missing.json")
	got, err := InspectEndpoint(path)
	if err != nil || got.Source != "none" || got.CredentialStatus != "not_configured" {
		t.Fatalf("inspection = %+v, %v", got, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("inspection created config")
	}
	for _, timeout := range []string{"37", "0"} {
		t.Setenv(envOCRLLMTimeout, timeout)
		got, err = InspectEndpoint(path)
		if err != nil || got.TimeoutSource != envOCRLLMTimeout || (timeout == "37" && got.TimeoutSec != 37) || (timeout == "0" && got.TimeoutSec != 0) {
			t.Fatalf("missing configuration timeout = %+v, %v", got, err)
		}
	}
	t.Setenv(envOCRLLMTimeout, "")
	path = writeConfig(t, map[string]any{"provider": "anthropic", "providers": map[string]any{"anthropic": map[string]any{}}})
	got, err = InspectEndpoint(path)
	if err != nil || got.Model != "" || got.CredentialStatus != "not_configured" {
		t.Fatalf("inspection = %+v, %v", got, err)
	}
	for _, invalid := range []string{`{"secret":"json-secret",broken}`, `{"provider":"password-secret"}`} {
		if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := InspectEndpoint(path)
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe diagnostic: %v", err)
		}
	}
	for key, value := range map[string]string{envOCRLLMExtraHeaders: "header-secret", envOCRLLMTimeout: "timeout-secret"} {
		t.Setenv(key, value)
		_, err := InspectEndpoint(path)
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe environment diagnostic: %v", err)
		}
		t.Setenv(key, "")
	}
}

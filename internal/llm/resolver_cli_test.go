// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveClaudeCLIWithoutKey pins that the claude-code preset resolves with
// no api_key and no url: the CLI protocol is ambient-auth, so the completeness
// check must treat an empty URL and Token as valid.
func TestResolveClaudeCLIWithoutKey(t *testing.T) {
	path := writeConfig(t, map[string]any{
		"provider":  "claude-code",
		"model":     "claude-sonnet-5",
		"providers": map[string]any{"claude-code": map[string]any{}},
	})

	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.Protocol != ProtocolClaudeCLI {
		t.Errorf("Protocol = %q, want %q", ep.Protocol, ProtocolClaudeCLI)
	}
	if !ep.AmbientAuth {
		t.Error("AmbientAuth = false, want true")
	}
	if ep.URL != "" || ep.Token != "" {
		t.Errorf("URL/Token = %q/%q, want empty", ep.URL, ep.Token)
	}
	if ep.Model != "claude-sonnet-5" {
		t.Errorf("Model = %q, want claude-sonnet-5", ep.Model)
	}
}

// TestResolveCustomCodexCLIWithoutURLOrKey covers a custom provider selecting a
// CLI protocol: neither a url nor a key is demanded.
func TestResolveCustomCodexCLIWithoutURLOrKey(t *testing.T) {
	path := writeConfig(t, map[string]any{
		"provider": "mine",
		"model":    "gpt-5.5",
		"custom_providers": map[string]any{
			"mine": map[string]any{"protocol": ProtocolCodexCLI},
		},
	})

	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.Protocol != ProtocolCodexCLI {
		t.Errorf("Protocol = %q, want %q", ep.Protocol, ProtocolCodexCLI)
	}
	if !ep.AmbientAuth {
		t.Error("AmbientAuth = false, want true")
	}
	if ep.URL != "" || ep.Token != "" {
		t.Errorf("URL/Token = %q/%q, want empty", ep.URL, ep.Token)
	}
}

// TestCLIPathAndArgsReachEndpoint pins that cli_path and cli_args flow from the
// provider entry onto the resolved endpoint, where NewLLMClient reads them.
func TestCLIPathAndArgsReachEndpoint(t *testing.T) {
	path := writeConfig(t, map[string]any{
		"provider": "claude-code",
		"model":    "claude-sonnet-5",
		"providers": map[string]any{
			"claude-code": map[string]any{
				"cli_path": "/opt/bin/claude",
				"cli_args": []string{"--foo", "bar"},
			},
		},
	})

	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.CLIPath != "/opt/bin/claude" {
		t.Errorf("CLIPath = %q, want /opt/bin/claude", ep.CLIPath)
	}
	if len(ep.CLIArgs) != 2 || ep.CLIArgs[0] != "--foo" || ep.CLIArgs[1] != "bar" {
		t.Errorf("CLIArgs = %v, want [--foo bar]", ep.CLIArgs)
	}
}

// TestCLIProviderDoesNotRunAPIKeyCmd mirrors the bedrock guarantee: an ambient
// CLI provider never executes api_key_cmd, so a secret-manager read is not
// triggered for a value nothing consumes. The sentinel proves non-execution.
func TestCLIProviderDoesNotRunAPIKeyCmd(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "ran")
	path := writeConfig(t, map[string]any{
		"provider": "claude-code",
		"model":    "claude-sonnet-5",
		"providers": map[string]any{
			"claude-code": map[string]any{"api_key_cmd": "touch '" + sentinel + "'; echo sk-should-never-be-used"},
		},
	})

	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Error("api_key_cmd ran for an ambient-auth CLI provider")
	}
	if ep.Token != "" {
		t.Errorf("Token = %q, want empty", ep.Token)
	}
}

// TestCLIProtocolIsRejectedOnTheURLAndTokenPaths pins that the env and legacy
// url+token strategies refuse a CLI protocol: neither carries a place for a CLI
// path, and a CLI protocol has no use for the url or token they do carry.
func TestCLIProtocolIsRejectedOnTheURLAndTokenPaths(t *testing.T) {
	t.Run("OCR_LLM_PROTOCOL", func(t *testing.T) {
		t.Setenv("OCR_LLM_URL", "https://example.invalid/v1")
		t.Setenv("OCR_LLM_TOKEN", "sk-test")
		t.Setenv("OCR_LLM_MODEL", "claude-sonnet-5")
		t.Setenv("OCR_LLM_PROTOCOL", ProtocolClaudeCLI)

		_, err := ResolveEndpoint(writeConfig(t, map[string]any{}))
		if err == nil {
			t.Fatal("resolved with OCR_LLM_PROTOCOL=claude-cli; want an error naming the variable")
		}
		for _, want := range []string{"OCR_LLM_PROTOCOL", "claude-code"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
	})

	t.Run("llm.protocol", func(t *testing.T) {
		for _, k := range []string{"OCR_LLM_URL", "OCR_LLM_TOKEN", "OCR_LLM_MODEL", "OCR_LLM_PROTOCOL"} {
			t.Setenv(k, "")
		}
		path := writeConfig(t, map[string]any{
			"llm": map[string]any{
				"url":        "https://example.invalid/v1",
				"auth_token": "sk-test",
				"model":      "gpt-5.5",
				"protocol":   ProtocolCodexCLI,
			},
		})

		_, err := ResolveEndpoint(path)
		if err == nil {
			t.Fatal("resolved with llm.protocol=codex-cli; want an error naming the key")
		}
		if !strings.Contains(err.Error(), "llm.protocol") {
			t.Errorf("error %q does not mention llm.protocol", err)
		}
	})
}

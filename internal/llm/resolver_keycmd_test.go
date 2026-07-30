//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Every test in this file drives a credential command, and all of them are POSIX
// shell (`printf`, `exit N`), which would run through `cmd /C` on Windows.

package llm

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigJSON(t *testing.T, cfg configFile) string {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

// (a) api_key_cmd resolves when no static key is present.
func TestResolveEndpoint_ProviderAPIKeyCmd(t *testing.T) {
	clearAllEnv(t)
	cfgPath := writeConfigJSON(t, configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKeyCmd: "printf 'sk-from-cmd\\n'", Model: "claude-sonnet-4-6"},
		},
	})
	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "sk-from-cmd" {
		t.Errorf("Token = %q, want %q", ep.Token, "sk-from-cmd")
	}
}

// (a2) the command runs exactly once per resolution. "No caching" is correct
// today only because resolution happens once per process; a second call would
// mean a second pinentry prompt per review.
func TestResolveEndpoint_APIKeyCmdRunsExactlyOnce(t *testing.T) {
	clearAllEnv(t)
	counter := filepath.Join(t.TempDir(), "runs")
	cfgPath := writeConfigJSON(t, configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {
				APIKeyCmd: "echo run >> " + counter + "; printf 'sk-once\\n'",
				Model:     "claude-sonnet-4-6",
			},
		},
	})
	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "sk-once" {
		t.Fatalf("Token = %q, want %q", ep.Token, "sk-once")
	}
	data, err := os.ReadFile(counter)
	if err != nil {
		t.Fatalf("read counter file: %v", err)
	}
	if got := strings.Count(string(data), "\n"); got != 1 {
		t.Errorf("api_key_cmd ran %d times, want exactly 1 (counter file %q)", got, data)
	}
}

// (b) static api_key wins even when api_key_cmd is also set — and the command
// does not run at all. Asserting only on ep.Token would pass just as well if the
// command ran and its output were discarded, which for a real config means a
// pinentry/Touch ID prompt on every review that keeps a command as a fallback.
func TestResolveEndpoint_ProviderStaticKeyWinsOverCmd(t *testing.T) {
	clearAllEnv(t)
	marker := filepath.Join(t.TempDir(), "ran")
	cfgPath := writeConfigJSON(t, configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {
				APIKey:    "sk-static",
				APIKeyCmd: "touch " + marker + "; printf 'sk-from-cmd\\n'",
				Model:     "claude-sonnet-4-6",
			},
		},
	})
	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "sk-static" {
		t.Errorf("Token = %q, want %q (static api_key must win)", ep.Token, "sk-static")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("api_key_cmd executed even though a static api_key was set")
	}
}

// (b4) a whitespace-only api_key_cmd is a typo, not a command: it must not
// suppress the env-var fallback the way a real command does. Same rule the
// static api_key already follows.
func TestResolveEndpoint_WhitespaceOnlyCmdFallsBackToEnv(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-from-env")
	cfgPath := writeConfigJSON(t, configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKeyCmd: "   ", Model: "claude-sonnet-4-6"},
		},
	})
	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "sk-from-env" {
		t.Errorf("Token = %q, want %q (whitespace-only api_key_cmd must be treated as unset)", ep.Token, "sk-from-env")
	}
}

// (b5) same rule on the legacy block: whitespace-only auth_token_cmd leaves the
// block incomplete rather than running an empty command and hard-failing.
func TestResolveEndpoint_LegacyWhitespaceOnlyCmdIsUnset(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("ANTHROPIC_BASE_URL", "https://env.test")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "sk-from-env")
	t.Setenv("ANTHROPIC_MODEL", "m")
	cfgPath := writeConfigJSON(t, configFile{
		Llm: llmFileConfig{URL: "https://example.test", Model: "m", AuthTokenCmd: "  \t "},
	})
	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "sk-from-env" {
		t.Errorf("Token = %q, want %q (whitespace-only auth_token_cmd must be treated as unset)", ep.Token, "sk-from-env")
	}
}

// captureStderr swaps os.Stderr for a pipe around fn and returns what was written.
// Output here is tiny, so reading after the writer is closed avoids any pipe-buffer
// deadlock without a goroutine.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	return string(out)
}

// (b2) when both api_key and api_key_cmd are set, a warning is emitted on stderr
// and the resolved token is still the static api_key.
func TestResolveEndpoint_BothSetWarnsAndUsesStaticKey(t *testing.T) {
	clearAllEnv(t)
	cfgPath := writeConfigJSON(t, configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKey: "sk-static", APIKeyCmd: "printf 'sk-from-cmd\\n'", Model: "claude-sonnet-4-6"},
		},
	})
	var ep ResolvedEndpoint
	var err error
	stderr := captureStderr(t, func() {
		ep, err = ResolveEndpoint(cfgPath)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "sk-static" {
		t.Errorf("Token = %q, want %q (static api_key must win)", ep.Token, "sk-static")
	}
	// Match the message, not the log prefix, so this does not break when the
	// warning prefix is restyled.
	want := `provider "anthropic" has both api_key and api_key_cmd set; using the static api_key`
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr %q does not contain warning %q", stderr, want)
	}
}

// (e2) legacy path: both auth_token and auth_token_cmd set -> warning + static wins.
func TestResolveEndpoint_LegacyBothSetWarnsAndUsesStaticToken(t *testing.T) {
	clearAllEnv(t)
	cfgPath := writeConfigJSON(t, configFile{
		Llm: llmFileConfig{
			URL:          "https://api.example.com/v1/messages",
			AuthToken:    "legacy-static",
			AuthTokenCmd: "printf 'legacy-from-cmd\\n'",
			Model:        "claude-sonnet-4-6",
		},
	})
	var ep ResolvedEndpoint
	var err error
	stderr := captureStderr(t, func() {
		ep, err = ResolveEndpoint(cfgPath)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "legacy-static" {
		t.Errorf("Token = %q, want %q (static auth_token must win)", ep.Token, "legacy-static")
	}
	want := "llm config has both auth_token and auth_token_cmd set; using the static auth_token"
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr %q does not contain warning %q", stderr, want)
	}
}

// (b3) a whitespace-only api_key is a typo, not a credential: it must not shadow
// the command (which used to resolve Token="   " -> 401, command never run), and
// the both-set warning must stay quiet since nothing is really being shadowed.
func TestResolveEndpoint_WhitespaceOnlyStaticKeyUsesCmd(t *testing.T) {
	clearAllEnv(t)
	cfgPath := writeConfigJSON(t, configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKey: "   ", APIKeyCmd: "printf 'sk-from-cmd\\n'", Model: "claude-sonnet-4-6"},
		},
	})
	var ep ResolvedEndpoint
	var err error
	stderr := captureStderr(t, func() {
		ep, err = ResolveEndpoint(cfgPath)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "sk-from-cmd" {
		t.Errorf("Token = %q, want %q (whitespace-only api_key must not shadow api_key_cmd)", ep.Token, "sk-from-cmd")
	}
	if strings.Contains(stderr, "both api_key and api_key_cmd") {
		t.Errorf("warned about a shadowed command that was actually used; stderr: %q", stderr)
	}
}

// (e3b) the same whitespace rule reaches the env-var fallback, which is the last
// source in the chain and had been exempt: a whitespace-only value there used to
// resolve successfully and send `Authorization: Bearer  `, producing an opaque 401
// instead of naming the missing credential.
func TestResolveEndpoint_WhitespaceOnlyEnvVarIsNotACredential(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "   ")
	cfgPath := writeConfigJSON(t, configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {Model: "claude-sonnet-4-6"},
		},
	})
	_, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatal("expected an error: a whitespace-only env var is not a credential")
	}
	if !strings.Contains(err.Error(), "no api_key or api_key_cmd configured") {
		t.Errorf("error %q does not name the missing credential", err.Error())
	}
}

// (e4) same on the legacy path.
func TestResolveEndpoint_LegacyWhitespaceOnlyStaticTokenUsesCmd(t *testing.T) {
	clearAllEnv(t)
	cfgPath := writeConfigJSON(t, configFile{
		Llm: llmFileConfig{
			URL:          "https://api.example.com/v1/messages",
			AuthToken:    "\t\n ",
			AuthTokenCmd: "printf 'legacy-from-cmd\\n'",
			Model:        "claude-sonnet-4-6",
		},
	})
	var ep ResolvedEndpoint
	var err error
	stderr := captureStderr(t, func() {
		ep, err = ResolveEndpoint(cfgPath)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "legacy-from-cmd" {
		t.Errorf("Token = %q, want %q (whitespace-only auth_token must not shadow auth_token_cmd)", ep.Token, "legacy-from-cmd")
	}
	if strings.Contains(stderr, "both auth_token and auth_token_cmd") {
		t.Errorf("warned about a shadowed command that was actually used; stderr: %q", stderr)
	}
}

// (c) custom provider with api_key_cmd resolves (custom providers have no env fallback).
func TestResolveEndpoint_CustomProviderAPIKeyCmd(t *testing.T) {
	clearAllEnv(t)
	cfgPath := writeConfigJSON(t, configFile{
		Provider: "my-gateway",
		CustomProviders: map[string]providerEntryConfig{
			"my-gateway": {
				APIKeyCmd: "printf 'gw-token\\n'",
				URL:       "https://gateway.internal.com/v1",
				Protocol:  "openai",
				Model:     "llama-3-8b",
			},
		},
	})
	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "gw-token" {
		t.Errorf("Token = %q, want %q", ep.Token, "gw-token")
	}
}

// (d) a failing api_key_cmd is a hard error, not a silent fallback.
func TestResolveEndpoint_ProviderAPIKeyCmdFailsHard(t *testing.T) {
	clearAllEnv(t)
	cfgPath := writeConfigJSON(t, configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKeyCmd: "exit 7", Model: "claude-sonnet-4-6"},
		},
	})
	_, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatal("expected hard error from failing api_key_cmd, got nil")
	}
	if !strings.Contains(err.Error(), "api_key_cmd") {
		t.Errorf("error %q does not mention api_key_cmd", err.Error())
	}
}

// (d2) the property the design calls non-negotiable: a misconfigured credential
// command must never silently downgrade to an env var. TestResolveEndpoint_
// ProviderAPIKeyCmdFailsHard runs under clearAllEnv, so it would still pass if
// someone reintroduced an env-var fallback on command failure; this one sets the
// preset's env var so that regression cannot hide.
func TestResolveEndpoint_APIKeyCmdFailureDoesNotFallBackToEnv(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "env-api-key")
	cfgPath := writeConfigJSON(t, configFile{
		Provider: "anthropic",
		Providers: map[string]providerEntryConfig{
			"anthropic": {APIKeyCmd: "exit 7", Model: "claude-sonnet-4-6"},
		},
	})
	ep, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatalf("expected hard error from failing api_key_cmd, got nil (Token %q)", ep.Token)
	}
	if !strings.Contains(err.Error(), "api_key_cmd") {
		t.Errorf("error %q does not mention api_key_cmd", err.Error())
	}
	// Not an assertion on ep: every error path returns a zero ResolvedEndpoint, so
	// ep.Token is "" by construction whenever err != nil. The witness that no
	// fallback happened is err being non-nil at all -- with the env var set, a
	// silent fallback would have returned success.
}

// (e) legacy auth_token_cmd resolves on an otherwise-complete llm block.
func TestResolveEndpoint_LegacyAuthTokenCmd(t *testing.T) {
	clearAllEnv(t)
	cfgPath := writeConfigJSON(t, configFile{
		Llm: llmFileConfig{
			URL:          "https://api.example.com/v1/messages",
			AuthTokenCmd: "printf 'legacy-token\\n'",
			Model:        "claude-sonnet-4-6",
		},
	})
	ep, err := ResolveEndpoint(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Token != "legacy-token" {
		t.Errorf("Token = %q, want %q", ep.Token, "legacy-token")
	}
}

// (e3) legacy path: an otherwise-complete llm block whose auth_token_cmd fails is
// a hard error. The Claude Code env vars are set to prove it does not fall through
// to that strategy -- a failing credential command must not be papered over by a
// lower-priority source.
func TestResolveEndpoint_LegacyAuthTokenCmdFailsHard(t *testing.T) {
	clearAllEnv(t)
	t.Setenv("ANTHROPIC_BASE_URL", "https://cc.example.com")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "cc-env-token")
	t.Setenv("ANTHROPIC_MODEL", "claude-sonnet-4-6")
	cfgPath := writeConfigJSON(t, configFile{
		Llm: llmFileConfig{
			URL:          "https://api.example.com/v1/messages",
			AuthTokenCmd: "exit 9",
			Model:        "claude-sonnet-4-6",
		},
	})
	ep, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatalf("expected hard error from failing auth_token_cmd, got nil (Source %q, Token %q)", ep.Source, ep.Token)
	}
	if !strings.Contains(err.Error(), "auth_token_cmd") {
		t.Errorf("error %q does not mention auth_token_cmd", err.Error())
	}
}

// (f) an incomplete legacy block (missing url) with auth_token_cmd set does NOT
// run the command and falls through to later strategies.
func TestResolveEndpoint_LegacyIncompleteDoesNotRunCmd(t *testing.T) {
	clearAllEnv(t)
	// Command would exit non-zero if ever executed; if it ran, we'd see that
	// error instead of the generic "no valid endpoint" fall-through error.
	cfgPath := writeConfigJSON(t, configFile{
		Llm: llmFileConfig{
			AuthTokenCmd: "exit 9",
			Model:        "claude-sonnet-4-6",
			// URL intentionally omitted -> incomplete
		},
	})
	_, err := ResolveEndpoint(cfgPath)
	if err == nil {
		t.Fatal("expected no-endpoint error, got nil")
	}
	if strings.Contains(err.Error(), "auth_token_cmd") {
		t.Errorf("command should not have run for incomplete legacy config; error: %v", err)
	}
	if !strings.Contains(err.Error(), "no valid LLM endpoint") {
		t.Errorf("expected fall-through no-endpoint error, got: %v", err)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func init() {
	if mode := os.Getenv("OCR_TEST_OAUTH_CHILD"); mode != "" {
		if mode == "auth" {
			_ = json.NewEncoder(os.Stdout).Encode(os.Args[1:])
			os.Exit(0)
		}
		if strings.HasPrefix(mode, "claude-") {
			fakeClaudeClient(mode)
			os.Exit(0)
		}
		os.Exit(2)
	}
}

func fakeOAuthExecutable(t *testing.T, mode string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("OCR_CLAUDE_PATH", executable)
	t.Setenv("OCR_TEST_OAUTH_CHILD", mode)
}

func TestOfficialAuthCommands(t *testing.T) {
	fakeOAuthExecutable(t, "auth")
	for action, flag := range map[string]string{"login": "--claudeai", "status": "status", "logout": "logout"} {
		var output bytes.Buffer
		if err := RunOAuthAuth(context.Background(), ProtocolAnthropicOAuth, action, nil, &output, io.Discard); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), flag) {
			t.Fatalf("args = %s, want %s", output.String(), flag)
		}
	}
	for _, tc := range []struct{ provider, action string }{{"openai", "login"}, {ProtocolAnthropicOAuth, "invalid"}} {
		if RunOAuthAuth(context.Background(), tc.provider, tc.action, nil, io.Discard, io.Discard) == nil {
			t.Fatalf("invalid auth command succeeded: %+v", tc)
		}
	}
	t.Setenv("OCR_CLAUDE_PATH", filepath.Join(t.TempDir(), "missing"))
	if RunOAuthAuth(context.Background(), ProtocolAnthropicOAuth, "login", nil, io.Discard, io.Discard) == nil {
		t.Fatal("missing executable accepted")
	}
}

func TestAnthropicOAuthUsesSubscriptionLogin(t *testing.T) {
	fakeOAuthExecutable(t, "auth")
	t.Setenv("OCR_ANT_PATH", filepath.Join(t.TempDir(), "must-not-use-console"))
	var output bytes.Buffer
	if err := RunOAuthAuth(context.Background(), ProtocolAnthropicOAuth, "login", nil, &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	var args []string
	if err := json.Unmarshal(output.Bytes(), &args); err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != "auth login --claudeai" {
		t.Fatalf("subscription login args = %v", args)
	}
}

func TestOAuthProviderResolution(t *testing.T) {
	name := ProtocolAnthropicOAuth
	path := writeConfig(t, map[string]any{"provider": name, "model": "account-specific-model", "providers": map[string]any{name: map[string]any{}}})
	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ep.AmbientAuth || ep.Token != "" || ep.Protocol != name {
		t.Fatalf("endpoint = %+v", ep)
	}
	for _, field := range []string{"api_key", "api_key_cmd", "url", "auth_header"} {
		path := writeConfig(t, map[string]any{"provider": name, "model": "test", "providers": map[string]any{name: map[string]any{field: "must-not-be-used"}}})
		if _, err := ResolveEndpoint(path); err == nil || !strings.Contains(err.Error(), "official OAuth credentials") {
			t.Fatalf("%s: error = %v", field, err)
		}
	}
	t.Setenv("OCR_LLM_EXTRA_HEADERS", "Authorization=Bearer must-not-be-used")
	for _, opts := range []ResolveOptions{{}, {Provider: name}} {
		if _, err := ResolveEndpointWithOptions(path, opts); err == nil || !strings.Contains(err.Error(), "OCR_LLM_EXTRA_HEADERS") {
			t.Fatalf("global OAuth header override error = %v", err)
		}
	}
}

func TestOAuthRelativeExecutableRoundTrip(t *testing.T) {
	fakeOAuthExecutable(t, "claude-text")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Chdir(dir)
	relative, err := filepath.Rel(dir, executable)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("OCR_CLAUDE_PATH", relative)
	client := NewLLMClient(ResolvedEndpoint{Protocol: ProtocolAnthropicOAuth, Model: "test-model"}, nil, nil)
	response, err := client.CompletionsWithCtx(context.Background(), claudeTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	if response.Content() != "review complete" {
		t.Fatalf("unexpected response: %q", response.Content())
	}
}

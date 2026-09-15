// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
)

func TestMCPRootAndReadOnlyOutputBranches(t *testing.T) {
	setupMCPTestHome(t, &Config{})
	setMCPTestInteractive(t, false)
	root := newMCPCommand()
	out := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(new(bytes.Buffer))
	if err := runMCPRoot(root, nil); err != nil {
		t.Fatalf("runMCPRoot: %v", err)
	}
	for _, want := range []string{"No MCP servers configured", "Manage Model Context Protocol servers", "Available Commands"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("root output missing %q: %s", want, out)
		}
	}

	enabled := true
	cfg := &Config{
		MCP: &ocrmcp.MCPConfig{Version: 1, Enabled: &enabled, DefaultPermission: ocrmcp.PermissionAsk},
		MCPServers: map[string]MCPServerConfig{
			"z-remote": {
				Type: "remote", URL: "https://user:pass@example.test/mcp?token=secret",
				Headers: map[string]string{"X-Token": "${TOKEN}"}, Env: []string{"IGNORED=${TOKEN}"},
			},
			"a-local": {
				Type: "stdio", Command: "server", Args: []string{"--token", "secret"},
				Env: []string{"TOKEN=${TOKEN}"}, Tools: []string{"read"},
				ToolPermissions:      map[string]ocrmcp.Permission{"read": ocrmcp.PermissionAsk},
				ToolDefinitionSHA256: map[string]string{"read": mcpTestFingerprint},
			},
		},
	}
	path := setupMCPTestHome(t, cfg)
	t.Setenv("OCR_CONFIG_PATH", path)

	cmd, listOut, _ := newMCPTestCommand("")
	if err := runMCPList(cmd, false); err != nil {
		t.Fatal(err)
	}
	if strings.Index(listOut.String(), "a-local") > strings.Index(listOut.String(), "z-remote") {
		t.Fatalf("list is not sorted: %s", listOut)
	}
	cmd, showOut, _ := newMCPTestCommand("")
	if err := runMCPShow(cmd, "a-local", false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Environment names: TOKEN", "Tools:", "fingerprint=recorded"} {
		if !strings.Contains(showOut.String(), want) {
			t.Errorf("show output missing %q: %s", want, showOut)
		}
	}
	cmd, showOut, _ = newMCPTestCommand("")
	if err := runMCPShow(cmd, "z-remote", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(showOut.String(), "Header names: X-Token") || !strings.Contains(showOut.String(), "Tools: none enabled") {
		t.Fatalf("remote show output = %s", showOut)
	}
	if err := runMCPShow(cmd, "missing", false); err == nil {
		t.Fatal("show accepted an unknown server")
	}
}

func TestMCPDiscoverSuccessAndSafeFailures(t *testing.T) {
	path := setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{
		"local": {Type: "stdio", Command: "example-mcp"},
	}})
	t.Setenv("OCR_CONFIG_PATH", path)
	setMCPTestInteractive(t, false)
	setMCPTestDiscovery(t, func(ctx context.Context, name string, server MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
		if ctx == nil || name != "local" || server.Command != "example-mcp" {
			t.Fatalf("unexpected discovery request: %v %q %+v", ctx, name, server)
		}
		return []ocrmcp.DiscoveredTool{
			{Name: "zeta", Description: "line\nmetadata", DefinitionSHA256: strings.Repeat("b", 64)},
			{Name: "alpha", DefinitionSHA256: strings.Repeat("a", 64)},
		}, nil
	})

	cmd, out, _ := newMCPTestCommand("")
	if err := runMCPDiscover(cmd, "local", false); err != nil {
		t.Fatal(err)
	}
	if strings.Index(out.String(), "alpha") > strings.Index(out.String(), "zeta") || !strings.Contains(out.String(), "untrusted") {
		t.Fatalf("discovery output = %s", out)
	}
	cmd, out, _ = newMCPTestCommand("")
	if err := runMCPDiscoverConfirmed(cmd, "local", true, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"untrusted_metadata": true`) {
		t.Fatalf("JSON discovery output = %s", out)
	}
	if err := runMCPDiscover(cmd, "missing", false); err == nil {
		t.Fatal("discovery accepted an unknown server")
	}

	if got := sanitizeMCPDiscoveryError("local", nil); got != nil {
		t.Fatalf("nil discovery error = %v", got)
	}
	if got := sanitizeMCPDiscoveryError("local", context.DeadlineExceeded); got == nil || !strings.Contains(got.Error(), "timed out") {
		t.Fatalf("deadline error = %v", got)
	}
	secretErr := sanitizeMCPDiscoveryError("local", errors.New("Bearer discovery-secret"))
	if secretErr == nil || strings.Contains(secretErr.Error(), "discovery-secret") {
		t.Fatalf("generic discovery error leaked details: %v", secretErr)
	}

	setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
		return nil, errors.New("remote secret")
	})
	if _, err := discoverToolsWithTimeout(nil, "local", MCPServerConfig{}); err == nil {
		t.Fatal("discovery error was suppressed")
	}
	if _, err := discoverMCPServerTools(context.Background(), "bad", MCPServerConfig{Type: "remote", URL: "://bad"}); err == nil {
		t.Fatal("invalid connection reached discovery")
	}
}

func TestMCPEnableDisableAndRemoveTransactions(t *testing.T) {
	disabled := false
	setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{
		"local": {
			Type: "stdio", Command: "example-mcp", Enabled: &disabled, Setup: "legacy setup",
			Tools: []string{"read"}, ToolDefinitionSHA256: map[string]string{"read": mcpTestFingerprint},
		},
	}})
	setMCPTestInteractive(t, false)

	cmd, out, _ := newMCPTestCommand("")
	cmd.Use = "enable"
	if err := runMCPEnable(cmd, "local", true, true); err != nil {
		t.Fatal(err)
	}
	server := loadMCPTestConfig(t).MCPServers["local"]
	if server.Enabled == nil || !*server.Enabled || server.Setup != "" {
		t.Fatalf("enabled server = %+v", server)
	}
	if !strings.Contains(out.String(), "Enabled") || !strings.Contains(out.String(), "legacy setup") {
		t.Fatalf("enable output = %s", out)
	}

	cmd, out, _ = newMCPTestCommand("")
	cmd.Use = "disable"
	if err := runMCPEnable(cmd, "local", false, true); err != nil {
		t.Fatal(err)
	}
	server = loadMCPTestConfig(t).MCPServers["local"]
	if server.Enabled == nil || *server.Enabled || !strings.Contains(out.String(), "Disabled") {
		t.Fatalf("disabled server/output = %+v / %s", server, out)
	}
	if err := runMCPEnable(cmd, "missing", true, true); err == nil {
		t.Fatal("enable accepted an unknown server")
	}
	if err := runMCPEnable(cmd, "local", true, false); err == nil {
		t.Fatal("non-interactive enable omitted --yes")
	}

	cmd, out, _ = newMCPTestCommand("")
	if err := runMCPRemove(cmd, "local", true); err != nil {
		t.Fatal(err)
	}
	if len(loadMCPTestConfig(t).MCPServers) != 0 || !strings.Contains(out.String(), "Removed") {
		t.Fatalf("remove did not persist/output: %s", out)
	}
	if err := runMCPRemove(cmd, "local", true); err == nil {
		t.Fatal("remove accepted an unknown server")
	}
	if err := runMCPRemove(cmd, "local", false); err == nil {
		t.Fatal("non-interactive remove omitted --yes")
	}
}

func TestMCPInteractiveMutationCancellationAndEditGate(t *testing.T) {
	setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{
		"local": {Type: "stdio", Command: "server"},
	}})
	setMCPTestInteractive(t, true)

	cmd, out, _ := newMCPTestCommand("n\n")
	cmd.Use = "enable"
	if err := runMCPEnable(cmd, "local", true, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Cancelled") {
		t.Fatalf("enable cancellation output = %s", out)
	}
	if server := loadMCPTestConfig(t).MCPServers["local"]; server.Enabled != nil {
		t.Fatalf("cancelled enable changed server: %+v", server)
	}

	cmd, out, _ = newMCPTestCommand("n\n")
	if err := runMCPRemove(cmd, "local", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Cancelled") || len(loadMCPTestConfig(t).MCPServers) != 1 {
		t.Fatalf("cancelled removal changed config/output: %s", out)
	}

	setMCPTestInteractive(t, false)
	edit := newMCPEditCommand()
	edit.SetArgs([]string{"local"})
	if err := edit.Execute(); err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("non-interactive edit error = %v", err)
	}
}

func TestMCPCommandHelperBranches(t *testing.T) {
	for input, want := range map[string]bool{
		"localhost": true, "api.localhost.": true, "127.0.0.1": true,
		"::1": true, "example.test": false, "localhost.example": false,
	} {
		if got := mcpLoopbackHost(input); got != want {
			t.Errorf("mcpLoopbackHost(%q) = %v, want %v", input, got, want)
		}
	}
	for input, want := range map[string]string{"": "d", "enable": "Enabled", "disable": "Disabled", "archive": "Archived"} {
		if got := mcpPastTense(input); got != want {
			t.Errorf("mcpPastTense(%q) = %q, want %q", input, got, want)
		}
	}
	if got := mcpTitle("server"); got != "Server" {
		t.Errorf("mcpTitle = %q", got)
	}
	if got := mcpTitle(""); got != "" {
		t.Errorf("empty mcpTitle = %q", got)
	}

	remote := MCPServerConfig{Type: "remote", URL: "https://user:pass@example.test/mcp?token=secret#fragment"}
	if got := redactedMCPEndpoint(remote); got != "https://example.test/mcp" {
		t.Errorf("remote endpoint = %q", got)
	}
	for _, server := range []MCPServerConfig{
		{Type: "remote", URL: "://bad"},
		{Type: "stdio"},
		{Type: "stdio", Command: "server"},
		{Type: "stdio", Command: "server", Args: []string{"secret"}},
	} {
		if got := redactedMCPEndpoint(server); got == "" || strings.Contains(got, "secret") {
			t.Errorf("unsafe/empty endpoint for %+v: %q", server, got)
		}
	}

	if names := mcpEnvironmentNames([]string{" Z =one", "=bad", "A=two", "\x00=bad"}); len(names) != 2 || names[0] != "A" || names[1] != "Z" {
		t.Errorf("environment names = %#v", names)
	}
	if names := mcpHeaderNames(map[string]string{" Z ": "one", "": "bad", "A": "two"}); len(names) != 2 || names[0] != "A" || names[1] != "Z" {
		t.Errorf("header names = %#v", names)
	}
	if err := writeMCPJSON(&failingWriter{err: errors.New("boom")}, map[string]string{"x": "y"}); err == nil {
		t.Fatal("writeMCPJSON suppressed writer error")
	}

	badFingerprint := MCPServerConfig{Tools: []string{"x"}, ToolDefinitionSHA256: map[string]string{"x": strings.Repeat("G", 64)}}
	if !mcpToolFingerprintsNeedReview(badFingerprint) || mcpToolFingerprintValid(badFingerprint, "x") {
		t.Fatal("invalid fingerprint was treated as current")
	}
	if err := validateMCPPersistentAllow(nil); err != nil {
		t.Fatalf("nil persistent allow config: %v", err)
	}
	if mcpEnabled(&ocrmcp.MCPConfig{Enabled: boolPointer(false)}) || !mcpEnabled(nil) {
		t.Fatal("global enabled helpers returned the wrong result")
	}

	for _, name := range mcpCIEnvironmentVariables {
		t.Setenv(name, "")
	}
	t.Setenv("TERM", "dumb")
	if defaultMCPInteractiveTerminal() {
		t.Fatal("TERM=dumb was treated as interactive")
	}
	t.Setenv("TERM", "xterm")
	t.Setenv("CI", "yes")
	if defaultMCPInteractiveTerminal() {
		t.Fatal("CI was treated as interactive")
	}
}

func TestMCPAddValidationAndConfigLoadBranches(t *testing.T) {
	setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{
		"exists": {Type: "stdio", Command: "server"},
	}})
	setMCPTestInteractive(t, false)
	cmd, _, _ := newMCPTestCommand("")
	if err := runMCPAdd(cmd, nil, mcpAddOptions{yes: true}); err == nil {
		t.Fatal("add accepted a missing name")
	}
	if err := runMCPAdd(cmd, []string{"new"}, mcpAddOptions{}); err == nil {
		t.Fatal("add omitted --yes")
	}
	if err := runMCPAdd(cmd, []string{"bad name"}, mcpAddOptions{yes: true, transport: "stdio", command: "server"}); err == nil {
		t.Fatal("add accepted an invalid name")
	}
	if err := runMCPAdd(cmd, []string{"exists"}, mcpAddOptions{yes: true, transport: "stdio", command: "server"}); err == nil {
		t.Fatal("add replaced an existing server")
	}
	if _, err := mcpServerFromAddOptions(mcpAddOptions{}); err == nil {
		t.Fatal("empty add options were accepted")
	}
	if _, err := parseMCPHeaderFlags([]string{"Broken"}); err == nil {
		t.Fatal("malformed header flag was accepted")
	}
	if _, err := parseMCPHeaderFlags([]string{"X=${A}", "x=${B}"}); err == nil {
		t.Fatal("duplicate header flag was accepted")
	}
	if headers, err := parseMCPHeaderFlags(nil); err != nil || headers != nil {
		t.Fatalf("empty headers = %#v, %v", headers, err)
	}

	path := setupMCPTestHome(t, &Config{})
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OCR_CONFIG_PATH", path)
	cfg, err := loadReadOnlyMCPConfig()
	if err != nil || cfg == nil {
		t.Fatalf("missing read-only config = %#v, %v", cfg, err)
	}
}

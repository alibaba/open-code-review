// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
	"github.com/spf13/cobra"
)

const mcpTestFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestMCPCommandTree(t *testing.T) {
	cmd := newMCPCommand()
	want := map[string]bool{
		"add": false, "list": false, "show": false, "edit": false,
		"discover": false, "tools": false, "permissions": false,
		"enable": false, "disable": false, "remove": false,
	}
	for _, child := range cmd.Commands() {
		if _, ok := want[child.Name()]; ok {
			want[child.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing ocr mcp %s command", name)
		}
	}
}

func TestMCPNonInteractiveAddIsDisabledAndEmpty(t *testing.T) {
	setupMCPTestHome(t, &Config{})
	setMCPTestInteractive(t, false)
	called := 0
	setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
		called++
		return nil, nil
	})
	cmd, output, _ := newMCPTestCommand("")
	err := runMCPAdd(cmd, []string{"local"}, mcpAddOptions{
		transport: "stdio", command: "example-mcp", args: []string{"--safe"}, yes: true,
	})
	if err != nil {
		t.Fatalf("runMCPAdd: %v", err)
	}
	if called != 0 {
		t.Fatalf("non-interactive add performed discovery %d time(s)", called)
	}
	cfg := loadMCPTestConfig(t)
	server := cfg.MCPServers["local"]
	if server.Enabled == nil || *server.Enabled {
		t.Fatal("new non-interactive server must be disabled")
	}
	if len(server.Tools) != 0 {
		t.Fatalf("new non-interactive server tools = %v, want none", server.Tools)
	}
	if cfg.MCP == nil || cfg.MCP.DefaultPermission != ocrmcp.PermissionAsk || cfg.MCP.ApprovalTimeoutSeconds != 60 {
		t.Fatalf("global MCP defaults = %+v", cfg.MCP)
	}
	if !strings.Contains(output.String(), "disabled") || !strings.Contains(output.String(), "no enabled tools") {
		t.Errorf("output does not explain safe state: %q", output.String())
	}
	path, _ := defaultConfigPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows file modes do not represent ACLs; match the provider config tests.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("config mode = %o, want 600", info.Mode().Perm())
	}
}

func TestMCPRemotePlainHTTPRequiresExplicitFlag(t *testing.T) {
	_, err := mcpServerFromAddOptions(mcpAddOptions{
		transport: "remote", url: "http://example.com/mcp",
	})
	if err == nil || !strings.Contains(err.Error(), "allow-insecure-http") {
		t.Fatalf("plain HTTP error = %v", err)
	}
	server, err := mcpServerFromAddOptions(mcpAddOptions{
		transport: "remote", url: "http://example.com/mcp", allowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("explicit plain HTTP: %v", err)
	}
	if !server.AllowInsecureHTTP {
		t.Fatal("allow_insecure_http was not persisted in draft")
	}
	if _, err := mcpServerFromAddOptions(mcpAddOptions{transport: "remote", url: "http://127.0.0.1:8080/mcp"}); err != nil {
		t.Fatalf("loopback HTTP should not need an override: %v", err)
	}
	for name, opts := range map[string]mcpAddOptions{
		"fragment":        {transport: "remote", url: "https://example.com/mcp#hidden"},
		"reserved header": {transport: "remote", url: "https://example.com/mcp", headers: []string{"MCP-Session-Id=value"}},
		"invalid header":  {transport: "remote", url: "https://example.com/mcp", headers: []string{"Bad Header=value"}},
		"duplicate header": {transport: "remote", url: "https://example.com/mcp", headers: []string{
			"X-Token=${FIRST_TOKEN}", "x-token=${SECOND_TOKEN}",
		}},
	} {
		if _, err := mcpServerFromAddOptions(opts); err == nil {
			t.Errorf("%s connection was accepted", name)
		}
	}
}

func TestMCPNewCredentialsRequireEnvironmentReferences(t *testing.T) {
	t.Parallel()

	if _, err := mcpServerFromAddOptions(mcpAddOptions{
		transport: "stdio", command: "example-mcp", env: []string{"TOKEN=literal-secret"},
	}); err == nil || !strings.Contains(err.Error(), "${ENV_NAME}") || strings.Contains(err.Error(), "literal-secret") {
		t.Fatalf("literal stdio credential error = %v", err)
	}
	if _, err := mcpServerFromAddOptions(mcpAddOptions{
		transport: "stdio", command: "example-mcp", env: []string{"TOKEN=${SOURCE_TOKEN}"},
	}); err != nil {
		t.Fatalf("environment reference was rejected: %v", err)
	}
	if _, err := mcpServerFromAddOptions(mcpAddOptions{
		transport: "remote", url: "https://example.com/mcp", headers: []string{"Authorization=Bearer literal-secret"},
	}); err == nil || !strings.Contains(err.Error(), "${ENV_NAME}") || strings.Contains(err.Error(), "literal-secret") {
		t.Fatalf("literal remote credential error = %v", err)
	}
	if _, err := mcpServerFromAddOptions(mcpAddOptions{
		transport: "remote", url: "https://example.com/mcp", headers: []string{"Authorization=Bearer ${MCP_TOKEN}"},
	}); err != nil {
		t.Fatalf("header environment reference was rejected: %v", err)
	}

	legacy := MCPServerConfig{Env: []string{"TOKEN=literal-secret"}}
	if !mcpServerSummaryFor("legacy", &Config{}, legacy).LegacyLiteralCredentials {
		t.Fatal("legacy literal credential was not marked deprecated")
	}
}

func TestMCPConfigSetPolicyAndInsecureHTTP(t *testing.T) {
	cfg := &Config{}
	for key, value := range map[string]string{
		"mcp.enabled":                            "false",
		"mcp.default_permission":                 "deny",
		"mcp.approval_timeout_seconds":           "90",
		"mcp_servers.remote.allow_insecure_http": "true",
		"mcp_servers.remote.default_permission":  "inherit",
	} {
		if err := setConfigValue(cfg, key, value); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
	if cfg.MCP == nil || cfg.MCP.Enabled == nil || *cfg.MCP.Enabled || cfg.MCP.DefaultPermission != ocrmcp.PermissionDeny || cfg.MCP.ApprovalTimeoutSeconds != 90 {
		t.Fatalf("global config = %+v", cfg.MCP)
	}
	server := cfg.MCPServers["remote"]
	if !server.AllowInsecureHTTP || server.DefaultPermission != ocrmcp.PermissionInherit {
		t.Fatalf("server config = %+v", server)
	}
	for _, value := range []string{"0", "601", "not-a-number"} {
		if err := setConfigValue(&Config{}, "mcp.approval_timeout_seconds", value); err == nil {
			t.Errorf("timeout %q was accepted", value)
		}
	}
	if got := configDisplayValue("mcp_servers.remote.headers", `{"Authorization":"secret"}`); got != "***" {
		t.Errorf("masked header display = %q", got)
	}
}

func TestMCPConfigSetCannotBypassPersistentAllowEntryPoint(t *testing.T) {
	t.Parallel()

	for key, value := range map[string]string{
		"mcp.default_permission":               "allow",
		"mcp_servers.local.default_permission": "allow",
		"mcp_servers.local.tool_permissions":   `{"read":"allow"}`,
	} {
		cfg := &Config{MCPServers: map[string]MCPServerConfig{
			"local": {Tools: []string{"read"}},
		}}
		err := setConfigValue(cfg, key, value)
		if err == nil || !strings.Contains(err.Error(), "ocr mcp permissions") {
			t.Errorf("setConfigValue(%q, %q) error = %v, want permissions-only rejection", key, value, err)
		}
	}
}

func TestMCPListAndShowNeverLeakConnectionSecrets(t *testing.T) {
	secretValues := []string{"header-canary", "env-canary", "arg-canary", "query-canary", "user-canary", "pass-canary"}
	setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{
		"remote": {
			Type: "remote", URL: "https://user-canary:pass-canary@example.com/mcp?token=query-canary",
			Headers: map[string]string{"Authorization": "header-canary"},
		},
		"stdio": {
			Type: "stdio", Command: "example-mcp", Args: []string{"arg-canary"}, Env: []string{"TOKEN=env-canary"},
		},
	}})
	for _, asJSON := range []bool{false, true} {
		cmd, output, _ := newMCPTestCommand("")
		if err := runMCPList(cmd, asJSON); err != nil {
			t.Fatalf("runMCPList: %v", err)
		}
		for _, secret := range secretValues {
			if strings.Contains(output.String(), secret) {
				t.Errorf("list json=%v leaked %q: %q", asJSON, secret, output.String())
			}
		}
	}
	for _, name := range []string{"remote", "stdio"} {
		cmd, output, _ := newMCPTestCommand("")
		if err := runMCPShow(cmd, name, true); err != nil {
			t.Fatalf("runMCPShow(%s): %v", name, err)
		}
		for _, secret := range secretValues {
			if strings.Contains(output.String(), secret) {
				t.Errorf("show %s leaked %q: %q", name, secret, output.String())
			}
		}
	}
}

func TestMCPConnectionPreviewRedactsCredentialArguments(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	printMCPConnectionPreview(&output, MCPServerConfig{
		Type: "stdio", Command: "example-mcp",
		Args: []string{
			"--token=first-canary", "--api-key", "second-canary",
			"https://example.com/mcp?credential=query-canary", "visible",
		},
	})
	for _, secret := range []string{"first-canary", "second-canary", "query-canary"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("connection preview leaked %q: %s", secret, output.String())
		}
	}
	for _, visible := range []string{"--token", "--api-key", "credential", "visible", "<redacted>"} {
		if !strings.Contains(output.String(), visible) {
			t.Errorf("connection preview omitted safe structure %q: %s", visible, output.String())
		}
	}
}

func TestMCPDiscoveryConfirmationFailsClosed(t *testing.T) {
	setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{
		"local": {Type: "stdio", Command: "example-mcp"},
	}})
	called := 0
	setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
		called++
		return nil, nil
	})

	setMCPTestInteractive(t, false)
	cmd, _, _ := newMCPTestCommand("")
	if err := runMCPDiscoverConfirmed(cmd, "local", false, false); err == nil {
		t.Fatal("non-interactive discovery without --yes succeeded")
	}
	if called != 0 {
		t.Fatalf("non-interactive refusal still connected %d time(s)", called)
	}

	setMCPTestInteractive(t, true)
	cmd, output, preview := newMCPTestCommand("n\n")
	if err := runMCPDiscoverConfirmed(cmd, "local", false, false); err != nil {
		t.Fatalf("interactive cancellation: %v", err)
	}
	if called != 0 {
		t.Fatalf("cancelled discovery still connected %d time(s)", called)
	}
	if !strings.Contains(preview.String(), `Command: "example-mcp"`) || !strings.Contains(output.String(), "not started") {
		t.Errorf("preview/output = %q / %q", preview.String(), output.String())
	}
}

func TestMCPToolsConfirmationFailsClosed(t *testing.T) {
	setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{
		"local": {Type: "stdio", Command: "example-mcp"},
	}})
	called := 0
	setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
		called++
		return nil, nil
	})
	setMCPTestInteractive(t, true)
	cmd, output, preview := newMCPTestCommand("n\n")
	if err := runMCPTools(cmd, "local", mcpToolsOptions{}); err != nil {
		t.Fatalf("interactive tools cancellation: %v", err)
	}
	if called != 0 {
		t.Fatalf("cancelled tools flow connected %d time(s)", called)
	}
	if !strings.Contains(preview.String(), `Command: "example-mcp"`) || !strings.Contains(output.String(), "not started") {
		t.Errorf("preview/output = %q / %q", preview.String(), output.String())
	}

	setMCPTestInteractive(t, false)
	cmd, _, _ = newMCPTestCommand("")
	if err := runMCPTools(cmd, "local", mcpToolsOptions{enable: []string{"read"}}); err == nil {
		t.Fatal("non-interactive tools flow without --yes succeeded")
	}
	if called != 0 {
		t.Fatalf("non-interactive tools refusal connected %d time(s)", called)
	}
}

func TestMCPToolsEnableIsExactAndTransactional(t *testing.T) {
	setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{
		"local": {Type: "stdio", Command: "example-mcp", Setup: "legacy-setup"},
	}})
	setMCPTestInteractive(t, false)
	setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
		return []ocrmcp.DiscoveredTool{
			{Name: "read", DefinitionSHA256: mcpTestFingerprint},
			{Name: "write", DefinitionSHA256: strings.Repeat("a", 64)},
		}, nil
	})
	cmd, _, _ := newMCPTestCommand("")
	if err := runMCPTools(cmd, "local", mcpToolsOptions{yes: true, enable: []string{"read"}}); err != nil {
		t.Fatalf("runMCPTools: %v", err)
	}
	cfg := loadMCPTestConfig(t)
	server := cfg.MCPServers["local"]
	if len(server.Tools) != 1 || server.Tools[0] != "read" {
		t.Fatalf("tools = %v, want [read]", server.Tools)
	}
	if server.ToolPermissions["read"] != ocrmcp.PermissionAsk {
		t.Errorf("permission = %q, want ask", server.ToolPermissions["read"])
	}
	if server.ToolDefinitionSHA256["read"] != mcpTestFingerprint {
		t.Errorf("fingerprint = %q", server.ToolDefinitionSHA256["read"])
	}
	if server.Setup != "" {
		t.Errorf("managed server retained legacy setup %q", server.Setup)
	}

	path, _ := defaultConfigPath()
	before, _ := os.ReadFile(path)
	if err := runMCPTools(cmd, "local", mcpToolsOptions{yes: true, enable: []string{"READ"}}); err == nil {
		t.Fatal("unknown case-variant tool was accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("failed tool change modified config")
	}
}

func TestMCPToolDefinitionChangesResetPersistentAllow(t *testing.T) {
	t.Parallel()

	oldFingerprint := strings.Repeat("a", 64)
	newFingerprint := strings.Repeat("b", 64)
	server := MCPServerConfig{
		Type: "stdio", Command: "example-mcp", Tools: []string{"read"},
		ToolPermissions:      map[string]ocrmcp.Permission{"read": ocrmcp.PermissionAllow},
		ToolDefinitionSHA256: map[string]string{"read": oldFingerprint},
	}
	discovered := map[string]ocrmcp.DiscoveredTool{
		"read": {Name: "read", DefinitionSHA256: newFingerprint},
	}
	if err := applyMCPToolChanges(&server, discovered, []string{"read"}, nil); err != nil {
		t.Fatal(err)
	}
	if server.ToolPermissions["read"] != ocrmcp.PermissionAsk || server.ToolDefinitionSHA256["read"] != newFingerprint {
		t.Fatalf("changed definition retained allow or wrong fingerprint: %+v", server)
	}

	server.ToolPermissions["read"] = ocrmcp.PermissionAllow
	server.ToolDefinitionSHA256["read"] = oldFingerprint
	markMCPDefinitionDrift(&server, discovered)
	if server.ToolPermissions["read"] != ocrmcp.PermissionAsk || server.ToolDefinitionSHA256["read"] != "" {
		t.Fatalf("unaccepted drift did not become ask/needs-review: %+v", server)
	}
	cfg := &Config{
		MCP:        &ocrmcp.MCPConfig{Version: 1, DefaultPermission: ocrmcp.PermissionAllow},
		MCPServers: map[string]MCPServerConfig{"local": server},
	}
	if err := validateMCPPersistentAllow(cfg); err != nil {
		t.Fatalf("safe ask override should permit saving drift state: %v", err)
	}
}

func TestMCPPermissionsCannotExpandToolsOrAllowWithoutFingerprint(t *testing.T) {
	setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{
		"local": {Type: "stdio", Command: "example-mcp", Tools: []string{"read"}},
	}})
	setMCPTestInteractive(t, false)
	cmd, _, _ := newMCPTestCommand("")
	if err := runMCPPermissions(cmd, "local", mcpPermissionsOptions{yes: true, toolPermissions: []string{"write=allow"}}); err == nil {
		t.Fatal("permission expanded the tool allowlist")
	}
	if err := runMCPPermissions(cmd, "local", mcpPermissionsOptions{yes: true, toolPermissions: []string{"read=allow"}}); err == nil || !strings.Contains(err.Error(), "definition fingerprint") {
		t.Fatalf("allow without fingerprint error = %v", err)
	}

	cfg := loadMCPTestConfig(t)
	server := cfg.MCPServers["local"]
	server.ToolDefinitionSHA256 = map[string]string{"read": mcpTestFingerprint}
	cfg.MCPServers["local"] = server
	path, _ := defaultConfigPath()
	if err := saveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	if err := runMCPPermissions(cmd, "local", mcpPermissionsOptions{yes: true, toolPermissions: []string{"read=allow"}}); err != nil {
		t.Fatalf("allow with fingerprint: %v", err)
	}
	if got := loadMCPTestConfig(t).MCPServers["local"].ToolPermissions["read"]; got != ocrmcp.PermissionAllow {
		t.Errorf("permission = %q, want allow", got)
	}
}

func TestMCPConnectionWizardCancelWritesNothing(t *testing.T) {
	path := setupMCPTestHome(t, &Config{Language: "English"})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	setMCPTestInteractive(t, true)
	called := 0
	setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
		called++
		return nil, nil
	})
	cmd, output, _ := newMCPTestCommand("cancel\n")
	if err := runMCPConnectionWizard(cmd, "new", false, mcpAddOptions{}); err != nil {
		t.Fatalf("cancel wizard: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("cancelled wizard modified config")
	}
	if called != 0 {
		t.Fatalf("cancelled wizard connected %d time(s)", called)
	}
	if !strings.Contains(output.String(), "No changes saved") {
		t.Errorf("output = %q", output.String())
	}
}

func TestMCPInteractiveAndCIHelpers(t *testing.T) {
	for value, want := range map[string]bool{"": false, "0": false, "false": false, "no": false, "1": true, "true": true} {
		if got := mcpTruthyEnvironment(value); got != want {
			t.Errorf("mcpTruthyEnvironment(%q) = %v, want %v", value, got, want)
		}
	}
	for _, name := range mcpCIEnvironmentVariables {
		t.Setenv(name, "")
	}
	if mcpCIEnvironment() {
		t.Fatal("empty CI variables detected as CI")
	}
	t.Setenv("CI", "true")
	if !mcpCIEnvironment() {
		t.Fatal("CI=true was not detected")
	}
}

func TestMCPApprovalDefaultsToDenyAndRedactsArguments(t *testing.T) {
	invocation := ocrmcp.Invocation{
		Grant: ocrmcp.ToolGrant{
			ID:                   ocrmcp.ToolID{Server: "server", Name: "tool"},
			UntrustedDescription: "server text\x1b[31m",
		},
		Arguments: map[string]any{
			"query": "visible", "api_key": "key-canary",
			"nested": map[string]any{"authorization": "auth-canary", "value": "safe"},
		},
	}
	model := newMCPApprovalModel(invocation)
	if model.selected != 2 {
		t.Fatalf("default selected choice = %d, want Deny once", model.selected)
	}
	for _, secret := range []string{"key-canary", "auth-canary"} {
		if strings.Contains(model.arguments, secret) {
			t.Errorf("approval arguments leaked %q: %s", secret, model.arguments)
		}
	}
	if !strings.Contains(model.arguments, "visible") || !strings.Contains(model.arguments, "[redacted]") {
		t.Errorf("approval arguments = %s", model.arguments)
	}
	view := model.View().Content
	if strings.Contains(view, "\x1b") || !strings.Contains(view, "Untrusted server description") {
		t.Errorf("approval view did not sanitize/label description: %q", view)
	}
}

func TestMCPManagementBubbleTeaDefaultsAndCancellation(t *testing.T) {
	t.Parallel()

	confirmation := newMCPManagementConfirmModel("Connect?")
	updated, command := confirmation.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil {
		t.Fatal("Enter did not quit the confirmation model")
	}
	confirmed := updated.(mcpManagementConfirmModel)
	if !confirmed.submitted || confirmed.cancelled || confirmed.selected != 0 {
		t.Fatalf("default confirmation = %+v, want submitted No", confirmed)
	}

	text := newMCPManagementTextModel("Server", "fallback")
	updated, command = text.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if command == nil || !updated.(mcpManagementTextModel).cancelled {
		t.Fatal("Esc did not cancel the text prompt")
	}
	secretInput := newMCPManagementTextModel("Environment entries", "")
	if secretInput.input.EchoMode != textinput.EchoPassword {
		t.Fatal("credential management prompt is not masked")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runMCPManagementTextPrompt(ctx, "Server", "", strings.NewReader(""), io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Bubble Tea prompt error = %v, want context.Canceled", err)
	}
}

func TestMCPStatusNeedsReviewWithoutCurrentFingerprint(t *testing.T) {
	cfg := &Config{MCPServers: map[string]MCPServerConfig{
		"local": {Type: "stdio", Command: "example-mcp", Tools: []string{"read"}},
	}}
	summary := mcpServerSummaryFor("local", cfg, cfg.MCPServers["local"])
	if got := summary.Status; got != mcpStatusNeedsReview {
		t.Errorf("status = %q, want needs-review", got)
	}
	if got := summary.FingerprintStatus["read"]; got != mcpFingerprintNeedsReview {
		t.Errorf("fingerprint status = %q, want needs-review", got)
	}
	server := cfg.MCPServers["local"]
	server.ToolDefinitionSHA256 = map[string]string{"read": mcpTestFingerprint}
	cfg.MCPServers["local"] = server
	summary = mcpServerSummaryFor("local", cfg, server)
	if got := summary.Status; got != mcpStatusReady {
		t.Errorf("status = %q, want ready", got)
	}
	if got := summary.FingerprintStatus["read"]; got != mcpFingerprintRecorded {
		t.Errorf("fingerprint status = %q, want recorded", got)
	}

	server.ToolPermissions = map[string]ocrmcp.Permission{"other": ocrmcp.PermissionAllow}
	summary = mcpServerSummaryFor("local", cfg, server)
	if summary.Status != mcpStatusError || summary.ErrorCategory != "invalid_server_policy" {
		t.Fatalf("invalid summary = %+v", summary)
	}
}

func setupMCPTestHome(t *testing.T, cfg *Config) string {
	t.Helper()
	setTestHome(t, t.TempDir())
	path, err := defaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	return path
}

func loadMCPTestConfig(t *testing.T) *Config {
	t.Helper()
	path, err := defaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := loadOrCreateConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func setMCPTestInteractive(t *testing.T, interactive bool) {
	t.Helper()
	previous := mcpInteractiveTerminal
	mcpInteractiveTerminal = func() bool { return interactive }
	t.Cleanup(func() { mcpInteractiveTerminal = previous })
}

func setMCPTestDiscovery(t *testing.T, discover func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error)) {
	t.Helper()
	previous := mcpDiscoverServerTools
	mcpDiscoverServerTools = discover
	t.Cleanup(func() { mcpDiscoverServerTools = previous })
}

func newMCPTestCommand(input string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	cmd := &cobra.Command{Use: "test"}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd, stdout, stderr
}

func TestSaveConfigUsesAtomicPrivateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")
	if err := saveConfig(path, &Config{Language: "English"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows file modes do not represent ACLs; match the provider config tests.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", info.Mode().Perm())
	}
	temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".ocr-config-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Errorf("temporary files remain after save: %v", temps)
	}
}

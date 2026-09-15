// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
)

func setupEnter(m mcpSetupModel, value string) (mcpSetupModel, tea.Cmd) {
	m.input.SetValue(value)
	next, command := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return next.(mcpSetupModel), command
}

func setupKey(m mcpSetupModel, key tea.KeyPressMsg) mcpSetupModel {
	next, _ := m.Update(key)
	return next.(mcpSetupModel)
}

func TestMCPSetupStdioTransaction(t *testing.T) {
	cfg := &Config{}
	m, err := newMCPSetupModel(context.Background(), cfg, "local", false, mcpAddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m, _ = setupEnter(m, "local")
	m, _ = setupEnter(m, "")
	m, _ = setupEnter(m, "npx")
	m, _ = setupEnter(m, "-y")
	m, _ = setupEnter(m, "some path with spaces")
	if !reflect.DeepEqual(m.server.Args, []string{"-y", "some path with spaces"}) {
		t.Fatal(m.server.Args)
	}
	m = setupKey(m, tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m, _ = setupEnter(m, "@example/mcp")
	m, _ = setupEnter(m, "")
	m, _ = setupEnter(m, "API_TOKEN")
	m, _ = setupEnter(m, "MY_TOKEN")
	if !reflect.DeepEqual(m.server.Env, []string{"API_TOKEN=${MY_TOKEN}"}) {
		t.Fatal(m.server.Env)
	}
	m, _ = setupEnter(m, "")
	if m.screen != mcpSetupConnect || m.choice != 0 {
		t.Fatal("connection must require explicit selection")
	}
	if cfg.MCPServers != nil {
		t.Fatal("draft mutated original")
	}
	calls := 0
	setMCPTestDiscovery(t, func(_ context.Context, name string, server MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
		calls++
		if name != "local" || server.Command != "npx" {
			t.Fatal("wrong discovery identity")
		}
		return []ocrmcp.DiscoveredTool{{Name: "z", DefinitionSHA256: mcpTestFingerprint}, {Name: "a", DefinitionSHA256: mcpTestFingerprint}}, nil
	})
	m.choice = 1
	m, cmd := setupEnter(m, "")
	if !m.busy || calls != 0 || cmd == nil {
		t.Fatal("discovery is not deferred")
	}
	updated, _ := m.Update(cmd())
	m = updated.(mcpSetupModel)
	if calls != 1 || len(m.selected) != 0 || m.tools[0].Name != "a" {
		t.Fatal("unsafe discovery defaults")
	}
	m = setupKey(m, tea.KeyPressMsg{Code: ' ', Text: " "})
	m, _ = setupEnter(m, "")
	if !m.selected["a"] || m.confirmed {
		t.Fatal("selection must not imply save")
	}
	m = setupKey(m, tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	if m.screen != mcpSetupTools || !m.selected["a"] {
		t.Fatal("back lost selection")
	}
	m, _ = setupEnter(m, "")
	m.choice = 1
	m, _ = setupEnter(m, "")
	if !m.confirmed || m.cancelled {
		t.Fatal("save not confirmed")
	}
	server := m.server
	setMCPSelectedTools(&server, []ocrmcp.DiscoveredTool{m.tools[0]})
	if server.ToolPermissions["a"] != ocrmcp.PermissionAsk || server.ToolDefinitionSHA256["a"] != mcpTestFingerprint {
		t.Fatal("selection does not preserve accepted identity")
	}
}

func TestMCPSetupRemoteCredentialsBackAndCancel(t *testing.T) {
	const secret = "query_secret_sentinel"
	server := MCPServerConfig{Type: "remote", URL: "https://example.com/mcp?key=" + secret, Headers: map[string]string{"Authorization": "legacy_secret_sentinel"}}
	m, err := newMCPSetupModel(nil, &Config{MCPServers: map[string]MCPServerConfig{"remote": server}}, "remote", true, mcpAddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m, _ = setupEnter(m, "remote")
	m, _ = setupEnter(m, "")
	if strings.Contains(m.View().Content, secret) || strings.Contains(m.input.Placeholder, secret) {
		t.Fatal("stored URL displayed")
	}
	m, _ = setupEnter(m, "")
	if m.server.URL != server.URL {
		t.Fatal("blank did not preserve stored URL")
	}
	m, _ = setupEnter(m, "Authorization")
	m, _ = setupEnter(m, "REMOTE_TOKEN")
	m, _ = setupEnter(m, "Bearer ")
	if m.server.Headers["Authorization"] != "Bearer ${REMOTE_TOKEN}" {
		t.Fatal("bad header template")
	}
	m, _ = setupEnter(m, "")
	if strings.Contains(m.View().Content, secret) || strings.Contains(m.View().Content, "legacy_secret_sentinel") {
		t.Fatal("connection preview leaked")
	}
	m.choice = 2
	m, _ = setupEnter(m, "")
	if m.screen != mcpSetupSave || len(m.selected) != 0 || m.busy {
		t.Fatal("disabled save connected")
	}
	m = setupKey(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.cancelled || m.confirmed {
		t.Fatal("cancel committed")
	}
}

func TestMCPSetupValidationRetryAndTransportReset(t *testing.T) {
	m, _ := newMCPSetupModel(nil, &Config{}, "", false, mcpAddOptions{})
	m, _ = setupEnter(m, "invalid name")
	if m.screen != mcpSetupName || m.errorText == "" {
		t.Fatal("invalid name advanced")
	}
	m, _ = setupEnter(m, "demo")
	m.choice = 1
	m, _ = setupEnter(m, "")
	m, _ = setupEnter(m, "https://user:pass@example.com/mcp")
	if m.screen != mcpSetupEndpoint || strings.Contains(m.errorText, "pass@") {
		t.Fatal("unsafe endpoint accepted")
	}
	m, _ = setupEnter(m, "http://example.com/mcp")
	m, _ = setupEnter(m, "MCP-Session-Id")
	if m.errorText == "" {
		t.Fatal("reserved header accepted")
	}
	m, _ = setupEnter(m, "Authorization")
	m, _ = setupEnter(m, "not an env name")
	if m.errorText == "" {
		t.Fatal("invalid reference accepted")
	}
	m, _ = setupEnter(m, "TEST_TOKEN")
	m, _ = setupEnter(m, "Bearer ")
	m, _ = setupEnter(m, "")
	if !strings.Contains(m.View().Content, "WARNING") {
		t.Fatal("insecure warning missing")
	}
	m.choice = 2
	m, _ = setupEnter(m, "")
	if !m.server.AllowInsecureHTTP {
		t.Fatal("explicit HTTP exception not recorded")
	}
	m.screen = mcpSetupConnect
	m.busy = true
	next, _ := m.Update(mcpSetupDiscovered{err: errors.New("connection failed secret")})
	m = next.(mcpSetupModel)
	if m.busy || m.errorText == "" || strings.Contains(m.errorText, "secret") {
		t.Fatal("bad retry error")
	}
	m.screen = mcpSetupTransport
	m.choice = 0
	m, _ = setupEnter(m, "")
	if m.server.Type != "stdio" || m.server.URL != "" || len(m.server.Headers) != 0 {
		t.Fatal("transport retained remote credentials")
	}
}

func TestMCPToolSelectionNumbersMatchDisplay(t *testing.T) {
	tools := []ocrmcp.DiscoveredTool{{Name: "z"}, {Name: "a"}}
	selected, err := selectMCPDiscoveredTools(tools, "1")
	if err != nil || len(selected) != 1 || selected[0].Name != "a" || tools[0].Name != "z" {
		t.Fatalf("selection = %v, %v", selected, err)
	}
}

func TestMCPPermissionsPreserveExactNames(t *testing.T) {
	for _, name := range []string{"read=v1", " read "} {
		setupMCPTestHome(t, &Config{MCP: &ocrmcp.MCPConfig{Version: 1}, MCPServers: map[string]MCPServerConfig{"exact": {
			Command: "unused", Tools: []string{name}, ToolDefinitionSHA256: map[string]string{name: mcpTestFingerprint},
		}}})
		setMCPTestInteractive(t, true)
		cmd, _, _ := newMCPTestCommand("inherit\ndeny\ny\n")
		if err := runMCPPermissions(cmd, "exact", mcpPermissionsOptions{}); err != nil {
			t.Fatal(err)
		}
		if loadMCPTestConfig(t).MCPServers["exact"].ToolPermissions[name] != ocrmcp.PermissionDeny {
			t.Fatal("interactive permission lost exact name")
		}
		if err := runMCPPermissions(cmd, "exact", mcpPermissionsOptions{toolPermissions: []string{name + "=ask"}, yes: true}); err != nil {
			t.Fatal(err)
		}
		if loadMCPTestConfig(t).MCPServers["exact"].ToolPermissions[name] != ocrmcp.PermissionAsk {
			t.Fatal("flag permission lost exact name")
		}
	}
}

func TestMCPToolRevocationWorksOffline(t *testing.T) {
	setupMCPTestHome(t, &Config{MCP: &ocrmcp.MCPConfig{Version: 1}, MCPServers: map[string]MCPServerConfig{"offline": {
		Command: "does-not-exist", Tools: []string{"read", "write"}, ToolPermissions: map[string]ocrmcp.Permission{"read": ocrmcp.PermissionAsk, "write": ocrmcp.PermissionAllow}, ToolDefinitionSHA256: map[string]string{"read": mcpTestFingerprint, "write": mcpTestFingerprint},
	}}})
	setMCPTestInteractive(t, false)
	setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
		t.Fatal("revocation tried to connect")
		return nil, nil
	})
	cmd, _, _ := newMCPTestCommand("")
	if err := runMCPTools(cmd, "offline", mcpToolsOptions{disable: []string{"write"}}); err != nil {
		t.Fatal(err)
	}
	got := loadMCPTestConfig(t).MCPServers["offline"]
	if !reflect.DeepEqual(got.Tools, []string{"read"}) || got.ToolDefinitionSHA256["read"] != mcpTestFingerprint || got.ToolPermissions["write"] != "" || got.ToolDefinitionSHA256["write"] != "" {
		t.Fatalf("bad local revocation: %+v", got)
	}
}

func TestMCPConfigOverrideIsSharedByReadsAndWrites(t *testing.T) {
	defaultPath := setupMCPTestHome(t, &Config{Language: "default-language"})
	before, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(t.TempDir(), "custom.json")
	t.Setenv("OCR_CONFIG_PATH", custom)
	setMCPTestInteractive(t, false)
	cmd, _, _ := newMCPTestCommand("")
	if err := runMCPAdd(cmd, []string{"demo"}, mcpAddOptions{transport: "stdio", command: "unused", yes: true}); err != nil {
		t.Fatal(err)
	}
	if err := runConfigSet("mcp.approval_timeout_seconds", "120"); err != nil {
		t.Fatal(err)
	}
	if err := runConfigUnset("mcp.default_permission"); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadOrCreateConfig(custom)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MCP.ApprovalTimeoutSeconds != 120 || len(cfg.MCPServers) != 1 {
		t.Fatal("writes ignored override")
	}
	after, _ := os.ReadFile(defaultPath)
	if !bytes.Equal(before, after) {
		t.Fatal("default config was modified")
	}
}

func TestMCPTimeoutFlagBounds(t *testing.T) {
	setupMCPTestHome(t, &Config{})
	setMCPTestInteractive(t, false)
	for _, value := range []string{"0", "601", "-1"} {
		cmd := newMCPCommand()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"permissions", "--timeout", value, "--yes"})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted timeout %s", value)
		}
	}
	for _, value := range []string{"1", "600"} {
		cmd := newMCPCommand()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"permissions", "--timeout", value, "--yes"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("timeout %s: %v", value, err)
		}
	}
}

func TestMCPPromptAndArgumentPreviewsHideCredentials(t *testing.T) {
	for _, label := range []string{"Remote URL", "Arguments", "Environment", "Headers"} {
		m := newMCPManagementTextModel(label, "stored_sentinel")
		if strings.Contains(m.View().Content, "stored_sentinel") {
			t.Fatal("TUI placeholder leaked")
		}
		var out bytes.Buffer
		p := mcpPrompter{in: strings.NewReader("\n"), out: &out}
		value, err := p.prompt(label, "stored_sentinel")
		if err != nil || value != "stored_sentinel" || strings.Contains(out.String(), "stored_sentinel") {
			t.Fatal("fallback prompt leaked or lost value")
		}
	}
	for _, args := range [][]string{{"--api_key", "value_sentinel"}, {"--private.key=value_sentinel"}, {"--header", "Authorization: value_sentinel"}, {"https://user:value_sentinel@example.com/#fragment_sentinel"}} {
		got := strings.Join(redactedMCPCommandArguments(args), " ")
		if strings.Contains(got, "value_sentinel") || strings.Contains(got, "fragment_sentinel") {
			t.Fatalf("argument leaked: %s", got)
		}
	}
}

func TestMCPChecklistSelectionAndViewport(t *testing.T) {
	tools := []ocrmcp.DiscoveredTool{{Name: "first", Description: "first untrusted description"}, {Name: "second"}, {Name: "third", Description: "third untrusted description"}}
	m := mcpToolSelectionModel{mcpSetupModel{screen: mcpSetupTools, tools: tools, selected: map[string]bool{}, height: 14}}
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: ' ', Text: " "}} {
		next, _ := m.Update(key)
		m = next.(mcpToolSelectionModel)
	}
	if !m.selected["third"] || m.selected["first"] {
		t.Fatal("checklist selected wrong identity")
	}
	view := m.View().Content
	for _, want := range []string{"[x] third", "Tool 3/3", "third untrusted description"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in %s", want, view)
		}
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !next.(mcpToolSelectionModel).confirmed || cmd == nil {
		t.Fatal("checklist not confirmed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	selected, err := runMCPToolSelection(ctx, tools, strings.NewReader(" \r"), &bytes.Buffer{})
	if err != nil || len(selected) != 1 || selected[0].Name != "first" {
		t.Fatalf("runner: %v %v", selected, err)
	}
	_, err = runMCPToolSelection(ctx, tools, strings.NewReader("\x03"), &bytes.Buffer{})
	if !errors.Is(err, errMCPPromptCancelled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestMCPMenuKeyboardAndRendering(t *testing.T) {
	m := mcpSelectionModel{label: "Permission", choices: []string{"ask", "allow", "deny"}, height: 10}
	if m.Init() != nil {
		t.Fatal("menu init must not perform work")
	}
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: tea.KeyUp}} {
		next, _ := m.Update(key)
		m = next.(mcpSelectionModel)
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m = next.(mcpSelectionModel)
	if !strings.Contains(m.View().Content, "> allow") || m.confirmed {
		t.Fatal("menu state wrong")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !next.(mcpSelectionModel).confirmed || cmd == nil {
		t.Fatal("menu enter failed")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.(mcpSelectionModel).confirmed {
		t.Fatal("escape confirmed menu")
	}
}

func TestMCPSetupViewsBusyAndKeyboard(t *testing.T) {
	m, err := newMCPSetupModel(nil, &Config{}, "demo", false, mcpAddOptions{command: "test", args: []string{"--api_key", "hidden_sentinel"}})
	if err != nil {
		t.Fatal(err)
	}
	if m.Init() == nil {
		t.Fatal("missing input initialization")
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(mcpSetupModel)
	if m.width != 60 || m.height != 20 {
		t.Fatal("resize ignored")
	}
	m.input.Reset()
	m = setupKey(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.input.Value() != "x" {
		t.Fatal("typed input ignored")
	}
	for _, screen := range []mcpSetupScreen{mcpSetupName, mcpSetupTransport, mcpSetupEndpoint, mcpSetupArguments, mcpSetupCredentials, mcpSetupTools, mcpSetupSave} {
		m.screen = screen
		m.resetInput()
		m.errorText = "validation explanation"
		view := m.View().Content
		if !strings.Contains(view, "validation explanation") || strings.Contains(view, "hidden_sentinel") {
			t.Fatalf("unsafe or missing UI feedback on %d", screen)
		}
	}
	m.busy = true
	m.screen = mcpSetupConnect
	if !strings.Contains(m.View().Content, "Discovering tools") || !strings.Contains(m.View().Content, "30s timeout") || !strings.Contains(m.View().Content, "Esc Cancel") {
		t.Fatal("busy state missing progress, timeout or cancellation")
	}
	m = setupKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.busy || m.confirmed {
		t.Fatal("busy input advanced")
	}
	m = setupKey(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if !m.cancelled || m.confirmed {
		t.Fatal("busy cancellation failed")
	}
}

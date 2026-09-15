// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
)

func dashboardFixture() mcpDashboardModel {
	return mcpDashboardModel{height: 24, checks: make(map[string]mcpDashboardCheck), cfg: &Config{MCP: &ocrmcp.MCPConfig{Version: 1}, MCPServers: map[string]MCPServerConfig{
		"docs": {Command: "never-start", Tools: []string{"search"}, ToolDefinitionSHA256: map[string]string{"search": mcpTestFingerprint}},
	}}}
}

func dashboardKey(m mcpDashboardModel, code rune) mcpDashboardModel {
	next, _ := m.Update(tea.KeyPressMsg{Code: code})
	return next.(mcpDashboardModel)
}

func TestMCPDashboardNavigation(t *testing.T) {
	m := dashboardFixture()
	if m.Init() != nil || len(m.checks) != 0 {
		t.Fatal("unsafe initial screen")
	}
	m = dashboardKey(m, tea.KeyEnter)
	if m.server != "docs" || m.page != "server" {
		t.Fatal(m)
	}
	if !strings.Contains(m.View().Content, "not checked this session") {
		t.Fatal("invented connection status")
	}
	m = dashboardKey(m, tea.KeyEnter)
	if m.page != "tools" || !strings.Contains(m.View().Content, "Ask before calling") {
		t.Fatal(m.View())
	}
	m = dashboardKey(m, tea.KeyEnter)
	if m.page != "tool" || m.tool != "search" {
		t.Fatal(m)
	}
	m = dashboardKey(m, tea.KeyEscape)
	m = dashboardKey(m, tea.KeyLeft)
	m = dashboardKey(m, tea.KeyEscape)
	if m.page != "" {
		t.Fatal("back lost hierarchy")
	}
	m = dashboardKey(m, tea.KeyUp)
	if m.cursor != 0 {
		t.Fatal("negative cursor")
	}
	for range 20 {
		m = dashboardKey(m, tea.KeyDown)
	}
	if m.cursor != len(m.items())-1 {
		t.Fatal("cursor escaped list")
	}
	m = dashboardKey(m, tea.KeyEnter)
	if m.action != "quit" {
		t.Fatal(m)
	}
	m = dashboardFixture()
	m = dashboardKey(m, tea.KeyEscape)
	if m.action != "quit" {
		t.Fatal(m)
	}
	m = dashboardFixture()
	m = dashboardKey(m, 'q')
	if m.action != "quit" {
		t.Fatal(m)
	}
	m = dashboardFixture()
	m.cfg.MCPServers = nil
	if !strings.Contains(m.View().Content, "No servers yet") {
		t.Fatal(m.View())
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 15})
	m = next.(mcpDashboardModel)
	m.notice = "Action failed. Retry."
	if !strings.Contains(m.View().Content, m.notice) {
		t.Fatal("missing recovery")
	}
}

func TestMCPDashboardEffectivePolicyAndDrift(t *testing.T) {
	m := dashboardFixture()
	m.server = "docs"
	m.page = "tools"
	if !strings.Contains(m.toolState("other"), "Not enabled") {
		t.Fatal("scope widened")
	}
	server := m.cfg.MCPServers["docs"]
	server.ToolPermissions = map[string]ocrmcp.Permission{"search": ocrmcp.PermissionAllow}
	m.cfg.MCPServers["docs"] = server
	if !strings.Contains(m.toolState("search"), "No prompt") {
		t.Fatal("allow missing")
	}
	m.cfg.MCP.DefaultPermission = ocrmcp.PermissionDeny
	if !strings.Contains(m.toolState("search"), "Blocked") {
		t.Fatal("parent deny lost")
	}
	m.cfg.MCP.DefaultPermission = "bad"
	if !strings.Contains(m.toolState("search"), "invalid policy") {
		t.Fatal("invalid policy allowed")
	}
	m.cfg.MCP = nil
	m.checks["docs"] = mcpDashboardCheck{at: time.Now(), tools: []ocrmcp.DiscoveredTool{{Name: "search", DefinitionSHA256: mcpTestFingerprint, Description: "External description"}, {Name: "extra"}}}
	if len(m.toolNames()) != 2 || m.toolNames()[0] != "extra" {
		t.Fatal(m.toolNames())
	}
	m.page = "tool"
	m.tool = "search"
	if !strings.Contains(m.View().Content, "Untrusted server description") || !strings.Contains(m.View().Content, "Last check:") {
		t.Fatal(m.View())
	}
	server.ToolDefinitionSHA256 = nil
	m.cfg.MCPServers["docs"] = server
	if !strings.Contains(m.toolState("search"), "changed definition") {
		t.Fatal("drift missed")
	}
	m.checks["docs"] = mcpDashboardCheck{failed: true}
	if !strings.Contains(m.toolState("search"), "no accepted fingerprint") || !strings.Contains(m.View().Content, "failed;") {
		t.Fatal("failure displayed as success")
	}
	m.tool = "extra"
	if m.items()[0].action != "select-tool" {
		t.Fatal("unselected tool lacks safe enable")
	}
	server.Enabled = boolPointer(false)
	m.cfg.MCPServers["docs"] = server
	m.page = "server"
	if m.items()[4].action != "enable" {
		t.Fatal("wrong toggle")
	}
}

func TestMCPDashboardDiscoveryConsentAndOfflineRevoke(t *testing.T) {
	m := dashboardFixture()
	m.server = "docs"
	m.action = "discover"
	setupMCPTestHome(t, m.cfg)
	setMCPTestInteractive(t, true)
	calls := 0
	setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
		calls++
		return []ocrmcp.DiscoveredTool{{Name: "search", DefinitionSHA256: mcpTestFingerprint}}, nil
	})
	cmd, _, _ := newMCPTestCommand("n\n")
	if err := m.perform(cmd); err != nil || calls != 0 || len(m.checks) != 0 {
		t.Fatal("discovery without confirmation")
	}
	cmd, _, _ = newMCPTestCommand("y\n")
	if err := m.perform(cmd); err != nil || calls != 1 || m.checks["docs"].failed {
		t.Fatal(err)
	}
	setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
		calls++
		return nil, errors.New("SECRET")
	})
	cmd, _, _ = newMCPTestCommand("y\n")
	if m.perform(cmd) == nil || !m.checks["docs"].failed {
		t.Fatal("failed check lost")
	}
	m.action = "revoke"
	m.tool = "search"
	cmd, _, _ = newMCPTestCommand("")
	if err := m.perform(cmd); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(loadMCPTestConfig(t).MCPServers["docs"].Tools) != 0 {
		t.Fatal("revocation connected or failed")
	}
}

func TestMCPDashboardReusesTransactionalActions(t *testing.T) {
	for _, action := range []string{"add", "edit", "permissions", "global", "enable", "disable", "remove", "tool-permission", "select-tool", "import", "unknown"} {
		t.Run(action, func(t *testing.T) {
			m := dashboardFixture()
			m.server = "docs"
			m.tool = "search"
			m.action = action
			setupMCPTestHome(t, m.cfg)
			setMCPTestInteractive(t, true)
			cmd, _, _ := newMCPTestCommand("cancel\n")
			if err := m.perform(cmd); err != nil {
				t.Fatal(err)
			}
			if len(loadMCPTestConfig(t).MCPServers["docs"].Tools) != 1 {
				t.Fatal("cancel changed scope")
			}
		})
	}
	m := dashboardFixture()
	m.server = "docs"
	m.tool = "search"
	m.action = "tool-permission"
	setupMCPTestHome(t, m.cfg)
	setMCPTestInteractive(t, true)
	cmd, _, _ := newMCPTestCommand("allow\ny\n")
	if err := m.perform(cmd); err != nil {
		t.Fatal(err)
	}
	if loadMCPTestConfig(t).MCPServers["docs"].ToolPermissions["search"] != ocrmcp.PermissionAllow {
		t.Fatal("permission not saved")
	}
}

func TestMCPDashboardLegacySecretLabelsAndSelection(t *testing.T) {
	m := dashboardFixture()
	t.Setenv("OCR_DASH_SECRET", "SECRET_SENTINEL")
	s := m.cfg.MCPServers["docs"]
	s.Env = []string{"TOKEN=${OCR_DASH_SECRET}-${OCR_DASH_UNSET}"}
	s.Headers = map[string]string{"Authorization": "HEADER_SENTINEL"}
	s.Args = []string{"--token", "ARG_SENTINEL"}
	s.Tools = []string{"tool_SECRET_SENTINEL_HEADER_SENTINEL_ARG_SENTINEL"}
	m.cfg.MCPServers["docs"] = s
	m.server = "docs"
	m.page = "tools"
	if strings.Contains(m.View().Content, "SENTINEL") {
		t.Fatal("legacy labels leaked credentials")
	}
	m.cfg.MCPServers["aaa"] = MCPServerConfig{Command: "server"}
	m.page = "server"
	m = m.back()
	if m.cursor != 1 || m.items()[m.cursor].identity != "docs" {
		t.Fatal("return lost selected server")
	}
}

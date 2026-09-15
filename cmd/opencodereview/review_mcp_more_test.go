// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
	"github.com/alibaba/open-code-review/internal/tool"
)

// silenceStderr redirects os.Stderr to /dev/null for the duration of fn so the
// MCP init warnings do not clutter test logs.
func silenceStderr(t *testing.T, fn func()) {
	t.Helper()
	orig := os.Stderr
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	os.Stderr = devnull
	defer func() {
		os.Stderr = orig
		_ = devnull.Close()
	}()
	fn()
}

// TestInitMCPClients_ErrorBranches covers connection failures after the
// fail-closed visibility preflight, plus the legacy setup no-execution rule.
func TestInitMCPClients_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	global := &ocrmcp.MCPConfig{Version: 1, DefaultPermission: ocrmcp.PermissionAllow}
	fingerprint := strings.Repeat("a", 64)

	t.Run("remote connect failure skipped", func(t *testing.T) {
		reg := tool.NewRegistry()
		cfg := &Config{MCP: global, MCPServers: map[string]MCPServerConfig{
			// Port 1 is reserved and refuses connections immediately.
			"r": {
				Type: "remote", URL: "http://127.0.0.1:1/mcp",
				Tools: []string{"probe"}, DefaultPermission: ocrmcp.PermissionAllow,
				ToolDefinitionSHA256: map[string]string{"probe": fingerprint},
			},
		}}
		var clients []interface{}
		silenceStderr(t, func() {
			for _, c := range initMCPClients(ctx, cfg, reg, t.TempDir(), "v") {
				clients = append(clients, c)
			}
		})
		if len(clients) != 0 {
			t.Errorf("got %d clients, want 0 (connect should fail)", len(clients))
		}
	})

	t.Run("stdio start failure skipped", func(t *testing.T) {
		reg := tool.NewRegistry()
		cfg := &Config{MCP: global, MCPServers: map[string]MCPServerConfig{
			"s": {
				Type: "stdio", Command: "ocr-nonexistent-binary-xyz",
				Tools: []string{"probe"}, DefaultPermission: ocrmcp.PermissionAllow,
				ToolDefinitionSHA256: map[string]string{"probe": fingerprint},
			},
		}}
		var n int
		silenceStderr(t, func() {
			n = len(initMCPClients(ctx, cfg, reg, t.TempDir(), "v"))
		})
		if n != 0 {
			t.Errorf("got %d clients, want 0 (start should fail)", n)
		}
	})

	t.Run("legacy setup is never executed", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("shell marker assertion is Unix-specific")
		}
		reg := tool.NewRegistry()
		marker := filepath.Join(t.TempDir(), "legacy-setup-ran")
		cfg := &Config{MCP: global, MCPServers: map[string]MCPServerConfig{
			"s": {
				Type: "stdio", Command: "ocr-nonexistent-binary-xyz",
				Setup: "touch " + strconv.Quote(marker),
				Tools: []string{"probe"}, DefaultPermission: ocrmcp.PermissionAllow,
				ToolDefinitionSHA256: map[string]string{"probe": fingerprint},
			},
		}}
		var n int
		silenceStderr(t, func() {
			n = len(initMCPClients(ctx, cfg, reg, t.TempDir(), "v"))
		})
		if n != 0 {
			t.Errorf("got %d clients, want 0 (server start should fail)", n)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("legacy setup executed; marker stat error = %v", err)
		}
	})
}

func TestMCPServerPotentialVisibilityPreflight(t *testing.T) {
	fingerprint := strings.Repeat("a", 64)
	base := MCPServerConfig{
		Tools:                []string{"read"},
		ToolDefinitionSHA256: map[string]string{"read": fingerprint},
	}
	tests := []struct {
		name        string
		global      ocrmcp.MCPConfig
		server      MCPServerConfig
		interactive bool
		want        bool
		wantErr     bool
	}{
		{name: "empty allowlist", global: ocrmcp.MCPConfig{Version: 1}, server: MCPServerConfig{}},
		{name: "v1 missing fingerprint", global: ocrmcp.MCPConfig{Version: 1, DefaultPermission: ocrmcp.PermissionAllow}, server: MCPServerConfig{Tools: []string{"read"}}},
		{name: "ask hidden non-interactive", global: ocrmcp.MCPConfig{Version: 1}, server: base},
		{name: "ask visible interactive", global: ocrmcp.MCPConfig{Version: 1}, server: base, interactive: true, want: true},
		{name: "allow visible non-interactive", global: ocrmcp.MCPConfig{Version: 1, DefaultPermission: ocrmcp.PermissionAllow}, server: base, want: true},
		{name: "global deny ceiling", global: ocrmcp.MCPConfig{Version: 1, DefaultPermission: ocrmcp.PermissionDeny}, server: func() MCPServerConfig {
			server := base
			server.DefaultPermission = ocrmcp.PermissionAllow
			return server
		}()},
		{name: "legacy missing fingerprint forced ask in CI", global: ocrmcp.MCPConfig{DefaultPermission: ocrmcp.PermissionAllow}, server: MCPServerConfig{Tools: []string{"read"}, DefaultPermission: ocrmcp.PermissionAllow}},
		{name: "legacy missing fingerprint interactive", global: ocrmcp.MCPConfig{DefaultPermission: ocrmcp.PermissionAllow}, server: MCPServerConfig{Tools: []string{"read"}, DefaultPermission: ocrmcp.PermissionAllow}, interactive: true, want: true},
		{name: "invalid policy", global: ocrmcp.MCPConfig{Version: 1}, server: MCPServerConfig{Tools: []string{"read"}, ToolPermissions: map[string]ocrmcp.Permission{"other": ocrmcp.PermissionAllow}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := mcpServerPotentiallyVisible(test.global, test.server, test.interactive)
			if got != test.want || (err != nil) != test.wantErr {
				t.Fatalf("mcpServerPotentiallyVisible() = (%v, %v), want (%v, error=%v)", got, err, test.want, test.wantErr)
			}
		})
	}
}

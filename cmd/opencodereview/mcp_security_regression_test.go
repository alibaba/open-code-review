// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestLegacyDeniedServersNeverConnect(t *testing.T) {
	setMCPTestInteractive(t, true)
	var calls atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(http.StatusForbidden) }))
	defer endpoint.Close()
	for _, variant := range []string{"global-disabled", "server-disabled", "global-deny", "server-deny", "tool-deny"} {
		global := &ocrmcp.MCPConfig{}
		server := MCPServerConfig{Type: "remote", URL: endpoint.URL, Tools: []string{"read"}}
		switch variant {
		case "global-disabled":
			global.Enabled = boolPointer(false)
		case "server-disabled":
			server.Enabled = boolPointer(false)
		case "global-deny":
			global.DefaultPermission = ocrmcp.PermissionDeny
		case "server-deny":
			server.DefaultPermission = ocrmcp.PermissionDeny
		case "tool-deny":
			server.ToolPermissions = map[string]ocrmcp.Permission{"read": ocrmcp.PermissionDeny}
		}
		visible, err := mcpServerPotentiallyVisible(*global, server, true)
		if err != nil {
			t.Fatal(err)
		}
		if visible {
			t.Errorf("%s marked visible", variant)
		}
		state := initMCPRuntime(context.Background(), &Config{MCP: global, MCPServers: map[string]MCPServerConfig{"test": server}}, tool.NewRegistry(), t.TempDir(), "test")
		closeMCPClients(state.clients)
	}
	if calls.Load() != 0 {
		t.Fatalf("denied endpoints received %d requests", calls.Load())
	}
}

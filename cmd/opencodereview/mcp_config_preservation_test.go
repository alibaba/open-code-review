// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPManagementPreservesUnknownConfigFields(t *testing.T) {
	var original Config
	if err := json.Unmarshal([]byte(`{"future_top":{"value":7},"mcp":{"version":1,"future_policy":{"value":8}},"mcp_servers":{"docs":{"command":"never-start","tools":["read"],"tool_permissions":{"read":"allow"},"future_server":{"value":9}}}}`), &original); err != nil {
		t.Fatal(err)
	}
	setupMCPTestHome(t, &original)
	setMCPTestInteractive(t, false)
	cmd, _, _ := newMCPTestCommand("")
	if err := runMCPTools(cmd, "docs", mcpToolsOptions{disable: []string{"read"}, yes: true}); err != nil {
		t.Fatal(err)
	}
	cfg := loadMCPTestConfig(t)
	draft, err := cloneAppConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := setConfigValue(draft, "language", "English"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	// Save to a separate file only after dropping the original file's revision.
	draft.revision = nil
	if err := saveConfig(path, draft); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	assertJSONValue(t, got, []string{"future_top", "value"}, float64(7))
	assertJSONValue(t, got, []string{"mcp", "future_policy", "value"}, float64(8))
	assertJSONValue(t, got, []string{"mcp_servers", "docs", "future_server", "value"}, float64(9))
	server := draft.MCPServers["docs"]
	if len(server.Tools) != 0 || len(server.ToolPermissions) != 0 {
		t.Fatal("revoked tools were restored")
	}
}

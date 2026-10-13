// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfigJSONPreservesExtensionsWithoutRestoringRevokedTools(t *testing.T) {
	var server MCPServerConfig
	if err := json.Unmarshal([]byte(`{"command":"unused","TOOLS":["read"],"tool_permissions":{"read":"allow"},"future":{"id":9007199254740993}}`), &server); err != nil {
		t.Fatal(err)
	}
	clone := cloneServerConfig(server)
	clone.unknownJSONFields["future"][0] = ' '
	if !json.Valid(server.unknownJSONFields["future"]) {
		t.Fatal("clone aliases preserved data")
	}
	server.Tools = nil
	server.ToolPermissions = nil
	data, err := json.Marshal(server)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(data)), "tools") || !strings.Contains(string(data), "9007199254740993") {
		t.Fatalf("incorrect round trip: %s", data)
	}
	var restored MCPServerConfig
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if permission, err := ResolvePermission(MCPConfig{DefaultPermission: PermissionAllow}, restored, "read"); err == nil || permission != PermissionDeny {
		t.Fatal("unknown fields restored authorization")
	}
}

func TestGlobalConfigJSONPreservesExtensions(t *testing.T) {
	var config MCPConfig
	if err := json.Unmarshal([]byte(`{"version":1,"APPROVAL_TIMEOUT_SECONDS":120,"future":{"id":9007199254740993}}`), &config); err != nil {
		t.Fatal(err)
	}
	config.ApprovalTimeoutSeconds = 0
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(data)), "approval_timeout_seconds") || !strings.Contains(string(data), "9007199254740993") {
		t.Fatalf("incorrect round trip: %s", data)
	}
	for _, input := range []string{`{"version":"invalid"}`, `{"approval_timeout_seconds":{}}`} {
		if json.Unmarshal([]byte(input), &config) == nil {
			t.Fatalf("accepted malformed policy: %s", input)
		}
	}
	var server MCPServerConfig
	if json.Unmarshal([]byte(`{"tools":true}`), &server) == nil {
		t.Fatal("accepted malformed tools")
	}
}

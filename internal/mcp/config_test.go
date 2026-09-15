// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func boolPointer(value bool) *bool { return &value }

func TestApprovalTimeout(t *testing.T) {
	tests := []struct {
		seconds int
		want    time.Duration
		wantErr bool
	}{
		{0, 60 * time.Second, false},
		{1, time.Second, false},
		{600, 10 * time.Minute, false},
		{-1, 0, true},
		{601, 0, true},
	}
	for _, test := range tests {
		got, err := (MCPConfig{ApprovalTimeoutSeconds: test.seconds}).ApprovalTimeout()
		if (err != nil) != test.wantErr || got != test.want {
			t.Errorf("ApprovalTimeout(%d) = (%v, %v), want (%v, error=%v)", test.seconds, got, err, test.want, test.wantErr)
		}
	}
}

func TestResolvePermission(t *testing.T) {
	base := MCPServerConfig{Tools: []string{"search"}}
	tests := []struct {
		name       string
		global     MCPConfig
		server     MCPServerConfig
		tool       string
		want       Permission
		wantErrIs  error
		wantAnyErr bool
	}{
		{name: "zero value asks", server: base, tool: "search", want: PermissionAsk},
		{name: "tool is not implicitly enabled", server: base, tool: "write", want: PermissionDeny, wantErrIs: ErrToolNotEnabled},
		{name: "global allow", global: MCPConfig{DefaultPermission: PermissionAllow}, server: base, tool: "search", want: PermissionAllow},
		{name: "server is most specific", global: MCPConfig{DefaultPermission: PermissionAllow}, server: MCPServerConfig{Tools: []string{"search"}, DefaultPermission: PermissionAsk}, tool: "search", want: PermissionAsk},
		{name: "tool is most specific", server: MCPServerConfig{Tools: []string{"search"}, ToolPermissions: map[string]Permission{"search": PermissionAllow}}, tool: "search", want: PermissionAllow},
		{name: "global deny is ceiling", global: MCPConfig{DefaultPermission: PermissionDeny}, server: MCPServerConfig{Tools: []string{"search"}, DefaultPermission: PermissionAllow, ToolPermissions: map[string]Permission{"search": PermissionAllow}}, tool: "search", want: PermissionDeny},
		{name: "server deny is ceiling", global: MCPConfig{DefaultPermission: PermissionAllow}, server: MCPServerConfig{Tools: []string{"search"}, DefaultPermission: PermissionDeny, ToolPermissions: map[string]Permission{"search": PermissionAllow}}, tool: "search", want: PermissionDeny},
		{name: "disabled global", global: MCPConfig{Enabled: boolPointer(false)}, server: base, tool: "search", want: PermissionDeny},
		{name: "disabled server", server: MCPServerConfig{Enabled: boolPointer(false), Tools: []string{"search"}}, tool: "search", want: PermissionDeny},
		{name: "global inherit invalid", global: MCPConfig{DefaultPermission: PermissionInherit}, server: base, tool: "search", want: PermissionDeny, wantAnyErr: true},
		{name: "invalid tool policy fails closed", server: MCPServerConfig{Tools: []string{"search"}, ToolPermissions: map[string]Permission{"search": "bogus"}}, tool: "search", want: PermissionDeny, wantAnyErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolvePermission(test.global, test.server, test.tool)
			if got != test.want {
				t.Errorf("permission = %q, want %q", got, test.want)
			}
			if test.wantErrIs != nil && !errors.Is(err, test.wantErrIs) {
				t.Errorf("error = %v, want errors.Is(%v)", err, test.wantErrIs)
			}
			if test.wantAnyErr && err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestValidateMCPServerConfig(t *testing.T) {
	validFingerprint := strings.Repeat("a", 64)
	tests := []struct {
		name    string
		config  MCPServerConfig
		wantErr bool
	}{
		{name: "empty tools is valid and means none", config: MCPServerConfig{}},
		{name: "valid", config: MCPServerConfig{Tools: []string{"a"}, ToolPermissions: map[string]Permission{"a": PermissionAsk}, ToolDefinitionSHA256: map[string]string{"a": validFingerprint}}},
		{name: "duplicate tools", config: MCPServerConfig{Tools: []string{"a", "a"}}, wantErr: true},
		{name: "empty tool", config: MCPServerConfig{Tools: []string{""}}, wantErr: true},
		{name: "permission outside allowlist", config: MCPServerConfig{ToolPermissions: map[string]Permission{"a": PermissionAllow}}, wantErr: true},
		{name: "fingerprint outside allowlist", config: MCPServerConfig{ToolDefinitionSHA256: map[string]string{"a": validFingerprint}}, wantErr: true},
		{name: "uppercase fingerprint", config: MCPServerConfig{Tools: []string{"a"}, ToolDefinitionSHA256: map[string]string{"a": strings.Repeat("A", 64)}}, wantErr: true},
		{name: "short fingerprint", config: MCPServerConfig{Tools: []string{"a"}, ToolDefinitionSHA256: map[string]string{"a": "abc"}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateMCPServerConfig(test.config)
			if (err != nil) != test.wantErr {
				t.Errorf("ValidateMCPServerConfig() error = %v, wantErr=%v", err, test.wantErr)
			}
		})
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	defaultApprovalTimeout = 60 * time.Second
	minApprovalTimeout     = time.Second
	maxApprovalTimeout     = 10 * time.Minute
)

// Permission controls whether an explicitly enabled MCP tool is denied,
// approved interactively, or approved without an interactive prompt.
// Permission never enables a tool by itself.
type Permission string

const (
	PermissionInherit Permission = "inherit"
	PermissionAsk     Permission = "ask"
	PermissionAllow   Permission = "allow"
	PermissionDeny    Permission = "deny"
)

// MCPConfig contains global MCP policy. The zero value is safe: MCP is enabled
// only for explicitly selected tools, every invocation asks for approval, and
// an unanswered prompt expires after 60 seconds.
type MCPConfig struct {
	unknownJSONFields map[string]json.RawMessage

	Version                int        `json:"version,omitempty"`
	Enabled                *bool      `json:"enabled,omitempty"`
	DefaultPermission      Permission `json:"default_permission,omitempty"`
	ApprovalTimeoutSeconds int        `json:"approval_timeout_seconds,omitempty"`
}

// MCPServerConfig contains connection details and the explicit tool allowlist
// for one MCP server. An empty Tools slice enables no tools.
type MCPServerConfig struct {
	unknownJSONFields map[string]json.RawMessage

	Type    string            `json:"type,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     []string          `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	// AllowInsecureHTTP permits non-loopback plain HTTP only when the user has
	// made that downgrade explicit.
	AllowInsecureHTTP bool `json:"allow_insecure_http,omitempty"`
	// Setup is retained for config compatibility. The MCP runtime never executes
	// it; installation commands require an explicit management-flow action.
	Setup string `json:"setup,omitempty"`

	Enabled              *bool                 `json:"enabled,omitempty"`
	DefaultPermission    Permission            `json:"default_permission,omitempty"`
	Tools                []string              `json:"tools,omitempty"`
	ToolPermissions      map[string]Permission `json:"tool_permissions,omitempty"`
	ToolDefinitionSHA256 map[string]string     `json:"tool_definition_sha256,omitempty"`
}

// ErrToolNotEnabled means the named tool is outside the explicit Tools
// allowlist. Permission settings cannot expand that allowlist.
var ErrToolNotEnabled = errors.New("MCP tool is not explicitly enabled")

// ApprovalTimeout returns the validated global approval timeout.
func (c MCPConfig) ApprovalTimeout() (time.Duration, error) {
	if c.ApprovalTimeoutSeconds == 0 {
		return defaultApprovalTimeout, nil
	}
	d := time.Duration(c.ApprovalTimeoutSeconds) * time.Second
	if d < minApprovalTimeout || d > maxApprovalTimeout {
		return 0, fmt.Errorf("MCP approval timeout must be between 1 and 600 seconds")
	}
	return d, nil
}

// ResolvePermission resolves global, server, and tool policy for an explicitly
// enabled tool. A deny at any ancestor is a hard ceiling. Otherwise the most
// specific non-inherit permission wins. Invalid policy always fails closed.
func ResolvePermission(global MCPConfig, server MCPServerConfig, toolName string) (Permission, error) {
	if err := validateGlobalConfig(global); err != nil {
		return PermissionDeny, err
	}
	if err := validateServerPolicy(server); err != nil {
		return PermissionDeny, err
	}
	if !enabled(global.Enabled) || !enabled(server.Enabled) {
		return PermissionDeny, nil
	}
	if !slices.Contains(server.Tools, toolName) {
		return PermissionDeny, fmt.Errorf("%w: %q", ErrToolNotEnabled, toolName)
	}

	globalPermission := global.DefaultPermission
	if globalPermission == "" || globalPermission == PermissionInherit {
		globalPermission = PermissionAsk
	}
	if globalPermission == PermissionDeny {
		return PermissionDeny, nil
	}

	serverPermission := server.DefaultPermission
	if serverPermission == "" || serverPermission == PermissionInherit {
		serverPermission = globalPermission
	}
	if serverPermission == PermissionDeny {
		return PermissionDeny, nil
	}

	toolPermission, ok := server.ToolPermissions[toolName]
	if !ok || toolPermission == "" || toolPermission == PermissionInherit {
		toolPermission = serverPermission
	}
	if toolPermission == PermissionDeny {
		return PermissionDeny, nil
	}
	return toolPermission, nil
}

// ValidateMCPConfig validates global MCP policy without resolving a tool.
func ValidateMCPConfig(config MCPConfig) error {
	if config.Version < 0 || config.Version > 1 {
		return fmt.Errorf("unsupported MCP config version %d", config.Version)
	}
	if _, err := config.ApprovalTimeout(); err != nil {
		return err
	}
	if !validPermission(config.DefaultPermission, false) {
		return fmt.Errorf("invalid global MCP permission %q", config.DefaultPermission)
	}
	return nil
}

// ValidateMCPServerConfig validates a server's policy and explicit tool scope.
func ValidateMCPServerConfig(config MCPServerConfig) error {
	if !validPermission(config.DefaultPermission, true) {
		return fmt.Errorf("invalid MCP server permission %q", config.DefaultPermission)
	}
	seen := make(map[string]struct{}, len(config.Tools))
	for _, name := range config.Tools {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("MCP tool allowlist contains an empty name")
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("MCP tool allowlist contains duplicate name %q", name)
		}
		seen[name] = struct{}{}
	}
	for name, permission := range config.ToolPermissions {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("MCP tool permission contains an empty name")
		}
		if !validPermission(permission, true) {
			return fmt.Errorf("invalid permission %q for MCP tool %q", permission, name)
		}
		if _, selected := seen[name]; !selected {
			return fmt.Errorf("permission for MCP tool %q cannot enable a tool outside the allowlist", name)
		}
	}
	for name, fingerprint := range config.ToolDefinitionSHA256 {
		if _, selected := seen[name]; !selected {
			return fmt.Errorf("definition fingerprint for MCP tool %q is outside the allowlist", name)
		}
		if len(fingerprint) != 64 || !isLowerHex(fingerprint) {
			return fmt.Errorf("invalid definition fingerprint for MCP tool %q", name)
		}
	}
	return nil
}

func validateGlobalConfig(config MCPConfig) error       { return ValidateMCPConfig(config) }
func validateServerPolicy(config MCPServerConfig) error { return ValidateMCPServerConfig(config) }

func validPermission(permission Permission, allowInherit bool) bool {
	switch permission {
	case "", PermissionAsk, PermissionAllow, PermissionDeny:
		return true
	case PermissionInherit:
		return allowInherit
	default:
		return false
	}
}

func enabled(value *bool) bool {
	return value == nil || *value
}

func cloneServerConfig(config MCPServerConfig) MCPServerConfig {
	if config.unknownJSONFields != nil {
		fields := make(map[string]json.RawMessage, len(config.unknownJSONFields))
		for key, value := range config.unknownJSONFields {
			fields[key] = slices.Clone(value)
		}
		config.unknownJSONFields = fields
	}
	config.Args = slices.Clone(config.Args)
	config.Env = slices.Clone(config.Env)
	config.Tools = slices.Clone(config.Tools)
	config.Headers = cloneStringMap(config.Headers)
	if config.ToolPermissions != nil {
		permissions := make(map[string]Permission, len(config.ToolPermissions))
		for name, permission := range config.ToolPermissions {
			permissions[name] = permission
		}
		config.ToolPermissions = permissions
	}
	config.ToolDefinitionSHA256 = cloneStringMap(config.ToolDefinitionSHA256)
	return config
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func isLowerHex(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

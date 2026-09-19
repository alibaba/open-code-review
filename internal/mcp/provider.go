// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"unicode"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/tool"
)

// Provider is the authorization-gated bridge for one immutable MCP tool grant.
type Provider struct {
	grant      ToolGrant
	client     *Client
	authorizer Authorizer
}

func (p *Provider) Tool() tool.Tool { return tool.Dynamic(p.grant.ModelAlias) }

// SensitiveToolCall marks MCP arguments and results as untrusted, potentially
// secret-bearing content for callers that support redacted observability.
func (p *Provider) SensitiveToolCall() bool { return true }

// Execute independently authorizes immediately before the private tools/call
// sink. Every non-allow outcome fails closed.
func (p *Provider) Execute(ctx context.Context, args map[string]any) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if p == nil || p.client == nil || p.authorizer == nil {
		return "", ErrInteractionUnavailable
	}
	executionArguments := cloneArguments(args)
	decision, err := p.authorizer.Authorize(ctx, Invocation{
		Grant: p.grant, Arguments: cloneArguments(executionArguments),
	})
	if err != nil {
		return "", err
	}
	if decision != DecisionAllowOnce && decision != DecisionAllowReview {
		return "", ErrPermissionDenied
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	result, err := p.client.callTool(ctx, p.grant.ID.Name, executionArguments)
	if contextErr := ctx.Err(); contextErr != nil {
		return "", contextErr
	}
	return result, err
}

// Binding couples one ToolGrant to the exact model definition and provider
// created from the same discovered catalog record.
type Binding struct {
	Grant      ToolGrant
	Definition llm.ToolDef
	provider   *Provider
}

// Registration is the immutable result of a successful selected-tool build.
type Registration struct {
	Bindings []Binding
	ToolDefs []llm.ToolDef
	Skipped  []SkippedTool
}

// SkippedTool explains why a selected tool was intentionally hidden.
type SkippedTool struct {
	ID          ToolID
	Reason      string
	NeedsReview bool
}

// RegisterSelected validates the complete selection before mutating reg, then
// registers only explicit, policy-visible tools. In non-interactive mode, ask
// grants are hidden from the model because they cannot be authorized.
func RegisterSelected(
	reg *tool.Registry,
	clients []*Client,
	global MCPConfig,
	servers map[string]MCPServerConfig,
	authorizer Authorizer,
	interactive bool,
	existingDefs ...[]llm.ToolDef,
) (Registration, error) {
	if reg == nil {
		return Registration{}, errors.New("MCP registration requires a tool registry")
	}
	if authorizer == nil {
		return Registration{}, errors.New("MCP registration requires an authorizer")
	}
	if err := ValidateMCPConfig(global); err != nil {
		return Registration{}, err
	}
	if !enabled(global.Enabled) {
		return Registration{}, nil
	}

	reserved := make(map[string]struct{})
	for _, group := range existingDefs {
		for _, definition := range group {
			if definition.Function.Name != "" {
				reserved[definition.Function.Name] = struct{}{}
			}
		}
	}

	orderedClients := append([]*Client(nil), clients...)
	sort.Slice(orderedClients, func(i, j int) bool {
		if orderedClients[i] == nil {
			return false
		}
		if orderedClients[j] == nil {
			return true
		}
		return orderedClients[i].Name() < orderedClients[j].Name()
	})
	seenServers := make(map[string]struct{}, len(orderedClients))
	seenAliases := make(map[string]struct{})
	var pending []Binding
	var skipped []SkippedTool
	for _, client := range orderedClients {
		if client == nil {
			return Registration{}, errors.New("MCP client list contains nil")
		}
		serverName := client.Name()
		if err := validateServerName(serverName); err != nil {
			return Registration{}, err
		}
		if _, duplicate := seenServers[serverName]; duplicate {
			return Registration{}, fmt.Errorf("duplicate MCP client for server %q", serverName)
		}
		seenServers[serverName] = struct{}{}
		server, ok := servers[serverName]
		if !ok {
			return Registration{}, fmt.Errorf("MCP client %q has no matching server configuration", serverName)
		}
		if err := ValidateMCPServerConfig(server); err != nil {
			return Registration{}, fmt.Errorf("invalid MCP server %q policy: %w", serverName, err)
		}
		if err := verifyConnectionIdentity(client.connection, server); err != nil {
			return Registration{}, fmt.Errorf("MCP client %q connection does not match its configuration: %w", serverName, err)
		}
		if !enabled(server.Enabled) || len(server.Tools) == 0 {
			continue
		}
		catalog := make(map[string]DiscoveredTool, len(client.discovered))
		for _, item := range client.discovered {
			catalog[item.Name] = item
		}
		selected := append([]string(nil), server.Tools...)
		sort.Strings(selected)
		for _, toolName := range selected {
			id := ToolID{Server: serverName, Name: toolName}
			permission, err := ResolvePermission(global, server, toolName)
			if err != nil {
				return Registration{}, fmt.Errorf("resolve MCP server %q tool %q: %w", serverName, toolName, err)
			}
			if permission == PermissionDeny {
				skipped = append(skipped, SkippedTool{ID: id, Reason: "permission denied by policy"})
				continue
			}
			item, found := catalog[toolName]
			if !found {
				skipped = append(skipped, SkippedTool{ID: id, Reason: "selected tool was not discovered", NeedsReview: true})
				continue
			}
			fingerprint, err := definitionFingerprint(serverName, server, item.Name, item.Description, item.InputSchema)
			if err != nil {
				return Registration{}, fmt.Errorf("fingerprint MCP server %q tool %q: %w", serverName, toolName, err)
			}
			expected := server.ToolDefinitionSHA256[toolName]
			if global.Version >= 1 && expected == "" {
				skipped = append(skipped, SkippedTool{ID: id, Reason: "approved definition fingerprint is missing", NeedsReview: true})
				continue
			}
			if expected != "" && expected != fingerprint {
				skipped = append(skipped, SkippedTool{ID: id, Reason: "tool definition changed", NeedsReview: true})
				continue
			}
			if global.Version == 0 && expected == "" {
				permission = PermissionAsk
			}
			if !interactive && permission == PermissionAsk {
				skipped = append(skipped, SkippedTool{ID: id, Reason: "interactive approval is unavailable"})
				continue
			}
			alias := ModelAlias(id)
			if _, conflict := reserved[alias]; conflict {
				return Registration{}, fmt.Errorf("MCP tool alias %q conflicts with an existing tool definition", alias)
			}
			if _, conflict := reg.Get(alias); conflict {
				return Registration{}, fmt.Errorf("MCP tool alias %q conflicts with an existing provider", alias)
			}
			if _, conflict := seenAliases[alias]; conflict {
				return Registration{}, fmt.Errorf("MCP tools produced duplicate model alias %q", alias)
			}
			definition, err := definitionForBinding(alias, serverName, item)
			if err != nil {
				return Registration{}, err
			}
			grant := ToolGrant{
				ID: id, ModelAlias: alias, Permission: permission,
				DefinitionSHA256: fingerprint, UntrustedDescription: item.Description,
			}
			provider := &Provider{grant: grant, client: client, authorizer: authorizer}
			pending = append(pending, Binding{Grant: grant, Definition: definition, provider: provider})
			seenAliases[alias] = struct{}{}
		}
	}

	registration := Registration{
		Bindings: make([]Binding, len(pending)),
		ToolDefs: make([]llm.ToolDef, len(pending)),
		Skipped:  append([]SkippedTool(nil), skipped...),
	}
	copy(registration.Bindings, pending)
	for i := range pending {
		reg.Register(pending[i].provider)
		registration.ToolDefs[i] = pending[i].Definition
	}
	return registration, nil
}

func definitionForBinding(alias, serverName string, item DiscoveredTool) (llm.ToolDef, error) {
	var parameters map[string]any
	if err := json.Unmarshal(item.InputSchema, &parameters); err != nil {
		return llm.ToolDef{}, fmt.Errorf("decode MCP server %q tool %q schema: %w", serverName, item.Name, err)
	}
	description := fmt.Sprintf("UNTRUSTED server-provided MCP metadata from server %q, tool %q", serverName, item.Name)
	if item.Description != "" {
		description += ": " + item.Description
	}
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name: alias, Description: description, Parameters: parameters,
		},
	}, nil
}

func verifyConnectionIdentity(actual, configured MCPServerConfig) error {
	actualIdentity, err := sanitizedConnectionIdentity(actual)
	if err != nil {
		return err
	}
	configuredIdentity, err := sanitizedConnectionIdentity(configured)
	if err != nil {
		return err
	}
	actualJSON, err := json.Marshal(actualIdentity)
	if err != nil {
		return err
	}
	configuredJSON, err := json.Marshal(configuredIdentity)
	if err != nil {
		return err
	}
	if string(actualJSON) != string(configuredJSON) {
		return errors.New("sanitized connection identity differs")
	}
	return nil
}

func validateServerName(name string) error {
	if name == "" {
		return errors.New("MCP server name is empty")
	}
	if len(name) > maxToolNameBytes {
		return fmt.Errorf("MCP server name exceeds %d bytes", maxToolNameBytes)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("MCP server name contains control characters")
		}
	}
	return nil
}

func cloneArguments(args map[string]any) map[string]any {
	if args == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(args))
	for key, value := range args {
		result[key] = cloneArgumentValue(value)
	}
	return result
}

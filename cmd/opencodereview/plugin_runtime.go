// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/mcp"
	"github.com/alibaba/open-code-review/internal/plugin"
	"github.com/alibaba/open-code-review/internal/tool"
)

// initRuntimeMCPClients runs only after the command has selected a real review or
// scan. The resolver supplies the same installed snapshot used for rule identity.
func initRuntimeMCPClients(ctx context.Context, cfg *Config, resolver rules.Resolver, tools *tool.Registry, repoDir, version string) ([]*mcp.Client, error) {
	var installed []plugin.Installed
	if source, ok := resolver.(interface{ EnabledPlugins() []plugin.Installed }); ok {
		installed = source.EnabledPlugins()
	}
	pluginCfg, err := pluginMCPConfig(cfg, installed)
	if err != nil {
		return nil, err
	}
	// User-configured tools retain priority when a plugin exposes a tool with
	// the same name. The existing MCP warning identifies skipped collisions.
	clients := initMCPClients(ctx, cfg, tools, repoDir, version)
	clients = append(clients, initMCPClients(ctx, pluginCfg, tools, repoDir, version)...)
	return clients, nil
}

func pluginMCPConfig(cfg *Config, installed []plugin.Installed) (*Config, error) {
	servers := make(map[string]MCPServerConfig)
	for _, p := range installed {
		if !p.Enabled {
			continue
		}
		for name, server := range p.Manifest.MCPServers {
			label := "plugin/" + p.Manifest.ID + "/" + name
			if cfg != nil {
				if _, exists := cfg.MCPServers[label]; exists {
					return nil, fmt.Errorf("plugin MCP server %q conflicts with a user-configured server; rename the configured server", label)
				}
			}
			if _, exists := servers[label]; exists {
				return nil, fmt.Errorf("duplicate plugin MCP server %q", label)
			}
			replace := func(value string) string {
				return plugin.ExpandCommandValue(value, p.Dir)
			}
			args := make([]string, len(server.Args))
			for i, arg := range server.Args {
				args[i] = replace(arg)
			}
			env := make([]string, len(server.Env))
			for i, value := range server.Env {
				env[i] = replace(value)
			}
			headers := make(map[string]string, len(server.Headers))
			for key, value := range server.Headers {
				headers[key] = value
			}
			servers[label] = MCPServerConfig{
				Type: server.Type, Command: replace(server.Command), Args: args, Env: env,
				URL: server.URL, Headers: headers, Tools: append([]string(nil), server.Tools...),
			}
		}
	}
	if len(servers) == 0 {
		return nil, nil
	}
	return &Config{MCPServers: servers}, nil
}

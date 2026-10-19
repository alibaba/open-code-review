// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/alibaba/open-code-review/internal/plugin"
	"github.com/spf13/cobra"
)

type mcpExecutionInfo struct {
	Name       string `json:"name"`
	Transport  string `json:"transport"`
	Execution  string `json:"execution"`
	Activation string `json:"activation"`
}

func describeMCPExecution(name, transport string) mcpExecutionInfo {
	info := mcpExecutionInfo{Name: name, Transport: "stdio", Execution: "local_process", Activation: "review_or_scan"}
	if transport == "remote" {
		info.Transport, info.Execution = "remote", "remote_connection"
	}
	return info
}

type mcpListEntry struct {
	mcpExecutionInfo
	Source        string `json:"source"`
	Plugin        string `json:"plugin,omitempty"`
	Version       string `json:"version,omitempty"`
	State         string `json:"state"`
	Authorization string `json:"authorization"`
}

type mcpListResult struct {
	ProjectScoped    bool           `json:"project_scoped"`
	ConnectionStatus string         `json:"connection_status"`
	Servers          []mcpListEntry `json:"servers"`
}

func newMCPCmd() *cobra.Command {
	command := &cobra.Command{Use: "mcp", Short: "Inspect MCP configuration from user settings and plugins", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	var repo string
	var jsonOutput bool
	list := &cobra.Command{
		Use: "list", Short: "List MCP configuration without starting servers or checking connections", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			directory, _, err := resolveWorkingDir(repo, false)
			if err != nil {
				return err
			}
			result, err := inspectMCPConfiguration(cmd.Context(), directory)
			if err != nil {
				return err
			}
			if jsonOutput {
				return pluginJSON(cmd.OutOrStdout(), result)
			}
			out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(out, "SERVER\tSOURCE\tPLUGIN\tVERSION\tEXECUTION\tSTATE\tAUTHORIZATION")
			for _, entry := range result.Servers {
				fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", pluginText(entry.Name), entry.Source,
					pluginText(entry.Plugin), pluginText(entry.Version), entry.Execution, entry.State, entry.Authorization)
			}
			if err := out.Flush(); err != nil {
				return err
			}
			if len(result.Servers) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No MCP servers configured for this directory.")
			}
			if result.ProjectScoped {
				fmt.Fprintln(cmd.OutOrStdout(), "Project plugins selected; global plugin entries are excluded.")
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Configuration only. Connections and tool-name conflicts are not checked; no setup scripts or servers were started.")
			return err
		},
	}
	addRepoFlag(list, &repo)
	list.Flags().Lookup("repo").Usage = "project root containing .opencodereview (default: current dir)"
	list.Flags().BoolVar(&jsonOutput, "json", false, "output structured configuration and authorization state")
	command.AddCommand(list)
	return command
}

func inspectMCPConfiguration(ctx context.Context, directory string) (mcpListResult, error) {
	result := mcpListResult{ConnectionStatus: "not_checked", Servers: []mcpListEntry{}}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	path, err := defaultConfigPath()
	if err != nil {
		return result, err
	}
	cfg, err := loadOrCreateConfig(path)
	if err != nil {
		return result, err
	}
	for name, server := range cfg.MCPServers {
		entry := mcpListEntry{mcpExecutionInfo: describeMCPExecution(name, server.Type), Source: "user_config", State: "configured", Authorization: "user_configured"}
		if server.Type == "remote" {
			if server.URL == "" {
				entry.State = "missing_url"
			}
		} else {
			if server.Command == "" {
				entry.State = "missing_command"
			}
			if server.Setup != "" {
				entry.Execution = "local_setup_and_process"
			}
		}
		result.Servers = append(result.Servers, entry)
	}
	store, err := plugin.DefaultStore()
	if err != nil {
		return result, err
	}
	project, err := store.Project(directory)
	if err != nil {
		return result, err
	}
	inspection, err := project.Inspect(ctx)
	if err != nil {
		return result, err
	}
	result.ProjectScoped = inspection.Present
	installed := inspection.Plugins
	if !inspection.Present {
		installed, err = store.List()
		if err != nil {
			return result, err
		}
	}
	conflicts := make(map[string]bool)
	for _, item := range installed {
		incompatible := item.Enabled && item.Manifest.CheckCompatibility(plugin.HostVersion) != nil
		for name, server := range item.Manifest.MCPServers {
			label := "plugin/" + item.Manifest.ID + "/" + name
			entry := mcpListEntry{mcpExecutionInfo: describeMCPExecution(label, server.Type), Source: "global_plugin",
				Plugin: item.Manifest.ID, Version: item.Manifest.Version, State: "disabled", Authorization: "not_enabled"}
			if item.Enabled {
				entry.State, entry.Authorization = "configured", "enabled_by_user"
				if incompatible {
					entry.State = "incompatible"
				}
			}
			if inspection.Present {
				entry.Source = "project_plugin"
				entry.State, entry.Authorization = "authorization_required", "required"
				if inspection.Authorized {
					entry.State, entry.Authorization = "configured", "trusted_project_lock"
				}
			}
			if _, exists := cfg.MCPServers[label]; exists && entry.State == "configured" {
				conflicts[label] = true
			}
			result.Servers = append(result.Servers, entry)
		}
	}
	for index := range result.Servers {
		if conflicts[result.Servers[index].Name] {
			result.Servers[index].State = "server_name_conflict"
		}
	}
	sort.Slice(result.Servers, func(i, j int) bool {
		a, b := result.Servers[i], result.Servers[j]
		if a.Name == b.Name {
			return a.Source < b.Source
		}
		return a.Name < b.Name
	})
	return result, nil
}

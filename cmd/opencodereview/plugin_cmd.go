// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/alibaba/open-code-review/internal/plugin"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

type pluginReviewInfo struct {
	Protocol string   `json:"protocol"`
	Stages   []string `json:"stages"`
}

// pluginInfo omits execution arguments, environment variables and headers,
// which can contain credentials.
type pluginInfo struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	Version             string               `json:"version"`
	Description         string               `json:"description"`
	Enabled             bool                 `json:"enabled"`
	Source              string               `json:"source"`
	Directory           string               `json:"directory"`
	Rules               []string             `json:"rules"`
	MCPServers          []string             `json:"mcp_servers"`
	MCPExecution        []mcpExecutionInfo   `json:"mcp_execution"`
	Review              *pluginReviewInfo    `json:"review,omitempty"`
	SHA256              string               `json:"sha256,omitempty"`
	ArchiveSHA256       string               `json:"archive_sha256,omitempty"`
	ContentSHA256       string               `json:"content_sha256,omitempty"`
	ContentDigestFormat string               `json:"content_digest_format,omitempty"`
	Requires            *plugin.Requirements `json:"requires,omitempty"`
	Revision            string               `json:"revision,omitempty"`
}

func pluginManagementInfo(installed plugin.Installed) pluginInfo {
	m := installed.Manifest
	names := make([]string, 0, len(m.MCPServers))
	for name := range m.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	execution := make([]mcpExecutionInfo, 0, len(names))
	for _, name := range names {
		execution = append(execution, describeMCPExecution(name, m.MCPServers[name].Type))
	}
	var review *pluginReviewInfo
	if m.Review != nil {
		review = &pluginReviewInfo{Protocol: m.Review.Protocol, Stages: append([]string{}, m.Review.Stages...)}
	}
	return pluginInfo{
		ID: m.ID, Name: m.Name, Version: m.Version, Description: m.Description,
		Enabled: installed.Enabled, Source: plugin.DisplaySource(installed.Source), Directory: installed.Dir,
		Rules: append([]string{}, m.Rules...), MCPServers: names, MCPExecution: execution, Review: review,
		SHA256: installed.SHA256, Revision: installed.Revision,
		ArchiveSHA256: installed.ArchiveSHA256, ContentSHA256: installed.ContentSHA256, ContentDigestFormat: installed.ContentDigestFormat, Requires: m.Requires,
	}
}

type pluginInstallPreview struct {
	Source  string       `json:"source"`
	Plugins []pluginInfo `json:"plugins"`
}

func printPluginInstallPreview(out io.Writer, preview pluginInstallPreview, jsonOutput bool) error {
	if jsonOutput {
		return pluginJSON(out, preview)
	}
	if _, err := fmt.Fprintf(out, "Install source: %s\n", pluginText(preview.Source)); err != nil {
		return err
	}
	for _, p := range preview.Plugins {
		if _, err := fmt.Fprintf(out, "%s  %s  %s\n  %s\n", pluginText(p.ID), pluginText(p.Version), pluginText(p.Name), pluginText(p.Description)); err != nil {
			return err
		}
		if len(preview.Plugins) == 1 {
			if _, err := fmt.Fprintf(out, "Rules: %s\nMCP servers: %s\n", pluginText(strings.Join(p.Rules, ", ")), pluginText(strings.Join(p.MCPServers, ", "))); err != nil {
				return err
			}
			if p.Review != nil {
				if _, err := fmt.Fprintf(out, "Review stages: %s\n", pluginText(strings.Join(p.Review.Stages, ", "))); err != nil {
					return err
				}
			}
			if err := printMCPExecution(out, p.MCPExecution); err != nil {
				return err
			}
		}
	}
	message := "No plugin was installed."
	if len(preview.Plugins) > 1 {
		message += " Use --plugin ID to choose from multiple plugins."
	}
	_, err := fmt.Fprintln(out, message)
	return err
}

func printMCPExecution(out io.Writer, entries []mcpExecutionInfo) error {
	for _, entry := range entries {
		effect := "starts a local process with your user permissions"
		if entry.Transport == "remote" {
			effect = "opens a connection to a remote service"
		}
		if _, err := fmt.Fprintf(out, "MCP %s: %s during review/scan when the plugin is active and authorized.\n", pluginText(entry.Name), effect); err != nil {
			return err
		}
	}
	return nil
}

func pluginText(value string) string {
	return strings.Join(strings.Fields(sanitizeTerminal(ansi.Strip(value))), " ")
}

func pluginJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func newPluginCmd() *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use: "plugin", Aliases: []string{"plugins"}, Short: "Manage OCR plugins",
		Long: "Install rule packs and MCP tools for OCR. Installation copies the package without executing it. Enable a plugin to use its rules and tools in subsequent reviews.",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	command.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output structured JSON")
	addProjectPluginCommands(command, &jsonOutput)
	withStore := func(action func(*cobra.Command, []string, *plugin.Store) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			store, err := plugin.DefaultStore()
			if err != nil {
				return err
			}
			return action(cmd, args, store)
		}
	}
	printInstalled := func(cmd *cobra.Command, installed plugin.Installed) error {
		info := pluginManagementInfo(installed)
		if jsonOutput {
			return pluginJSON(cmd.OutOrStdout(), info)
		}
		state := "disabled"
		if info.Enabled {
			state = "enabled"
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s (%s)\n%s\nSource: %s\nPackage: %s\nRules: %s\nMCP servers: %s\n",
			pluginText(info.ID), pluginText(info.Version), state, pluginText(info.Description), pluginText(info.Source),
			pluginText(info.Directory), pluginText(strings.Join(info.Rules, ", ")), pluginText(strings.Join(info.MCPServers, ", ")))
		if err == nil && info.Review != nil {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Run once: ocr review --review-plugin %s\n", pluginText(info.ID))
		}
		if err == nil {
			err = printMCPExecution(cmd.OutOrStdout(), info.MCPExecution)
		}
		if err == nil && info.Revision != "" {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Revision: %s\n", pluginText(info.Revision))
		}
		if err == nil && info.ArchiveSHA256 != "" {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Archive SHA-256: %s\n", pluginText(info.ArchiveSHA256))
		}
		if err == nil && info.ContentSHA256 != "" {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Content SHA-256 (%s): %s\n", pluginText(info.ContentDigestFormat), pluginText(info.ContentSHA256))
		} else if err == nil && info.SHA256 != "" {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Legacy SHA-256 (kind not recorded): %s\n", pluginText(info.SHA256))
		}
		return err
	}
	command.AddCommand(&cobra.Command{
		Use: "list", Short: "List installed plugins and their enabled state", Args: cobra.NoArgs,
		RunE: withStore(func(cmd *cobra.Command, _ []string, store *plugin.Store) error {
			installed, err := store.List()
			if err != nil {
				return err
			}
			infos := make([]pluginInfo, 0, len(installed))
			for _, item := range installed {
				infos = append(infos, pluginManagementInfo(item))
			}
			if jsonOutput {
				return pluginJSON(cmd.OutOrStdout(), infos)
			}
			if len(infos) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "No plugins installed. Use 'ocr plugin install URL_OR_PATH'.")
				return err
			}
			out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(out, "ID\tVERSION\tSTATE\tCAPABILITIES")
			for _, info := range infos {
				state := "disabled"
				if info.Enabled {
					state = "enabled"
				}
				capabilities := fmt.Sprintf("%d rule packs, %d MCP servers", len(info.Rules), len(info.MCPServers))
				if info.Review != nil {
					capabilities += ", review policy (explicit selection)"
				}
				fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", pluginText(info.ID), pluginText(info.Version), state, capabilities)
			}
			return out.Flush()
		}),
	})
	command.AddCommand(&cobra.Command{
		Use: "show <id>", Short: "Inspect an installed plugin without exposing tool credentials", Args: exactArgs(1),
		RunE: withStore(func(cmd *cobra.Command, args []string, store *plugin.Store) error {
			installed, err := findInstalledPlugin(store, args[0])
			if err != nil {
				return err
			}
			return printInstalled(cmd, installed)
		}),
	})
	var enable, dryRun bool
	var selectedPlugin string
	install := &cobra.Command{
		Use: "install <url|path>", Short: "Install from a plugin URL or local package",
		Example: "  ocr plugin install https://github.com/OWNER/PLUGIN\n  ocr plugin install https://example.com/plugin.tar.gz --enable\n  ocr plugin install ./examples/ocr-plugins/review-checklist", Args: exactArgs(1),
		RunE: withStore(func(cmd *cobra.Command, args []string, store *plugin.Store) error {
			prepared, err := store.PrepareInstall(cmd.Context(), args[0], selectedPlugin)
			var choices *plugin.SelectionRequiredError
			if errors.As(err, &choices) {
				preview := pluginInstallPreview{Source: plugin.DisplaySource(args[0]), Plugins: make([]pluginInfo, 0, len(choices.Candidates))}
				for _, item := range choices.Candidates {
					preview.Plugins = append(preview.Plugins, pluginInfo{ID: item.ID, Name: item.Name, Version: item.Version, Description: item.Description, Source: plugin.DisplaySource(item.Source)})
				}
				if printErr := printPluginInstallPreview(cmd.OutOrStdout(), preview, jsonOutput); printErr != nil {
					return printErr
				}
				if dryRun {
					return nil
				}
				return fmt.Errorf("source contains multiple plugins; choose one with --plugin ID")
			}
			if err != nil {
				return err
			}
			defer prepared.Close()
			if dryRun {
				info := pluginManagementInfo(plugin.Installed{Manifest: prepared.Manifest, Source: prepared.Source, SHA256: prepared.SHA256, Revision: prepared.Revision, ArchiveSHA256: prepared.ArchiveSHA256, ContentSHA256: prepared.ContentSHA256, ContentDigestFormat: prepared.ContentDigestFormat})
				return printPluginInstallPreview(cmd.OutOrStdout(), pluginInstallPreview{Source: info.Source, Plugins: []pluginInfo{info}}, jsonOutput)
			}
			installed, err := store.InstallPrepared(cmd.Context(), prepared, enable)
			if err != nil {
				return err
			}
			if err := printInstalled(cmd, installed); err != nil {
				return err
			}
			if !jsonOutput && !installed.Enabled {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Enable with: ocr plugin enable %s\n", pluginText(installed.Manifest.ID))
			}
			return err
		}),
	}
	install.Flags().BoolVar(&enable, "enable", false, "activate the package for subsequent reviews; its MCP tools can execute code")
	install.Flags().StringVar(&selectedPlugin, "plugin", "", "select a plugin ID when a link contains multiple plugins")
	install.Flags().BoolVar(&dryRun, "dry-run", false, "download and inspect the source without installing or enabling plugins")
	command.AddCommand(install)
	var acceptPermissions bool
	update := &cobra.Command{
		Use: "update <id>", Short: "Refresh a plugin; changed execution declarations require explicit acceptance", Args: exactArgs(1),
		RunE: withStore(func(cmd *cobra.Command, args []string, store *plugin.Store) error {
			installed, err := store.UpdateWithOptions(cmd.Context(), args[0], plugin.UpdateOptions{AcceptPermissions: acceptPermissions})
			if err != nil {
				var changed *plugin.ExecutionChangesError
				if jsonOutput && errors.As(err, &changed) {
					if printErr := pluginJSON(cmd.OutOrStdout(), map[string]any{"error": map[string]any{"code": "PLUGIN_EXECUTION_CHANGED", "plugin_id": changed.PluginID, "changes": changed.Changes}}); printErr != nil {
						return printErr
					}
				}
				return err
			}
			return printInstalled(cmd, installed)
		}),
	}
	update.Flags().BoolVar(&acceptPermissions, "accept-permissions", false, "accept added or changed MCP and workflow execution declarations")
	command.AddCommand(update)
	for _, action := range []string{"enable", "disable", "remove"} {
		command.AddCommand(&cobra.Command{
			Use: action + " <id>", Short: strings.ToUpper(action[:1]) + action[1:] + " an installed plugin", Args: exactArgs(1),
			RunE: withStore(func(cmd *cobra.Command, args []string, store *plugin.Store) error {
				var execution []mcpExecutionInfo
				var err error
				if action == "remove" {
					err = store.Remove(args[0])
				} else {
					var item plugin.Installed
					item, err = store.SetEnabledWithResult(args[0], action == "enable")
					if err == nil && action == "enable" {
						execution = pluginManagementInfo(item).MCPExecution
					}
				}
				if err != nil {
					return err
				}
				if jsonOutput {
					result := map[string]any{"id": args[0], "action": action, "status": "ok"}
					if action == "enable" {
						result["mcp_execution"] = execution
					}
					return pluginJSON(cmd.OutOrStdout(), result)
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Plugin %s: %s complete. Changes apply to subsequent reviews.\n", pluginText(args[0]), action)
				if err == nil {
					err = printMCPExecution(cmd.OutOrStdout(), execution)
				}
				return err
			}),
		})
	}
	return command
}

func findInstalledPlugin(store *plugin.Store, id string) (plugin.Installed, error) {
	installed, err := store.List()
	if err != nil {
		return plugin.Installed{}, err
	}
	for _, item := range installed {
		if item.Manifest.ID == id {
			return item, nil
		}
	}
	return plugin.Installed{}, fmt.Errorf("plugin %q is not installed", id)
}

// runPluginCommand shares command parsing and behavior with the TUI without
// executing a shell, using the global Cobra command, or writing to the terminal.
func runPluginCommand(ctx context.Context, args []string) (string, error) {
	var out bytes.Buffer
	command := newPluginCmd()
	command.SetOut(&out)
	command.SetErr(&out)
	command.SetArgs(args)
	err := command.ExecuteContext(ctx)
	return out.String(), err
}

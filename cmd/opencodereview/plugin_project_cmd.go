// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/alibaba/open-code-review/internal/plugin"
	"github.com/spf13/cobra"
)

func addProjectPluginCommands(parent *cobra.Command, jsonOutput *bool) {
	command := func(use, short string, args cobra.PositionalArgs, action func(*cobra.Command, []string, *plugin.Project) (any, error)) *cobra.Command {
		var repo string
		cmd := &cobra.Command{Use: use, Short: short, Args: args}
		addRepoFlag(cmd, &repo)
		cmd.Flags().Lookup("repo").Usage = "project root containing .opencodereview (default: current dir)"
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			directory, _, err := resolveWorkingDir(repo, false)
			var store *plugin.Store
			if err == nil {
				store, err = plugin.DefaultStore()
			}
			var value any
			if err == nil {
				var project *plugin.Project
				project, err = store.Project(directory)
				if err == nil {
					value, err = action(cmd, args, project)
				}
			}
			if err != nil {
				if *jsonOutput {
					coded := &plugin.ProjectError{Code: "PROJECT_PLUGIN_ERROR", Message: err.Error()}
					var projectErr *plugin.ProjectError
					if errors.As(err, &projectErr) {
						coded = projectErr
					}
					if printErr := pluginJSON(cmd.OutOrStdout(), map[string]any{"error": coded}); printErr != nil {
						return printErr
					}
				}
				return err
			}
			if *jsonOutput {
				return pluginJSON(cmd.OutOrStdout(), value)
			}
			switch value := value.(type) {
			case plugin.ProjectLock:
				platform := runtime.GOOS + "/" + runtime.GOARCH
				for _, entry := range value.Platforms[platform] {
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s  %s\n", pluginText(entry.ID), pluginText(entry.Version), pluginText(entry.Source))
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Project lock: %s. Run 'ocr plugin sync --frozen' to install it.\n", platform)
			case []pluginInfo:
				for _, item := range value {
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", pluginText(item.ID), pluginText(item.Version))
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Project plugins verified. No plugin commands were executed.")
			case map[string]bool:
				message := "Project execution authorization revoked."
				if value["trusted"] {
					message = "Project execution authorization saved for this lock and platform."
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), message)
			default:
				_, err = fmt.Fprintln(cmd.OutOrStdout(), value)
			}
			return err
		}
		parent.AddCommand(cmd)
		return cmd
	}
	var selection string
	add := command("add <source>", "Declare and lock a plugin for this project", exactArgs(1), func(cmd *cobra.Command, args []string, p *plugin.Project) (any, error) {
		return p.Add(cmd.Context(), args[0], selection)
	})
	add.Flags().StringVar(&selection, "plugin", "", "select a plugin ID from a multi-plugin source")
	command("lock", "Resolve declared sources for this platform and write the project lock", cobra.NoArgs, func(cmd *cobra.Command, _ []string, p *plugin.Project) (any, error) {
		return p.Lock(cmd.Context())
	})
	var frozen, offline bool
	sync := command("sync", "Install exact project artifacts without executing plugins", cobra.NoArgs, func(cmd *cobra.Command, _ []string, p *plugin.Project) (any, error) {
		if !frozen {
			return nil, &plugin.ProjectError{Code: "LOCK_MISMATCH", Message: "sync requires --frozen; use lock to resolve new versions"}
		}
		items, err := p.Sync(cmd.Context(), offline)
		return projectPluginInfo(items), err
	})
	sync.Flags().BoolVar(&frozen, "frozen", false, "require the committed declaration and lock to match")
	sync.Flags().BoolVar(&offline, "offline", false, "use cached snapshots or repository-local sources without network access")
	var revoke bool
	trust := command("trust", "Authorize the current project lock's executable contributions locally", cobra.NoArgs, func(cmd *cobra.Command, _ []string, p *plugin.Project) (any, error) {
		err := p.Trust(cmd.Context(), revoke)
		return map[string]bool{"trusted": !revoke}, err
	})
	trust.Flags().BoolVar(&revoke, "revoke", false, "revoke authorization for the current project lock")
	command("project", "Inspect the project lock without installing or authorizing it", cobra.NoArgs, func(_ *cobra.Command, _ []string, p *plugin.Project) (any, error) {
		return p.Explain()
	})
	var fingerprint bool
	preflight := command("preflight", "Verify project plugins and execution authorization without model access", cobra.NoArgs, func(cmd *cobra.Command, _ []string, p *plugin.Project) (any, error) {
		if fingerprint {
			return p.Fingerprint(cmd.Context())
		}
		items, present, err := p.Load(cmd.Context())
		if err == nil && !present {
			err = &plugin.ProjectError{Code: "LOCK_MISMATCH", Message: "no project plugins declared"}
		}
		return projectPluginInfo(items), err
	})
	preflight.Flags().BoolVar(&fingerprint, "fingerprint", false, "print the verified lock identity for incremental review")
}

func projectPluginInfo(items []plugin.Installed) []pluginInfo {
	result := make([]pluginInfo, 0, len(items))
	for _, item := range items {
		result = append(result, pluginManagementInfo(item))
	}
	return result
}

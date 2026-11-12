// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/alibaba/open-code-review/internal/plugin"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

func pluginText(value string) string {
	return strings.Join(strings.Fields(sanitizeTerminal(ansi.Strip(value))), " ")
}

func newPluginCmd() *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use: "plugin", Short: "Manage local rule plugins", Args: cobra.NoArgs,
		Long:         "Install local rule packages as snapshots. Enable a package to add its rules to subsequent reviews and scans.",
		SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	command.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output structured JSON")
	withStore := func(action func(*cobra.Command, []string, *plugin.Store) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			store, err := plugin.DefaultStore()
			if err != nil {
				return err
			}
			return action(cmd, args, store)
		}
	}
	encode := func(cmd *cobra.Command, value any) error {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	printItem := func(cmd *cobra.Command, item plugin.Installed) error {
		if jsonOutput {
			return encode(cmd, item)
		}
		state := "disabled"
		if item.Enabled {
			state = "enabled"
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s (%s)\n%s\nSource: %s\nPackage: %s\nRules: %s\nContent SHA-256: %s\n",
			pluginText(item.Manifest.ID), pluginText(item.Manifest.Version), state, pluginText(item.Manifest.Description),
			pluginText(item.Source), pluginText(item.Dir), pluginText(strings.Join(item.Manifest.Rules, ", ")), pluginText(item.ContentSHA256))
		return err
	}
	command.AddCommand(&cobra.Command{
		Use: "list", Short: "List installed rule plugins", Args: cobra.NoArgs,
		RunE: withStore(func(cmd *cobra.Command, _ []string, store *plugin.Store) error {
			items, err := store.List()
			if err != nil {
				return err
			}
			if jsonOutput {
				return encode(cmd, items)
			}
			out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(out, "ID\tVERSION\tSTATE\tRULE PACKS")
			for _, item := range items {
				state := "disabled"
				if item.Enabled {
					state = "enabled"
				}
				fmt.Fprintf(out, "%s\t%s\t%s\t%d\n", pluginText(item.Manifest.ID), pluginText(item.Manifest.Version), state, len(item.Manifest.Rules))
			}
			return out.Flush()
		}),
	})
	command.AddCommand(&cobra.Command{
		Use: "show <id>", Short: "Inspect an installed rule plugin", Args: exactArgs(1),
		RunE: withStore(func(cmd *cobra.Command, args []string, store *plugin.Store) error {
			items, err := store.List()
			if err != nil {
				return err
			}
			for _, item := range items {
				if item.Manifest.ID == args[0] {
					return printItem(cmd, item)
				}
			}
			return fmt.Errorf("plugin %q is not installed", args[0])
		}),
	})
	var enable bool
	install := &cobra.Command{
		Use: "install <directory>", Short: "Install a local rule package", Args: exactArgs(1),
		RunE: withStore(func(cmd *cobra.Command, args []string, store *plugin.Store) error {
			item, err := store.Install(cmd.Context(), args[0], enable)
			if err != nil {
				return err
			}
			return printItem(cmd, item)
		}),
	}
	install.Flags().BoolVar(&enable, "enable", false, "enable rules after installation")
	command.AddCommand(install)
	for _, enabled := range []bool{true, false} {
		verb := "disable"
		if enabled {
			verb = "enable"
		}
		command.AddCommand(&cobra.Command{
			Use: verb + " <id>", Short: verb + " a rule plugin", Args: exactArgs(1),
			RunE: withStore(func(cmd *cobra.Command, args []string, store *plugin.Store) error {
				item, err := store.SetEnabledWithResult(args[0], enabled)
				if err != nil {
					return err
				}
				return printItem(cmd, item)
			}),
		})
	}
	command.AddCommand(&cobra.Command{
		Use: "remove <id>", Short: "Unregister a rule plugin", Args: exactArgs(1),
		RunE: withStore(func(cmd *cobra.Command, args []string, store *plugin.Store) error {
			if err := store.Remove(args[0]); err != nil {
				return err
			}
			if jsonOutput {
				return encode(cmd, map[string]string{"removed": args[0]})
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", pluginText(args[0]))
			return err
		}),
	})
	return command
}

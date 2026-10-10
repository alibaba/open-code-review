// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/spf13/cobra"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Manage official provider OAuth login"}
	for _, action := range []string{"login", "status", "logout"} {
		child := &cobra.Command{
			Use:   action + " <codex-oauth|anthropic-oauth>",
			Short: action + " using the provider's official credential store",
			Args:  exactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				headless, _ := cmd.Flags().GetBool("headless")
				return llm.RunOAuthAuth(cmd.Context(), args[0], action, headless, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			},
		}
		if action == "login" {
			child.Flags().Bool("headless", false, "Use device authentication (Codex only)")
		}
		cmd.AddCommand(child)
	}
	return cmd
}

func init() { rootCmd.AddCommand(newAuthCmd()) }

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import "github.com/spf13/cobra"

var (
	authLoginDevice    bool
	authLoginNoBrowser bool
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage Sign in with ChatGPT authentication",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Sign in with ChatGPT",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateAuthProvider(); err != nil {
			return err
		}
		return runChatGPTLogin(cmd.Context(), cmd.OutOrStdout())
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show saved ChatGPT registrations and authentication status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateAuthProvider(); err != nil {
			return err
		}
		return runChatGPTStatus(cmd.OutOrStdout())
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Revoke the ChatGPT session and clear local tokens",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateAuthProvider(); err != nil {
			return err
		}
		return runChatGPTLogout(cmd.Context(), cmd.OutOrStdout())
	},
}

func init() {
	authLoginCmd.Flags().BoolVar(&authLoginDevice, "device", false, "unsupported in the ChatGPT preview; use the local browser loopback flow")
	authLoginCmd.Flags().BoolVar(&authLoginNoBrowser, "no-browser", false, "print a one-time local URL instead of opening a browser")
	authLoginCmd.MarkFlagsMutuallyExclusive("device", "no-browser")
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)
}

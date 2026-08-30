// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/alibaba/open-code-review/internal/codexauth"
	"github.com/spf13/cobra"
)

var (
	authStore           codexauth.CodexStore = codexauth.FileStore{}
	newCodexOAuthClient                      = codexauth.NewOAuthClient
	authLoginDevice     bool
	authLoginNoBrowser  bool
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage ChatGPT Codex subscription authentication",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Sign in with a ChatGPT Codex subscription",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newCodexOAuthClient()
		var auth *codexauth.CodexAuth
		var err error
		if authLoginDevice {
			auth, err = client.LoginDevice(cmd.Context(), authStore, cmd.OutOrStdout())
		} else {
			auth, err = client.LoginLoopback(cmd.Context(), authStore, authLoginNoBrowser, cmd.OutOrStdout())
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Signed in to ChatGPT account %s.\n", maskedAccount(auth.AccountID))
		return err
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current Codex subscription authentication status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAuthStatus(cmd.OutOrStdout(), authStore, time.Now())
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Revoke and remove local Codex subscription credentials",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAuthLogout(cmd.Context(), cmd.OutOrStdout(), authStore, newCodexOAuthClient())
	},
}

func init() {
	authLoginCmd.Flags().BoolVar(&authLoginDevice, "device", false, "use the device-code flow for headless or remote shells")
	authLoginCmd.Flags().BoolVar(&authLoginNoBrowser, "no-browser", false, "print the authorization URL instead of opening a browser")
	authLoginCmd.MarkFlagsMutuallyExclusive("device", "no-browser")
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)
}

func runAuthStatus(out io.Writer, store codexauth.CodexStore, now time.Time) error {
	auth, err := store.Load()
	if errors.Is(err, codexauth.ErrNotFound) {
		_, err = fmt.Fprintln(out, "Not signed in, run ocr auth login.")
		return err
	}
	if err != nil {
		return fmt.Errorf("load Codex authentication status: %w", err)
	}
	plan := auth.PlanType
	if plan == "" {
		plan = "(unknown)"
	}
	expiry := "(unknown)"
	if !auth.ExpiresAt.IsZero() {
		expiry = auth.ExpiresAt.Local().Format(time.RFC3339)
		if !auth.ExpiresAt.After(now) {
			if auth.RefreshToken != "" {
				expiry += " (expired; will refresh on next use)"
			} else {
				expiry += " (expired)"
			}
		}
	}
	_, err = fmt.Fprintf(out, "Account: %s\nPlan:    %s\nExpires: %s\n", maskedAccount(auth.AccountID), plan, expiry)
	return err
}

func runAuthLogout(ctx context.Context, out io.Writer, store codexauth.CodexStore, client *codexauth.OAuthClient) error {
	var loadErr, revokeErr error
	// Hold the same cross-process lock a concurrent refresh uses: without it a
	// mid-refresh `ocr review` can save rotated credentials after this Clear,
	// resurrecting a session whose pre-rotation token the Revoke just killed.
	err := codexauth.WithRefreshLock(ctx, func() error {
		auth, loadLoadErr := store.Load()
		loadErr = loadLoadErr
		if loadLoadErr == nil {
			revokeErr = client.Revoke(ctx, auth)
		}
		if clearErr := store.Clear(); clearErr != nil {
			return fmt.Errorf("clear local Codex credentials: %w", clearErr)
		}
		return nil
	})
	if err != nil {
		return err
	}
	const accessTokenCaveat = "Already-issued access tokens may remain valid for up to ten days."
	if loadErr != nil && !errors.Is(loadErr, codexauth.ErrNotFound) {
		_, _ = fmt.Fprintln(out, "Local credentials were removed, but loading them for server-side revocation failed.", accessTokenCaveat)
		return nil
	}
	if errors.Is(loadErr, codexauth.ErrNotFound) {
		_, _ = fmt.Fprintln(out, "No local Codex credentials were found.", accessTokenCaveat)
		return nil
	}
	if revokeErr != nil {
		_, _ = fmt.Fprintln(out, "Local credentials were removed, but server-side revocation failed.", accessTokenCaveat)
		return nil
	}
	_, err = fmt.Fprintln(out, "The refresh token was revoked and local Codex credentials were removed.", accessTokenCaveat)
	return err
}

func maskedAccount(account string) string {
	if account == "" {
		return "(unknown)"
	}
	if len(account) <= 8 {
		return "***"
	}
	return account[:4] + "***" + account[len(account)-4:]
}

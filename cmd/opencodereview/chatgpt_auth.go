// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/alibaba/open-code-review/internal/chatgptauth"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/spf13/cobra"
)

var authProvider = "chatgpt"
var authAccount string
var authNewAccount bool
var authEnablePlan bool
var loginChatGPT = func(ctx context.Context, store chatgptauth.Store, account string, newAccount, noBrowser, consent bool, out io.Writer) (*chatgptauth.Auth, error) {
	return chatgptauth.NewClient().Login(ctx, store, account, newAccount, noBrowser, consent, out)
}
var logoutChatGPT = func(ctx context.Context, store chatgptauth.Store, account string) error {
	return chatgptauth.NewClient().Logout(ctx, store, account)
}

func validateAuthProvider() error {
	if authProvider != "chatgpt" {
		return fmt.Errorf("unknown auth provider %q; expected chatgpt", authProvider)
	}
	return nil
}
func runChatGPTLogin(ctx context.Context, out io.Writer) error {
	if authLoginDevice {
		return errors.New("chatgpt SIWC preview does not support --device; use the local browser loopback flow")
	}
	s, err := chatgptauth.DefaultStore()
	if err != nil {
		return err
	}
	auth, err := loginChatGPT(ctx, s, authAccount, authNewAccount, authLoginNoBrowser, authEnablePlan, out)
	if err != nil {
		return err
	}
	if !auth.PlanEnabled() {
		_, err = fmt.Fprintln(out, "Signed in, but ChatGPT plan usage is disabled. Run ocr auth login --provider chatgpt --enable-plan to grant plan permission, or configure an API-key provider.")
		return err
	}
	_, err = fmt.Fprintln(out, "Signed in with ChatGPT. Using ChatGPT plan. Manage usage:", chatgptauth.UsageURL)
	return err
}
func runChatGPTStatus(out io.Writer) error {
	s, err := chatgptauth.DefaultStore()
	if err != nil {
		return err
	}
	d, err := s.Snapshot()
	if err != nil {
		return err
	}
	if len(d.Accounts) == 0 {
		_, err = fmt.Fprintln(out, "Not signed in, run ocr auth login --provider chatgpt.")
		return err
	}
	for _, a := range d.Accounts {
		selected := ""
		if a.ClientID == d.Active {
			selected = " (active)"
		}
		status := "signed out"
		if a.AccessToken != "" {
			status = "signed in; ChatGPT plan usage disabled; run ocr auth login --provider chatgpt --enable-plan --account " + a.ClientID
			if a.PlanEnabled() {
				status = "Using ChatGPT plan"
			}
		}
		if _, err = fmt.Fprintf(out, "Account: %s%s\nEmail: %s\nStatus: %s\n", a.ClientID, selected, a.Email, status); err != nil {
			return err
		}
		if a.AccessToken != "" {
			if _, err = fmt.Fprintf(out, "Expires: %s\n", a.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")); err != nil {
				return err
			}
		}
	}
	_, err = fmt.Fprintln(out, "Manage usage:", chatgptauth.UsageURL)
	return err
}
func runChatGPTLogout(ctx context.Context, out io.Writer) error {
	s, err := chatgptauth.DefaultStore()
	if err != nil {
		return err
	}
	err = logoutChatGPT(ctx, s, authAccount)
	if errors.Is(err, chatgptauth.ErrNotFound) {
		_, err = fmt.Fprintln(out, "No local ChatGPT credentials were found.")
		return err
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "ChatGPT session revoked and local tokens cleared. Account registration and host ID retained.")
	return err
}
func init() {
	authCmd.PersistentFlags().StringVar(&authProvider, "provider", "chatgpt", "authentication provider: chatgpt")
	authCmd.PersistentFlags().StringVar(&authAccount, "account", "", "ChatGPT registration client ID (default: active account)")
	authLoginCmd.Flags().BoolVar(&authEnablePlan, "enable-plan", false, "request consent for plan usage on a retained ChatGPT registration")
	authLoginCmd.Flags().BoolVar(&authNewAccount, "new-account", false, "register a separate ChatGPT account or workspace")
	authCmd.AddCommand(&cobra.Command{Use: "select <client-id>", Short: "Select a saved ChatGPT registration", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateAuthProvider(); err != nil {
			return err
		}
		s, err := chatgptauth.DefaultStore()
		if err != nil {
			return err
		}
		return s.Select(cmd.Context(), args[0])
	}})
	llmCmd.AddCommand(&cobra.Command{Use: "models", Short: "List current account-specific ChatGPT models", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		models, err := llm.ChatGPTModels(cmd.Context())
		if err != nil {
			return err
		}
		for _, m := range models {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", m.Slug, m.DisplayName); err != nil {
				return err
			}
		}
		return nil
	}})
}

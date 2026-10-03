// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/alibaba/open-code-review/internal/chatgpt"
	"github.com/alibaba/open-code-review/internal/viewer"
	"github.com/spf13/cobra"
)

type chatGPTClient interface {
	Login(context.Context, string, func(string) error) (chatgpt.Profile, error)
	Logout(context.Context) error
	Profiles() ([]chatgpt.Profile, error)
	Models(context.Context) ([]chatgpt.Model, error)
	Select(context.Context, string) error
}

var newChatGPTClient = func() (chatGPTClient, error) { return chatgpt.DefaultClient() }
var openChatGPTBrowser = viewer.OpenBrowser

func init() {
	var accountID, model string
	var newAccount bool
	args := func(cmd *cobra.Command, args []string) error {
		if err := exactArgs(1)(cmd, args); err != nil {
			return err
		}
		if args[0] != chatgpt.ProviderName {
			return fmt.Errorf("this command supports only %s", chatgpt.ProviderName)
		}
		return nil
	}
	login := &cobra.Command{Use: "login openai-chatgpt", Short: "Continue with ChatGPT and configure subscription inference", Args: args,
		Long: `Connect an eligible ChatGPT plan through browser sign-in and grant permission for code review inference.
Credentials are stored separately from provider configuration and renewed automatically.
The subscription route uses OpenAI's output limits; max_tokens and temperature are not sent.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChatGPTLogin(cmd.Context(), accountID, newAccount, model)
		}}
	login.Flags().StringVar(&accountID, "account", "", "Reuse a saved client ID shown by 'ocr llm status openai-chatgpt'")
	login.Flags().BoolVar(&newAccount, "new-account", false, "Register another ChatGPT account or workspace")
	login.Flags().StringVar(&model, "model", "", "Select an available model (defaults to the configured model if still offered, else the first listed)")
	login.MarkFlagsMutuallyExclusive("account", "new-account")
	logout := &cobra.Command{Use: "logout openai-chatgpt", Short: "Revoke and clear the active ChatGPT session", Args: args,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newChatGPTClient()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			if err := c.Logout(ctx); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Signed out of ChatGPT.")
			return nil
		}}
	status := &cobra.Command{Use: "status openai-chatgpt", Short: "Show saved ChatGPT registrations without exposing credentials", Args: args,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newChatGPTClient()
			if err != nil {
				return err
			}
			profiles, err := c.Profiles()
			if err != nil {
				return err
			}
			if len(profiles) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "ChatGPT is not connected. Run 'ocr llm login openai-chatgpt'.")
			}
			for _, p := range profiles {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  active=%t connected=%t plan_usage=%t\n", p.ClientID, p.Email, p.Active, p.Connected, p.PlanEnabled)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Manage usage: %s\n", chatgpt.UsageURL)
			return nil
		}}
	models := &cobra.Command{Use: "models openai-chatgpt", Short: "List models available to the active ChatGPT account", Args: args,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newChatGPTClient()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute)
			defer cancel()
			models, err := c.Models(ctx)
			if err != nil {
				return err
			}
			for _, m := range models {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", m.Slug, m.DisplayName)
			}
			return nil
		}}
	selectAccount := &cobra.Command{Use: "account <client-id>", Short: "Select a saved ChatGPT registration", Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newChatGPTClient()
			if err != nil {
				return err
			}
			if err := c.Select(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "ChatGPT account selected. Run 'ocr llm models openai-chatgpt' to check available models.")
			return nil
		}}
	llmCmd.AddCommand(login, logout, status, models, selectAccount)
}

func runChatGPTLogin(ctx context.Context, accountID string, newAccount bool, model string) error {
	path, err := defaultConfigPath()
	if err != nil {
		return err
	}
	cfg, err := loadOrCreateConfig(path)
	if err != nil {
		return err
	}
	c, err := newChatGPTClient()
	if err != nil {
		return err
	}
	if accountID == "" && !newAccount {
		profiles, err := c.Profiles()
		if err != nil {
			return err
		}
		for _, p := range profiles {
			if p.Active {
				accountID = p.ClientID
				break
			}
		}
	}
	profile, err := c.Login(ctx, accountID, func(authURL string) error {
		fmt.Printf("Continue with ChatGPT:\n%s\n", authURL)
		if err := openChatGPTBrowser(authURL); err != nil {
			fmt.Println("Could not open a browser automatically. Open the sign-in URL above.")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !profile.PlanEnabled {
		return fmt.Errorf("signed in, but ChatGPT plan usage was not authorized; run 'ocr llm login openai-chatgpt --account %s' to grant it", profile.ClientID)
	}
	models, err := c.Models(ctx)
	if err != nil {
		return fmt.Errorf("ChatGPT connected, but model discovery failed; retry 'ocr llm models openai-chatgpt': %w", err)
	}
	entry := cfg.Providers[chatgpt.ProviderName]
	model, err = chatGPTModel(models, model, activeModelForProvider(cfg, chatgpt.ProviderName, entry))
	if err != nil {
		return err
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderEntry{}
	}
	entry.Model = model
	entry.Models = nil
	for _, m := range models {
		entry.Models = append(entry.Models, m.Slug)
	}
	cfg.Providers[chatgpt.ProviderName] = entry
	cfg.Provider = chatgpt.ProviderName
	cfg.Model = model
	if err := saveConfig(path, cfg); err != nil {
		return err
	}
	fmt.Printf("ChatGPT connected: %s\nProvider: %s\nModel: %s\nRun 'ocr llm test' to verify inference and tool calls.\n", profile.Email, chatgpt.ProviderName, model)
	return nil
}

// chatGPTModel validates an explicit choice. Without one, a re-login keeps the
// configured model while the account still offers it, rather than resetting
// the user's pick to the first listed model.
func chatGPTModel(models []chatgpt.Model, chosen, current string) (string, error) {
	if len(models) == 0 {
		return "", fmt.Errorf("no models are available for this ChatGPT account")
	}
	if chosen == "" {
		for _, m := range models {
			if current != "" && m.Slug == current {
				return current, nil
			}
		}
		return models[0].Slug, nil
	}
	for _, m := range models {
		if m.Slug == chosen {
			return chosen, nil
		}
	}
	return "", fmt.Errorf("model %q is not available to this ChatGPT account", chosen)
}

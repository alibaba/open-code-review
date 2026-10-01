// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/chatgptauth"
	"github.com/spf13/cobra"
)

func TestAuthDefaultsToChatGPT(t *testing.T) {
	flag := authCmd.PersistentFlags().Lookup("provider")
	if flag == nil || flag.DefValue != "chatgpt" {
		t.Fatalf("provider flag = %v, want default chatgpt", flag)
	}
	commandChatGPTAccount(t)
	oldProvider, oldLogin, oldLogout := authProvider, loginChatGPT, logoutChatGPT
	t.Cleanup(func() { authProvider, loginChatGPT, logoutChatGPT = oldProvider, oldLogin, oldLogout })
	authProvider = flag.DefValue
	loginCalled, logoutCalled := false, false
	loginChatGPT = func(context.Context, chatgptauth.Store, string, bool, bool, bool, io.Writer) (*chatgptauth.Auth, error) {
		loginCalled = true
		return &chatgptauth.Auth{Scopes: strings.Fields(chatgptauth.Scope)}, nil
	}
	logoutChatGPT = func(context.Context, chatgptauth.Store, string) error {
		logoutCalled = true
		return nil
	}
	var out bytes.Buffer
	for _, cmd := range []*cobra.Command{authLoginCmd, authStatusCmd, authLogoutCmd} {
		oldOut, oldContext := cmd.OutOrStdout(), cmd.Context()
		cmd.SetOut(&out)
		cmd.SetContext(context.Background())
		err := cmd.RunE(cmd, nil)
		cmd.SetOut(oldOut)
		cmd.SetContext(oldContext)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !loginCalled || !logoutCalled || !strings.Contains(out.String(), "Using ChatGPT plan") || !strings.Contains(out.String(), "local tokens cleared") {
		t.Fatalf("login=%t logout=%t output=%q", loginCalled, logoutCalled, out.String())
	}
}

func TestAuthRejectsUnsupportedProviderWithoutAccountFlags(t *testing.T) {
	old := authProvider
	t.Cleanup(func() { authProvider = old })
	authProvider = "codex"
	if err := validateAuthProvider(); err == nil || !strings.Contains(err.Error(), "expected chatgpt") {
		t.Fatalf("validateAuthProvider = %v", err)
	}
}

func TestAuthLoginFlagsAreMutuallyExclusive(t *testing.T) {
	rootCmd.SetArgs([]string{"auth", "login", "--device", "--no-browser"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		for _, name := range []string{"device", "no-browser"} {
			flag := authLoginCmd.Flags().Lookup(name)
			_ = flag.Value.Set("false")
			flag.Changed = false
		}
	})
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "group [device no-browser]") {
		t.Errorf("Execute error = %v", err)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/chatgptauth"
	"github.com/spf13/cobra"
)

func commandChatGPTAccount(t *testing.T) chatgptauth.Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	s, err := chatgptauth.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Host(context.Background()); err != nil {
		t.Fatal(err)
	}
	d := chatgptauth.Database{Active: "oaiapp_one", Accounts: []chatgptauth.Auth{{ClientID: "oaiapp_one", Issuer: chatgptauth.Issuer, Subject: "subject", Email: "example@example.invalid", AccessToken: "fixture-secret", RefreshToken: "refresh-fixture", IDToken: "id-fixture", Scopes: strings.Fields(chatgptauth.Scope), ExpiresAt: time.Now().Add(time.Hour)}, {ClientID: "oaiapp_two", Issuer: chatgptauth.Issuer, Subject: "subject-two", AccessToken: "fixture-two"}, {ClientID: "oaiapp_three", Issuer: chatgptauth.Issuer, Subject: "subject-three"}}}
	b, _ := json.Marshal(d)
	if err := os.WriteFile(filepath.Join(s.Dir, "accounts.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestChatGPTCommandLifecycle(t *testing.T) {
	s := commandChatGPTAccount(t)
	for _, cmd := range []*cobra.Command{authLoginCmd, authStatusCmd, authLogoutCmd} {
		oldContext := cmd.Context()
		cmd.SetContext(context.Background())
		t.Cleanup(func() { cmd.SetContext(oldContext) })
	}
	oldProvider, oldAccount, oldNew, oldDevice, oldNoBrowser := authProvider, authAccount, authNewAccount, authLoginDevice, authLoginNoBrowser
	oldLogin, oldLogout := loginChatGPT, logoutChatGPT
	defer func() {
		authProvider, authAccount, authNewAccount, authLoginDevice, authLoginNoBrowser = oldProvider, oldAccount, oldNew, oldDevice, oldNoBrowser
		loginChatGPT, logoutChatGPT = oldLogin, oldLogout
	}()
	authProvider = "chatgpt"
	authAccount = ""
	authNewAccount = false
	authLoginDevice = false
	authLoginNoBrowser = false
	if err := validateAuthProvider(); err != nil {
		t.Fatal(err)
	}
	authProvider = "codex"
	authAccount = "oaiapp_one"
	if err := validateAuthProvider(); err == nil {
		t.Fatal("codex selected ChatGPT account")
	}
	authAccount = ""
	authNewAccount = true
	if err := validateAuthProvider(); err == nil {
		t.Fatal("codex registered ChatGPT account")
	}
	authProvider = "chatgpt"
	authNewAccount = false
	var out bytes.Buffer
	if err := authStatusCmd.RunE(authStatusCmd, nil); err != nil {
		t.Fatal(err)
	}
	if err := runChatGPTStatus(&out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "fixture-secret") || !strings.Contains(out.String(), "(active)") || !strings.Contains(out.String(), "signed out") || !strings.Contains(out.String(), "disabled") {
		t.Fatal(out.String())
	}
	loginChatGPT = func(context.Context, chatgptauth.Store, string, bool, bool, bool, io.Writer) (*chatgptauth.Auth, error) {
		return &chatgptauth.Auth{Scopes: strings.Fields(chatgptauth.Scope)}, nil
	}
	authLoginCmd.SetOut(&out)
	defer authLoginCmd.SetOut(nil)
	if err := authLoginCmd.RunE(authLoginCmd, nil); err != nil {
		t.Fatal(err)
	}
	loginChatGPT = func(context.Context, chatgptauth.Store, string, bool, bool, bool, io.Writer) (*chatgptauth.Auth, error) {
		return &chatgptauth.Auth{}, nil
	}
	if err := runChatGPTLogin(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	loginChatGPT = func(context.Context, chatgptauth.Store, string, bool, bool, bool, io.Writer) (*chatgptauth.Auth, error) {
		return nil, errors.New("fixture failure")
	}
	if err := runChatGPTLogin(context.Background(), &out); err == nil {
		t.Fatal("login error swallowed")
	}
	logoutChatGPT = func(ctx context.Context, store chatgptauth.Store, account string) error {
		return store.Clear(ctx, account)
	}
	authLogoutCmd.SetOut(&out)
	defer authLogoutCmd.SetOut(nil)
	if err := authLogoutCmd.RunE(authLogoutCmd, nil); err != nil {
		t.Fatal(err)
	}
	a, err := s.Selected("")
	if err != nil || a.AccessToken != "" {
		t.Fatal(a, err)
	}
	logoutChatGPT = func(context.Context, chatgptauth.Store, string) error { return errors.New("revocation not confirmed") }
	if err := runChatGPTLogout(context.Background(), &out); err == nil {
		t.Fatal("logout error swallowed")
	}
	var selectCmdFound bool
	for _, cmd := range authCmd.Commands() {
		if cmd.Name() == "select" {
			selectCmdFound = true
			cmd.SetContext(context.Background())
			if err := cmd.RunE(cmd, []string{"oaiapp_two"}); err != nil {
				t.Fatal(err)
			}
			authProvider = "codex"
			if err := cmd.RunE(cmd, []string{"oaiapp_one"}); err == nil {
				t.Fatal("codex select")
			}
			authProvider = "invalid"
			if err := cmd.RunE(cmd, []string{"oaiapp_one"}); err == nil {
				t.Fatal("invalid select")
			}
		}
	}
	if !selectCmdFound {
		t.Fatal("missing select")
	}
	authProvider = "invalid"
	if err := authLoginCmd.RunE(authLoginCmd, nil); err == nil {
		t.Fatal("invalid provider login")
	}
	if err := authStatusCmd.RunE(authStatusCmd, nil); err == nil {
		t.Fatal("invalid provider status")
	}
	if err := authLogoutCmd.RunE(authLogoutCmd, nil); err == nil {
		t.Fatal("invalid provider logout")
	}
}
func TestChatGPTStatusExpiryAfterSignOut(t *testing.T) {
	s := commandChatGPTAccount(t)
	d, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runChatGPTStatus(&out); err != nil {
		t.Fatal(err)
	}
	expiry := "Expires: " + d.Accounts[0].ExpiresAt.Format(time.RFC3339)
	if !strings.Contains(out.String(), expiry) {
		t.Fatal("signed-in expiry omitted", out.String())
	}
	for _, a := range d.Accounts {
		if err := s.Clear(context.Background(), a.ClientID); err != nil {
			t.Fatal(err)
		}
	}
	out.Reset()
	if err := runChatGPTStatus(&out); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "Status: signed out") != len(d.Accounts) || strings.Contains(out.String(), "Expires:") || strings.Contains(out.String(), "0001-") {
		t.Fatal("signed-out expiry displayed", out.String())
	}
}

func TestChatGPTCommandStoreErrors(t *testing.T) {
	t.Setenv("HOME", "")
	var out bytes.Buffer
	if err := runChatGPTStatus(&out); err == nil {
		t.Fatal("missing HOME")
	}
	if err := runChatGPTLogout(context.Background(), &out); err == nil {
		t.Fatal("missing HOME")
	}
	old := authLoginDevice
	authLoginDevice = false
	defer func() { authLoginDevice = old }()
	if err := runChatGPTLogin(context.Background(), &out); err == nil {
		t.Fatal("missing HOME")
	}
}

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
)

func TestChatGPTEnablePlanCLI(t *testing.T) {
	commandChatGPTAccount(t)
	oldProvider, oldAccount, oldNew, oldEnable, oldDevice, oldBrowser := authProvider, authAccount, authNewAccount, authEnablePlan, authLoginDevice, authLoginNoBrowser
	oldLogin := loginChatGPT
	defer func() {
		authProvider, authAccount, authNewAccount, authEnablePlan, authLoginDevice, authLoginNoBrowser = oldProvider, oldAccount, oldNew, oldEnable, oldDevice, oldBrowser
		loginChatGPT = oldLogin
	}()
	flag := authLoginCmd.Flags().Lookup("enable-plan")
	if flag == nil {
		t.Fatal("missing consent flag")
	}
	if err := flag.Value.Set("true"); err != nil {
		t.Fatal(err)
	}
	authProvider = "codex"
	authAccount = ""
	authNewAccount = false
	if err := validateAuthProvider(); err == nil {
		t.Fatal("codex accepted consent")
	}
	authProvider = "chatgpt"
	authAccount = "oaiapp_one"
	authLoginDevice = false
	authLoginNoBrowser = true
	called := false
	loginChatGPT = func(_ context.Context, _ chatgptauth.Store, account string, newAccount, noBrowser, consent bool, _ io.Writer) (*chatgptauth.Auth, error) {
		called = true
		if account != "oaiapp_one" || newAccount || !noBrowser || !consent {
			t.Fatal("consent flags not passed")
		}
		return &chatgptauth.Auth{}, nil
	}
	var out bytes.Buffer
	if err := runChatGPTLogin(context.Background(), &out); err != nil || !called {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ocr auth login --provider chatgpt --enable-plan") {
		t.Fatal("disabled plan advice", out.String())
	}
	out.Reset()
	if err := runChatGPTStatus(&out); err != nil || !strings.Contains(out.String(), "--enable-plan --account oaiapp_two") {
		t.Fatal("status advice", out.String(), err)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/chatgptauth"
)

func TestChatGPTAuthCLIValidation(t *testing.T) {
	old := authProvider
	oldDevice := authLoginDevice
	defer func() { authProvider = old; authLoginDevice = oldDevice }()
	authProvider = "bad"
	if err := validateAuthProvider(); err == nil {
		t.Fatal("unknown provider")
	}
	authProvider = "chatgpt"
	authLoginDevice = true
	if err := runChatGPTLogin(context.Background(), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "device") {
		t.Fatal(err)
	}
}
func TestChatGPTAuthStatusEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	if err := runChatGPTStatus(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Not signed in") {
		t.Fatal(out.String())
	}
}
func TestChatGPTAuthLogoutEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, err := chatgptauth.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Host(context.Background()); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runChatGPTLogout(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No local") {
		t.Fatal(out.String())
	}
}

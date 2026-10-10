// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAuthCommandValidation(t *testing.T) {
	for _, args := range [][]string{{"login"}, {"login", "openai"}, {"status", "codex-oauth", "--headless"}, {"logout", "codex-oauth", "extra"}} {
		cmd := newAuthCmd()
		cmd.SetArgs(args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		if cmd.Execute() == nil {
			t.Fatalf("invalid arguments accepted: %v", args)
		}
	}
	cmd := newAuthCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"login", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "--headless") {
		t.Fatal("login help omits headless authentication")
	}
}

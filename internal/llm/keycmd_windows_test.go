//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestNewKeyCmd_CmdLine locks in the two decisions in newKeyCmd that no runtime
// test can observe: the command reaches cmd.exe through SysProcAttr.CmdLine
// verbatim (not through Args, whose syscall.EscapeArg quoting mangles embedded
// double quotes), and Args keeps its one-element default so Cmd.String() cannot
// panic on Args[1:].
func TestNewKeyCmd_CmdLine(t *testing.T) {
	c := newKeyCmd(context.Background(), `op read "op://Private/My Vault/api-key"`)

	want := `cmd.exe /S /C "op read "op://Private/My Vault/api-key""`
	if c.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil; the command line would be built from Args instead")
	}
	if got := c.SysProcAttr.CmdLine; got != want {
		t.Errorf("CmdLine = %q, want %q", got, want)
	}
	if len(c.Args) == 0 {
		t.Error("Args is empty; Cmd.String() indexes Args[1:] and panics on a nil slice")
	}
	// Panics if Args were nilled out.
	if s := c.String(); s == "" {
		t.Error("Cmd.String() returned empty")
	}
}

func TestResolveKeyCmd(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		want    string
		wantErr string // substring the error must contain; "" means success
	}{
		{name: "success", cmd: "echo sk-test", want: "sk-test"},
		// ECHO eats exactly one delimiter after the command token, so stdout here is
		// "   sk-test  \r\n" -- the trim is what produces the credential.
		{name: "surrounding whitespace trimmed", cmd: "echo    sk-test  ", want: "sk-test"},
		// The case the CmdLine detour exists for: quotes and spaces must arrive at
		// cmd.exe exactly as written. Routed through Args instead, EscapeArg would
		// wrap and backslash-escape them and the output would carry the backslashes.
		{name: "embedded quotes survive verbatim", cmd: `echo sk-"a b"-token`, want: `sk-"a b"-token`},
		{name: "non-zero exit", cmd: "exit 3", wantErr: "failed: exit status 3"},
		{name: "no output", cmd: "rem", wantErr: "produced empty output"},
		{name: "blank line only", cmd: "echo.", wantErr: "produced empty output"},
		// & is cmd.exe's command separator, so both echoes run and produce two lines.
		{name: "multi-line output", cmd: "echo a& echo b", wantErr: "produced multi-line output"},
		// The two rows below pin down what the outer quote pair we add does and does
		// not protect, because "the command line could split" reads like a hole until
		// you know which shapes actually split. /S makes cmd.exe strip the first
		// character and the last quote and run the remainder unchanged, so a bare
		// interior quote leaves the following & inside a quoted region: it stays one
		// command and echo prints the & literally.
		{name: "interior quote keeps & quoted", cmd: `echo A" & echo B`, want: `A" & echo B`},
		// A doubled quote closes that region, so this & is a real separator and both
		// echoes run. It is not a privilege boundary -- api_key_cmd is already a
		// command line its author asked us to run -- but it is the one shape where the
		// line splits, and the single-line guard is what stops the extra output from
		// being mistaken for the credential.
		{name: "doubled quote lets & split the line", cmd: `echo A"" & echo B`, wantErr: "produced multi-line output"},
		{name: "command not found", cmd: "this-cmd-does-not-exist-xyz", wantErr: "failed:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveKeyCmd(tt.cmd, `api_key_cmd for provider "x"`)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (output %q)", tt.wantErr, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveKeyCmd_Timeout(t *testing.T) {
	origTimeout, origDelay := keyCmdTimeout, keyCmdWaitDelay
	keyCmdTimeout = 50 * time.Millisecond
	keyCmdWaitDelay = 100 * time.Millisecond
	t.Cleanup(func() { keyCmdTimeout, keyCmdWaitDelay = origTimeout, origDelay })

	// ping, not timeout.exe: timeout.exe refuses to run when stdin is redirected,
	// and resolveKeyCmd hands the child the test binary's stdin. Its stderr is
	// redirected for the same reason the unix twin redirects it: the killed
	// command's orphan would otherwise hold the test binary's stderr, which
	// cmd/go reads to EOF, stalling the run past the point resolveKeyCmd returned.
	_, err := resolveKeyCmd("ping -n 6 127.0.0.1 2>nul", `api_key_cmd for provider "x"`)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("error %q does not mention timeout", err.Error())
	}
}

// TestResolveKeyCmd_StdinWired proves the child inherits our stdin: with Stdin
// left nil, os/exec hands the child NUL, findstr reads EOF immediately and
// prints nothing, so this would fail with "produced empty output" instead.
func TestResolveKeyCmd_StdinWired(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()

	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })

	if _, err := w.WriteString("passphrase-from-stdin\r\n"); err != nil {
		t.Fatalf("write to stdin pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close stdin pipe writer: %v", err)
	}

	// findstr "^" copies every stdin line to stdout; ^ is passed through verbatim
	// under /S rather than treated as cmd.exe's escape character.
	got, err := resolveKeyCmd(`findstr "^"`, `api_key_cmd for provider "x"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "passphrase-from-stdin" {
		t.Fatalf("got %q, want %q", got, "passphrase-from-stdin")
	}
}

func TestResolveKeyCmd_LabelInError(t *testing.T) {
	_, err := resolveKeyCmd("exit 1", `auth_token_cmd for llm config`)
	if err == nil || !strings.HasPrefix(err.Error(), "auth_token_cmd for llm config") {
		t.Fatalf("expected label prefix in error, got %v", err)
	}
}

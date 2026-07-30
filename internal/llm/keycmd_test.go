//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestResolveKeyCmd(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		want    string
		wantErr string // substring the error must contain; "" means success
	}{
		{name: "success", cmd: "printf 'sk-test\\n'", want: "sk-test"},
		{name: "trailing whitespace trimmed", cmd: "printf '  sk-test  \\n'", want: "sk-test"},
		{name: "no trailing newline", cmd: "printf 'sk-test'", want: "sk-test"},
		{name: "crlf line ending trimmed", cmd: "printf 'sk-crlf\\r\\n'", want: "sk-crlf"},
		{name: "non-zero exit", cmd: "exit 3", wantErr: "failed: exit status 3"},
		{name: "false", cmd: "false", wantErr: "failed:"},
		{name: "empty output", cmd: "true", wantErr: "produced empty output"},
		{name: "empty printf", cmd: "printf ''", wantErr: "produced empty output"},
		{name: "whitespace-only output", cmd: "printf '  \\n'", wantErr: "produced empty output"},
		{name: "multi-line output", cmd: "printf 'a\\nb\\n'", wantErr: "produced multi-line output"},
		// A lone interior CR is a line break too, and one that survives both
		// TrimRight("\r\n") and TrimSpace. Refuse it here rather than let it reach
		// net/http, which rejects the Authorization header with an opaque error.
		{name: "interior carriage return", cmd: "printf 'a\\rb'", wantErr: "produced multi-line output"},
		{name: "multi-line error names the fix", cmd: "printf 'a\\nb\\n'", wantErr: "pipe through 'head -n1'"},
		// Every other control byte net/http rejects (httpguts.ValidHeaderFieldValue:
		// anything < 0x20 except TAB, plus DEL) must be named here rather than reach
		// the request as an opaque "invalid header field value" failure.
		{name: "nul byte", cmd: "printf 'sk-a\\0b'", wantErr: "control byte 0x00 at offset 4"},
		{name: "vertical tab", cmd: "printf 'sk-a\\013b'", wantErr: "control byte 0x0B at offset 4"},
		{name: "form feed", cmd: "printf 'sk-a\\014b'", wantErr: "control byte 0x0C at offset 4"},
		{name: "delete byte", cmd: "printf 'sk-a\\177b'", wantErr: "control byte 0x7F at offset 4"},
		// TAB is legal in a header value, so it survives (interior only; TrimSpace
		// takes the edges).
		{name: "interior tab kept", cmd: "printf 'sk-a\\tb\\n'", want: "sk-a\tb"},
		{name: "command not found", cmd: "this-cmd-does-not-exist-xyz", wantErr: "failed:"},
		// Boundary: exactly the cap is fine, one byte more is refused. The child
		// dies of SIGPIPE as soon as we stop accepting, so this stays fast.
		{name: "output exactly at cap", cmd: "head -c 65536 /dev/zero | tr '\\0' a", want: strings.Repeat("a", keyCmdMaxOutput)},
		{name: "output over cap", cmd: "yes aaaaaaaaaa | head -c 200000 | tr -d '\\n'", wantErr: "produced more than 64KiB of output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveKeyCmd(tt.cmd, "api_key_cmd for provider \"x\"")
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
	// `sleep 5` inherits the stdout pipe and outlives the SIGKILL'd shell, so
	// without a shrunk WaitDelay this test waits the full default 5s.
	keyCmdWaitDelay = 100 * time.Millisecond
	t.Cleanup(func() { keyCmdTimeout, keyCmdWaitDelay = origTimeout, origDelay })

	_, err := resolveKeyCmd("sleep 5 2>/dev/null", "api_key_cmd for provider \"x\"")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("error %q does not mention timeout", err.Error())
	}
}

// A grandchild that inherited the stdout pipe keeps it open after the shell
// exits, which used to block Wait until the grandchild died. WaitDelay bounds
// that: this must finish in well under the 30s sleep.
func TestResolveKeyCmd_WaitDelayBoundsOrphanHoldingPipe(t *testing.T) {
	origTimeout, origDelay := keyCmdTimeout, keyCmdWaitDelay
	keyCmdTimeout = 50 * time.Millisecond
	keyCmdWaitDelay = 100 * time.Millisecond
	t.Cleanup(func() { keyCmdTimeout, keyCmdWaitDelay = origTimeout, origDelay })

	// The grandchild must keep the inherited *stdout* pipe open (that is the case
	// under test) but not our stderr: it outlives the test, and `go test` reads
	// the test binary's stderr until EOF, so leaving it attached would stall the
	// run for the full sleep even though resolveKeyCmd returned immediately.
	start := time.Now()
	_, err := resolveKeyCmd("sleep 30 2>/dev/null & printf tok", `api_key_cmd for provider "x"`)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Fatalf("took %s; WaitDelay did not bound the orphaned grandchild", elapsed)
	}
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("error %q does not mention timeout", err.Error())
	}
}

// TestResolveKeyCmd_StdinWired proves the child inherits our stdin: with Stdin
// left nil, os/exec hands the child /dev/null, `read` sees EOF and prints
// nothing, so this would fail with "produced empty output" instead.
//
// os.Stdin under `go test` is not a usable prompt source, so swap in a pipe.
// Mutating the global is safe here: this test is not parallel, and the only
// parallel tests in the package are subtests of TestResolveKeyCmd, which
// finishes before any later top-level test starts.
func TestResolveKeyCmd_StdinWired(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()

	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })

	// Written and closed up front (well under the pipe buffer, so no blocking)
	// so the child reads a full line and then EOF.
	if _, err := w.WriteString("passphrase-from-stdin\n"); err != nil {
		t.Fatalf("write to stdin pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close stdin pipe writer: %v", err)
	}

	got, err := resolveKeyCmd(`read -r x; printf %s "$x"`, `api_key_cmd for provider "x"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "passphrase-from-stdin" {
		t.Fatalf("got %q, want %q", got, "passphrase-from-stdin")
	}
}

func TestResolveKeyCmd_LabelInError(t *testing.T) {
	_, err := resolveKeyCmd("false", `auth_token_cmd for llm config`)
	if err == nil || !strings.HasPrefix(err.Error(), "auth_token_cmd for llm config") {
		t.Fatalf("expected label prefix in error, got %v", err)
	}
}

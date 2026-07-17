//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
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
		{name: "non-zero exit", cmd: "exit 3", wantErr: "failed: exit status 3"},
		{name: "false", cmd: "false", wantErr: "failed:"},
		{name: "empty output", cmd: "true", wantErr: "produced empty output"},
		{name: "empty printf", cmd: "printf ''", wantErr: "produced empty output"},
		{name: "whitespace-only output", cmd: "printf '  \\n'", wantErr: "produced empty output"},
		{name: "multi-line output", cmd: "printf 'a\\nb\\n'", wantErr: "produced multi-line output"},
		{name: "command not found", cmd: "this-cmd-does-not-exist-xyz", wantErr: "failed:"},
	}
	for _, tt := range tests {
		tt := tt
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
	orig := keyCmdTimeout
	keyCmdTimeout = 50 * time.Millisecond
	t.Cleanup(func() { keyCmdTimeout = orig })

	_, err := resolveKeyCmd("sleep 5", "api_key_cmd for provider \"x\"")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("error %q does not mention timeout", err.Error())
	}
}

func TestResolveKeyCmd_LabelInError(t *testing.T) {
	_, err := resolveKeyCmd("false", `auth_token_cmd for llm config`)
	if err == nil || !strings.HasPrefix(err.Error(), "auth_token_cmd for llm config") {
		t.Fatalf("expected label prefix in error, got %v", err)
	}
}

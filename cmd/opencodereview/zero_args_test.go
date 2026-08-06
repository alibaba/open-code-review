// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestZeroArgumentCommands_RejectUnexpectedArgs(t *testing.T) {
	tests := []struct {
		name    string
		command *cobra.Command
		path    string
	}{
		{name: "version", command: versionCmd, path: "ocr version"},
		{name: "llm test", command: llmTestCmd, path: "ocr llm test"},
		{name: "llm providers", command: llmProvidersCmd, path: "ocr llm providers"},
		{name: "session list", command: sessionListCmd, path: "ocr session list"},
		{name: "delegate preview", command: delegatePreviewCmd, path: "ocr delegate preview"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.command.Args == nil {
				t.Fatalf("%s does not define positional argument validation", tt.path)
			}

			err := tt.command.Args(tt.command, []string{"unexpected"})
			if err == nil {
				t.Fatalf("expected an error for unexpected positional argument")
			}

			want := `unknown command "unexpected" for "` + tt.path + `"`
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %q, want it to contain %q", err, want)
			}
		})
	}
}

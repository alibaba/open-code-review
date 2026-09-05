// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestCLIBinaryName(t *testing.T) {
	tests := []struct {
		protocol string
		want     string
	}{
		{llm.ProtocolClaudeCLI, "claude"},
		{llm.ProtocolCodexCLI, "codex"},
		{"future-cli", "future-cli"}, // unknown falls back to the protocol name
	}
	for _, tc := range tests {
		if got := cliBinaryName(tc.protocol); got != tc.want {
			t.Errorf("cliBinaryName(%q) = %q, want %q", tc.protocol, got, tc.want)
		}
	}
}

// TestRunLLMProvidersShowsLocalCLIForCLIProviders: the CLI presets have no base
// URL, so their BASE URL column must read "(local CLI)" rather than blank.
func TestRunLLMProvidersShowsLocalCLIForCLIProviders(t *testing.T) {
	out := captureStdout(t, runLLMProviders)

	for _, name := range []string{"claude-code", "codex"} {
		var row string
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, name) {
				row = line
				break
			}
		}
		if row == "" {
			t.Fatalf("provider %q missing from `ocr llm providers` output:\n%s", name, out)
		}
		if !strings.Contains(row, "(local CLI)") {
			t.Errorf("provider %q row = %q, want it to show (local CLI) in the BASE URL column", name, row)
		}
	}
}

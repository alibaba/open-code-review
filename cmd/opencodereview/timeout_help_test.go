// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestTaskTimeoutHelp(t *testing.T) {
	for _, name := range []string{"review", "scan"} {
		t.Run(name, func(t *testing.T) {
			cmd := &cobra.Command{Use: name}
			if name == "review" {
				registerReviewFlags(cmd, &reviewOptions{})
			} else {
				registerScanFlags(cmd, &scanOptions{})
			}
			flag := cmd.Flags().Lookup("timeout")
			if flag.DefValue != "15" {
				t.Fatalf("default = %s, want 15", flag.DefValue)
			}
			for _, text := range []string{"minutes", "retries", "0 = unlimited", "independent", "OCR_LLM_TIMEOUT", "timeout_sec", "seconds", "300"} {
				if !strings.Contains(flag.Usage, text) {
					t.Errorf("help %q does not contain %q", flag.Usage, text)
				}
			}
		})
	}
}

func TestReviewHelpMentionsFlagConstraints(t *testing.T) {
	cmd := &cobra.Command{Use: "review"}
	opts := &reviewOptions{}
	registerReviewFlags(cmd, opts)
	for _, flag := range []string{"from", "to", "commit"} {
		if !strings.Contains(cmd.Flags().Lookup(flag).Usage, "mutually exclusive") {
			t.Errorf("--%s help does not mention that review modes are mutually exclusive", flag)
		}
	}
	if !strings.Contains(cmd.Flags().Lookup("format").Usage, "sarif is not supported with --preview") {
		t.Error("--format help does not mention the preview SARIF restriction")
	}
	if !strings.Contains(cmd.Flags().Lookup("background-file").Usage, "relative paths resolve from the repository root") {
		t.Error("--background-file help does not explain relative path resolution")
	}
	if !strings.Contains(taskTimeoutHelp, "effort's review rounds") {
		t.Error("--timeout help does not explain review effort scaling")
	}
}

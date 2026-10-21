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

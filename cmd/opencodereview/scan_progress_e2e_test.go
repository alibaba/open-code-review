// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestScanProgressStructuredOutput(t *testing.T) {
	for _, tc := range []struct {
		format, audience string
		wantProgress     bool
	}{
		{"json", "human", true},
		{"sarif", "human", true},
		{"json", "agent", false},
	} {
		t.Run(tc.format+"/"+tc.audience, func(t *testing.T) {
			repo := initTestGitRepo(t)
			gitCommitFile(t, repo, "main.go", "package main\n", "add main")
			startFakeLLM(t, newFakeLLM())
			var output string
			var scanErr error
			errorsOutput := captureStderr(t, func() {
				output = captureStdout(t, func() {
					scanErr = executeScan(scanOptions{
						repoDir: repo, outputFormat: tc.format, audience: tc.audience,
						noPlan: true, noDedup: true, noSummary: true, concurrency: 1,
					})
				})
			})
			if scanErr != nil {
				t.Fatalf("scan: %v\nstderr: %s", scanErr, errorsOutput)
			}
			var document map[string]any
			if err := json.Unmarshal([]byte(output), &document); err != nil {
				t.Fatalf("stdout must remain a single structured document: %v\n%s", err, output)
			}
			if strings.Contains(output, "scan progress:") {
				t.Fatalf("progress leaked into stdout: %s", output)
			}
			progress := "[ocr] scan progress: 1/1 files processed"
			if got := strings.Contains(errorsOutput, progress); got != tc.wantProgress {
				t.Fatalf("stderr progress = %v, want %v: %s", got, tc.wantProgress, errorsOutput)
			}
		})
	}
}

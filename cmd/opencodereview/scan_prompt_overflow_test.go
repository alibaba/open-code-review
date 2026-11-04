// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestExecuteScanPromptOverflowCannotReportClean(t *testing.T) {
	freshOCRHome(t)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected LLM request", http.StatusBadRequest)
	}))
	defer server.Close()
	t.Setenv("OCR_LLM_URL", server.URL+"/v1/chat/completions")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "test-model")
	t.Setenv("OCR_LLM_PROTOCOL", "openai")
	t.Setenv("OCR_USE_ANTHROPIC", "false")

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			opts, err := parseScanFlags([]string{
				"--repo", repo, "--max-tokens", "100", "--no-plan", "--no-dedup",
				"--no-summary", "--format", format, "--audience", "agent",
			})
			if err != nil {
				t.Fatal(err)
			}
			var scanErr error
			output := captureStdout(t, func() { scanErr = executeScan(opts) })
			t.Logf("scan error: %v; output: %s", scanErr, output)
			if scanErr == nil || !strings.Contains(scanErr.Error(), "all 1 file scan(s) failed") {
				t.Errorf("scan error = %v, want complete scan failure", scanErr)
			}
			if strings.Contains(output, "Looks good to me") {
				t.Errorf("unreviewed file was reported as clean: %s", output)
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("LLM requests = %d, want zero because the prompt was rejected locally", got)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/mcp"
)

func TestReviewOutputCommittedBeforeMCPShutdown(t *testing.T) {
	repoDir := retryTestRepo(t)
	startFakeLLM(t, newFakeLLM())
	outputPath := filepath.Join(t.TempDir(), "review.json")

	originalClose := closeReviewMCPClients
	t.Cleanup(func() { closeReviewMCPClients = originalClose })
	shutdownObserved := false
	closeReviewMCPClients = func(clients []*mcp.Client) {
		shutdownObserved = true
		data, err := os.ReadFile(outputPath)
		if err != nil {
			t.Errorf("read output at MCP shutdown boundary: %v", err)
		} else {
			var report jsonOutput
			if err := json.Unmarshal(data, &report); err != nil {
				t.Errorf("output at MCP shutdown boundary is incomplete JSON: %v", err)
			}
		}
		originalClose(clients)
	}

	err := runReview([]string{
		"--repo", repoDir,
		"--from", "HEAD~1",
		"--to", "HEAD",
		"--format", "json",
		"--audience", "agent",
		"--output", outputPath,
	})
	if err != nil {
		t.Fatalf("review must succeed: %v", err)
	}
	if !shutdownObserved {
		t.Fatal("MCP shutdown boundary was not observed")
	}
}

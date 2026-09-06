// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"testing"
)

func TestEstimatePreflight(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "main.go", []byte("package main\n\nfunc main() {}\n"))
	gitCommit(t, repo, "init")

	est, err := EstimatePreflight(context.Background(), Args{
		RepoDir:          repo,
		MaxFileSizeBytes: 2 << 20,
	})
	if err != nil {
		t.Fatalf("EstimatePreflight: %v", err)
	}
	if est.Files != 1 {
		t.Fatalf("Files = %d, want 1", est.Files)
	}
	if est.TotalTokens <= 0 {
		t.Fatalf("TotalTokens = %d, want > 0", est.TotalTokens)
	}
	if est.TotalTokens != est.InputTokens+est.OutputTokens {
		t.Fatalf("TotalTokens = %d, input + output = %d", est.TotalTokens, est.InputTokens+est.OutputTokens)
	}
}

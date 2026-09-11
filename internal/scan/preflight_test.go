// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/session"
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

func TestEstimatePreflightExcludesOversizedFiles(t *testing.T) {
	repo := initTestRepo(t)
	content := "package main\n\n" + strings.Repeat("var value = 1234567890\n", 200)
	writeFile(t, repo, "main.go", []byte(content))
	gitCommit(t, repo, "init")

	est, err := EstimatePreflight(context.Background(), Args{
		RepoDir:          repo,
		MaxFileSizeBytes: 2 << 20,
		Template:         template.ScanTemplate{MaxTokens: 64},
	})
	if err != nil {
		t.Fatalf("EstimatePreflight: %v", err)
	}
	if est.Files != 0 || est.TotalTokens != 0 {
		t.Fatalf("oversized scan estimate = %+v, want no dispatchable work", est)
	}
}

func TestEstimatePreflightExcludesReusableResumeFiles(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "main.go", []byte("package main\n\nfunc main() {}\n"))
	gitCommit(t, repo, "init")

	provider := NewProvider(repo, nil, nil, 2<<20)
	items, err := provider.Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("enumerated items = %d, want 1", len(items))
	}
	fingerprint := scanItemFingerprint(items[0])
	resume := &session.ResumeState{
		Items: map[string]session.ResumeItem{
			fingerprint: {Fingerprint: fingerprint},
		},
	}

	est, err := EstimatePreflight(context.Background(), Args{
		RepoDir:          repo,
		MaxFileSizeBytes: 2 << 20,
		Resume:           resume,
	})
	if err != nil {
		t.Fatalf("EstimatePreflight: %v", err)
	}
	if est.Files != 0 || est.TotalTokens != 0 {
		t.Fatalf("resumed scan estimate = %+v, want no newly dispatchable work", est)
	}
}

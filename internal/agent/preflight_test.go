// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/session"
)

func initPreflightTestRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")
	runGit("config", "commit.gpgsign", "false")

	path := filepath.Join(repo, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc value() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "main.go")
	runGit("commit", "-m", "base")
	return repo, path
}

func TestEstimatePreflight(t *testing.T) {
	repo, path := initPreflightTestRepo(t)
	if err := os.WriteFile(path, []byte("package main\n\nfunc value() int { return 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	est, err := EstimatePreflight(context.Background(), Args{RepoDir: repo})
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

func TestEstimatePreflightExcludesOversizedDiffs(t *testing.T) {
	repo, path := initPreflightTestRepo(t)
	content := "package main\n\n" + strings.Repeat("var value = 1234567890\n", 200)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	est, err := EstimatePreflight(context.Background(), Args{
		RepoDir:  repo,
		Template: template.Template{MaxTokens: 64},
	})
	if err != nil {
		t.Fatalf("EstimatePreflight: %v", err)
	}
	if est.Files != 0 || est.TotalTokens != 0 {
		t.Fatalf("oversized diff estimate = %+v, want no dispatchable work", est)
	}
}

func TestEstimatePreflightExcludesReusableResumeDiffs(t *testing.T) {
	repo, path := initPreflightTestRepo(t)
	if err := os.WriteFile(path, []byte("package main\n\nfunc value() int { return 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	probe := &Agent{args: Args{RepoDir: repo}}
	if err := probe.loadDiffs(context.Background()); err != nil {
		t.Fatalf("loadDiffs: %v", err)
	}
	probe.diffs = probe.filterDiffs(probe.diffs)
	if len(probe.diffs) != 1 {
		t.Fatalf("filtered diffs = %d, want 1", len(probe.diffs))
	}
	fingerprint := reviewItemFingerprint(probe.reviewMode(), probe.diffs[0])
	resume := &session.ResumeState{
		Items: map[string]session.ResumeItem{
			fingerprint: {Fingerprint: fingerprint},
		},
		Manifest: &session.RunManifest{
			Coverage: session.Coverage{
				Completed: []session.CoverageItem{{Fingerprint: fingerprint}},
			},
		},
	}

	est, err := EstimatePreflight(context.Background(), Args{RepoDir: repo, Resume: resume})
	if err != nil {
		t.Fatalf("EstimatePreflight: %v", err)
	}
	if est.Files != 0 || est.TotalTokens != 0 {
		t.Fatalf("resumed estimate = %+v, want no newly dispatchable work", est)
	}
}

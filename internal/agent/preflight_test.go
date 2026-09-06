// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEstimatePreflight(t *testing.T) {
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

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestRangeDiffPreservesTrackedGitignoreFilteringWhileReturningNoisyDirs(t *testing.T) {
	repo := t.TempDir()
	runGitTest(t, repo, "init", "-q")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test User")
	runGitTest(t, repo, "config", "commit.gpgsign", "false")

	for _, dir := range []string{"src", "docs/generated", "vendor"} {
		if err := os.MkdirAll(filepath.Join(repo, dir), 0o755); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}

	files := map[string]string{
		"src/main.go":           "package src\n\nconst Value = 1\n",
		"docs/generated/api.go": "package generated\n\nconst Value = 1\n",
		"vendor/lib.go":         "package vendor\n\nconst Value = 1\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	runGitTest(t, repo, "add", "src/main.go", "docs/generated/api.go", "vendor/lib.go")
	runGitTest(t, repo, "commit", "-q", "-m", "initial tracked files")

	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("docs/generated/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	updates := map[string]string{
		"src/main.go":           "package src\n\nconst Value = 2\n",
		"docs/generated/api.go": "package generated\n\nconst Value = 2\n",
		"vendor/lib.go":         "package vendor\n\nconst Value = 2\n",
	}
	for name, content := range updates {
		if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatalf("rewrite %s: %v", name, err)
		}
	}
	runGitTest(t, repo, "add", "-u")
	runGitTest(t, repo, "add", ".gitignore")
	runGitTest(t, repo, "commit", "-q", "-m", "update tracked files")

	provider := NewProvider(repo, "HEAD~1", "HEAD", gitcmd.New(0))
	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff returned error: %v", err)
	}

	paths := make(map[string]bool, len(diffs))
	for _, d := range diffs {
		paths[d.NewPath] = true
	}
	if !paths["src/main.go"] {
		t.Fatalf("tracked diff paths = %v, want src/main.go", paths)
	}
	if !paths["vendor/lib.go"] {
		t.Fatalf("tracked diff paths = %v, want noisy-dir vendor/lib.go returned for agent accounting", paths)
	}
	if paths["docs/generated/api.go"] {
		t.Fatalf("tracked diff paths = %v, want repository-gitignored docs/generated/api.go filtered", paths)
	}
}

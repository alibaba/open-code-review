// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build !windows

package diff

// These tests exercise the quoted-path decoder through a real git subprocess.
// core.quotepath=false (always passed by the providers) strips non-ASCII but
// not a double quote or backslash, so the non-ASCII e2e in git_test.go never
// reaches the decoder. Such bytes are illegal in Windows filenames, hence the
// build tag.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

// cQuotedRelPath holds both escapes git emits under core.quotepath=false: a
// double quote (\") and a backslash (\\). Git renders the header as
// "src/back\\slash\"quote.ts".
const cQuotedRelPath = `src/back\slash"quote.ts`

// initRepoWithCQuotedChange creates a repository with one committed file whose
// path forces git's C-quoting, then modifies it in the working tree. It mirrors
// initRepoWithNonASCIIChange but chooses a name quoting cannot strip.
func initRepoWithCQuotedChange(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()

	runGitTest(t, repo, "init", "-q")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test User")
	runGitTest(t, repo, "config", "commit.gpgsign", "false")

	file := filepath.Join(repo, filepath.FromSlash(cQuotedRelPath))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("create C-quoted path: %v", err)
	}
	if err := os.WriteFile(file, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write C-quoted file: %v", err)
	}
	runGitTest(t, repo, "add", "--", cQuotedRelPath)
	runGitTest(t, repo, "commit", "-q", "-m", "initial commit")

	if err := os.WriteFile(file, []byte("after\n"), 0o644); err != nil {
		t.Fatalf("modify C-quoted file: %v", err)
	}
	return repo, cQuotedRelPath
}

func TestDiffModesPreserveCQuotedPaths(t *testing.T) {
	tests := []struct {
		name     string
		provider func(t *testing.T, repo string, runner *gitcmd.Runner) *Provider
	}{
		{
			name: "workspace",
			provider: func(_ *testing.T, repo string, runner *gitcmd.Runner) *Provider {
				return NewWorkspaceProvider(repo, runner)
			},
		},
		{
			name: "commit",
			provider: func(t *testing.T, repo string, runner *gitcmd.Runner) *Provider {
				runGitTest(t, repo, "add", "-A")
				runGitTest(t, repo, "commit", "-q", "-m", "update C-quoted file")
				return NewCommitProvider(repo, "HEAD", runner)
			},
		},
		{
			name: "range",
			provider: func(t *testing.T, repo string, runner *gitcmd.Runner) *Provider {
				runGitTest(t, repo, "add", "-A")
				runGitTest(t, repo, "commit", "-q", "-m", "update C-quoted file")
				return NewProvider(repo, "HEAD~1", "HEAD", runner)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, relPath := initRepoWithCQuotedChange(t)
			provider := tt.provider(t, repo, gitcmd.New(0))

			diffs, err := provider.GetDiff(context.Background())
			if err != nil {
				t.Fatalf("GetDiff returned error: %v", err)
			}
			if len(diffs) != 1 {
				t.Fatalf("got %d diffs, want 1: %+v", len(diffs), diffs)
			}
			if diffs[0].NewPath != relPath {
				t.Errorf("NewPath = %q, want %q", diffs[0].NewPath, relPath)
			}
			if diffs[0].NewFileContent != "after\n" {
				t.Errorf("NewFileContent = %q, want %q", diffs[0].NewFileContent, "after\n")
			}
		})
	}
}

func TestWorkspaceDiffPreservesCQuotedUntrackedPath(t *testing.T) {
	repo, trackedPath := initRepoWithCQuotedChange(t)
	runGitTest(t, repo, "checkout", "--", trackedPath)

	untrackedPath := `src/new"file\added.ts`
	if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(untrackedPath)), []byte("untracked\n"), 0o644); err != nil {
		t.Fatalf("write C-quoted untracked file: %v", err)
	}

	provider := NewWorkspaceProvider(repo, gitcmd.New(0))
	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff returned error: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("got %d diffs, want 1: %+v", len(diffs), diffs)
	}
	if diffs[0].NewPath != untrackedPath {
		t.Errorf("NewPath = %q, want %q", diffs[0].NewPath, untrackedPath)
	}
	if diffs[0].NewFileContent != "untracked\n" {
		t.Errorf("NewFileContent = %q, want %q", diffs[0].NewFileContent, "untracked\n")
	}
}

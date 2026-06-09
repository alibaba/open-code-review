package diff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/open-code-review/open-code-review/internal/gitcmd"
)

// runGitTest runs a git command in dir and fails the test on error.
func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// writeGarbageExternalDiff writes a shell script that emits non-diff output and
// returns its path. When git invokes it via GIT_EXTERNAL_DIFF / diff.external it
// replaces the normal unified-diff machinery, so the output can no longer be
// parsed into model.Diff structs unless the git command opts out with
// --no-ext-diff.
func writeGarbageExternalDiff(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "garbage-diff.sh")
	// GIT_EXTERNAL_DIFF programs receive 7 args; we ignore them and print junk.
	body := "#!/bin/sh\necho \"not a diff\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write garbage diff script: %v", err)
	}
	return script
}

// initRepoWithChange creates a real git repository with one committed file and
// an uncommitted working-tree modification, returning the repo dir. There is a
// genuine textual diff between HEAD and the working tree.
func initRepoWithChange(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()

	runGitTest(t, repo, "init", "-q")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test User")
	runGitTest(t, repo, "config", "commit.gpgsign", "false")

	file := filepath.Join(repo, "sample.txt")
	if err := os.WriteFile(file, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatalf("write sample.txt: %v", err)
	}
	runGitTest(t, repo, "add", "sample.txt")
	runGitTest(t, repo, "commit", "-q", "-m", "initial commit")

	// Working-tree modification: a real, parseable diff vs HEAD.
	if err := os.WriteFile(file, []byte("line1\nCHANGED\nline3\n"), 0o644); err != nil {
		t.Fatalf("modify sample.txt: %v", err)
	}
	return repo
}

// TestWorkspaceDiffSurvivesExternalDiffTool guards against issue #82: when a
// user has configured an external diff tool (GIT_EXTERNAL_DIFF or
// diff.external), git diff/show emit the tool's output instead of unified diff
// text, which the parser cannot read -> 0 diffs -> a silent "No files changed".
// Passing --no-ext-diff (and --no-textconv) to every git diff/show call site
// makes the provider immune to the user's diff configuration.
//
// RED (before fix): the workspace diff call sites omit --no-ext-diff, so the
// garbage script's output is returned and len(diffs) == 0 -> this test FAILS.
// GREEN (after fix): --no-ext-diff bypasses the env var, the unified diff is
// produced and parsed, len(diffs) > 0 -> this test PASSES.
func TestWorkspaceDiffSurvivesExternalDiffTool(t *testing.T) {
	repo := initRepoWithChange(t)
	garbage := writeGarbageExternalDiff(t)

	// Activate the user-hostile external diff tool for this test process. The
	// provider shells out to git, which inherits this environment.
	t.Setenv("GIT_EXTERNAL_DIFF", garbage)

	runner := gitcmd.New(0)
	provider := NewWorkspaceProvider(repo, runner)

	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff returned error: %v", err)
	}

	if len(diffs) == 0 {
		t.Fatalf("expected at least one parsed diff with an external diff tool "+
			"active, got 0 -- git diff call sites must pass --no-ext-diff "+
			"(issue #82). GIT_EXTERNAL_DIFF=%s", garbage)
	}
}

// TestRangeDiffSurvivesExternalDiffTool covers the ModeRange call site
// (git diff <base> <to>), which likewise must pass --no-ext-diff so that a
// user's external diff tool does not break range comparisons.
func TestRangeDiffSurvivesExternalDiffTool(t *testing.T) {
	repo := initRepoWithChange(t)

	// Commit the change so there is a committed delta between two refs.
	runGitTest(t, repo, "add", "sample.txt")
	runGitTest(t, repo, "commit", "-q", "-m", "second commit")

	garbage := writeGarbageExternalDiff(t)
	t.Setenv("GIT_EXTERNAL_DIFF", garbage)

	runner := gitcmd.New(0)
	// Range: HEAD~1..HEAD -> the second commit's change.
	provider := NewProvider(repo, "HEAD~1", "HEAD", runner)

	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff (range) returned error: %v", err)
	}

	if len(diffs) == 0 {
		t.Fatalf("expected at least one parsed range diff with an external diff "+
			"tool active, got 0 -- git diff range call site must pass "+
			"--no-ext-diff (issue #82). GIT_EXTERNAL_DIFF=%s", garbage)
	}
}

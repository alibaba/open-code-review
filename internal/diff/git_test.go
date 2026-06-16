package diff

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestCommitDiffSurvivesExternalDiffTool covers the ModeCommit call site
// (git show <commit>), which likewise must pass --no-ext-diff so that a
// user's external diff tool does not break single-commit analysis.
func TestCommitDiffSurvivesExternalDiffTool(t *testing.T) {
	repo := initRepoWithChange(t)

	runGitTest(t, repo, "add", "sample.txt")
	runGitTest(t, repo, "commit", "-q", "-m", "second commit")

	garbage := writeGarbageExternalDiff(t)
	t.Setenv("GIT_EXTERNAL_DIFF", garbage)

	runner := gitcmd.New(0)
	provider := NewCommitProvider(repo, "HEAD", runner)

	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff (commit) returned error: %v", err)
	}

	if len(diffs) == 0 {
		t.Fatalf("expected at least one parsed commit diff with an external diff "+
			"tool active, got 0 -- git show call site must pass "+
			"--no-ext-diff (issue #82). GIT_EXTERNAL_DIFF=%s", garbage)
	}
}

func TestCommitDiffTreatsOptionLikeRefAsRevision(t *testing.T) {
	repo := initRepoWithChange(t)
	pagerPath := filepath.Join(repo, "pwn.sh")
	proofPath := filepath.Join(repo, "PROOF")
	if err := os.WriteFile(pagerPath, []byte("#!/bin/sh\nprintf pwned > PROOF\n"), 0755); err != nil {
		t.Fatalf("write pager: %v", err)
	}

	runner := gitcmd.New(0)
	provider := NewCommitProvider(repo, "-O./pwn.sh", runner)

	_, err := provider.GetDiff(context.Background())
	if err == nil {
		t.Fatal("expected option-like commit ref to fail as an invalid revision")
	}
	if _, statErr := os.Stat(proofPath); statErr == nil {
		t.Fatal("option-like commit ref was interpreted as a git show option")
	} else if !os.IsNotExist(statErr) {
		t.Fatal(statErr)
	}
}

func TestWorkspaceUntrackedSymlinkDoesNotReadExternalTarget(t *testing.T) {
	repo := t.TempDir()
	runGitTest(t, repo, "init", "-q")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test User")
	runGitTest(t, repo, "config", "commit.gpgsign", "false")

	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base.txt: %v", err)
	}
	runGitTest(t, repo, "add", "base.txt")
	runGitTest(t, repo, "commit", "-q", "-m", "initial commit")

	secret := "TOP_SECRET_ISSUE123_SHOULD_NOT_LEAK\n"
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte(secret), 0o644); err != nil {
		t.Fatalf("write external secret: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "leaked_link")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	provider := NewWorkspaceProvider(repo, gitcmd.New(0))
	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff returned error: %v", err)
	}

	var found bool
	for _, d := range diffs {
		if strings.Contains(d.Diff, secret) || strings.Contains(d.NewFileContent, secret) {
			t.Fatalf("workspace diff leaked external symlink target content: %+v", d)
		}
		if d.NewPath == "leaked_link" {
			found = true
			if d.NewFileContent != outside {
				t.Fatalf("NewFileContent = %q, want symlink target %q", d.NewFileContent, outside)
			}
		}
	}
	if !found {
		t.Fatal("expected diff for untracked symlink")
	}
}

func TestWorkspaceTrackedFileChangedToSymlinkDoesNotReadExternalTarget(t *testing.T) {
	repo := t.TempDir()
	runGitTest(t, repo, "init", "-q")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test User")
	runGitTest(t, repo, "config", "commit.gpgsign", "false")

	victim := filepath.Join(repo, "victim.txt")
	if err := os.WriteFile(victim, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("write victim.txt: %v", err)
	}
	runGitTest(t, repo, "add", "victim.txt")
	runGitTest(t, repo, "commit", "-q", "-m", "initial commit")

	secret := "TRACKED_SYMLINK_SECRET_SHOULD_NOT_LEAK\n"
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte(secret), 0o644); err != nil {
		t.Fatalf("write external secret: %v", err)
	}
	if err := os.Remove(victim); err != nil {
		t.Fatalf("remove victim.txt: %v", err)
	}
	if err := os.Symlink(outside, victim); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	provider := NewWorkspaceProvider(repo, gitcmd.New(0))
	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff returned error: %v", err)
	}

	var foundSymlinkAdd bool
	for _, d := range diffs {
		if strings.Contains(d.Diff, secret) || strings.Contains(d.NewFileContent, secret) {
			t.Fatalf("workspace diff leaked external symlink target content: %+v", d)
		}
		if d.NewPath == "victim.txt" && d.IsNew {
			foundSymlinkAdd = true
			if d.NewFileContent != outside {
				t.Fatalf("NewFileContent = %q, want symlink target %q", d.NewFileContent, outside)
			}
		}
	}
	if !foundSymlinkAdd {
		t.Fatal("expected new-file diff for tracked file changed to symlink")
	}
}

// TestRangeDiffDetectsRename guards against issue #99: when a file is renamed
// on the target branch, `ocr review --from master --to BRANCH` must recognize
// the rename and read content at the NEW path. Before the fix the rename could
// surface as delete(old)+add(new) (e.g. diff.renames=false) and the parser's
// broken /dev/null detection sent the deleted half into `git show ref:oldpath`
// -> "WARNING: cannot read file ... exit status 128".
func TestRangeDiffDetectsRename(t *testing.T) {
	repo := initRepoWithChange(t)

	// Reset the working-tree modification left by the helper.
	runGitTest(t, repo, "checkout", "--", "sample.txt")
	// Simulate a user config where git does NOT detect renames on its own;
	// the provider must force --find-renames.
	runGitTest(t, repo, "config", "diff.renames", "false")

	// Commit a file large enough for git's similarity detection to work
	// (tiny files fall below the rename threshold even for 1-line edits).
	var content strings.Builder
	for i := 1; i <= 50; i++ {
		fmt.Fprintf(&content, "line%d\n", i)
	}
	orig := filepath.Join(repo, "orig.txt")
	if err := os.WriteFile(orig, []byte(content.String()), 0o644); err != nil {
		t.Fatalf("write orig.txt: %v", err)
	}
	runGitTest(t, repo, "add", "orig.txt")
	runGitTest(t, repo, "commit", "-q", "-m", "add orig.txt")

	// Rename on a feature branch, with a small edit (like the issue repro).
	runGitTest(t, repo, "checkout", "-q", "-b", "feature")
	runGitTest(t, repo, "mv", "orig.txt", "renamed.txt")
	edited := strings.Replace(content.String(), "line25\n", "line25-edited\n", 1)
	if err := os.WriteFile(filepath.Join(repo, "renamed.txt"), []byte(edited), 0o644); err != nil {
		t.Fatalf("edit renamed.txt: %v", err)
	}
	runGitTest(t, repo, "add", "-A")
	runGitTest(t, repo, "commit", "-q", "-m", "rename orig.txt")

	runner := gitcmd.New(0)
	provider := NewProvider(repo, "HEAD~1", "feature", runner)

	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff (range, rename) returned error: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected exactly 1 diff for a rename, got %d: %+v", len(diffs), diffs)
	}
	d := diffs[0]
	if !d.IsRenamed {
		t.Errorf("IsRenamed = false, want true")
	}
	if d.OldPath != "orig.txt" || d.NewPath != "renamed.txt" {
		t.Errorf("OldPath/NewPath = %q/%q, want orig.txt/renamed.txt", d.OldPath, d.NewPath)
	}
	if d.NewFileContent == "" {
		t.Errorf("NewFileContent is empty: content at new path was not read at ref")
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

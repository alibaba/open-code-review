package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/open-code-review/open-code-review/internal/config/rules"
)

func TestApplyCLIExcludes_Empty(t *testing.T) {
	cc := &commonContext{FileFilter: &rules.FileFilter{Exclude: []string{"a"}}}
	applyCLIExcludes(cc, nil)
	if len(cc.FileFilter.Exclude) != 1 {
		t.Errorf("expected 1 exclude, got %d", len(cc.FileFilter.Exclude))
	}
}

func TestApplyCLIExcludes_AppendsPatterns(t *testing.T) {
	cc := &commonContext{FileFilter: &rules.FileFilter{Exclude: []string{"a"}}}
	applyCLIExcludes(cc, []string{"b", "c"})
	if len(cc.FileFilter.Exclude) != 3 {
		t.Errorf("expected 3 excludes, got %d", len(cc.FileFilter.Exclude))
	}
}

func TestApplyCLIExcludes_NilFileFilter(t *testing.T) {
	cc := &commonContext{}
	applyCLIExcludes(cc, []string{"x"})
	if cc.FileFilter == nil {
		t.Fatal("expected FileFilter to be created")
	}
	if len(cc.FileFilter.Exclude) != 1 || cc.FileFilter.Exclude[0] != "x" {
		t.Errorf("expected [x], got %v", cc.FileFilter.Exclude)
	}
}

func TestNewQuietHandle_NoOp(t *testing.T) {
	h := newQuietHandle("text", "developer")
	if h.fn != nil {
		t.Error("expected no-op handle for text/developer")
	}
	h.Restore()
}

func TestNewQuietHandle_JSON(t *testing.T) {
	h := newQuietHandle("json", "developer")
	if h.fn == nil {
		t.Error("expected fn to be set for json format")
	}
	h.Restore()
	if h.fn != nil {
		t.Error("expected fn to be nil after Restore")
	}
}

func TestNewQuietHandle_Agent(t *testing.T) {
	h := newQuietHandle("text", "agent")
	if h.fn == nil {
		t.Error("expected fn to be set for agent audience")
	}
	h.Restore()
}

func TestQuietHandle_NilReceiver(t *testing.T) {
	var h *quietHandle
	h.Restore()
}

func TestQuietHandle_IdempotentRestore(t *testing.T) {
	h := newQuietHandle("json", "developer")
	h.Restore()
	h.Restore()
	if h.fn != nil {
		t.Error("expected nil after double restore")
	}
}

func TestResolveWorkingDir_CurrentDir(t *testing.T) {
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("restore chdir: %v", err)
		}
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	absPath, isGit, err := resolveWorkingDir("", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if absPath == "" {
		t.Error("expected non-empty absPath")
	}
	if isGit {
		t.Error("temp dir should not be a git repo")
	}
}

func TestResolveWorkingDir_RequireGitFails(t *testing.T) {
	dir := t.TempDir()
	_, _, err := resolveWorkingDir(dir, true)
	if err == nil {
		t.Fatal("expected error for non-git dir with requireGit=true")
	}
}

func TestResolveWorkingDir_NonExistent(t *testing.T) {
	_, _, err := resolveWorkingDir(filepath.Join(t.TempDir(), "no-such-dir"), false)
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

// TestResolveWorkingDir_MonorepoSubdir reproduces #287: running `ocr review`
// from a subdirectory of a git repo must anchor RepoDir at the git top-level
// (git reports diff / `git show HEAD:<path>` paths relative to the repo root),
// while `ocr scan` (requireGit=false) must keep the subdirectory so its walk
// stays scoped.
func TestResolveWorkingDir_MonorepoSubdir(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	git("config", "user.email", "t@t.co")
	git("config", "user.name", "t")

	sub := filepath.Join(root, "subproject1", "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// macOS /var -> /private/var symlink means t.TempDir() differs from the
	// canonicalized toplevel git returns; compare via EvalSymlinks.
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", root, err)
	}

	// review path: hoisted to the git top-level.
	got, isGit, err := resolveWorkingDir(sub, true)
	if err != nil {
		t.Fatalf("resolveWorkingDir(sub, true) error: %v", err)
	}
	if !isGit {
		t.Error("expected isGit=true for a git subdirectory")
	}
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", got, err)
	}
	if gotResolved != wantRoot {
		t.Errorf("review RepoDir = %q, want git top-level %q", gotResolved, wantRoot)
	}

	// scan path: keeps the subdirectory unchanged.
	gotScan, _, err := resolveWorkingDir(sub, false)
	if err != nil {
		t.Fatalf("resolveWorkingDir(sub, false) error: %v", err)
	}
	gotScanResolved, err := filepath.EvalSymlinks(gotScan)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", gotScan, err)
	}
	wantSub, err := filepath.EvalSymlinks(sub)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", sub, err)
	}
	if gotScanResolved != wantSub {
		t.Errorf("scan RepoDir = %q, want subdir %q (must stay scoped)", gotScanResolved, wantSub)
	}
}

// TestResolveWorkingDir_BareRepoFailsLoudly guards the #287 fix: a bare repo has
// no work tree, so `git rev-parse --git-dir` succeeds (isGit=true) but
// `--show-toplevel` fails. The review path (requireGit=true) must return an
// error rather than silently reusing the input dir, which would reproduce the
// original root-relative-path bug.
func TestResolveWorkingDir_BareRepoFailsLoudly(t *testing.T) {
	bare := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", bare)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}

	_, _, err := resolveWorkingDir(bare, true)
	if err == nil {
		t.Fatal("expected error for a bare repo (no work tree), got nil")
	}
}

func TestResolveWorkingDir_GitRepo(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	absPath, isGit, err := resolveWorkingDir(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if absPath == "" {
		t.Error("expected non-empty absPath")
	}
	_ = isGit
}

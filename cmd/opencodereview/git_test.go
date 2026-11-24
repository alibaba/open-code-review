// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/tool"
)

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"git", "-C", dir, "init"},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init: %v: %s", err, out)
		}
	}
	f := filepath.Join(dir, "README.md")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	cmd := exec.Command("git", "-C", dir, "add", ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	cmd = exec.Command("git", "-C", dir, "commit", "-m", "initial commit")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	return dir
}

func TestRunGitCmd_Success(t *testing.T) {
	dir := initTestGitRepo(t)
	out, err := runGitCmd(dir, "rev-parse", "--git-dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected non-empty output")
	}
}

func TestRunGitCmd_Failure(t *testing.T) {
	dir := t.TempDir()
	_, err := runGitCmd(dir, "rev-parse", "--git-dir")
	if err == nil {
		t.Error("expected error for non-git dir")
	}
}

func TestGetCommitMessage(t *testing.T) {
	dir := initTestGitRepo(t)
	msg, err := getCommitMessage(dir, "HEAD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "initial commit" {
		t.Errorf("msg = %q, want 'initial commit'", msg)
	}
}

func TestGetCommitMessage_InvalidCommit(t *testing.T) {
	dir := initTestGitRepo(t)
	_, err := getCommitMessage(dir, "nonexistent-ref-xyz")
	if err == nil {
		t.Fatal("expected error for invalid commit")
	}
}

// TestGetCommitMessage_IgnoresGitStderr covers ocr review --commit, which
// uses the commit message as review background. getCommitMessage used
// CombinedOutput, so GIT_TRACE / git warnings were prepended to the message
// and sent to the LLM. runGitCmdStdout already exists for this class of bug.
func TestGetCommitMessage_IgnoresGitStderr(t *testing.T) {
	dir := initTestGitRepo(t)
	t.Setenv("GIT_TRACE", "1")
	msg, err := getCommitMessage(dir, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if msg != "initial commit" {
		t.Errorf("msg = %q, want %q (git stderr mixed into commit message?)", msg, "initial commit")
	}
}

func TestResolveRepoDir_ValidGitRepo(t *testing.T) {
	dir := initTestGitRepo(t)
	resolved, err := resolveRepoDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved == "" {
		t.Error("expected non-empty resolved path")
	}
}

// TestResolveRepoDir_NotGitRepo covers #1704: rules check resolves rules from
// file content in the working tree (the resolver gets no ref), so a non-git
// directory must resolve to itself instead of failing like the review path.
func TestResolveRepoDir_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	resolved, err := resolveRepoDir(dir)
	if err != nil {
		t.Fatalf("unexpected error for non-git dir: %v", err)
	}
	got, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", resolved, err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}
	if got != want {
		t.Errorf("resolved = %q, want %q", got, want)
	}
}

// TestResolveRepoDir_MonorepoSubdirAnchors pins the git-repo half of the
// behavior: inside a git repo, `ocr rules check` still anchors at the git
// top-level so project rules (.opencodereview/rule.json) resolve the same way
// as the review path when run from a monorepo subdirectory (#287).
func TestResolveRepoDir_MonorepoSubdirAnchors(t *testing.T) {
	root := initTestGitRepo(t)
	sub := filepath.Join(root, "subproject1", "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveRepoDir(sub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", resolved, err)
	}
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", root, err)
	}
	if got != wantRoot {
		t.Errorf("resolved = %q, want git top-level %q", got, wantRoot)
	}
}

// TestResolveRepoDir_BareRepoFailsLoudly guards the anchoring step: a bare
// repo passes the --git-dir probe (isGit=true) but has no work tree, so
// --show-toplevel fails. It must fail loudly rather than silently applying
// rules from the input dir, matching the review path.
func TestResolveRepoDir_BareRepoFailsLoudly(t *testing.T) {
	bare := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", bare)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}

	_, err := resolveRepoDir(bare)
	if err == nil {
		t.Fatal("expected error for a bare repo (no work tree), got nil")
	}
	if !strings.Contains(err.Error(), "without a work tree") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProbeGitDir_PlainDir(t *testing.T) {
	isGit, err := probeGitDir(t.TempDir())
	if err != nil {
		t.Fatalf("plain dir must classify as no-repository, not error: %v", err)
	}
	if isGit {
		t.Error("plain dir must not report a repository")
	}
}

func TestIsNoGitRepoProbe(t *testing.T) {
	cmd := exec.Command("git", "-C", t.TempDir(), "rev-parse", "--git-dir")
	_, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 128 {
		t.Fatalf("expected git to exit with status 128, got %v", err)
	}

	for _, tt := range []struct {
		name   string
		output string
		want   bool
	}{
		{"no repository", "fatal: not a git repository (or any of the parent directories): .git\n", true},
		{"mount boundary", "fatal: not a git repository (or any parent up to mount point /dev)\nStopping at filesystem boundary (GIT_DISCOVERY_ACROSS_FILESYSTEM not set).\n", true},
		{"dubious ownership", "fatal: detected dubious ownership in repository at '/worktree'\n", false},
		{"invalid git directory", "fatal: not a git repository: /worktree/.git\n", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNoGitRepoProbe([]byte(tt.output), err); got != tt.want {
				t.Errorf("isNoGitRepoProbe() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProbeGitDir_GitRepo(t *testing.T) {
	isGit, err := probeGitDir(initTestGitRepo(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isGit {
		t.Error("git repo must report a repository")
	}
}

// TestProbeGitDir_MissingGitBinary pins the other half of the classification:
// with no git to ask, "is this inside a repository" has no answer, so the
// probe must report that instead of guessing a plain directory.
func TestProbeGitDir_MissingGitBinary(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := probeGitDir(t.TempDir())
	if err == nil {
		t.Fatal("expected an error when git cannot be executed")
	}
	if !strings.Contains(err.Error(), "cannot determine") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestResolveRepoDir_GitProbeFailureFailsLoudly verifies that when git
// refuses to read the repository (dubious ownership in a worktree
// subdirectory is the case that bit), `ocr rules check` must fail loudly
// instead of silently treating the subdirectory as a plain project root,
// which would print a global or built-in rule for the wrong directory. The
// repo is initialized before t.Setenv: under GIT_TEST_ASSUME_DIFFERENT_OWNER
// even `git add` is refused, so initTestGitRepo would fail.
func TestResolveRepoDir_GitProbeFailureFailsLoudly(t *testing.T) {
	repo := initTestGitRepo(t)
	sub := filepath.Join(repo, "subproject1", "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// Make git distrust the repo it just created, as if it were owned by
	// someone else. Neutralized global/system configs keep a machine's
	// safe.directory exception from silencing the refusal.
	t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	if out, err := runGitCmd(sub, "rev-parse", "--git-dir"); err == nil {
		t.Skip("installed git does not honor GIT_TEST_ASSUME_DIFFERENT_OWNER")
	} else if !strings.Contains(string(out), "dubious ownership") {
		t.Fatalf("expected git ownership refusal, got %v: %s", err, out)
	}

	prevRepoDir := rulesCheckRepoDir
	rulesCheckRepoDir = sub
	t.Cleanup(func() { rulesCheckRepoDir = prevRepoDir })
	if err := runRulesCheck("src/Example.java"); err == nil {
		t.Fatal("rules check must fail when git refuses to read the repository")
	} else if !strings.Contains(err.Error(), "dubious ownership") {
		t.Errorf("rules check should surface git's refusal, got: %v", err)
	}
}

func TestResolveRepoDir_EmptyUsesWd(t *testing.T) {
	dir := initTestGitRepo(t)
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

	resolved, err := resolveRepoDir("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved == "" {
		t.Error("expected non-empty resolved path")
	}
}

func TestRequireGitRepo_Valid(t *testing.T) {
	dir := initTestGitRepo(t)
	if err := requireGitRepo(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireGitRepo_Invalid(t *testing.T) {
	dir := t.TempDir()
	err := requireGitRepo(dir)
	if err == nil {
		t.Fatal("expected error for non-git dir")
	}
}

func TestValidateReviewRefs_ValidCommit(t *testing.T) {
	dir := initTestGitRepo(t)
	err := validateReviewRefs(dir, reviewOptions{commit: "HEAD"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateReviewRefs_InvalidCommit(t *testing.T) {
	dir := initTestGitRepo(t)
	err := validateReviewRefs(dir, reviewOptions{commit: "nonexistent-ref-xyz"})
	if err == nil {
		t.Fatal("expected error for invalid commit ref")
	}
}

func TestValidateReviewRefs_EmptySkipped(t *testing.T) {
	dir := initTestGitRepo(t)
	err := validateReviewRefs(dir, reviewOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildToolRegistry(t *testing.T) {
	reg := buildToolRegistry(nil, nil)
	for _, name := range []string{"file_read", "file_find", "code_search", "code_comment"} {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("review tools must include %s", name)
		}
	}
	p, ok := reg.Get(tool.FileReadDiff.Name())
	if !ok {
		t.Fatal("review tools must include file_read_diff")
	}
	frd, ok := p.(*tool.FileReadDiffProvider)
	if !ok {
		t.Fatalf("unexpected diff provider type: %T", p)
	}
	const diff = "@@ -1 +1 @@\n-old\n+new\n"
	frd.SetDiffMap(tool.NewDiffMap(map[string]string{"main.go": diff}))
	got, err := p.Execute(context.Background(), map[string]any{"path_array": []any{"main.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, diff) {
		t.Fatalf("review diff = %q, want %q", got, diff)
	}
}

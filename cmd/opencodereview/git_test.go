// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

// TestResolveRepoDir_NotGitRepo covers #1704: rule resolution has no git
// dependency (the resolver reads the working tree when no ref is given), so a
// non-git directory must resolve instead of failing like the review path.
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

// TestResolveRepoDir_MonorepoSubdirAnchors pins the git-repo half of #1704:
// inside a git repo, `ocr rules check` must still anchor at the git top-level
// like the review path, so project rules (.opencodereview/rule.json) resolve
// the same way when run from a monorepo subdirectory (#287).
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

// TestResolveRepoDir_BareRepoFailsLoudly guards the re-anchor branch: a bare
// repo has no work tree, so --show-toplevel fails. Fail loudly rather than
// silently reusing the input dir, matching the review path's behavior.
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
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
}

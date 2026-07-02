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
	os.WriteFile(f, []byte("hello"), 0o644)
	cmd := exec.Command("git", "-C", dir, "add", ".")
	cmd.CombinedOutput()
	cmd = exec.Command("git", "-C", dir, "commit", "-m", "initial commit")
	cmd.CombinedOutput()
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

func TestResolveRepoDir_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := resolveRepoDir(dir)
	if err == nil {
		t.Fatal("expected error for non-git dir")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveRepoDir_EmptyUsesWd(t *testing.T) {
	dir := initTestGitRepo(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

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

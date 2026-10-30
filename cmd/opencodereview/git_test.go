// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
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

func TestBuildReviewToolRegistry(t *testing.T) {
	reg := buildReviewToolRegistry(nil, nil)
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

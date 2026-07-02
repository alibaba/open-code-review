package main

import (
	"os"
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
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

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

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitCommitFile writes a file and commits it, returning after the commit lands.
func gitCommitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", msg}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

// silenceStdout redirects os.Stdout to /dev/null for the duration of fn so the
// delegate commands' Printf output does not clutter test logs.
func silenceStdout(t *testing.T, fn func()) {
	t.Helper()
	orig := os.Stdout
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	os.Stdout = devnull
	defer func() {
		os.Stdout = orig
		_ = devnull.Close()
	}()
	fn()
}

func TestExecuteDelegatePreview_Workspace(t *testing.T) {
	dir := initTestGitRepo(t)
	// Uncommitted change so the workspace preview has at least one entry.
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("package app\n"), 0o644); err != nil {
		t.Fatalf("write app.go: %v", err)
	}
	silenceStdout(t, func() {
		if err := executeDelegatePreview(delegateOptions{repoDir: dir}); err != nil {
			t.Fatalf("executeDelegatePreview(workspace) error: %v", err)
		}
	})
}

func TestExecuteDelegatePreview_Range(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "b.go", "package b\n", "second commit")
	silenceStdout(t, func() {
		err := executeDelegatePreview(delegateOptions{repoDir: dir, from: "HEAD~1", to: "HEAD"})
		if err != nil {
			t.Fatalf("executeDelegatePreview(range) error: %v", err)
		}
	})
}

func TestExecuteDelegatePreview_Commit(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "c.go", "package c\n", "add c")
	silenceStdout(t, func() {
		// commit mode auto-fills background from the commit message.
		err := executeDelegatePreview(delegateOptions{repoDir: dir, commit: "HEAD"})
		if err != nil {
			t.Fatalf("executeDelegatePreview(commit) error: %v", err)
		}
	})
}

func TestExecuteDelegateRule(t *testing.T) {
	dir := initTestGitRepo(t)
	silenceStdout(t, func() {
		err := executeDelegateRule(delegateOptions{repoDir: dir}, []string{"README.md"})
		if err != nil {
			t.Fatalf("executeDelegateRule error: %v", err)
		}
	})
}

func TestLoadDelegateContext_BackgroundFile(t *testing.T) {
	dir := initTestGitRepo(t)
	bgPath := filepath.Join(dir, "bg.txt")
	if err := os.WriteFile(bgPath, []byte("extra background"), 0o644); err != nil {
		t.Fatalf("write bg: %v", err)
	}
	dc, err := loadDelegateContext(delegateOptions{repoDir: dir, backgroundFile: "bg.txt", background: "base"})
	if err != nil {
		t.Fatalf("loadDelegateContext error: %v", err)
	}
	if dc.opts.background == "" {
		t.Error("expected merged background, got empty")
	}
}

func TestLoadDelegateContext_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadDelegateContext(delegateOptions{repoDir: dir}); err == nil {
		t.Fatal("expected error for non-git dir")
	}
}

func TestDelegateContextMergeBase_Range(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "d.go", "package d\n", "add d")
	dc, err := loadDelegateContext(delegateOptions{repoDir: dir, from: "HEAD~1", to: "HEAD"})
	if err != nil {
		t.Fatalf("loadDelegateContext error: %v", err)
	}
	if got := dc.mergeBase(context.Background()); got == "" {
		t.Error("expected non-empty merge base for range mode")
	}
}

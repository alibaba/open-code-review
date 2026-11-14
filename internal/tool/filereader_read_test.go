// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestFileReader_Read_Workspace(t *testing.T) {
	dir := t.TempDir()
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(filepath.Join(dir, "test.go"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	got, err := fr.Read(context.Background(), "test.go")
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if got != content {
		t.Errorf("Read() = %q, want %q", got, content)
	}
}

func TestFileReader_Read_WorkspaceNotFound(t *testing.T) {
	dir := t.TempDir()
	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	_, err := fr.Read(context.Background(), "missing.go")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestFileReader_Read_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}

	_, err := fr.Read(context.Background(), "../../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestFileReader_Read_SymlinkOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	secretFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("sensitive"), 0644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(secretFile, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	_, err := fr.Read(context.Background(), "link.txt")
	if err == nil {
		t.Error("expected error for symlink pointing outside repo")
	}
}

func TestFileReader_ReadLines_Workspace(t *testing.T) {
	dir := t.TempDir()
	content := "aaa\nbbb\nccc\nddd\n"
	if err := os.WriteFile(filepath.Join(dir, "lines.txt"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}

	t.Run("all lines", func(t *testing.T) {
		lines, total, err := fr.ReadLines(context.Background(), "lines.txt", 1, 100)
		if err != nil {
			t.Fatal(err)
		}
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
		if len(lines) != 5 {
			t.Errorf("lines count = %d, want 5", len(lines))
		}
	})

	t.Run("start from line 2 with limit", func(t *testing.T) {
		lines, total, err := fr.ReadLines(context.Background(), "lines.txt", 2, 2)
		if err != nil {
			t.Fatal(err)
		}
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
		if len(lines) != 2 {
			t.Fatalf("lines count = %d, want 2", len(lines))
		}
		if lines[0] != "bbb" || lines[1] != "ccc" {
			t.Errorf("lines = %v, want [bbb ccc]", lines)
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		_, _, err := fr.ReadLines(context.Background(), "../../etc/passwd", 1, 10)
		if err == nil {
			t.Error("expected error for path traversal")
		}
	})
}

func TestFileReader_Read_CommitMode(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)

	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}
	got, err := fr.Read(context.Background(), "hello.go")
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if !strings.Contains(got, "package main") {
		t.Errorf("Read() = %q, want containing 'package main'", got)
	}
	if !strings.Contains(got, "func Hello()") {
		t.Errorf("Read() = %q, want containing 'func Hello()'", got)
	}
}

func TestFileReader_Read_CommitMode_MissingFile(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)

	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}
	_, err := fr.Read(context.Background(), "nonexistent.go")
	if err == nil {
		t.Error("expected error for missing file in commit mode")
	}
}

func TestFileReader_Read_CommitMode_WithRunner(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)
	runner := gitcmd.New(4)

	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit, Runner: runner}
	got, err := fr.Read(context.Background(), "hello.go")
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if !strings.Contains(got, "package main") {
		t.Errorf("Read() = %q, want containing 'package main'", got)
	}
}

func TestFileReader_Read_CommitMode_WithRunner_MissingFile(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)
	runner := gitcmd.New(4)

	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit, Runner: runner}
	_, err := fr.Read(context.Background(), "nonexistent.go")
	if err == nil {
		t.Error("expected error for missing file in commit mode with runner")
	}
}

func TestFileReader_ReadLines_CommitMode_WithRunner(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)
	runner := gitcmd.New(4)

	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit, Runner: runner}
	lines, total, err := fr.ReadLines(context.Background(), "hello.go", 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Errorf("totalLines = %d, want 4", total)
	}
	if len(lines) < 1 || lines[0] != "package main" {
		t.Errorf("first line = %q, want %q", lines[0], "package main")
	}
}

func TestFileReader_ReadLines_CommitMode_MissingFile(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)

	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}
	_, _, err := fr.ReadLines(context.Background(), "nonexistent.go", 1, 100)
	if err == nil {
		t.Error("expected error for missing file in commit mode")
	}
}

func TestFileReader_Read_SubdirectoryFile(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "src", "pkg")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	got, err := fr.Read(context.Background(), "src/pkg/main.go")
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if got != "package main" {
		t.Errorf("Read() = %q, want %q", got, "package main")
	}
}

// TestFileReader_Read_CommitMode_MonorepoSubdirPath reproduces #287 at the
// git-show layer: in a monorepo, git reports paths relative to the repo root
// (e.g. "subproject1/src/models/request_meta.py"). With RepoDir anchored at the
// git top-level (the fix), `git show HEAD:<root-relative-path>` must resolve —
// this is the exact command that failed in the issue.
func TestFileReader_Read_CommitMode_MonorepoSubdirPath(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	git("config", "user.email", "t@t.co")
	git("config", "user.name", "t")

	rel := filepath.Join("subproject1", "src", "models", "request_meta.py")
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "class RequestMeta:\n    id = 1\n"
	if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "init")
	commit := getHeadCommit(t, dir)

	// Use the git-style forward-slash path the diff/LLM would supply.
	gitPath := "subproject1/src/models/request_meta.py"

	// git-show (commit mode): the exact path from the issue error.
	frShow := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}
	got, err := frShow.Read(context.Background(), gitPath)
	if err != nil {
		t.Fatalf("commit-mode Read(%q) error: %v", gitPath, err)
	}
	if got != content {
		t.Errorf("commit-mode Read = %q, want %q", got, content)
	}

	// disk (workspace mode): same root-relative path resolves too.
	frDisk := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	gotDisk, err := frDisk.Read(context.Background(), gitPath)
	if err != nil {
		t.Fatalf("workspace-mode Read(%q) error: %v", gitPath, err)
	}
	if gotDisk != content {
		t.Errorf("workspace-mode Read = %q, want %q", gotDisk, content)
	}
}

func TestFileReader_Read_WindowsPath_Workspace(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "pkg", "util")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "package util\n"
	if err := os.WriteFile(filepath.Join(subDir, "helper.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}

	windowsPath := `pkg\util\helper.go`
	got, err := fr.Read(context.Background(), windowsPath)
	if err != nil {
		t.Fatalf("Read(%q) error: %v", windowsPath, err)
	}
	if got != content {
		t.Errorf("Read = %q, want %q", got, content)
	}

	lines, total, err := fr.ReadLines(context.Background(), windowsPath, 1, 10)
	if err != nil {
		t.Fatalf("ReadLines(%q) error: %v", windowsPath, err)
	}
	if total != 2 || len(lines) == 0 || lines[0] != "package util" {
		t.Errorf("ReadLines = %v, total = %d", lines, total)
	}
}

func TestFileReader_Read_WindowsPath_CommitMode(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)

	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}

	windowsPath := `pkg\util.go`
	got, err := fr.Read(context.Background(), windowsPath)
	if err != nil {
		t.Fatalf("Read(%q) error: %v", windowsPath, err)
	}
	if !strings.Contains(got, "package pkg") {
		t.Errorf("Read(%q) = %q, want containing 'package pkg'", windowsPath, got)
	}

	lines, total, err := fr.ReadLines(context.Background(), windowsPath, 1, 10)
	if err != nil {
		t.Fatalf("ReadLines(%q) error: %v", windowsPath, err)
	}
	if total == 0 || len(lines) == 0 {
		t.Errorf("ReadLines(%q) returned empty result", windowsPath)
	}
}

func setupDualBackslashRepo(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX literal backslashes cannot coexist with directory separators on Windows")
	}

	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	git("config", "user.email", "t@t.co")
	git("config", "user.name", "t")

	if err := os.WriteFile(filepath.Join(dir, `pkg\util.go`), []byte("package backslashfile\n\nfunc LiteralBackslash() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "util.go"), []byte("package slashpkg\n\nfunc SlashPath() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "--", ".")
	git("commit", "-m", "add dual backslash and slash files")
	return dir, getHeadCommit(t, dir)
}

func TestFileReader_POSIXDualBackslashAndSlashFiles(t *testing.T) {
	dir, commit := setupDualBackslashRepo(t)
	runner := gitcmd.New(2)

	readers := []struct {
		name string
		fr   *FileReader
	}{
		{name: "workspace", fr: &FileReader{RepoDir: dir, Mode: ModeWorkspace}},
		{name: "commit", fr: &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}},
		{name: "commit_with_runner", fr: &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit, Runner: runner}},
	}

	for _, r := range readers {
		t.Run(r.name, func(t *testing.T) {
			for _, q := range []string{`pkg\util.go`, `./pkg\util.go`} {
				got, err := r.fr.Read(context.Background(), q)
				if err != nil {
					t.Fatalf("Read(%q) error: %v", q, err)
				}
				if !strings.Contains(got, "package backslashfile") {
					t.Errorf("Read(%q) = %q, want backslashfile content", q, got)
				}

				lines, _, err := r.fr.ReadLines(context.Background(), q, 1, 10)
				if err != nil {
					t.Fatalf("ReadLines(%q) error: %v", q, err)
				}
				if len(lines) == 0 || lines[0] != "package backslashfile" {
					t.Errorf("ReadLines(%q) = %v, want first line 'package backslashfile'", q, lines)
				}
			}

			for _, q := range []string{"pkg/util.go", "./pkg/util.go"} {
				got, err := r.fr.Read(context.Background(), q)
				if err != nil {
					t.Fatalf("Read(%q) error: %v", q, err)
				}
				if !strings.Contains(got, "package slashpkg") {
					t.Errorf("Read(%q) = %q, want slashpkg content", q, got)
				}

				lines, _, err := r.fr.ReadLines(context.Background(), q, 1, 10)
				if err != nil {
					t.Fatalf("ReadLines(%q) error: %v", q, err)
				}
				if len(lines) == 0 || lines[0] != "package slashpkg" {
					t.Errorf("ReadLines(%q) = %v, want first line 'package slashpkg'", q, lines)
				}
			}

			for _, ambQuery := range []string{`.\pkg\util.go`, `pkg\.\util.go`} {
				got, err := r.fr.Read(context.Background(), ambQuery)
				if err == nil {
					t.Fatalf("Read(%q) succeeded with %q, want ambiguity error", ambQuery, got)
				}
				if !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "pkg/util.go") || !strings.Contains(err.Error(), `pkg\util.go`) {
					t.Errorf("Read(%q) error = %v, want ambiguity error naming both candidates", ambQuery, err)
				}

				lines, _, err := r.fr.ReadLines(context.Background(), ambQuery, 1, 10)
				if err == nil {
					t.Fatalf("ReadLines(%q) succeeded with %v, want ambiguity error", ambQuery, lines)
				}
				if !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "pkg/util.go") || !strings.Contains(err.Error(), `pkg\util.go`) {
					t.Errorf("ReadLines(%q) error = %v, want ambiguity error naming both candidates", ambQuery, err)
				}
			}
		})
	}
}

func TestFileReader_POSIXLiteralBackslashOnlyNotSubstituted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX literal backslashes cannot exist on Windows")
	}

	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	git("config", "user.email", "t@t.co")
	git("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, `pkg\util.go`), []byte("package backslashonly\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "--", `pkg\util.go`)
	git("commit", "-m", "literal backslash file only")
	commit := getHeadCommit(t, dir)

	for _, fr := range []*FileReader{
		{RepoDir: dir, Mode: ModeWorkspace},
		{RepoDir: dir, Mode: ModeCommit, Ref: commit},
	} {
		if got, err := fr.Read(context.Background(), `pkg\util.go`); err != nil || !strings.Contains(got, "package backslashonly") {
			t.Errorf("Read(pkg\\util.go) = (%q, %v), want backslashonly", got, err)
		}
		if got, err := fr.Read(context.Background(), "pkg/util.go"); err == nil {
			t.Errorf("Read(pkg/util.go) = %q, want error instead of substituting literal backslash file", got)
		}
		if lines, _, err := fr.ReadLines(context.Background(), "pkg/util.go", 1, 10); err == nil {
			t.Errorf("ReadLines(pkg/util.go) = %v, want error instead of substituting literal backslash file", lines)
		}
	}
}

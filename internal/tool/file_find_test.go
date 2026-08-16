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

// TestFileFind_NonGitDirectoryFallback verifies file_find works in a plain
// (non-git) directory by falling back to a filesystem walk instead of
// failing with git's exit 128.
func TestFileFind_NonGitDirectoryFallback(t *testing.T) {
	dir := t.TempDir() // plain dir, no `git init`

	write := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("server.go", "package main\n")
	write("internal/handler.go", "package internal\n")
	write("node_modules/lib/index.js", "x\n") // excluded by blocklist
	write(".gitignore", "ignored.go\n")
	write("ignored.go", "package x\n") // excluded by root .gitignore

	p := NewFileFind(&FileReader{RepoDir: dir, Mode: ModeWorkspace})

	out, err := p.Execute(context.Background(), map[string]any{"query_name": ".go"})
	if err != nil {
		t.Fatalf("Execute should not error in a non-git dir, got: %v", err)
	}

	if !strings.Contains(out, "server.go") || !strings.Contains(out, "internal/handler.go") {
		t.Errorf("expected go files in result, got:\n%s", out)
	}
	if strings.Contains(out, "node_modules") {
		t.Errorf("node_modules should be excluded, got:\n%s", out)
	}
	if strings.Contains(out, "ignored.go") {
		t.Errorf("ignored.go should be excluded by .gitignore, got:\n%s", out)
	}
}

// TestFileFind_NonGitDirectoryNoMatch verifies the not-found path in a
// non-git dir returns the sentinel rather than an error.
func TestFileFind_NonGitDirectoryNoMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewFileFind(&FileReader{RepoDir: dir, Mode: ModeWorkspace})

	out, err := p.Execute(context.Background(), map[string]any{"query_name": "nonexistent_xyz"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("expected not-found sentinel, got: %q", out)
	}
}

func TestFileFindProvider_Tool(t *testing.T) {
	p := NewFileFind(&FileReader{RepoDir: "/tmp"})
	if p.Tool() != FileFind {
		t.Errorf("Tool() = %v, want FileFind", p.Tool())
	}
}

func TestFileFind_BlankQuery(t *testing.T) {
	p := NewFileFind(&FileReader{RepoDir: "/tmp"})
	got, err := p.Execute(context.Background(), map[string]any{"query_name": "  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "not found") {
		t.Errorf("expected not-found for blank query, got: %q", got)
	}
}

func setupFileFindRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", args, err, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")

	write := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", "package main\n")
	write("pkg/util.go", "package pkg\n")
	write("Makefile", "all:\n")
	write("Dockerfile", "FROM scratch\n")
	write("LICENSE", "MIT\n")
	write("data_binary", "binary\n")

	run("git", "add", ".")
	run("git", "commit", "-m", "init")
	return dir
}

func TestFileFind_GitRepo_WorkspaceMode(t *testing.T) {
	dir := setupFileFindRepo(t)
	p := NewFileFind(&FileReader{RepoDir: dir, Mode: ModeWorkspace})

	got, err := p.Execute(context.Background(), map[string]any{"query_name": ".go"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "main.go") || !strings.Contains(got, "util.go") {
		t.Errorf("expected .go files, got: %s", got)
	}
}

func TestFileFind_GitRepo_CommitMode(t *testing.T) {
	dir := setupFileFindRepo(t)
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(string(out))

	p := NewFileFind(&FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit})

	got, execErr := p.Execute(context.Background(), map[string]any{"query_name": ".go"})
	if execErr != nil {
		t.Fatal(execErr)
	}
	if !strings.Contains(got, "main.go") {
		t.Errorf("expected main.go in commit mode, got: %s", got)
	}
}

func TestFileFind_GitRepo_PreservesSpecialPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test paths contain characters unsupported by Windows")
	}

	dir := setupFileFindRepo(t)
	specialPaths := []string{
		"unicode-测试.go",
		"tab\tquote\"file.go",
		"line\nbreak.go",
	}
	for _, path := range specialPaths {
		if err := os.WriteFile(filepath.Join(dir, path), []byte("package special\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("add", ".")
	git("commit", "-m", "add special paths")
	commit := git("rev-parse", "HEAD")

	tests := []struct {
		name string
		mode ReviewMode
		ref  string
	}{
		{name: "workspace", mode: ModeWorkspace},
		{name: "commit", mode: ModeCommit, ref: commit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewFileFind(&FileReader{RepoDir: dir, Mode: tt.mode, Ref: tt.ref})

			files, err := p.listGitFiles(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			// Newline paths are asserted only at the enumeration layer because
			// Execute intentionally joins multiple results with newlines.
			for _, want := range append([]string{"main.go"}, specialPaths...) {
				found := false
				for _, got := range files {
					if got == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("listGitFiles() missing original path %q; got %q", want, files)
				}
			}

			for _, query := range []struct {
				name string
				want string
			}{
				{name: "main.go", want: "main.go"},
				{name: "unicode-", want: "unicode-测试.go"},
				{name: "quote", want: "tab\tquote\"file.go"},
			} {
				got, err := p.Execute(context.Background(), map[string]any{"query_name": query.name})
				if err != nil {
					t.Fatal(err)
				}
				if got != query.want {
					t.Errorf("Execute(query_name=%q) = %q, want %q", query.name, got, query.want)
				}
			}
		})
	}
}

func TestFileFind_GitRepo_WithRunner(t *testing.T) {
	dir := setupFileFindRepo(t)
	runner := gitcmd.New(4)
	p := NewFileFind(&FileReader{RepoDir: dir, Mode: ModeWorkspace, Runner: runner})

	got, err := p.Execute(context.Background(), map[string]any{"query_name": ".go"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "main.go") {
		t.Errorf("expected main.go via Runner, got: %s", got)
	}
}

func TestFileFind_CaseSensitive(t *testing.T) {
	dir := setupFileFindRepo(t)
	p := NewFileFind(&FileReader{RepoDir: dir, Mode: ModeWorkspace})

	got, err := p.Execute(context.Background(), map[string]any{
		"query_name":     "makefile",
		"case_sensitive": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Makefile") {
		t.Errorf("case-insensitive should find Makefile, got: %s", got)
	}

	got2, err2 := p.Execute(context.Background(), map[string]any{
		"query_name":     "makefile",
		"case_sensitive": true,
	})
	if err2 != nil {
		t.Fatal(err2)
	}
	if strings.Contains(got2, "Makefile") {
		t.Errorf("case-sensitive should not find Makefile when searching 'makefile', got: %s", got2)
	}
}

func TestShouldSkipFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"main.go", false},
		{"pkg/util.go", false},
		{"README.md", false},
		{"Makefile", false},
		{"Dockerfile", false},
		{"LICENSE", false},
		{"Vagrantfile", false},
		{"Containerfile", false},
		{"some_binary", true},
		{"dir/unknown_file", true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := shouldSkipFile(tt.path)
			if got != tt.want {
				t.Errorf("shouldSkipFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

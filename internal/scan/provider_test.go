// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestCountLines(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want int
	}{
		{"empty", []byte(""), 0},
		{"single line no newline", []byte("foo"), 1},
		{"single line trailing newline", []byte("foo\n"), 1},
		{"two lines no trailing newline", []byte("foo\nbar"), 2},
		{"two lines trailing newline", []byte("foo\nbar\n"), 2},
		{"only newline", []byte("\n"), 1},
		{"three lines mixed", []byte("a\n\nb"), 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countLines(tt.in); got != tt.want {
				t.Errorf("countLines(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestPathMatches(t *testing.T) {
	tests := []struct {
		name  string
		f     string
		want  string
		match bool
	}{
		{name: "exact file", f: "README.md", want: "README.md", match: true},
		{name: "directory itself", f: "internal/agent", want: "internal/agent", match: true},
		{name: "file under directory", f: "internal/agent/agent.go", want: "internal/agent", match: true},
		{name: "prefix not at boundary", f: "internal/agent/agent.go", want: "internal/age"},
		{name: "partial file name", f: "internal/scan/provider.go", want: "internal/scan/provider"},
		{name: "other path", f: "README.md", want: "docs"},
		{name: "case variant is a different path", f: "SRC/Keep.go", want: "src"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pathMatches(tt.f, tt.want); got != tt.match {
				t.Errorf("pathMatches(%q, %q) = %v, want %v", tt.f, tt.want, got, tt.match)
			}
		})
	}
}

func TestNewProvider_NormalizesPaths(t *testing.T) {
	p := NewProvider("/tmp/repo", []string{
		"   ",
		"./internal/agent/",
		"cmd",
		"   internal/diff   ",
		filepath.FromSlash("a/b"),
	}, nil, 0)
	want := []string{"internal/agent", "cmd", "internal/diff", "a/b"}
	if !reflect.DeepEqual(p.paths, want) {
		t.Errorf("paths = %v, want %v", p.paths, want)
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	run("config", "commit.gpgsign", "false")
	return repo
}

func writeFile(t *testing.T, root, rel string, content []byte) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func gitCommit(t *testing.T, repo, msg string) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", msg}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestProvider_Enumerate_FullRepo(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "main.go", []byte("package main\n\nfunc main() {}\n"))
	writeFile(t, repo, "pkg/util.go", []byte("package pkg\n"))
	writeFile(t, repo, "image.bin", []byte{0x00, 0x01, 0x02})
	writeFile(t, repo, ".gitignore", []byte("ignored.txt\n"))
	writeFile(t, repo, "ignored.txt", []byte("should not appear\n"))
	gitCommit(t, repo, "init")

	got, err := NewProvider(repo, nil, nil, 0).Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}

	paths := make([]string, 0, len(got))
	for _, it := range got {
		paths = append(paths, it.Path)
	}
	sort.Strings(paths)
	want := []string{".gitignore", "image.bin", "main.go", "pkg/util.go"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}

	for _, it := range got {
		switch it.Path {
		case "main.go":
			if it.IsBinary {
				t.Errorf("main.go must not be binary")
			}
			if !strings.Contains(it.Content, "package main") {
				t.Errorf("main.go content unexpected: %q", it.Content)
			}
			if it.LineCount != 3 {
				t.Errorf("main.go LineCount = %d, want 3", it.LineCount)
			}
		case "image.bin":
			if !it.IsBinary {
				t.Errorf("image.bin must be binary")
			}
			if it.Content != "" {
				t.Errorf("binary item must not store content, got %d bytes", len(it.Content))
			}
		}
	}
}

func TestProvider_Enumerate_NonGitDirectory(t *testing.T) {
	// Plain temp dir — no `git init`. Walker fallback should kick in.
	repo := t.TempDir()
	writeFile(t, repo, "main.go", []byte("package main\n"))
	writeFile(t, repo, "pkg/util.go", []byte("package pkg\n"))
	writeFile(t, repo, ".gitignore", []byte("*\n!*/\n!.gitignore\n!*.go\n!*.js\n!logs/keep.log\n"))
	writeFile(t, repo, "logs/keep.log", []byte("keep\n"))
	writeFile(t, repo, "sub/logs/keep.log", []byte("ignore\n"))
	writeFile(t, repo, "ignored.txt", []byte("should be excluded by root .gitignore\n"))
	writeFile(t, repo, "node_modules/lib/foo.js", []byte("module.exports = 1;\n")) // should be skipped via ExcludedDirs

	got, err := NewProvider(repo, nil, nil, 0).Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate (non-git): %v", err)
	}

	paths := make([]string, 0, len(got))
	for _, it := range got {
		paths = append(paths, it.Path)
	}
	sort.Strings(paths)

	want := []string{".gitignore", "logs/keep.log", "main.go", "pkg/util.go"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("paths = %v, want %v (ignored.txt must be filtered by .gitignore, node_modules/* by ExcludedDirs)", paths, want)
	}
}

// TestProvider_Enumerate_RespectsContextCancellation guards the
// per-iteration ctx.Err() check that was previously missing.
func TestProvider_Enumerate_RespectsContextCancellation(t *testing.T) {
	repo := initTestRepo(t)
	for i := range 30 {
		writeFile(t, repo, "pkg/"+strings.Repeat("a", i+1)+".go", []byte("package pkg\n"))
	}
	gitCommit(t, repo, "init")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled
	_, err := NewProvider(repo, nil, nil, 0).Enumerate(ctx)
	if err == nil {
		t.Fatal("expected ctx-cancelled error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestProvider_Enumerate_PathFilter(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "a.go", []byte("package a\n"))
	writeFile(t, repo, "pkg/b.go", []byte("package pkg\n"))
	writeFile(t, repo, "pkg/sub/c.go", []byte("package sub\n"))
	gitCommit(t, repo, "init")

	got, err := NewProvider(repo, []string{"pkg"}, nil, 0).Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	paths := make([]string, 0, len(got))
	for _, it := range got {
		paths = append(paths, it.Path)
	}
	sort.Strings(paths)
	want := []string{"pkg/b.go", "pkg/sub/c.go"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}
}

func TestProvider_Enumerate_GitignoreAncestorRules(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, ".gitignore", []byte("src/**\n!src/*.go\ndocs/generated\n"))
	writeFile(t, repo, "src/a.go", []byte("package src\n"))
	writeFile(t, repo, "src/sub/b.go", []byte("package sub\n"))
	writeFile(t, repo, "docs/generated/c.go", []byte("package generated\n"))
	writeFile(t, repo, "nested/docs/generated/d.go", []byte("package generated\n"))
	gitCommit(t, repo, "init")

	got, err := NewProvider(repo, nil, nil, 0).Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	paths := make([]string, 0, len(got))
	for _, it := range got {
		paths = append(paths, it.Path)
	}
	sort.Strings(paths)
	// src/a.go: src/** ignores the contents, but the directory itself stays
	// unignored so !src/*.go re-includes it.
	// src/sub/b.go: excluded — the negation only reaches direct children.
	// docs/generated/c.go: excluded — the root-relative rule matches its own
	// subtree (the control).
	// nested/docs/generated/d.go: included — an internal slash keeps the rule
	// relative to the .gitignore location, so the unrelated subtree survives.
	want := []string{".gitignore", "nested/docs/generated/d.go", "src/a.go"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}
}

func TestProvider_Enumerate_GitignoreAncestorExceptions(test *testing.T) {
	cases := []struct {
		name     string
		patterns string
		path     string
	}{
		{
			name:     "slashless globstar negation",
			patterns: "cache\n!**cache\n",
			path:     "src/cache/a.go",
		},
		{
			name:     "path wildcard with nested exception",
			patterns: "src/*\n!src/ok/\n!src/ok/nested/*.go\n",
			path:     "src/ok/nested/a.go",
		},
		{
			name:     "anchored wildcard with nested exception",
			patterns: "/*\n!/.gitignore\n!/src/\n!*.go\n",
			path:     "src/inner/a.go",
		},
	}
	for _, testCase := range cases {
		test.Run(testCase.name, func(test *testing.T) {
			repo := initTestRepo(test)
			writeFile(test, repo, ".gitignore", []byte(testCase.patterns))
			writeFile(test, repo, testCase.path, []byte("package fixture\n"))
			gitCommit(test, repo, "init")

			items, err := NewProvider(repo, nil, nil, 0).Enumerate(context.Background())
			if err != nil {
				test.Fatalf("Enumerate: %v", err)
			}
			paths := make([]string, 0, len(items))
			for _, item := range items {
				paths = append(paths, item.Path)
			}
			sort.Strings(paths)
			want := []string{".gitignore", testCase.path}
			if !reflect.DeepEqual(paths, want) {
				test.Errorf("paths = %v, want %v", paths, want)
			}
		})
	}
}

func TestProvider_Enumerate_NestedIgnoreBeforeAndAfterStaging(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "tracked.log", []byte("tracked before ignore rules\n"))
	writeFile(t, repo, "pkg/hidden.go", []byte("package fixture\n"))
	gitCommit(t, repo, "track log")
	writeFile(t, repo, ".gitignore", []byte("generated/\n*.log\n"))
	writeFile(t, repo, "pkg/.gitignore", []byte("!generated/\nhidden.go\n"))
	gitCommit(t, repo, "set ignore rules")
	for _, name := range []string{"pkg/generated/a.go", "generated/ignored.go", "vendor/blocked.go"} {
		writeFile(t, repo, name, []byte("package fixture\n"))
	}
	want := []string{".gitignore", "pkg/.gitignore", "pkg/generated/a.go"}
	check := func() {
		t.Helper()
		if got := enumeratePaths(t, repo, nil); !reflect.DeepEqual(got, want) {
			t.Errorf("enumerated %q, want %q", got, want)
		}
	}
	check()
	if output, err := exec.Command("git", "-C", repo, "add", "--", "pkg/generated/a.go").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, output)
	}
	check()
}

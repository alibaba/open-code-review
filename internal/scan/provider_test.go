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
	"runtime"
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

func TestFilterByPaths(t *testing.T) {
	all := []string{
		"cmd/main.go",
		"internal/agent/agent.go",
		"internal/agent/fullscan.go",
		"internal/scan/provider.go",
		"README.md",
	}
	tests := []struct {
		name  string
		paths []string
		want  []string
	}{
		{"exact file", []string{"README.md"}, []string{"README.md"}},
		{"dir prefix", []string{"internal/agent"}, []string{"internal/agent/agent.go", "internal/agent/fullscan.go"}},
		{"multi", []string{"cmd/main.go", "internal/scan"}, []string{"cmd/main.go", "internal/scan/provider.go"}},
		{"prefix not at boundary", []string{"internal/age"}, nil},
		{"no match", []string{"does/not/exist"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterByPaths(all, tt.paths)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterByPaths() = %v, want %v", got, tt.want)
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
		filepath.Join(".", "internal", "scan") + string(filepath.Separator),
	}, nil, 0)
	want := []string{"internal/agent", "cmd", "internal/diff", "a/b", "internal/scan"}
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
	writeFile(t, repo, ".gitignore", []byte("ignored.txt\n"))
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

	want := []string{".gitignore", "main.go", "pkg/util.go"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("paths = %v, want %v (ignored.txt must be filtered by .gitignore, node_modules/* by ExcludedDirs)", paths, want)
	}
}

// TestProvider_Enumerate_RespectsContextCancellation guards the
// per-iteration ctx.Err() check that was previously missing.
func TestProvider_Enumerate_RespectsContextCancellation(t *testing.T) {
	repo := initTestRepo(t)
	for i := 0; i < 30; i++ {
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

// TestNormalizeScanPath pins the selector spellings that used to survive
// normalization and then match no `git ls-files` entry. A "" result means
// "repository root", which the caller turns into "no path filter".
func TestNormalizeScanPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{".", ""},
		{"./", ""},
		{"././", ""},
		{"   .   ", ""},
		{"internal/..", ""},
		{"./internal/agent/", "internal/agent"},
		{"internal/agent/.", "internal/agent"},
		{"a//b", "a/b"},
		{"cmd", "cmd"},
		{"   internal/diff   ", "internal/diff"},
		{"./a/b/../c", "a/c"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := normalizeScanPath(tt.in); got != tt.want {
				t.Errorf("normalizeScanPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}

	// On Windows a backslash is the separator, so a native spelling must
	// normalize like its slash form. Elsewhere it is a legal filename
	// character and has to be left alone.
	native := []struct{ in, want string }{
		{filepath.Join(".", "internal", "scan"), "internal/scan"},
		{filepath.Join(".", "internal", "scan") + string(filepath.Separator), "internal/scan"},
		{filepath.FromSlash("./pkg/"), "pkg"},
	}
	if runtime.GOOS == "windows" {
		native = append(native,
			struct{ in, want string }{`.\internal\scan`, "internal/scan"},
			struct{ in, want string }{`.\`, ""},
		)
	}
	for _, tt := range native {
		t.Run(tt.in, func(t *testing.T) {
			if got := normalizeScanPath(tt.in); got != tt.want {
				t.Errorf("normalizeScanPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestNewProvider_RootSelectorsDropFilter covers the spellings users reach for
// when they mean "the whole repository". A leftover "." or "" selector matches
// no `git ls-files` entry, so it used to turn a full scan into a silent no-op.
func TestNewProvider_RootSelectorsDropFilter(t *testing.T) {
	for _, sel := range []string{".", "./", "././", "internal/..", "   .   "} {
		t.Run(sel, func(t *testing.T) {
			if got := NewProvider("/tmp/repo", []string{sel}, nil, 0).paths; len(got) != 0 {
				t.Errorf("NewProvider(%q).paths = %q, want no filter", sel, got)
			}
		})
	}

	// A root selector mixed with a real one must not leave a stray empty entry
	// behind, which would otherwise shadow the real selector.
	p := NewProvider("/tmp/repo", []string{"./", "cmd"}, nil, 0)
	if want := []string{"cmd"}; !reflect.DeepEqual(p.paths, want) {
		t.Errorf("paths = %q, want %q", p.paths, want)
	}
}

func TestProvider_Enumerate_RootPathSelector(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "a.go", []byte("package a\n"))
	writeFile(t, repo, "pkg/b.go", []byte("package pkg\n"))
	gitCommit(t, repo, "init")

	want := []string{"a.go", "pkg/b.go"}
	for _, sel := range []string{".", "./", filepath.Join(".", "")} {
		t.Run(sel, func(t *testing.T) {
			got, err := NewProvider(repo, []string{sel}, nil, 0).Enumerate(context.Background())
			if err != nil {
				t.Fatalf("Enumerate: %v", err)
			}
			paths := make([]string, 0, len(got))
			for _, it := range got {
				paths = append(paths, it.Path)
			}
			sort.Strings(paths)
			if !reflect.DeepEqual(paths, want) {
				t.Errorf("paths = %v, want %v", paths, want)
			}
		})
	}
}

// TestProvider_Enumerate_NativeSeparatorPathSelector covers the same class of
// miss for native separators. On Windows a selector like `.\pkg` used to keep
// its leading `.\` and match no listed file.
func TestProvider_Enumerate_NativeSeparatorPathSelector(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "a.go", []byte("package a\n"))
	writeFile(t, repo, "pkg/b.go", []byte("package pkg\n"))
	gitCommit(t, repo, "init")

	for _, sel := range []string{
		filepath.Join(".", "pkg"),
		filepath.Join(".", "pkg") + string(filepath.Separator),
		filepath.FromSlash("./pkg/"),
	} {
		t.Run(sel, func(t *testing.T) {
			got, err := NewProvider(repo, []string{sel}, nil, 0).Enumerate(context.Background())
			if err != nil {
				t.Fatalf("Enumerate: %v", err)
			}
			paths := make([]string, 0, len(got))
			for _, it := range got {
				paths = append(paths, it.Path)
			}
			sort.Strings(paths)
			// a.go must stay excluded: the selector resolves to "pkg", not the
			// repository root.
			if want := []string{"pkg/b.go"}; !reflect.DeepEqual(paths, want) {
				t.Errorf("paths = %v, want %v", paths, want)
			}
		})
	}
}

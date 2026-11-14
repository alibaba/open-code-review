// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/vcs"
)

type scanFileSource struct {
	list func(context.Context, vcs.ContentRef) ([]string, error)
	read func(context.Context, vcs.ContentRef, string, func(io.Reader) error) error
}

func (s scanFileSource) ListFiles(ctx context.Context, ref vcs.ContentRef) ([]string, error) {
	return s.list(ctx, ref)
}

func (s scanFileSource) ReadFile(ctx context.Context, ref vcs.ContentRef, path string, consume func(io.Reader) error) error {
	return s.read(ctx, ref, path, consume)
}

func TestProvider_EnumerateWithSource(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("PATH", "") // The injected source must not require Git.
	for _, path := range []string{"pkg/main.go", "pkg/image.bin", "pkg/growing.go", "other.go"} {
		writeFile(t, repo, path, []byte("disk\n"))
	}
	writeFile(t, repo, "pkg/large.go", []byte(strings.Repeat("x", 64)))
	if err := os.Mkdir(filepath.Join(repo, "pkg", "directory.go"), 0o755); err != nil {
		t.Fatal(err)
	}
	var reads []string
	source := scanFileSource{
		list: func(_ context.Context, ref vcs.ContentRef) ([]string, error) {
			if !ref.IsWorkspace() {
				t.Errorf("scan listed revision %q", ref.Revision)
			}
			return []string{"pkg/main.go", "pkg/image.bin", "pkg/growing.go", "pkg/large.go", "pkg/directory.go", "other.go"}, nil
		},
		read: func(_ context.Context, ref vcs.ContentRef, path string, consume func(io.Reader) error) error {
			if !ref.IsWorkspace() {
				t.Errorf("scan read revision %q", ref.Revision)
			}
			reads = append(reads, path)
			contents := map[string]string{
				"pkg/main.go":    "package source\n\n",
				"pkg/image.bin":  "a\x00b",
				"pkg/growing.go": strings.Repeat("x", 64),
			}
			return consume(strings.NewReader(contents[path]))
		},
	}
	items, err := NewProviderWithSource(repo, []string{"./pkg/"}, source, 32).Enumerate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := itemPaths(items); !reflect.DeepEqual(got, []string{"pkg/image.bin", "pkg/main.go"}) {
		t.Fatalf("paths = %v", got)
	}
	if items[0].Content != "package source\n\n" || items[0].LineCount != 2 {
		t.Errorf("source content = %+v", items[0])
	}
	if !items[1].IsBinary || items[1].Content != "" {
		t.Errorf("binary placeholder = %+v", items[1])
	}
	if !reflect.DeepEqual(reads, []string{"pkg/main.go", "pkg/image.bin", "pkg/growing.go"}) {
		t.Errorf("reads = %v", reads)
	}
}

func TestProvider_SourceErrors(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "main.go", []byte("disk\n"))
	failure := errors.New("source failed")
	source := scanFileSource{
		list: func(context.Context, vcs.ContentRef) ([]string, error) { return nil, failure },
	}
	if _, err := NewProviderWithSource(repo, nil, source, 0).Enumerate(t.Context()); !errors.Is(err, failure) {
		t.Fatalf("listing error = %v", err)
	}
	source.list = func(context.Context, vcs.ContentRef) ([]string, error) { return []string{"main.go"}, nil }
	source.read = func(context.Context, vcs.ContentRef, string, func(io.Reader) error) error { return failure }
	if items, err := NewProviderWithSource(repo, nil, source, 0).Enumerate(t.Context()); err != nil || len(items) != 0 {
		t.Fatalf("failed read = %v, %v", items, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	source.read = func(context.Context, vcs.ContentRef, string, func(io.Reader) error) error {
		cancel()
		return context.Canceled
	}
	if _, err := NewProviderWithSource(repo, nil, source, 0).Enumerate(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read = %v", err)
	}
}

func TestReadScanContent_BoundsReads(t *testing.T) {
	text := strings.NewReader(strings.Repeat("x", 100))
	if _, _, err := readScanContent(text, 32); !errors.Is(err, errScanFileTooLarge) {
		t.Fatalf("oversize error = %v", err)
	}
	if text.Len() != 67 {
		t.Errorf("read %d bytes, want cap + 1", 100-text.Len())
	}
	binary := strings.NewReader("\x00" + strings.Repeat("x", 2*binarySniffWindow))
	if content, isBinary, err := readScanContent(binary, DefaultMaxFileSizeBytes); err != nil || !isBinary || len(content) != 0 {
		t.Fatalf("binary read = %q, %v, %v", content, isBinary, err)
	}
	if binary.Len() != binarySniffWindow+1 {
		t.Errorf("binary sniff read %d bytes", 2*binarySniffWindow+1-binary.Len())
	}
	if content, isBinary, err := readScanContent(strings.NewReader("exact"), 5); err != nil || isBinary || string(content) != "exact" {
		t.Fatalf("exact cap = %q, %v, %v", content, isBinary, err)
	}
}

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

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

// TestProvider_Enumerate_OversizeSkip covers the size-cap branch: a file larger
// than maxFileSizeBytes is skipped with a warning while smaller files remain.
func TestProvider_Enumerate_OversizeSkip(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "small.go", []byte("package s\n"))
	writeFile(t, repo, "big.go", []byte("package big // "+string(make([]byte, 200))+"\n"))
	gitCommit(t, repo, "init")

	got, err := NewProvider(repo, nil, nil, 32).Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	paths := itemPaths(got)
	if contains(paths, "big.go") {
		t.Errorf("big.go should be skipped (exceeds size cap), got %v", paths)
	}
	if !contains(paths, "small.go") {
		t.Errorf("small.go should be present, got %v", paths)
	}
}

// TestProvider_Enumerate_NonRegularSkip covers the !IsRegular branch: a symlink
// tracked by git is enumerated but skipped because it is not a regular file.
func TestProvider_Enumerate_NonRegularSkip(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "real.go", []byte("package r\n"))
	if err := os.Symlink("real.go", filepath.Join(repo, "link.go")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	gitCommit(t, repo, "init")

	got, err := NewProvider(repo, nil, nil, 0).Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	paths := itemPaths(got)
	if contains(paths, "link.go") {
		t.Errorf("symlink link.go must be skipped (non-regular), got %v", paths)
	}
	if !contains(paths, "real.go") {
		t.Errorf("real.go should be present, got %v", paths)
	}
}

// TestProvider_Enumerate_SniffError covers the binary-sniff error branch: a file
// that cannot be opened for sniffing is skipped with a warning.
func TestProvider_Enumerate_SniffError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permission checks")
	}
	repo := initTestRepo(t)
	writeFile(t, repo, "ok.go", []byte("package ok\n"))
	writeFile(t, repo, "locked.go", []byte("package locked\n"))
	gitCommit(t, repo, "init")
	// Make locked.go unreadable so isBinaryFile's os.Open fails.
	if err := os.Chmod(filepath.Join(repo, "locked.go"), 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(repo, "locked.go"), 0o644) })

	got, err := NewProvider(repo, nil, nil, 0).Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	paths := itemPaths(got)
	if contains(paths, "locked.go") {
		t.Errorf("unreadable locked.go must be skipped, got %v", paths)
	}
	if !contains(paths, "ok.go") {
		t.Errorf("ok.go should be present, got %v", paths)
	}
}

// TestIsBinaryFile covers detection plus the open-error branch.
func TestIsBinaryFile(t *testing.T) {
	dir := t.TempDir()
	text := filepath.Join(dir, "text.txt")
	if err := os.WriteFile(text, []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("write text: %v", err)
	}
	bin := filepath.Join(dir, "bin.dat")
	if err := os.WriteFile(bin, []byte{'a', 0x00, 'b'}, 0o644); err != nil {
		t.Fatalf("write bin: %v", err)
	}
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatalf("write empty: %v", err)
	}

	t.Run("text is not binary", func(t *testing.T) {
		b, err := isBinaryFile(text)
		if err != nil || b {
			t.Errorf("isBinaryFile(text) = %v, %v; want false, nil", b, err)
		}
	})
	t.Run("NUL byte is binary", func(t *testing.T) {
		b, err := isBinaryFile(bin)
		if err != nil || !b {
			t.Errorf("isBinaryFile(bin) = %v, %v; want true, nil", b, err)
		}
	})
	t.Run("empty is not binary", func(t *testing.T) {
		b, err := isBinaryFile(empty)
		if err != nil || b {
			t.Errorf("isBinaryFile(empty) = %v, %v; want false, nil", b, err)
		}
	})
	t.Run("missing file errors", func(t *testing.T) {
		if _, err := isBinaryFile(filepath.Join(dir, "nope")); err == nil {
			t.Error("expected error for missing file")
		}
	})
}

func itemPaths(items []model.ScanItem) []string {
	paths := make([]string, 0, len(items))
	for _, it := range items {
		paths = append(paths, it.Path)
	}
	sort.Strings(paths)
	return paths
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

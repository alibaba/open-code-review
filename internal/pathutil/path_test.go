package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWithinBase(t *testing.T) {
	base := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}

	cases := []struct {
		name   string
		target string
		want   bool
	}{
		{name: "base", target: base, want: true},
		{name: "child", target: filepath.Join(base, "dir", "file.txt"), want: true},
		{name: "parent", target: filepath.Dir(base), want: false},
		{name: "sibling with prefix", target: base + "-other", want: false},
		{name: "cleaned traversal", target: filepath.Join(base, "..", filepath.Base(base)+"-other"), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WithinBase(base, tc.target); got != tc.want {
				t.Fatalf("WithinBase(%q, %q) = %v, want %v", base, tc.target, got, tc.want)
			}
		})
	}
}

func TestCanonicalPathResolvesSymlink(t *testing.T) {
	realDir := t.TempDir()
	linkParent := t.TempDir()
	linkPath := filepath.Join(linkParent, "repo-link")
	if err := os.Symlink(realDir, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	got, err := CanonicalPath(linkPath)
	if err != nil {
		t.Fatalf("CanonicalPath: %v", err)
	}
	want, err := filepath.EvalSymlinks(realDir)
	if err != nil {
		t.Fatalf("EvalSymlinks realDir: %v", err)
	}
	if got != want {
		t.Fatalf("CanonicalPath(%q) = %q, want %q", linkPath, got, want)
	}
}

func TestCanonicalPath_NonExistentPath(t *testing.T) {
	_, err := CanonicalPath(filepath.Join(t.TempDir(), "does", "not", "exist"))
	if err == nil {
		t.Fatal("expected error for non-existent path, got nil")
	}
}

func TestCanonicalPath_RelativePath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(oldWd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	got, err := CanonicalPath("test.txt")
	if err != nil {
		t.Fatalf("CanonicalPath: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("result should be absolute, got %q", got)
	}
}

func TestCanonicalPath_NestedSymlink(t *testing.T) {
	realDir := t.TempDir()
	realFile := filepath.Join(realDir, "file.txt")
	if err := os.WriteFile(realFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	linkDir := t.TempDir()
	link1 := filepath.Join(linkDir, "link1")
	if err := os.Symlink(realDir, link1); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	link2 := filepath.Join(linkDir, "link2")
	if err := os.Symlink(link1, link2); err != nil {
		t.Skipf("nested symlink not supported: %v", err)
	}

	got, err := CanonicalPath(filepath.Join(link2, "file.txt"))
	if err != nil {
		t.Fatalf("CanonicalPath: %v", err)
	}
	want, _ := filepath.EvalSymlinks(realFile)
	if got != want {
		t.Errorf("CanonicalPath through nested symlinks = %q, want %q", got, want)
	}
}

func TestWithinBase_AdditionalCases(t *testing.T) {
	cases := []struct {
		name   string
		base   string
		target string
		want   bool
	}{
		{name: "same path", base: "/a/b", target: "/a/b", want: true},
		{name: "deep child", base: "/a/b", target: "/a/b/c/d/e/f", want: true},
		{name: "double dotdot escape", base: "/a/b/c", target: "/a/b/c/../../x", want: false},
		{name: "dotdot only", base: "/a/b", target: "/a", want: false},
		{name: "root base with child", base: "/", target: "/anything", want: true},
		{name: "empty relative after clean", base: "/a/b", target: "/a/b/./c", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WithinBase(tc.base, tc.target); got != tc.want {
				t.Errorf("WithinBase(%q, %q) = %v, want %v", tc.base, tc.target, got, tc.want)
			}
		})
	}
}

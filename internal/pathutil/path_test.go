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

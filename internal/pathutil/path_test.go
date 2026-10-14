// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package pathutil

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
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

func TestWithinBase_SameFileFallback(t *testing.T) {
	dir := t.TempDir()
	baseDir := filepath.Join(dir, "base")
	targetDir := filepath.Join(dir, "target")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}

	base := filepath.Join(baseDir, "rules.md")
	target := filepath.Join(targetDir, "rules.md")
	if err := os.WriteFile(base, []byte("same file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(base, target); err != nil {
		t.Skipf("hard links not supported: %v", err)
	}

	if !WithinBase(base, target) {
		t.Fatalf("WithinBase(%q, %q) = false, want true via SameFile fallback", base, target)
	}
}

func TestWithinBase_CaseInsensitiveVariant(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "Repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "rules.md"), []byte("legit"), 0o644); err != nil {
		t.Fatal(err)
	}

	// This only reproduces on a case-insensitive filesystem. Detect that first so
	// the test can pass on case-sensitive CI runners.
	variantDir := filepath.Join(dir, "repo")
	lowerInfo, err := os.Stat(variantDir)
	if err != nil || !os.SameFile(mustStat(t, repo), lowerInfo) {
		t.Skip("filesystem is case-sensitive")
	}

	variant := filepath.Join(variantDir, "rules.md")
	if !WithinBase(repo, variant) {
		t.Fatalf("WithinBase(%q, %q) = false, want true for case variant", repo, variant)
	}
}

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
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
		// filepath.Rel cannot relate a relative base to an absolute target,
		// so WithinBase must fall through its error branch and report false.
		{name: "rel error on mixed abs/rel", base: "relative", target: "/absolute", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WithinBase(tc.base, tc.target); got != tc.want {
				t.Errorf("WithinBase(%q, %q) = %v, want %v", tc.base, tc.target, got, tc.want)
			}
		})
	}
}

func TestPatternBackslashIssue(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		want    bool
	}{
		{name: "windows separators", pattern: `src\gen\*`, want: true},
		{name: "windows separators without wildcard", pattern: `src\gen\file.go`, want: true},
		{name: "separator before a wildcard", pattern: `node_modules\**`, want: true},
		{name: "separator before wildcards on both sides", pattern: `src\**\*.log`, want: true},
		{name: "leading separator before a wildcard", pattern: `**\*.log`, want: true},
		{name: "escaped star cannot name a file on windows", pattern: `src\*`, want: true},
		{name: "escaped question mark cannot name a file on windows", pattern: `src\?one.go`, want: true},
		{name: "backslash before slash", pattern: `src\/gen`, want: true},
		{name: "dangling backslash", pattern: `src\`, want: true},
		{name: "mixed with legitimate escapes", pattern: `src\gen\[x\]\*.go`, want: true},
		{name: "slash separators", pattern: "src/gen/*", want: false},
		{name: "escaped brackets stay silent", pattern: `src/\[generated\]/file.go`, want: false},
		{name: "escaped brackets beside a wildcard stay silent", pattern: `src/\[generated\]/*.go`, want: false},
		{name: "escaped braces stay silent", pattern: `src/\{a,b\}/x`, want: false},
		{name: "escaped backslash stays silent", pattern: `src\\gen`, want: false},
		{name: "no backslash at all", pattern: "**/*_test.go", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := patternBackslashIssue(tc.pattern); got != tc.want {
				t.Errorf("patternBackslashIssue(%q) = %v, want %v", tc.pattern, got, tc.want)
			}
		})
	}
}

func TestWarnPatternBackslashes(t *testing.T) {
	unsafe := "src\\gen\x1b[2J\nforged"
	patterns := []string{
		`src\gen\*`,
		`node_modules\**`,
		unsafe,
		"src/gen/*",
		`src/\[generated\]/file.go`,
	}

	var buf bytes.Buffer
	WarnPatternBackslashes(&buf, "windows", "--exclude", patterns)
	got := buf.String()
	for _, want := range []string{`src\gen\*`, `node_modules\**`, unsafe} {
		if !strings.Contains(got, strconv.Quote(want)) {
			t.Errorf("expected a warning naming %q, got %q", want, got)
		}
	}
	if n := strings.Count(got, "[ocr] WARNING:"); n != 3 {
		t.Errorf("expected exactly 3 warning lines, got %d: %q", n, got)
	}
	if strings.ContainsRune(got, '\x1b') || strings.Count(got, "\n") != 3 {
		t.Errorf("warning contains raw terminal controls or extra lines: %q", got)
	}

	buf.Reset()
	WarnPatternBackslashes(&buf, "linux", "--exclude", patterns)
	if buf.Len() != 0 {
		t.Errorf("expected no warning off windows, got %q", buf.String())
	}
}

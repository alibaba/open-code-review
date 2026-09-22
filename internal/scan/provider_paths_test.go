// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

// TestProvider_Enumerate_PreservesWhitespaceInTrackedPaths covers issue #1493.
//
// `git ls-files -z` exists so that a pathname needs no escaping: every byte
// between two NULs is the name. Trimming those records treats leading and
// trailing spaces as record formatting, which they are not. The file then
// drops out of the scan silently — the trimmed name does not exist, Lstat
// fails with a warning, and the loop moves on — so a full scan reports success
// while having reviewed fewer files than the repository holds.
//
// Both ends are checked because a trailing space is the easier one to lose: it
// survives `filepath.Clean`, and is invisible in any log line that follows.
func TestProvider_Enumerate_PreservesWhitespaceInTrackedPaths(t *testing.T) {
	requireWhitespaceFilenames(t)

	repo := initTestRepo(t)
	writeFile(t, repo, " leading.go", []byte("package p\n"))
	writeFile(t, repo, "trailing.go ", []byte("package q\n"))
	writeFile(t, repo, "plain.go", []byte("package r\n"))
	gitCommit(t, repo, "init")

	// nil runner and injected runner are separate code paths in gitLs, and
	// every real `ocr scan` takes the injected one.
	for _, runner := range []*gitcmd.Runner{nil, gitcmd.New(2)} {
		got, err := NewProvider(repo, nil, runner, 0).Enumerate(context.Background())
		if err != nil {
			t.Fatalf("Enumerate: %v", err)
		}
		paths := make([]string, 0, len(got))
		for _, it := range got {
			paths = append(paths, it.Path)
		}
		sort.Strings(paths)
		want := []string{" leading.go", "plain.go", "trailing.go "}
		if len(paths) != len(want) {
			t.Fatalf("enumerated %q, want %q", paths, want)
		}
		for i := range want {
			if paths[i] != want[i] {
				t.Errorf("path %d = %q, want %q", i, paths[i], want[i])
			}
		}
	}
}

// TestProvider_Enumerate_StderrWarningIsNotParsedAsAPath covers the second half
// of the same defect. gitLs took stdout only on its fallback branch and said so
// in a comment, then took stdout+stderr on the branch every real scan uses.
//
// An unreadable directory makes `git ls-files --others` warn and still exit 0,
// which is the shape that matters: on a failure gitLs returns the error and
// parses nothing. With -z there is no line structure to resynchronise on, so
// the warning does not arrive as a stray record — it is glued to the front of
// the first real pathname, and that file is renamed out of the scan.
func TestProvider_Enumerate_StderrWarningIsNotParsedAsAPath(t *testing.T) {
	requireWhitespaceFilenames(t)

	repo := initTestRepo(t)
	writeFile(t, repo, "kept.go", []byte("package p\n"))
	unreadable := filepath.Join(repo, "unreadable")
	if err := os.MkdirAll(unreadable, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unreadable, "hidden.go"), []byte("package q\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	// Restore the mode so t.TempDir's cleanup can remove the tree.
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })
	// Ask the filesystem whether the mode took, rather than inferring it from
	// the OS. Root is exempt from the mode, and a filesystem mounted without
	// permission support ignores it outright -- in either case git reads the
	// directory happily, writes no warning, and the test would assert against
	// a stream that has nothing wrong with it.
	if _, err := os.ReadDir(unreadable); err == nil {
		t.Skip("this environment does not enforce directory modes, so git has nothing to warn about")
	}

	got, err := NewProvider(repo, nil, gitcmd.New(2), 0).Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	for _, it := range got {
		if it.Path != filepath.Clean(it.Path) || it.Path == "" {
			t.Errorf("enumerated a path git did not emit: %q", it.Path)
		}
	}
	if len(got) != 1 || got[0].Path != "kept.go" {
		paths := make([]string, 0, len(got))
		for _, it := range got {
			paths = append(paths, it.Path)
		}
		t.Errorf("enumerated %q, want exactly [\"kept.go\"]", paths)
	}
}

func requireWhitespaceFilenames(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// Win32 strips trailing spaces and dots from a name before it reaches
		// the filesystem, and has no POSIX directory modes. The parsing under
		// test is platform-independent.
		t.Skip("windows normalises away the filenames these fixtures depend on")
	}
}

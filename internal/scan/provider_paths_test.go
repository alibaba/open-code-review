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

// enumeratePaths returns the sorted paths NewProvider selects for the given
// --path selectors, so a case reads as selector in, file list out.
func enumeratePaths(t *testing.T, repo string, selectors []string) []string {
	t.Helper()
	got, err := NewProvider(repo, selectors, gitcmd.New(2), 0).Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate(%q): %v", selectors, err)
	}
	paths := make([]string, 0, len(got))
	for _, it := range got {
		paths = append(paths, it.Path)
	}
	sort.Strings(paths)
	return paths
}

func pathSelectorRepo(t *testing.T) string {
	t.Helper()
	repo := initTestRepo(t)
	writeFile(t, repo, "main.go", []byte("package main\n"))
	writeFile(t, repo, filepath.Join("internal", "scan", "provider.go"), []byte("package scan\n"))
	gitCommit(t, repo, "init")
	return repo
}

// TestProvider_Enumerate_RootSelectorScansRepository pins the root selector.
//
// `git ls-files` prints repository-relative names such as `main.go`, never
// `./main.go`, and filterByPaths matches an exact path or a `<selector>/`
// prefix. A selector left as "." therefore matches nothing, so `ocr scan
// --path .` reported an empty selection and exited 0 while the same repository
// scanned fine with --path omitted. The failure is silent, which is what makes
// it costly: a review that covered no files still looks like a clean run.
func TestProvider_Enumerate_RootSelectorScansRepository(t *testing.T) {
	repo := pathSelectorRepo(t)
	want := enumeratePaths(t, repo, nil)
	if len(want) == 0 {
		t.Fatal("baseline scan enumerated no files")
	}

	for _, selector := range []string{".", "./", " . ", "././"} {
		got := enumeratePaths(t, repo, []string{selector})
		if len(got) != len(want) {
			t.Errorf("--path %q enumerated %q, want the full scan %q", selector, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("--path %q file %d = %q, want %q", selector, i, got[i], want[i])
			}
		}
	}
}

// TestProvider_Enumerate_RootSelectorWidensANarrowerOne pins the union rule: a
// root selector covers the whole repository, so a subdirectory named beside it
// must not narrow the scan back down.
func TestProvider_Enumerate_RootSelectorWidensANarrowerOne(t *testing.T) {
	repo := pathSelectorRepo(t)
	want := enumeratePaths(t, repo, nil)

	got := enumeratePaths(t, repo, []string{"internal/scan", "."})
	if len(got) != len(want) {
		t.Fatalf("enumerated %q, want the full scan %q", got, want)
	}
}

// TestProvider_Enumerate_DotSlashPrefixSelectsSubdirectory pins the normalization
// order. filepath.ToSlash has to run before the "./" trim, or a selector that
// only becomes "./dir" during conversion keeps its prefix and matches nothing.
// This case exercises the order on any platform; the backslash spelling itself
// is covered below, on Windows only.
func TestProvider_Enumerate_DotSlashPrefixSelectsSubdirectory(t *testing.T) {
	repo := pathSelectorRepo(t)

	for _, selector := range []string{"./internal/scan", "internal/scan/", "./internal/scan/"} {
		got := enumeratePaths(t, repo, []string{selector})
		want := []string{filepath.ToSlash(filepath.Join("internal", "scan", "provider.go"))}
		if len(got) != len(want) || got[0] != want[0] {
			t.Errorf("--path %q enumerated %q, want %q", selector, got, want)
		}
	}
}

// TestProvider_Enumerate_WindowsBackslashSelector pins the Windows spelling.
// filepath.ToSlash only rewrites separators on Windows, and a backslash is a
// legal filename byte on POSIX, so this case cannot run anywhere else.
func TestProvider_Enumerate_WindowsBackslashSelector(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("filepath.ToSlash only rewrites separators on Windows")
	}
	repo := pathSelectorRepo(t)

	got := enumeratePaths(t, repo, []string{`.\internal\scan`})
	want := []string{"internal/scan/provider.go"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf(`--path '.\internal\scan' enumerated %q, want %q`, got, want)
	}
}

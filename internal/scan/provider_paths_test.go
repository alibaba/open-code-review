// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
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
	got, err := enumeratePathsErr(t, repo, selectors)
	if err != nil {
		t.Fatalf("Enumerate(%q): %v", selectors, err)
	}
	return got
}

// enumeratePathsErr is enumeratePaths for selectors expected to fail, such as
// a path that names nothing in the working tree.
func enumeratePathsErr(t *testing.T, repo string, selectors []string) ([]string, error) {
	t.Helper()
	got, err := NewProvider(repo, selectors, gitcmd.New(2), 0).Enumerate(context.Background())
	paths := make([]string, 0, len(got))
	for _, it := range got {
		paths = append(paths, it.Path)
	}
	sort.Strings(paths)
	return paths, err
}

// fsIsCaseInsensitive reports whether the filesystem holding dir folds case,
// by asking it: one probe name spelled two ways must resolve to one file.
// Windows and default macOS answer true; a case-sensitive filesystem answers
// false. Tests that depend on either rule skip on the other side rather than
// assume an OS, because a case-sensitive macOS volume or a case-sensitive
// Windows directory would silently invert them.
func fsIsCaseInsensitive(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "case-probe")
	if err := os.WriteFile(probe, nil, 0o644); err != nil {
		t.Fatalf("write case probe: %v", err)
	}
	// Removed with the helper, before a test can enumerate: a probe left in
	// the repository would be listed by git ls-files and fail the selection
	// expectations it exists to guard.
	defer func() { _ = os.Remove(probe) }()
	_, err := os.Stat(filepath.Join(dir, "CASE-PROBE"))
	return err == nil
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

// TestProvider_Enumerate_UnknownPathIsAnError pins the difference between an
// empty selection and a mistyped selector. `--path` names a path, not a search
// pattern, so a selector the filesystem cannot resolve must fail loudly: the
// old silent "No files changed." exit 0 made a review that covered nothing
// look like a clean run.
func TestProvider_Enumerate_UnknownPathIsAnError(t *testing.T) {
	repo := pathSelectorRepo(t)

	// `internal/sc` is merely a prefix of `internal/scan/provider.go`, and
	// `internal/scan/provider` lacks the extension; neither names a path.
	for _, selector := range []string{"does/not/exist", "internal/sc", "internal/scan/provider"} {
		got, err := enumeratePathsErr(t, repo, []string{selector})
		if err == nil {
			t.Errorf("--path %q enumerated %q, want an error for a path that names nothing", selector, got)
			continue
		}
		if !strings.Contains(err.Error(), selector) {
			t.Errorf("--path %q error %q should name the selector", selector, err)
		}
		if !strings.Contains(err.Error(), "does not exist in the working tree") {
			t.Errorf("--path %q error %q should say the path does not exist", selector, err)
		}
	}

	// A path that exists but has nothing reviewable stays a clean skip: an
	// empty directory holds no entries, and git tracks files, not directories.
	if err := os.MkdirAll(filepath.Join(repo, "empty"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got, err := enumeratePathsErr(t, repo, []string{"empty"}); err != nil || len(got) != 0 {
		t.Errorf("--path empty over an existing empty directory = %q, %v; want no files and no error", got, err)
	}
}

// TestProvider_Enumerate_CaseVariantSelectorResolvesOnCaseInsensitiveFS is
// the Windows/macOS repair: `--path SRC` names `src/` because the filesystem
// resolves the spelling, so the scan selects the tracked spelling instead of
// reporting "No files changed.".
func TestProvider_Enumerate_CaseVariantSelectorResolvesOnCaseInsensitiveFS(t *testing.T) {
	repo := initTestRepo(t)
	if !fsIsCaseInsensitive(t, repo) {
		t.Skip("case-variant spellings name different paths on this filesystem")
	}
	writeFile(t, repo, "main.go", []byte("package main\n"))
	writeFile(t, repo, filepath.Join("SRC", "Keep.go"), []byte("package keep\n"))
	gitCommit(t, repo, "init")

	want := []string{"SRC/Keep.go"}
	for _, selector := range []string{"src", "SRC", "src/keep.go", "SRC/KEEP.GO"} {
		got := enumeratePaths(t, repo, []string{selector})
		if len(got) != len(want) || got[0] != want[0] {
			t.Errorf("--path %q enumerated %q, want %q", selector, got, want)
		}
	}

	// Resolution is not fuzziness: a selector the filesystem cannot resolve is
	// still an error, not an empty selection.
	if got, err := enumeratePathsErr(t, repo, []string{"src/keep"}); err == nil {
		t.Errorf("--path src/keep enumerated %q, want an error", got)
	}
}

// TestProvider_Enumerate_ExactSpellingWinsOnCaseSensitiveFS pins the rule
// where case is meaningful: `src` and `SRC` are two paths, the spelled path
// wins, and a spelling that names neither is an error rather than a folded
// match of both -- widening to both would silently grow the review and its
// cost.
func TestProvider_Enumerate_ExactSpellingWinsOnCaseSensitiveFS(t *testing.T) {
	repo := initTestRepo(t)
	if fsIsCaseInsensitive(t, repo) {
		t.Skip("case-variant spellings are one path on this filesystem")
	}
	writeFile(t, repo, "src/lower.go", []byte("package lower\n"))
	writeFile(t, repo, filepath.Join("SRC", "Keep.go"), []byte("package keep\n"))
	gitCommit(t, repo, "init")

	for _, tc := range []struct{ selector, want string }{
		{"src", "src/lower.go"},
		{"SRC", "SRC/Keep.go"},
	} {
		got := enumeratePaths(t, repo, []string{tc.selector})
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("--path %q enumerated %q, want [%q]", tc.selector, got, tc.want)
		}
	}

	if got, err := enumeratePathsErr(t, repo, []string{"srC"}); err == nil {
		t.Errorf("--path srC enumerated %q, want an error, not a folded match of both directories", got)
	}
}

// TestProvider_Enumerate_CaseVariantFileNameIsNotFoldedOnCaseSensitiveFS is
// the same rule at file level: `src/KEEP.go` names no file where `src/keep.go`
// and `src/Keep.go` both exist, and pooling both cases into one match would
// review two files the user did not name.
func TestProvider_Enumerate_CaseVariantFileNameIsNotFoldedOnCaseSensitiveFS(t *testing.T) {
	repo := initTestRepo(t)
	if fsIsCaseInsensitive(t, repo) {
		t.Skip("case-variant spellings are one path on this filesystem")
	}
	writeFile(t, repo, "src/keep.go", []byte("package keep\n"))
	writeFile(t, repo, "src/Keep.go", []byte("package keep\n"))
	gitCommit(t, repo, "init")

	if got, err := enumeratePathsErr(t, repo, []string{"src/KEEP.go"}); err == nil {
		t.Errorf("--path src/KEEP.go enumerated %q, want an error", got)
	}
}

// TestProvider_Enumerate_EmptyCaseVariantDirectoryStaysEmpty pins the
// follow-up from review: `src/docs/` exists as an empty directory while
// `src/DOCS/` holds tracked files. A selector resolves to the path the
// working tree holds at its spelling, and that path is matched exactly, so
// the empty directory selects nothing -- folding the string would have
// pulled in the sibling the user did not name.
func TestProvider_Enumerate_EmptyCaseVariantDirectoryStaysEmpty(t *testing.T) {
	repo := initTestRepo(t)
	if fsIsCaseInsensitive(t, repo) {
		t.Skip("case-variant directories are one directory on this filesystem")
	}
	writeFile(t, repo, filepath.Join("src", "DOCS", "main.go"), []byte("package docs\n"))
	gitCommit(t, repo, "init")
	if err := os.MkdirAll(filepath.Join(repo, "src", "docs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := enumeratePathsErr(t, repo, []string{"src/docs"})
	if err != nil || len(got) != 0 {
		t.Errorf("--path src/docs over an empty directory = %q, %v; want no files and no error", got, err)
	}
	if got := enumeratePaths(t, repo, []string{"src/DOCS"}); len(got) != 1 || got[0] != "src/DOCS/main.go" {
		t.Errorf("--path src/DOCS enumerated %q, want [src/DOCS/main.go]", got)
	}
}

// TestProvider_Enumerate_ResolvedAliasSelectsTrackedSpelling covers
// resolution through a symlink on a case-sensitive filesystem: `src` is a
// symlink to the tracked `SRC/`, so resolving the selector follows it to the
// tracked spelling even though no byte-exact entry matches. An empty result
// for a path the user can see would read as "nothing to review".
func TestProvider_Enumerate_ResolvedAliasSelectsTrackedSpelling(t *testing.T) {
	repo := initTestRepo(t)
	if fsIsCaseInsensitive(t, repo) {
		t.Skip("the alias would collide with the tracked directory on this filesystem")
	}
	writeFile(t, repo, ".gitignore", []byte("src\n"))
	writeFile(t, repo, filepath.Join("SRC", "Keep.go"), []byte("package keep\n"))
	gitCommit(t, repo, "init")

	if err := os.Symlink("SRC", filepath.Join(repo, "src")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got := enumeratePaths(t, repo, []string{"src"})
	want := []string{"SRC/Keep.go"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("--path src through the alias enumerated %q, want %q", got, want)
	}
}

// TestProvider_ResolveSelector_RepositoryBoundary covers selectors that
// resolve to the repository root or outside it: no tracked entry can match
// the result, so the scan stays a clean skip rather than widening to whatever
// the spelling points at.
func TestProvider_ResolveSelector_RepositoryBoundary(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	outside := filepath.Join(dir, "outside")
	for _, d := range []string{filepath.Join(repo, "src"), outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	p := &Provider{repoDir: repo}
	for _, tc := range []struct{ selector, want string }{
		{selector: ".", want: "."},
		{selector: "src/..", want: "."},
		{selector: "../outside", want: "../outside"},
	} {
		got, err := p.resolveSelector(tc.selector)
		if err != nil || got != tc.want {
			t.Errorf("resolveSelector(%q) = %q, %v; want %q and no error", tc.selector, got, err, tc.want)
		}
	}
}

// TestOnDiskName pins the two lookups resolution depends on: an exactly
// spelled entry is returned as is, and a spelling the filesystem resolves to
// a differently-cased entry takes that entry's name -- the arm macOS needs,
// since Windows is already normalised inside EvalSymlinks. On a
// case-sensitive filesystem the variant names nothing and must error instead
// of guessing.
func TestOnDiskName(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "MixedCase"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got, err := onDiskName(dir, "MixedCase"); err != nil || got != "MixedCase" {
		t.Errorf("onDiskName(exact) = %q, %v; want %q and no error", got, err, "MixedCase")
	}
	if fsIsCaseInsensitive(t, dir) {
		if got, err := onDiskName(dir, "mixedcase"); err != nil || got != "MixedCase" {
			t.Errorf("onDiskName(case variant) = %q, %v; want %q and no error", got, err, "MixedCase")
		}
		return
	}
	if got, err := onDiskName(dir, "mixedcase"); err == nil {
		t.Errorf("onDiskName(case variant) = %q, nil; want an error on a case-sensitive filesystem", got)
	}
}

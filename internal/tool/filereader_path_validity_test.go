// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

// TestReadLines_CommitMode_MissingPathIsStructural covers the ref-backed
// existence decision: a path that is not in the tree at the reviewed ref is
// rejected as a structural rejection, and the read is never attempted for it.
func TestReadLines_CommitMode_MissingPathIsStructural(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)

	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}
	_, _, err := fr.ReadLines(context.Background(), "missing.go", 1, 10)
	if err == nil {
		t.Fatal("ReadLines(missing.go) error = nil, want a rejection")
	}
	if !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("ReadLines(missing.go) error = %v, want ErrPathNotFound", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, `read file "missing.go"`) {
		t.Errorf("error = %q, want it to name the path", msg)
	}
	if !strings.Contains(msg, commit) {
		t.Errorf("error = %q, want it to name the ref", msg)
	}
	if strings.Contains(msg, "git show") || strings.Contains(msg, "exit status") {
		t.Errorf("error = %q, want no trace of the read command", msg)
	}
	if strings.Contains(msg, dir) {
		t.Errorf("error = %q, want no absolute filesystem path", msg)
	}

	// The rejection carries its details as data, so a consumer can read them
	// without parsing the message.
	var rejected *PathNotFoundError
	if !errors.As(err, &rejected) {
		t.Fatalf("error = %v, want it to carry a *PathNotFoundError", err)
	}
	if rejected.Path != "missing.go" {
		t.Errorf("rejected.Path = %q, want %q", rejected.Path, "missing.go")
	}
	if !strings.Contains(rejected.Target, commit) {
		t.Errorf("rejected.Target = %q, want it to name the ref", rejected.Target)
	}

	// The exported Read path must reach the same verdict.
	if _, err := fr.Read(context.Background(), "missing.go"); !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("Read(missing.go) error = %v, want ErrPathNotFound", err)
	}
}

// TestReadLines_CommitMode_PresentAndDirectoryPathsStillRead pins the paths
// that must keep working: a file that is present, and a directory, which the
// underlying read presents as a tree listing.
func TestReadLines_CommitMode_PresentAndDirectoryPathsStillRead(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)

	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}
	lines, total, err := fr.ReadLines(context.Background(), "hello.go", 1, 10)
	if err != nil {
		t.Fatalf("ReadLines(hello.go) error = %v, want nil", err)
	}
	if total == 0 || len(lines) == 0 {
		t.Fatalf("ReadLines(hello.go) = %d lines, want content", total)
	}

	// A directory exists in the tree, so it is present: the read keeps
	// today's behaviour and returns the tree listing.
	if _, _, err := fr.ReadLines(context.Background(), "pkg", 1, 10); err != nil {
		t.Fatalf("ReadLines(pkg) error = %v, want nil for a directory", err)
	}
}

// TestReadLines_CommitMode_GlobPathIsStructural covers a path that names no
// file: the underlying read treats it as a pathspec that matches nothing and
// yields an empty body, while the existence decision reports it as absent.
func TestReadLines_CommitMode_GlobPathIsStructural(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)

	for _, path := range []string{"*.go", "hello*", "?.go", "[h]ello.go", "pkg/*"} {
		t.Run(path, func(t *testing.T) {
			fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}
			_, _, err := fr.ReadLines(context.Background(), path, 1, 10)
			if err == nil {
				t.Fatalf("ReadLines(%q) error = nil, want a rejection", path)
			}
			if !errors.Is(err, ErrPathNotFound) {
				t.Fatalf("ReadLines(%q) error = %v, want ErrPathNotFound", path, err)
			}
		})
	}
}

// TestReadLines_CommitMode_LegitimatePathSpellings pins the path spellings
// that must keep reading exactly as before: unicode, a space, a colon,
// brackets, a leading dash, an empty file, and a path in a subdirectory.
func TestReadLines_CommitMode_LegitimatePathSpellings(t *testing.T) {
	dir := setupSpellingRepo(t)
	commit := getHeadCommit(t, dir)

	for _, path := range []string{
		"docs/云配置项.md", // allow-non-english: fixture exercises a non-ASCII path
		"we ird/space file.go",
		"we:ird/colon.txt",
		"br[ack]et.txt",
		"-weird.txt",
		"empty.txt",
		"pkg/sub/impl.go",
	} {
		t.Run(path, func(t *testing.T) {
			fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}
			if _, _, err := fr.ReadLines(context.Background(), path, 1, 100); err != nil {
				t.Fatalf("ReadLines(%q) error = %v, want nil", path, err)
			}
		})
	}
}

// setupSpellingRepo builds a repository holding one file per path spelling
// that must keep working.
func setupSpellingRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", args, err, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	for _, rel := range []string{
		"docs/云配置项.md", // allow-non-english: fixture exercises a non-ASCII path
		"we ird/space file.go",
		"we:ird/colon.txt",
		"br[ack]et.txt",
		"-weird.txt",
		"empty.txt",
		"pkg/sub/impl.go",
	} {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		content := "package x\n"
		if rel == "empty.txt" {
			content = ""
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "init")
	return dir
}

// TestReadLines_CommitMode_RunnerBackedMissingPathIsStructural covers the
// ref-backed decision on the runner-backed path, which is the one production
// uses: the shared runner reports the same exit status as a direct call.
func TestReadLines_CommitMode_RunnerBackedMissingPathIsStructural(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)

	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit, Runner: gitcmd.New(4)}
	_, _, err := fr.ReadLines(context.Background(), "missing.go", 1, 10)
	if !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("ReadLines(missing.go) error = %v, want ErrPathNotFound", err)
	}
	if strings.Contains(err.Error(), "git show") {
		t.Errorf("error = %q, want no trace of the read command", err)
	}
}

// TestReadLines_CommitMode_CancelledContextFallsBackToRead covers the
// cancelled-context arm of the fail-open: a context that has already ended
// makes the decision impossible, and the read's own error must surface.
func TestReadLines_CommitMode_CancelledContextFallsBackToRead(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}
	_, _, err := fr.ReadLines(ctx, "missing.go", 1, 10)
	if err == nil {
		t.Fatal("ReadLines(missing.go) error = nil, want the read's own error")
	}
	if errors.Is(err, ErrPathNotFound) {
		t.Fatalf("ReadLines(missing.go) error = %v, want the read's own error, not a rejection", err)
	}
}

// TestReadLines_Workspace_DanglingSymlinkIsAbsent covers a symlink inside the
// repository whose target does not exist: its own parent is inside the
// repository, so the absence is what is reported — the same class as before
// the change.
func TestReadLines_Workspace_DanglingSymlinkIsAbsent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}
	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	if err := os.Mkdir(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "nowhere.txt"), filepath.Join(repoDir, "link.txt")); err != nil {
		t.Fatal(err)
	}

	fr := &FileReader{RepoDir: repoDir, Mode: ModeWorkspace}
	_, _, err := fr.ReadLines(context.Background(), "link.txt", 1, 10)
	if !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("ReadLines(link.txt) error = %v, want ErrPathNotFound", err)
	}
}

// TestExecute_MissingFileIsStructural covers the tool layer's own message
// composition: the rejection reaches the model intact, and it is not
// re-labelled as a not-found failure.
func TestExecute_MissingFileIsStructural(t *testing.T) {
	dir := t.TempDir()
	p := NewFileRead(&FileReader{RepoDir: dir, Mode: ModeWorkspace})

	_, err := p.Execute(context.Background(), map[string]any{"file_path": "missing.txt"})
	if err == nil {
		t.Fatal("Execute(missing.txt) error = nil, want a rejection")
	}
	if !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("Execute(missing.txt) error = %v, want ErrPathNotFound", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "not found") {
		t.Errorf("error = %q, want the rejection not to be labelled as a not-found failure", msg)
	}
	if !strings.HasPrefix(msg, `read file "missing.txt": `) {
		t.Errorf("error = %q, want the rejection's own message", msg)
	}
	if strings.Contains(msg, dir) {
		t.Errorf("error = %q, want no absolute filesystem path", msg)
	}
}

// TestExecute_NonAbsenceFailureIsNotLabelledNotFound covers the other half of
// the message composition: a failure that is not an absence must not be
// reported as a missing file.
func TestExecute_NonAbsenceFailureIsNotLabelledNotFound(t *testing.T) {
	dir := t.TempDir()
	p := NewFileRead(&FileReader{RepoDir: dir, Mode: ModeWorkspace})

	_, err := p.Execute(context.Background(), map[string]any{"file_path": "../outside.txt"})
	if err == nil {
		t.Fatal("Execute(../outside.txt) error = nil, want an error")
	}
	if strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want a failure that is not an absence not to be labelled not found", err)
	}
}

// TestReadLines_CommitMode_UndecidableRefFallsBackToRead covers the
// fail-open path: when the existence decision cannot be made, the ordinary
// read proceeds and its own error surfaces — the decision must never report
// a transport failure as an absent path.
func TestReadLines_CommitMode_UndecidableRefFallsBackToRead(t *testing.T) {
	dir := setupTestRepo(t)

	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: "nosuchref"}
	_, _, err := fr.ReadLines(context.Background(), "missing.go", 1, 10)
	if err == nil {
		t.Fatal("ReadLines(missing.go) error = nil, want the read's own error")
	}
	if errors.Is(err, ErrPathNotFound) {
		t.Fatalf("ReadLines(missing.go) error = %v, want the read's own error, not a rejection", err)
	}
	if !strings.Contains(err.Error(), "git show") {
		t.Errorf("error = %q, want the read's own error", err)
	}
}

// TestReadLines_CommitMode_NonRepoDirFallsBackToRead covers the other
// fail-open branch: the existence decision itself fails (the directory is
// not a git repository, so git exits with a status other than 1), and the
// read's own error must surface — a transport failure must never be
// reported as an absent path.
func TestReadLines_CommitMode_NonRepoDirFallsBackToRead(t *testing.T) {
	fr := &FileReader{RepoDir: t.TempDir(), Mode: ModeCommit, Ref: "HEAD"}
	_, _, err := fr.ReadLines(context.Background(), "missing.go", 1, 10)
	if err == nil {
		t.Fatal("ReadLines(missing.go) error = nil, want the read's own error")
	}
	if errors.Is(err, ErrPathNotFound) {
		t.Fatalf("ReadLines(missing.go) error = %v, want the read's own error, not a rejection", err)
	}
	if !strings.Contains(err.Error(), "git show") {
		t.Errorf("error = %q, want the read's own error", err)
	}
}

// TestReadLines_CommitMode_NoRefFallsBackToRead covers the reader without a
// ref: there is nothing to resolve the path against, so there is nothing to
// decide and the ordinary read reports what it finds.
func TestReadLines_CommitMode_NoRefFallsBackToRead(t *testing.T) {
	fr := &FileReader{RepoDir: setupTestRepo(t), Mode: ModeCommit}
	_, _, err := fr.ReadLines(context.Background(), "missing.go", 1, 10)
	if err == nil {
		t.Fatal("ReadLines(missing.go) error = nil, want the read's own error")
	}
	if errors.Is(err, ErrPathNotFound) {
		t.Fatalf("ReadLines(missing.go) error = %v, want the read's own error, not a rejection", err)
	}
}

// TestReadLines_CommitMode_DotDotPathIsStructural covers a path whose last
// segment is "..": git parses that as a revision suffix, so the underlying
// read returns the parent commit's diff rather than file content. The path
// names no file, so the existence decision reports it as absent.
func TestReadLines_CommitMode_DotDotPathIsStructural(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)

	for _, path := range []string{"..", "pkg/..", "./.."} {
		t.Run(path, func(t *testing.T) {
			fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}
			_, _, err := fr.ReadLines(context.Background(), path, 1, 10)
			if err == nil {
				t.Fatalf("ReadLines(%q) error = nil, want a rejection", path)
			}
			if !errors.Is(err, ErrPathNotFound) {
				t.Fatalf("ReadLines(%q) error = %v, want ErrPathNotFound", path, err)
			}
		})
	}
}

// TestReadLines_Workspace_MissingPathIsStructural covers the workspace
// decision: a path that is not in the working tree is rejected as a
// structural rejection, and the rejection still satisfies the existing
// workspace read contract.
func TestReadLines_Workspace_MissingPathIsStructural(t *testing.T) {
	dir := t.TempDir()

	fr := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
	_, _, err := fr.ReadLines(context.Background(), "missing.txt", 1, 10)
	if err == nil {
		t.Fatal("ReadLines(missing.txt) error = nil, want a rejection")
	}
	if !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("ReadLines(missing.txt) error = %v, want ErrPathNotFound", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadLines(missing.txt) error = %v, want os.ErrNotExist to survive", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, `read file "missing.txt"`) {
		t.Errorf("error = %q, want it to name the file", msg)
	}
	if strings.Contains(msg, "resolve file") {
		t.Errorf("error = %q, want it not to read as a path-resolution failure", msg)
	}
	if strings.Contains(msg, dir) {
		t.Errorf("error = %q, want no absolute filesystem path", msg)
	}
}

// TestReadLines_Workspace_OutOfRepoDirSymlinkIsRefused covers the containment
// ordering: a path that leaves the repository through a symlinked directory
// must still be refused as out-of-repository even when the file it names does
// not exist. Today the missing file hides that escape behind a bare
// "does not exist".
func TestReadLines_Workspace_OutOfRepoDirSymlinkIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}
	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	if err := os.Mkdir(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	outsideDir := filepath.Join(base, "outside")
	if err := os.Mkdir(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(repoDir, "link")); err != nil {
		t.Fatal(err)
	}

	fr := &FileReader{RepoDir: repoDir, Mode: ModeWorkspace}
	_, _, err := fr.ReadLines(context.Background(), filepath.Join("link", "missing.txt"), 1, 10)
	if err == nil {
		t.Fatal("ReadLines(link/missing.txt) error = nil, want a rejection")
	}
	if !strings.Contains(err.Error(), "outside repository") {
		t.Fatalf("ReadLines(link/missing.txt) error = %v, want outside repository", err)
	}
}

// TestFileReader_GitInvocationCount counts the git invocations one read
// produces, using a shim installed ahead of git on PATH. It is the
// measurement the plan's cost model is built on, and it is what makes the
// "the read is not attempted" property observable at the subprocess level.
func TestFileReader_GitInvocationCount(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the git shim is a shell script")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not found in PATH")
	}

	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)

	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "git.log")
	shim := filepath.Join(shimDir, "git")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logPath + "\"\nexec \"" + realGit + "\" \"$@\"\n"
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	readInvocations := func() []string {
		t.Helper()
		data, err := os.ReadFile(logPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			t.Fatal(err)
		}
		var lines []string
		for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			if line != "" {
				lines = append(lines, line)
			}
		}
		return lines
	}

	// A path that is present: one existence decision and one read.
	fr := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit}
	if _, _, err := fr.ReadLines(context.Background(), "hello.go", 1, 10); err != nil {
		t.Fatalf("ReadLines(hello.go) error = %v, want nil", err)
	}
	present := readInvocations()
	if len(present) != 2 {
		t.Fatalf("git invocations for a present path = %v, want 2", present)
	}
	if !strings.Contains(present[0], "rev-parse") || !strings.Contains(present[0], "hello.go") {
		t.Errorf("first invocation = %q, want the existence decision", present[0])
	}
	if !strings.Contains(present[1], "show") {
		t.Errorf("second invocation = %q, want the read", present[1])
	}

	// A path that is absent: the existence decision and the ref check, and
	// no read at all.
	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fr.ReadLines(context.Background(), "missing.go", 1, 10); !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("ReadLines(missing.go) error = %v, want ErrPathNotFound", err)
	}
	absent := readInvocations()
	if len(absent) != 2 {
		t.Fatalf("git invocations for an absent path = %v, want 2", absent)
	}
	for _, invocation := range absent {
		if strings.Contains(invocation, "show") {
			t.Errorf("invocation %q, want no read for an absent path", invocation)
		}
	}
	if !strings.Contains(absent[0], "rev-parse") || !strings.Contains(absent[0], "missing.go") {
		t.Errorf("first invocation = %q, want the path probe", absent[0])
	}
	if !strings.Contains(absent[1], "rev-parse") || !strings.Contains(absent[1], "^{commit}") {
		t.Errorf("second invocation = %q, want the ref probe", absent[1])
	}

	// The runner-backed reader — the one production uses — spawns the same
	// number of git invocations.
	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}
	runnerReader := &FileReader{RepoDir: dir, Mode: ModeCommit, Ref: commit, Runner: gitcmd.New(4)}
	if _, _, err := runnerReader.ReadLines(context.Background(), "hello.go", 1, 10); err != nil {
		t.Fatalf("ReadLines(hello.go) error = %v, want nil", err)
	}
	if runnerPresent := readInvocations(); len(runnerPresent) != 2 {
		t.Fatalf("git invocations for a present path through the runner = %v, want 2", runnerPresent)
	}

	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runnerReader.ReadLines(context.Background(), "missing.go", 1, 10); !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("ReadLines(missing.go) error = %v, want ErrPathNotFound", err)
	}
	runnerAbsent := readInvocations()
	if len(runnerAbsent) != 2 {
		t.Fatalf("git invocations for an absent path through the runner = %v, want 2", runnerAbsent)
	}
	for _, invocation := range runnerAbsent {
		if strings.Contains(invocation, "show") {
			t.Errorf("invocation %q, want no read for an absent path", invocation)
		}
	}
}

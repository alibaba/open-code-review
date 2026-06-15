package tool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBuildGrepArgs_WorkspaceMode(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: ""})
	args := p.buildGrepArgs("myFunc", false, false, nil)

	assertContainsInOrder(t, args, "-e", "myFunc", "--")
	assertContains(t, args, "-i")
	if idx := slices.Index(args, "--"); idx >= 0 {
		for i := 0; i < idx; i++ {
			if args[i] == "myFunc" && (i == 0 || args[i-1] != "-e") {
				t.Error("myFunc should only appear as argument to -e, not as positional")
			}
		}
	}
}

func TestBuildGrepArgs_CommitMode(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: "abc1234"})
	args := p.buildGrepArgs("myFunc", false, false, []string{"pkg/"})

	assertContainsInOrder(t, args, "-e", "myFunc", "--end-of-options", "abc1234", "--", "pkg/")
}

func TestBuildGrepArgs_RefUsesEndOfOptions(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: "-O./pwn.sh"})
	args := p.buildGrepArgs("myFunc", false, false, nil)

	assertContainsInOrder(t, args, "-e", "myFunc", "--end-of-options", "-O./pwn.sh", "--")
}

func TestBuildGrepArgs_PatternStartingWithDash(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: ""})
	args := p.buildGrepArgs("-myOption", false, false, nil)

	idx := slices.Index(args, "-e")
	if idx < 0 || idx+1 >= len(args) || args[idx+1] != "-myOption" {
		t.Errorf("expected -e to immediately precede -myOption, got %v", args)
	}
}

func TestBuildGrepArgs_CaseSensitive(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: ""})
	args := p.buildGrepArgs("foo", true, false, nil)

	assertNotContains(t, args, "-i")
}

func TestBuildGrepArgs_CaseInsensitive(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: ""})
	args := p.buildGrepArgs("foo", false, false, nil)

	assertContains(t, args, "-i")
}

func TestBuildGrepArgs_PerlRegexp(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: ""})
	args := p.buildGrepArgs("foo", false, true, nil)

	assertContains(t, args, "-P")
	assertNotContains(t, args, "-F")
}

func TestBuildGrepArgs_FixedString(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: ""})
	args := p.buildGrepArgs("foo", false, false, nil)

	assertContains(t, args, "-F")
	assertNotContains(t, args, "-E")
	assertNotContains(t, args, "-P")
}

func setupTestRepo(t *testing.T) string {
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
	if err := os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\n\nfunc Hello() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "util.go"), []byte("package pkg\n\nfunc Util() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "init")
	return dir
}

func getHeadCommit(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestGitGrep_WorkspaceMode_Found(t *testing.T) {
	dir := setupTestRepo(t)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: "", Mode: ModeWorkspace})
	result, err := p.gitGrep(context.Background(), "Hello", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "hello.go") {
		t.Errorf("expected hello.go in result, got: %s", result)
	}
}

func TestGitGrep_WorkspaceMode_NoMatch(t *testing.T) {
	dir := setupTestRepo(t)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: "", Mode: ModeWorkspace})
	result, err := p.gitGrep(context.Background(), "nonexistentXYZ", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != "No matches found" {
		t.Errorf("expected 'No matches found', got: %s", result)
	}
}

func TestGitGrep_CommitMode_Found(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: commit, Mode: ModeCommit})
	result, err := p.gitGrep(context.Background(), "Hello", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "hello.go") {
		t.Errorf("expected hello.go in result, got: %s", result)
	}
	if !strings.Contains(result, "Match lines: 1") {
		t.Errorf("expected 1 match line, got: %s", result)
	}
}

func TestGitGrep_CommitMode_NoMatch(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: commit, Mode: ModeCommit})
	result, err := p.gitGrep(context.Background(), "nonexistentXYZ", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != "No matches found" {
		t.Errorf("expected 'No matches found', got: %s", result)
	}
}

func TestGitGrep_CommitMode_WithPathspec(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: commit, Mode: ModeCommit})

	result, err := p.gitGrep(context.Background(), "Util", false, false, []string{"pkg/"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "util.go") {
		t.Errorf("expected util.go in result, got: %s", result)
	}

	result2, err2 := p.gitGrep(context.Background(), "Hello", false, false, []string{"pkg/"})
	if err2 != nil {
		t.Fatal(err2)
	}
	if result2 != "No matches found" {
		t.Errorf("expected 'No matches found' when pathspec excludes match, got: %s", result2)
	}
}

func TestGitGrep_OptionLikeRefDoesNotLaunchPager(t *testing.T) {
	dir := setupTestRepo(t)
	proofPath := filepath.Join(dir, "PROOF")
	pagerPath := filepath.Join(dir, "pwn.sh")
	if err := os.WriteFile(pagerPath, []byte("#!/bin/sh\nprintf pwned > PROOF\n"), 0755); err != nil {
		t.Fatal(err)
	}

	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: "-O./pwn.sh", Mode: ModeCommit})
	result, err := p.gitGrep(context.Background(), "Hello", false, false, []string{"hello.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "unable to resolve revision") && !strings.Contains(result, "Not a valid object name") {
		t.Fatalf("expected invalid revision error, got: %s", result)
	}
	if _, err := os.Stat(proofPath); err == nil {
		t.Fatal("option-like ref launched pager and created proof file")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestGitGrep_CommitMode_WithBadPathspec(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: commit, Mode: ModeCommit})

	result, err := p.gitGrep(context.Background(), "Hello", false, false, []string{"nonexistent/"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "No matches found" {
		t.Errorf("expected 'No matches found' with bad pathspec, got: %s", result)
	}
}

func TestGitGrep_LiteralWithRegexMetaChars(t *testing.T) {
	dir := setupTestRepo(t)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: "", Mode: ModeWorkspace})
	result, err := p.gitGrep(context.Background(), "Hello()", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "hello.go") {
		t.Errorf("expected hello.go in result for literal 'Hello()' search, got: %s", result)
	}
}

func TestGitGrep_CommitMode_LiteralWithRegexMetaChars(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: commit, Mode: ModeCommit})
	result, err := p.gitGrep(context.Background(), "Hello()", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "hello.go") {
		t.Errorf("expected hello.go in result for literal 'Hello()' search at commit, got: %s", result)
	}
}

func TestGitGrep_InvalidRef_ReturnsError(t *testing.T) {
	dir := setupTestRepo(t)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: "nonexistent_ref_abc123", Mode: ModeCommit})
	result, err := p.gitGrep(context.Background(), "Hello", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Error:") {
		t.Errorf("expected error message for invalid ref, got: %s", result)
	}
}

func TestGitGrep_PerlRegexp_InvalidPattern_ReturnsError(t *testing.T) {
	dir := setupTestRepo(t)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: "", Mode: ModeWorkspace})
	result, err := p.gitGrep(context.Background(), "(unclosed", false, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Error:") {
		t.Errorf("expected error message for invalid perl regexp, got: %s", result)
	}
}

func assertContains(t *testing.T, args []string, val string) {
	t.Helper()
	if !slices.Contains(args, val) {
		t.Errorf("expected args to contain %q, got %v", val, args)
	}
}

func assertNotContains(t *testing.T, args []string, val string) {
	t.Helper()
	if slices.Contains(args, val) {
		t.Errorf("expected args NOT to contain %q, got %v", val, args)
	}
}

func assertContainsInOrder(t *testing.T, args []string, vals ...string) {
	t.Helper()
	idx := 0
	for _, a := range args {
		if idx < len(vals) && a == vals[idx] {
			idx++
		}
	}
	if idx != len(vals) {
		t.Errorf("expected args to contain %v in order, got %v (matched up to index %d)", vals, args, idx)
	}
}

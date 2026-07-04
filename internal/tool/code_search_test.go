package tool

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/gitcmd"
)

func TestBuildGrepArgs_WorkspaceMode(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: ""})
	args := p.buildGrepArgs("myFunc", false, false, false, nil)

	assertContainsInOrder(t, args, "-e", "myFunc", "--")
	assertContains(t, args, "-i")
	assertContains(t, args, "--untracked")
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
	args := p.buildGrepArgs("myFunc", false, false, false, []string{"pkg/"})

	assertContainsInOrder(t, args, "-e", "myFunc", "--end-of-options", "abc1234", "--", "pkg/")
	assertNotContains(t, args, "--untracked")
}

func TestBuildGrepArgs_RefUsesEndOfOptions(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: "-O./pwn.sh"})
	args := p.buildGrepArgs("myFunc", false, false, false, nil)

	assertContainsInOrder(t, args, "-e", "myFunc", "--end-of-options", "-O./pwn.sh", "--")
}

func TestBuildGrepArgs_PatternStartingWithDash(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: ""})
	args := p.buildGrepArgs("-myOption", false, false, false, nil)

	idx := slices.Index(args, "-e")
	if idx < 0 || idx+1 >= len(args) || args[idx+1] != "-myOption" {
		t.Errorf("expected -e to immediately precede -myOption, got %v", args)
	}
}

func TestBuildGrepArgs_CaseSensitive(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: ""})
	args := p.buildGrepArgs("foo", true, false, false, nil)

	assertNotContains(t, args, "-i")
}

func TestBuildGrepArgs_CaseInsensitive(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: ""})
	args := p.buildGrepArgs("foo", false, false, false, nil)

	assertContains(t, args, "-i")
}

func TestBuildGrepArgs_PerlRegexp(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: ""})
	args := p.buildGrepArgs("foo", false, true, false, nil)

	assertContains(t, args, "-P")
	assertNotContains(t, args, "-F")
}

func TestBuildGrepArgs_FixedString(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: ""})
	args := p.buildGrepArgs("foo", false, false, false, nil)

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
	if !strings.HasPrefix(result, "Error:") {
		t.Fatalf("expected git error for invalid ref, got: %s", result)
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

func TestGitGrep_WorkspaceMode_UntrackedFile(t *testing.T) {
	dir := setupTestRepo(t)
	untrackedDir := filepath.Join(dir, "newpkg")
	if err := os.MkdirAll(untrackedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(untrackedDir, "untracked.go"), []byte("package newpkg\n\nfunc UntrackedFunc() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: "", Mode: ModeWorkspace})
	result, err := p.gitGrep(context.Background(), "UntrackedFunc", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "untracked.go") {
		t.Errorf("expected untracked.go in result, got: %s", result)
	}
}

// TestGitGrep_NonGitDirectoryFallback verifies code_search works in a plain
// (non-git) directory by retrying git grep in --no-index mode instead of
// failing with git's exit 128, while still honoring .gitignore.
func TestGitGrep_NonGitDirectoryFallback(t *testing.T) {
	dir := t.TempDir() // plain dir, no `git init`

	write := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("server.go", "package main\n\nfunc Handler() {}\n")
	write("internal/svc.go", "package internal\n\nfunc Handler() {}\n")
	write(".gitignore", "node_modules/\n")
	write("node_modules/lib.js", "function Handler() {}\n") // excluded by .gitignore

	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: "", Mode: ModeWorkspace})

	out, err := p.gitGrep(context.Background(), "Handler", false, false, nil)
	if err != nil {
		t.Fatalf("gitGrep should not error in a non-git dir, got: %v", err)
	}
	if !strings.Contains(out, "server.go") || !strings.Contains(out, "internal/svc.go") {
		t.Errorf("expected matches in tracked-like files, got:\n%s", out)
	}
	if strings.Contains(out, "node_modules") {
		t.Errorf("node_modules should be excluded via --exclude-standard, got:\n%s", out)
	}
}

// TestGitGrep_NonGitDirectoryNoMatch verifies the no-match path in a non-git
// dir returns the sentinel rather than an error.
func TestGitGrep_NonGitDirectoryNoMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: "", Mode: ModeWorkspace})

	out, err := p.gitGrep(context.Background(), "nonexistentXYZ", false, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "No matches found" {
		t.Errorf("expected 'No matches found', got: %q", out)
	}
}

func TestCodeSearchProvider_Tool(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp"})
	if p.Tool() != CodeSearch {
		t.Errorf("Tool() = %v, want CodeSearch", p.Tool())
	}
}

func TestCodeSearchProvider_Execute_BlankSearchText(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp"})
	got, err := p.Execute(context.Background(), map[string]any{"search_text": "  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Error: search_text is blank" {
		t.Errorf("Execute() = %q, want blank error", got)
	}
}

func TestCodeSearchProvider_Execute_Found(t *testing.T) {
	dir := setupTestRepo(t)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Mode: ModeWorkspace})

	got, err := p.Execute(context.Background(), map[string]any{
		"search_text": "Hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "hello.go") {
		t.Errorf("expected hello.go in result, got: %s", got)
	}
}

func TestCodeSearchProvider_Execute_WithFilePatterns(t *testing.T) {
	dir := setupTestRepo(t)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Mode: ModeWorkspace})

	got, err := p.Execute(context.Background(), map[string]any{
		"search_text":   "Util",
		"file_patterns": []any{"pkg/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "util.go") {
		t.Errorf("expected util.go in result, got: %s", got)
	}
}

func TestCodeSearchProvider_Execute_RejectsTraversalPattern(t *testing.T) {
	dir := setupTestRepo(t)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Mode: ModeWorkspace})
	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{name: "leading parent", pattern: "../pkg", want: "Error: file_patterns must not contain .."},
		{name: "middle parent", pattern: "pkg/../internal", want: "Error: file_patterns must not contain .."},
		{name: "trailing parent", pattern: "pkg/..", want: "Error: file_patterns must not contain .."},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := p.Execute(context.Background(), map[string]any{
				"search_text":   "Hello",
				"file_patterns": []any{test.pattern},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Errorf("Execute() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCodeSearchProvider_Execute_AllowsDoubleDotInFilename(t *testing.T) {
	dir := setupTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "foo..bar.go"), []byte("package main\n\nfunc DoubleDotName() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewCodeSearch(&FileReader{RepoDir: dir, Mode: ModeWorkspace})
	got, err := p.Execute(context.Background(), map[string]any{
		"search_text":   "DoubleDotName",
		"file_patterns": []any{"foo..bar.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "foo..bar.go") {
		t.Errorf("expected foo..bar.go in result, got: %s", got)
	}
}

func TestCodeSearchProvider_Execute_CaseSensitive(t *testing.T) {
	dir := setupTestRepo(t)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Mode: ModeWorkspace})

	got, err := p.Execute(context.Background(), map[string]any{
		"search_text":    "hello",
		"case_sensitive": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "Hello") {
		t.Errorf("case-sensitive search for 'hello' should not match 'Hello', got: %s", got)
	}
}

func TestCodeSearchProvider_Execute_PerlRegexp(t *testing.T) {
	dir := setupTestRepo(t)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Mode: ModeWorkspace})

	got, err := p.Execute(context.Background(), map[string]any{
		"search_text":     "Hell\\w+",
		"use_perl_regexp": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "hello.go") {
		t.Errorf("expected hello.go in perl regexp result, got: %s", got)
	}
}

func TestGitGrep_WithRunner(t *testing.T) {
	dir := setupTestRepo(t)
	runner := gitcmd.New(4)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Mode: ModeWorkspace, Runner: runner})

	result, err := p.gitGrep(context.Background(), "Hello", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "hello.go") {
		t.Errorf("expected hello.go in result via Runner, got: %s", result)
	}
}

func TestGitGrep_WithRunner_NoMatch(t *testing.T) {
	dir := setupTestRepo(t)
	runner := gitcmd.New(4)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Mode: ModeWorkspace, Runner: runner})

	result, err := p.gitGrep(context.Background(), "nonexistentXYZ", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != "No matches found" {
		t.Errorf("expected 'No matches found', got: %s", result)
	}
}

func TestGitGrep_WithRunner_CommitMode(t *testing.T) {
	dir := setupTestRepo(t)
	commit := getHeadCommit(t, dir)
	runner := gitcmd.New(4)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Ref: commit, Mode: ModeCommit, Runner: runner})

	result, err := p.gitGrep(context.Background(), "Hello", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "hello.go") {
		t.Errorf("expected hello.go in result via Runner commit mode, got: %s", result)
	}
}

func TestGitGrep_Timeout(t *testing.T) {
	dir := setupTestRepo(t)
	p := NewCodeSearch(&FileReader{RepoDir: dir, Mode: ModeWorkspace})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := p.gitGrep(ctx, "Hello", false, false, nil)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got: %v", err)
		}
		return
	}
	if !strings.Contains(result, "timed out") && !strings.Contains(result, "No matches found") {
		t.Errorf("expected timeout or no matches message, got: %s", result)
	}
}

func TestBuildGrepArgs_NoIndex(t *testing.T) {
	p := NewCodeSearch(&FileReader{RepoDir: "/tmp", Ref: ""})
	args := p.buildGrepArgs("foo", false, false, true, nil)

	assertContains(t, args, "--no-index")
	assertContains(t, args, "--exclude-standard")
	assertNotContains(t, args, "--untracked")
}

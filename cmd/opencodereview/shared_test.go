// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/stdout"
)

func TestResolveMaxTokensPrecedence(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		cliOverride int
		template    int
		want        int
		wantErr     bool
	}{
		{name: "template default", template: 58888, want: 58888},
		{name: "zero config is unset", cfg: &Config{}, template: 58888, want: 58888},
		{name: "saved config", cfg: &Config{MaxTokens: 128000}, template: 58888, want: 128000},
		{name: "cli overrides config", cfg: &Config{MaxTokens: 128000}, cliOverride: 200000, template: 58888, want: 200000},
		{name: "negative config", cfg: &Config{MaxTokens: -1}, template: 58888, wantErr: true},
		{name: "negative cli", cliOverride: -1, template: 58888, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveMaxTokens(tt.template, tt.cfg, tt.cliOverride)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveMaxTokens() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("resolveMaxTokens() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestApplyCLIExcludes_Empty(t *testing.T) {
	cc := &commonContext{FileFilter: &rules.FileFilter{Exclude: []string{"a"}}}
	applyCLIExcludes(cc, nil)
	if len(cc.FileFilter.Exclude) != 1 {
		t.Errorf("expected 1 exclude, got %d", len(cc.FileFilter.Exclude))
	}
}

func TestLoadCommonContext_MaxToolsOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()

	tests := []struct {
		name     string
		maxTools int
		want     int
	}{
		{name: "template default", maxTools: 0, want: 30},
		{name: "lower bound", maxTools: 10, want: 10},
		{name: "lower than default", maxTools: 15, want: 15},
		{name: "higher than default", maxTools: 40, want: 40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc, err := loadCommonContext(repoDir, "", tt.maxTools, 0, false)
			if err != nil {
				t.Fatalf("loadCommonContext() error: %v", err)
			}
			if got := cc.Template.MaxToolRequestTimes; got != tt.want {
				t.Errorf("MaxToolRequestTimes = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestApplyCLIExcludes_AppendsPatterns(t *testing.T) {
	cc := &commonContext{FileFilter: &rules.FileFilter{Exclude: []string{"a"}}}
	applyCLIExcludes(cc, []string{"b", "c"})
	if len(cc.FileFilter.Exclude) != 3 {
		t.Errorf("expected 3 excludes, got %d", len(cc.FileFilter.Exclude))
	}
}

func TestApplyCLIExcludes_NilFileFilter(t *testing.T) {
	cc := &commonContext{}
	applyCLIExcludes(cc, []string{"x"})
	if cc.FileFilter == nil {
		t.Fatal("expected FileFilter to be created")
	}
	if len(cc.FileFilter.Exclude) != 1 || cc.FileFilter.Exclude[0] != "x" {
		t.Errorf("expected [x], got %v", cc.FileFilter.Exclude)
	}
}

// TestNewQuietHandle_Routing pins where [ocr] progress lines go for each
// (format, audience) pair. The machine-readable + human rows are the fix for
// #928: those runs used to discard progress entirely, leaving the user staring
// at a silent terminal until the document appeared.
func TestNewQuietHandle_Routing(t *testing.T) {
	tests := []struct {
		name       string
		format     string
		audience   string
		wantWriter io.Writer
	}{
		{name: "text human keeps stdout", format: "text", audience: "human", wantWriter: os.Stdout},
		{name: "json human streams to stderr", format: "json", audience: "human", wantWriter: os.Stderr},
		{name: "sarif human streams to stderr", format: "sarif", audience: "human", wantWriter: os.Stderr},
		{name: "text agent discards", format: "text", audience: "agent", wantWriter: io.Discard},
		{name: "json agent discards", format: "json", audience: "agent", wantWriter: io.Discard},
		{name: "sarif agent discards", format: "sarif", audience: "agent", wantWriter: io.Discard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := stdout.Writer()
			h := newQuietHandle(tt.format, tt.audience)
			if got := stdout.Writer(); got != tt.wantWriter {
				t.Errorf("progress writer = %#v, want %#v", got, tt.wantWriter)
			}
			h.Restore()
			if got := stdout.Writer(); got != before {
				t.Errorf("Restore left writer as %#v, want %#v", got, before)
			}
		})
	}
}

// TestNewQuietHandle_JSONHumanKeepsStdoutForDocument guards the invariant the
// stderr redirect relies on: progress written through stdout.Writer() must not
// reach os.Stdout, so the JSON document stays the only thing on stdout and
// remains parseable.
func TestNewQuietHandle_JSONHumanKeepsStdoutForDocument(t *testing.T) {
	var out string
	// captureStderr must wrap the handle creation: newQuietHandle captures the
	// current value of os.Stderr, so the pipe has to be installed first.
	progress := captureStderr(t, func() {
		out = captureStdout(t, func() {
			h := newQuietHandle("json", "human")
			defer h.Restore()
			fmt.Fprintln(stdout.Writer(), "[ocr] reviewing foo.go")
		})
	})

	if out != "" {
		t.Errorf("stdout must stay clean for the document, got %q", out)
	}
	if !strings.Contains(progress, "[ocr] reviewing foo.go") {
		t.Errorf("progress must reach stderr, got %q", progress)
	}
}

func TestQuietHandle_NilReceiver(t *testing.T) {
	var h *quietHandle
	h.Restore()
}

func TestQuietHandle_IdempotentRestore(t *testing.T) {
	h := newQuietHandle("json", "human")
	h.Restore()
	h.Restore()
	if h.fn != nil {
		t.Error("expected nil after double restore")
	}
}

func TestResolveWorkingDir_CurrentDir(t *testing.T) {
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("restore chdir: %v", err)
		}
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	absPath, isGit, err := resolveWorkingDir("", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if absPath == "" {
		t.Error("expected non-empty absPath")
	}
	if isGit {
		t.Error("temp dir should not be a git repo")
	}
}

func TestResolveWorkingDir_RequireGitFails(t *testing.T) {
	dir := t.TempDir()
	_, _, err := resolveWorkingDir(dir, true)
	if err == nil {
		t.Fatal("expected error for non-git dir with requireGit=true")
	}
}

func TestResolveWorkingDir_NonExistent(t *testing.T) {
	_, _, err := resolveWorkingDir(filepath.Join(t.TempDir(), "no-such-dir"), false)
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

// TestResolveWorkingDir_MonorepoSubdir reproduces #287: running `ocr review`
// from a subdirectory of a git repo must anchor RepoDir at the git top-level
// (git reports diff / `git show HEAD:<path>` paths relative to the repo root),
// while `ocr scan` (requireGit=false) must keep the subdirectory so its walk
// stays scoped.
func TestResolveWorkingDir_MonorepoSubdir(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	git("config", "user.email", "t@t.co")
	git("config", "user.name", "t")

	sub := filepath.Join(root, "subproject1", "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// macOS /var -> /private/var symlink means t.TempDir() differs from the
	// canonicalized toplevel git returns; compare via EvalSymlinks.
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", root, err)
	}

	// review path: hoisted to the git top-level.
	got, isGit, err := resolveWorkingDir(sub, true)
	if err != nil {
		t.Fatalf("resolveWorkingDir(sub, true) error: %v", err)
	}
	if !isGit {
		t.Error("expected isGit=true for a git subdirectory")
	}
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", got, err)
	}
	if gotResolved != wantRoot {
		t.Errorf("review RepoDir = %q, want git top-level %q", gotResolved, wantRoot)
	}

	// scan path: keeps the subdirectory unchanged.
	gotScan, _, err := resolveWorkingDir(sub, false)
	if err != nil {
		t.Fatalf("resolveWorkingDir(sub, false) error: %v", err)
	}
	gotScanResolved, err := filepath.EvalSymlinks(gotScan)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", gotScan, err)
	}
	wantSub, err := filepath.EvalSymlinks(sub)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", sub, err)
	}
	if gotScanResolved != wantSub {
		t.Errorf("scan RepoDir = %q, want subdir %q (must stay scoped)", gotScanResolved, wantSub)
	}
}

// TestResolveWorkingDir_BareRepoFailsLoudly guards the #287 fix: a bare repo has
// no work tree, so `git rev-parse --git-dir` succeeds (isGit=true) but
// `--show-toplevel` fails. The review path (requireGit=true) must return an
// error rather than silently reusing the input dir, which would reproduce the
// original root-relative-path bug.
func TestResolveWorkingDir_BareRepoFailsLoudly(t *testing.T) {
	bare := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", bare)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}

	_, _, err := resolveWorkingDir(bare, true)
	if err == nil {
		t.Fatal("expected error for a bare repo (no work tree), got nil")
	}
}

func TestResolveWorkingDir_GitRepo(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	absPath, isGit, err := resolveWorkingDir(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if absPath == "" {
		t.Error("expected non-empty absPath")
	}
	_ = isGit
}

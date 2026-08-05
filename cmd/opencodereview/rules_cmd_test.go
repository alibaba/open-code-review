// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initRulesCheckTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	git("config", "user.email", "t@t.co")
	git("config", "user.name", "t")
	return repo
}

func writeRulesCheckTestFile(t *testing.T, repo, relPath, content string) {
	t.Helper()
	full := filepath.Join(repo, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setRulesCheckRepo points the rulesCheckCmd's package-level --repo flag at
// repo for the duration of the test, restoring it afterward. runRulesCheck
// reads rulesCheckRepoDir directly (it's a singleton cobra command's bound
// flag var, not a per-call parameter), so tests must set it this way.
func setRulesCheckRepo(t *testing.T, repo string) {
	t.Helper()
	orig := rulesCheckRepoDir
	rulesCheckRepoDir = repo
	t.Cleanup(func() { rulesCheckRepoDir = orig })
}

// TestRunRulesCheck_ObjCSniffOverridesMatlab exercises peekFirstLine's actual
// disk-read path: system_rules.json maps "**/*.m" to matlab.md, but an
// Objective-C file (recognizable by its #import/@implementation header)
// should sniff away from that pattern and use the dedicated objc.md rule
// (rather than the incorrect MATLAB rule) instead.
func TestRunRulesCheck_ObjCSniffOverridesMatlab(t *testing.T) {
	repo := initRulesCheckTestRepo(t)
	writeRulesCheckTestFile(t, repo, "ios/ViewController.m",
		"#import \"ViewController.h\"\n\n@implementation ViewController\n@end\n")
	setRulesCheckRepo(t, repo)

	got := captureStdout(t, func() {
		if err := runRulesCheck("ios/ViewController.m"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(got, "Pattern: **/*.m (sniffed: objc)") {
		t.Errorf("expected the sniffed-objc pattern label, got:\n%s", got)
	}
	if strings.Contains(got, "MATLAB") {
		t.Errorf("expected MATLAB-specific guidance to be replaced by the objc rule, got:\n%s", got)
	}
}

// TestRunRulesCheck_MatlabFileStaysMatlab is the control case: a genuine
// MATLAB file (function header, no ObjC signals) must still resolve via the
// plain "**/*.m" pattern.
func TestRunRulesCheck_MatlabFileStaysMatlab(t *testing.T) {
	repo := initRulesCheckTestRepo(t)
	writeRulesCheckTestFile(t, repo, "Models/main.m",
		"function y = main(x)\n  y = x + 1;\nend\n")
	setRulesCheckRepo(t, repo)

	got := captureStdout(t, func() {
		if err := runRulesCheck("Models/main.m"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(got, "Pattern: **/*.m") {
		t.Errorf("expected the matlab pattern to still match, got:\n%s", got)
	}
}

// TestRunRulesCheck_MissingFileFallsBackToPathOnlyMatch covers peekFirstLine's
// error path: a file path that doesn't exist on disk (e.g. checking a rule
// before creating the file) must not error out — content sniffing is simply
// skipped and resolution falls back to plain path matching.
func TestRunRulesCheck_MissingFileFallsBackToPathOnlyMatch(t *testing.T) {
	repo := initRulesCheckTestRepo(t)
	setRulesCheckRepo(t, repo)

	got := captureStdout(t, func() {
		if err := runRulesCheck("Models/does_not_exist.m"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(got, "Pattern: **/*.m") {
		t.Errorf("expected the matlab pattern to match by path alone, got:\n%s", got)
	}
}

// TestRunRulesCheck_BlankOnlyFileFallsBackToPathOnlyMatch covers
// peekFirstLine's other empty-result path: the file exists but has no
// non-blank line to sniff (e.g. only whitespace so far), so it must behave
// like no content was available rather than erroring.
func TestRunRulesCheck_BlankOnlyFileFallsBackToPathOnlyMatch(t *testing.T) {
	repo := initRulesCheckTestRepo(t)
	writeRulesCheckTestFile(t, repo, "Models/blank.m", "\n   \n\t\n")
	setRulesCheckRepo(t, repo)

	got := captureStdout(t, func() {
		if err := runRulesCheck("Models/blank.m"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(got, "Pattern: **/*.m") {
		t.Errorf("expected the matlab pattern to match by path alone, got:\n%s", got)
	}
}

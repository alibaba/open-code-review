// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeProjectRule gives repo a .opencodereview/rule.json whose only rule
// covers src/app/**, so a lookup that reaches it reports the project layer.
func writeProjectRule(t *testing.T, repo string) {
	t.Helper()
	dir := filepath.Join(repo, ".opencodereview")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rule := `{"rules":[{"path":"src/app/**","rule":"APP RULE"}]}`
	if err := os.WriteFile(filepath.Join(dir, "rule.json"), []byte(rule), 0o644); err != nil {
		t.Fatal(err)
	}
}

// filePathSpellings are ways a shell hands over the same repository file:
// typed by hand, tab-completed with a "./" prefix, OS separators (backslashes
// on Windows, where PowerShell completion produces them), and absolute paths.
// The absolute path keeps the temp dir's spelling, which git does not: it
// resolves macOS /var -> /private/var and Windows 8.3 names such as RUNNER~1.
func filePathSpellings(t *testing.T, repo string) map[string]string {
	t.Helper()
	spellings := map[string]string{
		"repo-relative":   "src/app/main.go",
		"dot prefix":      "./src/app/main.go",
		"os separators":   filepath.FromSlash("./src/app/main.go"),
		"absolute":        filepath.Join(repo, "src", "app", "main.go"),
		"doubled slashes": "src//app/main.go",
	}
	link := filepath.Join(t.TempDir(), "repo-link")
	if err := os.Symlink(repo, link); err == nil {
		spellings["absolute through a symlink"] = filepath.Join(link, "src", "app", "main.go")
	} else {
		t.Logf("no symlink spelling: %v", err)
	}
	return spellings
}

func TestRunRulesCheckAcceptsPathSpellings(t *testing.T) {
	dir := initTestGitRepo(t)
	writeProjectRule(t, dir)

	prev := rulesCheckRepoDir
	rulesCheckRepoDir = dir
	t.Cleanup(func() { rulesCheckRepoDir = prev })

	for name, arg := range filePathSpellings(t, dir) {
		t.Run(name, func(t *testing.T) {
			var runErr error
			out := captureStdout(t, func() { runErr = runRulesCheck(arg) })
			if runErr != nil {
				t.Fatalf("runRulesCheck(%q) error: %v", arg, runErr)
			}
			if !strings.Contains(out, "File: src/app/main.go\n") ||
				!strings.Contains(out, "Source: Project (.opencodereview/rule.json)\n") ||
				!strings.Contains(out, "Pattern: src/app/**\n") {
				t.Fatalf("runRulesCheck(%q) did not resolve the project rule:\n%s", arg, out)
			}
		})
	}
}

// TestRunRulesCheck drives the full `ocr rules check` helper against a real git
// repo so the resolver-build, DetailResolver assertion, and formatted print
// path all run.
func TestRunRulesCheck(t *testing.T) {
	dir := initTestGitRepo(t)

	// runRulesCheck reads the package-level flag var; set and restore it.
	prev := rulesCheckRepoDir
	rulesCheckRepoDir = dir
	t.Cleanup(func() { rulesCheckRepoDir = prev })

	t.Run("resolves a rule for a Go file", func(t *testing.T) {
		silenceStdout(t, func() {
			if err := runRulesCheck("internal/foo/bar.go"); err != nil {
				t.Fatalf("runRulesCheck error: %v", err)
			}
		})
	})

	t.Run("non-git repo dir errors", func(t *testing.T) {
		rulesCheckRepoDir = t.TempDir()
		defer func() { rulesCheckRepoDir = dir }()
		if err := runRulesCheck("x.go"); err == nil {
			t.Fatal("expected error for non-git repo dir")
		}
	})
}

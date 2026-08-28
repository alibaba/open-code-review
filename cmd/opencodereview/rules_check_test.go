// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// TestRunRulesCheck_RuleFilesOutput verifies `ocr rules check` prints a
// "Rule Files:" line for both array rules and single-file rules.
func TestRunRulesCheck_RuleFilesOutput(t *testing.T) {
	t.Run("array rule lists each file", func(t *testing.T) {
		setTestHome(t, t.TempDir()) // isolate from real ~/.opencodereview
		dir := initTestGitRepo(t)
		if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("General review guidance.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("Kotlin-specific guidance.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfgDir := filepath.Join(dir, ".opencodereview")
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfgDir, "rule.json"),
			[]byte(`{"rules":[{"path":"**/*.kt","rule":["a.md","b.md"]}]}`), 0o644); err != nil {
			t.Fatal(err)
		}

		setRulesCheckRepo(t, dir)

		var runErr error
		out := captureStdout(t, func() {
			runErr = runRulesCheck("src/Foo.kt")
		})
		if runErr != nil {
			t.Fatalf("runRulesCheck error: %v", runErr)
		}
		if !strings.Contains(out, "Rule Files: a.md, b.md") {
			t.Errorf("expected 'Rule Files: a.md, b.md' line, got:\n%s", out)
		}
	})

	t.Run("single file rule prints one file", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		dir := initTestGitRepo(t)
		if err := os.WriteFile(filepath.Join(dir, "kotlin.md"), []byte("Kotlin rules\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfgDir := filepath.Join(dir, ".opencodereview")
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfgDir, "rule.json"),
			[]byte(`{"rules":[{"path":"**/*.kt","rule":"kotlin.md"}]}`), 0o644); err != nil {
			t.Fatal(err)
		}

		setRulesCheckRepo(t, dir)

		var runErr error
		out := captureStdout(t, func() {
			runErr = runRulesCheck("src/Foo.kt")
		})
		if runErr != nil {
			t.Fatalf("runRulesCheck error: %v", runErr)
		}
		if !strings.Contains(out, "Rule Files: kotlin.md") {
			t.Errorf("expected 'Rule Files: kotlin.md' line, got:\n%s", out)
		}
	})

	t.Run("missing single file prints no Rule Files line", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		dir := initTestGitRepo(t)
		cfgDir := filepath.Join(dir, ".opencodereview")
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Referencing a single .md file that does not exist yields no "Rule
		// Files:" line: the entry resolves to no rule body of its own and
		// falls back to the system built-in rule, which has no rule files to
		// report.
		if err := os.WriteFile(filepath.Join(cfgDir, "rule.json"),
			[]byte(`{"rules":[{"path":"**/*.kt","rule":"missing.md"}]}`), 0o644); err != nil {
			t.Fatal(err)
		}

		setRulesCheckRepo(t, dir)

		var runErr error
		out := captureStdout(t, func() {
			runErr = runRulesCheck("src/Foo.kt")
		})
		if runErr != nil {
			t.Fatalf("runRulesCheck error: %v", runErr)
		}
		if strings.Contains(out, "Rule Files:") {
			t.Errorf("missing single file must not print a Rule Files line, got:\n%s", out)
		}
	})

	t.Run("empty single element array with merge_system_rule prints no Rule Files line", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		dir := initTestGitRepo(t)
		cfgDir := filepath.Join(dir, ".opencodereview")
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		// rule:[""] with merge_system_rule:true merges with the system
		// rule. The empty element contributes neither a rule file nor an
		// inline body, so no "Rule Files:" line appears — and not a spurious
		// "<inline>" label.
		if err := os.WriteFile(filepath.Join(cfgDir, "rule.json"),
			[]byte(`{"rules":[{"path":"**/*.kt","merge_system_rule":true,"rule":[""]}]}`), 0o644); err != nil {
			t.Fatal(err)
		}

		setRulesCheckRepo(t, dir)

		var runErr error
		out := captureStdout(t, func() {
			runErr = runRulesCheck("src/Foo.kt")
		})
		if runErr != nil {
			t.Fatalf("runRulesCheck error: %v", runErr)
		}
		if strings.Contains(out, "Rule Files:") {
			t.Errorf("empty [\"\"] must not print a Rule Files line, got:\n%s", out)
		}
	})

	t.Run("whitespace single value prints Rule Files inline", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		dir := initTestGitRepo(t)
		cfgDir := filepath.Join(dir, ".opencodereview")
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		// A whitespace-only single-value rule is treated as an inline rule
		// regardless of its content, so it is labeled "<inline>" — uniform
		// with any other inline rule, not suppressed.
		if err := os.WriteFile(filepath.Join(cfgDir, "rule.json"),
			[]byte(`{"rules":[{"path":"**/*.kt","rule":"  "}]}`), 0o644); err != nil {
			t.Fatal(err)
		}

		setRulesCheckRepo(t, dir)

		var runErr error
		out := captureStdout(t, func() {
			runErr = runRulesCheck("src/Foo.kt")
		})
		if runErr != nil {
			t.Fatalf("runRulesCheck error: %v", runErr)
		}
		if !strings.Contains(out, "Rule Files: <inline>") {
			t.Errorf("whitespace inline rule should print 'Rule Files: <inline>', got:\n%s", out)
		}
	})
}

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

	t.Run("non-git repo dir still resolves (#1704)", func(t *testing.T) {
		plain := t.TempDir()
		rulesCheckRepoDir = plain
		defer func() { rulesCheckRepoDir = dir }()

		// A distinctive project rule proves the project layer is really
		// consulted for the non-git dir: exit-0 alone would still pass if
		// rules were silently loaded from another root.
		ruleDir := filepath.Join(plain, ".opencodereview")
		if err := os.MkdirAll(ruleDir, 0o755); err != nil {
			t.Fatal(err)
		}
		ruleJSON := `{"rules":[{"path":"**/*.ruleprobe","rule":"DISTINCTIVE PROJECT RULE"}]}`
		if err := os.WriteFile(filepath.Join(ruleDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
			t.Fatal(err)
		}

		out := captureStdout(t, func() {
			if err := runRulesCheck("pkg/RuleProbe.ruleprobe"); err != nil {
				t.Fatalf("runRulesCheck error for non-git dir: %v", err)
			}
		})
		for _, want := range []string{
			"Source: Project (.opencodereview/rule.json)",
			"Pattern: **/*.ruleprobe",
			"DISTINCTIVE PROJECT RULE",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q, got:\n%s", want, out)
			}
		}
	})
}

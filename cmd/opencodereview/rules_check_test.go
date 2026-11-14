// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
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
		rulesCheckRepoDir = t.TempDir()
		defer func() { rulesCheckRepoDir = dir }()
		silenceStdout(t, func() {
			if err := runRulesCheck("x.go"); err != nil {
				t.Fatalf("runRulesCheck error for non-git dir: %v", err)
			}
		})
	})
}

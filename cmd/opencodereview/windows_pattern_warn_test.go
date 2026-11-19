// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/rules"
)

// A Windows-style pattern that arrives only from rule.json — with no CLI
// --exclude on the command line — must still be reported. applyCLIExcludes
// returns early when it has nothing to append, so a filter built by
// NewResolver from the project layer used to reach the user unreported.
func TestWindowsPatternWarning_CoversProjectRuleWithoutCLIExcludes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("warning is a Windows-only affordance")
	}

	cc := &commonContext{FileFilter: &rules.FileFilter{Exclude: []string{`src\generated\**`}}}

	// The CLI supplied no --exclude at all, which is the reported gap.
	got := captureStderr(t, func() {
		applyCLIExcludes(cc, nil)
	})

	if !strings.Contains(got, `"src\\generated\\**"`) {
		t.Errorf("a project-rule Windows pattern was not reported without CLI excludes, got:\n%s", got)
	}
}

// The filter is the single source of truth, so the report must happen after
// every layer has been merged. Appending CLI patterns to a filter that already
// carries a project-rule pattern must still name each pattern once.
func TestWindowsPatternWarning_AfterMergeNamesEachPatternOnce(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("warning is a Windows-only affordance")
	}

	cc := &commonContext{FileFilter: &rules.FileFilter{Exclude: []string{`src\gen\*`}}}

	got := captureStderr(t, func() {
		applyCLIExcludes(cc, []string{`docs\out\*`})
	})

	if n := strings.Count(got, `"src\\gen\\*"`); n != 1 {
		t.Errorf("project-rule pattern reported %d times, want 1:\n%s", n, got)
	}
	if n := strings.Count(got, `"docs\\out\\*"`); n != 1 {
		t.Errorf("CLI pattern reported %d times, want 1:\n%s", n, got)
	}
}

// A run with no Windows-style patterns anywhere must stay silent.
func TestWindowsPatternWarning_QuietWhenNothingLooksLikeWindows(t *testing.T) {
	cc := &commonContext{FileFilter: &rules.FileFilter{Exclude: []string{"src/gen/*", "*.go"}}}

	got := captureStderr(t, func() {
		applyCLIExcludes(cc, nil)
	})

	if got != "" {
		t.Errorf("clean filter produced output: %q", got)
	}
}

// A nil filter, and a context that never built one, must not panic.
func TestWindowsPatternWarning_NilFilterIsQuiet(t *testing.T) {
	got := captureStderr(t, func() {
		applyCLIExcludes(&commonContext{}, nil)
		applyCLIExcludes(&commonContext{FileFilter: nil}, nil)
	})

	if got != "" {
		t.Errorf("nil filter produced output: %q", got)
	}
}

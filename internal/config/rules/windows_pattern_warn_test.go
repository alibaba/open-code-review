// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"runtime"
	"strings"
	"testing"
)

// The scenario the duplicate-warning bug came from: NewResolver builds the
// filter from rule.json layers, then applyCLIExcludes appends CLI patterns to
// that same filter and reports. Each pattern must be named exactly once, or a
// user with a project rule sees the same line twice.
func TestWindowsPatternWarnings_EachPatternNamedOnce(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("warning is a Windows-only affordance")
	}

	// What buildFileFilter produces from a project layer.
	f := buildFileFilter(&ProjectRule{Exclude: []string{`src\gen\*`}})
	if f == nil {
		t.Fatal("buildFileFilter returned nil")
	}
	// What applyCLIExcludes then appends.
	f.Exclude = append(f.Exclude, `docs\out\*`)

	got := WindowsPatternWarnings(f)
	if len(got) != 2 {
		t.Fatalf("WindowsPatternWarnings returned %d warnings, want 2: %q", len(got), got)
	}
	if got[0] == got[1] {
		t.Errorf("the same warning was emitted twice: %q", got[0])
	}
	// %q doubles each backslash.
	for i, want := range []string{`"src\\gen\\*"`, `"docs\\out\\*"`} {
		if !strings.Contains(got[i], want) {
			t.Errorf("warning %d = %q, want it to name %q", i, got[i], want)
		}
	}
}

// A clean filter yields nothing, and a nil filter does not panic.
func TestWindowsPatternWarnings_QuietCases(t *testing.T) {
	if got := WindowsPatternWarnings(&FileFilter{Exclude: []string{`src/gen/*`, `*.go`}}); len(got) != 0 {
		t.Errorf("clean filter produced %d warnings, want 0: %q", len(got), got)
	}
	if got := WindowsPatternWarnings(nil); len(got) != 0 {
		t.Errorf("nil filter produced %d warnings, want 0", len(got))
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"runtime"
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
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
	// `src\/gen` is a working pattern: doublestar reads the pair as an escaped
	// slash and matches `src/gen`, so it must not be reported at all.
	if got := WindowsPatternWarnings(&FileFilter{Exclude: []string{`src\/gen`}}); len(got) != 0 {
		t.Errorf("escaped slash produced %d warnings, want 0: %q", len(got), got)
	}
	if got := WindowsPatternWarnings(nil); len(got) != 0 {
		t.Errorf("nil filter produced %d warnings, want 0", len(got))
	}
}

// The same pattern can arrive from rule.json and from --exclude. The merged
// filter keeps both copies, but a user can only act on one line, so the report
// has to collapse them. The filter itself is left alone.
func TestWindowsPatternWarnings_DuplicatePatternNamedOnce(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("warning is a Windows-only affordance")
	}

	f := buildFileFilter(&ProjectRule{Exclude: []string{`src\gen\*`}})
	if f == nil {
		t.Fatal("buildFileFilter returned nil")
	}
	f.Exclude = append(f.Exclude, `src\gen\*`)
	if len(f.Exclude) != 2 {
		t.Fatalf("filter should keep both copies, got %q", f.Exclude)
	}

	got := WindowsPatternWarnings(f)
	if len(got) != 1 {
		t.Fatalf("WindowsPatternWarnings returned %d warnings, want 1: %q", len(got), got)
	}
	if !strings.Contains(got[0], `"src\\gen\\*"`) {
		t.Errorf("warning = %q, want it to name the duplicated pattern", got[0])
	}

	// A pattern repeated in the include group is deduplicated there as well,
	// and the two groups still report their own line.
	both := WindowsPatternWarnings(&FileFilter{
		Include: []string{`src\inc`, `src\inc`},
		Exclude: []string{`src\inc`},
	})
	if len(both) != 2 {
		t.Fatalf("got %d warnings, want one per group: %q", len(both), both)
	}
}

// A trailing backslash is inert rather than a separator, so it survives into
// the suggestion, and `src/gen\` is not a valid pattern. The warning is right —
// the pattern matches nothing — but offering that repair would hand over a
// second broken pattern.
func TestWindowsPatternWarnings_TrailingBackslashGetsNoRewrite(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("warning is a Windows-only affordance")
	}

	got := WindowsPatternWarnings(&FileFilter{Exclude: []string{`src\gen\`}})
	if len(got) != 1 {
		t.Fatalf("WindowsPatternWarnings returned %d warnings, want 1: %q", len(got), got)
	}
	if strings.Contains(got[0], "write it as") {
		t.Errorf("warning offers a rewrite whose result is not a valid pattern: %q", got[0])
	}

	// Pin the reason: the withheld suggestion really is invalid, so this test
	// fails if doublestar ever starts accepting a trailing backslash and the
	// advice becomes correct to give.
	if doublestar.ValidatePattern(`src/gen\`) {
		t.Errorf("ValidatePattern(%q) = true; the advice could be offered again", `src/gen\`)
	}
	if !strings.Contains(got[0], "looks like a Windows path") {
		t.Errorf("warning lost its diagnosis: %q", got[0])
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"runtime"
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
)

// The suggested replacement must keep the pattern's glob semantics. Rewriting
// every backslash turns `a\b\\*` — where `\b` is an escape and `\\*` is an
// escaped backslash followed by a star — into `a/b//*`, which matches different
// files. Only the backslashes the heuristic identified as separators may move.
func TestSuggestedSlashForm_PreservesGlobSemantics(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		// A pattern that already works must come back untouched: rewriting its
		// escapes would change what it matches.
		{"escaped star is preserved", `foo\**`, `foo\**`},
		{"escaped bracket is preserved", `a\[b`, `a\[b`},
		{"no backslash at all", `src/gen/*`, `src/gen/*`},

		// A separator becomes a slash; the escape around the star stays.
		{"separator becomes a slash", `src\gen\*`, `src/gen\*`},
		{"drive letter", `c:\gen`, `c:/gen`},
		// The escaped backslash stays an escaped backslash. This is the case
		// where rewriting every backslash used to break the semantics.
		{"escaped backslash stays escaped", `a\b\\*`, `a/b\\*`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := suggestedSlashForm(tt.pattern); got != tt.want {
				t.Errorf("suggestedSlashForm(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

// Whatever the warning prints as "write it as ..." must match the file the user
// meant. That is the only reason to print a suggestion at all.
func TestSuggestedSlashForm_MatchesWhatTheUserMeant(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
	}{
		{`src\gen\file.go`, "src/gen/file.go"},
		{`src\gen`, "src/gen"},
		{`c:\gen`, "c:/gen"},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := suggestedSlashForm(tt.pattern)
			if matched, err := doublestar.Match(got, tt.path); err != nil || !matched {
				t.Errorf("suggestion %q did not match %q: matched=%v err=%v", got, tt.path, matched, err)
			}
		})
	}
}

// An escape the heuristic leaves alone must survive the rewrite untouched. The
// heuristic reads `\*` and `\[` as escapes, so a suggestion that converted them
// would change the pattern's meaning.
func TestSuggestedSlashForm_LeavesRecognizedEscapesAlone(t *testing.T) {
	for _, pattern := range []string{`foo\**`, `a\[b`, `src\?gen`, `src\\gen`} {
		if got := suggestedSlashForm(pattern); got != pattern {
			t.Errorf("suggestedSlashForm(%q) = %q, want it unchanged", pattern, got)
		}
	}
}

// The rewrite must not change what a pattern matches. For a pattern that
// already works the suggestion must match exactly the same paths; for a broken
// one the suggestion is meant to start matching the path the user intended.
// This is the property the blanket ReplaceAll broke.
func TestSuggestedSlashForm_PreservesMatching(t *testing.T) {
	paths := []string{"a/b/*", "a/b/x", "ab*", "a/b/", `a\b\*`, "gen*", "src/gen/file.go"}

	// Patterns that already match something: the suggestion must agree with the
	// original on every path, so no working pattern is silently altered.
	working := []string{`a\b\\*`, `foo\**`, `a\[b`}
	for _, pattern := range working {
		got := suggestedSlashForm(pattern)
		for _, p := range paths {
			want := false
			if m, err := doublestar.Match(pattern, p); err == nil {
				want = m
			}
			m, err := doublestar.Match(got, p)
			if err != nil {
				t.Fatalf("suggestion %q is not a valid pattern: %v", got, err)
			}
			if m != want {
				t.Errorf("pattern %q matches %q = %v; suggestion %q matches it = %v; the rewrite changed the meaning",
					pattern, p, want, got, m)
			}
		}
	}

	// Broken patterns: the suggestion must start matching the slash-separated
	// path the user meant, which is the entire point of printing it.
	broken := [][2]string{
		{`src\gen\file.go`, "src/gen/file.go"},
		{`c:\gen`, "c:/gen"},
	}
	for _, pair := range broken {
		pattern, path := pair[0], pair[1]
		if m, err := doublestar.Match(pattern, path); err == nil && m {
			t.Fatalf("precondition: %q already matches %q, so it is not a broken pattern", pattern, path)
		}
		if m, err := doublestar.Match(suggestedSlashForm(pattern), path); err != nil || !m {
			t.Errorf("suggestion for %q did not start matching %q: matched=%v err=%v", pattern, path, m, err)
		}
	}
}

// A pattern the heuristic cannot read unambiguously must not get a rewrite
// printed as advice. `src\gen\*` keeps its `\*` as an escape, which yields a
// suggestion matching nothing — offering it would silently pick a reading for
// the user.
func TestUnambiguousRewrite_WithholdsAdviceWhenAmbiguous(t *testing.T) {
	if unambiguousRewrite(`src\gen\*`, suggestedSlashForm(`src\gen\*`)) {
		t.Errorf(`src\gen\* should be treated as ambiguous`)
	}
	// Only separators moved, no escape survived: safe to advise.
	if !unambiguousRewrite(`src\gen\file.go`, suggestedSlashForm(`src\gen\file.go`)) {
		t.Errorf(`src\gen\file.go should be an unambiguous rewrite`)
	}
	if !unambiguousRewrite(`c:\gen`, suggestedSlashForm(`c:\gen`)) {
		t.Errorf(`c:\gen should be an unambiguous rewrite`)
	}
}

// The warning text must carry the advice exactly when the rewrite is
// unambiguous, and must still name the pattern when it is not.
func TestWindowsPatternWarnings_AdviceOnlyWhenUnambiguous(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("warning is a Windows-only affordance")
	}

	got := WindowsPatternWarnings(&FileFilter{
		Exclude: []string{`src\gen\file.go`, `src\gen\*`},
	})
	if len(got) != 2 {
		t.Fatalf("got %d warnings, want 2: %q", len(got), got)
	}
	if !strings.Contains(got[0], `"src/gen/file.go"`) {
		t.Errorf("unambiguous pattern should be advised, got:\n%s", got[0])
	}
	if strings.Contains(got[1], "write it as") {
		t.Errorf("ambiguous pattern should not be advised, got:\n%s", got[1])
	}
	if !strings.Contains(got[1], `"src\\gen\\*"`) {
		t.Errorf("ambiguous pattern should still be named, got:\n%s", got[1])
	}
}

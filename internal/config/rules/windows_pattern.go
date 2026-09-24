// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// globEscapable lists the characters a backslash is allowed to escape in a
// doublestar pattern. A backslash before one of these is a real escape and is
// left alone.
//
// doublestar escapes the byte after a backslash unconditionally, so strictly
// speaking any `\x` is an escape. Restricting the set to the glob
// metacharacters is what makes this a useful signal: `src\gen` is far more
// likely to be a Windows path than a pattern for a file literally named
// "src\gen", and a pattern for a literal backslash in a filename is written
// `\\` anyway.
const globEscapable = `*?[]{}\!,\`

// scanBackslashes walks pattern once and reports, for each backslash, whether
// it is doing the job of a path separator. A trailing backslash is inert and
// an escaped backslash consumes both characters, so neither separates.
//
// The two callers below share this one walk on purpose: the predicate that
// decides a pattern is broken and the rewrite that fixes it must agree on
// which backslashes are separators, or the suggestion would change the
// pattern's glob semantics.
func scanBackslashes(pattern string) []bool {
	separator := make([]bool, len(pattern))
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '\\' {
			continue
		}
		if i+1 >= len(pattern) {
			// Trailing backslash: escapes nothing, separates nothing.
			return separator
		}
		if pattern[i+1] == '\\' {
			// An escaped backslash consumes both characters; the second one
			// cannot open a new escape of its own.
			i++
			continue
		}
		if !strings.ContainsRune(globEscapable, rune(pattern[i+1])) {
			separator[i] = true
		}
	}
	return separator
}

// looksLikeWindowsPath reports whether pattern holds a backslash that is doing
// the job of a path separator rather than a glob escape.
//
// Include/exclude patterns are matched against the slash-separated paths git
// reports, so a backslash is only meaningful as an escape. On Windows a user
// typing a native path writes `src\gen\*`, and doublestar reads that as "src",
// literal "g", "en", literal "*" — which matches nothing. The failure is
// silent: the pattern is simply never satisfied.
//
// The tell is the character after the backslash. Before a glob metacharacter
// (`*?[]{}\`) it is a real escape and must be left alone; before anything else
// it is most likely a separator the user meant as "/". A trailing backslash
// escapes nothing and separates nothing, so it is inert.
//
// This is a heuristic and deliberately reported as a warning rather than acted
// on: where a backslash precedes a metacharacter the intent is genuinely
// ambiguous, and silently rewriting the pattern would change matching
// semantics without the user's agreement.
func looksLikeWindowsPath(pattern string) bool {
	for _, isSeparator := range scanBackslashes(pattern) {
		if isSeparator {
			return true
		}
	}
	return false
}

// suggestedSlashForm rewrites only the backslashes scanBackslashes identified
// as separators, leaving every escape in place. Rewriting all of them instead
// would silently change the pattern's meaning: `a\b\\*` reads as an escaped
// "b" followed by an escaped backslash and a star, and turning that into
// `a/b//*` makes it match different files.
//
// The rewrite is safe but not always useful. `src\gen\*` leaves the `\*` alone
// because the heuristic reads it as an escape, which produces `src/gen\*` — a
// pattern that matches nothing. Whether the user meant a glob star or a literal
// one is exactly the ambiguity the heuristic cannot resolve, so the caller
// checks unambiguousRewrite before printing the result as advice.
func suggestedSlashForm(pattern string) string {
	separator := scanBackslashes(pattern)
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		if separator[i] {
			b.WriteByte('/')
			continue
		}
		b.WriteByte(pattern[i])
	}
	return b.String()
}

// unambiguousRewrite reports whether suggestedSlashForm produced a pattern that
// keeps the escapes the heuristic recognized and turns nothing else into an
// escape. A surviving escape means the rewrite is only one reading of an
// ambiguous pattern, and offering it as "write it as ..." would ask the user to
// accept that reading silently.
func unambiguousRewrite(pattern, rewritten string) bool {
	if rewritten == pattern {
		return true
	}
	separator := scanBackslashes(pattern)
	for i := 0; i < len(pattern); i++ {
		if separator[i] || pattern[i] != '\\' {
			continue
		}
		// A backslash the heuristic kept as an escape survives into the
		// suggestion, so the suggestion depends on that reading.
		if i+1 < len(pattern) && pattern[i+1] != '\\' {
			return false
		}
		if i+1 < len(pattern) && pattern[i+1] == '\\' {
			i++
		}
	}
	return true
}

// WindowsPatternWarnings describes every include/exclude pattern in f that
// looks like a Windows path, in a form ready to print. It returns nil unless
// running on Windows, where the ambiguity exists at all, and for a nil filter.
//
// Returning the text rather than writing it keeps this package free of an
// output decision and lets the caller emit it once, after all layers are
// merged — which is what stops a project-rule pattern from being reported a
// second time when CLI --exclude patterns are appended to the same filter.
//
// A rewrite is offered only when the separator conversion is unambiguous. Where
// a backslash could be either a separator or an escape the warning names the
// pattern and stops, because any automatic rewrite would pick a reading on the
// user's behalf.
func WindowsPatternWarnings(f *FileFilter) []string {
	if f == nil || runtime.GOOS != "windows" {
		return nil
	}
	var warnings []string
	for _, group := range []struct {
		label    string
		patterns []string
	}{
		{"include", f.Include},
		{"exclude", f.Exclude},
	} {
		for _, pattern := range group.patterns {
			if !looksLikeWindowsPath(pattern) {
				continue
			}
			rewritten := suggestedSlashForm(pattern)
			if rewritten == pattern {
				continue
			}
			warning := fmt.Sprintf(
				"[ocr] WARNING: %s pattern %q looks like a Windows path and will never match; "+
					"patterns are matched against slash-separated paths",
				group.label, pattern)
			if unambiguousRewrite(pattern, rewritten) {
				warning += fmt.Sprintf(", so write it as %q", rewritten)
			}
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

// WarnAboutWindowsPatterns prints every warning WindowsPatternWarnings
// reports. Matching semantics are untouched: converting the separator is a
// semantic decision (see the ambiguity where `\` precedes a metacharacter) and
// belongs to a separate, explicitly agreed change. This only turns a silent
// no-op into a visible one.
func WarnAboutWindowsPatterns(f *FileFilter) {
	for _, warning := range WindowsPatternWarnings(f) {
		fmt.Fprintln(os.Stderr, warning)
	}
}

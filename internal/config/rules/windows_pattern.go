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
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '\\' {
			continue
		}
		if i+1 >= len(pattern) {
			// Trailing backslash: escapes nothing, separates nothing.
			return false
		}
		if pattern[i+1] == '\\' {
			// An escaped backslash consumes both characters; the second one
			// cannot open a new escape of its own.
			i++
			continue
		}
		if !strings.ContainsRune(globEscapable, rune(pattern[i+1])) {
			return true
		}
	}
	return false
}

// WindowsPatternWarnings describes every include/exclude pattern in f that
// looks like a Windows path, in a form ready to print. It returns nil unless
// running on Windows, where the ambiguity exists at all, and for a nil filter.
//
// Returning the text rather than writing it keeps this package free of an
// output decision and lets the caller emit it once, after all layers are
// merged — which is what stops a project-rule pattern from being reported a
// second time when CLI --exclude patterns are appended to the same filter.
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
			if looksLikeWindowsPath(pattern) {
				warnings = append(warnings, fmt.Sprintf(
					"[ocr] WARNING: %s pattern %q looks like a Windows path and will never match; "+
						"patterns are matched against slash-separated paths, so write it as %q",
					group.label, pattern, strings.ReplaceAll(pattern, `\`, "/")))
			}
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

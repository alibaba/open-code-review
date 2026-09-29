// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package pathutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CanonicalPath returns an absolute path with symlinks resolved.
func CanonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// WithinBase reports whether target is base itself or contained under base.
func WithinBase(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		rel = ".."
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
		return true
	}

	return sameFileWithinBase(base, target)
}

func sameFileWithinBase(base, target string) bool {
	if !filepath.IsAbs(base) || !filepath.IsAbs(target) {
		return false
	}
	baseInfo, err := os.Stat(base)
	if err != nil {
		return false
	}
	for cur := target; ; {
		info, err := os.Stat(cur)
		if err == nil && os.SameFile(baseInfo, info) {
			return true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return false
		}
		cur = parent
	}
}

// plausibleEscapes lists the characters whose escaped form is trusted as an
// intentional literal: brackets, braces, comma, bang and a second backslash.
// '*' and '?' are deliberately absent. Windows forbids both in file names, so
// an escaped `\*` can never match a path there; the backslash that produced it
// separates directories, and reading it as an escape is what made patterns
// like `node_modules\**` fail silently (#1463).
const plausibleEscapes = `[]{}\!,`

// patternBackslashIssue reports whether pattern contains a backslash that
// doublestar will read as an escape instead of a native path separator: one
// followed by any character outside plausibleEscapes, or a dangling trailing
// backslash (doublestar returns ErrBadPattern for it, so the pattern matches
// nothing). Trusted escapes such as `src/\[generated\]/file.go` are not
// reported.
func patternBackslashIssue(pattern string) bool {
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '\\' {
			continue
		}
		if i+1 >= len(pattern) {
			return true
		}
		if !strings.ContainsRune(plausibleEscapes, rune(pattern[i+1])) {
			return true
		}
		i++ // skip the escaped character
	}
	return false
}

// WarnPatternBackslashes writes one [ocr] WARNING line per pattern whose
// backslashes doublestar will read as escapes instead of path separators.
// It is the visibility half of #1463: matching behavior is intentionally
// left unchanged. goos is runtime.GOOS at the call sites and a parameter so
// tests can exercise the Windows branch on any platform; warnings only fire
// on windows. origin names where the pattern came from, e.g. `--exclude` or
// `rule.json "exclude"`.
func WarnPatternBackslashes(w io.Writer, goos, origin string, patterns []string) {
	if goos != "windows" {
		return
	}
	for _, pattern := range patterns {
		if !patternBackslashIssue(pattern) {
			continue
		}
		fmt.Fprintf(w, "[ocr] WARNING: %s pattern %q uses backslash separators; doublestar treats '\\' as an escape, so the pattern will not match the intended paths. Use '/' separators instead.\n", origin, pattern)
	}
}

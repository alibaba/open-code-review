// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package allowedext provides file-level filtering for code review:
// an extension allowlist (which file types to review) and a path-based
// exclude list (which files to skip regardless of extension).
//
// # The default_exclude_patterns.json format
//
// The file holds a JSON array of strings, each one a glob exclude pattern.
// Supported wildcard syntax (as implemented by the doublestar library):
//
//	"*"        matches any character within a single path segment (never crosses /)
//	           e.g. "*_test.go" matches "foo_test.go" but not "pkg/foo_test.go"
//
//	"**"       matches zero or more path segments (may cross /)
//	           e.g. "**/*_test.go" matches both "foo_test.go" and "a/b/c_test.go"
//
//	"{a,b,c}"  brace expansion, matches any one of the alternatives
//	           e.g. "**/*.{js,ts}" matches .js and .ts files at any depth
//
// Combined examples:
//
//	"**/*_test.go"                 — Go test files at any depth
//	"**/src/test/java/**/*.java"   — everything under the standard Java test directory
//	"**/*.spec.{js,jsx,ts,tsx}"    — frontend spec test files at any depth
//	"*_test.go"                    — Go test files in the root directory only (does not cross directories)
package allowedext

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
)

//go:embed supported_file_types.json
var defaultData []byte

//go:embed default_exclude_patterns.json
var excludeData []byte

var (
	supported map[string]bool
	initOnce  sync.Once
)

// excludePattern is a default exclude pattern prepared for case-insensitive
// matching. wordTail is set when the pattern's file name is "*" followed by a
// capitalized word, such as "*Test.java" or "*Spec.hs": it holds the literal
// text after the "*" in its original case, so that the word boundary the
// capital letter marks survives lowercasing.
type excludePattern struct {
	glob     string // lowercased pattern (may contain {a,b} syntax)
	wordTail string
}

var (
	excludePatterns []excludePattern
	excludeOnce     sync.Once
)

func initMap() {
	var exts []string
	if err := json.Unmarshal(defaultData, &exts); err != nil {
		panic("allowedext: failed to parse supported_file_types.json: " + err.Error())
	}
	supported = make(map[string]bool, len(exts))
	for _, e := range exts {
		supported[strings.ToLower(e)] = true
	}
}

func initExclude() {
	var raw []string
	if err := json.Unmarshal(excludeData, &raw); err != nil {
		panic("allowedext: failed to parse default_exclude_patterns.json: " + err.Error())
	}
	excludePatterns = make([]excludePattern, len(raw))
	for i, p := range raw {
		excludePatterns[i] = excludePattern{glob: strings.ToLower(p), wordTail: camelWordTail(p)}
	}
}

// baseName returns the last segment of a slash-separated path or pattern.
func baseName(p string) string {
	return p[strings.LastIndexByte(p, '/')+1:]
}

// camelWordTail returns the text after the leading "*" of the pattern's file
// name when that text starts with an upper-case letter and holds no further
// glob syntax, and "" otherwise.
func camelWordTail(pattern string) string {
	tail, ok := strings.CutPrefix(baseName(pattern), "*")
	if !ok || strings.ContainsAny(tail, `*?[]{}\`) {
		return ""
	}
	if first, _ := utf8.DecodeRuneInString(tail); !unicode.IsUpper(first) {
		return ""
	}
	return tail
}

// startsWord reports whether the file name of p ends in tail, compared
// case-insensitively, with tail beginning a new word: either its first letter
// is upper-case ("FooTest.java", "FooTEST.java") or nothing or a non-letter
// precedes it ("Test.java", "foo_test.java"). The end of one longer word
// ("Contest.java", "Latest.swift") does not count.
func startsWord(p, tail string) bool {
	base := baseName(p)
	cut := len(base) - len(tail)
	if cut < 0 || !strings.EqualFold(base[cut:], tail) {
		return false
	}
	if first, _ := utf8.DecodeRuneInString(base[cut:]); unicode.IsUpper(first) {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(base[:cut])
	return cut == 0 || !unicode.IsLetter(prev)
}

// IsAllowedExt returns true when the given file extension is in the supported types list.
// The check is case-insensitive.
func IsAllowedExt(ext string) bool {
	initOnce.Do(initMap)
	return supported[strings.ToLower(ext)]
}

// IsExcludedPath returns true when the given file path matches any default exclude pattern.
// Patterns support ** (recursive directory matching), * (single-segment wildcard),
// and {a,b,c} brace expansion. The check is case-insensitive, except that a
// pattern whose file name is "*" followed by a capitalized word ("*Test.java",
// "*Spec.hs") matches only where that word starts a new word of the file name:
// "FooTest.java" and "foo_test.java" are excluded, "Contest.java" is not.
//
// Example patterns and their behavior:
//
//	"**/*_test.go"       matches "foo_test.go", "pkg/bar_test.go", "a/b/c_test.go"
//	"*_test.go"          matches "foo_test.go" only (no directory traversal)
//	"**/*.test.{js,ts}"  matches "src/app.test.js", "lib/util.test.ts"
func IsExcludedPath(path string) bool {
	excludeOnce.Do(initExclude)
	lowerPath := strings.ToLower(path)
	for _, pattern := range excludePatterns {
		matched, _ := doublestar.Match(pattern.glob, lowerPath)
		if matched && (pattern.wordTail == "" || startsWord(path, pattern.wordTail)) {
			return true
		}
	}
	return false
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build ignore

// Command verify-cjk fails when CJK characters appear in source files.
//
// Comments, identifiers and user-visible strings in this repository are
// written in English so that contributors who do not read Chinese can review
// and maintain every file. Translated content belongs in the locale-specific
// docs (README.zh-CN.md, pages/src/content/docs/zh/…) and in the i18n tables,
// not in code.
//
// Markdown is not scanned: the translated READMEs, CONTRIBUTING files and doc
// pages are legitimately non-English.
//
// Run it directly (the build tag keeps it out of ./... so it does not affect
// go vet, go build or the coverage threshold):
//
//	go run scripts/verify-cjk.go
//
// Two escape hatches exist, in order of preference:
//
//  1. Append an "allow-cjk" marker comment to the offending line, with a
//     reason — the right choice for a handful of lines, e.g. an encoding
//     fixture or a language-switcher label. The rest of the file stays
//     protected.
//
//  2. Add a prefix to allowedPrefixes below, for whole trees that are
//     inherently non-English (i18n tables) — or, temporarily, for a backlog
//     that has not been translated yet.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	// path, not path/filepath: every path here comes from git ls-files, which
	// always emits forward slashes — on Windows too, since that is how the
	// index stores them. The allowedPrefixes entries assume the same.
	"path"
	"strings"
	"unicode"
)

// scannedExts lists the extensions treated as source files.
var scannedExts = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".cjs": true,
	".mjs": true, ".py": true, ".sh": true, ".ps1": true, ".css": true,
	".html": true, ".yml": true, ".yaml": true, ".json": true,
}

// scannedNames lists extension-less files that are still source files.
var scannedNames = map[string]bool{"Makefile": true}

// allowedPrefixes exempts paths whose CJK content is expected. Keep each entry
// narrow and justified; a temporary entry must say what removes it.
var allowedPrefixes = []struct{ prefix, reason string }{
	{"pages/src/i18n/", "translated UI copy for the docs site"},
	{"extensions/vscode/", "TEMPORARY: the extension's comments, test names and zh-cn NLS bundle are still Chinese; drop this entry once they are translated"},
}

// exemptMarker on a line suppresses the report for that line.
const exemptMarker = "allow-cjk"

// isCJK reports whether r is a Han ideograph, Japanese kana, or one of the
// CJK/fullwidth punctuation forms. Punctuation matters as much as ideographs:
// a fullwidth colon (U+FF1A) or comma (U+FF0C) left in an English sentence is
// a typo that reads as correct and is invisible in review.
func isCJK(r rune) bool {
	switch {
	case unicode.Is(unicode.Han, r),
		unicode.Is(unicode.Hiragana, r),
		unicode.Is(unicode.Katakana, r):
		return true
	case r >= 0x3000 && r <= 0x303F: // CJK Symbols and Punctuation
		return true
	case r >= 0xFF00 && r <= 0xFFEF: // Halfwidth and Fullwidth Forms
		return true
	}
	return false
}

func isScanned(file string) bool {
	if scannedNames[path.Base(file)] {
		return true
	}
	return scannedExts[path.Ext(file)]
}

func allowedPrefix(file string) bool {
	for _, a := range allowedPrefixes {
		if strings.HasPrefix(file, a.prefix) {
			return true
		}
	}
	return false
}

type finding struct {
	file string
	line int
	text string
	char rune
}

func scan(file string) ([]finding, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var found []finding
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if strings.Contains(line, exemptMarker) {
			continue
		}
		for _, r := range line {
			if isCJK(r) {
				found = append(found, finding{file: file, line: n, text: strings.TrimSpace(line), char: r})
				break
			}
		}
	}
	return found, sc.Err()
}

// trim shortens a reported line so the report stays readable.
func trim(s string) string {
	const max = 100
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "…"
}

func run() error {
	// --others --exclude-standard includes files that are not committed yet, so
	// a new file is checked before it lands rather than the run after. Ignored
	// paths (dist/, node_modules/) stay out.
	out, err := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return fmt.Errorf("git ls-files: %w", err)
	}

	var findings []finding
	var scanned int
	for _, file := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if file == "" || !isScanned(file) || allowedPrefix(file) {
			continue
		}
		if _, err := os.Stat(file); err != nil {
			continue // deleted but still indexed
		}
		scanned++
		found, err := scan(file)
		if err != nil {
			return fmt.Errorf("scan %s: %w", file, err)
		}
		findings = append(findings, found...)
	}

	if len(findings) > 0 {
		fmt.Fprintf(os.Stderr, "ERROR: CJK characters found in %d line(s):\n", len(findings))
		for _, f := range findings {
			fmt.Fprintf(os.Stderr, "  %s:%d: %q in %s\n", f.file, f.line, f.char, trim(f.text))
		}
		fmt.Fprintf(os.Stderr, `
Source files are English-only: comments, identifiers and strings alike.
Translated prose belongs in README.<locale>.md, pages/src/content/docs/<locale>/
or an i18n table.

If the CJK text is intentional — an encoding fixture, a language-switcher
label — append a marker comment with a reason to that line:

    {"multibyte truncation", 6, "..."}, // %s: fixture exercises rune boundaries

For a whole tree that is inherently non-English, add a prefix to
allowedPrefixes in scripts/verify-cjk.go instead.
`, exemptMarker)
		return fmt.Errorf("%d line(s) contain CJK characters", len(findings))
	}

	fmt.Printf("No CJK characters in %d scanned source files.\n", scanned)
	return nil
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

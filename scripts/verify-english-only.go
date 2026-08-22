// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build ignore

// Command verify-english-only fails when unapproved non-English text appears in
// source files.
//
// Comments, identifiers and user-visible strings in this repository are written
// in English so that any contributor can review and maintain every file,
// whichever languages they happen to read. Translated content belongs in the
// locale-specific docs (README.zh-CN.md, pages/src/content/docs/zh/…) and in
// the i18n tables, not in code.
//
// What it detects, and the one thing it cannot:
//
//   - Detected: every letter outside ASCII, whichever the writing system. Han,
//     kana, Hangul, Cyrillic, Greek, Arabic, Hebrew and Devanagari, and equally
//     the diacritics that spell German, French, Turkish or Vietnamese. Plus CJK
//     and fullwidth punctuation, and combining accents.
//   - Not detected: another language spelled entirely in ASCII — a romanised
//     transcription, or German with its umlauts written out ("Loeschen der
//     Datei"). Telling that from English needs a dictionary rather than a
//     character test, so it stays a matter for review.
//
// Symbols are deliberately left alone: box drawing, arrows, emoji and maths
// (─ → ≥ ≈ ×) are not letters and appear throughout the TUI output on purpose.
//
// Markdown is not scanned: the translated READMEs, CONTRIBUTING files and doc
// pages are legitimately non-English.
//
// Run it directly (the build tag keeps it out of ./... so it does not affect
// go vet, go build or the coverage threshold):
//
//	go run scripts/verify-english-only.go
//
// Two escape hatches exist, in order of preference:
//
//  1. Append an "allow-non-english: <reason>" marker comment to the offending
//     line — the right choice for a handful of lines, e.g. an encoding fixture
//     or a language-switcher label. The rest of the file stays protected.
//
//  2. Add a prefix to allowedPrefixes below, for whole trees that are
//     inherently non-English (i18n tables) — or, temporarily, for a backlog
//     that has not been translated yet.
package main

import (
	"bufio"
	"errors"
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

// allowedPrefixes exempts paths whose non-English content is expected. Keep
// each entry narrow and justified; a temporary entry must say what removes it.
var allowedPrefixes = []struct{ prefix, reason string }{
	{"pages/src/i18n/", "translated UI copy for the docs site"},
	{"extensions/vscode/", "TEMPORARY: the extension's comments, test names and zh-cn NLS bundle are still Chinese; drop this entry once they are translated"},
}

// exemptMarker on a line suppresses the report for that line. The trailing
// colon is part of the marker so that a bare "allow-non-english" cannot exempt
// a line without saying why.
const exemptMarker = "allow-non-english:"

// isNonEnglish reports whether r is a letter no English word is written with,
// or one of the CJK/fullwidth punctuation forms.
//
// The rule is "a letter outside ASCII", not "a letter outside Latin". Written
// English needs no letter beyond the ASCII 26, so anything past that is another
// language: Cyrillic and Han as obviously as the diacritics of German, French or
// Turkish. Scripts are not enumerated, which keeps the rule stable as the
// contributor base grows — one that nobody has contributed in yet is covered on
// the day it arrives, with no edit here.
//
// Testing for letters, rather than for non-ASCII bytes, is what keeps symbols
// out of scope: the box drawing, arrows, emoji and maths in the TUI output are
// not letters, and neither are the em dashes used throughout these comments. A
// plain non-ASCII test would flag every one of them.
//
// Common and Inherited are the exception. Those two scripts hold the characters
// belonging to no writing system in particular, and the letterlike symbols among
// them are letters only by Unicode category: the information source (U+2139,
// category Ll) that renders as an info icon, the script small l (U+2113), the
// capitals of the maths alphabets. None of them writes a word in any language.
//
// Letterlike forms that Unicode does assign to a real script stay in scope, so
// the ohm sign (U+2126, script Greek because it is equivalent to U+03A9) is
// reported like any other Greek letter. A comment that spells sigma or omega as
// a glyph therefore needs a marker — deliberate, since exempting Greek to allow
// maths notation would exempt Greek prose with it.
//
// Punctuation is checked separately, and matters as much as letters: a
// fullwidth colon (U+FF1A) or comma (U+FF0C) left in an English sentence is a
// typo that reads as correct and is invisible in review. Vertical forms
// (U+FE10–U+FE19), CJK compatibility forms (U+FE30–U+FE4F) and small form
// variants (U+FE50–U+FE6F) are covered alongside the fullwidth block.
func isNonEnglish(r rune) bool {
	switch {
	case r < 0x80: // ASCII, the overwhelming majority of every scanned line
		return false
	case unicode.IsLetter(r) &&
		!unicode.Is(unicode.Common, r) &&
		!unicode.Is(unicode.Inherited, r):
		return true
	case r >= 0x0300 && r <= 0x036F:
		// Combining diacritical marks, so that the decomposed spelling of an
		// accented letter is caught too: NFD writes e-acute as "e" plus U+0301,
		// where the letter itself is plain ASCII and the accent carries the
		// language. Variation selectors (U+FE0F, which follows an emoji) are
		// combining marks as well, but sit outside this block and pass.
		return true
	case r >= 0x3000 && r <= 0x303F: // CJK Symbols and Punctuation
		return true
	case r >= 0xFE10 && r <= 0xFE19: // Vertical Forms
		return true
	case r >= 0xFE30 && r <= 0xFE6F: // CJK Compatibility Forms + Small Form Variants
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

// errReported marks a failure that run has already written to stderr in full,
// so main does not print a redundant one-line summary after the report.
var errReported = errors.New("findings already reported")

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
			if isNonEnglish(r) {
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
	//
	// -z separates paths with NUL and emits them verbatim; without it git quotes
	// and escapes any path that is not plain ASCII — exactly the kind of path
	// internal/diff/git_test.go has fixtures for.
	out, err := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		// Output() fills ExitError.Stderr; without it the error reads as a bare
		// "exit status 128" and the CI log never shows what git complained about.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return fmt.Errorf("git ls-files: %w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return fmt.Errorf("git ls-files: %w", err)
	}

	var findings []finding
	var scanned int
	// NUL-terminated, so the final element is empty; the file == "" guard below
	// drops it. Do not trim the output: a path may legitimately end in a space.
	for _, file := range strings.Split(string(out), "\x00") {
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
		fmt.Fprintf(os.Stderr, "ERROR: unapproved non-English text found in %d line(s):\n", len(findings))
		for _, f := range findings {
			fmt.Fprintf(os.Stderr, "  %s:%d: %q in %s\n", f.file, f.line, f.char, trim(f.text))
		}
		fmt.Fprintf(os.Stderr, `
Source files are English-only: comments, identifiers and strings alike.
Translated prose belongs in README.<locale>.md, pages/src/content/docs/<locale>/
or an i18n table.

If the non-English text is intentional — an encoding fixture, a
language-switcher label — append a marker comment, including the reason, to
that line:

    {"multibyte truncation", 6, "..."}, // %s fixture exercises rune boundaries

For a whole tree that is inherently non-English, add a prefix to
allowedPrefixes in scripts/verify-english-only.go instead.
`, exemptMarker)
		return errReported
	}

	fmt.Printf("No unapproved non-English text in %d scanned source files.\n", scanned)
	return nil
}

func main() {
	if err := run(); err != nil {
		if !errors.Is(err, errReported) {
			fmt.Fprintln(os.Stderr, "verify-english-only:", err)
		}
		os.Exit(1)
	}
}

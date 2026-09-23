// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"bytes"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
)

func TestLooksLikeWindowsPath(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    bool
	}{
		// A backslash followed by an ordinary character is a separator in the
		// user's head and a glob escape in doublestar's, which is the whole
		// problem: it matches nothing and says nothing.
		{"separator before a plain name", `src\gen`, true},
		{"separator before a star", `src\gen\*`, true},
		{"separator before a file name", `src\gen\file.go`, true},
		{"drive letter is not a separator", `c:\gen`, true},
		{"backslash before a dot", `src\.gitignore`, true},

		// Escapes doublestar genuinely understands must stay escapes.
		{"escaped star", `src\*gen`, false},
		{"escaped question", `src\?gen`, false},
		{"escaped bracket", `src\[gen`, false},
		{"escaped brace", `src\{gen`, false},
		{"escaped bang", `\!gen`, false},
		{"escaped comma", `src\{a\,b}`, false},
		{"escaped backslash", `src\\gen`, false},
		{"escaped backslash alone", `\\`, false},

		// No backslash at all.
		{"plain glob", `*.go`, false},
		{"forward slashes", `src/gen/*`, false},
		{"empty", ``, false},
		// A trailing backslash escapes nothing and separates nothing.
		{"trailing backslash is inert", `src\`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeWindowsPath(tt.pattern); got != tt.want {
				t.Errorf("looksLikeWindowsPath(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

// TestLooksLikeWindowsPath_MarksPatternsThatCannotMatch pins the reason the
// heuristic exists: every pattern it flags really does fail to match the
// slash-separated path the user meant. Without this, the predicate could be
// "correct" against its own table and still warn about patterns that work.
func TestLooksLikeWindowsPath_MarksPatternsThatCannotMatch(t *testing.T) {
	// Each pair is (pattern the user typed, path they meant to exclude).
	broken := [][2]string{
		{`src\gen`, "src/gen"},
		{`src\gen\*`, "src/gen/file.go"},
		{`src\gen\file.go`, "src/gen/file.go"},
	}
	for _, pair := range broken {
		pattern, path := pair[0], pair[1]
		if !looksLikeWindowsPath(pattern) {
			t.Errorf("looksLikeWindowsPath(%q) = false, want true", pattern)
		}
		if matched, err := doublestar.Match(pattern, path); err != nil || matched {
			t.Errorf("doublestar.Match(%q, %q) = %v, %v; want a miss, which is what the warning reports",
				pattern, path, matched, err)
		}
	}

	// The escape forms must keep matching, so they must not be flagged.
	working := [][2]string{
		{`gen\*`, "gen*"},
		{`src/gen/*`, "src/gen/file.go"},
	}
	for _, pair := range working {
		pattern, path := pair[0], pair[1]
		if looksLikeWindowsPath(pattern) {
			t.Errorf("looksLikeWindowsPath(%q) = true, want false", pattern)
		}
		if matched, err := doublestar.Match(pattern, path); err != nil || !matched {
			t.Errorf("doublestar.Match(%q, %q) = %v, %v; want a match, which the warning must not disturb",
				pattern, path, matched, err)
		}
	}
}

func TestWarnAboutWindowsPatterns(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("warning is a Windows-only affordance")
	}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	WarnAboutWindowsPatterns(&FileFilter{Exclude: []string{`src\gen\*`, `*.go`}})

	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	if !strings.Contains(got, `src\gen\*`) {
		t.Errorf("warning should name the offending pattern, got:\n%s", got)
	}
	if !strings.Contains(got, "src/gen/*") {
		t.Errorf("warning should suggest the slash form, got:\n%s", got)
	}
	if strings.Contains(got, `"*.go"`) {
		t.Errorf("warning should not mention a well-formed pattern, got:\n%s", got)
	}
}

func TestWarnAboutWindowsPatterns_IncludesBothGroups(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("warning is a Windows-only affordance")
	}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	WarnAboutWindowsPatterns(&FileFilter{
		Include: []string{`src\inc`},
		Exclude: []string{`src\exc`},
	})

	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	if !strings.Contains(got, "include") || !strings.Contains(got, "exclude") {
		t.Errorf("warning should cover both groups, got:\n%s", got)
	}
	if !strings.Contains(got, `src\inc`) || !strings.Contains(got, `src\exc`) {
		t.Errorf("warning should name both patterns, got:\n%s", got)
	}
}

func TestWarnAboutWindowsPatterns_NilFilter(t *testing.T) {
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	WarnAboutWindowsPatterns(nil)

	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("nil filter should produce no warning, got: %q", buf.String())
	}
}

// Off Windows the guard must hold even when a pattern would otherwise be
// flagged: a backslash there is unambiguously an escape, so the suggestion
// would be noise. This runs everywhere and is the counterpart to the skipped
// assertions above.
func TestWarnAboutWindowsPatterns_QuietOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this platform does emit the warning")
	}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	WarnAboutWindowsPatterns(&FileFilter{Exclude: []string{`src\gen\*`}})

	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("no warning expected off Windows, got: %q", buf.String())
	}
}

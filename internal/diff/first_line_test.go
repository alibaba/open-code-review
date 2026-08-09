// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import "testing"

// TestFirstLine covers firstLine: it returns the first non-empty trimmed line,
// skips leading blank lines, and returns "" when there is no non-empty line.
func TestFirstLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"single line with trailing newline", "abc123\n", "abc123"},
		{"trims surrounding whitespace", "  deadbeef  \n", "deadbeef"},
		{"skips leading blank lines", "\n\n  sha\n", "sha"},
		{"empty input", "", ""},
		{"only whitespace and newlines", "\n   \n\t\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstLine(tc.in); got != tc.want {
				t.Errorf("firstLine(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

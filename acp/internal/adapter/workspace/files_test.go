// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindingLocationBoundaries(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.go")
	if err := os.WriteFile(file, []byte("one\ntwo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("outside\n"), 0600); err != nil {
		t.Fatal(err)
	}
	hasSymlink := true
	if err := os.Symlink(outside, filepath.Join(root, "escape.go")); err != nil {
		hasSymlink = false
	} else if err := os.Symlink(file, filepath.Join(root, "inside.go")); err != nil {
		hasSymlink = false
	}
	for _, tc := range []struct {
		name       string
		path       string
		start, end int
		valid      bool
	}{
		{"line range", "file.go", 1, 2, true},
		{"absolute inside", file, 2, 2, true},
		{"file level", "file.go", 0, 0, true},
		{"internal symlink", "inside.go", 1, 1, true},
		{"symlink escape", "escape.go", 1, 1, false},
		{"absolute outside", outside, 1, 1, false},
		{"traversal", "../file.go", 1, 1, false},
		{"embedded traversal", "x/../file.go", 1, 1, false},
		{"missing", "missing.go", 1, 1, false},
		{"directory", ".", 0, 0, false},
		{"uri", "file:///file.go", 1, 1, false},
		{"windows path", "C:\\file.go", 1, 1, false},
		{"backslash relative", ".\\file.go", 1, 1, false},
		{"empty", "", 0, 0, false},
		{"newline", "file.go\n", 1, 1, false},
		{"beyond eof", "file.go", 3, 3, false},
		{"end beyond eof", "file.go", 1, 3, false},
		{"reversed", "file.go", 2, 1, false},
		{"negative", "file.go", -1, -1, false},
		{"missing end", "file.go", 1, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !hasSymlink && (tc.name == "internal symlink" || tc.name == "symlink escape") {
				t.Skip("symlinks unavailable")
			}
			got := ResolveLocation(root, tc.path, tc.start, tc.end)
			if (got != nil) != tc.valid {
				t.Fatalf("location = %#v; valid = %v", got, tc.valid)
			}
			if got != nil {
				if !filepath.IsAbs(got.Path) {
					t.Fatal("location not absolute")
				}
				if tc.start == 0 && got.Line != nil {
					t.Fatal("invented file-level line")
				}
				if tc.start > 0 && (got.Line == nil || *got.Line != tc.start) {
					t.Fatal("incorrect line")
				}
			}
		})
	}
}

func TestHasLineBounds(t *testing.T) {
	for _, tc := range []struct {
		text string
		line int
		want bool
	}{
		{"", 1, false},
		{"one", 1, true},
		{"one\n", 2, false},
		{"one\ntwo", 2, true},
		{"\n\n", 2, true},
		{strings.Repeat("x", 32768) + "\nend", 2, true},
		{strings.Repeat("x", 32768), 2, false},
	} {
		if got := hasLine(strings.NewReader(tc.text), tc.line); got != tc.want {
			t.Fatalf("line %d, input length %d: got %v, want %v", tc.line, len(tc.text), got, tc.want)
		}
	}
}

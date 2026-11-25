// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitNoIndexDiff returns the unified diff git itself produces between two
// file versions, so the tests exercise real hunk headers rather than
// hand-written ones.
func gitNoIndexDiff(t *testing.T, oldContent, newContent string, extraArgs ...string) string {
	t.Helper()
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old")
	newPath := filepath.Join(dir, "new")
	if err := os.WriteFile(oldPath, []byte(oldContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte(newContent), 0o600); err != nil {
		t.Fatal(err)
	}
	args := append([]string{"diff", "--no-index", "--no-color"}, extraArgs...)
	out, err := exec.Command("git", append(args, oldPath, newPath)...).Output()
	var exitErr *exec.ExitError
	if err != nil && !(errors.As(err, &exitErr) && exitErr.ExitCode() == 1) {
		t.Fatalf("git diff --no-index: %v", err)
	}
	return string(out)
}

func TestOldContent_ReversesRealDiffs(t *testing.T) {
	tests := []struct {
		name     string
		old, new string
	}{
		{"line added at top", "package a\n\nfunc F() {}\n", "// header\npackage a\n\nfunc F() {}\n"},
		{"line removed at top", "// header\npackage a\n", "package a\n"},
		{"body edit far from top", "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\n", "a\nb\nc\nd\ne\nf\ng\nh\nI\nj\nk\n"},
		{"several hunks", "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n16\n", "0\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n16\n"},
		{"lines moved below a comment", "package auth\n\n// @generated\nfunc A() bool { return check() }\n", "// @generated\npackage auth\n\nfunc A() bool { return true }\n"},
		{"pure deletion in the middle", "a\nb\nc\nd\n", "a\nd\n"},
		{"missing final newline", "a\nb", "a\nb\nc\n"},
		{"crlf", "a\r\nb\r\n", "x\r\na\r\nb\r\n"},
		{"identical", "same\n", "same\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := gitNoIndexDiff(t, tt.old, tt.new)
			got, ok := OldContent(tt.new, raw)
			if !ok {
				t.Fatalf("OldContent rejected git's own diff:\n%s", raw)
			}
			// "\ No newline at end of file" is not carried into hunk lines,
			// so a file that lacked a final newline comes back with one.
			if got != tt.old && got != tt.old+"\n" {
				t.Errorf("OldContent = %q, want %q", got, tt.old)
			}
		})
	}
}

// Without context lines a deletion-only hunk has a zero-length new side, and
// git numbers it by the line the deletion follows: deleting the second of
// three lines is "@@ -2 +1,0 @@".
func TestOldContent_ZeroContextDeletions(t *testing.T) {
	tests := []struct {
		name     string
		old, new string
	}{
		{"middle line", "a\nb\nc\n", "a\nc\n"},
		{"last line", "a\nb\nc\nd\n", "a\nb\nc\n"},
		{"first line", "a\nb\n", "b\n"},
		{"whole file", "a\nb\n", ""},
		{"two separate deletions", "a\nb\nc\nd\ne\n", "a\nc\ne\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := gitNoIndexDiff(t, tt.old, tt.new, "-U0")
			got, ok := OldContent(tt.new, raw)
			if !ok {
				t.Fatalf("OldContent rejected git's own diff:\n%s", raw)
			}
			if got != tt.old && got != tt.old+"\n" {
				t.Errorf("OldContent = %q, want %q\n%s", got, tt.old, raw)
			}
		})
	}
}

func TestOldContent_RejectsMismatchedDiff(t *testing.T) {
	raw := gitNoIndexDiff(t, "a\nb\n", "a\nB\n")
	if _, ok := OldContent("something else\nentirely\n", raw); ok {
		t.Error("a diff that does not match the new content must not be reversed")
	}
	overlapping := "@@ -3,1 +3,1 @@\n-x\n+y\n@@ -1,1 +1,1 @@\n-p\n+q\n"
	if _, ok := OldContent("q\nb\ny\n", overlapping); ok {
		t.Error("hunks out of order must be rejected")
	}
	truncated := "@@ -1,2 +1,2 @@\n-a\n+A\n"
	if _, ok := OldContent("A\nb\n", truncated); ok {
		t.Error("a hunk with fewer lines than its header counts must be rejected")
	}
	pastEnd := "@@ -9,1 +9,1 @@\n-x\n+y\n"
	if _, ok := OldContent("a\n", pastEnd); ok {
		t.Error("a hunk past the end of the file must be rejected")
	}
}

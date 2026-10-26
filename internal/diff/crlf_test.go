// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"strings"
	"testing"
)

// CRLF-terminated diff text reaches the parser from a .patch checked out under
// core.autocrlf=true, or from output captured through a Windows shell. Before
// the fix the trailing "\r" rode into the "diff --git" capture, so NewPath was
// "fresh.go\r" and the file could not be opened for review.
func TestParseDiffText_CRLFKeepsPathsClean(t *testing.T) {
	diffText := "diff --git a/fresh.go b/fresh.go\r\n" +
		"new file mode 100644\r\n" +
		"--- /dev/null\r\n" +
		"+++ b/fresh.go\r\n" +
		"@@ -0,0 +1,1 @@\r\n" +
		"+line1\r\n"

	diffs, err := ParseDiffText(context.Background(), diffText, t.TempDir(), "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	d := diffs[0]
	if d.NewPath != "fresh.go" {
		t.Errorf("NewPath = %q, want %q", d.NewPath, "fresh.go")
	}
	if d.OldPath != "fresh.go" {
		t.Errorf("OldPath = %q, want %q", d.OldPath, "fresh.go")
	}
	if !d.IsNew {
		t.Error("IsNew = false, want true")
	}
	if strings.Contains(d.Diff, "\r") {
		t.Errorf("prompt diff still carries CR: %q", d.Diff)
	}
}

// The "new file mode" / "deleted file mode" headers are matched by prefix, so
// they survive a trailing CR on their own. The /dev/null markers are compared
// for equality, so they do not: a diff whose only new/deleted signal is the
// marker silently lost the flag.
func TestParseDiffText_CRLFKeepsDevNullMetadata(t *testing.T) {
	tests := []struct {
		name          string
		diffText      string
		wantNew       bool
		wantDeleted   bool
		wantNewPath   string
		wantInsertion int64
	}{
		{
			name: "added file, marker is the only signal",
			diffText: "diff --git a/a.go b/a.go\r\n" +
				"--- /dev/null\r\n" +
				"+++ b/a.go\r\n" +
				"@@ -0,0 +1,1 @@\r\n" +
				"+x\r\n",
			wantNew:       true,
			wantNewPath:   "a.go",
			wantInsertion: 1,
		},
		{
			name: "deleted file, marker is the only signal",
			diffText: "diff --git a/b.go b/b.go\r\n" +
				"--- a/b.go\r\n" +
				"+++ /dev/null\r\n" +
				"@@ -1,1 +0,0 @@\r\n" +
				"-x\r\n",
			wantDeleted: true,
			// finalizeDiff rewrites NewPath for a deletion.
			wantNewPath: "/dev/null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diffs, err := ParseDiffText(context.Background(), tt.diffText, t.TempDir(), "", nil)
			if err != nil {
				t.Fatalf("ParseDiffText: %v", err)
			}
			if len(diffs) != 1 {
				t.Fatalf("expected 1 diff, got %d", len(diffs))
			}
			d := diffs[0]
			if d.IsNew != tt.wantNew {
				t.Errorf("IsNew = %v, want %v", d.IsNew, tt.wantNew)
			}
			if d.IsDeleted != tt.wantDeleted {
				t.Errorf("IsDeleted = %v, want %v", d.IsDeleted, tt.wantDeleted)
			}
			if d.NewPath != tt.wantNewPath {
				t.Errorf("NewPath = %q, want %q", d.NewPath, tt.wantNewPath)
			}
			if tt.wantInsertion > 0 && d.Insertions != tt.wantInsertion {
				t.Errorf("Insertions = %d, want %d", d.Insertions, tt.wantInsertion)
			}
		})
	}
}

// ParseHunks splits the same text, so hunk content carried the CR too. Line
// content is compared against file content when resolving comment positions.
func TestParseHunks_CRLFKeepsLineContentClean(t *testing.T) {
	rawDiff := "diff --git a/x.go b/x.go\r\n" +
		"--- a/x.go\r\n" +
		"+++ b/x.go\r\n" +
		"@@ -1,2 +1,3 @@\r\n" +
		" kept\r\n" +
		"+added\r\n" +
		"-removed\r\n"

	hunks := ParseHunks(rawDiff)
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	var got []string
	for _, line := range hunks[0].Lines {
		if strings.Contains(line.Content, "\r") {
			t.Errorf("hunk line still carries CR: %q", line.Content)
		}
		got = append(got, line.Content)
	}
	// The trailing newline of the last line yields one empty context line.
	// That is existing behaviour, identical for LF input, and unrelated to CR.
	want := []string{"kept", "added", "removed", ""}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %q, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// LF diff text must be untouched, including a lone CR that is genuinely part
// of a line rather than a line terminator.
func TestSplitDiffLines_LeavesLFTextAlone(t *testing.T) {
	got := splitDiffLines("alpha\nbeta\n")
	want := []string{"alpha", "beta", ""}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}

	// Only the final CR of a line is a terminator; an interior one stays.
	if lines := splitDiffLines("a\rb\n"); lines[0] != "a\rb" {
		t.Errorf("interior CR was stripped: %q", lines[0])
	}
}

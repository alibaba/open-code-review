// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDiffHeader(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		wantOld string
		wantNew string
		wantOK  bool
	}{
		{
			name:    "ordinary path",
			header:  "diff --git a/main.go b/main.go",
			wantOld: "main.go",
			wantNew: "main.go",
			wantOK:  true,
		},
		{
			name:    "delimiter substring in path",
			header:  "diff --git a/dir b/file.go b/dir b/file.go",
			wantOld: "dir b/file.go",
			wantNew: "dir b/file.go",
			wantOK:  true,
		},
		{
			name:    "both paths quoted",
			header:  `diff --git "a/tab\told.go" "b/tab\tnew.go"`,
			wantOld: "tab\told.go",
			wantNew: "tab\tnew.go",
			wantOK:  true,
		},
		{
			name:    "new path quoted",
			header:  `diff --git a/old.go "b/tab\tnew.go"`,
			wantOld: "old.go",
			wantNew: "tab\tnew.go",
			wantOK:  true,
		},
		{
			name:    "old path quoted",
			header:  `diff --git "a/tab\told.go" b/new.go`,
			wantOld: "tab\told.go",
			wantNew: "new.go",
			wantOK:  true,
		},
		{
			name:   "not a git diff header",
			header: "@@ -1 +1 @@",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldPath, newPath, ok := parseDiffHeader(tt.header)
			if ok != tt.wantOK || oldPath != tt.wantOld || newPath != tt.wantNew {
				t.Errorf("parseDiffHeader(%q) = %q, %q, %v; want %q, %q, %v",
					tt.header, oldPath, newPath, ok, tt.wantOld, tt.wantNew, tt.wantOK)
			}
		})
	}
}

func TestParseDiffText_PathContainingHeaderDelimiter(t *testing.T) {
	repo := t.TempDir()
	relPath := "dir b/file.go"
	fullPath := filepath.Join(repo, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create test directory: %v", err)
	}
	const wantContent = "package demo\nconst Value = 2\n"
	if err := os.WriteFile(fullPath, []byte(wantContent), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	diffText := `diff --git a/dir b/file.go b/dir b/file.go
index 74f3ac7..c32d45f 100644
--- a/dir b/file.go
+++ b/dir b/file.go
@@ -1,2 +1,2 @@
 package demo
-const Value = 1
+const Value = 2
`

	diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("got %d diffs, want 1", len(diffs))
	}
	d := diffs[0]
	if d.OldPath != relPath || d.NewPath != relPath {
		t.Errorf("OldPath/NewPath = %q/%q, want %q/%q",
			d.OldPath, d.NewPath, relPath, relPath)
	}
	if d.NewFileContent != wantContent {
		t.Errorf("NewFileContent = %q, want %q", d.NewFileContent, wantContent)
	}
}

func TestParseDiffText_CQuotedPath(t *testing.T) {
	repo := t.TempDir()
	relPath := "tab\tfile.go"
	const wantContent = "package demo\nconst Value = 2\n"
	if err := os.WriteFile(filepath.Join(repo, relPath), []byte(wantContent), 0o644); err != nil {
		t.Skipf("filesystem does not support tab in file names: %v", err)
	}

	diffText := `diff --git "a/tab\tfile.go" "b/tab\tfile.go"
index 74f3ac7..c32d45f 100644
--- "a/tab\tfile.go"
+++ "b/tab\tfile.go"
@@ -1,2 +1,2 @@
 package demo
-const Value = 1
+const Value = 2
`

	diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("got %d diffs, want 1", len(diffs))
	}
	d := diffs[0]
	if d.OldPath != relPath || d.NewPath != relPath {
		t.Errorf("OldPath/NewPath = %q/%q, want %q/%q",
			d.OldPath, d.NewPath, relPath, relPath)
	}
	if d.NewFileContent != wantContent {
		t.Errorf("NewFileContent = %q, want %q", d.NewFileContent, wantContent)
	}
}

func TestParseDiffText_CQuotedRename(t *testing.T) {
	diffText := `diff --git "a/tab\told.go" "b/tab\tnew.go"
similarity index 100%
rename from "tab\told.go"
rename to "tab\tnew.go"
`

	diffs, err := ParseDiffText(context.Background(), diffText, t.TempDir(), "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("got %d diffs, want 1", len(diffs))
	}
	d := diffs[0]
	if !d.IsRenamed || d.OldPath != "tab\told.go" || d.NewPath != "tab\tnew.go" {
		t.Errorf("got IsRenamed=%v OldPath=%q NewPath=%q, want true/%q/%q",
			d.IsRenamed, d.OldPath, d.NewPath, "tab\told.go", "tab\tnew.go")
	}
}

func TestParseDiffText_StripsIndexHeadersFromPromptDiff(t *testing.T) {
	diffText := `diff --git a/first.go b/first.go
index 1234567..89abcde 100644
--- a/first.go
+++ b/first.go
@@ -1,1 +1,2 @@
 first
+index added-content
diff --git a/second.go b/second.go
new file mode 100644
index 0000000..7654321
--- /dev/null
+++ b/second.go
@@ -0,0 +1 @@
+package second
`

	diffs, err := ParseDiffText(context.Background(), diffText, t.TempDir(), "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 2 {
		t.Fatalf("expected 2 diffs, got %d", len(diffs))
	}

	for _, d := range diffs {
		if strings.Contains(d.Diff, "\nindex ") {
			t.Errorf("prompt diff contains index header:\n%s", d.Diff)
		}
	}
	if !strings.Contains(diffs[0].Diff, "diff --git a/first.go b/first.go") {
		t.Errorf("prompt diff lost git header:\n%s", diffs[0].Diff)
	}
	if !strings.Contains(diffs[0].Diff, "+index added-content") {
		t.Errorf("prompt diff lost index-prefixed hunk content:\n%s", diffs[0].Diff)
	}
	if !diffs[1].IsNew {
		t.Error("new-file metadata was not preserved")
	}
}

// TestParseDiffText_Rename guards against issue #99: a renamed file must be
// recognized via the "rename from"/"rename to" extended header lines so that
// the parser reads content at the NEW path instead of warning about the old
// path ("cannot read file ... exit status 128").
func TestParseDiffText_Rename(t *testing.T) {
	diffText := `diff --git a/pkg/old name.go b/pkg/new name.go
similarity index 95%
rename from pkg/old name.go
rename to pkg/new name.go
index 1234567..89abcde 100644
--- a/pkg/old name.go
+++ b/pkg/new name.go
@@ -1,3 +1,3 @@
 line1
-line2
+line2 changed
 line3
`
	diffs, err := ParseDiffText(context.Background(), diffText, t.TempDir(), "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	d := diffs[0]
	if !d.IsRenamed {
		t.Errorf("IsRenamed = false, want true")
	}
	if d.OldPath != "pkg/old name.go" {
		t.Errorf("OldPath = %q, want %q", d.OldPath, "pkg/old name.go")
	}
	if d.NewPath != "pkg/new name.go" {
		t.Errorf("NewPath = %q, want %q", d.NewPath, "pkg/new name.go")
	}
	if d.IsNew || d.IsDeleted {
		t.Errorf("IsNew/IsDeleted = %v/%v, want false/false", d.IsNew, d.IsDeleted)
	}
}

// TestParseDiffText_PureRename covers a 100% similarity rename, which carries
// no hunks and no ---/+++ lines at all.
func TestParseDiffText_PureRename(t *testing.T) {
	diffText := `diff --git a/old.go b/new.go
similarity index 100%
rename from old.go
rename to new.go
`
	diffs, err := ParseDiffText(context.Background(), diffText, t.TempDir(), "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	d := diffs[0]
	if !d.IsRenamed || d.OldPath != "old.go" || d.NewPath != "new.go" {
		t.Errorf("got IsRenamed=%v OldPath=%q NewPath=%q, want true/old.go/new.go",
			d.IsRenamed, d.OldPath, d.NewPath)
	}
}

// TestParseDiffText_DeletedFile guards the /dev/null detection: git emits
// "+++ /dev/null" WITHOUT the b/ prefix, which the old regexes required, so
// deletions were misclassified and triggered a doomed `git show ref:path`.
func TestParseDiffText_DeletedFile(t *testing.T) {
	diffText := `diff --git a/gone.go b/gone.go
deleted file mode 100644
index 1234567..0000000
--- a/gone.go
+++ /dev/null
@@ -1,2 +0,0 @@
-line1
-line2
`
	diffs, err := ParseDiffText(context.Background(), diffText, t.TempDir(), "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	d := diffs[0]
	if !d.IsDeleted {
		t.Errorf("IsDeleted = false, want true")
	}
	if d.NewPath != "/dev/null" {
		t.Errorf("NewPath = %q, want /dev/null", d.NewPath)
	}
	if d.OldPath != "gone.go" {
		t.Errorf("OldPath = %q, want gone.go", d.OldPath)
	}
}

// TestParseDiffText_NewFile covers "--- /dev/null" (no a/ prefix).
func TestParseDiffText_NewFile(t *testing.T) {
	diffText := `diff --git a/fresh.go b/fresh.go
new file mode 100644
index 0000000..1234567
--- /dev/null
+++ b/fresh.go
@@ -0,0 +1,2 @@
+line1
+line2
`
	repo := t.TempDir()
	diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	d := diffs[0]
	if !d.IsNew {
		t.Errorf("IsNew = false, want true")
	}
	if d.IsDeleted {
		t.Errorf("IsDeleted = true, want false")
	}
	if d.Insertions != 2 {
		t.Errorf("Insertions = %d, want 2", d.Insertions)
	}
}

// TestParseDiffText_BinaryMarkerAnchored guards two binary-detection cases:
// a text file whose CONTENT mentions "Binary files " must not be classified
// as binary (the unanchored regex used to match any line in the section and
// the file was silently excluded from review), while a real binary diff must
// still be detected.
func TestParseDiffText_BinaryMarkerAnchored(t *testing.T) {
	diffText := `diff --git a/docs.md b/docs.md
index 1234567..89abcde 100644
--- a/docs.md
+++ b/docs.md
@@ -1,2 +1,3 @@
 line1
+Note: Binary files are handled specially by git.
 line2
diff --git a/blob.bin b/blob.bin
index 1234567..89abcde 100644
Binary files a/blob.bin and b/blob.bin differ
`
	diffs, err := ParseDiffText(context.Background(), diffText, t.TempDir(), "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 2 {
		t.Fatalf("expected 2 diffs, got %d", len(diffs))
	}
	if diffs[0].IsBinary {
		t.Errorf("docs.md IsBinary = true, want false (content line mentioning "+
			"'Binary files ' must not mark the file binary); diff:\n%s", diffText)
	}
	if diffs[0].Insertions != 1 {
		t.Errorf("docs.md Insertions = %d, want 1", diffs[0].Insertions)
	}
	if !diffs[1].IsBinary {
		t.Errorf("blob.bin IsBinary = false, want true")
	}
}

// TestParseDiffText_CountsContentLinesWithPlusMinusPrefix covers content
// lines that themselves begin with "++"/"--": an added line "++i" renders in
// the diff as "+++i", and the old "exclude +++/--- header" guard used to drop
// it from the insertion count (same for deletions), skewing per-file stats
// and the changeLines threshold that gates the plan phase.
func TestParseDiffText_CountsContentLinesWithPlusMinusPrefix(t *testing.T) {
	diffText := `diff --git a/counter.go b/counter.go
index 1234567..89abcde 100644
--- a/counter.go
+++ b/counter.go
@@ -1,3 +1,3 @@
 func inc() {
---oldFlag
+++newFlag
 }
`
	diffs, err := ParseDiffText(context.Background(), diffText, t.TempDir(), "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	d := diffs[0]
	if d.Insertions != 1 {
		t.Errorf("Insertions = %d, want 1 (added content line \"+++newFlag\")", d.Insertions)
	}
	if d.Deletions != 1 {
		t.Errorf("Deletions = %d, want 1 (deleted content line \"---oldFlag\")", d.Deletions)
	}
}

// TestParseDiffText_DevNullStringInsideHunk ensures an added content line
// whose rendered form is exactly "+++ /dev/null" (i.e. the file gained a line
// reading "++ /dev/null") is treated as hunk content, not as the deleted-file
// header marker.
func TestParseDiffText_DevNullStringInsideHunk(t *testing.T) {
	diffText := `diff --git a/paths.txt b/paths.txt
index 1234567..89abcde 100644
--- a/paths.txt
+++ b/paths.txt
@@ -1,1 +1,2 @@
 first
+++ /dev/null
`
	diffs, err := ParseDiffText(context.Background(), diffText, t.TempDir(), "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	d := diffs[0]
	if d.IsDeleted {
		t.Errorf("IsDeleted = true, want false (\"+++ /dev/null\" inside a hunk is content)")
	}
	if d.Insertions != 1 {
		t.Errorf("Insertions = %d, want 1", d.Insertions)
	}
}

package diff

import (
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
)

// canonAFile is a two-hunk canonical diff for "a.txt" used across cases.
// Hunk 1 adds "added-early" near the top; hunk 2 adds "added-late" near line 11.
const canonAPreamble = "diff --git a/a.txt b/a.txt\n" +
	"index 1111111..2222222 100644\n" +
	"--- a/a.txt\n" +
	"+++ b/a.txt"

const canonAHunk1 = "@@ -1,3 +1,4 @@\n" +
	" line1\n" +
	"+added-early\n" +
	" line2\n" +
	" line3"

const canonAHunk2 = "@@ -10,3 +11,4 @@\n" +
	" line10\n" +
	" line11\n" +
	"+added-late\n" +
	" line12"

func canonAFile() model.Diff {
	return model.Diff{
		OldPath:        "a.txt",
		NewPath:        "a.txt",
		Diff:           canonAPreamble + "\n" + canonAHunk1 + "\n" + canonAHunk2,
		NewFileContent: "HEAD-CONTENT-A",
		Insertions:     2,
	}
}

// selPatch wraps hunks with a minimal file header for "a.txt".
func selAPatch(hunks ...string) string {
	head := "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n"
	return head + strings.Join(hunks, "\n")
}

func TestApplySelection_Success(t *testing.T) {
	tests := []struct {
		name           string
		canonical      []model.Diff
		patch          string
		wantFiles      int
		wantContains   []string // substrings that must appear in result[0].Diff
		wantExcludes   []string // substrings that must NOT appear in result[0].Diff
		wantInsertions int64
		wantDeletions  int64
		wantHEAD       string
	}{
		{
			name:           "single hunk",
			canonical:      []model.Diff{canonAFile()},
			patch:          selAPatch(canonAHunk1),
			wantFiles:      1,
			wantContains:   []string{"@@ -1,3 +1,4 @@", "added-early"},
			wantExcludes:   []string{"added-late", "@@ -10,3 +11,4 @@"},
			wantInsertions: 1,
			wantHEAD:       "HEAD-CONTENT-A",
		},
		{
			name:           "multiple selected hunks in one file",
			canonical:      []model.Diff{canonAFile()},
			patch:          selAPatch(canonAHunk1, canonAHunk2),
			wantFiles:      1,
			wantContains:   []string{"added-early", "added-late", "@@ -1,3 +1,4 @@", "@@ -10,3 +11,4 @@"},
			wantInsertions: 2,
		},
		{
			name:           "non-selected hunk excluded (only second selected)",
			canonical:      []model.Diff{canonAFile()},
			patch:          selAPatch(canonAHunk2),
			wantFiles:      1,
			wantContains:   []string{"added-late", "@@ -10,3 +11,4 @@"},
			wantExcludes:   []string{"added-early", "@@ -1,3 +1,4 @@"},
			wantInsertions: 1,
		},
		{
			name:         "preamble preserved from canonical even if selection omits index",
			canonical:    []model.Diff{canonAFile()},
			patch:        selAPatch(canonAHunk1),
			wantFiles:    1,
			wantContains: []string{"index 1111111..2222222 100644"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ApplySelection(tt.canonical, tt.patch)
			if err != nil {
				t.Fatalf("ApplySelection() unexpected error: %v", err)
			}
			if len(got) != tt.wantFiles {
				t.Fatalf("got %d files, want %d", len(got), tt.wantFiles)
			}
			d := got[0]
			for _, s := range tt.wantContains {
				if !strings.Contains(d.Diff, s) {
					t.Errorf("result diff missing %q; diff=\n%s", s, d.Diff)
				}
			}
			for _, s := range tt.wantExcludes {
				if strings.Contains(d.Diff, s) {
					t.Errorf("result diff should not contain %q; diff=\n%s", s, d.Diff)
				}
			}
			if tt.wantInsertions != 0 && d.Insertions != tt.wantInsertions {
				t.Errorf("Insertions=%d, want %d", d.Insertions, tt.wantInsertions)
			}
			if tt.wantHEAD != "" && d.NewFileContent != tt.wantHEAD {
				t.Errorf("NewFileContent=%q, want %q (must stay anchored at true HEAD)", d.NewFileContent, tt.wantHEAD)
			}
		})
	}
}

func TestApplySelection_MultipleFiles(t *testing.T) {
	fileB := model.Diff{
		OldPath:        "b.txt",
		NewPath:        "b.txt",
		Diff:           "diff --git a/b.txt b/b.txt\nindex aaa..bbb 100644\n--- a/b.txt\n+++ b/b.txt\n@@ -1,2 +1,3 @@\n x\n+bee\n y",
		NewFileContent: "HEAD-CONTENT-B",
	}
	canonical := []model.Diff{canonAFile(), fileB}

	// Select hunk1 of a.txt and the only hunk of b.txt.
	patch := selAPatch(canonAHunk1) + "\n" +
		"diff --git a/b.txt b/b.txt\n--- a/b.txt\n+++ b/b.txt\n@@ -1,2 +1,3 @@\n x\n+bee\n y"

	got, err := ApplySelection(canonical, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d files, want 2", len(got))
	}
	// canonical order preserved: a.txt first, b.txt second.
	if got[0].NewPath != "a.txt" || got[1].NewPath != "b.txt" {
		t.Fatalf("unexpected order: %s, %s", got[0].NewPath, got[1].NewPath)
	}
	if strings.Contains(got[0].Diff, "added-late") {
		t.Errorf("a.txt should only contain hunk1")
	}
	if !strings.Contains(got[1].Diff, "bee") {
		t.Errorf("b.txt hunk missing")
	}
}

func TestApplySelection_SelectingOneFileDropsOthers(t *testing.T) {
	fileB := model.Diff{
		OldPath: "b.txt", NewPath: "b.txt",
		Diff: "diff --git a/b.txt b/b.txt\n--- a/b.txt\n+++ b/b.txt\n@@ -1,1 +1,2 @@\n x\n+bee",
	}
	canonical := []model.Diff{canonAFile(), fileB}
	got, err := ApplySelection(canonical, selAPatch(canonAHunk1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].NewPath != "a.txt" {
		t.Fatalf("expected only a.txt, got %+v", got)
	}
}

func TestApplySelection_Errors(t *testing.T) {
	tests := []struct {
		name      string
		canonical []model.Diff
		patch     string
		wantErr   string
	}{
		{
			name:      "empty selection",
			canonical: []model.Diff{canonAFile()},
			patch:     "   \n  ",
			wantErr:   "empty",
		},
		{
			name:      "no diff --git header",
			canonical: []model.Diff{canonAFile()},
			patch:     "this is not a patch\njust text",
			wantErr:   "before first 'diff --git'",
		},
		{
			name:      "invented path not in canonical",
			canonical: []model.Diff{canonAFile()},
			patch:     "diff --git a/ghost.txt b/ghost.txt\n--- a/ghost.txt\n+++ b/ghost.txt\n@@ -1,1 +1,2 @@\n x\n+y",
			wantErr:   "not part of the canonical diff",
		},
		{
			name:      "modified hunk body",
			canonical: []model.Diff{canonAFile()},
			patch:     selAPatch("@@ -1,3 +1,4 @@\n line1\n+MUTATED\n line2\n line3"),
			wantErr:   "modified or partial hunk",
		},
		{
			name:      "wrong ranges (no such hunk location)",
			canonical: []model.Diff{canonAFile()},
			patch:     selAPatch("@@ -50,3 +50,4 @@\n line1\n+added-early\n line2\n line3"),
			wantErr:   "not present in the canonical diff",
		},
		{
			name:      "truncated/partial hunk body",
			canonical: []model.Diff{canonAFile()},
			patch:     selAPatch("@@ -1,3 +1,4 @@\n line1\n+added-early"),
			wantErr:   "modified or partial hunk",
		},
		{
			name:      "duplicate hunk selection",
			canonical: []model.Diff{canonAFile()},
			patch:     selAPatch(canonAHunk1, canonAHunk1),
			wantErr:   "more than once",
		},
		{
			name:      "path traversal",
			canonical: []model.Diff{canonAFile()},
			patch:     "diff --git a/../etc/passwd b/../etc/passwd\n--- a/../etc/passwd\n+++ b/../etc/passwd\n@@ -1,1 +1,2 @@\n x\n+y",
			wantErr:   "path traversal",
		},
		{
			name:      "absolute path",
			canonical: []model.Diff{canonAFile()},
			patch:     "diff --git a//etc/passwd b//etc/passwd\n--- a//etc/passwd\n+++ b//etc/passwd\n@@ -1,1 +1,2 @@\n x\n+y",
			wantErr:   "absolute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ApplySelection(tt.canonical, tt.patch)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestApplySelection_AmbiguousHunk(t *testing.T) {
	// Craft a (pathological) canonical file with two hunks that have identical
	// headers AND identical bodies so the selection cannot be disambiguated.
	dupHunk := "@@ -5,2 +5,3 @@\n ctx\n+dup\n ctx2"
	canonical := []model.Diff{{
		OldPath: "d.txt", NewPath: "d.txt",
		Diff: "diff --git a/d.txt b/d.txt\n--- a/d.txt\n+++ b/d.txt\n" + dupHunk + "\n" + dupHunk,
	}}
	patch := "diff --git a/d.txt b/d.txt\n--- a/d.txt\n+++ b/d.txt\n" + dupHunk
	_, err := ApplySelection(canonical, patch)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous error, got %v", err)
	}
}

func TestApplySelection_DuplicateBodyDifferentLocationDisambiguated(t *testing.T) {
	// Two hunks with identical body but different headers must be disambiguated
	// by header (not treated as ambiguous).
	h1 := "@@ -1,2 +1,3 @@\n ctx\n+same\n ctx2"
	h2 := "@@ -20,2 +21,3 @@\n ctx\n+same\n ctx2"
	canonical := []model.Diff{{
		OldPath: "e.txt", NewPath: "e.txt",
		Diff: "diff --git a/e.txt b/e.txt\n--- a/e.txt\n+++ b/e.txt\n" + h1 + "\n" + h2,
	}}
	patch := "diff --git a/e.txt b/e.txt\n--- a/e.txt\n+++ b/e.txt\n" + h2
	got, err := ApplySelection(canonical, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got[0].Diff, "@@ -20,2 +21,3 @@") {
		t.Errorf("expected second hunk selected; diff=\n%s", got[0].Diff)
	}
	if strings.Contains(got[0].Diff, "@@ -1,2 +1,3 @@") {
		t.Errorf("first hunk should not be selected; diff=\n%s", got[0].Diff)
	}
}

func TestApplySelection_Rename(t *testing.T) {
	canonical := []model.Diff{{
		OldPath:        "old.txt",
		NewPath:        "new.txt",
		IsRenamed:      true,
		Diff:           "diff --git a/old.txt b/new.txt\nsimilarity index 90%\nrename from old.txt\nrename to new.txt\n--- a/old.txt\n+++ b/new.txt\n@@ -1,2 +1,3 @@\n keep\n+extra\n tail",
		NewFileContent: "HEAD-RENAMED",
	}}
	patch := "diff --git a/old.txt b/new.txt\nrename from old.txt\nrename to new.txt\n--- a/old.txt\n+++ b/new.txt\n@@ -1,2 +1,3 @@\n keep\n+extra\n tail"
	got, err := ApplySelection(canonical, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].NewPath != "new.txt" || !got[0].IsRenamed {
		t.Fatalf("rename not preserved: %+v", got)
	}
	if got[0].NewFileContent != "HEAD-RENAMED" {
		t.Errorf("HEAD content not preserved")
	}
}

func TestApplySelection_Delete(t *testing.T) {
	canonical := []model.Diff{{
		OldPath:   "gone.txt",
		NewPath:   "/dev/null",
		IsDeleted: true,
		Diff:      "diff --git a/gone.txt b/gone.txt\ndeleted file mode 100644\n--- a/gone.txt\n+++ /dev/null\n@@ -1,3 +0,0 @@\n-a\n-b\n-c",
	}}
	patch := "diff --git a/gone.txt b/gone.txt\ndeleted file mode 100644\n--- a/gone.txt\n+++ /dev/null\n@@ -1,3 +0,0 @@\n-a\n-b\n-c"
	got, err := ApplySelection(canonical, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || !got[0].IsDeleted {
		t.Fatalf("delete not preserved: %+v", got)
	}
	if got[0].Deletions != 3 {
		t.Errorf("Deletions=%d, want 3", got[0].Deletions)
	}
}

func TestApplySelection_BinaryRejected(t *testing.T) {
	canonical := []model.Diff{{
		OldPath: "img.png", NewPath: "img.png", IsBinary: true,
		Diff: "diff --git a/img.png b/img.png\nindex 111..222 100644\nBinary files a/img.png and b/img.png differ",
	}}
	patch := "diff --git a/img.png b/img.png\nBinary files a/img.png and b/img.png differ"
	_, err := ApplySelection(canonical, patch)
	if err == nil || !strings.Contains(err.Error(), "binary") {
		t.Fatalf("expected binary rejection, got %v", err)
	}
}

func TestApplySelection_BinaryMentionInHunkBodyNotRejected(t *testing.T) {
	// A hunk body line whose content merely mentions "Binary files " must not
	// mark the file as binary: only a real (line-anchored) binary marker counts.
	hunk := "@@ -1,2 +1,3 @@\n docs\n+Binary files are compared here\n tail"
	canonical := []model.Diff{{
		OldPath: "doc.md", NewPath: "doc.md",
		Diff: "diff --git a/doc.md b/doc.md\n--- a/doc.md\n+++ b/doc.md\n" + hunk,
	}}
	patch := "diff --git a/doc.md b/doc.md\n--- a/doc.md\n+++ b/doc.md\n" + hunk
	got, err := ApplySelection(canonical, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got[0].Diff, "Binary files are compared here") {
		t.Errorf("selected hunk content lost: %q", got[0].Diff)
	}
}

func TestApplySelection_SubmoduleRejected(t *testing.T) {
	canonical := []model.Diff{{
		OldPath: "sub", NewPath: "sub",
		Diff: "diff --git a/sub b/sub\nindex 111..222 160000\n--- a/sub\n+++ b/sub\n@@ -1 +1 @@\n-Subproject commit 1111111\n+Subproject commit 2222222",
	}}
	patch := "diff --git a/sub b/sub\nindex 111..222 160000\n--- a/sub\n+++ b/sub\n@@ -1 +1 @@\n-Subproject commit 1111111\n+Subproject commit 2222222"
	_, err := ApplySelection(canonical, patch)
	if err == nil || !strings.Contains(err.Error(), "submodule") {
		t.Fatalf("expected submodule rejection, got %v", err)
	}
}

func TestApplySelection_CRLFContentPreserved(t *testing.T) {
	// A file with CRLF line endings: the CR is part of each line's content in
	// the diff. The selection is sliced verbatim, so it matches exactly.
	hunk := "@@ -1,2 +1,3 @@\n keep\r\n+added\r\n tail\r"
	canonical := []model.Diff{{
		OldPath: "crlf.txt", NewPath: "crlf.txt",
		Diff:           "diff --git a/crlf.txt b/crlf.txt\n--- a/crlf.txt\n+++ b/crlf.txt\n" + hunk,
		NewFileContent: "keep\r\nadded\r\ntail\r\n",
	}}
	patch := "diff --git a/crlf.txt b/crlf.txt\n--- a/crlf.txt\n+++ b/crlf.txt\n" + hunk
	got, err := ApplySelection(canonical, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got[0].Diff, "added\r") {
		t.Errorf("CR content not preserved: %q", got[0].Diff)
	}
}

func TestApplySelection_UnicodeContent(t *testing.T) {
	hunk := "@@ -1,2 +1,3 @@\n 日本語\n+café-résumé-🚀\n مرحبا"
	canonical := []model.Diff{{
		OldPath: "u.txt", NewPath: "u.txt",
		Diff: "diff --git a/u.txt b/u.txt\n--- a/u.txt\n+++ b/u.txt\n" + hunk,
	}}
	patch := "diff --git a/u.txt b/u.txt\n--- a/u.txt\n+++ b/u.txt\n" + hunk
	got, err := ApplySelection(canonical, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got[0].Diff, "café-résumé-🚀") {
		t.Errorf("unicode content lost: %q", got[0].Diff)
	}
}

func TestApplySelection_NoNewlineMarker(t *testing.T) {
	hunk := "@@ -1,2 +1,2 @@\n keep\n-old\n+new\n\\ No newline at end of file"
	canonical := []model.Diff{{
		OldPath: "n.txt", NewPath: "n.txt",
		Diff: "diff --git a/n.txt b/n.txt\n--- a/n.txt\n+++ b/n.txt\n" + hunk,
	}}
	// Success when marker included verbatim.
	patch := "diff --git a/n.txt b/n.txt\n--- a/n.txt\n+++ b/n.txt\n" + hunk
	got, err := ApplySelection(canonical, patch)
	if err != nil {
		t.Fatalf("unexpected error with no-newline marker: %v", err)
	}
	if !strings.Contains(got[0].Diff, "No newline at end of file") {
		t.Errorf("no-newline marker dropped")
	}

	// Failure when marker omitted (body differs from canonical).
	bad := "diff --git a/n.txt b/n.txt\n--- a/n.txt\n+++ b/n.txt\n@@ -1,2 +1,2 @@\n keep\n-old\n+new"
	if _, err := ApplySelection(canonical, bad); err == nil {
		t.Fatalf("expected mismatch when no-newline marker omitted")
	}
}

// TestApplySelection_LineResolverAccuracy verifies the narrowed diff still
// resolves comment line numbers to true-HEAD positions.
func TestApplySelection_LineResolverAccuracy(t *testing.T) {
	got, err := ApplySelection([]model.Diff{canonAFile()}, selAPatch(canonAHunk2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "added-late" is at new-file line 13 in canonAHunk2 (@@ -10,3 +11,4 @@:
	// line10=11, line11=12, added-late=13).
	comments := []model.LlmComment{{Path: "a.txt", ExistingCode: "added-late"}}
	resolved := ResolveLineNumbers(comments, got)
	if resolved[0].StartLine != 13 || resolved[0].EndLine != 13 {
		t.Fatalf("line resolution wrong: got start=%d end=%d, want 13/13",
			resolved[0].StartLine, resolved[0].EndLine)
	}
}

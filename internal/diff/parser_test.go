package diff

import (
	"context"
	"testing"
)

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

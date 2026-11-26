// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestRelocateAcrossFiles_DeletedFileUsesOriginalPath(t *testing.T) {
	const removed = `diff --git a/src/legacy.c b/src/legacy.c
deleted file mode 100644
--- a/src/legacy.c
+++ /dev/null
@@ -1,3 +0,0 @@
-void close_handle(void) {
-    release(handle);
-}
`
	diffs, err := ParseDiffText(context.Background(), removed, t.TempDir(), "", nil)
	if err != nil || len(diffs) != 1 || !diffs[0].IsDeleted || diffs[0].NewPath != "/dev/null" {
		t.Fatalf("parsed deletion = %+v, err = %v", diffs, err)
	}

	t.Run("unique deleted file", func(t *testing.T) {
		cm := &model.LlmComment{Path: "src/caller.c", ExistingCode: "release(handle);"}
		path, ok := RelocateAcrossFiles(cm, diffs)
		if !ok || path != "src/legacy.c" || cm.Path != path || cm.StartLine != 2 || cm.EndLine != 2 {
			t.Fatalf("relocated to %q, ok = %v, comment = %+v", path, ok, cm)
		}
	})
	t.Run("original path is still excluded", func(t *testing.T) {
		cm := model.LlmComment{Path: "src/legacy.c", ExistingCode: "release(handle);"}
		before := cm
		if path, ok := RelocateAcrossFiles(&cm, diffs); ok || path != "" || cm != before {
			t.Fatalf("own file changed comment to %+v, path = %q, ok = %v", cm, path, ok)
		}
	})
	t.Run("deleted and surviving matches stay ambiguous", func(t *testing.T) {
		candidates := append([]model.Diff{}, diffs...)
		candidates = append(candidates, model.Diff{NewPath: "src/replacement.c", NewFileContent: "release(handle);\n"})
		cm := model.LlmComment{Path: "src/caller.c", ExistingCode: "release(handle);", StartLine: 7, EndLine: 8}
		before := cm
		if path, ok := RelocateAcrossFiles(&cm, candidates); ok || path != "" || cm != before {
			t.Fatalf("ambiguous match changed comment to %+v, path = %q, ok = %v", cm, path, ok)
		}
	})
	t.Run("renamed file keeps its new path", func(t *testing.T) {
		cm := &model.LlmComment{Path: "src/caller.c", ExistingCode: "release(handle);"}
		renamed := model.Diff{OldPath: "src/legacy.c", NewPath: "src/renamed.c", IsRenamed: true, NewFileContent: "release(handle);\n"}
		path, ok := RelocateAcrossFiles(cm, []model.Diff{renamed})
		if !ok || path != "src/renamed.c" || cm.Path != path || cm.StartLine != 1 || cm.EndLine != 1 {
			t.Fatalf("renamed file location = %+v, path = %q, ok = %v", cm, path, ok)
		}
	})
}

// headerDiff declares the function; sourceDiff implements it. This is the split
// that produces mis-filed comments in practice: the Agent reviews the header,
// reads the source through file_read_diff, and files a comment about the body
// against the header.
const headerDiff = `diff --git a/src/span.h b/src/span.h
--- a/src/span.h
+++ b/src/span.h
@@ -10,3 +10,4 @@
 void span_set_text(span_t * span, const char * text);
+void span_set_text_fmt(span_t * span, const char * fmt, ...);
`

const sourceDiff = `diff --git a/src/span.c b/src/span.c
--- a/src/span.c
+++ b/src/span.c
@@ -40,4 +40,7 @@
 void span_set_text_fmt(span_t * span, const char * fmt, ...)
 {
+	char * text = span_vfmt(fmt, args);
+	if(text == NULL) return;
+	va_end(args);
 }
`

func diffsFixture() []model.Diff {
	return []model.Diff{
		{NewPath: "src/span.h", OldPath: "src/span.h", Diff: headerDiff},
		{NewPath: "src/span.c", OldPath: "src/span.c", Diff: sourceDiff},
	}
}

func TestRelocateAcrossFiles_RefilesToImplementation(t *testing.T) {
	cm := &model.LlmComment{
		Path:         "src/span.h",
		Content:      "va_end is skipped on the early return",
		ExistingCode: "\tif(text == NULL) return;\n\tva_end(args);",
	}

	got, ok := RelocateAcrossFiles(cm, diffsFixture())
	if !ok {
		t.Fatal("expected a unique hit in src/span.c")
	}
	if got != "src/span.c" || cm.Path != "src/span.c" {
		t.Fatalf("path = %q, cm.Path = %q; want src/span.c for both", got, cm.Path)
	}
	// Line numbers must move with the path, or the comment is re-filed onto the
	// right file while still pointing at the wrong line.
	if cm.StartLine <= 0 || cm.EndLine < cm.StartLine {
		t.Fatalf("StartLine/EndLine = %d/%d; want a resolved range", cm.StartLine, cm.EndLine)
	}
}

func TestRelocateAcrossFiles_DeclinesWhenAmbiguous(t *testing.T) {
	// The same excerpt in two files: re-filing onto either one would just swap
	// one wrong location for another, so the comment must be left alone.
	dup := `diff --git a/src/other.c b/src/other.c
--- a/src/other.c
+++ b/src/other.c
@@ -1,2 +1,4 @@
+	char * text = span_vfmt(fmt, args);
+	if(text == NULL) return;
+	va_end(args);
 }
`
	diffs := append(diffsFixture(), model.Diff{NewPath: "src/other.c", OldPath: "src/other.c", Diff: dup})
	cm := &model.LlmComment{
		Path:         "src/span.h",
		ExistingCode: "\tif(text == NULL) return;\n\tva_end(args);",
	}

	if _, ok := RelocateAcrossFiles(cm, diffs); ok {
		t.Fatal("expected no verdict when the excerpt matches more than one file")
	}
	if cm.Path != "src/span.h" || cm.StartLine != 0 || cm.EndLine != 0 {
		t.Fatalf("cm mutated on decline: path=%q start=%d end=%d", cm.Path, cm.StartLine, cm.EndLine)
	}
}

func TestRelocateAcrossFiles_DeclinesWhenAbsent(t *testing.T) {
	cm := &model.LlmComment{
		Path:         "src/span.h",
		ExistingCode: "int nothing_here = 0;",
	}
	if _, ok := RelocateAcrossFiles(cm, diffsFixture()); ok {
		t.Fatal("expected no hit for code absent from every diff")
	}
	if cm.Path != "src/span.h" {
		t.Fatalf("cm.Path mutated to %q on decline", cm.Path)
	}
}

func TestRelocateAcrossFiles_SkipsOwnFileAndEmptyInputs(t *testing.T) {
	// A comment already resolvable in its own file must not be re-filed: the
	// caller only reaches here after same-file resolution failed, but the
	// function still has to exclude cm.Path so a self-match cannot masquerade
	// as a cross-file hit.
	cm := &model.LlmComment{
		Path:         "src/span.c",
		ExistingCode: "\tva_end(args);",
	}
	if _, ok := RelocateAcrossFiles(cm, diffsFixture()); ok {
		t.Fatal("expected the comment's own file to be excluded from the search")
	}

	for name, arg := range map[string]*model.LlmComment{
		"nil comment":        nil,
		"empty ExistingCode": {Path: "src/span.h"},
	} {
		if _, ok := RelocateAcrossFiles(arg, diffsFixture()); ok {
			t.Fatalf("%s: expected no hit", name)
		}
	}
	if _, ok := RelocateAcrossFiles(&model.LlmComment{ExistingCode: "x"}, nil); ok {
		t.Fatal("empty diff set: expected no hit")
	}
}

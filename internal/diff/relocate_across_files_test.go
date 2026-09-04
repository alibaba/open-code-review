// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/location"
	"github.com/alibaba/open-code-review/internal/model"
)

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

func TestRelocateAcrossFilesCarriesResolvedSide(t *testing.T) {
	comment := &model.LlmComment{
		Path:         "src/span.h",
		ExistingCode: "\tif(text == NULL) return;\n\tva_end(args);",
	}
	path, side, ok := RelocateAcrossFilesWithSide(comment, diffsFixture())
	if !ok || path != "src/span.c" || side != location.SideNew {
		t.Fatalf("RelocateAcrossFilesWithSide = %q, %q, %v", path, side, ok)
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

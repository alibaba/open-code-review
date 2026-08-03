package diff

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/hashline"
	"github.com/alibaba/open-code-review/internal/model"
)

func anchorFor(content string, line int) string {
	lines := strings.Split(content, "\n")
	return hashline.FormatAnchor(lines, line-1)
}

func TestResolveComment_AnchorFastPath(t *testing.T) {
	content := "a\nb\nrepeated\nc\nrepeated\nd\n"
	d := &model.Diff{NewPath: "f.go", NewFileContent: content, Diff: "@@ -1,6 +1,6 @@\n a\n b\n+repeated\n c\n repeated\n d"}

	// Anchor points at the SECOND "repeated" (line 5) — text matching would
	// always pick the first occurrence (line 3).
	cm := &model.LlmComment{Path: "f.go", Content: "x", ExistingCode: "repeated", Anchor: anchorFor(content, 5)}
	if !ResolveComment(cm, d) {
		t.Fatal("anchor resolution failed")
	}
	if cm.StartLine != 5 || cm.EndLine != 5 || cm.LocMethod != "anchor" {
		t.Fatalf("got (%d,%d,%s), want (5,5,anchor)", cm.StartLine, cm.EndLine, cm.LocMethod)
	}

	// Same comment without anchor: sliding window picks line 3 (ambiguity bug).
	cm2 := &model.LlmComment{Path: "f.go", Content: "x", ExistingCode: "repeated"}
	if !ResolveComment(cm2, d) {
		t.Fatal("text resolution failed")
	}
	if cm2.StartLine != 3 {
		t.Fatalf("expected legacy path to pick first occurrence (3), got %d", cm2.StartLine)
	}
}

func TestResolveComment_AnchorHintVeto(t *testing.T) {
	content := "alpha\nbeta\ngamma\n"
	d := &model.Diff{NewPath: "f.go", NewFileContent: content, Diff: "@@ -1,3 +1,3 @@\n alpha\n+beta\n gamma"}

	// Valid anchor for line 2 but existing_code claims totally different code:
	// the hint veto must reject the anchor, then text fallback also fails.
	cm := &model.LlmComment{Path: "f.go", Content: "x", ExistingCode: "does_not_exist()", Anchor: anchorFor(content, 2)}
	if ResolveComment(cm, d) {
		t.Fatalf("expected veto+fallback failure, got (%d,%d,%s)", cm.StartLine, cm.EndLine, cm.LocMethod)
	}
	if cm.LocMethod != "anchor_hint_veto" {
		t.Fatalf("LocMethod = %q, want anchor_hint_veto", cm.LocMethod)
	}
}

func TestResolveComment_BadAnchorFallsBackToText(t *testing.T) {
	content := "alpha\nbeta\ngamma\n"
	d := &model.Diff{NewPath: "f.go", NewFileContent: content, Diff: "@@ -1,3 +1,3 @@\n alpha\n+beta\n gamma"}

	cm := &model.LlmComment{Path: "f.go", Content: "x", ExistingCode: "beta", Anchor: "2#ZZ"} // wrong hash
	if !ResolveComment(cm, d) {
		t.Fatal("expected fallback text resolution to succeed")
	}
	if cm.StartLine != 2 || cm.LocMethod != "hunk" {
		t.Fatalf("got (%d,%s), want (2,hunk)", cm.StartLine, cm.LocMethod)
	}
}

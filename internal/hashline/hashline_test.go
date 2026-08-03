package hashline

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestAnchorRoundTrip(t *testing.T) {
	file := "package main\n\nfunc main() {\n\tx := 1\n\tx := 1\n\tprintln(x)\n}\n"
	lines := strings.Split(file, "\n")
	for i := range lines {
		spec := FormatAnchor(lines, i)
		s, e, ok := ResolveSpec(spec, file)
		if !ok || s != i+1 || e != i+1 {
			t.Fatalf("line %d: spec %q resolved to (%d,%d,%v)", i+1, spec, s, e, ok)
		}
	}
}

func TestIdenticalLinesGetDifferentContextHashes(t *testing.T) {
	// Lines 4 and 5 are identical ("\tx := 1") but have different neighbors.
	lines := []string{"a", "x := 1", "b", "c", "x := 1", "d"}
	h1 := ComputeLineHash(lines, 1)
	h2 := ComputeLineHash(lines, 4)
	if h1 == h2 {
		t.Fatalf("expected different hashes for identical lines in different contexts, both %q", h1)
	}
}

func TestWrongHashRejected(t *testing.T) {
	file := "one\ntwo\nthree\n"
	lines := strings.Split(file, "\n")
	good := FormatAnchor(lines, 1) // "2#XX"
	// Corrupt the hash deterministically: swap to a different valid pair.
	bad := good[:len(good)-2]
	if strings.HasSuffix(good, "ZZ") {
		bad += "PP"
	} else {
		bad += "ZZ"
	}
	if _, _, ok := ResolveSpec(bad, file); ok {
		t.Fatalf("corrupted anchor %q (from %q) should not resolve", bad, good)
	}
}

func TestRangeSpec(t *testing.T) {
	file := "alpha\nbeta\ngamma\ndelta\n"
	lines := strings.Split(file, "\n")
	spec := FormatAnchor(lines, 1) + "-" + FormatAnchor(lines, 3)
	s, e, ok := ResolveSpec(spec, file)
	if !ok || s != 2 || e != 4 {
		t.Fatalf("range spec %q => (%d,%d,%v), want (2,4,true)", spec, s, e, ok)
	}
}

func TestAnnotateDiff(t *testing.T) {
	newContent := "line one\nline two changed\nline three\nline four added\nline five\n"
	diffText := "@@ -1,4 +1,5 @@\n line one\n-line two\n+line two changed\n line three\n+line four added\n line five"
	d := &model.Diff{Diff: diffText, NewFileContent: newContent}
	out := AnnotateDiff(d)
	lines := strings.Split(out, "\n")
	newLines := strings.Split(newContent, "\n")

	wantPrefix := map[int]string{ // diff line idx -> expected anchor prefix
		1: FormatAnchor(newLines, 0) + ": line one",
		3: FormatAnchor(newLines, 1) + ":+line two changed",
		4: FormatAnchor(newLines, 2) + ": line three",
		5: FormatAnchor(newLines, 3) + ":+line four added",
		6: FormatAnchor(newLines, 4) + ": line five",
	}
	if lines[0] != "@@ -1,4 +1,5 @@" {
		t.Fatalf("hunk header changed: %q", lines[0])
	}
	if lines[2] != "-line two" {
		t.Fatalf("deleted line should be untouched, got %q", lines[2])
	}
	for idx, want := range wantPrefix {
		if lines[idx] != want {
			t.Errorf("diff line %d = %q, want %q", idx, lines[idx], want)
		}
	}

	// Every anchor in the annotated diff must resolve against the new file.
	for _, l := range lines {
		hashPos := strings.Index(l, "#")
		colon := strings.Index(l, ":")
		if hashPos < 0 || colon < 0 || hashPos > colon {
			continue
		}
		spec := l[:colon]
		if _, _, ok := ResolveSpec(spec, newContent); !ok {
			t.Errorf("annotated anchor %q does not resolve", spec)
		}
	}
}

func TestAnnotateDiffPassthroughWithoutContent(t *testing.T) {
	d := &model.Diff{Diff: "@@ -1 +1 @@\n-a\n+b"}
	if got := AnnotateDiff(d); got != d.Diff {
		t.Fatalf("expected passthrough, got %q", got)
	}
}

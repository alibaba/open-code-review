// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/model"
)

// sampleComment carries every field that drives a colored render path: the
// file header, a severity badge, and a suggestion diff.
func sampleComment() model.LlmComment {
	return model.LlmComment{
		Path:           "internal/mcp/client.go",
		StartLine:      27,
		EndLine:        28,
		Content:        "Potential environment variable leak.",
		Category:       "security",
		Severity:       "high",
		ExistingCode:   "old := 1\n",
		SuggestionCode: "new := 1\n",
	}
}

// TestRenderComment_NoColorIsPlain is the regression test for issue #682:
// piping review output must not emit ANSI escape sequences.
func TestRenderComment_NoColorIsPlain(t *testing.T) {
	setColor(t, false)
	out := captureStdout(t, func() { renderComment(sampleComment(), os.Stdout) })

	if strings.Contains(out, "\033") {
		t.Errorf("output contains an ANSI escape with color disabled:\n%q", out)
	}
	// Plain mode must still carry all the information, not just drop the styling.
	for _, want := range []string{
		"internal/mcp/client.go:27-28",
		"[security · high]",
		"Potential environment variable leak.",
		"+ new := 1",
		"- old := 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plain output missing %q:\n%s", want, out)
		}
	}
}

// TestRenderComment_ColorEmitsEscapes is the paired positive case: a real
// terminal still gets the styled render.
func TestRenderComment_ColorEmitsEscapes(t *testing.T) {
	setColor(t, true)
	out := captureStdout(t, func() { renderComment(sampleComment(), os.Stdout) })

	// The colorized badge itself is asserted by TestRenderComment_BadgeInline;
	// what is new here is the file header.
	if !strings.Contains(out, "\033[2m─── internal/mcp/client.go:27-28 ───\033[0m") {
		t.Errorf("expected colorized file header:\n%q", out)
	}
}

func TestPrintDiffLine_NoColor(t *testing.T) {
	setColor(t, false)
	for _, tc := range []struct{ prefix, content, want string }{
		{"+", "added line", "+ added line\n"},
		{"-", "removed line", "- removed line\n"},
		{" ", "context line", "  context line\n"},
	} {
		got := captureStdout(t, func() {
			printDiffLine(os.Stdout, tc.prefix, tc.content, "\033[92m", "\033[48;2;0;60;0m")
		})
		if got != tc.want {
			t.Errorf("printDiffLine(%q) = %q, want %q", tc.prefix, got, tc.want)
		}
	}
}

func TestStatusBadge_NoColor(t *testing.T) {
	setColor(t, false)
	want := map[string]string{
		"added": "[A]", "modified": "[M]", "deleted": "[D]",
		"renamed": "[R]", "binary": "[B]", "scan": "[S]", "unknown": "[?]",
	}
	for status, exp := range want {
		if got := statusBadge(status); got != exp {
			t.Errorf("statusBadge(%q) = %q, want bare %q", status, got, exp)
		}
	}
}

func TestStatusBadge_Color(t *testing.T) {
	setColor(t, true)
	if got := statusBadge("added"); got != "\033[32m[A]\033[0m" {
		t.Errorf("statusBadge(added) = %q, want colorized", got)
	}
	// The unknown badge has no color in either mode.
	if got := statusBadge("unknown"); got != "[?]" {
		t.Errorf("statusBadge(unknown) = %q, want %q", got, "[?]")
	}
}

func previewFixture() *agent.DiffPreview {
	return &agent.DiffPreview{
		TotalFiles:      2,
		TotalInsertions: 10,
		TotalDeletions:  3,
		ReviewableCount: 1,
		ExcludedCount:   1,
		Entries: []agent.DiffPreviewEntry{
			{Path: "cmd/main.go", Status: "modified", Insertions: 10, Deletions: 3, WillReview: true},
			{Path: "go.sum", Status: "modified", WillReview: false, ExcludeReason: "excluded_pattern"},
		},
	}
}

// TestOutputPreviewText_NoColorIsPlain covers the second text path that emitted
// escapes: `ocr review --preview` piped to a file.
func TestOutputPreviewText_NoColorIsPlain(t *testing.T) {
	setColor(t, false)
	out := captureStdout(t, func() { outputPreviewText(previewFixture(), os.Stdout) })

	if strings.Contains(out, "\033") {
		t.Errorf("preview contains an ANSI escape with color disabled:\n%q", out)
	}
	for _, want := range []string{
		"Preview: 2 file(s) changed  |  +10  -3",
		"Will review (1):",
		"[M]", "cmd/main.go", "+10", "-3",
		"Excluded from review (1):",
		"go.sum", "(excluded_pattern)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plain preview missing %q:\n%s", want, out)
		}
	}
}

// TestOutputPreviewText_ColumnsAlign guards the padding: the counts are padded
// before colorizing, so the path/count columns line up in both modes.
func TestOutputPreviewText_ColumnsAlign(t *testing.T) {
	p := previewFixture()
	p.ReviewableCount = 2
	p.Entries = []agent.DiffPreviewEntry{
		{Path: "a.go", Status: "added", Insertions: 1, Deletions: 2, WillReview: true},
		{Path: "b.go", Status: "modified", Insertions: 1000, Deletions: 2000, WillReview: true},
	}
	p.ExcludedCount = 0

	setColor(t, false)
	plain := captureStdout(t, func() { outputPreviewText(p, os.Stdout) })
	var widths []int
	for _, ln := range strings.Split(plain, "\n") {
		if strings.Contains(ln, ".go") {
			widths = append(widths, strings.Index(ln, "+"))
		}
	}
	if len(widths) != 2 {
		t.Fatalf("expected 2 entry lines, got %d in:\n%s", len(widths), plain)
	}
	if widths[0] != widths[1] {
		t.Errorf("count columns misaligned at %v:\n%s", widths, plain)
	}
}

func TestOutputPreviewText_ColorEmitsEscapes(t *testing.T) {
	setColor(t, true)
	out := captureStdout(t, func() { outputPreviewText(previewFixture(), os.Stdout) })
	if !strings.Contains(out, "\033[32m+10\033[0m") {
		t.Errorf("expected colorized insertion total:\n%q", out)
	}
	if !strings.Contains(out, "\033[1mWill review (1):\033[0m") {
		t.Errorf("expected bold section header:\n%q", out)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestCaptureCommentEvidenceReviewedSides(t *testing.T) {
	d := &model.Diff{NewPath: "a.go", OldPath: "a.go", NewFileContent: "header\n  newCall()\n", Diff: "@@ -1,2 +1,2 @@\n header\n-  oldCall()\n+  newCall()\n"}
	for _, tc := range []struct {
		name, snippet, status, side, match, excerpt string
	}{
		{"new code", "  newCall()", "matched", "new", "exact", "  newCall()"},
		{"deleted code", "  oldCall()", "matched", "old", "exact", "  oldCall()"},
		{"whitespace", "newCall()", "matched", "new", "whitespace", "  newCall()"},
		{"unmatched claim", "missingCall()", "unmatched", "new", "none", "  newCall()"},
		{"missing snippet", "", "snippet_missing", "new", "none", "  newCall()"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cm := model.LlmComment{Path: "a.go", StartLine: 2, EndLine: 2, ExistingCode: tc.snippet}
			evidence := CaptureCommentEvidence(cm, d)
			if evidence.Status != tc.status || evidence.SnapshotSHA256 != evidenceHash(d.NewFileContent) || evidence.DiffSHA256 != evidenceHash(d.Diff) {
				t.Fatalf("snapshot evidence = %+v", evidence)
			}
			for _, span := range evidence.Spans {
				if span.Side == tc.side {
					if span.Match != tc.match || span.Excerpt != tc.excerpt || span.SHA256 != evidenceHash(tc.excerpt) {
						t.Fatalf("source bytes or match strength lost: %+v", span)
					}
					return
				}
			}
			t.Fatalf("missing side %s: %+v", tc.side, evidence.Spans)
		})
	}
	context := CaptureCommentEvidence(model.LlmComment{Path: "a.go", StartLine: 1, EndLine: 1, ExistingCode: "header"}, d)
	if len(context.Spans) != 2 || context.Spans[0].Match != "exact" || context.Spans[1].Match != "exact" {
		t.Fatal("context shared by both sides must not be assigned an invented unique side")
	}
}

func TestCaptureCommentEvidenceUnavailableAndHunks(t *testing.T) {
	cm := model.LlmComment{Path: "a.go", StartLine: 1, EndLine: 1, ExistingCode: "new"}
	for _, d := range []*model.Diff{nil, {NewPath: "other.go"}, {NewPath: "a.go"}} {
		if got := CaptureCommentEvidence(cm, d); got.Status != "unavailable" || len(got.Spans) != 0 {
			t.Fatalf("missing snapshot became matching evidence: %+v", got)
		}
	}
	for _, bounds := range [][2]int{{0, 0}, {2, 1}, {1, 2}} {
		probe := cm
		probe.StartLine, probe.EndLine = bounds[0], bounds[1]
		got := CaptureCommentEvidence(probe, &model.Diff{NewPath: "a.go", NewFileContent: "new\n"})
		if got.Status != "unavailable" {
			t.Fatalf("invalid or out-of-range coordinates became evidence: %+v", got)
		}
	}
	got := CaptureCommentEvidence(cm, &model.Diff{NewPath: "a.go", Diff: "@@ -1 +1 @@\n-old\n+new\n"})
	if got.Status != "matched" || got.Spans[0].Source != "diff_hunk" || got.Spans[0].Side != "new" {
		t.Fatalf("new-side hunk fallback = %+v", got)
	}
	cm.StartLine, cm.EndLine = 2, 2
	pastHunk := CaptureCommentEvidence(cm, &model.Diff{NewPath: "a.go", Diff: "@@ -1 +1 @@\n-old\n+new\n"})
	if len(pastHunk.Spans) != 0 || pastHunk.Status != "unavailable" {
		t.Fatalf("trailing diff newline became source evidence: %+v", pastHunk)
	}
	deleted := CaptureCommentEvidence(model.LlmComment{Path: "a.go", StartLine: 1, EndLine: 1, ExistingCode: "old"},
		&model.Diff{OldPath: "a.go", NewPath: "/dev/null", IsDeleted: true, Diff: "@@ -1 +0,0 @@\n-old\n"})
	if deleted.Status != "matched" || len(deleted.Spans) != 1 || deleted.Spans[0].Side != "old" {
		t.Fatalf("deleted file evidence = %+v", deleted)
	}
}

func TestCaptureCommentEvidenceBoundedExcerpt(t *testing.T) {
	code := strings.Repeat("x", 16383) + "\u03bb" + strings.Repeat("x", 20)
	got := CaptureCommentEvidence(model.LlmComment{Path: "a.go", StartLine: 1, EndLine: 1, ExistingCode: code}, &model.Diff{NewPath: "a.go", NewFileContent: code})
	span := got.Spans[0]
	if !span.ExcerptTruncated || len(span.Excerpt) > 16384 || !utf8.ValidString(span.Excerpt) || span.SHA256 != evidenceHash(code) || span.Match != "exact" {
		t.Fatal("bounded excerpt must preserve the full-span hash and match result")
	}
}

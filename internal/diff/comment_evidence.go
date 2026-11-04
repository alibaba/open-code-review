// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/model"
)

// CaptureCommentEvidence uses the reviewed snapshot, never the current working tree.
func CaptureCommentEvidence(cm model.LlmComment, d *model.Diff) model.CommentCodeEvidence {
	evidence := model.CommentCodeEvidence{Status: "unavailable", Path: cm.Path, Spans: []model.CommentCodeSpan{}}
	if d == nil || (d.NewPath != cm.Path && d.OldPath != cm.Path) {
		return evidence
	}
	evidence.DiffSHA256 = evidenceHash(d.Diff)
	if d.NewFileContent != "" {
		evidence.SnapshotSHA256 = evidenceHash(d.NewFileContent)
	}
	if cm.StartLine <= 0 || cm.EndLine < cm.StartLine {
		return evidence
	}
	appendSpan := func(side, source, content string) {
		match := "none"
		if strings.TrimSpace(cm.ExistingCode) != "" {
			if content == cm.ExistingCode {
				match = "exact"
			} else if strings.Join(strings.Fields(content), " ") == strings.Join(strings.Fields(cm.ExistingCode), " ") {
				match = "whitespace"
			}
		}
		excerpt := content
		const maxExcerptBytes = 16 * 1024
		if len(excerpt) > maxExcerptBytes {
			excerpt = excerpt[:maxExcerptBytes]
			for !utf8.ValidString(excerpt) {
				excerpt = excerpt[:len(excerpt)-1]
			}
		}
		evidence.Spans = append(evidence.Spans, model.CommentCodeSpan{
			Side: side, Source: source, StartLine: cm.StartLine, EndLine: cm.EndLine,
			SHA256: evidenceHash(content), Match: match, Excerpt: excerpt, ExcerptTruncated: len(excerpt) < len(content),
		})
	}
	if d.NewFileContent != "" && !d.IsDeleted {
		lines := strings.Split(strings.ReplaceAll(d.NewFileContent, "\r\n", "\n"), "\n")
		if lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		if cm.EndLine <= len(lines) {
			appendSpan("new", "new_file_content", strings.Join(lines[cm.StartLine-1:cm.EndLine], "\n"))
		}
	}
	for _, hunk := range ParseHunks(d.Diff) {
		for _, side := range []bool{true, false} {
			if side && d.NewFileContent != "" && !d.IsDeleted {
				continue
			}
			var span []string
			next := hunk.OldStart
			count := hunk.OldCount
			if side {
				next = hunk.NewStart
				count = hunk.NewCount
			}
			// ParseHunks may retain a trailing empty split line; the header
			// bounds keep it from becoming evidence for a nonexistent line.
			if next <= 0 || count <= 0 || cm.StartLine < next || cm.EndLine-next >= count {
				continue
			}
			for _, line := range hunk.Lines {
				if (side && line.Type == HunkDeleted) || (!side && line.Type == HunkAdded) {
					continue
				}
				if next >= cm.StartLine && next <= cm.EndLine {
					span = append(span, line.Content)
				}
				next++
			}
			if len(span) == cm.EndLine-cm.StartLine+1 {
				label := "old"
				if side {
					label = "new"
				}
				appendSpan(label, "diff_hunk", strings.Join(span, "\n"))
			}
		}
	}
	if strings.TrimSpace(cm.ExistingCode) == "" {
		evidence.Status = "snippet_missing"
	} else if len(evidence.Spans) > 0 {
		evidence.Status = "unmatched"
		for _, span := range evidence.Spans {
			if span.Match != "none" {
				evidence.Status = "matched"
			}
		}
	}
	return evidence
}

func evidenceHash(s string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))
}

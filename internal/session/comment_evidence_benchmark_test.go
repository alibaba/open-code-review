// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"bufio"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

type evidenceByteCounter struct{ bytes int64 }

func (c *evidenceByteCounter) Write(p []byte) (int, error) {
	c.bytes += int64(len(p))
	return len(p), nil
}

func BenchmarkCommentEvidenceSerialization(b *testing.B) {
	counter := &evidenceByteCounter{}
	w := &jsonlWriter{sessionID: "benchmark-session", writer: bufio.NewWriter(counter)}
	sh := &SessionHistory{persist: w}
	tr := &TaskRecord{Type: MainTask, RequestNo: 1, requestID: "request-id", responseID: "response-id", fileSession: &FileSession{FilePath: "a.go", session: sh}}
	call := llm.ToolCall{ID: "call-id", Function: llm.FunctionCall{Name: "code_comment"}}
	original := model.LlmComment{Path: "a.go", Content: "Check the error classification", ExistingCode: "  reviewedCall()", StartLine: 2, EndLine: 2}
	code := model.CommentCodeEvidence{Status: "matched", Path: "a.go", SnapshotSHA256: "snapshot-sha256", DiffSHA256: "diff-sha256", Spans: []model.CommentCodeSpan{{Side: "new", Source: "new_file_content", StartLine: 2, EndLine: 2, Excerpt: "  reviewedCall()", Match: "exact", SHA256: "span-sha256"}}}
	code.SnapshotSHA256, code.DiffSHA256, code.Spans[0].SHA256 = strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	b.ReportAllocs()
	for b.Loop() {
		cm := original
		tr.RecordCommentEvidence(call, 0, original, &cm, code, nil)
		tr.RecordCommentDecision("review_filter", 0, cm, &cm)
	}
	b.StopTimer()
	if err := w.writer.Flush(); err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(counter.bytes)/float64(b.N), "jsonl-bytes/comment")
}

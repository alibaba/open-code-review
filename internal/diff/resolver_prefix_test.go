// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestMatchConsecutive_RepeatedPrefixes(t *testing.T) {
	tests := []struct {
		name   string
		source []string
		target []string
		start  int
		end    int
	}{
		{"overlap", []string{"a", "b", "a", "b", "a", "b", "c"}, []string{"a", "b", "a", "b", "c"}, 2, 6},
		{"nested fallback", []string{"a", "a", "a", "b", "a", "a", "b"}, []string{"a", "a", "b", "a", "a", "b"}, 1, 6},
		{"first occurrence", []string{"a", "b", "a", "b", "a"}, []string{"a", "b", "a"}, 0, 2},
		{"unmatched suffix", []string{"a", "b", "a", "b", "a", "b"}, []string{"a", "b", "a", "b", "c"}, -1, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := make([]indexedLine, len(tt.source))
			for i, content := range tt.source {
				lines[i] = indexedLine{lineNum: 10 + i*3, content: content}
			}
			start, end, found := matchConsecutive(lines, tt.target)
			if tt.start < 0 {
				if found {
					t.Fatalf("unexpected match at %d-%d", start, end)
				}
				return
			}
			if !found || start != lines[tt.start].lineNum || end != lines[tt.end].lineNum {
				t.Fatalf("match = %d-%d, %v; want %d-%d", start, end, found, lines[tt.start].lineNum, lines[tt.end].lineNum)
			}
		})
	}
}

func TestResolveFromFileContent_RepeatedPrefixLineNumbers(t *testing.T) {
	d := model.Diff{NewFileContent: "a\r\n\r\nb\r\na\r\nb\r\na\r\n \t\r\nb\r\nc\r\na\r\nb\r\na\r\nb\r\nc\r\n"}
	cm := model.LlmComment{ExistingCode: "a\nb\na\nb\nc"}
	if !resolveFromFileContent(&d, &cm) || cm.StartLine != 4 || cm.EndLine != 9 {
		t.Fatalf("expected first complete match at 4-9, got %+v", cm)
	}
}

func TestResolveFromFileContent_RepeatedPrefixMissPreservesComment(t *testing.T) {
	d := model.Diff{NewFileContent: "a\nb\na\nb\na\nb\n"}
	cm := model.LlmComment{ExistingCode: "a\nb\na\nb\nc", StartLine: 7, EndLine: 11}
	if resolveFromFileContent(&d, &cm) || cm.StartLine != 7 || cm.EndLine != 11 {
		t.Fatalf("failed match changed comment: %+v", cm)
	}
}

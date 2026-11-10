// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package reviewtask

import (
	"context"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/tool"
)

// TestExecuteToolCall_DropsModelLineWhenNewFileEmpty locks the empty-body
// cases lineHintInFile used to accept: a deleted diff, a binary diff, and a
// read that left NewFileContent empty. A line past that body is not an anchor.
func TestExecuteToolCall_DropsModelLineWhenNewFileEmpty(t *testing.T) {
	cases := []struct {
		name string
		diff model.Diff
	}{
		{
			name: "deleted",
			diff: model.Diff{
				IsDeleted: true,
				NewPath:   "/dev/null",
				Diff:      "@@ -1,2 +0,0 @@\n-keep\n-me\n",
			},
		},
		{
			name: "binary",
			diff: model.Diff{IsBinary: true, NewPath: "blob.bin"},
		},
		{
			name: "unread",
			diff: model.Diff{NewPath: "missing.go"},
		},
	}
	const arguments = `{"comments":[{"path":"gone.go","content":"removed","existing_code":"not in the file","start_line":99999,"end_line":99999}]}`
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comments := collectCodeComment(t, arguments, func(string) *model.Diff {
				d := tc.diff
				return &d
			})
			if comments[0].StartLine != 0 || comments[0].EndLine != 0 {
				t.Fatalf("finding line = %d-%d, want 0-0 when NewFileContent is empty", comments[0].StartLine, comments[0].EndLine)
			}
		})
	}
}

// TestExecuteToolCall_KeepsModelLineWhenDiffNotLoaded is the one case with
// nothing on hand to disprove a positive model range.
func TestExecuteToolCall_KeepsModelLineWhenDiffNotLoaded(t *testing.T) {
	const arguments = `{"comments":[{"path":"calc.go","content":"bug","existing_code":"a - b","start_line":4,"end_line":4}]}`
	comments := collectCodeComment(t, arguments, func(string) *model.Diff { return nil })
	if comments[0].StartLine != 4 || comments[0].EndLine != 4 {
		t.Fatalf("finding line = %d-%d, want 4-4 when no diff was loaded", comments[0].StartLine, comments[0].EndLine)
	}
}

func collectCodeComment(t *testing.T, arguments string, lookup func(string) *model.Diff) []model.LlmComment {
	t.Helper()
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()
	r := NewRunner(Deps{
		Tools:            reg,
		CommentCollector: collector,
		DiffLookup:       lookup,
	})
	cp := r.executeToolCall(context.Background(), "gone.go", llm.ToolCall{
		Function: llm.FunctionCall{
			Name:      tool.CodeComment.Name(),
			Arguments: arguments,
		},
	}, nil, "")
	if cp.Data != tool.CommentSucceed {
		t.Fatalf("cp.Data = %q, want CommentSucceed", cp.Data)
	}
	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatalf("collected %d comments, want 1", len(comments))
	}
	return comments
}

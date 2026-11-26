// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package reviewtask

import (
	"context"
	"testing"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestCodeCommentRelocatesDeletedFileUnderOriginalPath(t *testing.T) {
	const removed = `diff --git a/legacy.go b/legacy.go
deleted file mode 100644
--- a/legacy.go
+++ /dev/null
@@ -1,3 +0,0 @@
-func check() {
-    validate(request)
-}
`
	diffs, err := diff.ParseDiffText(context.Background(), removed, t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, async := range []bool{false, true} {
		name := "sync"
		if async {
			name = "async"
		}
		t.Run(name, func(t *testing.T) {
			collector := tool.NewCommentCollector()
			registry := tool.NewRegistry()
			registry.Register(&tool.CodeCommentProvider{Collector: collector})
			registry.Freeze()
			deps := Deps{
				Tools:            registry,
				CommentCollector: collector,
				AllDiffs:         func() []model.Diff { return diffs },
			}
			if async {
				deps.CommentWorkerPool = NewCommentWorkerPool(1)
			}
			runner := NewRunner(deps)
			result := runner.executeToolCall(context.Background(), "caller.go", llm.ToolCall{
				Function: llm.FunctionCall{
					Name:      tool.CodeComment.Name(),
					Arguments: `{"comments":[{"path":"caller.go","content":"The validation was removed without a replacement","existing_code":"validate(request)"}]}`,
				},
			}, nil, "")
			if result.Data != tool.CommentSucceed {
				t.Fatalf("code_comment result = %+v", result)
			}
			runner.CollectPendingComments()
			comments := collector.CommentsForPath("legacy.go")
			if len(comments) != 1 || comments[0].StartLine != 2 || comments[0].EndLine != 2 {
				t.Fatalf("findings for deleted file = %+v; all findings = %+v", comments, collector.Comments())
			}
			if len(collector.CommentsForPath("/dev/null")) != 0 || len(collector.CommentsForPath("caller.go")) != 0 {
				t.Fatalf("finding retained an incorrect path: %+v", collector.Comments())
			}
			warnings := runner.Warnings()
			if len(warnings) != 1 || warnings[0].Type != "comment_refiled" || warnings[0].File != "legacy.go" {
				t.Fatalf("relocation warnings = %+v", warnings)
			}
		})
	}
}

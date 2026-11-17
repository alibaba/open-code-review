// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package reviewtask

import (
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/tool"
	"testing"
)

func TestCollectPendingComments_NilPool(t *testing.T) {
	collector := tool.NewCommentCollector()
	collector.Add(model.LlmComment{Path: "a.go", Content: "fix"})

	r := NewRunner(Deps{
		CommentCollector:  collector,
		CommentWorkerPool: nil,
	})
	comments := r.CollectPendingComments()
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Path != "a.go" {
		t.Errorf("comment.Path = %q", comments[0].Path)
	}
}

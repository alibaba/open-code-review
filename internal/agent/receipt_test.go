// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/model"
)

// TestFileGroups_JoinsReceiptState pins the #2124 contract: the JSON result's
// groups carry the per-group state rollup joined from the session's receipt
// snapshot, keyed by dispatch order, while label/files keep their shape.
func TestFileGroups_JoinsReceiptState(t *testing.T) {
	a := New(Args{
		Model: "fake",
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 5,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{
					{Role: "user", Content: "Review {{diffs}}"},
				},
			},
		},
	})
	a.fileGroups = []FileGroup{
		{Label: "auth", Diffs: []model.Diff{{NewPath: "a.go"}, {NewPath: "b.go"}}},
		{Label: "docs", Diffs: []model.Diff{{NewPath: "c.md"}}},
	}
	a.session.SetReceiptGroups(receiptGroupsFrom(a.fileGroups))
	a.session.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", nil)
	a.session.RecordReviewItemFailed("b.go", "b.go", "b.go", "fp-b", "rate limited")

	groups := a.FileGroups()
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	first := groups[0]
	if first.Label != "auth" || len(first.Files) != 2 {
		t.Fatalf("label/files shape changed: %+v", first)
	}
	if first.State != "partial" || first.CompletedFiles != 1 {
		t.Errorf("rollup = state %q completed %d, want partial/1", first.State, first.CompletedFiles)
	}
	if len(first.MissingFiles) != 1 || first.MissingFiles[0] != "b.go" {
		t.Errorf("missing files = %v, want [b.go]", first.MissingFiles)
	}
	if first.FailureCode == "" {
		t.Errorf("failure_code must surface the failed item's reason")
	}
	second := groups[1]
	if second.State != "pending" || second.CompletedFiles != 0 || len(second.MissingFiles) != 1 {
		t.Errorf("untouched group rollup = %+v, want pending with c.md missing", second)
	}
}

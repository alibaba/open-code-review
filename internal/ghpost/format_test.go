// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package ghpost

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestBuildBadgeImage(t *testing.T) {
	tests := []struct {
		name    string
		comment model.LlmComment
		want    string
	}{
		{name: "high", comment: model.LlmComment{Category: "bug", Severity: "high"}, want: "![bug · high](https://img.shields.io/badge/bug-high-red)"},
		{name: "critical", comment: model.LlmComment{Category: "security", Severity: "critical"}, want: "![security · critical](https://img.shields.io/badge/security-critical-darkred)"},
		{name: "category only", comment: model.LlmComment{Category: "documentation"}, want: "![documentation](https://img.shields.io/badge/documentation-blue)"},
		{name: "unknown severity", comment: model.LlmComment{Category: "bug", Severity: "extreme"}, want: "![bug · extreme](https://img.shields.io/badge/bug-extreme-blue)"},
		{name: "escaped alt", comment: model.LlmComment{Category: "x]y", Severity: "high"}, want: "![x\\]y · high](https://img.shields.io/badge/x%5Dy-high-red)"},
		{name: "control chars", comment: model.LlmComment{Category: "bu\ng", Severity: "hi\tgh"}, want: "![bug · high](https://img.shields.io/badge/bug-high-red)"},
		{name: "empty", comment: model.LlmComment{}, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildBadgeImage(tc.comment); got != tc.want {
				t.Fatalf("buildBadgeImage = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatCommentBodyUsesMarkdownBadgeWithoutBold(t *testing.T) {
	body := formatCommentBody(model.LlmComment{
		Category:       "bug",
		Severity:       "medium",
		Content:        "Keep the guard.",
		SuggestionCode: "if ready {\n\trun()\n}",
	})
	if !strings.HasPrefix(body, "![bug · medium](https://img.shields.io/badge/bug-medium-orange)\n\nKeep the guard.") {
		t.Fatalf("body = %q", body)
	}
	if strings.Contains(body, "**![") || strings.Contains(body, "]**") {
		t.Fatalf("badge is bold: %q", body)
	}
	if !strings.Contains(body, "```suggestion\nif ready") {
		t.Fatalf("safe suggestion is not fenced: %q", body)
	}
}

func TestFormatCommentBodyIndentsSuggestionContainingFence(t *testing.T) {
	body := formatCommentBody(model.LlmComment{SuggestionCode: "before\n```go\nafter"})
	if !strings.Contains(body, "**Suggested code:**\n\n    before\n    ```go\n    after\n") {
		t.Fatalf("fenced suggestion = %q", body)
	}
}

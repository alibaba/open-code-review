// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alibaba/open-code-review/internal/model"
)

const (
	codeCommentCategoryBug             = "bug"
	codeCommentCategorySecurity        = "security"
	codeCommentCategoryPerformance     = "performance"
	codeCommentCategoryMaintainability = "maintainability"
	codeCommentCategoryTest            = "test"
	codeCommentCategoryStyle           = "style"
	codeCommentCategoryDocumentation   = "documentation"
	codeCommentCategoryOther           = "other"

	codeCommentSeverityCritical = "critical"
	codeCommentSeverityHigh     = "high"
	codeCommentSeverityMedium   = "medium"
	codeCommentSeverityLow      = "low"
)

var validCodeCommentCategories = map[string]struct{}{
	codeCommentCategoryBug:             {},
	codeCommentCategorySecurity:        {},
	codeCommentCategoryPerformance:     {},
	codeCommentCategoryMaintainability: {},
	codeCommentCategoryTest:            {},
	codeCommentCategoryStyle:           {},
	codeCommentCategoryDocumentation:   {},
	codeCommentCategoryOther:           {},
}

var validCodeCommentSeverities = map[string]struct{}{
	codeCommentSeverityCritical: {},
	codeCommentSeverityHigh:     {},
	codeCommentSeverityMedium:   {},
	codeCommentSeverityLow:      {},
}

// CodeCommentProvider submits review comments to the per-Agent CommentCollector.
type CodeCommentProvider struct {
	Collector *CommentCollector
}

func (p *CodeCommentProvider) Tool() Tool { return CodeComment }

func (p *CodeCommentProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	if p.Collector == nil {
		return "Error: comment collector is not configured", nil
	}

	comments, errMsg := ParseComments(args)
	if errMsg != "" {
		return errMsg, nil
	}

	for i := range comments {
		p.Collector.Add(comments[i])
	}
	return CommentSucceed, nil
}

// ParseCommentsWithPath is like ParseComments but uses defaultPath as a fallback
// when individual comment objects omit the path field.
//
// repairedChars is non-zero when `comments` arrived as a serialized string that
// only parsed after the deterministic repair (see comment_args_repair.go). The
// comments themselves are unaffected; callers report it so a schema violation
// the framework papers over still leaves a trace.
func ParseCommentsWithPath(args map[string]any, defaultPath string) (comments []model.LlmComment, repairedChars int, errMsg string) {
	return parseCommentsInner(args, defaultPath)
}

// ParseComments extracts LlmComment entries from tool call arguments without writing
// to the Collector. Returns parsed comments and an error message (empty on success).
func ParseComments(args map[string]any) ([]model.LlmComment, string) {
	comments, _, errMsg := parseCommentsInner(args, "")
	return comments, errMsg
}

func parseCommentsInner(args map[string]any, defaultPath string) ([]model.LlmComment, int, string) {
	var rawComments []any
	repairedChars := 0
	if arr, ok := args["comments"].([]any); ok && len(arr) > 0 {
		rawComments = arr
	} else if s, ok := args["comments"].(string); ok && s != "" {
		if err := json.Unmarshal([]byte(s), &rawComments); err != nil {
			// The schema declares an array; a string is a violation that in
			// practice fails to parse over an unescaped prose quote, taking the
			// whole batch with it. Try the deterministic repair before giving
			// up, and keep the original error when it does not hold up.
			entries, escaped := parseRepairedComments(s)
			if entries == nil {
				// Keep the wording — and specifically "invalid character" — as
				// the parser produced it. That phrasing is what leads the model
				// to regenerate the batch; describing the violation instead
				// makes it resend the same broken string.
				return nil, 0, fmt.Sprintf("Error: failed to parse 'comments' JSON string: %v", err)
			}
			rawComments = entries
			repairedChars = escaped
		}
	}
	if len(rawComments) == 0 {
		raw, _ := json.Marshal(args)
		return nil, 0, fmt.Sprintf("Error: 'comments' array is required. Got args: %s", string(raw))
	}

	var comments []model.LlmComment
	for _, raw := range rawComments {
		obj, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		cm := model.LlmComment{}

		if content, ok := obj["content"].(string); ok {
			cm.Content = content
		}
		if suggestion, ok := obj["suggestion_code"].(string); ok {
			cm.SuggestionCode = suggestion
		}
		if existing, ok := obj["existing_code"].(string); ok {
			cm.ExistingCode = existing
		}
		if thinking, ok := obj["thinking"].(string); ok {
			cm.Thinking = thinking
		}
		if category, ok := obj["category"].(string); ok {
			cm.Category = normalizeCodeCommentCategory(category)
		}
		if severity, ok := obj["severity"].(string); ok {
			cm.Severity = normalizeCodeCommentSeverity(severity)
		}
		if path, ok := obj["path"].(string); ok && path != "" {
			cm.Path = path
		}
		if cm.Path == "" {
			cm.Path = defaultPath
		}

		if cm.Path == "" || cm.Content == "" {
			continue
		}

		comments = append(comments, cm)
	}
	return comments, repairedChars, ""
}

func normalizeCodeCommentCategory(category string) string {
	normalized := strings.ToLower(category)
	if _, ok := validCodeCommentCategories[normalized]; ok {
		return normalized
	}
	return codeCommentCategoryOther
}

func normalizeCodeCommentSeverity(severity string) string {
	normalized := strings.ToLower(severity)
	if _, ok := validCodeCommentSeverities[normalized]; ok {
		return normalized
	}
	return codeCommentSeverityLow
}

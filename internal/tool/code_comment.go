// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path"
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

// CommentNoneUsableMsg reports a parsed batch with no usable comments.
const CommentNoneUsableMsg = "Error: none of the 'comments' entries could be filed; each entry needs a non-empty 'content' and a 'path' (or the reviewed file's path). Re-send the finding with those fields."

// CodeCommentProvider is registered so Tools.Get("code_comment") succeeds and
// the tool definition can be advertised. It is not a live dispatch path.
// Review, scan, and the agent all run tools through reviewtask.executeToolCall,
// which handles tool.CodeComment and returns before Provider.Execute. MCP
// skips the reserved name, so it cannot call this method either. Only unit
// tests invoke Execute.
//
// Execute parses arguments and adds them to the collector. It does not run
// ResolveComment, cross-file relocation, or the lineHintInFile bounds check.
// parseCommentsInner keeps a model start_line/end_line, and
// diff.ResolveLineNumbers skips existing_code matching when StartLine or
// EndLine is already greater than zero. A future live caller must apply the
// same lineHintInFile treatment and must not let a raw model hint skip that
// match.
type CodeCommentProvider struct {
	Collector *CommentCollector
}

func (p *CodeCommentProvider) Tool() Tool { return CodeComment }

// Execute is not used by production dispatch. See CodeCommentProvider.
func (p *CodeCommentProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	if p.Collector == nil {
		return "Error: comment collector is not configured", nil
	}

	comments, errMsg := ParseComments(args)
	if errMsg != "" {
		return errMsg, nil
	}
	if len(comments) == 0 {
		return CommentNoneUsableMsg, nil
	}

	for i := range comments {
		p.Collector.Add(comments[i])
	}
	return CommentSucceed, nil
}

// ParseCommentsWithPath is like ParseComments but uses defaultPath as a fallback
// when individual comment objects omit the path field.
//
// repair is non-nil when `comments` arrived as a serialized string that only
// parsed after the deterministic repair (see comment_args_repair.go). It reports a
// schema violation, not a change to the findings: a repair is accepted only when
// no recovered value looks cut short.
func ParseCommentsWithPath(args map[string]any, defaultPath string) (comments []model.LlmComment, repair *CommentRepair, errMsg string) {
	return parseCommentsInner(args, defaultPath)
}

// ParseComments extracts LlmComment entries from tool call arguments without writing
// to the Collector. Returns parsed comments and an error message (empty on success).
//
// The repair description is discarded because this path has no warning channel
// today. A future caller that starts carrying real traffic should switch to
// ParseCommentsWithPath rather than inherit silent repairs.
func ParseComments(args map[string]any) ([]model.LlmComment, string) {
	comments, _, errMsg := parseCommentsInner(args, "")
	return comments, errMsg
}

func parseCommentsInner(args map[string]any, defaultPath string) ([]model.LlmComment, *CommentRepair, string) {
	var rawComments []any
	var repair *CommentRepair
	if arr, ok := args["comments"].([]any); ok && len(arr) > 0 {
		rawComments = arr
	} else if s, ok := args["comments"].(string); ok && s != "" {
		if err := json.Unmarshal([]byte(s), &rawComments); err != nil {
			// A string here is a schema violation that in practice fails to
			// parse and takes the whole batch with it. Try the repair first.
			entries, rep := parseRepairedComments(s)
			if entries == nil {
				// Keep "invalid character" as the parser produced it: that
				// phrasing is what leads the model to regenerate the batch,
				// while naming the schema violation instead makes it resend the
				// same broken string.
				return nil, nil, fmt.Sprintf("Error: failed to parse 'comments' JSON string: %v", err)
			}
			rawComments = entries
			repair = rep
		}
	}
	if len(rawComments) == 0 {
		raw, _ := json.Marshal(args)
		return nil, nil, fmt.Sprintf("Error: 'comments' array is required. Got args: %s", string(raw))
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
		if pathVal, ok := obj["path"].(string); ok && pathVal != "" {
			cm.Path = normalizeCommentPath(pathVal)
		}
		if cm.Path == "" {
			cm.Path = normalizeCommentPath(defaultPath)
		}
		// start_line / end_line are not in the advertised schema. Models still
		// send them. Copy the pair through; the review loop may replace it
		// when existing_code matches the file, and drops it when it does not
		// name a real line. Leaving the pair at zero here is what made every
		// scan finding look unanchored once text matching failed.
		if start, ok := commentLineArg(obj["start_line"]); ok {
			cm.StartLine = start
		}
		if end, ok := commentLineArg(obj["end_line"]); ok {
			cm.EndLine = end
		}
		if cm.StartLine > 0 && cm.EndLine <= 0 {
			cm.EndLine = cm.StartLine
		}

		if cm.Path == "" || cm.Content == "" {
			continue
		}

		comments = append(comments, cm)
	}
	return comments, repair, ""
}

// commentLineArg reads a 1-based line number from a tool-argument value.
// JSON objects decode numbers as float64. A non-integer or a value below 1
// is not a line, so the caller leaves the field unset.
func commentLineArg(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if n < 1 || n != math.Trunc(n) || n > float64(math.MaxInt) {
			return 0, false
		}
		return int(n), true
	case int:
		if n < 1 {
			return 0, false
		}
		return n, true
	case int64:
		if n < 1 || n > math.MaxInt {
			return 0, false
		}
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil || i < 1 || i > math.MaxInt {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

func normalizeCommentPath(p string) string {
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	p = path.Clean(p)
	if p == "." {
		return ""
	}
	return strings.TrimPrefix(p, "/")
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

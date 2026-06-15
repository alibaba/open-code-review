package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/open-code-review/open-code-review/internal/model"
)

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

// ParseComments extracts LlmComment entries from tool call arguments without writing
// to the Collector. Returns parsed comments and an error message (empty on success).
func ParseComments(args map[string]any) ([]model.LlmComment, string) {
	var rawComments []any
	if arr, ok := args["comments"].([]any); ok && len(arr) > 0 {
		rawComments = arr
	} else if s, ok := args["comments"].(string); ok && s != "" {
		if err := json.Unmarshal([]byte(s), &rawComments); err != nil {
			return nil, fmt.Sprintf("Error: failed to parse 'comments' JSON string: %v", err)
		}
	}
	if len(rawComments) == 0 {
		if content, ok := args["content"].(string); ok && content != "" {
			cm := model.LlmComment{Content: content}
			if path, ok := args["path"].(string); ok {
				cm.Path = path
			}
			if start, ok := toInt(args["start_line"]); ok {
				cm.StartLine = start
				cm.EndLine = start
			}
			if end, ok := toInt(args["end_line"]); ok {
				if cm.StartLine > 0 {
					cm.EndLine = end
				} else {
					cm.EndLine = end
					cm.StartLine = end
				}
			}
			if existing, ok := args["existing_code"].(string); ok {
				cm.ExistingCode = existing
			}
			if suggestion, ok := args["suggestion_code"].(string); ok {
				cm.SuggestionCode = suggestion
			}
			if thinking, ok := args["thinking"].(string); ok {
				cm.Thinking = thinking
			}
			normalizeCommentLines(&cm)
			if cm.Content != "" && cm.Path != "" {
				return []model.LlmComment{cm}, ""
			}
			if content != "" && cm.Path == "" {
				return nil, "Error: flat code_comment requires path"
			}
		}
		raw, _ := json.Marshal(args)
		return nil, fmt.Sprintf("Error: 'comments' array is required. Got args: %s", string(raw))
	}

	topPath, _ := args["path"].(string)
	topStart, hasTopStart := toInt(args["start_line"])
	topEnd, hasTopEnd := toInt(args["end_line"])
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
		if path, ok := obj["path"].(string); ok && path != "" {
			cm.Path = path
		} else if topPath != "" {
			cm.Path = topPath
		}
		if start, ok := toInt(obj["start_line"]); ok {
			cm.StartLine = start
			if cm.EndLine == 0 {
				cm.EndLine = start
			}
		} else if hasTopStart {
			cm.StartLine = topStart
			cm.EndLine = topStart
		}
		if end, ok := toInt(obj["end_line"]); ok {
			if cm.StartLine > 0 {
				cm.EndLine = end
			} else {
				cm.StartLine = end
				cm.EndLine = end
			}
		} else if hasTopEnd && cm.StartLine > 0 {
			cm.EndLine = topEnd
		}

		normalizeCommentLines(&cm)

		if cm.Path == "" || cm.Content == "" {
			continue
		}

		comments = append(comments, cm)
	}
	if len(rawComments) > 0 && len(comments) == 0 {
		return nil, "Error: no valid comments parsed from comments[] array"
	}
	return comments, ""
}

func normalizeCommentLines(cm *model.LlmComment) {
	if cm.StartLine > 0 && cm.EndLine > 0 && cm.EndLine < cm.StartLine {
		cm.EndLine = cm.StartLine
	}
}

func toInt(v any) (int, bool) {
	var n int
	var ok bool
	switch t := v.(type) {
	case int:
		n, ok = t, true
	case int64:
		n, ok = int(t), true
	case float64:
		if t <= 0 || t != math.Trunc(t) || t > float64(math.MaxInt) {
			return 0, false
		}
		n, ok = int(t), true
	case json.Number:
		i, err := t.Int64()
		n, ok = int(i), err == nil
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(t))
		n, ok = i, err == nil
	default:
		return 0, false
	}
	if !ok || n <= 0 {
		return 0, false
	}
	return n, true
}

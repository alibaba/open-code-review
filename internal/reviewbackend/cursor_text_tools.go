package reviewbackend

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// normalizeCodeCommentArgs adapts flat or batch code_comment payloads for ParseComments.
func normalizeCodeCommentArgs(args map[string]any, defaultPath string) map[string]any {
	args = cloneMap(args)
	if comments, ok := args["comments"].([]any); ok && len(comments) > 0 {
		scopeCommentPaths(args, defaultPath)
		return args
	}

	path, _ := args["path"].(string)
	if path == "" {
		path = defaultPath
	}
	content, _ := args["content"].(string)
	if content == "" {
		return args
	}

	comment := map[string]any{"content": content}
	if existing, ok := args["existing_code"].(string); ok && existing != "" {
		comment["existing_code"] = existing
	} else if startLine, ok := args["start_line"]; ok {
		if _, hasLine := intFromAny(startLine); hasLine {
			comment["existing_code"] = lineAnchor(startLine)
		}
	}
	if suggestion, ok := args["suggestion_code"].(string); ok && suggestion != "" {
		comment["suggestion_code"] = suggestion
	}
	if start, ok := intFromAny(args["start_line"]); ok {
		comment["start_line"] = start
		comment["end_line"] = start
	}
	if end, ok := intFromAny(args["end_line"]); ok {
		if start, hasStart := intFromAny(args["start_line"]); hasStart {
			comment["start_line"] = start
		}
		comment["end_line"] = end
	}

	return map[string]any{
		"path":     path,
		"comments": []any{comment},
	}
}

func scopeCommentPaths(args map[string]any, defaultPath string) {
	if defaultPath == "" {
		return
	}
	if p, ok := args["path"].(string); !ok || p == "" {
		args["path"] = defaultPath
	}
	comments, ok := args["comments"].([]any)
	if !ok {
		return
	}
	cloned := make([]any, len(comments))
	for i, raw := range comments {
		item, ok := raw.(map[string]any)
		if !ok {
			cloned[i] = raw
			continue
		}
		copyItem := cloneMap(item)
		if p, ok := copyItem["path"].(string); !ok || p == "" {
			copyItem["path"] = defaultPath
		}
		cloned[i] = copyItem
	}
	args["comments"] = cloned
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func lineAnchor(line any) string {
	if n, ok := intFromAny(line); ok {
		return "+// line " + strconv.Itoa(n)
	}
	return "+// review anchor"
}

func intFromAny(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		if t <= 0 {
			return 0, false
		}
		return t, true
	case int64:
		if t <= 0 {
			return 0, false
		}
		return int(t), true
	case float64:
		if t <= 0 || t != math.Trunc(t) || t > float64(math.MaxInt) {
			return 0, false
		}
		return int(t), true
	case json.Number:
		i, err := t.Int64()
		if err != nil || i <= 0 {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil || i <= 0 {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

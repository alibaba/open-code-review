package reviewbackend

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const maxCursorTextReplayCalls = 20

var cursorReplayAllowedTools = map[string]bool{
	"code_comment": true,
	"task_done":    true,
}

func replayCursorTextToolCalls(ctx context.Context, text, filePath string, exec ToolExecutor, logf func(string, ...any), skipMCPCodeComment bool) {
	objects := extractJSONObjectStrings(text)
	codeCommentCalls := 0
	var deferredDone *ToolCallInput

	for i, obj := range objects {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(obj), &payload); err != nil {
			logf("[ocr] cursor text replay: skip invalid JSON: %v\n", err)
			continue
		}
		name := toolNameFromPayload(payload)
		if name == "" || !cursorReplayAllowedTools[name] {
			continue
		}

		args := toolArgsFromPayload(payload)
		if name == "code_comment" {
			if skipMCPCodeComment {
				continue
			}
			if codeCommentCalls >= maxCursorTextReplayCalls {
				logf("[ocr] cursor text replay: skipping code_comment beyond limit %d\n", maxCursorTextReplayCalls)
				continue
			}
			args = normalizeCodeCommentArgs(args, filePath)
			if _, ok := args["comments"].([]any); !ok {
				continue
			}
			codeCommentCalls++
		}

		raw, err := json.Marshal(args)
		if err != nil {
			logf("[ocr] cursor text replay: marshal %s args: %v\n", name, err)
			continue
		}

		call := ToolCallInput{
			ID:        fmt.Sprintf("cursor-text-replay-%d", i),
			Name:      name,
			Arguments: string(raw),
		}
		if name == "task_done" {
			deferred := call
			deferredDone = &deferred
			continue
		}

		exec(ctx, call)
	}

	if deferredDone != nil {
		out := exec(ctx, *deferredDone)
		if out.Completed {
			return
		}
	}
}

func toolNameFromPayload(payload map[string]any) string {
	if name, ok := payload["tool"].(string); ok && name != "" {
		return normalizeCursorToolName(name)
	}
	if name, ok := payload["name"].(string); ok && name != "" {
		return normalizeCursorToolName(name)
	}
	return ""
}

func toolArgsFromPayload(payload map[string]any) map[string]any {
	if args, ok := payload["arguments"].(map[string]any); ok {
		return args
	}
	if raw, ok := payload["arguments"].(string); ok && raw != "" {
		var args map[string]any
		if err := json.Unmarshal([]byte(raw), &args); err == nil {
			return args
		}
	}
	out := cloneMap(payload)
	delete(out, "tool")
	delete(out, "name")
	delete(out, "state")
	return out
}

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
		comment["existing_code"] = lineAnchor(startLine)
	} else {
		comment["existing_code"] = content
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

	out := map[string]any{
		"path":     path,
		"comments": []any{comment},
	}
	return out
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
		if t <= 0 {
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

func extractJSONObjectStrings(text string) []string {
	var out []string
	depth := 0
	start := -1
	inString := false
	escaped := false
	for i, ch := range text {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				out = append(out, text[start:i+1])
				start = -1
			}
		}
	}
	return out
}

package reviewbackend

import (
	"context"
	"strings"
	"testing"
)

func TestExtractJSONObjectStrings(t *testing.T) {
	text := `prefix {"tool":"code_comment","arguments":{"content":"issue"}} suffix {"name":"task_done","arguments":{"state":"DONE"}}`
	objs := extractJSONObjectStrings(text)
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objs))
	}
	if !strings.Contains(objs[0], "code_comment") {
		t.Fatalf("unexpected first object: %s", objs[0])
	}
}

func TestNormalizeCodeCommentArgs_FlatFormat(t *testing.T) {
	args := normalizeCodeCommentArgs(map[string]any{
		"path":       "foo.go",
		"start_line": float64(12),
		"content":    "SQL injection risk",
	}, "default.go")

	if args["path"] != "foo.go" {
		t.Fatalf("path = %v", args["path"])
	}
	comments, ok := args["comments"].([]any)
	if !ok || len(comments) != 1 {
		t.Fatalf("comments = %T %#v", args["comments"], args["comments"])
	}
	item := comments[0].(map[string]any)
	if item["content"] != "SQL injection risk" {
		t.Fatalf("content = %v", item["content"])
	}
}

func TestReplayCursorTextToolCalls(t *testing.T) {
	var calls []string
	text := `{"tool":"code_comment","arguments":{"path":"foo.go","start_line":1,"content":"bad"}}`
	tracker := &cursorReviewTracker{}
	exec := func(_ context.Context, call ToolCallInput) ToolCallOutput {
		calls = append(calls, call.Name)
		tracker.markTool(call.Name)
		if call.Name == "task_done" {
			return ToolCallOutput{Completed: true}
		}
		return ToolCallOutput{Result: "ok"}
	}
	replayCursorTextToolCalls(context.Background(), text, "foo.go", exec, func(string, ...any) {}, false)

	if len(calls) != 1 || calls[0] != "code_comment" {
		t.Fatalf("calls = %v", calls)
	}
	if tracker.commentCalls != 1 {
		t.Fatalf("commentCalls = %d", tracker.commentCalls)
	}
}

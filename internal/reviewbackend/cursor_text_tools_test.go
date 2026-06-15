package reviewbackend

import "testing"

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
	if item["start_line"] != 12 {
		t.Fatalf("start_line = %v", item["start_line"])
	}
}

func TestIntFromAny_RejectsFractionalFloat(t *testing.T) {
	if _, ok := intFromAny(12.5); ok {
		t.Fatal("expected fractional float to be rejected")
	}
}

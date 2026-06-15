package tool

import "testing"

func TestParseComments_FlatTopLevelFormat(t *testing.T) {
	comments, errMsg := ParseComments(map[string]any{
		"path":       "internal/reviewbackend/review_probe.go",
		"start_line": float64(12),
		"end_line":   float64(18),
		"content":    "Hardcoded API key fallback",
	})
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	cm := comments[0]
	if cm.Path != "internal/reviewbackend/review_probe.go" {
		t.Fatalf("path = %q", cm.Path)
	}
	if cm.StartLine != 12 || cm.EndLine != 18 {
		t.Fatalf("lines = %d-%d", cm.StartLine, cm.EndLine)
	}
	if cm.Content != "Hardcoded API key fallback" {
		t.Fatalf("content = %q", cm.Content)
	}
}

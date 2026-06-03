package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
)

// TestParseComments_CategorySeverityParsed verifies that the optional structured
// category and severity fields are read off each comment object when present.
func TestParseComments_CategorySeverityParsed(t *testing.T) {
	args := map[string]any{
		"path": "main.go",
		"comments": []any{
			map[string]any{
				"content":       "Potential nil pointer dereference.",
				"existing_code": "x := *p",
				"category":      "bug",
				"severity":      "high",
			},
		},
	}

	comments, errMsg := ParseComments(args)
	if errMsg != "" {
		t.Fatalf("unexpected error message: %s", errMsg)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if got := comments[0].Category; got != "bug" {
		t.Errorf("category = %q, want %q", got, "bug")
	}
	if got := comments[0].Severity; got != "high" {
		t.Errorf("severity = %q, want %q", got, "high")
	}
}

// TestParseComments_CategorySeverityOptional verifies that a finding without the
// new fields is still accepted and leaves them zero-valued (backward compatible).
func TestParseComments_CategorySeverityOptional(t *testing.T) {
	args := map[string]any{
		"path": "main.go",
		"comments": []any{
			map[string]any{
				"content":       "Consider renaming for clarity.",
				"existing_code": "a := 1",
			},
		},
	}

	comments, errMsg := ParseComments(args)
	if errMsg != "" {
		t.Fatalf("unexpected error message: %s", errMsg)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Category != "" {
		t.Errorf("category = %q, want empty", comments[0].Category)
	}
	if comments[0].Severity != "" {
		t.Errorf("severity = %q, want empty", comments[0].Severity)
	}
}

// TestLlmComment_JSONOmitsEmptyCategorySeverity verifies that omitempty drops the
// keys entirely when unset, so existing JSON consumers are unaffected.
func TestLlmComment_JSONOmitsEmptyCategorySeverity(t *testing.T) {
	cm := model.LlmComment{
		Path:    "main.go",
		Content: "no metadata",
	}
	b, err := json.Marshal(cm)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(b)
	if strings.Contains(out, "category") {
		t.Errorf("expected no category key, got %s", out)
	}
	if strings.Contains(out, "severity") {
		t.Errorf("expected no severity key, got %s", out)
	}
}

// TestLlmComment_JSONSerializesCategorySeverity verifies the fields serialize when set.
func TestLlmComment_JSONSerializesCategorySeverity(t *testing.T) {
	cm := model.LlmComment{
		Path:     "main.go",
		Content:  "sql injection",
		Category: "security",
		Severity: "critical",
	}
	b, err := json.Marshal(cm)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, `"category":"security"`) {
		t.Errorf("expected category in output, got %s", out)
	}
	if !strings.Contains(out, `"severity":"critical"`) {
		t.Errorf("expected severity in output, got %s", out)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestParseComments(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]any
		wantCount int
		wantErr   bool
	}{
		{
			name: "valid comments array with per-item path",
			args: map[string]any{
				"comments": []any{
					map[string]any{"content": "issue 1", "existing_code": "old", "path": "main.go"},
					map[string]any{"content": "issue 2", "suggestion_code": "new", "path": "main.go"},
				},
			},
			wantCount: 2,
		},
		{
			name: "comments as JSON string",
			args: map[string]any{
				"comments": `[{"content":"from string","path":"main.go"}]`,
			},
			wantCount: 1,
		},
		{
			name: "missing path skips comment",
			args: map[string]any{
				"comments": []any{
					map[string]any{"content": "no path"},
				},
			},
			wantCount: 0,
		},
		{
			name: "missing content skips comment",
			args: map[string]any{
				"comments": []any{
					map[string]any{"existing_code": "has no content", "path": "file.go"},
				},
			},
			wantCount: 0,
		},
		{
			name:    "empty comments array returns error",
			args:    map[string]any{"comments": []any{}},
			wantErr: true,
		},
		{
			name:    "no comments key returns error",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "invalid JSON string returns error",
			args:    map[string]any{"comments": "not json"},
			wantErr: true,
		},
		{
			name: "thinking field preserved",
			args: map[string]any{
				"comments": []any{
					map[string]any{"content": "c", "thinking": "my reasoning", "path": "a.go"},
				},
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comments, errMsg := ParseComments(tt.args)
			if tt.wantErr {
				if errMsg == "" {
					t.Error("expected error message, got empty")
				}
				return
			}
			if errMsg != "" {
				t.Fatalf("unexpected error: %s", errMsg)
			}
			if len(comments) != tt.wantCount {
				t.Errorf("len(comments) = %d, want %d", len(comments), tt.wantCount)
			}
		})
	}
}

func TestParseComments_Fields(t *testing.T) {
	args := map[string]any{
		"comments": []any{
			map[string]any{
				"content":         "fix null check",
				"existing_code":   "if (x == null)",
				"suggestion_code": "if (x === null)",
				"thinking":        "strict equality is safer",
				"path":            "src/app.ts",
			},
		},
	}
	comments, errMsg := ParseComments(args)
	if errMsg != "" {
		t.Fatal(errMsg)
	}
	if len(comments) != 1 {
		t.Fatal("expected 1 comment")
	}
	c := comments[0]
	if c.Path != "src/app.ts" {
		t.Errorf("Path = %q", c.Path)
	}
	if c.Content != "fix null check" {
		t.Errorf("Content = %q", c.Content)
	}
	if c.ExistingCode != "if (x == null)" {
		t.Errorf("ExistingCode = %q", c.ExistingCode)
	}
	if c.SuggestionCode != "if (x === null)" {
		t.Errorf("SuggestionCode = %q", c.SuggestionCode)
	}
	if c.Thinking != "strict equality is safer" {
		t.Errorf("Thinking = %q", c.Thinking)
	}
}

// TestParseComments_CategorySeverity verifies the structured category and severity
// fields are read off each comment object when present, and left zero-valued when
// absent (older/less-capable models that omit them still produce valid comments).
func TestParseComments_CategorySeverity(t *testing.T) {
	t.Run("parsed when present", func(t *testing.T) {
		args := map[string]any{
			"comments": []any{
				map[string]any{
					"content":       "Potential nil pointer dereference.",
					"existing_code": "x := *p",
					"category":      "bug",
					"severity":      "high",
					"path":          "main.go",
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
	})

	t.Run("zero-valued when absent", func(t *testing.T) {
		args := map[string]any{
			"comments": []any{
				map[string]any{
					"content":       "Consider renaming for clarity.",
					"existing_code": "a := 1",
					"path":          "main.go",
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
	})

	t.Run("normalizes casing", func(t *testing.T) {
		args := map[string]any{
			"comments": []any{
				map[string]any{
					"content":       "Potential nil pointer dereference.",
					"existing_code": "x := *p",
					"category":      "Security",
					"severity":      "Critical",
					"path":          "main.go",
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
		if got := comments[0].Category; got != "security" {
			t.Errorf("category = %q, want %q", got, "security")
		}
		if got := comments[0].Severity; got != "critical" {
			t.Errorf("severity = %q, want %q", got, "critical")
		}
	})
}

func TestParseComments_CategorySeveritySchemaDrift(t *testing.T) {
	args := map[string]any{
		"comments": []any{
			map[string]any{
				"content":       "Use the canonical metadata fallback.",
				"existing_code": "value := compute()",
				"category":      "correctness",
				"severity":      "info",
				"path":          "main.go",
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
	if got := comments[0].Category; got != "other" {
		t.Errorf("category = %q, want %q", got, "other")
	}
	if got := comments[0].Severity; got != "low" {
		t.Errorf("severity = %q, want %q", got, "low")
	}
	if got := comments[0].Content; got != "Use the canonical metadata fallback." {
		t.Errorf("content = %q", got)
	}
}

// TestLlmComment_JSONCategorySeverity verifies category and severity serialize as
// flat siblings alongside content when set, and are omitted entirely when empty so
// existing JSON consumers are unaffected.
func TestLlmComment_JSONCategorySeverity(t *testing.T) {
	t.Run("omitted when empty", func(t *testing.T) {
		b, err := json.Marshal(model.LlmComment{Path: "main.go", Content: "no metadata"})
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
	})

	t.Run("serialized when set", func(t *testing.T) {
		b, err := json.Marshal(model.LlmComment{
			Path:     "main.go",
			Content:  "sql injection",
			Category: "security",
			Severity: "critical",
		})
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
	})

	t.Run("posting provenance remains internal", func(t *testing.T) {
		b, err := json.Marshal(model.LlmComment{
			Path:    "main.go",
			Content: "finding",
			Side:    model.CommentSideRight,
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(b), `"side"`) {
			t.Fatalf("legacy comment JSON leaked posting provenance: %s", b)
		}
	})
}

func TestCodeCommentProvider_Execute(t *testing.T) {
	t.Run("adds comments to collector", func(t *testing.T) {
		collector := NewCommentCollector()
		p := &CodeCommentProvider{Collector: collector}
		result, err := p.Execute(context.Background(), map[string]any{
			"comments": []any{
				map[string]any{"content": "issue 1", "path": "main.go"},
				map[string]any{"content": "issue 2", "path": "main.go"},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if result != CommentSucceed {
			t.Errorf("result = %q, want %q", result, CommentSucceed)
		}
		if len(collector.Comments()) != 2 {
			t.Errorf("collector has %d comments, want 2", len(collector.Comments()))
		}
	})

	t.Run("nil collector returns error message", func(t *testing.T) {
		p := &CodeCommentProvider{Collector: nil}
		result, err := p.Execute(context.Background(), map[string]any{
			"comments": []any{map[string]any{"content": "x", "path": "main.go"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if result == CommentSucceed {
			t.Error("expected error message for nil collector")
		}
	})

	t.Run("invalid args returns error message", func(t *testing.T) {
		collector := NewCommentCollector()
		p := &CodeCommentProvider{Collector: collector}
		result, err := p.Execute(context.Background(), map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		if result == CommentSucceed {
			t.Error("expected error message for empty args")
		}
	})

	t.Run("tool type is CodeComment", func(t *testing.T) {
		p := &CodeCommentProvider{}
		if p.Tool() != CodeComment {
			t.Errorf("Tool() = %v, want CodeComment", p.Tool())
		}
	})
}

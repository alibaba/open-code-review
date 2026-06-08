package main

import (
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
)

func TestFilterBySeverity(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "a.go", Content: "critical bug", Severity: model.SeverityCritical},
		{Path: "b.go", Content: "warning issue", Severity: model.SeverityWarning},
		{Path: "c.go", Content: "suggestion", Severity: model.SeveritySuggestion},
		{Path: "d.go", Content: "nitpick", Severity: model.SeverityNitpick},
		{Path: "e.go", Content: "no severity"},
	}

	tests := []struct {
		minSeverity string
		wantCount   int
		wantPaths   []string
	}{
		{"", 5, []string{"a.go", "b.go", "c.go", "d.go", "e.go"}},
		{"nitpick", 5, []string{"a.go", "b.go", "c.go", "d.go", "e.go"}},
		{"suggestion", 4, []string{"a.go", "b.go", "c.go", "e.go"}},
		{"warning", 3, []string{"a.go", "b.go", "e.go"}},
		{"critical", 2, []string{"a.go", "e.go"}},
	}

	for _, tt := range tests {
		t.Run("min="+tt.minSeverity, func(t *testing.T) {
			got := filterBySeverity(comments, tt.minSeverity)
			if len(got) != tt.wantCount {
				t.Fatalf("got %d comments, want %d", len(got), tt.wantCount)
			}
			for i, want := range tt.wantPaths {
				if got[i].Path != want {
					t.Errorf("got[%d].Path = %q, want %q", i, got[i].Path, want)
				}
			}
		})
	}
}

func TestFilterBySeverity_Empty(t *testing.T) {
	got := filterBySeverity(nil, "warning")
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

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
		// No filter: all comments returned.
		{"", 5, []string{"a.go", "b.go", "c.go", "d.go", "e.go"}},
		// nitpick: keeps all with severity, filters out unset (rank 0 < 1).
		{"nitpick", 4, []string{"a.go", "b.go", "c.go", "d.go"}},
		// suggestion: keeps critical + warning + suggestion.
		{"suggestion", 3, []string{"a.go", "b.go", "c.go"}},
		// warning: keeps critical + warning.
		{"warning", 2, []string{"a.go", "b.go"}},
		// critical: keeps only critical.
		{"critical", 1, []string{"a.go"}},
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
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestFilterBySeverity_ReturnsNonNilSlice(t *testing.T) {
	// When minSeverity is set, result should always be non-nil (for JSON: [] not null).
	comments := []model.LlmComment{
		{Path: "a.go", Content: "nitpick", Severity: model.SeverityNitpick},
	}
	got := filterBySeverity(comments, "critical")
	if got == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected 0 comments, got %d", len(got))
	}
}

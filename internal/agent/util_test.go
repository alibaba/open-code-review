// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/session"
)

func TestStripEmptyPlanBlock(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "review plan wrapper is removed",
			input: "header\n### Review Plan\n{{plan_guidance}}\n\ntail",
			want:  "header\ntail",
		},
		{
			name:  "english template wrapper without trailing blank line is removed",
			input: "header\n### Review Plan (Optional)\n{{plan_guidance}}\ntail",
			want:  "header\ntail",
		},
		{
			name:  "no wrapper present is a no-op",
			input: "no plan block here\njust text",
			want:  "no plan block here\njust text",
		},
		{
			name:  "multiple wrappers all removed",
			input: "### Review Plan (Optional)\n{{plan_guidance}}\n\nmiddle\n### Review Plan\n{{plan_guidance}}\n\nend",
			want:  "middle\nend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripEmptyPlanBlock(tt.input)
			if got != tt.want {
				t.Errorf("stripEmptyPlanBlock() = %q, want %q", got, tt.want)
			}
			if strings.Contains(got, "{{plan_guidance}}") {
				t.Errorf("stripEmptyPlanBlock() left literal {{plan_guidance}} in output: %q", got)
			}
		})
	}
}

func TestStripEmptyPlanBlock_IntegrationWithReplaceAll(t *testing.T) {
	template := "header\n### Review Plan\n{{plan_guidance}}\n\ntail"

	stripped := stripEmptyPlanBlock(template)
	final := strings.ReplaceAll(stripped, "{{plan_guidance}}", "")

	want := "header\ntail"
	if final != want {
		t.Errorf("stripEmptyPlanBlock integration:\n  got:  %q\n  want: %q", final, want)
	}
	if strings.Contains(final, "{{plan_guidance}}") {
		t.Errorf("literal {{plan_guidance}} leaked: %q", final)
	}
	if strings.Contains(final, "Review Plan") {
		t.Errorf("dangling Review Plan header retained: %q", final)
	}
}

func TestReviewModeString(t *testing.T) {
	tests := []struct {
		from, to, commit string
		want             string
	}{
		{"", "", "abc123", session.ReviewModeCommit},
		{"main", "feature", "", session.ReviewModeRange},
		{"", "", "", session.ReviewModeWorkspace},
		{"main", "feature", "abc123", session.ReviewModeCommit},
	}

	for _, tt := range tests {
		got := reviewModeString(tt.from, tt.to, tt.commit)
		if got != tt.want {
			t.Errorf("reviewModeString(%q, %q, %q) = %q, want %q", tt.from, tt.to, tt.commit, got, tt.want)
		}
	}
}

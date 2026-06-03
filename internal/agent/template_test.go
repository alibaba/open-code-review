package agent

import (
	"strings"
	"testing"
)

func TestStripEmptyPlanBlock(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "english template wrapper is removed",
			input: "header\n### Review Plan (Optional)\n{{plan_guidance}}\n\ntail",
			want:  "header\ntail",
		},
		{
			name:  "english template wrapper without trailing blank line is removed",
			input: "header\n### Review Plan (Optional)\n{{plan_guidance}}\ntail",
			want:  "header\ntail",
		},
		{
			name:  "chinese template wrapper is removed",
			input: "header\n### 审查计划\n{{plan_guidance}}\n\ntail",
			want:  "header\ntail",
		},
		{
			name:  "chinese optional wrapper is removed",
			input: "header\n### 审查计划（可选）\n{{plan_guidance}}\n\ntail",
			want:  "header\ntail",
		},
		{
			name:  "no wrapper present is a no-op",
			input: "no plan block here\njust text",
			want:  "no plan block here\njust text",
		},
		{
			name:  "multiple wrappers all removed",
			input: "### Review Plan (Optional)\n{{plan_guidance}}\n\nmiddle\n### 审查计划\n{{plan_guidance}}\n\nend",
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
	// Regression: validate the executeSubtask call-site flow — the wrapper
	// passes through stripEmptyPlanBlock first (token still present), then
	// through ReplaceAll. If strip runs after the replace, the token is
	// already gone and the wrapper header leaks into the prompt.
	template := "header\n### Review Plan (Optional)\n{{plan_guidance}}\n\ntail"

	// Phase 1: strip (token must be present for regex to match)
	stripped := stripEmptyPlanBlock(template)

	// Phase 2: replace (no-op since token should already be consumed)
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

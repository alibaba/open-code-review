package delegate

import (
	"strings"
	"testing"
)

func TestRuleGroupsMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		groups   []RuleGroup
		contains []string
	}{
		{
			name:     "empty groups",
			groups:   nil,
			contains: nil,
		},
		{
			name: "single group",
			groups: []RuleGroup{
				{ID: 1, Source: "project", Pattern: "*.go", Text: "Review Go code", Files: []string{"a.go", "b.go"}},
			},
			contains: []string{
				"### Rule Group 1: project / *.go",
				"- a.go",
				"- b.go",
				"#### Content",
				"Review Go code",
			},
		},
		{
			name: "multiple groups separated by hr",
			groups: []RuleGroup{
				{ID: 1, Source: "system", Pattern: "default", Text: "rule A", Files: []string{"x.go"}},
				{ID: 2, Source: "custom", Pattern: "test/**", Text: "rule B", Files: []string{"y_test.go"}},
			},
			contains: []string{
				"### Rule Group 1: system / default",
				"---",
				"### Rule Group 2: custom / test/**",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RuleGroupsMarkdown(tt.groups)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
		})
	}
}

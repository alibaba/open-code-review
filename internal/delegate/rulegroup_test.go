package delegate

import (
	"testing"

	"github.com/open-code-review/open-code-review/internal/config/rules"
)

// stubResolver implements rules.Resolver only.
type stubResolver struct {
	ruleText string
}

func (s *stubResolver) Resolve(_ string) string { return s.ruleText }

// stubDetailResolver implements both rules.Resolver and rules.DetailResolver.
type stubDetailResolver struct {
	mapping map[string]rules.RuleDetail
}

func (s *stubDetailResolver) Resolve(path string) string {
	return s.mapping[path].Rule
}

func (s *stubDetailResolver) ResolveDetail(path string) rules.RuleDetail {
	if d, ok := s.mapping[path]; ok {
		return d
	}
	return rules.RuleDetail{Rule: "default-rule", Source: "system", Pattern: "default"}
}

func TestGroupRules(t *testing.T) {
	tests := []struct {
		name     string
		resolver rules.Resolver
		paths    []string
		want     []RuleGroup
	}{
		{
			name:     "empty input",
			resolver: &stubResolver{ruleText: "unused"},
			paths:    nil,
			want:     nil,
		},
		{
			name:     "single file with basic resolver",
			resolver: &stubResolver{ruleText: "check for bugs"},
			paths:    []string{"main.go"},
			want: []RuleGroup{
				{ID: 1, Source: "system", Pattern: "default", Text: "check for bugs", Files: []string{"main.go"}},
			},
		},
		{
			name:     "multiple files same rule grouped together",
			resolver: &stubResolver{ruleText: "shared rule"},
			paths:    []string{"a.go", "b.go", "c.go"},
			want: []RuleGroup{
				{ID: 1, Source: "system", Pattern: "default", Text: "shared rule", Files: []string{"a.go", "b.go", "c.go"}},
			},
		},
		{
			name: "detail resolver groups by rule text",
			resolver: &stubDetailResolver{
				mapping: map[string]rules.RuleDetail{
					"internal/api.go":    {Rule: "api rules", Source: "project", Pattern: "internal/**"},
					"internal/server.go": {Rule: "api rules", Source: "project", Pattern: "internal/**"},
					"test/api_test.go":   {Rule: "test rules", Source: "custom", Pattern: "test/**"},
				},
			},
			paths: []string{"internal/api.go", "internal/server.go", "test/api_test.go"},
			want: []RuleGroup{
				{ID: 1, Source: "project", Pattern: "internal/**", Text: "api rules", Files: []string{"internal/api.go", "internal/server.go"}},
				{ID: 2, Source: "custom", Pattern: "test/**", Text: "test rules", Files: []string{"test/api_test.go"}},
			},
		},
		{
			name: "same text different sources stay separate",
			resolver: &stubDetailResolver{
				mapping: map[string]rules.RuleDetail{
					"a.go": {Rule: "same text", Source: "project", Pattern: "*.go"},
					"b.go": {Rule: "same text", Source: "custom", Pattern: "b.*"},
				},
			},
			paths: []string{"a.go", "b.go"},
			want: []RuleGroup{
				{ID: 1, Source: "project", Pattern: "*.go", Text: "same text", Files: []string{"a.go"}},
				{ID: 2, Source: "custom", Pattern: "b.*", Text: "same text", Files: []string{"b.go"}},
			},
		},
		{
			name: "same text same source different patterns stay separate",
			resolver: &stubDetailResolver{
				mapping: map[string]rules.RuleDetail{
					"src/main.go": {Rule: "go rule", Source: "custom", Pattern: "src/**/*.go"},
					"cmd/main.go": {Rule: "go rule", Source: "custom", Pattern: "cmd/**/*.go"},
				},
			},
			paths: []string{"src/main.go", "cmd/main.go"},
			want: []RuleGroup{
				{ID: 1, Source: "custom", Pattern: "src/**/*.go", Text: "go rule", Files: []string{"src/main.go"}},
				{ID: 2, Source: "custom", Pattern: "cmd/**/*.go", Text: "go rule", Files: []string{"cmd/main.go"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GroupRules(tt.resolver, tt.paths)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d groups, want %d", len(got), len(tt.want))
			}
			for i := range got {
				g, w := got[i], tt.want[i]
				if g.ID != w.ID {
					t.Errorf("group[%d].ID = %d, want %d", i, g.ID, w.ID)
				}
				if g.Source != w.Source {
					t.Errorf("group[%d].Source = %q, want %q", i, g.Source, w.Source)
				}
				if g.Pattern != w.Pattern {
					t.Errorf("group[%d].Pattern = %q, want %q", i, g.Pattern, w.Pattern)
				}
				if g.Text != w.Text {
					t.Errorf("group[%d].Text = %q, want %q", i, g.Text, w.Text)
				}
				if len(g.Files) != len(w.Files) {
					t.Errorf("group[%d].Files len = %d, want %d", i, len(g.Files), len(w.Files))
					continue
				}
				for j := range g.Files {
					if g.Files[j] != w.Files[j] {
						t.Errorf("group[%d].Files[%d] = %q, want %q", i, j, g.Files[j], w.Files[j])
					}
				}
			}
		})
	}
}

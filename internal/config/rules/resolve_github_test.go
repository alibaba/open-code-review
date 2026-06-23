package rules

import (
	"strings"
	"testing"
)

func TestResolve_GitHubWorkflows(t *testing.T) {
	r, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path    string
		wantSub string // substring expected in the resolved rule
	}{
		{".github/workflows/ci.yml", "pull_request_target"},
		{".github/workflows/release.yaml", "pull_request_target"},
		{".github/ISSUE_TEMPLATE/bug_report.yml", "Issue Template"},
		{".github/release.yml", "Issue Template"},
		{"config/app.yaml", "spelling errors in yaml-keys"},
		{"k8s/deployment.yml", "spelling errors in yaml-keys"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := r.Resolve(tt.path)
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("Resolve(%q) does not contain %q;\ngot: %s", tt.path, tt.wantSub, got[:min(len(got), 200)])
			}
		})
	}
}

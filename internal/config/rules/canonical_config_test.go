package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// canonicalConfigurer is the optional interface the manifest layer uses to fold a
// resolver's effective rules into rule_config_sha256. Both concrete resolvers must
// satisfy it.
type canonicalConfigurer interface {
	CanonicalConfig() []string
}

func TestSystemRuleCanonicalConfig(t *testing.T) {
	sr := &SystemRule{
		DefaultRule: "d",
		PathRules: []PathRule{
			{Pattern: "*.go", Rule: "go"},
			{Pattern: "*.py", Rule: "py"},
		},
	}
	want := []string{
		"layer", "system", "default", "d",
		"layer", "system", "pattern", "*.go", "rule", "go",
		"layer", "system", "pattern", "*.py", "rule", "py",
	}
	got := sr.CanonicalConfig()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("CanonicalConfig() = %v, want %v", got, want)
	}
}

func TestComposedResolverCanonicalConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ruleJSON := `{"rules":[{"path":"force-api/**/*.java","rule":"project-java-rule"}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	resolver, _, err := NewResolver(dir, "")
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	cc, ok := resolver.(canonicalConfigurer)
	if !ok {
		t.Fatal("composedResolver does not implement CanonicalConfig")
	}

	fields := cc.CanonicalConfig()
	joined := strings.Join(fields, "\x00")

	// Deterministic across calls.
	if strings.Join(cc.CanonicalConfig(), "\x00") != joined {
		t.Fatal("CanonicalConfig is not deterministic")
	}
	// The project layer's entry is present, tagged as "project".
	if !strings.Contains(joined, "project") || !strings.Contains(joined, "project-java-rule") {
		t.Errorf("CanonicalConfig missing project layer entry: %v", fields)
	}
	// The system layer is always appended.
	if !strings.Contains(joined, "system") {
		t.Errorf("CanonicalConfig missing system layer: %v", fields)
	}
}

func TestComposedResolverCanonicalConfig_ProjectRuleChangeChangesOutput(t *testing.T) {
	build := func(rule string) string {
		t.Helper()
		home := t.TempDir()
		t.Setenv("HOME", home)
		dir := t.TempDir()
		ocrDir := filepath.Join(dir, ".opencodereview")
		if err := os.MkdirAll(ocrDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		ruleJSON := `{"rules":[{"path":"a/**","rule":"` + rule + `"}]}`
		if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
			t.Fatalf("write rule.json: %v", err)
		}
		resolver, _, err := NewResolver(dir, "")
		if err != nil {
			t.Fatalf("NewResolver: %v", err)
		}
		return strings.Join(resolver.(canonicalConfigurer).CanonicalConfig(), "\x00")
	}
	if build("rule-one") == build("rule-two") {
		t.Error("changing a project rule did not change CanonicalConfig")
	}
}

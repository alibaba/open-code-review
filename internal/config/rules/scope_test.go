// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProjectRule(t *testing.T, ruleJSON string) string {
	t.Helper()
	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}
	return dir
}

func newProjectResolver(t *testing.T, ruleJSON string) Resolver {
	t.Helper()
	setTestHome(t, t.TempDir())
	resolver, _, err := NewResolver(writeProjectRule(t, ruleJSON), "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return resolver
}

func TestScope_DefaultLeavesRuleUnchanged(t *testing.T) {
	resolver := newProjectResolver(t, `{"rules":[{"path":"**/*.go","rule":"project-go-rule"}]}`)

	if got := resolver.Resolve("main.go"); got != "project-go-rule" {
		t.Fatalf("Resolve = %q, want the bare rule", got)
	}
}

func TestScope_RulesOnlyPrefixesDirective(t *testing.T) {
	resolver := newProjectResolver(t, `{"rules":[{"path":"**/*.go","rule":"project-go-rule","scope":"rules_only"}]}`)

	want := rulesOnlyPreamble + "project-go-rule"
	if got := resolver.Resolve("main.go"); got != want {
		t.Fatalf("Resolve = %q, want %q", got, want)
	}
	detail := resolver.(DetailResolver).ResolveDetail("main.go")
	if detail.Rule != want {
		t.Fatalf("ResolveDetail.Rule = %q, want %q", detail.Rule, want)
	}
	if detail.Source != "project" || detail.Pattern != "**/*.go" {
		t.Fatalf("ResolveDetail source/pattern = %q/%q, want project/**/*.go", detail.Source, detail.Pattern)
	}
}

func TestScope_RulesOnlyCoversMergedSystemRule(t *testing.T) {
	resolver := newProjectResolver(t, `{"rules":[{"path":"**/*.go","rule":"project-go-rule","merge_system_rule":true,"scope":"rules_only"}]}`)
	systemRule, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}

	got := resolver.Resolve("main.go")
	if !strings.HasPrefix(got, rulesOnlyPreamble) {
		t.Fatalf("expected the directive first, got %q", truncate(got, 120))
	}
	if !strings.Contains(got, systemRule.Resolve("main.go")) || !strings.Contains(got, "project-go-rule") {
		t.Fatalf("expected both merged rules after the directive, got %q", truncate(got, 120))
	}
}

func TestScope_AppliesOnlyToMatchingEntry(t *testing.T) {
	resolver := newProjectResolver(t, `{"rules":[
		{"path":"**/*.go","rule":"go-conventions","scope":"rules_only"},
		{"path":"**/*.py","rule":"py-rule"}
	]}`)

	if got := resolver.Resolve("app.py"); got != "py-rule" {
		t.Fatalf("Resolve(app.py) = %q, want the bare rule", got)
	}
	if got := resolver.Resolve("main.go"); !strings.HasPrefix(got, rulesOnlyPreamble) {
		t.Fatalf("Resolve(main.go) = %q, want the directive", truncate(got, 120))
	}
}

func TestScope_UnknownValueFailsInEveryLayer(t *testing.T) {
	const bad = `{"rules":[{"path":"**/*.go","rule":"r","scope":"rule_only"}]}`

	t.Run("project", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		_, _, err := NewResolver(writeProjectRule(t, bad), "", ResolverOptions{})
		if err == nil || !strings.Contains(err.Error(), `unsupported scope "rule_only"`) {
			t.Fatalf("expected unsupported scope error, got %v", err)
		}
	})

	t.Run("custom", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		customPath := filepath.Join(t.TempDir(), "custom.json")
		if err := os.WriteFile(customPath, []byte(bad), 0o644); err != nil {
			t.Fatalf("write custom rule: %v", err)
		}
		_, _, err := NewResolver("", customPath, ResolverOptions{})
		if err == nil || !strings.Contains(err.Error(), `unsupported scope "rule_only"`) {
			t.Fatalf("expected unsupported scope error, got %v", err)
		}
	})

	t.Run("global", func(t *testing.T) {
		home := t.TempDir()
		setTestHome(t, home)
		if err := os.MkdirAll(filepath.Join(home, ".opencodereview"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(home, ".opencodereview", "rule.json"), []byte(bad), 0o644); err != nil {
			t.Fatalf("write global rule: %v", err)
		}
		_, _, err := NewResolver("", "", ResolverOptions{})
		if err == nil || !strings.Contains(err.Error(), `unsupported scope "rule_only"`) {
			t.Fatalf("expected unsupported scope error, got %v", err)
		}
	})
}

func TestScope_CanonicalConfig(t *testing.T) {
	config := func(ruleJSON string) []string {
		t.Helper()
		return newProjectResolver(t, ruleJSON).(canonicalConfigurer).CanonicalConfig()
	}

	plain := config(`{"rules":[{"path":"a/**","rule":"r"}]}`)
	if strings.Contains(strings.Join(plain, "\x00"), "\x00scope\x00") {
		t.Fatalf("an entry without scope must not add a scope field: %v", plain)
	}
	scoped := config(`{"rules":[{"path":"a/**","rule":"r","scope":"rules_only"}]}`)
	if strings.Join(plain, "\x00") == strings.Join(scoped, "\x00") {
		t.Fatal("setting scope did not change CanonicalConfig")
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"

	allowedext "github.com/alibaba/open-code-review/internal/config/allowlist"
	"github.com/alibaba/open-code-review/internal/pathutil"
)

func TestExpandBraces_NoBraces(t *testing.T) {
	got := expandBraces("*.java")
	if len(got) != 1 || got[0] != "*.java" {
		t.Errorf("expected [*.java], got %v", got)
	}
}

func TestExpandBraces_SingleGroup(t *testing.T) {
	got := expandBraces("*.{go,py}")
	want := []string{"*.go", "*.py"}
	if len(got) != len(want) {
		t.Fatalf("expected %d items, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestExpandBraces_MultipleOptions(t *testing.T) {
	got := expandBraces("**/*.{ts,js,tsx,jsx}")
	want := []string{"**/*.ts", "**/*.js", "**/*.tsx", "**/*.jsx"}
	if len(got) != len(want) {
		t.Fatalf("expected %d items, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestExpandBraces_UnclosedBrace(t *testing.T) {
	got := expandBraces("*.{go,py")
	if len(got) != 1 || got[0] != "*.{go,py" {
		t.Errorf("expected original pattern, got %v", got)
	}
}

func TestResolve_DefaultRules(t *testing.T) {
	rule, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}

	tests := []struct {
		path       string
		wantSubstr string // substring that should appear in the matched rule
	}{
		{"src/main/java/com/example/foo.java", "Logic Error Detection"},
		{"foo.java", "Logic Error Detection"},
		{"src/main/resources/templates/email.ftl", "Template Injection"},
		{"foo.ftl", "Template Injection"},
		{"foo.ftlh", "Template Injection"},
		{"foo.ftlx", "Template Injection"},
		{"templates/account.hbs", "Handlebars/Mustache Escaping Boundaries"},
		{"templates/account.HBS", "Handlebars/Mustache Escaping Boundaries"},
		{"templates/email.mustache", "Handlebars/Mustache Escaping Boundaries"},
		{"templates/email.MUSTACHE", "Handlebars/Mustache Escaping Boundaries"},
		{"templates/account.pug", "Pug Escaping and Output Contexts"},
		{"templates/account.PUG", "Pug Escaping and Output Contexts"},
		{"src/main/resources/mapper/usermapper.xml", "SQL Logic Error Detection"},
		{"src/main/resources/dao/userdao.xml", "SQL Logic Error Detection"},
		{"pom.xml", "snapshot"},
		{"submodule/pom.xml", "snapshot"},
		{"src/main/resources/application.properties", "Configuration Error Detection"},
		{"frontend/package.json", "latest"},
		{"composer.json", "Composer Manifest Review Principles"},
		{"packages/library/composer.json", "Dependency Constraints and Resolution"},
		{"config/app.yaml", "yaml-key"},
		{"deploy/values.yml", "yaml-key"},
		{"src/pages/index.astro", "client:*"},
		{"src/components/app.tsx", "React"},
		{"lib/utils.ts", "TypeScript"},
		{"scripts/config.mjs", "TypeScript"},
		{"server/bootstrap.cjs", "TypeScript"},
		{"app.kt", "Null Safety"},
		{"src/main/handler.cpp", "Smart Pointer"},
		{"src/main/handler.cxx", "Smart Pointer"},
		{"include/handler.hxx", "Smart Pointer"},
		{"driver.c", "malloc"},
		{"pages/Index.ets", "State Decorator"},
		{"components/Button.ets", "State Decorator"},
		{"entry/src/main/module.json5", "json-key"},
		{"entry/oh-package.json5", "json-key"},
		{"src/lib.rs", "Ownership and Lifetime Correctness"},
		{"crates/service/src/main.rs", "Unsafe Code Boundaries"},
		{"crates/service/Cargo.toml", "Cargo Manifest Hygiene"},
		{"scripts/deploy.py", "Mutable Default Arguments"},
		{"src/app/main.py", "Mutable Default Arguments"},
		{"notebook.ipynb", "Mutable Default Arguments"},
		{"src/notebooks/data.ipynb", "Mutable Default Arguments"},
		{"public/index.php", "PHP Review Principles"},
		{"templates/account/profile.phtml", "Web and Template Security Boundaries"},
		{"locale/zh_CN/LC_MESSAGES/messages.po", "Placeholder Mismatch"},
		{"i18n/app.po", "Plural Forms"},
		{"locale/messages.pot", "Placeholder Consistency"},
		{"i18n/app.pot", "Header Integrity"},
		{"api/schema.graphql", "Breaking Changes"},
		{"queries/user.gql", "Breaking Changes"},
		{"src/model.jl", "Type Stability"},
		{"MyPkg/src/solver.jl", "Type Stability"},
		{"main.tf", "Hardcoded Secrets"},
		{"modules/network/vpc.hcl", "Overly Permissive Access"},
		{"envs/prod.tfvars", "Hardcoded Secrets"},
		{"infra/main.bicep", "Hardcoded Secrets"},
		{"api/v1/user.proto", "Wire Compatibility"},
		{"service.proto", "Wire Compatibility"},
		{"src/Main.hs", "Partial Functions"},
		{"examples/Tutorial.lhs", "Partial Functions"},
		{"src/parser.nim", "Memory and Lifetime Safety"},
		{"scripts/build.nims", "Memory and Lifetime Safety"},
		{"project.nimble", "Memory and Lifetime Safety"},
		{"Sources/App/ContentView.swift", "Swift Review Principles"},
		{"MyApp/Models/UserStore.swift", "Swift Review Principles"},
		{"ChattyFit/ChattyFit/Views/WorkoutSessionView.swift", "SwiftUI State and Lifecycle"},
		{"src/Main.elm", "Elm Architecture"},
		{"app/Page/Home.elm", "Elm Architecture"},
		{"lib/config.libsonnet", "Late Binding"},
		{"environments/prod/main.jsonnet", "Late Binding"},
		{"jsonnet/kube-prometheus/components/grafana.libsonnet", "Late Binding"},
		{"src/foo.R", "R Code Review Principles"},
		{"analysis/plots.r", "R Code Review Principles"},
		{"src/main.zig", "Illegal Behavior"},
		{"build.zig", "Illegal Behavior"},
		{"idl/service.thrift", "Field IDs and Wire Compatibility"},
		{"if/common.thrift", "Field IDs and Wire Compatibility"},
		{"schema/addressbook.capnp", "Ordinals and Wire Compatibility"},
		{"src/rpc.capnp", "Ordinals and Wire Compatibility"},
		{"rtl/counter.v", "Blocking and Non-Blocking Assignments"},
		{"rtl/alu.sv", "Blocking and Non-Blocking Assignments"},
		{"rtl/defines.vh", "Blocking and Non-Blocking Assignments"},
		{"rtl/fifo.vhd", "numeric_std"},
		{"rtl/fifo.vhdl", "numeric_std"},
		{"Models/main.m", "Indexing, Shapes, and Implicit Expansion"},
		{"ios/ViewController.mm", "ARC and Object Ownership"},
		{"ios/ViewController.MM", "ARC and Object Ownership"},
		{"src/Counter.sol", "Checks-Effects-Interactions"},
		{"contracts/Vault.sol", "Delegatecall and Proxy Upgradeability"},
		{"contracts/token.vy", "Language Restrictions"},
		{"src/amm.vy", "Reentrancy and `@nonreentrant`"},
		{"scripts/deploy.ps1", "PowerShell Review Principles"},
		{"modules/Build/Build.psm1", "PowerShell Review Principles"},
		{"modules/Build/Build.psd1", "PowerShell Review Principles"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := rule.Resolve(tt.path)
			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("Resolve(%q): expected rule containing %q, got %q",
					tt.path, tt.wantSubstr, truncate(got, 80))
			}
		})
	}
}

func TestResolve_FallbackToDefault(t *testing.T) {
	rule, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}

	paths := []string{
		"readme.md",
		"docs/architecture.txt",
		"Makefile",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			got := rule.Resolve(path)
			if got != rule.DefaultRule {
				t.Errorf("Resolve(%q): expected DefaultRule, got %q", path, truncate(got, 80))
			}
		})
	}
}

func TestResolve_CustomRule_FirstMatchWins(t *testing.T) {
	rule := &SystemRule{
		DefaultRule: "default",
		PathRules: []PathRule{
			{Pattern: "**/special.java", Rule: "special-rule"},
			{Pattern: "**/*.java", Rule: "java-rule"},
		},
	}

	// special.java matches both patterns, but "special-rule" is first.
	got := rule.Resolve("src/special.java")
	if got != "special-rule" {
		t.Errorf("expected special-rule, got %q", got)
	}

	// Other java files match the second pattern.
	got = rule.Resolve("src/foo.java")
	if got != "java-rule" {
		t.Errorf("expected java-rule, got %q", got)
	}
}

func TestResolve_CustomRule_DefaultFallback(t *testing.T) {
	rule := &SystemRule{
		DefaultRule: "fallback-rule",
		PathRules: []PathRule{
			{Pattern: "**/*.java", Rule: "java-rule"},
		},
	}

	got := rule.Resolve("main.go")
	if got != "fallback-rule" {
		t.Errorf("expected fallback-rule, got %q", got)
	}
}

func TestResolve_CaseInsensitive(t *testing.T) {
	rule := &SystemRule{
		DefaultRule: "default",
		PathRules: []PathRule{
			{Pattern: "**/*.astro", Rule: "astro-rule"},
			{Pattern: "**/*.java", Rule: "java-rule"},
			{Pattern: "**/Cargo.toml", Rule: "cargo-rule"},
		},
	}

	got := rule.Resolve("Foo.Astro")
	if got != "astro-rule" {
		t.Errorf("expected astro-rule for uppercase extension, got %q", got)
	}

	got = rule.Resolve("foo.astro")
	if got != "astro-rule" {
		t.Errorf("expected astro-rule for lowercase, got %q", got)
	}

	got = rule.Resolve("Foo.Java")
	if got != "java-rule" {
		t.Errorf("expected java-rule for uppercase extension, got %q", got)
	}

	got = rule.Resolve("foo.java")
	if got != "java-rule" {
		t.Errorf("expected java-rule for lowercase, got %q", got)
	}

	got = rule.Resolve("crates/service/Cargo.toml")
	if got != "cargo-rule" {
		t.Errorf("expected cargo-rule for canonical Cargo.toml, got %q", got)
	}

	got = rule.Resolve("crates/service/cargo.toml")
	if got != "cargo-rule" {
		t.Errorf("expected cargo-rule for lowercased cargo.toml, got %q", got)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func TestNewResolver_DefaultOnly(t *testing.T) {
	setTestHome(t, t.TempDir())
	resolver, _, err := NewResolver(t.TempDir(), "", ResolverOptions{})

	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	got := resolver.Resolve("src/main.java")
	if !strings.Contains(got, "Logic Error Detection") {
		t.Errorf("expected system default java rule, got %q", truncate(got, 80))
	}
}

func TestNewResolver_ProjectFileMissing(t *testing.T) {
	resolver, _, err := NewResolver(t.TempDir(), "", ResolverOptions{})

	if err != nil {
		t.Fatalf("NewResolver should not fail when project rule is missing: %v", err)
	}
	got := resolver.Resolve("readme.md")
	if got == "" {
		t.Errorf("expected non-empty default rule")
	}
}

func TestNewResolver_ProjectRuleHighestPriority(t *testing.T) {
	setTestHome(t, t.TempDir())
	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ruleJSON := `{"rules":[{"path":"force-api/**/*.java","rule":"project-java-rule"}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	resolver, _, err := NewResolver(dir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	tests := []struct {
		path string
		want string
	}{
		{"force-api/src/foo.java", "project-java-rule"},
		{"other/src/bar.java", "Logic Error Detection"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := resolver.Resolve(tt.path)
			if !strings.Contains(got, tt.want) {
				t.Errorf("Resolve(%q) = %q, want containing %q", tt.path, truncate(got, 80), tt.want)
			}
		})
	}
}

func TestNewResolver_ProjectRuleFirstMatchWinsWithinFile(t *testing.T) {
	setTestHome(t, t.TempDir())

	// Project rule file:
	//   <repo>/.opencodereview/rule.json
	// Path under test:
	//   internal/config/rules/system_rules.go -> matches both project entries.
	// This verifies declaration order inside one JSON rule file: the first
	// matching entry wins even when a later entry is more specific.
	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ruleJSON := `{"rules":[{"path":"internal/**/*.go","rule":"first-go-rule"},{"path":"internal/config/**/*.go","rule":"second-config-rule"}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	resolver, _, err := NewResolver(dir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	got := resolver.Resolve("internal/config/rules/system_rules.go")
	if got != "first-go-rule" {
		t.Fatalf("expected first matching project rule, got %q", got)
	}
}

func TestNewResolver_ProjectRuleFallsBackToSystem(t *testing.T) {
	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ruleJSON := `{"rules":[{"path":"special/**/*.go","rule":"special-go-rule"}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	resolver, _, err := NewResolver(dir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	got := resolver.Resolve("other/main.go")
	if !strings.Contains(got, "Go Review Principles") {
		t.Errorf("expected system Go rule, got %q", truncate(got, 80))
	}
}

func TestNewResolver_CustomRuleOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	customRule := `{"rules":[{"path":"**/*.go","rule":"custom-go-rule"}]}`
	customPath := filepath.Join(dir, "custom_rules.json")
	if err := os.WriteFile(customPath, []byte(customRule), 0o644); err != nil {
		t.Fatalf("write custom rule: %v", err)
	}

	resolver, _, err := NewResolver(t.TempDir(), customPath, ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	got := resolver.Resolve("main.go")
	if got != "custom-go-rule" {
		t.Errorf("expected custom-go-rule, got %q", got)
	}
	// --rule not matched → falls through to system default
	got = resolver.Resolve("readme.md")
	if !strings.Contains(got, "Correctness") {
		t.Errorf("expected system default rule, got %q", truncate(got, 80))
	}
}

func TestNewResolver_EmptyRuleSkippedAndFallsBack(t *testing.T) {
	setTestHome(t, t.TempDir())

	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ruleJSON := `{"rules":[{"path":"**/*.go","rule":""},{"path":"internal/**/*.go","rule":"second-rule"}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	resolver, _, err := NewResolver(dir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	// main.go matches the first entry (empty rule) — should skip it and fall
	// back to system rule instead of returning "".
	got := resolver.Resolve("main.go")
	if got == "" {
		t.Fatal("expected fallback to system rule, got empty string")
	}

	// internal/pkg/foo.go matches both entries — the empty first entry should
	// be skipped, and the second entry should win.
	got = resolver.Resolve("internal/pkg/foo.go")
	if got != "second-rule" {
		t.Fatalf("expected second-rule, got %q", got)
	}
}

func TestNewResolver_EmptyRuleMergeSystemRuleReturnsSystemOnly(t *testing.T) {
	setTestHome(t, t.TempDir())

	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ruleJSON := `{"rules":[{"path":"**/*.go","rule":"","merge_system_rule":true}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	resolver, _, err := NewResolver(dir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	systemRule, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	wantSystemRule := systemRule.Resolve("main.go")

	got := resolver.Resolve("main.go")
	if got != wantSystemRule {
		t.Fatalf("expected system rule only, got %q", truncate(got, 120))
	}
	if strings.Contains(got, "User-Specific Rules") {
		t.Fatal("should not contain User-Specific Rules header when user rule is empty")
	}
}

func TestNewResolver_ProjectRuleReplacesSystemRuleByDefault(t *testing.T) {
	setTestHome(t, t.TempDir())

	// Project rule file:
	//   <repo>/.opencodereview/rule.json
	// Path under test:
	//   main.go -> matches the project **/*.go rule.
	// This verifies the default behavior: a user rule replaces the system rule
	// unless the matched rule entry opts into merge_system_rule.
	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ruleJSON := `{"rules":[{"path":"**/*.go","rule":"project-go-rule"}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	resolver, _, err := NewResolver(dir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	got := resolver.Resolve("main.go")
	if got != "project-go-rule" {
		t.Fatalf("expected only project rule when merge is disabled, got %q", got)
	}
}

func TestNewResolver_ProjectRuleMergesSystemRule(t *testing.T) {
	setTestHome(t, t.TempDir())

	// Project rule file:
	//   <repo>/.opencodereview/rule.json
	// Path under test:
	//   main.go -> matches both the system Go rule and the project **/*.go rule.
	// This verifies merge_system_rule keeps both rules without depending on the
	// exact merge markdown or ordering.
	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ruleJSON := `{"rules":[{"path":"**/*.go","rule":"project-go-rule","merge_system_rule":true}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	resolver, _, err := NewResolver(dir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	systemRule, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	wantSystemRule := systemRule.Resolve("main.go")
	wantUserRule := "project-go-rule"

	got := resolver.Resolve("main.go")
	systemIdx := strings.Index(got, wantSystemRule)
	if systemIdx < 0 {
		t.Fatalf("expected merged system rule, got %q", truncate(got, 120))
	}
	userIdx := strings.Index(got, wantUserRule)
	if userIdx < 0 {
		t.Fatalf("expected merged project rule, got %q", truncate(got, 120))
	}
}

func TestNewResolver_MergeSystemRuleKeepsRulePriority(t *testing.T) {
	setTestHome(t, t.TempDir())

	// Project rule file:
	//   <repo>/.opencodereview/rule.json
	// Custom rule file:
	//   <custom>/custom_rules.json, passed as --rule equivalent.
	// Path under test:
	//   main.go -> matches custom main.go first, then project **/*.go.
	// This verifies merging does not change layer priority: custom still wins.
	repoDir := t.TempDir()
	ocrDir := filepath.Join(repoDir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	projectRule := `{"rules":[{"path":"**/*.go","rule":"project-go-rule"}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(projectRule), 0o644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	customDir := t.TempDir()
	customRule := `{"rules":[{"path":"main.go","rule":"custom-main-rule","merge_system_rule":true}]}`
	customPath := filepath.Join(customDir, "custom_rules.json")
	if err := os.WriteFile(customPath, []byte(customRule), 0o644); err != nil {
		t.Fatalf("write custom rule: %v", err)
	}

	resolver, _, err := NewResolver(repoDir, customPath, ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	systemRule, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	wantSystemRule := systemRule.Resolve("main.go")

	got := resolver.Resolve("main.go")
	if !strings.Contains(got, wantSystemRule) {
		t.Fatalf("expected merged system rule, got %q", truncate(got, 120))
	}
	if !strings.Contains(got, "custom-main-rule") {
		t.Fatalf("expected custom rule to win, got %q", truncate(got, 120))
	}
	if strings.Contains(got, "project-go-rule") {
		t.Fatalf("project rule should not be merged when custom rule matches first, got %q", truncate(got, 120))
	}
}

func TestNewResolver_CustomOverridesProject(t *testing.T) {
	// Setup --rule file (highest priority)
	customDir := t.TempDir()
	customRule := `{"rules":[{"path":"**/*.java","rule":"custom-java-rule"}]}`
	customPath := filepath.Join(customDir, "custom_rules.json")
	if err := os.WriteFile(customPath, []byte(customRule), 0o644); err != nil {
		t.Fatalf("write custom rule: %v", err)
	}

	// Setup project rule with narrower pattern
	repoDir := t.TempDir()
	ocrDir := filepath.Join(repoDir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	projRule := `{"rules":[{"path":"force-api/**/*.java","rule":"project-java-rule"},{"path":"**/*.go","rule":"project-go-rule"}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(projRule), 0o644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	resolver, _, err := NewResolver(repoDir, customPath, ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	tests := []struct {
		path string
		want string
	}{
		{"force-api/src/foo.java", "custom-java-rule"}, // --rule wins (highest priority)
		{"other/src/bar.java", "custom-java-rule"},     // --rule wins
		{"main.go", "project-go-rule"},                 // --rule misses → project wins
		{"readme.md", "Correctness"},                   // all miss → system default
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := resolver.Resolve(tt.path)
			if !strings.Contains(got, tt.want) {
				t.Errorf("Resolve(%q) = %q, want containing %q", tt.path, truncate(got, 80), tt.want)
			}
		})
	}
}

func TestNewResolver_ProjectFileMalformed(t *testing.T) {
	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte("{invalid json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, _, err := NewResolver(dir, "", ResolverOptions{})
	if err == nil {
		t.Errorf("expected error for malformed project rule.json")
	}
}

func TestFileFilter_IsUserExcluded(t *testing.T) {
	f := &FileFilter{
		Exclude: []string{"**/generated/**", "**/*.pb.go", "vendor/**/*.{go,js}"},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"src/generated/api.java", true},
		{"pkg/foo.pb.go", true},
		{"vendor/lib/util.go", true},
		{"vendor/lib/util.js", true},
		{"src/main.go", false},
		{"src/generated.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := f.IsUserExcluded(tt.path); got != tt.want {
				t.Errorf("IsUserExcluded(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestFileFilter_IsUserIncluded(t *testing.T) {
	f := &FileFilter{
		Include: []string{"src/**/*.java", "src/**/*.{kt,kts}"},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"src/main/foo.java", true},
		{"src/main/bar.kt", true},
		{"src/build.kts", true},
		{"test/main.java", false},
		{"src/main/util.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := f.IsUserIncluded(tt.path); got != tt.want {
				t.Errorf("IsUserIncluded(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestFileFilter_IsUserIncluded_EmptyInclude(t *testing.T) {
	f := &FileFilter{}
	if f.IsUserIncluded("anything.java") {
		t.Errorf("expected false when include is empty")
	}
}

func TestFileFilter_CaseInsensitive(t *testing.T) {
	f := &FileFilter{
		Include: []string{"src/**/*.java", "**/CHANGELOG.md"},
		Exclude: []string{"**/generated/**", "README.md", "**/*.{Go,Java}"},
	}

	if !f.IsUserIncluded("SRC/Main/Foo.Java") {
		t.Errorf("expected case-insensitive include match")
	}
	if !f.IsUserExcluded("SRC/Generated/Api.java") {
		t.Errorf("expected case-insensitive exclude match")
	}

	// Verify patterns containing uppercase letters also match.
	if !f.IsUserIncluded("docs/CHANGELOG.md") {
		t.Errorf("expected uppercase include pattern to match case-insensitively")
	}
	if !f.IsUserExcluded("README.md") {
		t.Errorf("expected uppercase exclude pattern to match case-insensitively")
	}
	if !f.IsUserExcluded("pkg/Main.JAVA") {
		t.Errorf("expected brace-expanded uppercase pattern to match case-insensitively")
	}
}

func TestNewResolver_FileFilterMerged(t *testing.T) {
	repoDir := t.TempDir()
	ocrDir := filepath.Join(repoDir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	projJSON := `{"rules":[],"include":["src/**/*.java"],"exclude":["**/generated/**"]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(projJSON), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, filter, err := NewResolver(repoDir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	if filter == nil {
		t.Fatal("expected non-nil FileFilter")
	}
	if !filter.HasInclude() {
		t.Error("expected HasInclude to be true")
	}
	if !filter.IsUserIncluded("src/main/foo.java") {
		t.Error("expected src/main/foo.java to be included")
	}
	if !filter.IsUserExcluded("src/generated/api.java") {
		t.Error("expected src/generated/api.java to be excluded")
	}
}

func TestNewResolver_FileFilterNilWhenEmpty(t *testing.T) {
	_, filter, err := NewResolver(t.TempDir(), "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	if filter != nil {
		t.Errorf("expected nil FileFilter when no include/exclude configured, got %+v", filter)
	}
}

func TestNewResolver_FileFilterPriorityOverride(t *testing.T) {
	repoDir := t.TempDir()
	ocrDir := filepath.Join(repoDir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	projJSON := `{"rules":[],"include":["src/**/*.java"],"exclude":["**/gen/**"]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(projJSON), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	customDir := t.TempDir()
	customJSON := `{"rules":[],"include":["lib/**/*.kt"],"exclude":["**/tmp/**"]}`
	customPath := filepath.Join(customDir, "custom.json")
	if err := os.WriteFile(customPath, []byte(customJSON), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, filter, err := NewResolver(repoDir, customPath, ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	if filter == nil {
		t.Fatal("expected non-nil FileFilter")
	}

	// Custom (--rule) has highest priority, so only its patterns take effect
	if !filter.IsUserIncluded("lib/util.kt") {
		t.Error("expected custom include to be active")
	}
	if !filter.IsUserExcluded("lib/tmp/cache.kt") {
		t.Error("expected custom exclude to be active")
	}

	// Project patterns should NOT be active since custom overrides
	if filter.IsUserIncluded("src/main/foo.java") {
		t.Error("project include should not be active when custom is present")
	}
	if filter.IsUserExcluded("src/gen/api.java") {
		t.Error("project exclude should not be active when custom is present")
	}
}

func TestNewResolver_FileFilterFallsToProject(t *testing.T) {
	repoDir := t.TempDir()
	ocrDir := filepath.Join(repoDir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	projJSON := `{"rules":[],"include":["src/**/*.java"],"exclude":["**/gen/**"]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(projJSON), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Custom rule has no include/exclude — should fall through to project
	customDir := t.TempDir()
	customJSON := `{"rules":[{"path":"**/*.go","rule":"custom-go"}]}`
	customPath := filepath.Join(customDir, "custom.json")
	if err := os.WriteFile(customPath, []byte(customJSON), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, filter, err := NewResolver(repoDir, customPath, ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	if filter == nil {
		t.Fatal("expected non-nil FileFilter from project layer")
	}
	if !filter.IsUserIncluded("src/main/foo.java") {
		t.Error("expected project include to take effect when custom has none")
	}
}

func TestResolveDetail_SystemDefault(t *testing.T) {
	resolver, _, err := NewResolver(t.TempDir(), "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	dr := resolver.(DetailResolver)

	detail := dr.ResolveDetail("readme.md")
	if detail.Source != "system" {
		t.Errorf("expected source 'system', got %q", detail.Source)
	}
	if detail.Pattern != "default" {
		t.Errorf("expected pattern 'default', got %q", detail.Pattern)
	}
	if !strings.Contains(detail.Rule, "Correctness") {
		t.Errorf("expected default rule content, got %q", truncate(detail.Rule, 80))
	}
}

func TestResolveDetail_SystemPatternMatch(t *testing.T) {
	setTestHome(t, t.TempDir())
	resolver, _, err := NewResolver(t.TempDir(), "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	dr := resolver.(DetailResolver)

	detail := dr.ResolveDetail("src/main/foo.java")
	if detail.Source != "system" {
		t.Errorf("expected source 'system', got %q", detail.Source)
	}
	if detail.Pattern != "**/*.java" {
		t.Errorf("expected pattern '**/*.java', got %q", detail.Pattern)
	}
	if !strings.Contains(detail.Rule, "Logic Error Detection") {
		t.Errorf("expected java rule, got %q", truncate(detail.Rule, 80))
	}
}

func TestResolveDetail_SystemPrismaPatternMatch(t *testing.T) {
	setTestHome(t, t.TempDir())
	resolver, _, err := NewResolver(t.TempDir(), "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	dr := resolver.(DetailResolver)

	for _, path := range []string{"schema.prisma", "prisma/schema.prisma", "PRISMA/SCHEMA.PRISMA"} {
		t.Run(path, func(t *testing.T) {
			detail := dr.ResolveDetail(path)
			if detail.Source != "system" {
				t.Errorf("expected source 'system', got %q", detail.Source)
			}
			if detail.Pattern != "**/*.prisma" {
				t.Errorf("expected pattern '**/*.prisma', got %q", detail.Pattern)
			}
			if !strings.Contains(detail.Rule, "Prisma Schema Review Principles") {
				t.Errorf("expected Prisma rule, got %q", truncate(detail.Rule, 80))
			}
		})
	}
}

func TestResolveDetail_SystemPugPatternMatch(t *testing.T) {
	setTestHome(t, t.TempDir())
	resolver, _, err := NewResolver(t.TempDir(), "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	dr := resolver.(DetailResolver)

	for _, path := range []string{"index.pug", "views/account/profile.pug", "VIEWS/INDEX.PUG"} {
		t.Run(path, func(t *testing.T) {
			detail := dr.ResolveDetail(path)
			if detail.Source != "system" {
				t.Errorf("expected source 'system', got %q", detail.Source)
			}
			if detail.Pattern != "**/*.pug" {
				t.Errorf("expected pattern '**/*.pug', got %q", detail.Pattern)
			}
			for _, required := range []string{
				"Pug Escaping and Output Contexts",
				"server-side template injection",
				"&attributes",
				"compileClient",
				"Accessibility",
			} {
				if !strings.Contains(detail.Rule, required) {
					t.Errorf("expected Pug rule to contain %q", required)
				}
			}
		})
	}
}

func TestResolveDetail_SystemGoPatternMatch(t *testing.T) {
	setTestHome(t, t.TempDir())
	resolver, _, err := NewResolver(t.TempDir(), "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	dr := resolver.(DetailResolver)

	for _, path := range []string{"main.go", "internal/service/user.go", "CMD/MAIN.GO"} {
		t.Run(path, func(t *testing.T) {
			detail := dr.ResolveDetail(path)
			if detail.Source != "system" {
				t.Errorf("expected source 'system', got %q", detail.Source)
			}
			if detail.Pattern != "**/*.go" {
				t.Errorf("expected pattern '**/*.go', got %q", detail.Pattern)
			}
			for _, required := range []string{
				"Go Review Principles",
				"Go 1.23+",
				"defer` inside a loop",
				"crypto/rand",
			} {
				if !strings.Contains(detail.Rule, required) {
					t.Errorf("expected Go rule to contain %q", required)
				}
			}
		})
	}
}

func TestResolveDetail_SystemPHPPatternMatch(t *testing.T) {
	setTestHome(t, t.TempDir())
	resolver, _, err := NewResolver(t.TempDir(), "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	dr := resolver.(DetailResolver)

	for _, path := range []string{"index.php", "src/Controller/UserController.php", "TEMPLATES/INDEX.PHTML"} {
		t.Run(path, func(t *testing.T) {
			detail := dr.ResolveDetail(path)
			if detail.Source != "system" {
				t.Errorf("expected source 'system', got %q", detail.Source)
			}
			if detail.Pattern != "**/*.{php,phtml}" {
				t.Errorf("expected pattern '**/*.{php,phtml}', got %q", detail.Pattern)
			}
			for _, required := range []string{
				"PHP Review Principles",
				"foreach` value variable iterated by reference",
				"unserialize()",
				"PHPStan",
			} {
				if !strings.Contains(detail.Rule, required) {
					t.Errorf("expected PHP rule to contain %q", required)
				}
			}
		})
	}
}

func TestResolveDetail_SystemComposerPatternPrecedesJSON(t *testing.T) {
	setTestHome(t, t.TempDir())
	resolver, _, err := NewResolver(t.TempDir(), "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	dr := resolver.(DetailResolver)

	for _, path := range []string{"composer.json", "packages/library/composer.json", "PACKAGES/APP/COMPOSER.JSON"} {
		t.Run(path, func(t *testing.T) {
			detail := dr.ResolveDetail(path)
			if detail.Source != "system" {
				t.Errorf("expected source 'system', got %q", detail.Source)
			}
			if detail.Pattern != "**/composer.json" {
				t.Errorf("expected pattern '**/composer.json', got %q", detail.Pattern)
			}
			for _, required := range []string{
				"Composer Manifest Review Principles",
				"config.allow-plugins",
				"PSR-4",
			} {
				if !strings.Contains(detail.Rule, required) {
					t.Errorf("expected Composer rule to contain %q", required)
				}
			}
		})
	}
}

func TestResolveDetail_ProjectOverridesSystem(t *testing.T) {
	setTestHome(t, t.TempDir())
	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ruleJSON := `{"rules":[{"path":"src/**/*.java","rule":"project-java-rule"}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	resolver, _, err := NewResolver(dir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	dr := resolver.(DetailResolver)

	detail := dr.ResolveDetail("src/main/foo.java")
	if detail.Source != "project" {
		t.Errorf("expected source 'project', got %q", detail.Source)
	}
	if detail.Pattern != "src/**/*.java" {
		t.Errorf("expected pattern 'src/**/*.java', got %q", detail.Pattern)
	}
	if detail.Rule != "project-java-rule" {
		t.Errorf("expected 'project-java-rule', got %q", detail.Rule)
	}

	// Unmatched path falls to system
	detail = dr.ResolveDetail("other/bar.java")
	if detail.Source != "system" {
		t.Errorf("expected source 'system', got %q", detail.Source)
	}
}

func TestResolveDetail_MergeSystemRule(t *testing.T) {
	setTestHome(t, t.TempDir())

	// Project rule file:
	//   <repo>/.opencodereview/rule.json
	// Path under test:
	//   src/main/foo.java -> matches both the system Java rule and the project rule.
	// This verifies ResolveDetail reports the matched user rule metadata while
	// returning merged rule text when the entry sets merge_system_rule.
	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ruleJSON := `{"rules":[{"path":"src/**/*.java","rule":"project-java-rule","merge_system_rule":true}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	resolver, _, err := NewResolver(dir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	dr := resolver.(DetailResolver)

	systemRule, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	wantSystemRule := systemRule.Resolve("src/main/foo.java")

	detail := dr.ResolveDetail("src/main/foo.java")
	if detail.Source != "project" {
		t.Errorf("expected source 'project', got %q", detail.Source)
	}
	if detail.Pattern != "src/**/*.java" {
		t.Errorf("expected pattern 'src/**/*.java', got %q", detail.Pattern)
	}
	if !strings.Contains(detail.Rule, wantSystemRule) {
		t.Fatalf("expected merged system rule, got %q", truncate(detail.Rule, 120))
	}
	if !strings.Contains(detail.Rule, "project-java-rule") {
		t.Fatalf("expected merged project rule, got %q", truncate(detail.Rule, 120))
	}
}

func TestResolveDetail_CustomOverridesAll(t *testing.T) {
	// Project rule
	repoDir := t.TempDir()
	ocrDir := filepath.Join(repoDir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	projJSON := `{"rules":[{"path":"**/*.java","rule":"project-java-rule"}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(projJSON), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Custom rule (highest priority)
	customDir := t.TempDir()
	customJSON := `{"rules":[{"path":"**/*.java","rule":"custom-java-rule"}]}`
	customPath := filepath.Join(customDir, "custom.json")
	if err := os.WriteFile(customPath, []byte(customJSON), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	resolver, _, err := NewResolver(repoDir, customPath, ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	dr := resolver.(DetailResolver)

	detail := dr.ResolveDetail("src/foo.java")
	if detail.Source != "custom" {
		t.Errorf("expected source 'custom', got %q", detail.Source)
	}
	if detail.Rule != "custom-java-rule" {
		t.Errorf("expected 'custom-java-rule', got %q", detail.Rule)
	}
}

func TestNewResolver_BraceExpansionInProjectRule(t *testing.T) {
	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ruleJSON := `{"rules":[{"path":"src/**/*.{java,kt}","rule":"jvm-rule"}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	resolver, _, err := NewResolver(dir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	tests := []struct {
		path string
		want string
	}{
		{"src/main/foo.java", "jvm-rule"},
		{"src/main/bar.kt", "jvm-rule"},
		{"src/main/baz.swift", "Correctness"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := resolver.Resolve(tt.path)
			if !strings.Contains(got, tt.want) {
				t.Errorf("Resolve(%q) = %q, want containing %q", tt.path, truncate(got, 80), tt.want)
			}
		})
	}
}

// ── resolveRuleEntries tests ──

func TestResolveRuleEntries_BasicFile(t *testing.T) {
	dir := t.TempDir()
	ruleFile := filepath.Join(dir, "sql-rules.md")
	if err := os.WriteFile(ruleFile, []byte("Check for SQL injection\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.xml", Rule: "sql-rules.md"},
		{Path: "**/*.go", Rule: "Always check for nil"},
	}
	resolveRuleEntries(entries, dir, "")

	if entries[0].Rule != "Check for SQL injection" {
		t.Errorf("expected file content, got %q", entries[0].Rule)
	}
	if entries[1].Rule != "Always check for nil" {
		t.Errorf("inline rule should not change, got %q", entries[1].Rule)
	}
}

func TestResolveRuleEntries_MultiLineInline(t *testing.T) {
	dir := t.TempDir()
	// Create a file with the same name as the inline rule to make sure
	// multi-line detection prevents file lookup.
	if err := os.WriteFile(filepath.Join(dir, "security.md"), []byte("file content"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.ts", Rule: "security.md\nBut this is multi-line\nso it should stay inline"},
	}
	resolveRuleEntries(entries, dir, "")

	if entries[0].Rule != "security.md\nBut this is multi-line\nso it should stay inline" {
		t.Errorf("multi-line rule should stay inline, got %q", entries[0].Rule)
	}
}

func TestResolveRuleEntries_MissingFile(t *testing.T) {
	dir := t.TempDir()

	entries := []ProjectRuleEntry{
		{Path: "**/*.xml", Rule: "nonexistent.md"},
	}
	resolveRuleEntries(entries, dir, "")

	// Missing file should clear the rule.
	if entries[0].Rule != "" {
		t.Errorf("missing file should clear rule, got %q", entries[0].Rule)
	}
}

func TestResolveRuleEntries_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	ruleFile := filepath.Join(dir, "my-rule.md")
	if err := os.WriteFile(ruleFile, []byte("absolute rule content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Use an absolute path pointing to a file in a different directory.
	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: ruleFile},
	}
	resolveRuleEntries(entries, "/some/other/repo", "")

	if entries[0].Rule != "absolute rule content" {
		t.Errorf("expected absolute file content, got %q", entries[0].Rule)
	}
}

func TestResolveRuleEntries_TooLarge(t *testing.T) {
	dir := t.TempDir()
	big := make([]byte, 513*1024)
	for i := range big {
		big[i] = 'a'
	}
	bigFile := filepath.Join(dir, "big.md")
	if err := os.WriteFile(bigFile, big, 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: "big.md"},
	}
	resolveRuleEntries(entries, dir, "")

	if entries[0].Rule != "" {
		t.Errorf("oversized file should clear rule, got %q", entries[0].Rule)
	}
}

func TestResolveRuleEntries_RelativePath(t *testing.T) {
	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "shared.md"), []byte("repo-level"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: "shared.md"},
	}
	resolveRuleEntries(entries, repoDir, "")

	if entries[0].Rule != "repo-level" {
		t.Errorf("repo-level should win, got %q", entries[0].Rule)
	}
}

func TestResolveRuleEntries_EmptyRule(t *testing.T) {
	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: ""},
		{Path: "**/*.ts", Rule: "  "},
		{Path: "**/*.java", Rule: "\t\n"},
	}
	resolveRuleEntries(entries, "/tmp", "")

	if entries[0].Rule != "" {
		t.Errorf("empty rule should stay empty, got %q", entries[0].Rule)
	}
	if entries[1].Rule != "  " {
		t.Errorf("whitespace-only rule should stay unchanged, got %q", entries[1].Rule)
	}
	if entries[2].Rule != "\t\n" {
		t.Errorf("whitespace+newline rule should stay unchanged, got %q", entries[2].Rule)
	}
}

func TestResolveRuleEntries_SymlinkSafety(t *testing.T) {
	dir := t.TempDir()
	sensitiveFile := filepath.Join(dir, "secret.json")
	if err := os.WriteFile(sensitiveFile, []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a symlink with .md extension pointing to a .json file.
	// The extension check on the resolved path should reject .json.
	symlinkPath := filepath.Join(dir, "evil.md")
	if err := os.Symlink(sensitiveFile, symlinkPath); err != nil {
		// Creating a symlink on Windows needs SeCreateSymbolicLinkPrivilege, which
		// an unelevated CI account does not have. Same skip the other symlink tests
		// in this repo already use.
		t.Skipf("cannot create symlink: %v", err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: "evil.md"},
	}
	resolveRuleEntries(entries, dir, "")
	// The symlink target is .json, which is not in the whitelist.
	// The rule should be cleared.
	if entries[0].Rule != "" {
		t.Errorf("symlink to non-whitelisted file should clear rule, got %q", entries[0].Rule)
	}
}

func TestResolveRuleEntries_TxtExtension(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rules.txt"), []byte("rule from txt"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: "rules.txt"},
	}
	resolveRuleEntries(entries, dir, "")

	if entries[0].Rule != "rule from txt" {
		t.Errorf(".txt should be accepted, got %q", entries[0].Rule)
	}
}

func TestResolveRuleEntries_MarkdownExtension(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rules.markdown"), []byte("rule from markdown"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: "rules.markdown"},
	}
	resolveRuleEntries(entries, dir, "")

	if entries[0].Rule != "rule from markdown" {
		t.Errorf(".markdown should be accepted, got %q", entries[0].Rule)
	}
}

func TestResolveRuleEntries_SubdirectoryPath(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "my-rule.md"), []byte("nested rule"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: "docs/my-rule.md"},
	}
	resolveRuleEntries(entries, dir, "")

	if entries[0].Rule != "nested rule" {
		t.Errorf("subdirectory path should work, got %q", entries[0].Rule)
	}
}

// ── looksLikeFilePath tests ──

func TestLooksLikeFilePath_InlineContent(t *testing.T) {
	tests := []string{
		"Check for null pointers",
		"Always validate input",
		"security",
		"xss",
	}
	for _, s := range tests {
		if looksLikeFilePath(s) {
			t.Errorf("looksLikeFilePath(%q) should be false", s)
		}
	}
}

func TestLooksLikeFilePath_MultiLine(t *testing.T) {
	s := "line1\nline2\nline3"
	if looksLikeFilePath(s) {
		t.Errorf("multi-line should be false")
	}
}

func TestLooksLikeFilePath_FileExtensions(t *testing.T) {
	tests := []string{
		"rules.md",
		"doc.txt",
		"doc.markdown",
		"DOC.MD",
		"path/to/file.md",
	}
	for _, s := range tests {
		if !looksLikeFilePath(s) {
			t.Errorf("looksLikeFilePath(%q) should be true", s)
		}
	}
}

func TestLooksLikeFilePath_WithSpaces(t *testing.T) {
	// Values containing spaces are inline, not file paths.
	tests := []string{
		"Follow rules from team.md",
		"Ensure output is in .md",
		"use .txt format",
	}
	for _, s := range tests {
		if looksLikeFilePath(s) {
			t.Errorf("looksLikeFilePath(%q) should be false (contains space)", s)
		}
	}
}

func TestLooksLikeFilePath_PathWithoutExtension(t *testing.T) {
	// Paths without .md/.txt/.markdown are NOT treated as file paths.
	tests := []string{
		"docs/security",
		"shared/rules/go",
		"Use HTTP/2 for all requests",
	}
	for _, s := range tests {
		if looksLikeFilePath(s) {
			t.Errorf("looksLikeFilePath(%q) should be false (no .md/.txt/.markdown)", s)
		}
	}
}

// ── readRuleFileSafe tests ──

func TestReadRuleFileSafe_NormalFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.md")
	if err := os.WriteFile(f, []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	content, err := readRuleFileSafe(f, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "hello world" {
		t.Errorf("expected 'hello world', got %q", content)
	}
}

func TestReadRuleFileSafe_UnsupportedExt(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.json")
	if err := os.WriteFile(f, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := readRuleFileSafe(f, "")
	if err == nil {
		t.Fatal("expected error for .json")
	}
}

func TestReadRuleFileSafe_TooLarge(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "big.md")
	big := make([]byte, 513*1024)
	if err := os.WriteFile(f, big, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := readRuleFileSafe(f, "")
	if err == nil {
		t.Fatal("expected error for oversized file")
	}
}

func TestReadRuleFileSafe_Missing(t *testing.T) {
	_, err := readRuleFileSafe("/nonexistent/path.md", "")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ── path traversal tests ──

func TestResolveRuleEntries_PathTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	// Create a file outside the repo dir to prove it is NOT read.
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("should not be read\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: outside},         // absolute path to outside — allowed
		{Path: "**/*.ts", Rule: "../outside.md"}, // relative traversal — blocked
	}
	resolveRuleEntries(entries, dir, "")

	// Absolute path to outside is allowed (explicit design choice).
	if entries[0].Rule != "should not be read" {
		t.Errorf("absolute path to outside should be allowed, got %q", entries[0].Rule)
	}
	// Relative traversal should be blocked and rule cleared.
	if entries[1].Rule != "" {
		t.Errorf("relative traversal should be blocked, got %q", entries[1].Rule)
	}
}

func TestResolveRuleEntries_EmptyRepoDirRelative(t *testing.T) {
	// When repoDir is empty and rule is relative, it should be rejected.
	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: "rules.md"},
	}
	resolveRuleEntries(entries, "", "")

	if entries[0].Rule != "" {
		t.Errorf("relative path with empty repoDir should be rejected, got %q", entries[0].Rule)
	}
}

func TestResolveRuleEntries_EmptyRepoDirAbsolute(t *testing.T) {
	dir := t.TempDir()
	absFile := filepath.Join(dir, "abs.md")
	if err := os.WriteFile(absFile, []byte("absolute content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: absFile},
	}
	resolveRuleEntries(entries, "", "")

	if entries[0].Rule != "absolute content" {
		t.Errorf("absolute path with empty repoDir should work, got %q", entries[0].Rule)
	}
}

func TestResolveRuleEntries_GlobalRuleFileResolution(t *testing.T) {
	// Simulate loadGlobalRule: repoDir = filepath.Dir(~/.opencodereview/rule.json)
	homeDir := t.TempDir()
	setTestHome(t, homeDir)

	globalRuleDir := filepath.Join(homeDir, ".opencodereview")
	if err := os.MkdirAll(globalRuleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalRuleDir, "reusable.md"), []byte("global reusable rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: "reusable.md"},
	}
	// repoDir = ~/.opencodereview (where rule.json lives)
	resolveRuleEntries(entries, globalRuleDir, "")

	if entries[0].Rule != "global reusable rule" {
		t.Errorf("global rule file should be resolved, got %q", entries[0].Rule)
	}
}

// ── project-layer confinement tests ──

// canonicalRoot returns the canonical (symlink-resolved) form of dir, mirroring
// loadProjectRule's pathutil.CanonicalPath call.
func canonicalRoot(t *testing.T, dir string) string {
	t.Helper()
	root, err := pathutil.CanonicalPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// TestResolveRuleEntries_ProjectConfinesAbsolutePath verifies that with a confineRoot
// (the untrusted project rule layer) an absolute rule path is confined to repoDir: an
// absolute path inside the repo is allowed, one outside the repo is rejected.
func TestResolveRuleEntries_ProjectConfinesAbsolutePath(t *testing.T) {
	repoDir := t.TempDir()
	inside := filepath.Join(repoDir, "inside.md")
	if err := os.WriteFile(inside, []byte("inside repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("OUTSIDE SECRET\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: inside},  // absolute, inside repo — allowed
		{Path: "**/*.ts", Rule: outside}, // absolute, outside repo — blocked
	}
	resolveRuleEntries(entries, repoDir, canonicalRoot(t, repoDir))

	if entries[0].Rule != "inside repo" {
		t.Errorf("absolute path inside repo should be allowed, got %q", entries[0].Rule)
	}
	if entries[1].Rule != "" {
		t.Errorf("absolute path outside repo should be blocked, got %q", entries[1].Rule)
	}
}

// TestResolveRuleEntries_ProjectConfinesSymlinkEscape verifies that with confine=true,
// an in-repo symlink pointing at an out-of-repo file is rejected after resolution.
func TestResolveRuleEntries_ProjectConfinesSymlinkEscape(t *testing.T) {
	repoDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "host-secret.md")
	if err := os.WriteFile(outside, []byte("HOST SECRET\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(repoDir, "rules.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: "rules.md"},
	}
	resolveRuleEntries(entries, repoDir, canonicalRoot(t, repoDir))

	if entries[0].Rule != "" {
		t.Errorf("symlink escaping repo should be blocked, got %q", entries[0].Rule)
	}
}

// TestResolveRuleEntries_TrustedAbsoluteOutsideAllowed guards that the trusted layers
// (confine=false) keep the previous behavior: absolute paths may point outside repoDir.
func TestResolveRuleEntries_TrustedAbsoluteOutsideAllowed(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "shared.md")
	if err := os.WriteFile(outside, []byte("shared rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: outside},
	}
	resolveRuleEntries(entries, dir, "")

	if entries[0].Rule != "shared rules" {
		t.Errorf("trusted absolute path should still be allowed, got %q", entries[0].Rule)
	}
}

// TestReadRuleFileSafe_ConfineRejectsOutside verifies the confinement check itself.
func TestReadRuleFileSafe_ConfineRejectsOutside(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := readRuleFileSafe(outside, canonicalRoot(t, root)); err == nil {
		t.Fatal("expected confinement error for path outside root")
	}

	inside := filepath.Join(root, "ok.md")
	if err := os.WriteFile(inside, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readRuleFileSafe(inside, canonicalRoot(t, root)); err != nil {
		t.Fatalf("path inside root should be allowed, got %v", err)
	}
}

// TestLoadProjectRule_ConfinesUntrustedReferences exercises the real entry point: a
// malicious .opencodereview/rule.json referencing an absolute path outside the repo
// must not leak that file's contents into the resolved rule.
func TestLoadProjectRule_ConfinesUntrustedReferences(t *testing.T) {
	repoDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(outside, []byte("AWS_KEY=AKIA-SECRET\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgDir := filepath.Join(repoDir, ".opencodereview")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgData, err := json.Marshal(ProjectRule{
		Rules: []ProjectRuleEntry{{Path: "**/*.go", Rule: outside}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "rule.json"), cfgData, 0o644); err != nil {
		t.Fatal(err)
	}

	pr, err := loadProjectRule(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	if pr == nil || len(pr.Rules) != 1 {
		t.Fatalf("expected one project rule, got %+v", pr)
	}
	if pr.Rules[0].Rule != "" {
		t.Errorf("untrusted absolute reference must be cleared, got %q", pr.Rules[0].Rule)
	}
}

// TestLoadProjectRule_RejectsSymlinkedDir verifies that an attacker-committed
// ".opencodereview" directory symlink pointing outside the repo cannot make
// loadProjectRule read a rule file from outside the repository.
func TestLoadProjectRule_RejectsSymlinkedDir(t *testing.T) {
	repoDir := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "rule.json"), []byte(`{"rules":[{"path":"**/*.go","rule":"OUTSIDE"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(repoDir, ".opencodereview")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	pr, err := loadProjectRule(repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr != nil {
		t.Errorf("symlinked .opencodereview dir outside repo must be skipped, got %+v", pr)
	}
}

// TestLoadProjectRule_RejectsSymlinkedRuleFile verifies that a symlinked "rule.json"
// pointing outside the repo cannot be read.
func TestLoadProjectRule_RejectsSymlinkedRuleFile(t *testing.T) {
	repoDir := t.TempDir()
	cfgDir := filepath.Join(repoDir, ".opencodereview")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside-rule.json")
	if err := os.WriteFile(outside, []byte(`{"rules":[{"path":"**/*.go","rule":"OUTSIDE"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(cfgDir, "rule.json")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	pr, err := loadProjectRule(repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr != nil {
		t.Errorf("symlinked rule.json outside repo must be skipped, got %+v", pr)
	}
}

// TestResolveRuleEntries_ProjectRelativeInsideAllowed covers the confined relative
// (non-symlink) path that stays inside the repo.
func TestResolveRuleEntries_ProjectRelativeInsideAllowed(t *testing.T) {
	repoDir := t.TempDir()
	docs := filepath.Join(repoDir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "my-rule.md"), []byte("nested rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: "docs/my-rule.md"},
	}
	resolveRuleEntries(entries, repoDir, canonicalRoot(t, repoDir))

	if entries[0].Rule != "nested rule" {
		t.Errorf("confined relative in-repo path should be read, got %q", entries[0].Rule)
	}
}

// specialCaseRuleDocs lists rule_docs files loaded directly by Go code rather
// than referenced from system_rules.json's path_rule_map, so the orphan-file
// check in TestSystemRulesIntegrity must not flag them.
var specialCaseRuleDocs = map[string]bool{
	// Backs the MATLAB/Objective-C ".m" content sniff in sniffer.Resolve;
	// loaded via loadObjCRule, called from NewResolver.
	"objc.md": true,
}

// referencedRuleFiles reads the embedded system_rules.json and returns the set of
// rule_docs filenames it references (default_rule + every path_rule_map value).
// A plain map decode is enough here: we only need the value set, not key order.
func referencedRuleFiles(t *testing.T) map[string]bool {
	t.Helper()
	data, err := rulesFS.ReadFile("system_rules.json")
	if err != nil {
		t.Fatalf("read embedded system_rules.json: %v", err)
	}
	var raw struct {
		DefaultRule string            `json:"default_rule"`
		PathRuleMap map[string]string `json:"path_rule_map"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal system_rules.json: %v", err)
	}
	refs := make(map[string]bool, len(raw.PathRuleMap)+1)
	if raw.DefaultRule != "" {
		refs[raw.DefaultRule] = true
	}
	for _, name := range raw.PathRuleMap {
		refs[name] = true
	}
	return refs
}

// globExt reports the extension a brace-expanded glob selects on, and whether
// the glob is extension-based at all. Only a trailing "*.<ext>" segment with no
// further wildcard qualifies, so filename globs ("**/pom.xml") and infix globs
// ("**/*mapper*.xml") are skipped rather than misread as extension claims.
func globExt(pattern string) (string, bool) {
	base := pattern[strings.LastIndex(pattern, "/")+1:]
	if !strings.HasPrefix(base, "*.") || strings.ContainsAny(base[2:], "*?[{") {
		return "", false
	}
	return base[1:], true
}

func TestSystemRulesIntegrity(t *testing.T) {
	rule, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}

	t.Run("file_existence", func(t *testing.T) {
		refs := referencedRuleFiles(t)
		for name := range refs {
			if _, err := rulesFS.ReadFile("rule_docs/" + name); err != nil {
				t.Errorf("rule_docs/%s referenced by system_rules.json but not embedded: %v", name, err)
			}
		}
	})

	t.Run("pattern_validity", func(t *testing.T) {
		// Resolve expands braces before matching, so validate each expanded arm
		// rather than the raw pattern (which may contain "{go,py}").
		for _, pr := range rule.PathRules {
			for _, p := range expandBraces(pr.Pattern) {
				if !doublestar.ValidatePattern(p) {
					t.Errorf("pattern %q (expanded from %q) is not a valid glob", p, pr.Pattern)
				}
			}
		}
	})

	t.Run("no_orphan_files", func(t *testing.T) {
		refs := referencedRuleFiles(t)
		entries, err := rulesFS.ReadDir("rule_docs")
		if err != nil {
			t.Fatalf("read embedded rule_docs: %v", err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if !refs[e.Name()] && !specialCaseRuleDocs[e.Name()] {
				t.Errorf("rule_docs/%s is not referenced by system_rules.json (orphan file)", e.Name())
			}
		}
	})

	t.Run("extensions_are_allowlisted", func(t *testing.T) {
		// A rule doc is dead unless its extension also passes the allowlist:
		// scan/agent.go and agent/preview.go drop a file on its extension
		// before any rule is resolved. Only extension globs are checked;
		// filename globs like "**/pom.xml" carry no extension claim.
		for _, pr := range rule.PathRules {
			for _, p := range expandBraces(pr.Pattern) {
				ext, ok := globExt(p)
				if !ok {
					continue
				}
				t.Run(p, func(t *testing.T) {
					if !allowedext.IsAllowedExt(ext) {
						t.Errorf("path_rule_map glob %q targets extension %q, which is missing from "+
							"internal/config/allowlist/supported_file_types.json, so its rule can never run",
							pr.Pattern, ext)
					}
				})
			}
		}
	})

	t.Run("no_duplicate_patterns", func(t *testing.T) {
		// SystemRule.UnmarshalJSON preserves declaration order via a streaming
		// decoder, so duplicate keys survive in PathRules rather than being
		// silently collapsed by a map decode.
		seen := make(map[string]int)
		for _, pr := range rule.PathRules {
			seen[pr.Pattern]++
		}
		for pattern, count := range seen {
			if count > 1 {
				t.Errorf("pattern %q appears %d times in path_rule_map", pattern, count)
			}
		}
	})
}

// TestLoadRuleFile covers loadRuleFile's read-error and unmarshal-error
// branches plus the success path that resolves entries and returns the rule.
func TestLoadRuleFile(t *testing.T) {
	t.Run("read error on missing path", func(t *testing.T) {
		if _, err := loadRuleFile(filepath.Join(t.TempDir(), "nope.json")); err == nil {
			t.Fatal("expected read error for missing rule file, got nil")
		}
	})

	t.Run("unmarshal error on invalid JSON", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "rule.json")
		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatalf("write invalid rule: %v", err)
		}
		if _, err := loadRuleFile(path); err == nil {
			t.Fatal("expected unmarshal error for invalid JSON, got nil")
		}
	})

	t.Run("valid file returns rule", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "rule.json")
		if err := os.WriteFile(path, []byte(`{"rules":[{"rule":"be careful"}]}`), 0o644); err != nil {
			t.Fatalf("write valid rule: %v", err)
		}
		pr, err := loadRuleFile(path)
		if err != nil {
			t.Fatalf("loadRuleFile: %v", err)
		}
		if pr == nil || len(pr.Rules) != 1 || pr.Rules[0].Rule != "be careful" {
			t.Errorf("unexpected rule: %+v", pr)
		}
	})
}

// TestLoadGlobalRule covers loadGlobalRule's non-NotExist read error,
// unmarshal error, and success branches by pointing HOME at a temp dir.
func TestLoadGlobalRule(t *testing.T) {
	globalRulePath := func(home string) string {
		return filepath.Join(home, ".opencodereview", "rule.json")
	}

	t.Run("missing file is not an error", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		pr, err := loadGlobalRule()
		if err != nil || pr != nil {
			t.Fatalf("expected nil,nil for missing global rule: pr=%v err=%v", pr, err)
		}
	})

	t.Run("read error when path is a directory", func(t *testing.T) {
		home := t.TempDir()
		setTestHome(t, home)
		// Create the rule.json path as a directory so ReadFile fails with a
		// non-NotExist error (EISDIR), exercising the wrapped-error branch.
		if err := os.MkdirAll(globalRulePath(home), 0o755); err != nil {
			t.Fatalf("mkdir rule path: %v", err)
		}
		if _, err := loadGlobalRule(); err == nil {
			t.Fatal("expected read error when rule path is a directory, got nil")
		}
	})

	t.Run("unmarshal error on invalid JSON", func(t *testing.T) {
		home := t.TempDir()
		setTestHome(t, home)
		path := globalRulePath(home)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir parent: %v", err)
		}
		if err := os.WriteFile(path, []byte("{bad"), 0o644); err != nil {
			t.Fatalf("write invalid rule: %v", err)
		}
		if _, err := loadGlobalRule(); err == nil {
			t.Fatal("expected unmarshal error for invalid global rule, got nil")
		}
	})

	t.Run("valid file returns rule", func(t *testing.T) {
		home := t.TempDir()
		setTestHome(t, home)
		path := globalRulePath(home)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir parent: %v", err)
		}
		if err := os.WriteFile(path, []byte(`{"rules":[{"rule":"global rule"}]}`), 0o644); err != nil {
			t.Fatalf("write valid rule: %v", err)
		}
		pr, err := loadGlobalRule()
		if err != nil {
			t.Fatalf("loadGlobalRule: %v", err)
		}
		if pr == nil || len(pr.Rules) != 1 || pr.Rules[0].Rule != "global rule" {
			t.Errorf("unexpected rule: %+v", pr)
		}
	})
}

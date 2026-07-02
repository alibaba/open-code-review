package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		{"src/main/resources/mapper/usermapper.xml", "SQL Logic Error Detection"},
		{"src/main/resources/dao/userdao.xml", "SQL Logic Error Detection"},
		{"pom.xml", "snapshot"},
		{"submodule/pom.xml", "snapshot"},
		{"src/main/resources/application.properties", "Configuration Error Detection"},
		{"frontend/package.json", "latest"},
		{"config/app.yaml", "yaml-key"},
		{"deploy/values.yml", "yaml-key"},
		{"src/components/app.tsx", "React"},
		{"lib/utils.ts", "TypeScript"},
		{"app.kt", "Null Safety"},
		{"src/main/handler.cpp", "Smart Pointer"},
		{"driver.c", "malloc"},
		{"pages/Index.ets", "State Decorator"},
		{"components/Button.ets", "State Decorator"},
		{"entry/src/main/module.json5", "json-key"},
		{"entry/oh-package.json5", "json-key"},
		{"src/lib.rs", "Ownership and Lifetime Correctness"},
		{"crates/service/src/main.rs", "Unsafe Code Boundaries"},
		{"crates/service/Cargo.toml", "Cargo Manifest Hygiene"},
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
		"internal/agent/agent.go",
		"scripts/deploy.py",
		"ios/ViewController.m",
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
			{Pattern: "**/*.java", Rule: "java-rule"},
			{Pattern: "**/Cargo.toml", Rule: "cargo-rule"},
		},
	}

	got := rule.Resolve("Foo.Java")
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
	t.Setenv("HOME", t.TempDir())
	resolver, _, err := NewResolver(t.TempDir(), "")

	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	got := resolver.Resolve("src/main.java")
	if !strings.Contains(got, "Logic Error Detection") {
		t.Errorf("expected system default java rule, got %q", truncate(got, 80))
	}
}

func TestNewResolver_ProjectFileMissing(t *testing.T) {
	resolver, _, err := NewResolver(t.TempDir(), "")

	if err != nil {
		t.Fatalf("NewResolver should not fail when project rule is missing: %v", err)
	}
	got := resolver.Resolve("readme.md")
	if got == "" {
		t.Errorf("expected non-empty default rule")
	}
}

func TestNewResolver_ProjectRuleHighestPriority(t *testing.T) {
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
	t.Setenv("HOME", t.TempDir())

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

	resolver, _, err := NewResolver(dir, "")
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

	resolver, _, err := NewResolver(dir, "")
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	got := resolver.Resolve("other/main.go")
	if !strings.Contains(got, "Correctness") {
		t.Errorf("expected system default rule, got %q", truncate(got, 80))
	}
}

func TestNewResolver_CustomRuleOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	customRule := `{"rules":[{"path":"**/*.go","rule":"custom-go-rule"}]}`
	customPath := filepath.Join(dir, "custom_rules.json")
	if err := os.WriteFile(customPath, []byte(customRule), 0o644); err != nil {
		t.Fatalf("write custom rule: %v", err)
	}

	resolver, _, err := NewResolver(t.TempDir(), customPath)
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
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ruleJSON := `{"rules":[{"path":"**/*.go","rule":""},{"path":"internal/**/*.go","rule":"second-rule"}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	resolver, _, err := NewResolver(dir, "")
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
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ruleJSON := `{"rules":[{"path":"**/*.go","rule":"","merge_system_rule":true}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	resolver, _, err := NewResolver(dir, "")
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
	t.Setenv("HOME", t.TempDir())

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

	resolver, _, err := NewResolver(dir, "")
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	got := resolver.Resolve("main.go")
	if got != "project-go-rule" {
		t.Fatalf("expected only project rule when merge is disabled, got %q", got)
	}
}

func TestNewResolver_ProjectRuleMergesSystemRule(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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

	resolver, _, err := NewResolver(dir, "")
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
	t.Setenv("HOME", t.TempDir())

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

	resolver, _, err := NewResolver(repoDir, customPath)
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

	resolver, _, err := NewResolver(repoDir, customPath)
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

	_, _, err := NewResolver(dir, "")
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
		Include: []string{"src/**/*.java"},
		Exclude: []string{"**/generated/**"},
	}

	if !f.IsUserIncluded("SRC/Main/Foo.Java") {
		t.Errorf("expected case-insensitive include match")
	}
	if !f.IsUserExcluded("SRC/Generated/Api.java") {
		t.Errorf("expected case-insensitive exclude match")
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

	_, filter, err := NewResolver(repoDir, "")
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
	_, filter, err := NewResolver(t.TempDir(), "")
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

	_, filter, err := NewResolver(repoDir, customPath)
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

	_, filter, err := NewResolver(repoDir, customPath)
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
	resolver, _, err := NewResolver(t.TempDir(), "")
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
	t.Setenv("HOME", t.TempDir())
	resolver, _, err := NewResolver(t.TempDir(), "")
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

func TestResolveDetail_ProjectOverridesSystem(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	ocrDir := filepath.Join(dir, ".opencodereview")
	if err := os.MkdirAll(ocrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ruleJSON := `{"rules":[{"path":"src/**/*.java","rule":"project-java-rule"}]}`
	if err := os.WriteFile(filepath.Join(ocrDir, "rule.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	resolver, _, err := NewResolver(dir, "")
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
	t.Setenv("HOME", t.TempDir())

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

	resolver, _, err := NewResolver(dir, "")
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

	resolver, _, err := NewResolver(repoDir, customPath)
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

	resolver, _, err := NewResolver(dir, "")
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	tests := []struct {
		path string
		want string
	}{
		{"src/main/foo.java", "jvm-rule"},
		{"src/main/bar.kt", "jvm-rule"},
		{"src/main/baz.go", "Correctness"},
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
	resolveRuleEntries(entries, dir)

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
	resolveRuleEntries(entries, dir)

	if entries[0].Rule != "security.md\nBut this is multi-line\nso it should stay inline" {
		t.Errorf("multi-line rule should stay inline, got %q", entries[0].Rule)
	}
}

func TestResolveRuleEntries_MissingFile(t *testing.T) {
	dir := t.TempDir()

	entries := []ProjectRuleEntry{
		{Path: "**/*.xml", Rule: "nonexistent.md"},
	}
	resolveRuleEntries(entries, dir)

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
	resolveRuleEntries(entries, "/some/other/repo")

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
	resolveRuleEntries(entries, dir)

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
	resolveRuleEntries(entries, repoDir)

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
	resolveRuleEntries(entries, "/tmp")

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
		t.Fatal(err)
	}

	entries := []ProjectRuleEntry{
		{Path: "**/*.go", Rule: "evil.md"},
	}
	resolveRuleEntries(entries, dir)
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
	resolveRuleEntries(entries, dir)

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
	resolveRuleEntries(entries, dir)

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
	resolveRuleEntries(entries, dir)

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

	content, err := readRuleFileSafe(f)
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

	_, err := readRuleFileSafe(f)
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

	_, err := readRuleFileSafe(f)
	if err == nil {
		t.Fatal("expected error for oversized file")
	}
}

func TestReadRuleFileSafe_Missing(t *testing.T) {
	_, err := readRuleFileSafe("/nonexistent/path.md")
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
	resolveRuleEntries(entries, dir)

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
	resolveRuleEntries(entries, "")

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
	resolveRuleEntries(entries, "")

	if entries[0].Rule != "absolute content" {
		t.Errorf("absolute path with empty repoDir should work, got %q", entries[0].Rule)
	}
}

func TestResolveRuleEntries_GlobalRuleFileResolution(t *testing.T) {
	// Simulate loadGlobalRule: repoDir = filepath.Dir(~/.opencodereview/rule.json)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

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
	resolveRuleEntries(entries, globalRuleDir)

	if entries[0].Rule != "global reusable rule" {
		t.Errorf("global rule file should be resolved, got %q", entries[0].Rule)
	}
}

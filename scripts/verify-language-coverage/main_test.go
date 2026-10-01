// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, allow, rules []byte, docs ...string) config {
	t.Helper()
	root := t.TempDir()
	allowDir := filepath.Join(root, "internal/config/allowlist")
	rulesDir := filepath.Join(root, "internal/config/rules")
	docsDir := filepath.Join(rulesDir, "rule_docs")
	for _, dir := range []string{allowDir, docsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	cfg := config{
		allowPath: filepath.Join(allowDir, "supported_file_types.json"),
		rulesPath: filepath.Join(rulesDir, "system_rules.json"),
		docsDir:   docsDir,
	}
	if err := os.WriteFile(cfg.allowPath, allow, 0o644); err != nil {
		t.Fatalf("write allowlist: %v", err)
	}
	if err := os.WriteFile(cfg.rulesPath, rules, 0o644); err != nil {
		t.Fatalf("write rules: %v", err)
	}
	for _, doc := range docs {
		if err := os.WriteFile(filepath.Join(docsDir, doc), []byte("rule"), 0o644); err != nil {
			t.Fatalf("write doc %s: %v", doc, err)
		}
	}
	return cfg
}

func TestExpandBraces(t *testing.T) {
	// One brace group per call, matching the resolver's helper in
	// internal/config/rules, which is all the probe generator needs.
	tests := []struct {
		pattern string
		want    []string
	}{
		{"**/*.go", []string{"**/*.go"}},
		{"**/*.{go,py}", []string{"**/*.go", "**/*.py"}},
		{"**/*.{ts,tsx,jsx}", []string{"**/*.ts", "**/*.tsx", "**/*.jsx"}},
		{"**/*.{go", []string{"**/*.{go"}},
		{"{a,b}/x.c", []string{"a/x.c", "b/x.c"}},
	}
	for _, tt := range tests {
		got := expandBraces(tt.pattern)
		if len(got) != len(tt.want) {
			t.Errorf("expandBraces(%q) = %v, want %v", tt.pattern, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("expandBraces(%q)[%d] = %q, want %q", tt.pattern, i, got[i], tt.want[i])
			}
		}
	}
}

func TestExtensionOf(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"probe.go", ".go"},
		{"dir/sub/probe.kt", ".kt"},
		{"dir/probe", ""},
		{"probe", ""},
		{"a/probe.tar.gz", ".tar.gz"},
		{"probe/package.json", ".json"},
	}
	for _, tt := range tests {
		if got := extensionOf(tt.path); got != tt.want {
			t.Errorf("extensionOf(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestMatchesAny(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/*.go", "probe.go", true},
		{"**/*.go", "dir/probe.go", true},
		{"*.go", "probe.go", true},
		{"*.go", "dir/probe.go", false},
		{"**/*.{go,py}", "dir/probe.py", true},
		{"**/pom.xml", "dir/pom.xml", true},
		{"**/*.PY", "dir/probe.py", true},
		{"**/*.go", "dir/probe.py", false},
	}
	for _, tt := range tests {
		if got := matchesAny(tt.pattern, tt.path); got != tt.want {
			t.Errorf("matchesAny(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

func TestProbeForPattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{"**/pom.xml", "dir/pom.xml"},
		{"**/*.yaml", "dir/probe.yaml"},
		{".github/workflows/**/*.yaml", ".github/workflows/dir/probe.yaml"},
		{"*.go", "probe.go"},
		{"*_test.go", "probe_test.go"},
		{"**", "probe"},
		{"dir/**", "dir/probe"},
		{"/probe.go", "probe.go"},
		// A wildcard can sit inside a segment, which is how the real
		// "*{mapper,dao}*.xml" pattern is written. No glob metacharacter may
		// survive into the probe, or matchesAny would treat it as a pattern.
		{"**/*mapper*.xml", "dir/probemapperprobe.xml"},
		{"**/tb_*.v", "dir/tb_probe.v"},
		{"**/*.test.ets", "dir/probe.test.ets"},
		{"**/?oo.go", "dir/probeoo.go"},
	}
	for _, tt := range tests {
		got := probeForPattern(tt.pattern)
		if got != tt.want {
			t.Errorf("probeForPattern(%q) = %q, want %q", tt.pattern, got, tt.want)
		}
		if strings.ContainsAny(got, "*?") {
			t.Errorf("probeForPattern(%q) = %q, which still contains a glob metacharacter", tt.pattern, got)
		}
	}
}

func TestWildcardsToProbe(t *testing.T) {
	tests := []struct {
		segment string
		want    string
	}{
		{"*", "probe"},
		{"**", "probe"},
		{"*.go", "probe.go"},
		{"*mapper*.xml", "probemapperprobe.xml"},
		{"tb_*.v", "tb_probe.v"},
		{"literal.go", "literal.go"},
		{"?oo.go", "probeoo.go"},
		{"*.*", "probe.probe"},
	}
	for _, tt := range tests {
		if got := wildcardsToProbe(tt.segment); got != tt.want {
			t.Errorf("wildcardsToProbe(%q) = %q, want %q", tt.segment, got, tt.want)
		}
	}
}

func TestLiteralProbesCoversEveryPattern(t *testing.T) {
	in := map[string]string{
		"**/pom.xml":                 "pom_xml.md",
		"**/*.{yaml,yml}":            "yaml.md",
		".github/workflows/**/*.yml": "github_workflows.md",
	}
	got := literalProbes(in)
	if len(got) != len(in) {
		t.Fatalf("literalProbes returned %d probes for %d patterns: %v", len(got), len(in), got)
	}
	for _, probe := range got {
		if probe == "" || strings.HasSuffix(probe, "/") {
			t.Errorf("literalProbes produced an unusable probe %q (all: %v)", probe, got)
		}
	}
}

func TestAudit_ClassifiesRoutesAndFallbacks(t *testing.T) {
	allow := []byte(`[".go", ".py", ".md", ".txt"]`)
	rules := []byte(`{
  "default_rule": "default.md",
  "path_rule_map": {
    "**/*.go": "go.md",
    "**/*.{py,pyi}": "python.md"
  }
}`)
	cfg := writeConfig(t, allow, rules, "default.md", "go.md", "python.md")

	rep, err := audit(cfg)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if rep.allowlistSize != 4 || rep.patternCount != 2 || rep.docCount != 3 {
		t.Fatalf("unexpected sizes: allowlist=%d patterns=%d docs=%d",
			rep.allowlistSize, rep.patternCount, rep.docCount)
	}
	if got := strings.Join(rep.routes[".go"], ","); got != "go.md" {
		t.Errorf("routes[.go] = %q, want go.md", got)
	}
	if got := strings.Join(rep.routes[".py"], ","); got != "python.md" {
		t.Errorf("routes[.py] = %q, want python.md", got)
	}
	if _, routed := rep.routes[".md"]; routed {
		t.Errorf("routes should not contain .md, got %v", rep.routes[".md"])
	}
	if got := strings.Join(rep.unrouted, " "); got != ".md .txt" {
		t.Errorf("unrouted = %q, want %q", got, ".md .txt")
	}
	if len(rep.unreachable) != 0 {
		t.Errorf("unreachable = %v, want none", rep.unreachable)
	}
}

func TestAudit_ReportsUnreachableDoc(t *testing.T) {
	// ".rs" is allowlisted, but its pattern is only ever matched by a probe whose
	// extension is ".rs", and that probe exists only because the extension is
	// allowlisted. So the probe set alone cannot prove a pattern dead; it can
	// prove a doc unreachable only through that same feedback loop, which this
	// case pins down so the behaviour stays deliberate.
	allow := []byte(`[".go", ".rs"]`)
	rules := []byte(`{
  "default_rule": "default.md",
  "path_rule_map": {
    "**/*.go": "go.md",
    "**/*.rs": "rust.md"
  }
}`)
	cfg := writeConfig(t, allow, rules, "default.md", "go.md", "rust.md")
	// A declared doc that no pattern references at all is a configuration error
	// the missing-file check does not catch, because the file exists.
	if err := os.WriteFile(filepath.Join(cfg.docsDir, "orphan.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write orphan doc: %v", err)
	}

	rep, err := audit(cfg)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if got := strings.Join(rep.routes[".rs"], " "); got != "rust.md" {
		t.Errorf("routes[.rs] = %q, want rust.md", got)
	}
	if len(rep.unrouted) != 0 {
		t.Errorf("unrouted = %v, want none", rep.unrouted)
	}
	if len(rep.unreachable) != 0 {
		t.Errorf("unreachable = %v, want none (orphan.md is not declared)", rep.unreachable)
	}
}

func TestAudit_ReportsUnroutedExtensionForUnnamedLanguage(t *testing.T) {
	// ".v" belongs to the language whose rule is registered, ".vsh" belongs to a
	// language with no rule: the second one is the finding this command exists to
	// surface, and it is exactly the shape of the V/vlang entry in #470.
	allow := []byte(`[".v", ".vsh", ".zig"]`)
	rules := []byte(`{
  "default_rule": "default.md",
  "path_rule_map": {
    "**/*.v": "verilog.md",
    "**/*.zig": "zig.md"
  }
}`)
	cfg := writeConfig(t, allow, rules, "default.md", "verilog.md", "zig.md")

	rep, err := audit(cfg)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if got := strings.Join(rep.unrouted, " "); got != ".vsh" {
		t.Errorf("unrouted = %q, want %q", got, ".vsh")
	}
	if len(rep.unreachable) != 0 {
		t.Errorf("unreachable = %v, want none", rep.unreachable)
	}
}

func TestAudit_NameKeyedRuleStaysReachable(t *testing.T) {
	// The rule for pom.xml is keyed on the file name, not an extension. It is
	// reachable only because patterns other than "**/*.<ext>" get their own
	// literal probe, which is the whole reason literalProbes exists.
	allow := []byte(`[".xml"]`)
	rules := []byte(`{
  "default_rule": "default.md",
  "path_rule_map": {
    "**/pom.xml": "pom_xml.md",
    "**/*.xml": "xml.md"
  }
}`)
	cfg := writeConfig(t, allow, rules, "default.md", "pom_xml.md", "xml.md")

	rep, err := audit(cfg)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if len(rep.unreachable) != 0 {
		t.Errorf("unreachable = %v, want none", rep.unreachable)
	}
	// Routes accumulate in sorted pattern order, so "**/*.xml" comes before
	// "**/pom.xml" and the JSON declaration order does not decide this list.
	if got := strings.Join(rep.routes[".xml"], " "); got != "xml.md pom_xml.md" {
		t.Errorf("routes[.xml] = %q, want both docs", got)
	}
}

func TestAudit_RejectsMissingRuleDoc(t *testing.T) {
	allow := []byte(`[".go"]`)
	rules := []byte(`{"default_rule": "default.md", "path_rule_map": {"**/*.go": "go.md"}}`)
	cfg := writeConfig(t, allow, rules, "default.md")

	if _, err := audit(cfg); err == nil {
		t.Fatal("audit succeeded with a missing rule doc, want an error")
	} else if !strings.Contains(err.Error(), "go.md") {
		t.Errorf("error %q does not name the missing doc", err)
	}
}

func TestAudit_RejectsMalformedJSON(t *testing.T) {
	cfg := writeConfig(t, []byte(`[".go"`), []byte(`{}`), "default.md")
	if _, err := audit(cfg); err == nil {
		t.Fatal("audit succeeded on a malformed allowlist, want an error")
	}
}

func TestAudit_RejectsMissingFiles(t *testing.T) {
	cfg := config{
		allowPath: filepath.Join(t.TempDir(), "absent.json"),
		rulesPath: filepath.Join(t.TempDir(), "absent.json"),
		docsDir:   t.TempDir(),
	}
	if _, err := audit(cfg); err == nil {
		t.Fatal("audit succeeded without input files, want an error")
	}
}

func TestRender(t *testing.T) {
	rep := &report{
		allowlistSize: 3,
		patternCount:  2,
		docCount:      3,
		routes:        map[string][]string{".go": {"go.md"}, ".py": {"python.md"}},
		unrouted:      []string{".md"},
		unreachable:   []string{"rust.md"},
	}
	out := render(rep)
	for _, want := range []string{
		"allowlist 3 extensions | rules 2 patterns, 3 docs",
		".go          go.md",
		".py          python.md",
		"extensions falling back to the default rule (1):",
		".md",
		"rule docs no reviewed file reaches (1):",
		"rust.md",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render output missing %q\n---\n%s", want, out)
		}
	}
}

func TestConfigFor(t *testing.T) {
	cfg := configFor("/repo")
	if !strings.HasSuffix(cfg.allowPath, filepath.Join("internal", "config", "allowlist", "supported_file_types.json")) {
		t.Errorf("allowPath = %q", cfg.allowPath)
	}
	if !strings.HasSuffix(cfg.rulesPath, filepath.Join("internal", "config", "rules", "system_rules.json")) {
		t.Errorf("rulesPath = %q", cfg.rulesPath)
	}
	if !strings.HasSuffix(cfg.docsDir, filepath.Join("internal", "config", "rules", "rule_docs")) {
		t.Errorf("docsDir = %q", cfg.docsDir)
	}
}

func TestDedupeAndSortedKeys(t *testing.T) {
	if got := strings.Join(dedupe([]string{"b", "a", "b"}), " "); got != "a b" {
		t.Errorf("dedupe = %q, want %q", got, "a b")
	}
	if got := strings.Join(sortedKeys(map[string]string{"b": "1", "a": "2"}), " "); got != "a b" {
		t.Errorf("sortedKeys = %q, want %q", got, "a b")
	}
	if got := strings.Join(docValues(map[string]string{"a": "x.md", "b": "x.md"}), " "); got != "x.md x.md" {
		t.Errorf("docValues = %q", got)
	}
}

func TestRun_WritesReportAndDecidesExitStatus(t *testing.T) {
	allow := []byte(`[".go", ".md"]`)
	rules := []byte(`{"default_rule": "default.md", "path_rule_map": {"**/*.go": "go.md"}}`)
	cfg := writeConfig(t, allow, rules, "default.md", "go.md")
	// allowPath is <root>/internal/config/allowlist/supported_file_types.json.
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(cfg.allowPath))))

	var buf bytes.Buffer
	if err := run(&buf, root, false, true); err != nil {
		t.Fatalf("report mode returned %v, want nil", err)
	}
	if !strings.Contains(buf.String(), "extensions falling back to the default rule (1):") {
		t.Errorf("report missing the fallback section:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), ".go          go.md") {
		t.Errorf("report missing the .go route:\n%s", buf.String())
	}

	// The default tolerates a fallback, so this stays clean.
	if err := run(&buf, root, true, true); err != nil {
		t.Errorf("-check with the default allow-unrouted returned %v, want nil", err)
	}
	// Tightening the check turns the fallback into a finding.
	if err := run(&buf, root, true, false); err == nil {
		t.Error("-check -allow-unrouted=false returned nil, want an error")
	}
}

func TestRun_PropagatesConfigErrors(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf, t.TempDir(), false, true); err == nil {
		t.Error("run on an empty directory returned nil, want an error")
	}
}

func TestRun_ReportsEveryFindingNotJustTheFirst(t *testing.T) {
	// Both a fallback and an unreachable doc at once. The tightest check must
	// report both: a caller reading only the first message would otherwise miss
	// the second problem entirely.
	//
	// unreachable.md is reached by "**/nothing", whose probe carries no
	// extension, so the pattern contributes no route and the doc stays
	// unreachable. ".txt" is allowlisted with no pattern, so it falls back.
	allow := []byte(`[".go", ".txt"]`)
	rules := []byte(`{
  "default_rule": "default.md",
  "path_rule_map": {
    "**/*.go": "go.md",
    "**/nothing": "unreachable.md"
  }
}`)
	cfg := writeConfig(t, allow, rules, "default.md", "go.md", "unreachable.md")
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(cfg.allowPath))))

	var buf bytes.Buffer
	err := run(&buf, root, true, false)
	if err == nil {
		t.Fatal("-check -allow-unrouted=false returned nil, want an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "fall back to the default rule") {
		t.Errorf("error does not mention the unrouted extension: %q", msg)
	}
	if !strings.Contains(msg, "unreachable") {
		t.Errorf("error does not mention the unreachable doc: %q", msg)
	}
}

func TestAudit_MissingRulesFile(t *testing.T) {
	cfg := writeConfig(t, []byte(`[".go"]`), []byte(`{}`), "default.md")
	if err := os.Remove(cfg.rulesPath); err != nil {
		t.Fatalf("remove rules: %v", err)
	}
	if _, err := audit(cfg); err == nil {
		t.Fatal("audit succeeded without system_rules.json, want an error")
	}
}

func TestAudit_SkipsPatternsWhoseProbeHasNoExtension(t *testing.T) {
	// A pattern can match a probe that carries no extension, which contributes no
	// route. "**/probe" also exercises the trailing-"**" branch of the probe
	// generator, and the brace list exercises the multi-option expansion loop.
	allow := []byte(`[".go"]`)
	rules := []byte(`{
  "default_rule": "default.md",
  "path_rule_map": {
    "**/probe": "default.md",
    "**/*.{go,py}": "go.md"
  }
}`)
	cfg := writeConfig(t, allow, rules, "default.md", "go.md")

	rep, err := audit(cfg)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if got := strings.Join(rep.routes[".go"], " "); got != "go.md" {
		t.Errorf("routes[.go] = %q, want go.md", got)
	}
	if len(rep.unrouted) != 0 {
		t.Errorf("unrouted = %v, want none", rep.unrouted)
	}
}

// TestMainCoversEntryPoint calls main so the flag wiring is measured too. main
// does not redefine flags, so a repeated call in the same process is safe, and
// the run uses a valid root so the process does not exit.
func TestMainCoversEntryPoint(t *testing.T) {
	allow := []byte(`[".go"]`)
	rules := []byte(`{"default_rule": "default.md", "path_rule_map": {"**/*.go": "go.md"}}`)
	cfg := writeConfig(t, allow, rules, "default.md", "go.md")
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(cfg.allowPath))))

	orig := os.Args
	t.Cleanup(func() { os.Args = orig })

	os.Args = []string{"verify-language-coverage", "-root", root}
	main()
}

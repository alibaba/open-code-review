// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectRuleEntry_UnmarshalRuleForms(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantRule  string
		wantParts int
	}{
		{
			name:     "string inline",
			input:    `{"path":"**/*.go","rule":"Check nil handling"}`,
			wantRule: "Check nil handling",
		},
		{
			name:     "array inline",
			input:    `{"path":"**/*.go","rule":["First","Second"]}`,
			wantRule: "First\n\nSecond", wantParts: 2,
		},
		{
			name:     "array single element",
			input:    `{"path":"**/*.go","rule":["only.md"]}`,
			wantRule: "only.md", wantParts: 1,
		},
		{
			name:     "absent rule",
			input:    `{"path":"**/*.go"}`,
			wantRule: "",
		},
		{
			name:     "null rule",
			input:    `{"path":"**/*.go","rule":null}`,
			wantRule: "",
		},
		{
			name:     "empty array",
			input:    `{"path":"**/*.go","rule":[]}`,
			wantRule: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entry ProjectRuleEntry
			if err := json.Unmarshal([]byte(tt.input), &entry); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if entry.Rule != tt.wantRule {
				t.Errorf("Rule = %q, want %q", entry.Rule, tt.wantRule)
			}
			if len(entry.ruleParts) != tt.wantParts {
				t.Errorf("len(ruleParts) = %d, want %d", len(entry.ruleParts), tt.wantParts)
			}
		})
	}
}

func TestProjectRuleEntry_UnmarshalRejectsOtherRuleTypes(t *testing.T) {
	for _, input := range []string{
		`{"path":"**/*.go","rule":42}`,
		`{"path":"**/*.go","rule":["ok.md",7]}`,
		`{"path":"**/*.go","rule":{"text":"x"}}`,
	} {
		var entry ProjectRuleEntry
		if err := json.Unmarshal([]byte(input), &entry); err == nil {
			t.Errorf("expected error for %s", input)
		}
	}
}

// A null (or blank) element used to compose a spurious empty section into the
// prompt, because encoding/json decodes null as "". Reject it with the index so
// the config error points at the element to remove.
func TestProjectRuleEntry_UnmarshalRejectsBlankArrayPart(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "null element",
			input: `{"path":"**/*.go","rule":["rules.md",null]}`,
			want:  "rule[1] must not be empty",
		},
		{
			name:  "empty string element",
			input: `{"path":"**/*.go","rule":["","rules.md"]}`,
			want:  "rule[0] must not be empty",
		},
		{
			name:  "whitespace element",
			input: `{"path":"**/*.go","rule":["rules.md","   "]}`,
			want:  "rule[1] must not be empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entry ProjectRuleEntry
			err := json.Unmarshal([]byte(tt.input), &entry)
			if err == nil {
				t.Fatalf("expected an error for %s", tt.input)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

// Entries built in Go skip UnmarshalJSON, so resolveRuleParts filters blank
// parts itself instead of composing an empty section.
func TestResolveRuleEntries_ArraySkipsBlankParts(t *testing.T) {
	dir := t.TempDir()
	writeRuleFile(t, filepath.Join(dir, "present.md"), "PRESENT")

	entries := []ProjectRuleEntry{{
		Path:      "**/*.go",
		ruleParts: []string{"present.md", "", "   ", "INLINE"},
	}}
	resolveRuleEntries(entries, dir, "")

	if got, want := entries[0].Rule, "PRESENT\n\nINLINE"; got != want {
		t.Errorf("composed rule = %q, want %q", got, want)
	}
}

func TestResolveRuleEntries_ArrayResolvesPartsAndJoins(t *testing.T) {
	dir := t.TempDir()
	writeRuleFile(t, filepath.Join(dir, "sql.md"), "SQL RULES\n")
	writeRuleFile(t, filepath.Join(dir, "extra.md"), "EXTRA RULES\n")

	var pr ProjectRule
	jsonBody := `{"rules":[{"path":"**/*.sql","rule":["sql.md","Inline text","extra.md"]}]}`
	if err := json.Unmarshal([]byte(jsonBody), &pr); err != nil {
		t.Fatal(err)
	}
	resolveRuleEntries(pr.Rules, dir, "")

	want := "SQL RULES\n\nInline text\n\nEXTRA RULES"
	if got := pr.Rules[0].Rule; got != want {
		t.Errorf("composed rule = %q, want %q", got, want)
	}
	if len(pr.Rules[0].ruleParts) != 0 {
		t.Errorf("ruleParts should be cleared after resolution")
	}
}

func TestResolveRuleEntries_ArrayKeepsRemainingPartsWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	writeRuleFile(t, filepath.Join(dir, "present.md"), "PRESENT")

	var pr ProjectRule
	body := `{"rules":[{"path":"**/*.go","rule":["missing.md","still here","present.md"]}]}`
	if err := json.Unmarshal([]byte(body), &pr); err != nil {
		t.Fatal(err)
	}
	resolveRuleEntries(pr.Rules, dir, "")

	if got, want := pr.Rules[0].Rule, "still here\n\nPRESENT"; got != want {
		t.Errorf("composed rule = %q, want %q", got, want)
	}
}

func TestResolveRuleEntries_ArrayAllMissingIgnoresEntry(t *testing.T) {
	dir := t.TempDir()
	var pr ProjectRule
	body := `{"rules":[{"path":"**/*.go","rule":["missing-a.md","missing-b.md"]}]}`
	if err := json.Unmarshal([]byte(body), &pr); err != nil {
		t.Fatal(err)
	}
	resolveRuleEntries(pr.Rules, dir, "")
	if pr.Rules[0].Rule != "" {
		t.Errorf("expected empty rule, got %q", pr.Rules[0].Rule)
	}
}

func TestResolveRuleEntries_ArrayRespectsConfinement(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuleFile(t, filepath.Join(root, "outside.md"), "OUTSIDE")
	writeRuleFile(t, filepath.Join(repoDir, "inside.md"), "INSIDE")

	var pr ProjectRule
	body := `{"rules":[{"path":"**/*.go","rule":["../outside.md","inside.md"]}]}`
	if err := json.Unmarshal([]byte(body), &pr); err != nil {
		t.Fatal(err)
	}
	resolveRuleEntries(pr.Rules, repoDir, repoDir)

	if got, want := pr.Rules[0].Rule, "INSIDE"; got != want {
		t.Errorf("composed rule = %q, want %q", got, want)
	}
}

func TestResolveRuleEntries_ArrayTotalCap(t *testing.T) {
	dir := t.TempDir()
	part := strings.Repeat("a", 512*1024)
	writeRuleFile(t, filepath.Join(dir, "a.md"), part)
	writeRuleFile(t, filepath.Join(dir, "b.md"), part)

	var pr ProjectRule
	body := `{"rules":[{"path":"**/*.go","rule":["a.md","b.md"]}]}`
	if err := json.Unmarshal([]byte(body), &pr); err != nil {
		t.Fatal(err)
	}
	resolveRuleEntries(pr.Rules, dir, "")

	if got := len(pr.Rules[0].Rule); got != maxComposedRuleBytes {
		t.Errorf("composed length = %d, want %d", got, maxComposedRuleBytes)
	}
}

func TestLoadRuleFile_ArrayMergeSystemRule(t *testing.T) {
	dir := t.TempDir()
	writeRuleFile(t, filepath.Join(dir, "baseline.md"), "USER BASELINE")
	rulePath := filepath.Join(dir, "rule.json")
	body := `{
		"rules": [{
			"path": "**/*.go",
			"rule": ["baseline.md", "USER EXTRA"],
			"merge_system_rule": true
		}]
	}`
	if err := os.WriteFile(rulePath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver, _, err := NewResolver("", rulePath, ResolverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := resolver.Resolve("main.go")
	for _, want := range []string{"USER BASELINE", "USER EXTRA", "User-Specific Rules"} {
		if !strings.Contains(got, want) {
			t.Errorf("resolved rule missing %q:\n%s", want, got)
		}
	}
}

func TestLoadRuleFile_CanonicalConfigTracksArrayOrder(t *testing.T) {
	dir := t.TempDir()
	writeRuleFile(t, filepath.Join(dir, "one.md"), "ONE")
	writeRuleFile(t, filepath.Join(dir, "two.md"), "TWO")

	load := func(name, parts string) []string {
		path := filepath.Join(dir, name)
		body := `{"rules":[{"path":"**/*.go","rule":` + parts + `}]}`
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		resolver, _, err := NewResolver("", path, ResolverOptions{})
		if err != nil {
			t.Fatal(err)
		}
		composed, ok := resolver.(*composedResolver)
		if !ok {
			t.Fatalf("unexpected resolver type %T", resolver)
		}
		return composed.CanonicalConfig()
	}

	first := load("forward.json", `["one.md","two.md"]`)
	second := load("reverse.json", `["two.md","one.md"]`)
	if strings.Join(first, "\x00") == strings.Join(second, "\x00") {
		t.Fatal("canonical config must depend on array part order")
	}
}

func writeRuleFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

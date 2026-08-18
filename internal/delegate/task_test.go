// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package delegate

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTaskSpecJSONFields verifies the aggregated task document carries every
// field the host agent relies on, and that the JSON tags match the contract.
func TestTaskSpecJSONFields(t *testing.T) {
	spec := TaskSpec{
		SchemaVersion: TaskSchemaVersion,
		Repository:    "/home/dev/api-server",
		Scope: TaskScope{
			Mode:            "range",
			From:            "origin/main",
			To:              "HEAD",
			MergeBase:       "a1b2c3d",
			TotalFiles:      2,
			ReviewableCount: 1,
			ExcludedCount:   1,
			ReviewableFiles: []TaskFile{{Path: "internal/pay/charge.go", Status: "modified", Insertions: 42, Deletions: 8}},
			ExcludedFiles:   []TaskFile{{Path: "internal/pay/mock_test.go", Status: "modified", ExcludeReason: "test file"}},
		},
		Excludes: []string{"**/*_test.go"},
		Rules: []TaskRuleGroup{{
			GroupID: 0, Source: "system", Pattern: "**/*.go",
			Files: []string{"internal/pay/charge.go"},
			Rule:  "Error handling must wrap the underlying error with fmt.Errorf and add context.",
		}},
		Background:         "Payment channel integration; pay attention to concurrency safety.",
		Diffs:              []TaskDiff{{Path: "internal/pay/charge.go", Hunk: "@@ -120,7 +120,14 @@ func Charge"}},
		AcceptanceCriteria: DefaultAcceptanceCriteria(),
	}

	payload, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal task spec: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode task spec: %v", err)
	}

	checks := []struct {
		key   string
		field string
	}{
		{"schema_version", "2"},
		{"repository", "/home/dev/api-server"},
		{"excludes", ""},
		{"rules", ""},
		{"background", "Payment channel integration; pay attention to concurrency safety."},
		{"diffs", ""},
		{"acceptance_criteria", ""},
	}
	for _, c := range checks {
		if _, ok := decoded[c.key]; !ok {
			t.Errorf("task JSON missing top-level field %q", c.key)
		}
	}
	scope, ok := decoded["scope"].(map[string]any)
	if !ok {
		t.Fatalf("scope not an object: %#v", decoded["scope"])
	}
	for _, k := range []string{"mode", "from", "to", "merge_base", "reviewable_files", "excluded_files"} {
		if _, ok := scope[k]; !ok {
			t.Errorf("scope JSON missing field %q", k)
		}
	}
	// repository must not be duplicated inside scope.
	if _, ok := scope["repository"]; ok {
		t.Error("repository must not appear inside scope; it lives at the top level")
	}
}

// TestTaskMarkdownFaithful ensures the Markdown variant exposes the same fields
// as the JSON (the gap flagged during review): repository, merge_base, excluded
// file paths, and rule files must all be present.
func TestTaskMarkdownFaithful(t *testing.T) {
	spec := TaskSpec{
		SchemaVersion: TaskSchemaVersion,
		Repository:    "/home/dev/api-server",
		Scope: TaskScope{
			Mode:      "range",
			From:      "origin/main",
			To:        "HEAD",
			MergeBase: "a1b2c3d",
			ReviewableFiles: []TaskFile{
				{Path: "internal/pay/charge.go", Status: "modified", Insertions: 42, Deletions: 8},
			},
			ExcludedFiles: []TaskFile{
				{Path: "internal/pay/mock_test.go", Status: "modified", ExcludeReason: "test file"},
			},
		},
		Rules: []TaskRuleGroup{{
			GroupID: 0, Source: "system", Pattern: "**/*.go",
			Files: []string{"internal/pay/charge.go"},
			Rule:  "Error handling must wrap the underlying error with fmt.Errorf and add context.",
		}},
		Diffs: []TaskDiff{{Path: "internal/pay/charge.go", Hunk: "@@ -120,7 +120,14 @@ func Charge"}},
	}

	md := TaskMarkdown(spec)
	wants := []string{
		"/home/dev/api-server",               // repository
		"merge_base: a1b2c3d",                // merge_base
		"internal/pay/mock_test.go",          // excluded file path (not just reason)
		"applies to: internal/pay/charge.go", // rule files
		"internal/pay/charge.go",             // diff path
		"@@ -120,7 +120,14",                  // diff hunk
	}
	for _, w := range wants {
		if !strings.Contains(md, w) {
			t.Errorf("markdown missing %q\n---\n%s", w, md)
		}
	}
}

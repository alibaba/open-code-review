// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docLocales are the translations of review-rules.md. Every one of them carries
// the same rule table, so a new language has to be added to all five.
var docLocales = []string{"en", "ja", "ko", "ru", "zh"}

// docTableRow matches one row of the rule table and captures the glob in its
// first column: "| `**/*.go` | `go.md` — Go source. |".
var docTableRow = regexp.MustCompile("(?m)^\\|\\s*`([^`]+)`\\s*\\|")

// TestRuleTableDocumentsEveryPattern keeps pages/.../review-rules.md honest
// about path_rule_map.
//
// The table is the only place a user can find out which rule doc a file of
// theirs will be reviewed against, and it is maintained by hand: #1448 added
// F# to the JSON, the tests and all five translations in one commit, which is
// the shape that works. Nothing enforced it, so eight languages -- pug, nix,
// haskell, nim, swift, zig, ocaml and reason -- had reached the resolver
// without ever reaching the docs.
//
// Failing here is cheap to fix and the message says exactly which rows to add.
func TestRuleTableDocumentsEveryPattern(t *testing.T) {
	patterns := patternsFromJSON(t)

	for _, locale := range docLocales {
		t.Run(locale, func(t *testing.T) {
			documented := patternsFromDoc(t, locale)

			var missing []string
			for _, pattern := range patterns {
				if !documented[pattern] {
					missing = append(missing, pattern)
				}
			}
			if len(missing) > 0 {
				t.Errorf("review-rules.md (%s) does not list %d pattern(s) from system_rules.json:\n  %s",
					locale, len(missing), strings.Join(missing, "\n  "))
			}

			inJSON := make(map[string]bool, len(patterns))
			for _, pattern := range patterns {
				inJSON[pattern] = true
			}
			var stale []string
			for pattern := range documented {
				// The table also documents a handful of non-glob rows, such as
				// the default rule. Only globs can be compared with the map.
				if strings.Contains(pattern, "*") && !inJSON[pattern] {
					stale = append(stale, pattern)
				}
			}
			if len(stale) > 0 {
				t.Errorf("review-rules.md (%s) lists %d pattern(s) that system_rules.json no longer has:\n  %s",
					locale, len(stale), strings.Join(stale, "\n  "))
			}
		})
	}
}

// patternsFromJSON reads the glob keys of path_rule_map in declaration order.
// LoadDefault is not usable here: it replaces each rule name with the rule
// file's contents, and this test is about the names.
func patternsFromJSON(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("system_rules.json")
	if err != nil {
		t.Fatalf("read system_rules.json: %v", err)
	}
	var rule SystemRule
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal system_rules.json: %v", err)
	}
	if len(rule.PathRules) == 0 {
		t.Fatal("system_rules.json has no path rules, so this test would pass vacuously")
	}
	patterns := make([]string, 0, len(rule.PathRules))
	for _, pathRule := range rule.PathRules {
		patterns = append(patterns, pathRule.Pattern)
	}
	return patterns
}

// patternsFromDoc reads the first column of every table row in one locale's
// review-rules.md.
func patternsFromDoc(t *testing.T, locale string) map[string]bool {
	t.Helper()
	// internal/config/rules -> repository root.
	path := filepath.Join("..", "..", "..", "pages", "src", "content", "docs", locale, "review-rules.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	documented := map[string]bool{}
	for _, match := range docTableRow.FindAllStringSubmatch(string(data), -1) {
		documented[match[1]] = true
	}
	if len(documented) == 0 {
		t.Fatalf("no table rows found in %s, so this test would pass vacuously", path)
	}
	return documented
}

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

// reviewRulesLocales are the maintained translations of the review-rules guide
// under pages/src/content/docs. The pattern table on that page is the published
// form of system_rules.json: it tells a user which built-in rule document a
// file resolves to, so every locale has to carry the same rows.
var reviewRulesLocales = []string{"en", "zh", "ja", "ko", "ru"}

// reviewRulesTableRow matches a mapping-table row of that page and captures the
// glob column and the rule-doc column. The fallback row spells its glob column
// as "_(fallback)_" rather than a backticked pattern, so it is deliberately not
// matched: it has no counterpart in system_rules.json.
//
// The rule-doc capture is greedy on purpose, ending at the last "|" of the line,
// so that a row carrying a column-alignment padding is still captured whole.
var reviewRulesTableRow = regexp.MustCompile("^\\|\\s*`([^`]+)`\\s*\\|(.*)\\|\\s*$")

// ruleDocNameRE finds the rule-document filenames a row references. A row may
// name more than one (the ".m" row names matlab.md and points at objc.md for
// the content sniff), so membership, not equality, is what the caller checks.
var ruleDocNameRE = regexp.MustCompile("`([A-Za-z0-9_]+\\.md)`")

// embeddedPathRuleMap returns the embedded system_rules.json path_rule_map as a
// glob → rule-doc map. A plain map decode is enough: the check below compares
// the table as a set, and key order is asserted by the resolver's own tests.
func embeddedPathRuleMap(t *testing.T) map[string]string {
	t.Helper()
	data, err := rulesFS.ReadFile("system_rules.json")
	if err != nil {
		t.Fatalf("read embedded system_rules.json: %v", err)
	}
	var raw struct {
		PathRuleMap map[string]string `json:"path_rule_map"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal system_rules.json: %v", err)
	}
	if len(raw.PathRuleMap) == 0 {
		t.Fatal("system_rules.json has an empty path_rule_map")
	}
	return raw.PathRuleMap
}

// TestReviewRulesGuideListsEverySystemRulePattern keeps the review-rules guide's
// pattern table in step with the embedded mapping it documents.
//
// The table is hand-maintained across five locales, and rules were added to
// system_rules.json without a matching row more than once, which leaves a
// built-in language invisible in the guide while it still resolves to its own
// rule document. Three things are asserted per locale: every glob in the
// embedded map is listed, no listed glob is absent from it, and the rule
// document named on the row is the one the map points at (a translated row is
// just as able to name the wrong document as to go missing).
//
// Relative row order is not asserted: the page presents the rows in matching
// order, but the patterns are disjoint, so a reordered row cannot change which
// rule a file resolves to.
func TestReviewRulesGuideListsEverySystemRulePattern(t *testing.T) {
	mappings := embeddedPathRuleMap(t)
	for _, locale := range reviewRulesLocales {
		t.Run(locale, func(t *testing.T) {
			page := filepath.Join("..", "..", "..", "pages", "src", "content", "docs", locale, "review-rules.md")
			body, err := os.ReadFile(page)
			if err != nil {
				t.Fatalf("read %s: %v", page, err)
			}
			listed := make(map[string]string)
			for _, line := range strings.Split(string(body), "\n") {
				m := reviewRulesTableRow.FindStringSubmatch(line)
				if m == nil {
					continue
				}
				listed[m[1]] = m[2]
			}
			if len(listed) == 0 {
				t.Fatalf("%s lists no pattern rows", page)
			}

			for glob, doc := range mappings {
				cell, ok := listed[glob]
				if !ok {
					t.Errorf("%s does not list %q (%s)", page, glob, doc)
					continue
				}
				var names []string
				for _, name := range ruleDocNameRE.FindAllStringSubmatch(cell, -1) {
					names = append(names, name[1])
				}
				found := false
				for _, name := range names {
					if name == doc {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s lists %q as %q, want %s", page, glob, strings.TrimSpace(cell), doc)
				}
			}

			for glob := range listed {
				if _, ok := mappings[glob]; !ok {
					t.Errorf("%s lists %q, which is not in system_rules.json", page, glob)
				}
			}
		})
	}
}

// Package delegate provides the deterministic "spec" generation for delegation mode,
// where OCR produces review specifications without calling any LLM.
package delegate

import (
	"github.com/open-code-review/open-code-review/internal/config/rules"
)

// RuleGroup clusters files that share the same resolved rule text.
type RuleGroup struct {
	ID      int
	Source  string // "custom" | "project" | "global" | "system"
	Pattern string // glob pattern that matched, or "default"
	Text    string // resolved rule content
	Files   []string
}

// GroupRules groups the given file paths by their resolved rule content.
// Files with identical rule text are placed in the same group.
func GroupRules(resolver rules.Resolver, paths []string) []RuleGroup {
	dr, hasDetail := resolver.(rules.DetailResolver)

	keyIndex := make(map[string]int) // text -> group index
	var groups []RuleGroup

	for _, path := range paths {
		var source, pattern, text string
		if hasDetail {
			detail := dr.ResolveDetail(path)
			source = detail.Source
			pattern = detail.Pattern
			text = detail.Rule
		} else {
			text = resolver.Resolve(path)
			source = "system"
			pattern = "default"
		}

		idx, exists := keyIndex[text]
		if !exists {
			idx = len(groups)
			keyIndex[text] = idx
			groups = append(groups, RuleGroup{
				ID:      idx + 1,
				Source:  source,
				Pattern: pattern,
				Text:    text,
			})
		}
		groups[idx].Files = append(groups[idx].Files, path)
	}

	return groups
}

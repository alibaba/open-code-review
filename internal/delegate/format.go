package delegate

import (
	"fmt"
	"strings"
)

// RuleGroupsMarkdown renders rule groups into a markdown section.
func RuleGroupsMarkdown(groups []RuleGroup) string {
	var b strings.Builder
	for i, g := range groups {
		if i > 0 {
			b.WriteString("\n---\n\n")
		}
		fmt.Fprintf(&b, "### Rule Group %d: %s / %s\n\n", g.ID, g.Source, g.Pattern)
		b.WriteString("Applies to:\n")
		for _, f := range g.Files {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		b.WriteString("\n#### Content\n\n")
		b.WriteString(g.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

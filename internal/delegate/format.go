// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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

// TaskMarkdown renders a TaskSpec as a field-faithful Markdown document. Every
// non-omitted JSON field has a corresponding Markdown line, so a host agent that
// falls back to Markdown sees the same data as the JSON variant (no dropped
// fields). The JSON tags are the contract; this renderer must track them.
func TaskMarkdown(s TaskSpec) string {
	var b strings.Builder
	b.WriteString("# OCR Delegate Task\n\n")
	fmt.Fprintf(&b, "- schema_version: %s\n", s.SchemaVersion)

	// Header line: mode + refs.
	switch s.Scope.Mode {
	case "commit":
		fmt.Fprintf(&b, "- mode: commit (%s)\n", s.Scope.Commit)
	case "workspace":
		b.WriteString("- mode: workspace\n")
	default:
		from := s.Scope.From
		if from == "" {
			from = "origin/main"
		}
		to := s.Scope.To
		if to == "" {
			to = "HEAD"
		}
		fmt.Fprintf(&b, "- mode: range (%s..%s", from, to)
		if s.Scope.MergeBase != "" {
			fmt.Fprintf(&b, ", merge_base: %s", s.Scope.MergeBase)
		}
		b.WriteString(")\n")
	}
	if s.Repository != "" {
		fmt.Fprintf(&b, "- repository: %s\n", s.Repository)
	}
	b.WriteString("\n")

	b.WriteString("## Scope\n")
	fmt.Fprintf(&b, "- total_files: %d\n", s.Scope.TotalFiles)
	fmt.Fprintf(&b, "- reviewable_count: %d\n", s.Scope.ReviewableCount)
	fmt.Fprintf(&b, "- excluded_count: %d\n", s.Scope.ExcludedCount)
	fmt.Fprintf(&b, "- total_insertions: %d\n", s.Scope.TotalInsertions)
	fmt.Fprintf(&b, "- total_deletions: %d\n", s.Scope.TotalDeletions)
	fmt.Fprintf(&b, "### Reviewable (%d)\n", len(s.Scope.ReviewableFiles))
	for _, f := range s.Scope.ReviewableFiles {
		fmt.Fprintf(&b, "- %s  (%s, +%d / -%d)\n", f.Path, f.Status, f.Insertions, f.Deletions)
	}
	fmt.Fprintf(&b, "\n### Excluded (%d)\n", len(s.Scope.ExcludedFiles))
	for _, f := range s.Scope.ExcludedFiles {
		if f.ExcludeReason != "" {
			fmt.Fprintf(&b, "- %s  (%s, %s)\n", f.Path, f.Status, f.ExcludeReason)
		} else {
			fmt.Fprintf(&b, "- %s  (%s)\n", f.Path, f.Status)
		}
	}
	b.WriteString("\n")

	if len(s.Excludes) > 0 {
		b.WriteString("## Excludes\n")
		for _, e := range s.Excludes {
			fmt.Fprintf(&b, "- %s\n", e)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Rules\n")
	for _, g := range s.Rules {
		fmt.Fprintf(&b, "### Group %d\n", g.GroupID)
		fmt.Fprintf(&b, "- source: %s  pattern: %s\n", g.Source, g.Pattern)
		if len(g.Files) > 0 {
			b.WriteString("- applies to: " + strings.Join(g.Files, ", ") + "\n")
		}
		b.WriteString(g.Rule + "\n\n")
	}

	if s.Background != "" {
		b.WriteString("## Background\n")
		b.WriteString(s.Background + "\n\n")
	}

	b.WriteString("## Diffs\n")
	for _, d := range s.Diffs {
		fmt.Fprintf(&b, "### %s\n", d.Path)
		b.WriteString(d.Hunk + "\n\n")
	}

	b.WriteString("## Acceptance Criteria\n")
	for _, a := range s.AcceptanceCriteria {
		fmt.Fprintf(&b, "- %s\n", a)
	}
	b.WriteString("\n")

	return b.String()
}

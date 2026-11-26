// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"slices"
	"strings"

	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/model"
)

// A discussion can select which of several supported behaviors is actually
// used. Its original words identify the selection. Current code must still
// connect it to the reviewed change and demonstrate the resulting behavior.
const discussionReviewPrompt = `
For each listed (path, reference), first assess applicability and give its result in a separate top-level "applications" array, even when "checks" is empty. Read the dated discussion's original words. Separate a decision effective for this subject from a suggestion, an example, or a description of several supported modes. Check the reviewed entry point, surviving conditions, any replacement decision and contradictory evidence. A change to an implementation can violate an external agreement that still governs it. The existence of another implementation does not select that implementation.
Each application has "path", "reference", "decision_quote" (an exact clause from the original discussion), and "verdict" ("violated", "satisfied", "unrelated" or "unknown"). For a determined verdict supply "scope_path" and "scope_quote" (one exact current line connecting the changed operation to the subject or showing why it is unrelated). For "satisfied" or "violated", trace one concrete, discriminating input through the old and new code. Give "input", "expected", "before", and "after" for that same authorized condition. Include relevant non-default cases for which supported modes produce different results. State "unknown" for unresolved applicability or conflicting evidence.
If the patch introduces a failure under that condition, put a complete ordinary violation object, including its rule id, current source witness, changed line and actionable reason, in the application's "violation" field. The current witness may come from any relevant supplied rule. An additional defect in the same file can have its own application showing how it arises for an admitted input. Use the ordinary "checks" array for findings outside those discussions, direct computed property checks and proposed refutations. Never use a dated quote alone as proof of current program behavior.`

type discussionTarget struct {
	Path      string `json:"path"`
	Reference string `json:"reference"`
	Conflict  bool   `json:"unresolved_conflict,omitempty"`
	sources   []knowledge.Citation
}

func reviewDiscussions(rules knowledge.Snapshot, changes []model.Diff) []discussionTarget {
	var targets []discussionTarget
	for _, f := range rules {
		switch f.ContextStatus() {
		case knowledge.StatusCandidate, knowledge.StatusCurrent, knowledge.StatusTransition, knowledge.StatusConflict:
		default:
			continue
		}
		for _, c := range f.ReviewDiscussions() {
			if c.Kind != "discussion" || c.Reference == "" || c.ObservedAt == "" || c.Quote == "" {
				continue
			}
			for _, d := range changes {
				// A package-wide inferred rule must not bind an unrelated file
				// merely because the review also mentioned that package.
				p := d.NewPath
				if d.IsDeleted || !(c.Path == p || d.IsRenamed && c.Path == d.OldPath ||
					f.Scope == p || f.Scope != "." && strings.HasPrefix(p, f.Scope+"/") ||
					c.Path == "" && f.Source == p) {
					continue
				}
				i := slices.IndexFunc(targets, func(t discussionTarget) bool { return t.Path == p && t.Reference == c.Reference })
				if i < 0 {
					targets = append(targets, discussionTarget{Path: p, Reference: c.Reference})
					i = len(targets) - 1
				}
				if !slices.Contains(targets[i].sources, c) {
					targets[i].sources = append(targets[i].sources, c)
				}
				targets[i].Conflict = targets[i].Conflict || f.ContextStatus() == knowledge.StatusConflict
			}
		}
	}
	return targets
}

type discussionAssessment struct {
	Path          string         `json:"path"`
	Reference     string         `json:"reference"`
	Verdict       string         `json:"verdict"`
	DecisionQuote string         `json:"decision_quote"`
	ScopePath     string         `json:"scope_path"`
	ScopeQuote    string         `json:"scope_quote"`
	Input         string         `json:"input"`
	Expected      string         `json:"expected"`
	Before        string         `json:"before"`
	After         string         `json:"after"`
	Violation     *businessCheck `json:"violation,omitempty"`
}

func (b businessContext) currentScope(a discussionAssessment, lines map[string][]changedLine) bool {
	quote := strings.TrimSpace(a.ScopeQuote)
	if quote == "" || len(quote) > 1200 {
		return false
	}
	if a.ScopePath == a.Path && slices.ContainsFunc(lines[a.Path], func(l changedLine) bool { return l.text == quote }) {
		return true
	}
	witness := businessCheck{SourcePath: a.ScopePath, SourceQuote: quote}
	return slices.ContainsFunc(b.rules, func(f knowledge.Fact) bool {
		return slices.ContainsFunc(f.Citations, func(c knowledge.Citation) bool {
			return c.Kind == "discussion" && c.Reference == a.Reference
		}) && supportsBusinessCheck(f, witness)
	})
}

// Accept ordinary findings outside the scoped discussion and deterministic
// property checks. Within its scope a new inferred finding needs a concrete
// case under the selected conditions. Missing assessments stay visible.
func (b businessContext) assessedChecks(checks []businessCheck, rows []discussionAssessment, lines map[string][]changedLine) ([]businessCheck, int) {
	if len(b.discussions) == 0 {
		return checks, 0
	}
	var additions []businessCheck
	limited, unresolved := map[string]bool{}, map[string]bool{}
	missing := 0
	for _, target := range b.discussions {
		found := false
		for _, a := range rows {
			term := strings.TrimSpace(a.DecisionQuote)
			if a.Path != target.Path || a.Reference != target.Reference || term == "" {
				continue
			}
			i := slices.IndexFunc(target.sources, func(c knowledge.Citation) bool {
				return (len(term) >= 12 || term == c.Quote) && strings.Contains(c.Quote, term)
			})
			if i < 0 {
				continue
			}
			if a.Verdict == "unknown" {
				found, unresolved[a.Path] = true, true
				continue
			}
			if target.Conflict {
				continue
			}
			if !b.currentScope(a, lines) {
				continue
			}
			if a.Verdict == "unrelated" {
				found = true
				continue
			}
			if strings.TrimSpace(a.Input) == "" || strings.TrimSpace(a.Expected) == "" ||
				strings.TrimSpace(a.Before) == "" || strings.TrimSpace(a.After) == "" {
				continue
			}
			switch a.Verdict {
			case "satisfied":
				found, limited[a.Path] = true, true
			case "violated":
				if a.Violation == nil || a.Violation.Outcome != "violation" || a.Violation.Path != a.Path ||
					strings.TrimSpace(a.Before) == strings.TrimSpace(a.After) || strings.TrimSpace(a.Expected) == strings.TrimSpace(a.After) {
					continue
				}
				c := *a.Violation
				c.discussion = target.sources[i].URL
				if c.discussion == "" {
					c.discussion = target.Reference
				}
				additions = append(additions, c)
				found, limited[a.Path] = true, true
			}
		}
		if !found {
			unresolved[target.Path] = true
			missing++
		}
	}
	var result []businessCheck
	for _, c := range checks {
		if (limited[c.Path] || unresolved[c.Path]) && !slices.ContainsFunc(b.rules, func(f knowledge.Fact) bool { return f.ID == c.ID && f.CurrentSchema() }) {
			if unresolved[c.Path] || c.Outcome != "refuted" {
				missing++
				continue
			}
		}
		result = append(result, c)
	}
	for _, c := range additions {
		if !unresolved[c.Path] {
			result = append(result, c)
		}
	}
	return result, missing
}

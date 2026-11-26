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
const discussionApplicabilityPrompt = `
For each listed (path, reference), first assess applicability in a top-level "applications" array, even when "checks" is empty. Read the actual terms or checked-in configuration. Distinguish the selected behavior from suggestions, examples, reference tables and other supported modes. Trace which setting the reviewed entry point loads and what may override it. A dated decision may still govern changed code. Check for replacement, changed scope and contradictory evidence.
An external agreement and an implementation answer different questions. When an in-scope dated decision explicitly states the behavior in effect for a named subject, that statement is evidence of the external selection. Identify that same subject at the current entry point and verify the selected implementation or its observable result. The source list has already been screened for revision, time, replacements and withdrawals known to this review. An available alternative or a hypothetical later agreement does not displace a documented decision. Treat an actual applicable override, an unreconciled contrary decision or an ambiguous subject as uncertain. A suggestion or description of historical practice by itself cannot establish an agreement.
Each application has "path", "reference", "decision_quote" (an exact original clause or current configuration line), and "verdict" ("violated", "satisfied", "reference_only", "unrelated" or "unknown"). Use "reference_only" for a lookup table or example that defines behavior without selecting the deployed mode. For a determined verdict supply "scope_path" and "scope_quote" (one exact current code line connecting this selection to the changed operation or showing why it is unrelated). For "satisfied" or "violated", trace one discriminating "input" through the base and head. Name the output being compared in "observable". Use the same input and output names across sources for the same scenario. Give "expected", "before" and "after" as short strings in the same units and format. Use identical strings for equal outcomes. Describe the calculation in the reason. Use "unknown" when the effective choice cannot be established.`

const discussionReviewPrompt = discussionApplicabilityPrompt + `
If the patch turns a conforming outcome into a failure, put a complete violation in "violation" with its rule id, current source_path and source_quote, affected line and actionable reason. Use "source_id" to identify a different supplied rule for the implementation witness when needed. Publish the grounded failure even if a provisional finding only asks whether its condition applies. If it answers that whole tentative finding, include its "candidate" and complete original "refuted_text" in the new violation for independent verification. If a satisfied decision disproves the entire hypothetical finding, propose its specific refutation in "checks" with "reference" identifying the governing selection. Other genuinely actionable consequences remain. Use ordinary "checks" for unrelated issues and direct property checks. An archive alone never establishes current behavior.`

type discussionTarget struct {
	Path      string   `json:"path"`
	Reference string   `json:"reference"`
	Conflict  bool     `json:"unresolved_conflict,omitempty"`
	Kind      string   `json:"kind,omitempty"`
	Config    []string `json:"current_config_lines,omitempty"`
	sources   []knowledge.Citation
}

func reviewDiscussions(rules knowledge.Snapshot, changes []model.Diff) []discussionTarget {
	var targets []discussionTarget
	for _, f := range rules {
		switch f.ContextStatus() {
		case knowledge.StatusCandidate, knowledge.StatusCurrent, knowledge.StatusTransition, knowledge.StatusConflict, knowledge.StatusDiscussionSource:
		default:
			continue
		}
		citations := f.ReviewDiscussions()
		// A rule's implementation witnesses can include multiple configuration
		// files. One can enumerate supported modes while another selects the
		// effective one. Keep both available to the applicability check.
		for _, c := range f.Citations {
			if c.Kind == "config" && c.CurrentPath != "" && c.Quote != "" && !f.CurrentSchema() {
				c.Reference = "config:" + c.CurrentPath
				citations = append(citations, c)
			}
		}
		for _, c := range citations {
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
					targets = append(targets, discussionTarget{Path: p, Reference: c.Reference, Kind: c.Kind})
					i = len(targets) - 1
				}
				if !slices.Contains(targets[i].sources, c) {
					targets[i].sources = append(targets[i].sources, c)
					if c.Kind == "config" && !slices.Contains(targets[i].Config, c.Quote) {
						targets[i].Config = append(targets[i].Config, c.Quote)
					}
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
	Observable    string         `json:"observable"`
	Candidate     string         `json:"candidate,omitempty"`
	FindingQuote  string         `json:"finding_quote,omitempty"`
	Expected      string         `json:"expected"`
	Before        string         `json:"before"`
	After         string         `json:"after"`
	Violation     *businessCheck `json:"violation,omitempty"`
	operation     [2]int
}

type businessScenario struct{ input, observable, expected, before, after string }

func (a discussionAssessment) identifies(candidate, original string) bool {
	phrase := strings.TrimSpace(a.FindingQuote)
	return a.Candidate == candidate && (len(phrase) >= 8 || phrase == original && phrase != "") && strings.Contains(original, phrase)
}

func (b businessContext) currentScope(a discussionAssessment, lines map[string][]changedLine) bool {
	quote := strings.TrimSpace(a.ScopeQuote)
	if quote == "" || len(quote) > 1200 {
		return false
	}
	if a.ScopePath == a.Path && slices.ContainsFunc(lines[a.Path], func(l changedLine) bool { return l.text == quote }) {
		return true
	}
	witness := businessCheck{Reference: a.Reference, Path: a.Path, SourcePath: a.ScopePath, SourceQuote: quote}
	return slices.ContainsFunc(b.rules, func(f knowledge.Fact) bool {
		return ownsBusinessSelection(f, a.Reference) && (selectedBusinessWitness(f, witness) || supportsBusinessCheck(f, witness))
	})
}

// Two independently named entry points in one file can have their own
// inputs and contracts. Only separate them when current declarations locate
// both assessments, including the changed line for a reported violation.
func (b businessContext) discussionOperation(a discussionAssessment) (int, int) {
	if a.ScopePath != a.Path {
		return 0, 0
	}
	check := businessCheck{SourcePath: a.Path, SourceQuote: a.ScopeQuote}
	start, end := 0, 0
	for _, f := range b.rules {
		for _, w := range f.DiscussionWitnesses() {
			if w.Reference != a.Reference || w.Symbol == "" || !citedBusinessLine([]knowledge.Citation{w}, nil, check) {
				continue
			}
			left, right := w.CurrentStartLine, w.CurrentEndLine
			if left == 0 {
				left, right = w.StartLine, w.EndLine
			}
			if left <= 0 || right < left || a.Violation != nil && (a.Violation.Line < left || right < a.Violation.Line) {
				continue
			}
			if start != 0 && (start != left || end != right) {
				return 0, 0
			}
			start, end = left, right
		}
	}
	return start, end
}

// Accept ordinary findings outside the scoped discussion and deterministic
// property checks. Within its scope a new inferred finding needs a concrete
// case under the selected conditions. Missing assessments stay visible.
type scopedSource struct{ path, reference string }

func ownsBusinessSelection(f knowledge.Fact, reference string) bool {
	if reference == "" {
		return false
	}
	return slices.ContainsFunc(f.ReviewDiscussions(), func(note knowledge.Citation) bool { return note.Reference == reference }) ||
		slices.ContainsFunc(f.Citations, func(note knowledge.Citation) bool {
			return note.Kind == "config" && note.CurrentPath != "" && reference == "config:"+note.CurrentPath
		})
}

func selectedBusinessWitness(f knowledge.Fact, c businessCheck) bool {
	for _, w := range f.DiscussionWitnesses() {
		if (w.Reference == c.Reference || w.Reference == "" && w.Path == f.Scope && f.Scope == c.Path && ownsBusinessSelection(f, c.Reference)) &&
			citedBusinessLine([]knowledge.Citation{w}, nil, c) {
			return true
		}
	}
	return false
}

// A deployment decision and the implementation it names may be stored in
// different records. Keep both identities when assessing an original finding.
func (b businessContext) scopedRefutation(c businessCheck, nominatedID string, lines []changedLine) (businessCheck, bool) {
	if c.ID == "" || c.Reference == "" {
		return c, false
	}
	for _, authority := range b.rules {
		if nominatedID != "" && authority.ID != nominatedID || !ownsBusinessSelection(authority, c.Reference) {
			continue
		}
		check := c
		check.discussion = c.Reference
		if authority.ID != c.ID {
			// The assessor returned the implementation ID in the rule field.
			// Reconcile it only if the decision independently cites the same
			// implementation. An unrelated rule can happen to match the diff.
			if c.SourceID != "" && c.SourceID != c.ID {
				continue
			}
			check.ID, check.SourceID = authority.ID, c.ID
		}
		if b.proves(authority, check, lines) {
			return check, true
		}
	}
	return c, false
}

func (b businessContext) assessedChecks(checks []businessCheck, rows []discussionAssessment, lines map[string][]changedLine) ([]businessCheck, map[scopedSource][]discussionAssessment, int) {
	if len(b.discussions) == 0 {
		return checks, nil, 0
	}
	type addition struct {
		check       businessCheck
		application discussionAssessment
	}
	var additions []addition
	limited, unresolved := map[string]bool{}, map[string]bool{}
	neutral := map[scopedSource]bool{}
	cases := map[scopedSource][]discussionAssessment{}
	missing := 0
	for _, target := range b.discussions {
		key := scopedSource{target.Path, target.Reference}
		found := false
		for _, a := range rows {
			term := strings.TrimSpace(a.DecisionQuote)
			if a.Path != target.Path || a.Reference != target.Reference || term == "" {
				continue
			}
			i := slices.IndexFunc(target.sources, func(c knowledge.Citation) bool {
				if c.Kind == "config" {
					for line := range strings.SplitSeq(c.Quote, "\n") {
						if term == strings.TrimSpace(line) && (businessIdentifier.MatchString(term) || strings.ContainsAny(term, "0123456789")) {
							return true
						}
					}
					return false
				}
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
			if a.Verdict == "unrelated" || a.Verdict == "reference_only" {
				found = true
				neutral[key] = true
				continue
			}
			if strings.TrimSpace(a.Input) == "" || strings.TrimSpace(a.Expected) == "" ||
				strings.TrimSpace(a.Before) == "" || strings.TrimSpace(a.After) == "" {
				continue
			}
			if len(b.discussions) > 1 {
				a.operation[0], a.operation[1] = b.discussionOperation(a)
			}
			switch a.Verdict {
			case "satisfied":
				if strings.TrimSpace(a.Expected) != strings.TrimSpace(a.After) {
					continue
				}
				found, limited[a.Path] = true, true
			case "violated":
				if a.Violation == nil || a.Violation.Outcome != "violation" || a.Violation.Path != a.Path ||
					strings.TrimSpace(a.Before) != strings.TrimSpace(a.Expected) || strings.TrimSpace(a.Expected) == strings.TrimSpace(a.After) {
					continue
				}
				c := *a.Violation
				c.Reference = target.Reference
				c.scenario = businessScenario{strings.TrimSpace(a.Input), a.Observable, a.Expected, a.Before, a.After}
				c.discussion = target.sources[i].URL
				if c.discussion == "" {
					c.discussion = target.Reference
				}
				additions = append(additions, addition{c, a})
				found, limited[a.Path] = true, true
			default:
				continue
			}
			cases[key] = append(cases[key], a)
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
	consistent := func(a discussionAssessment) bool {
		if unresolved[a.Path] {
			return false
		}
		for _, t := range b.discussions {
			key := scopedSource{t.Path, t.Reference}
			if t.Path != a.Path || neutral[key] && len(cases[key]) == 0 {
				continue
			}
			found, separate := false, len(cases[key]) > 0
			for _, other := range cases[key] {
				if strings.TrimSpace(a.Input) != strings.TrimSpace(other.Input) {
					left, right := a.operation[0], a.operation[1]
					otherLeft, otherRight := other.operation[0], other.operation[1]
					separate = separate && left > 0 && otherLeft > 0 && (right < otherLeft || otherRight < left)
					continue
				}
				separate = false
				found = true
				// Two sources may constrain different outputs of one call.
				// Opposite outcomes for the same output cannot establish a
				// defect or authorize removal of an ordinary finding.
				if (a.Observable == "" || other.Observable == "" || a.Observable == other.Observable) &&
					(a.Verdict != other.Verdict || a.Expected != other.Expected || a.Before != other.Before || a.After != other.After) {
					return false
				}
			}
			if !found && !separate {
				return false
			}
		}
		return true
	}
	for _, item := range additions {
		if consistent(item.application) {
			result = append(result, item.check)
		} else if !unresolved[item.check.Path] {
			missing++
		}
	}
	satisfied := map[scopedSource][]discussionAssessment{}
	for key, group := range cases {
		for _, row := range group {
			if row.Verdict == "satisfied" && consistent(row) {
				satisfied[key] = append(satisfied[key], row)
			}
		}
	}
	return result, satisfied, missing
}

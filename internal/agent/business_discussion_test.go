// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/model"
)

func TestScopedDiscussionConstrainsTheReviewedScenario(t *testing.T) {
	g := routingReviewGroup(knowledge.StatusCandidate)
	const terms = "The distribution center uses the standard queue for overnight consignments."
	citation := knowledge.Citation{Kind: "discussion", Path: routingReviewPath, Quote: terms,
		Reference: "review-21", URL: "https://example.org/reviews/21", ObservedAt: "2025-04-08T09:00:00Z"}
	rule := g.business.rules[0]
	rule.Citations = append(rule.Citations, citation)
	g.business = selectBusinessContext(knowledge.Snapshot{rule}, g.Diffs)
	if len(g.business.discussions) != 1 || !strings.Contains(g.business.text, citation.Reference) {
		t.Fatal("dated decision missing from review")
	}
	check := businessCheck{ID: rule.ID, Outcome: "violation", Path: routingReviewPath, Quote: dispatchQuote, Line: 11,
		SourcePath: routingPolicy, SourceQuote: routingSource, Severity: "medium", Reason: "A standard overnight order now enters the express queue."}
	assessment := discussionAssessment{Path: routingReviewPath, Reference: citation.Reference, DecisionQuote: "uses the standard queue for overnight consignments",
		ScopePath: routingReviewPath, ScopeQuote: "if isRush(order) {", Input: "An overnight standard consignment.",
		Expected: "standard", Before: "standard", After: "express", Verdict: "violated", Violation: &check}
	run := func(t *testing.T, g FileGroup, plain []businessCheck, rows ...discussionAssessment) ([]model.LlmComment, error) {
		t.Helper()
		data, err := json.Marshal(map[string]any{"checks": append([]businessCheck{}, plain...), "applications": rows})
		if err != nil {
			t.Fatal(err)
		}
		add, _, err := groundBusinessChecks(string(data), g, nil)
		return add, err
	}
	add, err := run(t, g, nil, assessment)
	if err != nil || len(add) != 1 || !strings.Contains(add[0].Content, citation.URL) {
		t.Fatalf("scoped, independently grounded violation: %v %v", add, err)
	}
	for _, tc := range []struct {
		name string
		edit func(*discussionAssessment)
	}{
		{"invented clause", func(a *discussionAssessment) { a.DecisionQuote = "every consignment requires express" }},
		{"non-specific clause", func(a *discussionAssessment) { a.DecisionQuote = "the" }},
		{"another decision", func(a *discussionAssessment) { a.Reference = "review-22" }},
		{"another path", func(a *discussionAssessment) { a.Path = "billing.go" }},
		{"unsupported entry point", func(a *discussionAssessment) { a.ScopeQuote = "if isPickup(order) {" }},
		{"discussion as live source", func(a *discussionAssessment) { a.ScopeQuote = terms }},
		{"no input", func(a *discussionAssessment) { a.Input = "" }},
		{"same expected and observed", func(a *discussionAssessment) { a.After = a.Expected }},
		{"inherited behavior", func(a *discussionAssessment) { a.Before = a.After }},
		{"no located violation", func(a *discussionAssessment) { a.Violation = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := assessment
			tc.edit(&a)
			add, err := run(t, g, nil, a)
			if err == nil || len(add) != 0 {
				t.Fatalf("unverified branch published: %v %v", add, err)
			}
		})
	}
	if add, err := run(t, g, nil); err == nil || len(add) != 0 {
		t.Fatal("missing discussion assessment appeared complete")
	}
	satisfied := assessment
	satisfied.Verdict, satisfied.After, satisfied.Violation = "satisfied", satisfied.Expected, nil
	if add, err := run(t, g, nil, satisfied); err != nil || len(add) != 0 {
		t.Fatal("a legitimate case became a finding")
	}
	if add, err := run(t, g, []businessCheck{check}, satisfied); err == nil || len(add) != 0 {
		t.Fatal("a generic rule bypassed the effective selection")
	}
	unrelated := assessment
	unrelated.Verdict, unrelated.Violation = "unrelated", nil
	if add, err := run(t, g, []businessCheck{check}, unrelated); err != nil || len(add) != 1 {
		t.Fatal("an unrelated discussion hid an independent issue")
	}
	other := citation
	other.Reference = "review-32"
	rule.Citations = append(rule.Citations, other)
	g.business = selectBusinessContext(knowledge.Snapshot{rule}, g.Diffs)
	uncertain := assessment
	uncertain.Verdict, uncertain.Reference, uncertain.Violation = "unknown", other.Reference, nil
	if add, err := run(t, g, nil, assessment, uncertain); err != nil || len(add) != 0 {
		t.Fatal("an unresolved decision permitted a conditional finding")
	}
	independent := assessment
	independent.ScopePath, independent.ScopeQuote = routingPolicy, routingSource
	independent.Reference = other.Reference
	if add, err := run(t, g, nil, assessment, independent); err != nil || len(add) != 1 {
		t.Fatalf("matching decisions with independent live scope evidence: %v %v", add, err)
	}
}

func TestReviewDiscussionsRespectsScopeDateAndStatus(t *testing.T) {
	source := knowledge.Citation{Kind: "discussion", Path: "orders/price.go", Quote: "Each account has a separate balance.",
		Reference: "R1", ObservedAt: "2025-01-01T00:00:00Z"}
	f := knowledge.Fact{ID: "1", Source: source.Path, Scope: ".", Citations: []knowledge.Citation{source, source}}
	changes := []model.Diff{{NewPath: "orders/ledger.go", IsRenamed: true, OldPath: source.Path}, {NewPath: "other.go"}, {NewPath: source.Path, IsDeleted: true}}
	makeTargets := func(status string) []discussionTarget {
		return reviewDiscussions(knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: f, Status: status}}), changes)
	}
	if got := makeTargets(knowledge.StatusCandidate); len(got) != 1 || got[0].Path != "orders/ledger.go" || len(got[0].sources) != 1 {
		t.Fatalf("rename, deduplication or unrelated file scope: %+v", got)
	}
	if got := makeTargets(knowledge.StatusConflict); len(got) != 1 || !got[0].Conflict {
		t.Fatal("unresolved conflict treated as a settled decision")
	}
	f.Citations[0].ObservedAt, f.Citations[1].Kind = "", "history"
	if got := makeTargets(knowledge.StatusCurrent); len(got) != 0 {
		t.Fatal("undated discussion or a commit treated as a dated decision")
	}
}

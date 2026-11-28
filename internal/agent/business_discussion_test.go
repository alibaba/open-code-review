// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/llm"
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
	run := func(t *testing.T, g FileGroup, plain []businessCheck, rows ...discussionAssessment) ([]businessAddition, error) {
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
		{"different existing violation", func(a *discussionAssessment) { a.Before = "on hold" }},
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
	badSatisfied := satisfied
	badSatisfied.After = "express"
	if add, err := run(t, g, nil, badSatisfied); err == nil || len(add) != 0 {
		t.Fatal("an inconsistent result was described as satisfied")
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

func TestApplicabilityLinksThePolicyAndTheImplementation(t *testing.T) {
	g := routingReviewGroup(knowledge.StatusCandidate)
	const quote = "Dispatch uses expressRouting to select the queue for this service."
	c := knowledge.Citation{Kind: "discussion", Path: routingReviewPath, Reference: "review-12", ObservedAt: "2025-05-10T10:00:00Z", Quote: quote}
	raw := knowledge.Fact{ID: "service-choice", Scope: routingReviewPath, Source: routingReviewPath, Citations: []knowledge.Citation{c},
		ScopeWitnesses: []knowledge.Citation{{Kind: "code", Path: routingPolicy, Reference: c.Reference, Symbol: "expressRouting", Quote: routingSource}}}
	archived := knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: raw, Status: knowledge.StatusDiscussionSource}})[0]
	g.business = selectBusinessContext(append(g.business.rules, archived), g.Diffs)
	check := businessCheck{ID: raw.ID, SourceID: "express-policy", Outcome: "violation", Path: routingReviewPath, Quote: dispatchQuote, Line: 11,
		SourcePath: routingPolicy, SourceQuote: routingSource, Severity: "high", Reason: "Standard orders use the wrong queue."}
	if supportsBusinessCheck(archived, check) || g.business.proves(archived, check, reviewedLines(g.Diffs)[routingReviewPath]) {
		t.Fatal("an unassessed historical observation authorized a violation")
	}
	row := discussionAssessment{Path: routingReviewPath, Reference: c.Reference, DecisionQuote: quote,
		ScopePath: routingReviewPath, ScopeQuote: "if isRush(order) {", Input: "one standard order", Expected: "standard", Before: "standard", After: "express",
		Verdict: "violated", Violation: &check}
	reply := func() string {
		data, err := json.Marshal(map[string]any{"checks": []businessCheck{}, "applications": []discussionAssessment{row}})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	add, _, err := groundBusinessChecks(reply(), g, nil)
	if err != nil || len(add) != 1 || !strings.Contains(add[0].Content, raw.ID) || !strings.Contains(add[0].Content, check.SourceID) {
		t.Fatalf("scoped decision and separately cited implementation: %v, %v", add, err)
	}
	check.SourceID = "missing"
	if add, _, err = groundBusinessChecks(reply(), g, nil); err == nil || len(add) != 0 {
		t.Fatal("an invented implementation record authorized a violation")
	}
	check.SourceID = ""
	check.SourcePath, check.SourceQuote = c.Path, c.Quote
	if add, _, err = groundBusinessChecks(reply(), g, nil); err == nil || len(add) != 0 {
		t.Fatal("discussion text used as the current implementation")
	}
	// The derived rule shows the original agreement's current named
	// definition, even when its own primary code citation is another file.
	derived := knowledge.Fact{ID: "accepted-caller", Source: routingReviewPath, Scope: routingReviewPath,
		Citations: []knowledge.Citation{c, {Kind: "code", Path: routingReviewPath, Quote: "if isRush(order) {"}}}
	g.business = selectBusinessContext(knowledge.ReviewSnapshot([]knowledge.Checked{
		{Fact: raw, Status: knowledge.StatusDiscussionSource}, {Fact: derived, Status: knowledge.StatusCandidate},
	}), g.Diffs)
	check.ID, check.SourceID, check.SourcePath, check.SourceQuote = derived.ID, derived.ID, routingPolicy, routingSource
	if add, _, err = groundBusinessChecks(reply(), g, nil); err != nil || len(add) != 1 {
		t.Fatalf("dropped exact named witness shown in the derived record: %v, %v", add, err)
	}
	foreign := derived
	foreign.ID, foreign.Citations = "other-operation", []knowledge.Citation{{Kind: "code", Path: "returns.go", Quote: "return cancelled"}}
	g.business = selectBusinessContext(append(g.business.rules, knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: foreign, Status: knowledge.StatusCurrent}})...), g.Diffs)
	check.SourceID, check.SourcePath, check.SourceQuote = foreign.ID, "returns.go", "return cancelled"
	if add, _, err = groundBusinessChecks(reply(), g, nil); err == nil || len(add) != 0 {
		t.Fatal("an unrelated implementation supplied the active decision's premise")
	}
	check.ID = foreign.ID
	if add, _, err = groundBusinessChecks(reply(), g, nil); err == nil || len(add) != 0 {
		t.Fatal("an unrelated rule claimed ownership of the active decision")
	}
}

func TestDifferentBusinessScenariosOnOneLineSurvive(t *testing.T) {
	g := routingReviewGroup(knowledge.StatusCandidate)
	f := g.business.rules[0]
	note := knowledge.Citation{Kind: "discussion", Path: routingReviewPath, Quote: "Both returns and cancellations use the standard queue.",
		Reference: "routing-policy", ObservedAt: "2025-05-10T10:00:00Z"}
	f.Citations = append(f.Citations, note)
	g.business = selectBusinessContext(knowledge.Snapshot{f}, g.Diffs)
	check := businessCheck{ID: f.ID, Outcome: "violation", Path: routingReviewPath, Quote: dispatchQuote, Line: 11,
		SourcePath: routingPolicy, SourceQuote: routingSource, Severity: "high", Reason: "A returned parcel is shipped a second time."}
	row := discussionAssessment{Path: routingReviewPath, Reference: note.Reference, DecisionQuote: note.Quote, Verdict: "violated",
		ScopePath: routingReviewPath, ScopeQuote: "if isRush(order) {", Input: "a return", Observable: "shipments", Before: "0", Expected: "0", After: "1", Violation: &check}
	other, cancellation := row, check
	other.Input, other.Observable, other.Violation = "a cancellation", "customer charges", &cancellation
	cancellation.Reason = "A cancelled order incurs a new delivery fee."
	response, _ := json.Marshal(map[string]any{"checks": []businessCheck{}, "applications": []discussionAssessment{row, other, row}})
	add, _, err := groundBusinessChecks(string(response), g, nil)
	if err != nil || len(add) != 2 || !strings.Contains(add[0].Content, check.Reason) || !strings.Contains(add[1].Content, cancellation.Reason) {
		t.Fatalf("lost a distinct failure or repeated a defect: %+v, %v", add, err)
	}
}

func TestTwoNamedEntryPointsCanUseDifferentInputs(t *testing.T) {
	const file = "shipments.go"
	g := FileGroup{Diffs: []model.Diff{{NewPath: file, Diff: "@@ -1,6 +1,6 @@\n func shipOrder() {\n-  standard()\n+  express()\n }\n func cancelOrder() {\n   cancel()\n }"}}}
	raw := func(id, symbol, body string, start int) knowledge.Checked {
		note := knowledge.Citation{Kind: "discussion", Path: file, Quote: symbol + " uses this agreement.", Reference: id, ObservedAt: "2025-01-02T00:00:00Z"}
		witness := knowledge.Citation{Kind: "code", Symbol: symbol, Reference: id, Path: file, Quote: body, StartLine: start, EndLine: start + 2}
		return knowledge.Checked{Fact: knowledge.Fact{ID: id, Category: knowledge.CategoryDiscussionSource, Source: file, Scope: file,
			Citations: []knowledge.Citation{note}, ScopeWitnesses: []knowledge.Citation{witness}}, Status: knowledge.StatusDiscussionSource}
	}
	ship := raw("shipping", "shipOrder", "func shipOrder() {\n  express()\n}", 1)
	cancel := raw("cancellation", "cancelOrder", "func cancelOrder() {\n  cancel()\n}", 4)
	g.business = selectBusinessContext(knowledge.ReviewSnapshot([]knowledge.Checked{ship, cancel}), g.Diffs)
	first := discussionAssessment{Path: file, Reference: "shipping", DecisionQuote: ship.Citations[0].Quote, Verdict: "violated",
		ScopePath: file, ScopeQuote: "func shipOrder() {", Input: "a standard parcel", Observable: "shipping queue", Expected: "standard", Before: "standard", After: "express",
		Violation: &businessCheck{ID: ship.ID, Outcome: "violation", Path: file, Line: 2, Quote: "express()", SourcePath: file, SourceQuote: "func shipOrder() {", Severity: "medium", Reason: "A standard parcel now ships express."}}
	second := discussionAssessment{Path: file, Reference: "cancellation", DecisionQuote: cancel.Citations[0].Quote, Verdict: "satisfied",
		ScopePath: file, ScopeQuote: "func cancelOrder() {", Input: "a pending cancellation", Observable: "order status", Expected: "cancelled", Before: "cancelled", After: "cancelled"}
	results, _, missing := g.business.assessedChecks(nil, []discussionAssessment{first, second}, reviewedLines(g.Diffs))
	if missing != 0 || len(results) != 1 {
		t.Fatalf("separate signed entry points required the same input: %v %d", results, missing)
	}
	second.Input, second.Observable = first.Input, first.Observable
	results, _, missing = g.business.assessedChecks(nil, []discussionAssessment{first, second}, reviewedLines(g.Diffs))
	if missing != 0 || len(results) != 1 {
		t.Fatal("matching input and output names merged two separately located operations")
	}
	second.ScopeQuote = first.ScopeQuote
	results, _, _ = g.business.assessedChecks(nil, []discussionAssessment{first, second}, reviewedLines(g.Diffs))
	if len(results) != 0 {
		t.Fatal("ambiguous or overlapping entry points were treated as disjoint")
	}
}

func TestApplicabilityChecksTheInstalledConfiguration(t *testing.T) {
	g := routingReviewGroup(knowledge.StatusCandidate)
	rule := g.business.rules[0]
	const file, selection = "deploy/settings.json", `"q": "std"`
	rule.Citations = append(rule.Citations, knowledge.Citation{Kind: "config", Path: file, CurrentPath: file, Quote: "{\n  " + selection + "\n}"})
	c := knowledge.Citation{Kind: "discussion", Path: routingReviewPath, Reference: "review-4", Quote: "Both queue formats are available.", ObservedAt: "2025-05-10T10:00:00Z"}
	rule.Citations = append(rule.Citations, c)
	g.business = selectBusinessContext(knowledge.Snapshot{rule}, g.Diffs)
	if len(g.business.discussions) != 2 || !strings.Contains(g.business.text, "current_config_lines") {
		t.Fatal("the active configuration was lost behind other implementation witnesses")
	}
	check := businessCheck{ID: rule.ID, Outcome: "violation", Path: routingReviewPath, Quote: dispatchQuote, Line: 11,
		SourcePath: routingPolicy, SourceQuote: routingSource, Severity: "high", Reason: "The alternative mode cannot deliver the order."}
	row := discussionAssessment{Path: routingReviewPath, Reference: c.Reference, DecisionQuote: c.Quote, Verdict: "violated",
		ScopePath: routingReviewPath, ScopeQuote: "if isRush(order) {", Input: "rush mode", Expected: "delivered", Before: "delivered", After: "lost", Violation: &check}
	config := row
	config.Reference, config.DecisionQuote, config.Input = "config:"+file, selection, "standard mode"
	config.Verdict, config.After, config.Violation = "satisfied", config.Expected, nil
	result, _, missing := g.business.assessedChecks(nil, []discussionAssessment{row, config}, reviewedLines(g.Diffs))
	if missing == 0 || len(result) != 0 {
		t.Fatal("an unsupported alternative escaped the installed selection")
	}
	row.Input = config.Input
	result, satisfied, missing := g.business.assessedChecks(nil, []discussionAssessment{row, config}, reviewedLines(g.Diffs))
	if missing == 0 || len(result) != 0 || len(satisfied) != 0 {
		t.Fatal("conflicting results for the same installed input authorized an amendment")
	}
	row.Verdict, row.Violation = "reference_only", nil
	config.Verdict, config.After, config.Violation = "violated", "lost", &check
	result, _, missing = g.business.assessedChecks(nil, []discussionAssessment{row, config}, reviewedLines(g.Diffs))
	if missing != 0 || len(result) != 1 {
		t.Fatal("a capability table blocked a violation for the installed mode")
	}
}

func TestScopedRefutationBindsTheDecisionAndSeparateImplementation(t *testing.T) {
	g := routingReviewGroup(knowledge.StatusCandidate)
	const clause = "Dispatch uses expressRouting for rush orders in this installation."
	const body = "func expressRouting(priority string) bool {\n\treturn priority == \"rush\"\n}"
	note := knowledge.Citation{Kind: "discussion", Path: routingReviewPath, Quote: clause,
		Reference: "routing-decision", ObservedAt: "2025-05-10T10:00:00Z"}
	def := knowledge.Citation{Kind: "code", Path: routingPolicy, CurrentPath: routingPolicy, Quote: body, Reference: note.Reference, Symbol: "expressRouting"}
	current := knowledge.Fact{ID: "queue-implementation", Source: routingPolicy, Scope: routingPolicy,
		Identifiers: []string{"enqueueExpress"}, Citations: []knowledge.Citation{def}}
	archive := knowledge.Fact{ID: "installed-routing", Category: knowledge.CategoryDiscussionSource, Scope: routingReviewPath,
		Source: routingReviewPath, Citations: []knowledge.Citation{note}, ScopeWitnesses: []knowledge.Citation{def}}
	group := func(archive knowledge.Fact, status string) FileGroup {
		fg := g
		fg.business = selectBusinessContext(knowledge.ReviewSnapshot([]knowledge.Checked{
			{Fact: archive, Status: knowledge.StatusDiscussionSource}, {Fact: current, Status: status},
		}), fg.Diffs)
		return fg
	}
	const original = "Rush consignments would be lost if the installed queue cannot accept express delivery."
	draft := model.LlmComment{Path: routingReviewPath, StartLine: 9, EndLine: 9, ExistingCode: dispatchQuote, Content: original}
	row := discussionAssessment{Path: routingReviewPath, Reference: note.Reference, DecisionQuote: clause,
		Verdict: "satisfied", ScopePath: routingReviewPath, ScopeQuote: "if isRush(order) {",
		Input: "one rush consignment", Expected: "express", Before: "express", After: "express",
		Candidate: "c-0", FindingQuote: "if the installed queue cannot accept express delivery"}
	nomination := businessCheck{ID: archive.ID, Outcome: "refuted", Candidate: row.Candidate, Reference: note.Reference,
		Path: routingReviewPath, Line: 9, Quote: dispatchQuote, SourcePath: routingPolicy, SourceQuote: routingSource,
		Trigger: row.Input, Effect: "lost consignment", Reason: "The operation selects the installed rush queue.", RefutedText: original}
	reply := func(row discussionAssessment, check businessCheck) string {
		data, err := json.Marshal(map[string]any{"applications": []discussionAssessment{row}, "checks": []businessCheck{check}})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	t.Run("import verification receives the matching input from each assessment", func(t *testing.T) {
		bound := archive
		bound.ScopeWitnesses = nil
		fg := group(bound, knowledge.StatusCandidate)
		selected := nomination
		selected.SourceID, selected.SourceQuote = current.ID, body
		rush := row
		rush.Observable = "destination queue"
		ordinary := rush
		ordinary.Input = "one scheduled consignment"
		ordinary.Expected, ordinary.Before, ordinary.After = "standard", "standard", "standard"
		response := func(verified discussionAssessment) string {
			data, err := json.Marshal(map[string]any{"applications": []discussionAssessment{ordinary, verified}, "checks": []businessCheck{selected}})
			if err != nil {
				t.Fatal(err)
			}
			return string(data)
		}
		first := response(rush)
		rush.Expected, rush.Before, rush.After = "express queue", "express queue", "express queue"
		second := response(rush)
		var outcomes []string
		fg.business.verifyImport = func(_ knowledge.Fact, c businessCheck) bool {
			if len(c.scopeCases) != 1 || c.scopeCases[0].Input != selected.Trigger {
				t.Fatalf("wrong verified input: %+v", c.scopeCases)
			}
			outcomes = append(outcomes, c.scopeCases[0].After)
			return true
		}
		client := &fakeAgentClient{responses: []*llm.ChatResponse{
			{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &first}}}},
			{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &second}}}},
		}}
		a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 20000}})
		a.args.CommentCollector.Add(draft)
		a.verifyBusinessFindings(t.Context(), fg, "", fg.Diffs[0].Diff)
		if len(outcomes) != 2 || outcomes[0] != "express" || outcomes[1] != "express queue" || len(a.args.CommentCollector.CommentsForPath(draft.Path)) != 0 {
			t.Fatalf("verified outcomes = %v; finding should be resolved", outcomes)
		}
	})
	for _, tc := range []struct {
		name string
		edit func(*businessCheck, *knowledge.Fact, *string)
		want bool
	}{
		{"separate record and complete multiline declaration", func(*businessCheck, *knowledge.Fact, *string) {}, true},
		{"unrecognized implementation", func(c *businessCheck, _ *knowledge.Fact, _ *string) { c.ID = "unknown" }, false},
		{"only a different decision cites this definition", func(_ *businessCheck, f *knowledge.Fact, _ *string) { f.ScopeWitnesses = nil }, false},
		{"explicit reference to an unconnected code record", func(c *businessCheck, f *knowledge.Fact, _ *string) {
			c.ID, c.SourceID = f.ID, current.ID
			f.ScopeWitnesses = nil
		}, false},
		{"conflicted implementation", func(_ *businessCheck, _ *knowledge.Fact, status *string) { *status = knowledge.StatusConflict }, false},
		{"wrong independent source", func(c *businessCheck, _ *knowledge.Fact, _ *string) { c.SourceID = "unrelated" }, false},
		{"different finding trigger", func(c *businessCheck, _ *knowledge.Fact, _ *string) { c.Trigger = "standard consignment" }, false},
		{"another deployment", func(c *businessCheck, _ *knowledge.Fact, _ *string) { c.Reference = "other-site" }, false},
		{"different candidate", func(c *businessCheck, _ *knowledge.Fact, _ *string) { c.Candidate = "c-1" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			second, bound, status := nomination, archive, knowledge.StatusCandidate
			second.ID, second.SourceQuote = current.ID, body
			tc.edit(&second, &bound, &status)
			fg := group(bound, status)
			first, independent := reply(row, nomination), reply(row, second)
			if tc.want {
				_, proposed, err := groundBusinessChecks(first, fg, []model.LlmComment{draft})
				if err != nil || proposed[0].ID != archive.ID {
					t.Fatalf("lost scoped nomination: %+v, %v", proposed, err)
				}
			}
			client := &fakeAgentClient{responses: []*llm.ChatResponse{
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &first}}}},
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &independent}}}},
			}}
			a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 20000}})
			a.args.CommentCollector.Add(draft)
			a.verifyBusinessFindings(t.Context(), fg, "", fg.Diffs[0].Diff)
			if removed := len(a.args.CommentCollector.CommentsForPath(draft.Path)) == 0; removed != tc.want {
				t.Errorf("removed original finding = %t, want %t", removed, tc.want)
			}
		})
	}
}

func TestBusinessSourceQuotesRequireContiguousCompleteLines(t *testing.T) {
	const file = "policy.go"
	citation := knowledge.Citation{Kind: "code", Path: file, Quote: "func active() bool {\r\n\tload()\r\n\treturn enabled\r\n}"}
	for _, tc := range []struct {
		quote string
		ok    bool
	}{
		{"load()\n\treturn enabled", true},
		{"func active() bool {\r\n\tload()", true},
		{"func active() bool {\n\treturn enabled", false},
		{"return enabled\n\tload()", false},
		{"load()\nreturn enabled", false},
		{"load()\n\treturn", false},
	} {
		c := businessCheck{SourcePath: file, SourceQuote: tc.quote}
		if got := citedBusinessLine([]knowledge.Citation{citation}, nil, c); got != tc.ok {
			t.Errorf("cited %q = %v, want %v", tc.quote, got, tc.ok)
		}
	}
}

func TestApplicabilitySettlesOnlyTheLocatedTentativeFinding(t *testing.T) {
	g := routingReviewGroup(knowledge.StatusCandidate)
	f := g.business.rules[0]
	const clause = "Rush orders may use the express queue in this installation."
	c := knowledge.Citation{Kind: "discussion", Path: routingReviewPath, Reference: "review-4", Quote: clause, ObservedAt: "2025-05-10T10:00:00Z"}
	other := c
	other.Reference = "earlier-note"
	f.Citations = append(f.Citations, other, c)
	g.business = selectBusinessContext(knowledge.Snapshot{f}, g.Diffs)
	const tentative = "Check whether this installation permits express delivery for rush orders."
	base := discussionAssessment{Path: routingReviewPath, Reference: c.Reference, DecisionQuote: clause, Verdict: "satisfied",
		ScopePath: routingReviewPath, ScopeQuote: "if isRush(order) {", Input: "one rush order", Expected: "express", Before: "express", After: "express",
		Candidate: "c-0", FindingQuote: "express delivery for rush orders"}
	check := businessCheck{ID: f.ID, Reference: c.Reference, Outcome: "refuted", Path: routingReviewPath, Quote: dispatchQuote, Line: 9,
		SourcePath: routingPolicy, SourceQuote: routingSource, Candidate: "c-0", RefutedText: tentative, Trigger: "one rush order", Effect: "wrong queue", Reason: "The installed terms permit express routing."}
	response := func(rows []discussionAssessment, checks ...businessCheck) string {
		raw, err := json.Marshal(map[string]any{"applications": rows, "checks": append([]businessCheck{}, checks...)})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	for _, tc := range []struct {
		name               string
		cover, invalid     bool
		wrongTrigger       bool
		differentCase      bool
		explicitNomination bool
		differentOutput    bool
	}{
		{name: "disproven by an independently reproduced result"},
		{name: "unsupported refutation", invalid: true},
		{name: "same selection but another triggering input", invalid: true, wrongTrigger: true},
		{name: "both fields agree on an unrelated passing input", invalid: true, differentCase: true},
		{name: "explicit nomination followed by another passing input", invalid: true, explicitNomination: true, differentCase: true},
		{name: "explicit nomination followed by a different observable", invalid: true, explicitNomination: true, differentOutput: true},
		{name: "tentative finding answered by a published diagnosis", cover: true},
		{name: "missing confirmed finding identity", cover: true, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, v := base, check
			if tc.cover {
				newIssue := check
				newIssue.Outcome, newIssue.Severity, newIssue.Reason = "violation", "high", "An express consignment now enters a queue prohibited by this installation."
				r.Verdict, r.After, r.Violation = "violated", "standard", &newIssue
				v.Outcome = "covered"
				v.Quote = "  " + v.Quote
				if tc.invalid {
					v.ID = "another-issue"
				}
			} else if tc.wrongTrigger {
				v.Trigger = "one standard order"
			} else if tc.invalid && !tc.differentCase && !tc.differentOutput {
				v.SourceQuote = "invented behavior"
			}
			previous := r
			previous.Reference, previous.Violation = other.Reference, nil
			if tc.cover {
				previous.Verdict = "reference_only"
			}
			rows := []discussionAssessment{previous, r}
			first, second := response(rows), response(rows, v)
			if tc.explicitNomination {
				first = response(rows, check)
			}
			if tc.differentCase {
				r.Input, previous.Input, v.Trigger = "one returned order", "one returned order", "one returned order"
				second = response([]discussionAssessment{previous, r}, v)
			}
			if tc.differentOutput {
				r.Observable, previous.Observable = "printed title", "printed title"
				second = response([]discussionAssessment{previous, r}, v)
			}
			client := &fakeAgentClient{responses: []*llm.ChatResponse{
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &first}}}},
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &second}}}},
			}}
			a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 10000}})
			a.args.CommentCollector.Add(model.LlmComment{Path: routingReviewPath, StartLine: 9, EndLine: 9, ExistingCode: dispatchQuote, Content: tentative})
			a.verifyBusinessFindings(t.Context(), g, "", g.Diffs[0].Diff)
			comments := a.args.CommentCollector.CommentsForPath(routingReviewPath)
			hadOriginal, hadDiagnosis := false, false
			for _, cm := range comments {
				hadOriginal = hadOriginal || cm.Content == tentative
				hadDiagnosis = hadDiagnosis || strings.Contains(cm.Content, "prohibited by this installation")
			}
			if client.calls != 2 || hadOriginal != tc.invalid || hadDiagnosis != tc.cover {
				t.Fatalf("wrong final findings or missing independent check: %v", comments)
			}
		})
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

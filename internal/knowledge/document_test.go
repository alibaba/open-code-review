// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCheckBusinessDocument(t *testing.T) {
	const target = "orders/draft.json"
	rule := Fact{ID: "limit", Scope: target, Source: "contract.json", Property: "limit", Keyword: "enum", Value: json.RawMessage(`[9007199254740993,1.00,"USD",true,null]`)}
	current := ReviewSnapshot([]Checked{{Fact: rule, Status: StatusVerified}})
	for _, test := range []struct{ value, outcome string }{
		{"9007199254740993", "satisfied"}, {"9007199254740992", "violated"}, {"1e0", "satisfied"},
		{`"\u0055SD"`, "satisfied"}, {"true", "satisfied"}, {"null", "satisfied"}, {"[]", "violated"},
		{"1e999999999", "uncertain"}, {"false", "violated"},
	} {
		got := current.CheckDocument(target, []byte(`{"limit":`+test.value+"}"))
		if len(got) != 1 || got[0].Outcome != test.outcome || got[0].Rule != "limit" {
			t.Fatalf("limit %s: got %+v, want %s", test.value, got, test.outcome)
		}
	}
	for _, c := range []struct{ path, document string }{{target, "{}"}, {"orders/other.json", `{"limit":2}`}, {target, "invalid"}} {
		if got := current.CheckDocument(c.path, []byte(c.document)); len(got) != 0 {
			t.Fatalf("unexpected check for %+v: %v", c, got)
		}
	}
	for _, status := range []string{StatusConflict, StatusChanged, StatusCandidate} {
		s := ReviewSnapshot([]Checked{{Fact: rule, Status: status}})
		if got := s.CheckDocument(target, []byte(`{"limit":0}`)); len(got) != 0 {
			t.Fatalf("%s rule generated a verdict: %v", status, got)
		}
	}
}

func TestContractChangeMustOriginateInReviewedRange(t *testing.T) {
	r := newLifecycleRepo(t)
	r.write("unit.json", `{"properties":{"unit":{"const":"cents"}}}`)
	original := r.commit("initial units")
	facts, err := original.Learn(t.Context(), "unit.json", "order.json", "")
	if err != nil {
		t.Fatal(err)
	}
	r.write("unit.json", `{"properties":{"unit":{"enum":["mills","cents"]}}}`)
	migrated := r.commit("accept additional units")
	head, err := migrated.Check(t.Context(), facts)
	if err != nil {
		t.Fatal(err)
	}
	context := ReviewSnapshot(head)
	if err := context.DescribeReviewChanges(t.Context(), original, migrated, head); err != nil ||
		len(context) != 1 || context[0].CurrentSchema() || !strings.Contains(context[0].Claim, "mills") || !strings.Contains(context[0].Claim, "cents") {
		t.Fatalf("contract migration: %+v, %v", context, err)
	}
	r.write("notes.md", "documentation")
	later := r.commit("unrelated update")
	context = ReviewSnapshot(head)
	if err := context.DescribeReviewChanges(t.Context(), migrated, later, head); err != nil || context[0].Claim != "" {
		t.Fatalf("old change attributed to new diff: %+v, %v", context, err)
	}
}

func TestReviewTransitionKeepsOnlySurvivingWitnesses(t *testing.T) {
	ctx := t.Context()
	r := newLifecycleRepo(t)
	r.write("fee.py", "def fee(kind):\n    if kind == 'pickup':\n        return 0\n    return 7\n")
	r.write("test_fee.py", "assert fee('pickup') == 0\n")
	base := r.commit("pickup orders carry no handling fee")
	implementation, err := base.Cite(ctx, "fee.py", 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	test, err := base.Cite(ctx, "test_fee.py", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	f, err := base.Prepare(ctx, Fact{Category: CategoryBusinessRule, Authority: AuthorityConfirmed, Source: "fee.py", Scope: ".",
		Claim: "Pickup orders have no handling fee.", Citations: []Citation{implementation, test},
		Conditions: []Condition{{Description: "The order is collected.", Citation: &implementation}}})
	if err != nil {
		t.Fatal(err)
	}
	check := func(before, after Tree) Fact {
		t.Helper()
		head, err := after.CheckReview(ctx, before.Commit, []Fact{f})
		if err != nil || len(head) != 1 {
			t.Fatalf("check review: %+v, %v", head, err)
		}
		s := ReviewSnapshot(head)
		if err := s.DescribeReviewChanges(ctx, before, after, head); err != nil {
			t.Fatal(err)
		}
		return s[0]
	}
	r.write("fee.py", "def fee(kind):\n    if kind == 'pickup':\n        return 4\n    return 7\n")
	change := r.commit("apply a pickup surcharge")
	got := check(base, change)
	if got.ContextStatus() != StatusTransition || got.baseCommit != base.Commit || got.baseClaim != f.Claim ||
		got.Claim != "" || got.Authority != AuthorityHistorical || len(got.baseConditions) != 1 || len(got.Conditions) != 0 ||
		len(got.Citations) != 1 || got.Citations[0].CurrentPath != test.Path || got.Citations[0].Quote != test.Quote {
		t.Fatalf("patch lost the surviving expectation or treated the old implementation as current: %+v", got)
	}
	raw := Snapshot{got}.ForPaths([]string{"fee.py"})
	if !strings.Contains(raw, `"base_claim":"Pickup orders have no handling fee."`) || strings.Contains(raw, "return 0") {
		t.Fatal(raw)
	}
	r.write("test_fee.py", "assert fee('pickup') == 4\n")
	acceptedChange := r.commit("update the accepted policy")
	if changed := check(base, acceptedChange); len(changed.Citations) != 0 || changed.ContextStatus() != StatusTransition {
		t.Fatalf("changed expectations were offered as current witnesses: %+v", changed)
	}
	r.write("notes.md", "explain shipping\n")
	if later := check(acceptedChange, r.commit("unrelated change")); later.baseClaim != "" || later.ContextStatus() != StatusChanged {
		t.Fatalf("attributed an earlier policy change to this review: %+v", later)
	}
}

func TestReviewTransitionSeparatesArchivedApplicabilityFromCurrentProof(t *testing.T) {
	ctx := t.Context()
	r := newLifecycleRepo(t)
	const service = "dispatch.py"
	const config = "depot.ini"
	r.write(service, "def route(parcel):\n    return parcel.agreed_route\n")
	r.write(config, "lane = bonded\n")
	const historyText = "Use the manifest route for bonded consignments."
	base := r.commit(historyText)
	rawDate, err := base.Git.Output(ctx, base.Repo, "show", "-s", "--format=%cI", base.Commit)
	if err != nil {
		t.Fatal(err)
	}
	commitDate := strings.TrimSpace(string(rawDate))
	when, err := time.Parse(time.RFC3339, commitDate)
	if err != nil {
		t.Fatal(err)
	}
	base.Now = when.Add(time.Hour)
	note := strings.Repeat("The depot confirmed the attached manifest schedule. ", 7) +
		"Only bonded consignments routed through this depot must use the agreed route until the next signed amendment."
	discussion := Citation{Kind: "discussion", Path: service, Reference: "review-8/comment-21", URL: "https://example.test/review/8#comment-21",
		Revision: base.Commit, ObservedAt: when.Add(-time.Minute).Format(time.RFC3339), StartLine: 1, EndLine: 1, Quote: note, Digest: Digest([]byte(note))}
	history := Citation{Kind: "history", Reference: base.Commit, Revision: base.Commit, StartLine: 1, EndLine: 1, Quote: historyText, Digest: Digest([]byte(historyText))}
	implementation, err := base.Cite(ctx, service, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	lane, err := base.Cite(ctx, config, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	condition := strings.Repeat("Use the bonded route recorded in the manifest. ", 8) + "Only the signed seasonal lane is covered."
	f := prepareFact(t, base, Fact{Category: CategoryBusinessRule, Authority: AuthorityInferred, Source: service, Scope: service,
		Claim: "Bonded parcels use the agreed route.", Citations: []Citation{implementation, history, discussion, lane},
		Conditions: []Condition{{Description: condition, Key: "lane", Equals: json.RawMessage(`"bonded"`), Citation: &lane}}})
	read := func(head Tree, fact Fact) (Fact, contextRecord, string) {
		t.Helper()
		head.Now = base.Now
		checked, err := head.CheckReview(ctx, base.Commit, []Fact{fact})
		if err != nil {
			t.Fatal(err)
		}
		snapshot := ReviewSnapshot(checked)
		if err := snapshot.DescribeReviewChanges(ctx, base, head, checked); err != nil {
			t.Fatal(err)
		}
		raw := snapshot.ForPaths([]string{service})
		if len(snapshot) == 0 {
			return Fact{}, contextRecord{}, raw
		}
		_, payload, ok := strings.Cut(raw, "\n")
		var records []contextRecord
		if !ok || len(payload) > promptEvidenceBytes || json.Unmarshal([]byte(payload), &records) != nil || len(records) != 1 {
			t.Fatalf("invalid review payload: %s", raw)
		}
		return snapshot[0], records[0], raw
	}
	original, accepted, _ := read(base, f)
	if accepted.Context != StatusCandidate || len(accepted.Citations) != 3 || accepted.Citations[2].Quote != note {
		t.Fatalf("accepted candidate lost the signed scope after 120 bytes: %+v", accepted.Citations)
	}
	for _, status := range []string{StatusCandidate, StatusCurrent, StatusConflict} {
		original.contextStatus = status
		if visible := original.ReviewDiscussions(); len(visible) != 1 || visible[0].Quote != note || visible[0].Reference != discussion.Reference {
			t.Fatalf("%s lost the discussion visible in the prompt: %+v", status, visible)
		}
	}
	r.write(service, "def route(parcel):\n    return parcel.default_route\n")
	proposed := r.commit("switch to the default route")
	for _, witnesses := range []int{1, 0} {
		if witnesses == 0 {
			r.write(config, "lane = domestic\n")
			proposed = r.commit("change the installed lane")
		}
		transition, rec, _ := read(proposed, f)
		if rec.Context != StatusTransition || rec.Claim != "" || rec.Authority != AuthorityHistorical || rec.BaseScope != f.Scope || rec.BaseClaim != f.Claim ||
			len(rec.BaseConditions) != 1 || rec.BaseConditions[0] != `lane = "bonded". `+condition || len(rec.Conditions) != 0 {
			t.Fatalf("original scope or applicability lost: %+v", rec)
		}
		if rec.CurrentWitnessCount == nil || *rec.CurrentWitnessCount != witnesses || len(rec.Citations) != witnesses ||
			witnesses != 0 && (rec.Citations[0].Kind != "config" || rec.Citations[0].Quote != lane.Quote) {
			t.Fatalf("ineligible current witnesses: %+v", rec)
		}
		if len(transition.BaseProvenance()) != 2 || len(rec.BaseProvenance) != 2 || rec.BaseProvenance[0] != discussion ||
			rec.BaseProvenance[1].Kind != "history" || rec.BaseProvenance[1].Quote != historyText || rec.BaseProvenance[1].Reference != base.Commit ||
			rec.BaseProvenance[1].ObservedAt != commitDate || rec.BaseProvenance[1].CurrentPath != "" {
			t.Fatalf("lost dated, verbatim archived evidence: %+v", rec.BaseProvenance)
		}
		if shown := transition.ReviewDiscussions(); len(shown) != 1 || shown[0] != discussion {
			t.Fatalf("late evaluator cannot consume the exact visible discussion: %+v", shown)
		}
	}
	// The raw record can contain a well-formed historical citation to a commit
	// from the proposal. That commit cannot corroborate the accepted rule.
	f.Citations[1].Revision, f.Citations[1].Reference = proposed.Commit, proposed.Commit
	if _, rec, _ := read(proposed, f); len(rec.BaseProvenance) != 1 || rec.BaseProvenance[0] != discussion {
		t.Fatalf("proposal-only commit appeared as accepted provenance: %+v", rec.BaseProvenance)
	}
	f.Citations[1] = history
	reviewTime := base.Now
	base.Now = when.Add(-time.Second)
	if _, rec, _ := read(proposed, f); len(rec.BaseProvenance) != 1 || rec.BaseProvenance[0] != discussion {
		t.Fatalf("Git commit dated after the review appeared as archived evidence: %+v", rec.BaseProvenance)
	}
	base.Now = reviewTime
	// A later discussion edit is unavailable even when its text and location
	// still match the saved record.
	f.Citations[2].ObservedAt = base.Now.Add(time.Minute).Format(time.RFC3339)
	if _, _, prompt := read(proposed, f); prompt != "" {
		t.Fatalf("later discussion was disclosed to an earlier review: %s", prompt)
	}
}

func TestReviewDiscussionsStayWithinProjectedBudget(t *testing.T) {
	f := Fact{ID: "shipping", Scope: "route.go", Source: "route.go", baseCommit: strings.Repeat("a", 40)}
	for _, ref := range []string{"first", "second", "third", "fourth"} {
		q := strings.Repeat("x", 1990)
		f.Citations = append(f.Citations, Citation{Kind: "discussion", Reference: ref, Quote: q, ObservedAt: "2024-02-22T12:00:00Z", Digest: Digest([]byte(q))})
	}
	for _, status := range []string{StatusCandidate, StatusCurrent, StatusConflict, StatusTransition} {
		f.contextStatus = status
		projected := f.promptRecord()
		shown := f.ReviewDiscussions()
		exposed := projected.Citations
		if status == StatusTransition {
			exposed = projected.BaseProvenance
		}
		if !projected.fitsPrompt() || len(shown) < 2 || len(shown) >= len(f.Citations) ||
			len(exposed) != len(shown) || projected.AdditionalCitations != len(f.Citations)-len(shown) {
			t.Fatalf("%s did not retain distinct references within the budget: %d shown, %d omitted", status, len(shown), projected.AdditionalCitations)
		}
		for i, c := range shown {
			if c != exposed[i] || c.Reference != f.Citations[i].Reference || c.Quote != f.Citations[i].Quote {
				t.Fatalf("%s truncated or replaced a visible reference: %+v", status, c)
			}
		}
	}
}

func TestWholeFileDismissalSuppliesRecheckedReviewEvidence(t *testing.T) {
	r := newLifecycleRepo(t)
	r.write("policy.py", "def handling_fee(kind):\n    return 0 if kind == 'pickup' else 7\n")
	r.write("checkout.py", "print(handling_fee(kind))\n")
	base := r.commit("accept pickup exception")
	f, err := base.Decide(t.Context(), "policy.py", "checkout.py", "MR-8", "Pickup should charge seven.", "Pickup carries no fee.")
	if err != nil || len(f.Citations) != 0 {
		t.Fatalf("whole-file decision: %+v, %v", f, err)
	}
	view := func(head Tree) Snapshot {
		t.Helper()
		checked, err := head.CheckReview(t.Context(), base.Commit, []Fact{f})
		if err != nil {
			t.Fatal(err)
		}
		s := ReviewSnapshot(checked)
		if err := s.DescribeReviewChanges(t.Context(), base, head, checked); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if s := view(base); len(s) != 1 || len(s[0].Citations) != 1 || !strings.Contains(s[0].Citations[0].Quote, "kind == 'pickup'") {
		t.Fatalf("stored decision has no verifiable current source: %+v", s)
	}
	r.write("policy.py", "def handling_fee(kind):\n    return 7\n")
	if s := view(r.commit("remove the pickup exception")); len(s) != 0 {
		t.Fatalf("old decision survived changed evidence: %+v", s)
	}
}

func TestProposalSchemaCannotValidateItself(t *testing.T) {
	r := newLifecycleRepo(t)
	r.write("policy.json", `{"properties":{"unit":{"const":"cents"},"approved":{"const":true}}}`)
	base := r.commit("accept cent policy")
	r.write("policy.json", `{"properties":{"unit":{"const":"mills"},"approved":{"const":true}}}`)
	proposal := r.commit("propose mills")
	facts, err := proposal.Learn(t.Context(), "policy.json", "order.json", "")
	if err != nil {
		t.Fatal(err)
	}
	checked, err := proposal.CheckReview(t.Context(), base.Commit, facts)
	if err != nil || len(checked) != 1 || checked[0].Property != "approved" || checked[0].Status != StatusVerified {
		t.Fatalf("expected only the unchanged accepted assertion: %+v, %v", checked, err)
	}
}

func TestSchemaRefreshKeepsOnlyReachableScopedMappings(t *testing.T) {
	r := newLifecycleRepo(t)
	r.write("order.json", `{"properties":{"unit":{"const":"cents"},"region":{"const":"west"}}}`)
	origin := r.commit("initial contract")
	facts, err := origin.Learn(t.Context(), "order.json", "sale.json", "unit")
	if err != nil {
		t.Fatal(err)
	}
	r.write("order.json", `{"properties":{"unit":{"const":"dollars"},"region":{"const":"east"}}}`)
	current := r.commit("adopt new contract")
	bindings := append(append([]Fact{}, facts...), facts...)
	for _, tc := range []struct {
		tree  Tree
		scope string
		count int
	}{
		{origin, "", 0}, {current, "unrelated.go", 0}, {current, "sale.json", 1}, {current, ".", 1},
	} {
		got, err := tc.tree.RefreshSchemas(t.Context(), bindings, tc.scope)
		if err != nil || len(got) != tc.count {
			t.Fatalf("refresh at %s under %q: %+v, %v", tc.tree.Commit, tc.scope, got, err)
		}
		if len(got) != 0 && (string(got[0].Value) != `"dollars"` || got[0].Revision != current.Commit || got[0].Scope != "sale.json" || got[0].Property != "unit") {
			t.Fatalf("refresh widened or lost the original binding: %+v", got)
		}
	}
	r.write("order.json", `{"properties":{"region":{"const":"east"}}}`)
	removed := r.commit("remove the unit constraint")
	if got, err := removed.RefreshSchemas(t.Context(), facts, ""); err != nil || len(got) != 0 {
		t.Fatalf("removed constraint produced a replacement: %+v, %v", got, err)
	}
}

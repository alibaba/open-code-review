// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"encoding/json"
	"strings"
	"testing"
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
	if err := context.DescribeSchemaChanges(t.Context(), original, migrated, head); err != nil ||
		len(context) != 1 || context[0].CurrentSchema() || !strings.Contains(context[0].Claim, "mills") || !strings.Contains(context[0].Claim, "cents") {
		t.Fatalf("contract migration: %+v, %v", context, err)
	}
	r.write("notes.md", "documentation")
	later := r.commit("unrelated update")
	context = ReviewSnapshot(head)
	if err := context.DescribeSchemaChanges(t.Context(), migrated, later, head); err != nil || context[0].Claim != "" {
		t.Fatalf("old change attributed to new diff: %+v, %v", context, err)
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

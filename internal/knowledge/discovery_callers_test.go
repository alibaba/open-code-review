// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/model"
)

func TestDiscoverAcceptedCallerAndResultUse(t *testing.T) {
	repo := newDiscoveryRepo(t)
	const origin = "app/rules/transform.py"
	const consumer = "app/output/finalize.py"
	const direct = "app/output/direct.py"
	const content = "def normalize(raw):\n    adjusted = raw.strip()\n    return adjusted.lower()\n"
	repo.write(origin, content)
	repo.write("app/rules/__init__.py", "")
	repo.write("app/other/transform.py", content)
	for i := 0; i < 24; i++ {
		repo.write(fmt.Sprintf("app/rules/filler_%02d.py", i), "padding = 0\n")
	}
	head := "from ..rules import transform as values\n\ndef write(raw):\n    item = values.normalize(raw)\n"
	sink := "    return records.create(  # " + strings.Repeat("description ", 15) + "\n        label=item,  # Keep the original label."
	body := head + strings.Repeat("    # Some intervening work.\n", 52) + sink + "\n    )\n"
	repo.write(consumer, body)
	repo.write(direct, "from ..rules.transform import normalize as value\n\ndef forward(raw):\n    return value(raw)\n")
	repo.write("app/output/other.py", strings.ReplaceAll(body, "..rules", "..other"))
	repo.write("app/output/shadow.py", strings.Replace(body, "def write(raw):", "def write(raw, values):", 1))
	repo.write("app/output/literal.py", "from ..rules import transform as values\n\ndef write():\n    return 'values.normalize(raw)'\n")
	repo.write("app/output/broken.py", body+"if ?\n")
	repo.write("app/output/excluded.py", body+"# EXCLUDED_ONLY\n")
	repo.write("app/output/private.py", body+"api_key = 'CREDENTIAL_ONLY'\n")
	base := repo.commit("Add the accepted modules")
	repo.write(origin, strings.Replace(content, "lower()", "upper()", 1))
	repo.write("app/output/new.py", body+"# PROPOSED_ONLY\n")
	proposal := repo.commit("Change the value")
	opts := DiscoverOptions{FocusLines: map[string][]int{origin: {3}}, FileFilter: &rules.FileFilter{Exclude: []string{"**/excluded.py"}}}
	calls := 0
	result, err := base.Discover(t.Context(), opts, func(_ context.Context, _, corpus string) (string, error) {
		calls++
		for _, absent := range []string{"EXCLUDED_ONLY", "CREDENTIAL_ONLY", "PROPOSED_ONLY"} {
			if strings.Contains(corpus, absent) {
				t.Errorf("non-admissible content: %s", absent)
			}
		}
		var sources []discoverySource
		if err := json.Unmarshal([]byte(corpus), &sources); err != nil {
			t.Fatal(err)
		}
		current := map[string]bool{}
		var source, call, use, forwarded string
		for _, s := range sources {
			if s.current() {
				current[s.Path] = true
			}
			if s.current() && s.Path == origin {
				source = s.ID
			}
			if s.Caller == nil {
				continue
			}
			if s.Revision != base.Commit || s.Caller.CalleePath != origin || s.Caller.Callee != "normalize" {
				t.Fatalf("wrong caller provenance: %+v", s)
			}
			switch {
			case s.Path == consumer && s.Caller.Function == "write" && s.Caller.CallLine == 4:
				if s.Caller.Role == "call" && s.StartLine == 4 && strings.Contains(s.Text, "item = values.normalize(raw)") {
					call = s.ID
				}
				if s.Caller.Role == "result_use" && s.StartLine == 57 && s.Text == sink {
					use = s.ID
				}
			case s.Path == direct && s.Caller.Function == "forward" && s.StartLine == 4 && s.Text == "    return value(raw)":
				forwarded = s.ID
			default:
				t.Errorf("unrelated caller or source location: %+v", s)
			}
		}
		if len(current) > discoveryFiles || source == "" || call == "" || use == "" || forwarded == "" {
			t.Fatalf("origin/call/use/alias %q %q %q %q in %d current files (caller excerpts: %v)", source, call, use, forwarded, len(current), slices.DeleteFunc(sources, func(s discoverySource) bool { return s.Caller == nil }))
		}
		rule := discoveryRule{Category: CategoryBusinessRule, Claim: "A new record uses the normalized label.", Scope: origin,
			Citations: []discoveryQuote{{Source: source, Quote: "    adjusted = raw.strip()"}, {Source: source, Quote: "    return adjusted.lower()"},
				{Source: call, Quote: "    item = values.normalize(raw)"}, {Source: use, Quote: sink}}}
		raw, err := json.Marshal(map[string][]discoveryRule{"records": {rule}})
		return string(raw), err
	})
	if err != nil || calls != 1 || len(result.Facts) != 1 {
		t.Fatalf("discovery: %v, calls %d, %+v", err, calls, result)
	}
	fact := result.Facts[0]
	want := AcceptedCall{CallerPath: consumer, Caller: "write", CalleePath: origin, Callee: "normalize"}
	legacy := fact
	legacy.AcceptedCall = nil
	newID, _ := stableFactID(fact)
	oldID, _ := stableFactID(legacy)
	if fact.AcceptedCall == nil || *fact.AcceptedCall != want || newID != oldID {
		t.Fatalf("accepted function pair or original observation identity lost: %+v", fact.AcceptedCall)
	}
	if len(fact.Citations) != 4 || fact.Citations[1].Path != consumer || fact.Citations[1].StartLine != 57 || fact.Citations[1].EndLine != 58 || fact.Citations[1].Revision != base.Commit || fact.Citations[1].Symbol != "write" {
		t.Fatalf("cited caller location: %+v", fact.Citations)
	}
	baseContext, err := base.CheckReview(t.Context(), base.Commit, result.Facts)
	if err != nil {
		t.Fatal(err)
	}
	prompt := ReviewSnapshot(baseContext).ForPaths([]string{origin})
	if !strings.Contains(prompt, "raw.strip()") || !strings.Contains(prompt, "records.create(") || !strings.Contains(prompt, "label=item") {
		t.Fatalf("native review prompt lost the caller or implementation: %s", prompt)
	}
	checked, err := proposal.CheckReview(t.Context(), base.Commit, result.Facts)
	if err != nil {
		t.Fatal(err)
	}
	headContext := ReviewSnapshot(checked)
	if err := headContext.DescribeReviewChanges(t.Context(), base, proposal, checked); err != nil {
		t.Fatal(err)
	}
	selected := headContext.SelectForChange([]string{origin}, nil)
	if len(sink) <= 220 || len(selected) != 1 || !slices.ContainsFunc(selected[0].promptRecord().Citations, func(c Citation) bool {
		return c.Path == consumer && c.Quote == sink && c.Symbol == "write"
	}) {
		t.Fatalf("full unchanged consumer citation missing at the head: %s", headContext.ForPaths([]string{origin}))
	}
	change := model.Diff{NewPath: origin, NewFileContent: strings.Replace(content, "lower()", "upper()", 1)}
	resolve := func(tree Tree, f Fact, quote string) bool {
		t.Helper()
		_, ok := tree.ReviewCaller(t.Context(), base.Commit, f, change, 3, quote)
		return ok
	}
	if !resolve(proposal, selected[0], "label=item,  # Keep the original label.") ||
		resolve(proposal, selected[0], strings.Split(sink, "\n")[0]) {
		t.Fatal("only a complete citation covering the true result use can establish the relation")
	}
	excluded := proposal
	excluded.AllowPath = func(p string) bool { return p != "app/rules/__init__.py" }
	if resolve(excluded, selected[0], sink) {
		t.Fatal("an excluded initializer supplied a module binding")
	}
	proposed := selected[0]
	proposed.Revision = proposal.Commit
	if resolve(proposal, proposed, sink) {
		t.Fatal("a proposal-only discovery supplied an accepted call")
	}
	repo.write(consumer, strings.ReplaceAll(body, "..rules", "..other"))
	if resolve(repo.commit("Rebind the consumer to a different module"), selected[0], sink) {
		t.Fatal("an unchanged invocation spelling from a different module proved the rule")
	}
}

func TestDiscoveryCallerRequiresUnambiguousBinding(t *testing.T) {
	repo := newDiscoveryRepo(t)
	const target = "p/rule.py"
	const caller = "p/consumer.py"
	repo.write(target, "def choose():\n    return 1\n")
	repo.write(caller, "from . import rule\n\ndef collect():\n    return rule.choose()\n")
	repo.write("p/__init__.py", "")
	want := func(visible bool) {
		t.Helper()
		accepted := repo.commit("Change the module binding")
		seen := false
		_, err := accepted.Discover(t.Context(), DiscoverOptions{FocusLines: map[string][]int{target: {2}}}, func(_ context.Context, _, corpus string) (string, error) {
			var sources []discoverySource
			if err := json.Unmarshal([]byte(corpus), &sources); err != nil {
				return "", err
			}
			seen = slices.ContainsFunc(sources, func(s discoverySource) bool { return s.Path == caller && s.Caller != nil })
			return `{"records":[]}`, nil
		})
		if err != nil || seen != visible {
			t.Fatalf("accepted caller = %t, want %t: %v", seen, visible, err)
		}
	}
	want(true)
	repo.write(caller, "from . import rule\n\ndef collect():\n    return rule.choose()\n\ncollect = alternate\n")
	want(false)
	repo.write(caller, "from . import rule\n\ndef collect():\n    return rule.choose()\n")
	repo.write("p/__init__.py", "from .other import rule\n")
	want(false)
	repo.write("p/__init__.py", "")
	repo.write("p/rule/__init__.py", "from .other import choose\n")
	want(false)
}

func TestReviewCallerRechecksCitedUsesAndTransitions(t *testing.T) {
	const impl, consumer = "rules.py", "flow.py"
	const accepted = "def transform(n):\n    return n\n"
	const changed = "def transform(n):\n    return n + 1\n"
	const invocation = "    value = convert(n)"
	const prefix = "from .rules import transform as convert\n\ndef finish(n):\n" + invocation + "\n"
	for _, tc := range []struct {
		name, oldFlow, headFlow string
		citedLine, oldUse, use  int
		witness                 string
		valid                   bool
	}{
		{"legacy earlier use", prefix + "    observe(value)\n    if value:\n        accept()\n", prefix + "    observe(value)\n    if value:\n        accept()\n", 5, 5, 5, "observe(value)", true},
		{"rewritten use", prefix + "    return value\n", prefix + "    audit()\n    return round(value, 2)\n", 5, 5, 6, invocation, true},
		{"changed cited use before a later guard", prefix + "    record(value)\n    if value:\n        accept()\n", prefix + "    audit()\n    return record(round(value, 2))\n", 5, 5, 6, invocation, true},
		{"duplicate invocation", prefix + "    return value\n", prefix + "    if n:\n        value = convert(n)\n    return round(value, 2)\n", 5, 0, 0, invocation, false},
		{"same spelling rebound", prefix + "    return value\n", strings.Replace(prefix, ".rules", ".other", 1) + "    return round(value, 2)\n", 5, 0, 0, invocation, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newDiscoveryRepo(t)
			repo.write(impl, accepted)
			repo.write("other.py", accepted)
			repo.write(consumer, tc.oldFlow)
			base := repo.commit("introduce the accepted consumer")
			original, err := base.Prepare(t.Context(), Fact{Scope: consumer, Source: impl, Origin: "discovery",
				Category: CategoryBusinessRule, Authority: AuthorityInferred, Claim: "Finish uses the transformed input.",
				Citations: []Citation{{Kind: "code", Path: impl, StartLine: 2, EndLine: 2},
					{Kind: "code", Path: consumer, StartLine: tc.citedLine, EndLine: tc.citedLine}}})
			if err != nil {
				t.Fatal(err)
			}
			original.Citations[1].Symbol = "finish"
			repo.write(impl, changed)
			repo.write(consumer, tc.headFlow)
			head := repo.commit("change the implementation and its use")
			checked, err := head.CheckReview(t.Context(), base.Commit, []Fact{original})
			if err != nil {
				t.Fatal(err)
			}
			records := ReviewSnapshot(checked)
			if err := records.DescribeReviewChanges(t.Context(), base, head, checked); err != nil || len(records) != 1 || records[0].AcceptedCall == nil {
				t.Fatalf("recover the accepted target from the original citation: %+v, %v", records, err)
			}
			code, ok := head.ReviewCaller(t.Context(), base.Commit, records[0], model.Diff{NewPath: impl, NewFileContent: changed}, 2, tc.witness)
			if ok != tc.valid {
				t.Fatalf("reviewed call = %+v, want valid=%v; current citations: %+v", code, tc.valid, records[0].Citations)
			}
			if !ok {
				return
			}
			if code.Base.ResultLine != tc.oldUse || code.Head.ResultLine != tc.use || code.Base.CallLine != 4 || code.Head.CallLine != 4 ||
				!strings.Contains(records.ForPaths([]string{impl}), strings.TrimSpace(tc.witness)) {
				t.Fatalf("incorrect call or source destination: %+v", code)
			}
		})
	}
}

func TestDiscoveryMultipleFocusedCallsAndGuard(t *testing.T) {
	repo := newDiscoveryRepo(t)
	const origin = "app/rules.py"
	const consumer = "app/finish.py"
	repo.write(origin, "def allowed(value):\n    return value.ready\n\ndef format_value(value):\n    return value.label\n")
	repo.write(consumer, "from .rules import allowed, format_value\n\ndef finish(value):\n    permitted = allowed(value)\n    observe(permitted)\n    if permitted:\n        activate()\n    label = format_value(value)\n    persist(label)\n")
	base := repo.commit("Use both rules")
	opts := DiscoverOptions{FocusLines: map[string][]int{origin: {2, 5}}}
	_, err := base.Discover(t.Context(), opts, func(_ context.Context, _, corpus string) (string, error) {
		var sources []discoverySource
		if err := json.Unmarshal([]byte(corpus), &sources); err != nil {
			return "", err
		}
		uses, declared := map[string]string{}, 0
		for _, source := range sources {
			if source.Path == origin && source.current() {
				declared++
			}
			if c := source.Caller; c != nil && source.Path == consumer {
				if c.Function != "finish" || c.CalleePath != origin || source.Revision != base.Commit {
					t.Fatalf("different consumer or revision: %+v", source)
				}
				uses[c.Callee+" "+c.Role] = source.Text
			}
		}
		if declared != 2 || len(uses) != 4 ||
			uses["allowed call"] != "    permitted = allowed(value)" ||
			uses["allowed result_use"] != "    if permitted:\n        activate()" ||
			uses["format_value call"] != "    label = format_value(value)" ||
			uses["format_value result_use"] != "    persist(label)" {
			t.Errorf("focused declarations %d, caller evidence %#v", declared, uses)
		}
		return `{"records":[]}`, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

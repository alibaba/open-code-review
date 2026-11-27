// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestCallerLookupUsesCitationLinesOnTheRequestedSide(t *testing.T) {
	citation := knowledge.Citation{Path: "orders.py", StartLine: 10, EndLine: 12,
		CurrentPath: "orders/current.py", CurrentStartLine: 19, CurrentEndLine: 22}
	rules := knowledge.Snapshot{
		{AcceptedCall: &knowledge.AcceptedCall{}, Citations: []knowledge.Citation{citation, citation}},
		{Citations: []knowledge.Citation{{Path: "orders.py", StartLine: 2, EndLine: 2}}},
	}
	for _, tc := range []struct {
		side, path string
		lines      []int
	}{
		{"base", "orders.py", []int{10, 12}},
		{"head", "orders/current.py", []int{19, 22}},
		{"head", "orders.py", nil},
	} {
		got := (businessLookup{Path: tc.path, Side: tc.side}).nearbyCitations(rules)
		if !slices.Equal(got, tc.lines) {
			t.Fatalf("%s %s: source lines %v", tc.side, tc.path, got)
		}
	}
}

func TestMixedRuleGroupKeepsIndependentFindingsWhenCallerLookupsEnd(t *testing.T) {
	dir := initIdentityRepo(t)
	tree := &knowledge.Tree{Repo: dir, Workspace: true, Git: gitcmd.New(1)}
	const ask = `{"lookups":[{"path":"keep.go","symbol":"keep"}]}`
	const current = `{"currency":"USD"}`
	change := model.Diff{NewPath: "orders.json", OldPath: "orders.json", NewFileContent: current,
		Diff: "@@ -1 +1 @@\n-{\"currency\":\"EUR\"}\n+" + current}
	g, schema := schemaReviewGroup(change)
	link := knowledge.Fact{ID: "link", Origin: "discovery", Category: knowledge.CategoryBusinessRule, Scope: "gone.go", Source: "keep.go",
		Claim: "Keep the dispatch capacity.", AcceptedCall: &knowledge.AcceptedCall{CalleePath: "gone.go", CallerPath: "keep.go"}}
	g.Diffs = append(g.Diffs, model.Diff{NewPath: "gone.go", OldPath: "gone.go", Diff: "@@ -1 +1 @@\n-return 1\n+return 2"})
	g.business = selectBusinessContext(append(g.business.rules, knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: link, Status: knowledge.StatusCurrent}})...), g.Diffs)
	reason := "Orders in dollars violate the selected contract."
	original := businessReply(t, businessCheck{ID: schema.ID, Outcome: "violation", Path: change.NewPath, Quote: current, Reason: reason, Severity: "high"})
	for _, tc := range []struct {
		name    string
		lookups []string
	}{
		{"caller exceeds its two lookup rounds", []string{ask, ask, ask}},
		{"malformed supplemental reply", []string{"invalid reply"}},
		{"supplemental pass has no findings", []string{`{"checks":[]}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeAgentClient{}
			for _, answer := range append([]string{original}, tc.lookups...) {
				client.responses = append(client.responses, &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &answer}}}})
			}
			a := New(Args{RepoDir: dir, LLMClient: client, KnowledgeTree: tree, Template: template.Template{MaxTokens: 16000}})
			a.verifyBusinessFindings(t.Context(), g, "", change.Diff)
			published := a.args.CommentCollector.CommentsForPath(change.NewPath)
			if len(published) != 1 || !strings.Contains(published[0].Content, reason) || client.calls != len(tc.lookups)+1 {
				t.Fatalf("unrelated violation was lost or repeated: %+v, %d calls", published, client.calls)
			}
			tasks := a.session.GetOrCreateFileSession(fileGroupKey(g.Diffs)).TaskRecords[session.BusinessReviewTask]
			if strings.Contains(tasks[0].RequestMessages[0].ExtractText(), callerLookupPrompt) || !strings.Contains(tasks[1].RequestMessages[0].ExtractText(), callerLookupPrompt) {
				t.Fatal("independent rules were not settled before the caller search")
			}
			first, second := tasks[0].RequestMessages, tasks[1].RequestMessages
			if strings.Contains(first[len(first)-1].ExtractText(), link.Claim) || !strings.Contains(second[len(second)-1].ExtractText(), link.Claim) {
				t.Fatal("the independent assessment spent its budget on the caller-only rule")
			}
		})
	}
}

func TestMixedRuleGroupRetainsDirectCalleeChecks(t *testing.T) {
	const callee, caller, tests = "pricing.py", "invoice.py", "test_pricing.py"
	const callerTest, callerAssertion = "test_invoice.py", "    assert quote_amount(10) == 10"
	const policy, policyLimit = "pricing_policy.py", "MAX_QUOTE_CENTS = 100"
	const onlyPreview = "PREVIEW_MARKUP_CENTS = 1"
	const limit = "INVOICE_LIMIT_CENTS = 100"
	const oldReturn, newReturn = "return min(cents, bound)", "return min(cents + preview_markup, bound)"
	const previewBound = "    bound = min(INVOICE_LIMIT_CENTS, policy_max)"
	const localTest = "    assert normalize(100) == 100"
	const before = "from .pricing_policy import MAX_QUOTE_CENTS as policy_max, PREVIEW_MARKUP_CENTS as preview_markup\n\n" + limit + "\n\ndef normalize(raw):\n" +
		"    minimum = 0\n    cents = max(raw, minimum)\n    return min(cents, INVOICE_LIMIT_CENTS, policy_max)\n\n" +
		"def quote_amount(cents, mode='normal'):\n" + previewBound + "\n" +
		"    if mode == 'express':\n        bound = 200\n    " + oldReturn + "\n\ndef test_normalization_contract():\n" + localTest + "\n"
	const use = "    return store(amount)"
	const previewUse = "    return store(quote)"
	const flow = "from .pricing import normalize, quote_amount\n\ndef invoice(raw):\n    amount = normalize(raw)\n" + use +
		"\n\ndef preview(raw):\n    quote = quote_amount(raw)\n" + previewUse + "\n\ndef store(amount):\n    return amount\n"
	const prices = "    assert direct_quote(10) == 10\n    assert direct_quote(100) == 100"
	const spec = "from .pricing import quote_amount as direct_quote\n\ndef test_amount_in_cents():\n" + prices + "\n"
	const callerSpec = "from .invoice import invoice as quote_amount\nfrom .pricing import quote_amount as direct_quote\n\ndef test_stored_cents():\n" +
		"    assert direct_quote(10) == 10\n" + callerAssertion + "\n"
	const mixed = "Normalization and direct quotes preserve ordinary cents up to the upper bound."
	const local = "The invoice amount is bounded by the module's explicit limit."
	const external = "Direct quotes honor the accepted maximum in the pricing policy."
	const pure = "An invoice uses the result of normalization when stored."
	const downstream = "Only the stored invoice output is covered by this test."
	const other = "The normalization substitutes zero for a negative amount."
	const siblingTest = "A direct assertion of the quote sibling does not constrain normalization."
	const siblingLiteral = "The preview markup is only read by the quote sibling."
	const siblingCode = "The bounds assigned by the quote sibling are separate."
	const localDirectTest = "The same-module check directly asserts the normalization result."

	dir := initPreviewRepo(t)
	commitIn(t, dir, policy, policyLimit+"\n"+onlyPreview+"\n", "define the shared pricing maximum and preview markup")
	commitIn(t, dir, callee, before, "define accepted cent normalization")
	commitIn(t, dir, caller, flow, "use normalized cents when storing an invoice")
	commitIn(t, dir, tests, spec, "exercise the normalization directly")
	commitIn(t, dir, callerTest, callerSpec, "distinguish invoice output from the direct quote")
	ctx := t.Context()
	git := gitcmd.New(1)
	output := func(args ...string) string {
		t.Helper()
		data, err := git.Output(ctx, dir, args...)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	base := knowledge.Tree{Repo: dir, Git: git, Commit: strings.TrimSpace(output("rev-parse", "HEAD"))}
	discovered, err := base.Discover(ctx, knowledge.DiscoverOptions{FocusLines: map[string][]int{callee: {1, 6, 11}}}, func(_ context.Context, _, corpus string) (string, error) {
		var sources []struct {
			ID, Path, Text string
			Caller         *struct{ Role string }
		}
		if err := json.Unmarshal([]byte(corpus), &sources); err != nil {
			return "", err
		}
		cite := func(path, quote, role string) map[string]string {
			for _, source := range sources {
				if source.Path == path && strings.Contains(source.Text, quote) &&
					(role == "" && source.Caller == nil || source.Caller != nil && source.Caller.Role == role) {
					return map[string]string{"source": source.ID, "quote": quote}
				}
			}
			t.Fatalf("missing accepted quote %q in %s", quote, path)
			return nil
		}
		implementation := cite(callee, "    minimum = 0", "")
		consumer := cite(caller, use, "result_use")
		type quote = map[string]string
		var records []map[string]any
		for _, f := range []struct {
			claim     string
			citations []quote
		}{
			{mixed, []quote{cite(callee, previewBound, ""), cite(caller, previewUse, "result_use"), cite(tests, prices, "")}},
			{local, []quote{implementation, consumer, cite(callee, limit, "")}},
			{external, []quote{implementation, consumer, cite(policy, policyLimit, "")}},
			{pure, []quote{implementation, cite(callee, "    cents = max(raw, minimum)", ""), consumer}},
			{downstream, []quote{implementation, consumer, cite(callerTest, callerAssertion, "")}},
			{other, []quote{implementation}},
			{siblingTest, []quote{implementation, consumer, cite(tests, prices, "")}},
			{siblingLiteral, []quote{implementation, consumer, cite(policy, onlyPreview, "")}},
			{siblingCode, []quote{implementation, consumer, cite(callee, previewBound, "")}},
			{localDirectTest, []quote{implementation, consumer, cite(callee, localTest, "")}},
		} {
			records = append(records, map[string]any{"category": knowledge.CategoryBusinessRule, "claim": f.claim, "scope": callee, "citations": f.citations})
		}
		data, err := json.Marshal(map[string]any{"records": records})
		return string(data), err
	})
	if err != nil || len(discovered.Facts) != 10 {
		t.Fatalf("discovery did not produce the expected facts: %v, %+v", err, discovered)
	}
	after := strings.Replace(before, oldReturn, newReturn, 1)
	commitIn(t, dir, callee, after, "increment a directly returned amount")
	head := knowledge.Tree{Repo: dir, Git: git, Commit: strings.TrimSpace(output("rev-parse", "HEAD"))}
	checked, err := head.CheckReview(ctx, base.Commit, discovered.Facts)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, f := range checked {
		ids[f.Claim] = f.ID
	}
	snapshot := knowledge.ReviewSnapshot(checked)
	if err := snapshot.DescribeReviewChanges(ctx, base, head, checked); err != nil {
		t.Fatal(err)
	}
	byID := map[string]knowledge.Fact{}
	for _, f := range snapshot {
		byID[f.ID] = f
		if f.ID != ids[other] && (f.AcceptedCall == nil || f.AcceptedCall.CalleePath != callee || len(f.Citations) != 3 ||
			f.Citations[0].CurrentPath != callee || f.Citations[1].CurrentPath != caller) {
			t.Fatalf("expected the checked discovery layout, with callee then caller: %+v", f)
		}
		if f.ID == ids[mixed] && (f.ContextStatus() != knowledge.StatusTransition || f.AcceptedCall.Callee != "quote_amount") {
			t.Fatalf("the directly tested quote function should still be linked after it changes: %+v", f)
		}
		if f.ID == ids[external] && (f.ContextStatus() != knowledge.StatusCandidate || f.Citations[2].Kind != "code" || f.Citations[2].CurrentPath != policy) {
			t.Fatalf("the independent policy source was not rechecked at the head: %+v", f)
		}
	}
	change := model.Diff{NewPath: callee, OldPath: callee, NewFileContent: after, Diff: output("diff", base.Commit, head.Commit, "--", callee)}
	if len(ids) != 10 || len(snapshot) != 10 {
		t.Fatalf("bad mixed-rule fixture: %+v", snapshot)
	}
	for _, tc := range []struct {
		claim string
		want  bool
	}{{siblingTest, false}, {siblingLiteral, false}, {siblingCode, false}, {localDirectTest, true}} {
		if _, ok := head.WithDirectCalleeEvidence(ctx, byID[ids[tc.claim]], after); ok != tc.want {
			t.Errorf("direct evidence for %q = %v, want %v", tc.claim, ok, tc.want)
		}
	}
	unknown := byID[ids[external]]
	link := *unknown.AcceptedCall
	link.Callee = "missing_definition"
	unknown.AcceptedCall = &link
	if _, ok := head.WithDirectCalleeEvidence(ctx, unknown, after); ok {
		t.Error("an absent callee was admitted for direct review")
	}
	const falseFinding = "A normal quote for a raw amount of 100 now returns 101 cents, exceeding the invoice limit."
	const expressFinding = "An express quote for a raw amount of 100 now returns 101 cents. A normal quote of 100 remains capped."
	const missed = "A raw amount of 10 now returns 11 cents, so it is one cent higher than the accepted amount."
	line := 1 + strings.Count(after[:strings.Index(after, newReturn)], "\n")
	direct := businessCheck{ID: ids[mixed], Outcome: "violation", Path: callee, Quote: newReturn, Reason: missed,
		SourcePath: tests, SourceQuote: "assert direct_quote(10) == 10", Severity: "medium"}
	refute := businessCheck{ID: ids[external], Outcome: "refuted", Path: callee, Quote: newReturn, Line: line, Candidate: "c-0", RefutedText: falseFinding,
		SourcePath: policy, SourceQuote: policyLimit, Reason: "The upper bound still caps the result at 100 cents.",
		Trigger: "A normal quote for a raw amount of 100", Effect: "returns 101 cents"}
	express := refute
	express.RefutedText, express.Trigger = expressFinding, "An express quote for a raw amount of 100"
	normalOnly := express
	normalOnly.Trigger = "A normal quote of 100"
	differentEffect := refute
	differentEffect.Effect = "exceeding the invoice limit"
	missingTrigger := express
	missingTrigger.Trigger = ""
	unresolved := refute
	unresolved.counterproof = true
	if selectBusinessContext(snapshot, []model.Diff{change}).proves(byID[refute.ID], unresolved, nil) {
		t.Fatal("an inferred caller rule withdrew a finding without resolving its external policy")
	}
	effect, err := json.Marshal(map[string]any{"checks": []map[string]any{{
		"id": "e-0", "verdict": "confirmed", "input": "ten cents", "expected": "10", "before": "10", "after": "11",
		"reason": "The changed expression adds one cent before the cap.", "output_path": callee, "output_line": line, "output_quote": newReturn,
		"assertion_quote": direct.SourceQuote, "assertion_before_passes": true, "assertion_after_passes": false,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	mixedRules := []string{mixed, local, external, pure, downstream, other}
	for _, tc := range []struct {
		name               string
		rules              []string
		first, independent *businessCheck
		keepDraft          bool
	}{
		{"mixed group", mixedRules, &refute, &refute, false},
		{"normal cannot refute express", mixedRules, &express, &normalOnly, true},
		{"the two reviews must identify the same claimed effect", mixedRules, &refute, &differentEffect, true},
		{"missing first trigger cannot nominate", mixedRules, &missingTrigger, nil, true},
		{"all rules have a caller", []string{mixed, local, external, pure, downstream}, &refute, &refute, false},
		{"sole linked rule with a direct test", []string{mixed}, nil, nil, false},
		{"sole linked rule with a caller test", []string{downstream}, nil, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keep := func(claim string) bool { return slices.Contains(tc.rules, claim) }
			var selected knowledge.Snapshot
			for _, f := range snapshot {
				if slices.ContainsFunc(tc.rules, func(claim string) bool { return ids[claim] == f.ID }) {
					selected = append(selected, f)
				}
			}
			g := FileGroup{Diffs: []model.Diff{change}}
			g.business = selectBusinessContext(selected, g.Diffs)
			if len(g.business.rules) != len(tc.rules) || len(g.business.discussions) != 0 {
				t.Fatalf("bad rule selection: %+v", g.business)
			}
			const noChecks = `{"checks":[]}`
			var replies []string
			if tc.first != nil {
				replies = append(replies, businessReply(t, direct, *tc.first), string(effect))
				if tc.independent != nil {
					replies = append(replies, businessReply(t, *tc.independent))
				}
			} else if keep(mixed) {
				replies = append(replies, businessReply(t, direct), string(effect))
			}
			client := &fakeAgentClient{}
			for _, answer := range append(replies, noChecks) {
				client.responses = append(client.responses, &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &answer}}}})
			}
			a := New(Args{RepoDir: dir, LLMClient: client, KnowledgeTree: &head, Template: template.Template{MaxTokens: 16000}})
			a.inputResolution.ResolvedBase = base.Commit
			if tc.first != nil {
				a.args.CommentCollector.Add(model.LlmComment{Path: callee, StartLine: line, EndLine: line, ExistingCode: newReturn, Content: tc.first.RefutedText})
			}
			a.verifyBusinessFindings(ctx, g, "", change.Diff)
			records := a.session.GetOrCreateFileSession(fileGroupKey(g.Diffs)).TaskRecords
			reviews := records[session.BusinessReviewTask]
			if !keep(mixed) {
				if len(reviews) != 1 || !strings.Contains(reviews[0].RequestMessages[0].ExtractText(), callerLookupPrompt) {
					t.Fatal("caller-only testimony should run a single caller assessment")
				}
				return
			}
			if len(reviews) != 2 {
				t.Fatalf("want standard and supplemental assessments, got %d", len(reviews))
			}
			first, second := reviews[0].RequestMessages, reviews[1].RequestMessages
			standard, supplemental := first[len(first)-1].ExtractText(), second[len(second)-1].ExtractText()
			if strings.Contains(first[0].ExtractText(), callerRefutationScenarioPrompt) != (tc.first != nil) ||
				strings.Contains(second[0].ExtractText(), callerRefutationScenarioPrompt) {
				t.Error("scenario instructions should only accompany ordinary findings to recheck")
			}
			refutations := records[session.BusinessRefutationTask]
			if tc.independent != nil && (len(refutations) != 1 || !strings.Contains(refutations[0].RequestMessages[0].ExtractText(), callerRefutationScenarioPrompt) ||
				strings.Contains(refutations[0].RequestMessages[1].ExtractText(), "cases_to_reproduce")) {
				t.Error("the independent reviewer should quote its own scenario from the original finding")
			}
			if strings.Contains(first[0].ExtractText(), callerLookupPrompt) || !strings.Contains(second[0].ExtractText(), callerLookupPrompt) {
				t.Error("the caller assessment ran before the direct assessment")
			}
			for _, claim := range []string{pure, downstream} {
				if strings.Contains(standard, claim) || strings.Contains(supplemental, claim) != keep(claim) {
					t.Errorf("caller-only claim in the wrong assessment: %s", claim)
				}
			}
			for _, evidence := range []struct{ claim, quote string }{{mixed, "assert direct_quote(10) == 10"}, {local, limit}, {external, policyLimit}, {other, ""}} {
				if keep(evidence.claim) && (!strings.Contains(standard, evidence.claim) || !strings.Contains(standard, evidence.quote)) {
					t.Errorf("the standard assessment lost %s or its current evidence", evidence.claim)
				}
			}
			final := a.args.CommentCollector.CommentsForPath(callee)
			wantFinal := 1
			if tc.keepDraft {
				wantFinal++
			}
			if len(final) != wantFinal || !slices.ContainsFunc(final, func(c model.LlmComment) bool { return strings.Contains(c.Content, missed) }) ||
				tc.keepDraft && !slices.ContainsFunc(final, func(c model.LlmComment) bool { return c.Content == tc.first.RefutedText }) ||
				(len(records[session.BusinessRefutationTask]) == 1) != (tc.independent != nil) {
				t.Errorf("unexpected final findings: %+v, %d refutations", final, len(records[session.BusinessRefutationTask]))
			}
		})
	}
}

func TestBusinessLookupFeedsRealSourceToTheNextAssessment(t *testing.T) {
	dir := initIdentityRepo(t)
	tree := &knowledge.Tree{Repo: dir, Workspace: true, Git: gitcmd.New(1)}
	base, err := tree.Git.Output(t.Context(), dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	const query = `{"lookups":[{"path":"keep.go","symbol":"keep"},{"side":"base","path":"gone.go","symbol":"gone"},{"path":"keep.go","symbol":"keep"}]}`
	const checks = `{"checks":[]}`
	messages := []llm.Message{llm.NewTextMessage("system", "review instructions"), llm.NewTextMessage("user", "selected evidence")}
	for _, tc := range []struct {
		name    string
		caller  bool
		answers []string
		ok      bool
	}{
		{"read current and base", true, []string{query, checks}, true},
		{"third lookup exceeds the allowance", true, []string{query, query, query}, false},
		{"missing repository uses original request", false, []string{checks}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeAgentClient{}
			for _, answer := range tc.answers {
				client.responses = append(client.responses, &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &answer}}}})
			}
			a := New(Args{RepoDir: dir, KnowledgeTree: tree, LLMClient: client, Template: template.Template{MaxTokens: 16000}})
			a.inputResolution.ResolvedBase = strings.TrimSpace(string(base))
			if !tc.caller {
				a.args.KnowledgeTree = nil
			}
			rule := knowledge.Fact{Origin: "discovery"}
			if tc.caller {
				rule.AcceptedCall = &knowledge.AcceptedCall{CallerPath: "keep.go", CalleePath: "gone.go"}
			}
			g := FileGroup{Diffs: []model.Diff{{NewPath: "gone.go"}}, business: businessContext{rules: knowledge.Snapshot{rule}}}
			response, ok := a.reviewBusinessEvidence(t.Context(), "test-group", &g, messages)
			if ok != tc.ok || ok && response != checks || client.calls != len(tc.answers) {
				t.Fatalf("review: %q, %v, %d calls", response, ok, client.calls)
			}
			tasks := a.session.GetOrCreateFileSession("test-group").TaskRecords[session.BusinessReviewTask]
			if !tc.caller {
				if len(tasks) != 1 || tasks[0].RequestMessages[0].ExtractText() != messages[0].ExtractText() {
					t.Fatal("a rule without a caller changed the existing prompt")
				}
				return
			}
			if len(g.business.lookupEvidence) != 2 || g.business.lookupEvidence[0].Revision != "workspace" || g.business.lookupEvidence[1].Revision != a.inputResolution.ResolvedBase {
				t.Fatalf("missing or duplicate source: %+v", g.business.lookupEvidence)
			}
			second := tasks[1].RequestMessages
			data := second[len(second)-1].ExtractText()
			if second[len(second)-2].Role != "assistant" || !strings.Contains(data, "func keep()") || !strings.Contains(data, "package gone") {
				t.Fatalf("final assessment did not receive the searched source: %s", data)
			}
		})
	}
}

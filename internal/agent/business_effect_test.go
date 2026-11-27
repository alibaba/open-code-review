// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
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

func TestDirectCalleeAssertionMustSupportTheProposedOutcome(t *testing.T) {
	const callee, caller, tests = "sizing.py", "queue.py", "test_sizing.py"
	const baseLimit, currentLimit = "LIMIT: int = 8", "LIMIT: int = 10"
	const baseMissing, currentMissing = "MISSING_SIZE = 13", "MISSING_SIZE = 14"
	const siblingLimit = "FALLBACK_LIMIT = 6"
	const oldReturn = "return None if count == MISSING_SIZE else min(count, LIMIT)"
	const oldSiblingReturn = "return min(count, FALLBACK_LIMIT)"
	const siblingReturn = "return 120 + count"
	const sibling = "\n\ndef fallback_width(items):\n    count = len(items)\n    "
	const before = baseLimit + "\n" + siblingLimit + "\n" + baseMissing + "\nasync def width(items):\n    count = len(items)\n    " + oldReturn + "\n" + sibling + oldSiblingReturn + "\n"
	const changedReturn = "return None if count == MISSING_SIZE else max(count, LIMIT)"
	const after = siblingLimit + "\n" + currentLimit + "\n" + currentMissing + "\nasync def width(items):\n    count = len(items)\n    " + changedReturn + "\n" + sibling + siblingReturn + "\n"
	const consumer = "from .sizing import width\n\nasync def enqueue(items):\n    return await width(items)\n"
	const weak = "assert await width(range(12)) is not None"
	const strong = "assert await width(range(12)) == 8"
	const weakMixed = "assert await width(range(0, 12)) is not None"
	const strongMixed = "assert await width(range(0, 12)) == 8"
	const lookup = "def lookup(value):\n    return 'a batch plan' if value <= 8 else None"
	const wrapped = "assert lookup(await width(range(12))) is not None"
	const normalize = "def normalize(value):\n    return value if value is not None else 8"
	const defaulted = "assert normalize(await width(range(14))) is not None"
	const clamp = "def clamp(value):\n    return 8 if value > 8 else value"
	const delegated = "def normalized_width(value):\n    return clamp(value)\n\nasync def test_normalized_width():\n    "
	const checkedDelegate = "assert normalized_width(await width(range(12))) == 8"
	const sharedPlan = "PLAN = 'a batch plan'"
	const externalWrapper = "def external_plan(value):\n    return PLAN if value <= 8 else None\n\nasync def test_external_plan():\n    "
	const checkedExternal = "assert external_plan(await width(range(12))) is not None"
	const multilineStart = "assert await width("
	const multiline = multilineStart + "\n        range(12),\n    ) == 8"
	const neighbour = "assert await width(range(11)) == 8"
	const withNeighbour = multiline + "\n    " + neighbour
	const noneToPresent = "assert await width(range(13)) is None"
	const presentToNone = "assert await width(range(14)) is not None"
	const reverseNone = "assert await width(range(14)) is None"
	const reversePresent = "assert await width(range(13)) is not None"
	const source = "from .sizing import width\n\nasync def test_a_result_is_present():\n    " + weak + "\n\n" +
		"async def test_an_oversized_batch_is_capped():\n    " + strong + "\n\n" +
		"async def test_both_properties_of_a_batch():\n    " + weakMixed + "\n    " + strongMixed + "\n\n" +
		lookup + "\n\nasync def test_a_batch_plan_exists():\n    " + wrapped + "\n\n" +
		normalize + "\n\nasync def test_missing_result_is_defaulted():\n    " + defaulted + "\n\n" +
		clamp + "\n\n" + delegated + checkedDelegate + "\n\n" +
		sharedPlan + "\n\n" + externalWrapper + checkedExternal + "\n\n" +
		"async def test_multiline_width():\n    " + withNeighbour + "\n\n" +
		"async def test_none_directions():\n    " + noneToPresent + "\n    " + presentToNone + "\n    " + reverseNone + "\n    " + reversePresent + "\n"
	const ruleClaim = "A batch of twelve items has a width of at most eight items."
	const finding = "A batch of twelve items now returns a width of 12 and exceeds the eight-item cap."
	dir := initPreviewRepo(t)
	commitIn(t, dir, callee, before, "return the accepted capped batch width")
	commitIn(t, dir, caller, consumer, "enqueue the selected batch width")
	commitIn(t, dir, tests, source, "record separate presence and cap assertions")
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
	commitIn(t, dir, callee, after, "remove the cap from the batch computation")
	head := knowledge.Tree{Repo: dir, Git: git, Commit: strings.TrimSpace(output("rev-parse", "HEAD"))}
	change := model.Diff{OldPath: callee, NewPath: callee, NewFileContent: after, Diff: output("diff", base.Commit, head.Commit, "--", callee)}
	for _, tc := range []struct {
		name, witness, premise, quote string
		input, before, after          string
		changed, output               string
		passedAfter                   any
		publish, withholdEffect       bool
	}{
		{name: "presence accepts the claimed counterexample", witness: weak, passedAfter: true},
		{name: "presence cannot distinguish two non-None results", witness: weak, passedAfter: false},
		{name: "adjacent numeric test cannot strengthen the weak line", witness: weakMixed + "\n    " + strongMixed, premise: weakMixed, passedAfter: false},
		{name: "the explicit result rejects the counterexample", witness: strong, passedAfter: false, publish: true},
		{name: "the wrapper rejects an over-limit result", witness: lookup + "\n\nasync def test_a_batch_plan_exists():\n    " + wrapped, premise: wrapped, passedAfter: false, publish: true},
		{name: "the wrapper body must be included", witness: wrapped, passedAfter: false, withholdEffect: true},
		{name: "an omitted defaulting wrapper cannot authorize a failure", witness: defaulted, input: "fourteen items", before: "8", after: "None", passedAfter: false, withholdEffect: true},
		{name: "the cited defaulting wrapper still passes", witness: normalize + "\n\nasync def test_missing_result_is_defaulted():\n    " + defaulted,
			premise: defaulted, input: "fourteen items", before: "8", after: "None", passedAfter: true},
		{name: "the wrapper delegates to a missing clamp", witness: delegated + checkedDelegate, premise: checkedDelegate, passedAfter: false, withholdEffect: true},
		{name: "the wrapper reads an external value", witness: externalWrapper + checkedExternal, premise: checkedExternal, passedAfter: false, withholdEffect: true},
		{name: "the assertion cannot authorize a changed sibling", witness: strong, changed: siblingReturn, output: siblingReturn, passedAfter: false, withholdEffect: true},
		{name: "the output cannot cite a changed sibling", witness: strong, output: siblingReturn, passedAfter: false},
		{name: "confirmation omits the predicate outcome", witness: strong},
		{name: "is None passes only with an absent base value", witness: noneToPresent, input: "thirteen items", before: "None", after: "13", passedAfter: false, publish: true},
		{name: "is not None cannot pass with an absent base value", witness: reversePresent, input: "thirteen items", before: "None", after: "13", passedAfter: false},
		{name: "is not None fails only with an absent reviewed value", witness: presentToNone, input: "fourteen items", before: "8", after: "None", passedAfter: false, publish: true},
		{name: "is None cannot fail with an absent reviewed value", witness: reverseNone, input: "fourteen items", before: "8", after: "None", passedAfter: false},
		{name: "the complete multiline comparison is enough", witness: multiline, passedAfter: false, publish: true},
		{name: "a citation ending inside the call leaves no direct predicate", witness: multilineStart, passedAfter: false, withholdEffect: true},
		{name: "the verifier cannot return only the opening line", witness: multiline, quote: multilineStart, passedAfter: false},
		{name: "the verifier cannot append a neighbouring assertion", witness: withNeighbour, premise: multiline, quote: withNeighbour, passedAfter: false},
		{name: "a neighbouring test of another input cannot replace the selected predicate", witness: withNeighbour, premise: multiline, quote: neighbour, passedAfter: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := uniqueBusinessQuoteLine(source, tc.witness)
			if start == 0 {
				t.Fatalf("ambiguous fixture citation: %s", tc.witness)
			}
			end := start + strings.Count(tc.witness, "\n")
			premise, proof := tc.premise, tc.quote
			if premise == "" {
				premise = tc.witness
			}
			if proof == "" {
				proof = premise
			}
			input, beforeValue, afterValue, claim, reason := tc.input, tc.before, tc.after, ruleClaim, finding
			if input == "" {
				input, beforeValue, afterValue = "twelve items", "8", "12"
			} else {
				claim = "For " + input + ", this predicate must hold: " + premise
				reason = "For " + input + ", the width changed from " + beforeValue + " to " + afterValue + "."
			}
			fact, err := base.Prepare(ctx, knowledge.Fact{Origin: "discovery", Category: knowledge.CategoryBusinessRule, Claim: claim,
				Scope: callee, Source: callee, Authority: knowledge.AuthorityInferred, Citations: []knowledge.Citation{
					{Kind: "code", Path: callee, StartLine: 5, EndLine: 5},
					{Kind: "code", Path: caller, StartLine: 4, EndLine: 4},
					{Kind: "test", Path: tests, StartLine: start, EndLine: end},
				}})
			if err != nil {
				t.Fatal(err)
			}
			// Restore the caller from the legacy discovery stamp against Git.
			fact.Citations[1].Symbol = "enqueue"
			checked, err := head.CheckReview(ctx, base.Commit, []knowledge.Fact{fact})
			if err != nil {
				t.Fatal(err)
			}
			snapshot := knowledge.ReviewSnapshot(checked)
			if err := snapshot.DescribeReviewChanges(ctx, base, head, checked); err != nil {
				t.Fatal(err)
			}
			if len(snapshot) != 1 || snapshot[0].AcceptedCall == nil {
				t.Fatalf("accepted call was not restored: %+v", snapshot)
			}
			changed, returned := tc.changed, tc.output
			if changed == "" {
				changed = changedReturn
			}
			if returned == "" {
				returned = changedReturn
			}
			direct := businessReply(t, businessCheck{ID: snapshot[0].ID, Path: callee, Outcome: "violation", Severity: "medium",
				Line: uniqueBusinessQuoteLine(after, changed), Quote: changed, SourcePath: tests, SourceQuote: premise, Reason: reason})
			answer, err := json.Marshal(map[string]any{"checks": []map[string]any{{
				"id": "e-0", "verdict": "confirmed", "input": input, "expected": beforeValue, "before": beforeValue, "after": afterValue,
				"reason": "The returned value follows the selected branch.", "output_path": callee, "output_line": uniqueBusinessQuoteLine(after, returned), "output_quote": returned,
				"assertion_quote": proof, "assertion_before_passes": true, "assertion_after_passes": tc.passedAfter,
			}}})
			if err != nil {
				t.Fatal(err)
			}
			client := &fakeAgentClient{}
			for _, reply := range []string{direct, string(answer), `{"checks":[]}`} {
				client.responses = append(client.responses, &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}}})
			}
			a := New(Args{RepoDir: dir, LLMClient: client, KnowledgeTree: &head, Template: template.Template{MaxTokens: 16000}, SkipFilter: true})
			a.inputResolution.ResolvedBase = base.Commit
			const ordinary = "An independent issue from the ordinary review."
			a.args.CommentCollector.Add(model.LlmComment{Path: callee, StartLine: 1, EndLine: 1, Content: ordinary})
			g := FileGroup{Diffs: []model.Diff{change}}
			g.business = selectBusinessContext(snapshot, g.Diffs)
			a.verifyBusinessFindings(ctx, g, "", change.Diff)
			comments := a.args.CommentCollector.CommentsForPath(callee)
			records := a.session.GetOrCreateFileSession(fileGroupKey(g.Diffs)).TaskRecords[session.BusinessEffectTask]
			wantComments, wantChecks := 1, 1
			if tc.publish {
				wantComments++
			}
			if tc.withholdEffect {
				wantChecks = 0
			}
			if len(comments) != wantComments || comments[0].Content != ordinary ||
				tc.publish && !strings.Contains(comments[1].Content, reason) || len(records) != wantChecks {
				t.Fatalf("published = %+v, effect checks = %d, wanted publication = %t", comments, len(records), tc.publish)
			}
			if tc.withholdEffect {
				return
			}
			payload := records[0].RequestMessages[1].ExtractText()
			var request struct {
				Proposals []effectInput        `json:"proposals"`
				Functions []knowledge.Citation `json:"functions"`
				Patches   map[string]string    `json:"patches"`
			}
			if err := json.Unmarshal([]byte(payload), &request); err != nil || len(request.Proposals) != 1 {
				t.Fatalf("expected one effect proposal: %s", payload)
			}
			proposal := request.Proposals[0]
			if proposal.Assertion == nil || !strings.Contains(proposal.Assertion.Source.Quote, premise) ||
				!strings.Contains(proposal.Assertion.Rule, claim) {
				t.Fatalf("effect check omitted the asserted rule: %s", payload)
			}
			if !slices.Equal(proposal.Routines, []int{0}) || len(request.Functions) != 1 ||
				request.Functions[0].Path != callee || request.Functions[0].EndLine != 6 ||
				!strings.Contains(request.Functions[0].Quote, changedReturn) || proposal.BaseFunction == nil ||
				proposal.BaseFunction.Path != callee || proposal.BaseFunction.EndLine != 6 ||
				!strings.Contains(proposal.BaseFunction.Quote, oldReturn) || len(request.Patches) != 0 ||
				strings.Contains(payload, "fallback_width") || strings.Contains(payload, siblingLimit) {
				t.Fatalf("effect check must compare only the resolved callee: %s", payload)
			}
			baseDeclarations := []knowledge.Citation{
				{Kind: "code", Path: callee, StartLine: 1, EndLine: 1, Quote: baseLimit},
				{Kind: "code", Path: callee, StartLine: 3, EndLine: 3, Quote: baseMissing},
			}
			currentDeclarations := []knowledge.Citation{
				{Kind: "code", Path: callee, StartLine: 2, EndLine: 2, Quote: currentLimit},
				{Kind: "code", Path: callee, StartLine: 3, EndLine: 3, Quote: currentMissing},
			}
			if !slices.Equal(proposal.BaseDeclarations, baseDeclarations) ||
				!slices.Equal(proposal.CurrentDeclarations, currentDeclarations) {
				t.Fatalf("effect check omitted or mixed up the two revisions' bound literals: %s", payload)
			}
			if premise == wrapped && !strings.Contains(proposal.Assertion.Source.Quote, lookup) ||
				premise == defaulted && !strings.Contains(proposal.Assertion.Source.Quote, normalize) {
				t.Fatalf("effect check omitted the wrapper implementation: %s", payload)
			}
		})
	}
}

func TestBusinessEffectChecksTheFunctionThatProducesTheOutcome(t *testing.T) {
	const file = "account.go"
	const source = "package account\n" +
		"func allocate(accepted, price int) int {\n\treturn accepted * price\n}\n" +
		"func completion(accepted, rejected, price, paid int) int {\n" +
		"\tlimit := (accepted + rejected) * price\n\tif paid > limit { return -1 }\n" +
		"\treturn 100 * paid / (accepted * price)\n}\n"
	changes := []model.Diff{{NewPath: file, NewFileContent: source, Diff: "@@ -3 +3 @@\n- return total\n+ return accepted * price"}}
	addition := businessAddition{LlmComment: model.LlmComment{Path: file, StartLine: 3, Content: "The account remains incomplete."},
		check: businessCheck{Path: file, Quote: "return accepted * price", SourcePath: file, SourceQuote: "limit := (accepted + rejected) * price",
			Reason: "The account remains incomplete."}}
	for _, tc := range []struct {
		name, verdict, after, output string
		line                         int
		kept                         bool
	}{
		{"false intermediate denominator", "refuted", "100%", "return 100 * paid / (accepted * price)", 8, false},
		{"unresolved downstream effect", "unknown", "", "", 0, false},
		{"confirmed separate outcome", "confirmed", "90%", "return 100 * paid / (accepted * price)", 8, true},
		{"equal observable", "confirmed", "100%", "return 100 * paid / (accepted * price)", 8, false},
		{"invented terminal expression", "confirmed", "90%", "return 100 * paid / limit", 8, false},
		{"wrong location", "confirmed", "90%", "return 100 * paid / (accepted * price)", 6, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"checks": []map[string]any{{"id": "e-0", "verdict": tc.verdict, "input": "a partially paid account",
				"expected": "100%", "before": "100%", "after": tc.after, "reason": "The final percentage follows the returned expression.",
				"output_path": file, "output_line": tc.line, "output_quote": tc.output}}})
			if err != nil {
				t.Fatal(err)
			}
			reply := string(body)
			client := &fakeAgentClient{responses: []*llm.ChatResponse{{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}}}}}
			a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 16000}})
			original := model.LlmComment{Path: file, StartLine: 3, Content: "A separate confirmed finding."}
			a.args.CommentCollector.Add(original)
			got := a.verifyBusinessEffects(t.Context(), "account", changes, []businessAddition{addition})
			if client.calls != 1 || (len(got) == 1) != tc.kept || len(a.args.CommentCollector.CommentsForPath(file)) != 1 {
				t.Fatalf("published additions = %+v, calls = %d", got, client.calls)
			}
		})
	}
	if n := uniqueBusinessQuoteLine("same\nother\nsame", "same"); n != 0 {
		t.Fatalf("ambiguous function premise at line %d", n)
	}
	if n := uniqueBusinessQuoteLine(source, "limit := (accepted + rejected) * price\n\tif paid > limit { return -1 }"); n != 6 {
		t.Fatalf("actual multiline premise at line %d", n)
	}
	if n := uniqueBusinessQuoteLine(source, strings.TrimSpace("")); n != 0 {
		t.Fatal("blank premise selected")
	}
}

func TestBusinessEffectVerifiesTheOutcomeWithAModuleDeclaration(t *testing.T) {
	const file, premise = "billing.go", "const ceiling = 100"
	const function = "func approved(amount int) int {\n\tif amount > ceiling { return 0 }\n\treturn amount\n}"
	const source = "package billing\n" + premise + "\n" + function + "\n"
	const patch = "@@ -4 +4 @@\n-\tif amount > ceiling { return ceiling }\n+\tif amount > ceiling { return 0 }"
	changes := []model.Diff{{NewPath: file, NewFileContent: source, Diff: patch}}
	for _, tc := range []struct {
		name, sourceQuote, verdict, output string
		line                               int
		called, kept                       bool
	}{
		{"confirmed", premise, "confirmed", "if amount > ceiling { return 0 }", 4, true, true},
		{"refuted", premise, "refuted", "if amount > ceiling { return 0 }", 4, true, false},
		{"outcome cites only the declaration", premise, "confirmed", premise, 2, true, false},
		{"invented declaration", "const ceiling = 120", "confirmed", "if amount > ceiling { return 0 }", 4, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"checks": []map[string]any{{"id": "e-0", "verdict": tc.verdict, "input": "an amount of 150",
				"expected": "100", "before": "100", "after": "0", "reason": "The changed branch returns zero above the ceiling.",
				"output_path": file, "output_line": tc.line, "output_quote": tc.output}}})
			if err != nil {
				t.Fatal(err)
			}
			reply := string(body)
			client := &fakeAgentClient{responses: []*llm.ChatResponse{{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}}}}}
			a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 16000}})
			ordinary := model.LlmComment{Path: file, StartLine: 4, Content: "An existing ordinary review issue."}
			a.args.CommentCollector.Add(ordinary)
			addition := businessAddition{LlmComment: model.LlmComment{Path: file, StartLine: 4, Content: "Amounts above the ceiling now return zero."},
				check: businessCheck{Path: file, SourcePath: file, SourceQuote: tc.sourceQuote, Reason: "Amounts above the ceiling now return zero."}}
			got := a.verifyBusinessEffects(t.Context(), "billing", changes, []businessAddition{addition})
			keptOrdinary := a.args.CommentCollector.CommentsForPath(file)
			if (len(got) == 1) != tc.kept || len(keptOrdinary) != 1 || keptOrdinary[0].Content != ordinary.Content {
				t.Fatalf("additions = %+v, ordinary = %+v", got, keptOrdinary)
			}
			calls := a.session.GetOrCreateFileSession("billing").TaskRecords[session.BusinessEffectTask]
			wantCalls := 0
			if tc.called {
				wantCalls = 1
			}
			if len(calls) != wantCalls || client.calls != wantCalls {
				t.Fatalf("verification calls = %d, client calls = %d", len(calls), client.calls)
			}
			if !tc.called {
				return
			}
			var payload struct {
				Proposals    []effectInput        `json:"proposals"`
				Functions    []knowledge.Citation `json:"functions"`
				Declarations []knowledge.Citation `json:"declarations"`
				Patches      map[string]string    `json:"patches"`
			}
			if err := json.Unmarshal([]byte(calls[0].RequestMessages[1].ExtractText()), &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.Proposals) != 1 || len(payload.Functions) != 1 || len(payload.Declarations) != 1 || payload.Patches[file] != patch ||
				payload.Functions[0].Quote != function || payload.Declarations[0].Quote != premise || payload.Declarations[0].StartLine != 2 {
				t.Fatalf("incomplete verifier evidence: %+v", payload)
			}
			p := payload.Proposals[0]
			if len(p.Routines) != 1 || p.Routines[0] != 0 || p.Declaration == nil || *p.Declaration != 0 {
				t.Fatalf("incorrect proposal evidence indices: %+v", p)
			}
		})
	}
}

func TestJavaScriptBusinessEffectWithRegularExpressionReachesVerifier(t *testing.T) {
	const path = "billing.js"
	const source = "const brackets = /[)]/g;\n" +
		"function amount(total, rejected) {\n  return total - rejected;\n}\n" +
		"function receipt(total, rejected) {\n  return amount(total, rejected) * 2;\n}\n"
	const response = `{"checks":[{"id":"e-0","verdict":"confirmed","input":"total=10,rejected=4","expected":"6","before":"6","after":"12","reason":"The receipt doubles the approved amount.","output_path":"billing.js","output_line":6,"output_quote":"return amount(total, rejected) * 2;"}]}`
	reply := response
	client := &fakeAgentClient{responses: []*llm.ChatResponse{{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}}}}}
	a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 16000}})
	addition := businessAddition{LlmComment: model.LlmComment{Path: path, StartLine: 6, Content: "The receipt doubles the approved amount."},
		check: businessCheck{Path: path, SourcePath: path, SourceQuote: "return total - rejected;", Reason: "The receipt doubles the approved amount."}}
	changes := []model.Diff{{NewPath: path, NewFileContent: source,
		Diff: "@@ -6 +6 @@\n-  return amount(total, rejected);\n+  return amount(total, rejected) * 2;"}}
	if got := a.verifyBusinessEffects(t.Context(), "billing", changes, []businessAddition{addition}); len(got) != 1 || client.calls != 1 {
		t.Fatalf("additions = %+v, verifier calls = %d", got, client.calls)
	}
}

func TestUnlocatedBusinessEffectsPreserveOrdinaryReview(t *testing.T) {
	const file = "billing.js"
	const premise = "  return total - rejected;"
	const regularExpression = "function allocate(total, rejected) {\n" + premise + "\n}\n" +
		"function percentage(name, total) {\n  return /[(]/.test(name) ? total : 100;\n}\n"
	for _, tc := range []struct{ name, content string }{
		{"unavailable full file", ""},
		{"ambiguous premise", regularExpression + "function second(total, rejected) {\n" + premise + "\n}\n"},
		{"unavailable function boundaries", strings.TrimSuffix(regularExpression, "}\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply := `{"checks":[]}`
			client := &fakeAgentClient{responses: []*llm.ChatResponse{{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}}}}}
			a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 16000}})
			original := model.LlmComment{Path: file, StartLine: 5, Content: "Another issue in the ordinary review."}
			a.args.CommentCollector.Add(original)
			addition := businessAddition{LlmComment: model.LlmComment{Path: file, StartLine: 5, Content: "The receipt remains partially invoiced."},
				check: businessCheck{Path: file, SourcePath: file, SourceQuote: premise, Reason: "The receipt remains partially invoiced."}}
			got := a.verifyBusinessEffects(t.Context(), "billing", []model.Diff{{NewPath: file, NewFileContent: tc.content}}, []businessAddition{addition})
			ordinary := a.args.CommentCollector.CommentsForPath(file)
			warnings := a.Warnings()
			if len(got) != 0 || len(ordinary) != 1 || ordinary[0].Content != original.Content || client.calls != 0 ||
				len(warnings) != 1 || warnings[0].Type != "business_effect_unverified" {
				t.Fatalf("additions = %+v, ordinary = %+v, warnings = %+v, calls = %d", got, ordinary, warnings, client.calls)
			}
		})
	}
}

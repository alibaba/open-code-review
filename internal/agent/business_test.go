// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestGroundBusinessChecksAtCurrentDiff(t *testing.T) {
	const path = "orders/draft.json"
	g, rule := schemaReviewGroup(model.Diff{OldPath: path, NewPath: path, Diff: "@@ -1 +1 @@\n-{\"currency\":\"EUR\"}\n+{\"currency\":\"USD\"}", NewFileContent: `{"currency":"USD"}`})
	if !strings.Contains(g.business.text, `"outcome":"violated"`) {
		t.Fatalf("missing comparison: %s", g.business.text)
	}
	good := businessCheck{ID: rule.ID, Path: path, Outcome: "violation", Quote: g.Diffs[0].NewFileContent,
		Reason: "New orders use a currency that the order contract rejects.", Severity: "high"}
	deleted := good
	deleted.Quote = `{"currency":"EUR"}`
	deleted.Reason = "Wrong-side claim."
	foreign := good
	foreign.ID = "unknown"
	ordinary := []model.LlmComment{{Path: path, StartLine: 1, EndLine: 1, Content: "An independent field is missing."}}
	add, remove, err := groundBusinessChecks(businessReply(t, good, deleted, foreign), g, ordinary)
	if err == nil || len(remove) != 0 || len(add) != 1 {
		t.Fatalf("expected one grounded amendment: add=%+v remove=%v err=%v", add, remove, err)
	}
	got := add[0]
	if got.Path != path || got.StartLine != 1 || got.EndLine != 1 || got.ExistingCode != good.Quote ||
		got.Severity != good.Severity || !strings.Contains(got.Content, good.Reason) || !strings.Contains(got.Content, rule.Source) {
		t.Fatalf("wrong finding: %+v", got)
	}
	if add, remove, err := groundBusinessChecks("plain text", g, ordinary); err == nil || len(add) != 0 || len(remove) != 0 {
		t.Fatalf("malformed assessment: add=%+v remove=%v err=%v", add, remove, err)
	}
}

func schemaReviewGroup(change model.Diff) (FileGroup, knowledge.Fact) {
	f := knowledge.Fact{ID: "order-currency", Source: "contracts/order.json", Scope: change.NewPath,
		Property: "currency", Keyword: "const", Value: json.RawMessage(`"EUR"`)}
	rows := knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: f, Status: knowledge.StatusVerified}})
	g := FileGroup{Diffs: []model.Diff{change}}
	g.business = selectBusinessContext(rows, g.Diffs)
	return g, f
}

func TestBusinessChecksDoNotRediscoverUnchangedSchemaViolation(t *testing.T) {
	const path = "orders/draft.json"
	for _, tc := range []struct {
		name, patch, current, quote string
		line                        int
	}{
		{
			name: "multiline",
			patch: "@@ -1,4 +1,4 @@\n {\n   \"currency\": \"USD\",\n" +
				"-  \"description\": \"Old label\"\n+  \"description\": \"New label\"\n }\n",
			current: "{\n  \"currency\": \"USD\",\n  \"description\": \"New label\"\n}\n",
			quote:   `"description": "New label"`,
			line:    3,
		},
		{
			name: "single line",
			patch: "@@ -1 +1 @@\n-{\"currency\":\"USD\",\"description\":\"Old label\"}\n" +
				"+{\"currency\":\"USD\",\"description\":\"New label\"}\n",
			current: `{"currency":"USD","description":"New label"}` + "\n",
			quote:   `{"currency":"USD","description":"New label"}`,
			line:    1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, rule := schemaReviewGroup(model.Diff{OldPath: path, NewPath: path, Diff: tc.patch, NewFileContent: tc.current})
			c := businessCheck{ID: rule.ID, Path: path, Outcome: "violation", Line: tc.line, Quote: tc.quote,
				Severity: "high", Reason: "The document uses dollars where the order contract requires euros."}
			add, remove, _ := groundBusinessChecks(businessReply(t, c), g, nil)
			if len(add) != 0 || len(remove) != 0 {
				t.Fatalf("a description edit was attributed to the existing currency mismatch: add=%+v remove=%v", add, remove)
			}
		})
	}
}

func TestBusinessSchemaWitnessMatchesChangedProperty(t *testing.T) {
	const path = "orders/draft.json"
	patch := "@@ -1,5 +1,5 @@\n {\n   \"currency\":\n-    \"EUR\",\n+    \"USD\",\n" +
		"-  \"description\": \"Old label\"\n+  \"description\": \"New label\"\n }\n"
	current := "{\n  \"currency\":\n    \"USD\",\n  \"description\": \"New label\"\n}\n"
	g, rule := schemaReviewGroup(model.Diff{OldPath: path, NewPath: path, Diff: patch, NewFileContent: current})
	good := businessCheck{ID: rule.ID, Path: path, Outcome: "violation", Line: 3, Quote: `"USD",`,
		Severity: "high", Reason: "New orders use a currency that the order contract rejects."}
	wrong := good
	wrong.Line, wrong.Quote = 4, `"description": "New label"`
	add, remove, err := groundBusinessChecks(businessReply(t, wrong, good), g, nil)
	if err == nil || len(remove) != 0 || len(add) != 1 || add[0].StartLine != 3 || add[0].ExistingCode != good.Quote {
		t.Fatalf("schema result was detached from its property: add=%+v remove=%v err=%v", add, remove, err)
	}
}

func TestDeletionWitnessStaysAtTheDeletedBoundary(t *testing.T) {
	const file = "policy.go"
	change := model.Diff{NewPath: file, Diff: "@@ -1,5 +1,4 @@\n prepare()\n oldContext()\n-requireApproval()\n persist()\n finish()"}
	lines := reviewedLines([]model.Diff{change})[file]
	for _, l := range lines {
		if l.edited != (l.text == "persist()") {
			t.Fatalf("unexpected deletion anchor: %+v", l)
		}
	}
	change.Diff = "@@ -1,5 +1,3 @@\n keep()\n-old()\n-old2()\n+new()\n trailing()\n-cleanup()"
	lines = reviewedLines([]model.Diff{change})[file]
	if len(lines) != 3 || lines[0].edited || !lines[1].edited || !lines[2].edited {
		t.Fatalf("mixed replacement and end-of-hunk deletion: %+v", lines)
	}
}

func TestCrossFileContractUsesTheActualCallArguments(t *testing.T) {
	const quote = "amount = settle("
	for _, tc := range []struct {
		path string
		args []string
		want bool
	}{
		{"invoice.py", []string{"min(item.balance, row.cap),", "row.cap,", ")"}, true},
		{"invoice.py", []string{"amount=-history[row.balance] + 2,", ")"}, true},
		{"invoice.py", []string{`payload={"currency": row.balance if active else 0},`, ")"}, true},
		{"invoice.py", []string{"*row.balance,", ")"}, true},
		{"invoice.go", []string{"round(item.balance),", ")"}, true},
		{"invoice.py", []string{"row.total,", ")", "save(item.balance)"}, false},
		{"invoice.py", []string{`"item.balance", # item.balance`, ")"}, false},
		{"invoice.py", []string{"lambda: row.balance,", ")"}, false},
		{"invoice.go", []string{"func() int { return row.balance },", ")"}, false},
		{"invoice.go", []string{`"item.balance", // item.balance`, ")"}, false},
		{"invoice.py", []string{"min(item.balance, row.cap),"}, false},
	} {
		lines := []changedLine{{9, quote, true}}
		for i, next := range tc.args {
			lines = append(lines, changedLine{10 + i, next, true})
		}
		check := businessCheck{Path: tc.path, Quote: quote, Line: 9, SourceQuote: "return ledger.balance"}
		if got := usesCitedMember(check, lines); got != tc.want {
			t.Errorf("%s %v: got %v, want %v", tc.path, tc.args, got, tc.want)
		}
	}
}

func TestBusinessRetrievalRanksCodeSymbolsAndSkipsFileHeaders(t *testing.T) {
	checked := []knowledge.Checked{
		{Fact: knowledge.Fact{ID: "a-unrelated", Source: "payments/policy.go", Scope: "payments", Identifiers: []string{"settle"}}, Status: knowledge.StatusCandidate},
		{Fact: knowledge.Fact{ID: "z-matching", Source: "payments/policy.go", Scope: "payments", Identifiers: []string{"refund"}}, Status: knowledge.StatusCandidate},
		{Fact: knowledge.Fact{ID: "name-only", Source: "inventory/storage.go", Scope: "inventory", Identifiers: []string{"orders"}}, Status: knowledge.StatusCandidate},
	}
	changes := []model.Diff{{NewPath: "payments/orders.go", Diff: "--- a/payments/orders.go\n+++ b/payments/orders.go\n@@ -3 +3 @@\n-old()\n+refund()"}}
	ctx := selectBusinessContext(knowledge.ReviewSnapshot(checked), changes)
	if len(ctx.rules) != 2 || ctx.rules[0].ID != "z-matching" || ctx.rules[1].ID != "a-unrelated" {
		t.Fatalf("ranked using metadata instead of touched code: %+v", ctx.rules)
	}
}

func businessReply(t *testing.T, checks ...businessCheck) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"checks": checks})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

const (
	routingReviewPath = "dispatch.go"
	routingPolicy     = "policy.go"
	routingSource     = `return priority == "rush"`
	dispatchQuote     = "enqueueExpress(order)"
)

func routingReviewGroup(status string) FileGroup {
	f := knowledge.Fact{ID: "express-policy", Category: knowledge.CategoryBusinessRule, Scope: routingReviewPath, Source: routingPolicy,
		Claim:     "Express routing is allowed for rush orders and rejected for standard orders.",
		Citations: []knowledge.Citation{{Kind: "code", Path: routingPolicy, Quote: routingSource, StartLine: 6, EndLine: 6}}}
	snap := knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: f, Status: status}})
	patch := "@@ -8,5 +8,5 @@\n" +
		" \tif isRush(order) {\n" +
		"-\t\tenqueueLegacy(order)\n" +
		"+\t\tenqueueExpress(order)\n" +
		" \t}\n" +
		"-\tenqueueStandard(order)\n" +
		"+\tenqueueExpress(order)\n" +
		" \taudit(order)\n"
	g := FileGroup{Diffs: []model.Diff{{NewPath: routingReviewPath, Diff: patch}}}
	g.business = selectBusinessContext(snap, g.Diffs)
	return g
}

func TestBusinessViolationRequiresChangedLineAndSeparateSource(t *testing.T) {
	g := routingReviewGroup(knowledge.StatusCandidate)
	valid := businessCheck{ID: "express-policy", Path: routingReviewPath, Outcome: "violation", Line: 11, Quote: dispatchQuote,
		SourcePath: routingPolicy, SourceQuote: routingSource, Severity: "high",
		Reason: "A standard order enters the express queue through the new unconditional call."}
	add, remove, err := groundBusinessChecks(businessReply(t, valid, valid), g, nil)
	if err != nil || len(add) != 1 || add[0].StartLine != 11 || len(remove) != 0 {
		t.Fatalf("grounded duplicate amendment: add=%+v remove=%v err=%v", add, remove, err)
	}
	for _, tc := range []struct {
		name   string
		change func(*businessCheck)
	}{
		{"fabricated premise", func(c *businessCheck) { c.SourceQuote = `return priority != "rush"` }},
		{"premise substring", func(c *businessCheck) { c.SourceQuote = `priority == "rush"` }},
		{"wrong source file", func(c *businessCheck) { c.SourcePath = "routing.go" }},
		{"unchanged context", func(c *businessCheck) { c.Line, c.Quote = 12, "audit(order)" }},
		{"ambiguous changed quote", func(c *businessCheck) { c.Line = 0 }},
		{"deleted quote", func(c *businessCheck) { c.Line, c.Quote = 0, "enqueueStandard(order)" }},
		{"consistent control", func(c *businessCheck) { c.Outcome = "consistent" }},
		{"unsupported severity", func(c *businessCheck) { c.Severity = "info" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := valid
			tc.change(&c)
			add, remove, err := groundBusinessChecks(businessReply(t, c), g, nil)
			if err == nil || len(add) != 0 || len(remove) != 0 {
				t.Fatalf("unsupported amendment: add=%+v remove=%v err=%v", add, remove, err)
			}
		})
	}
}

func TestBusinessExceptionRefutesOnlyTheLocatedDraft(t *testing.T) {
	g := routingReviewGroup(knowledge.StatusCurrent)
	const disputed = "Rush orders must not use the express queue."
	drafts := []model.LlmComment{
		{Path: routingReviewPath, StartLine: 9, EndLine: 9, ExistingCode: dispatchQuote, Content: disputed},
		{Path: routingReviewPath, StartLine: 12, EndLine: 12, ExistingCode: "audit(order)", Content: "A blocked audit writer stalls dispatch."},
		{Path: "refunds.go", StartLine: 9, EndLine: 9, ExistingCode: dispatchQuote, Content: disputed},
	}
	original := slices.Clone(drafts)
	valid := businessCheck{ID: "express-policy", Path: routingReviewPath, Outcome: "refuted", Line: 9, Quote: dispatchQuote,
		SourcePath: routingPolicy, SourceQuote: routingSource, Candidate: "c-0", RefutedText: disputed,
		Reason: "The rush guard confines this call to orders that the policy explicitly allows in the express queue."}
	add, remove, err := groundBusinessChecks(businessReply(t, valid), g, drafts)
	if err != nil || len(add) != 0 || len(remove) != 1 || remove[0].Candidate != "c-0" || !slices.Equal(drafts, original) {
		t.Fatalf("refutation affected unrelated drafts: add=%+v remove=%v err=%v", add, remove, err)
	}
	for _, tc := range []struct {
		name   string
		change func(*businessCheck)
	}{
		{"unrelated candidate", func(c *businessCheck) { c.Candidate = "c-1" }},
		{"same finding at another path", func(c *businessCheck) { c.Candidate = "c-2" }},
		{"same code at another line", func(c *businessCheck) { c.Line = 11 }},
		{"ambiguous quote", func(c *businessCheck) { c.Line = 0 }},
		{"invented finding text", func(c *businessCheck) { c.RefutedText = "Standard orders cannot use the express queue." }},
		{"deleted witness", func(c *businessCheck) { c.Quote = "enqueueLegacy(order)" }},
		{"fabricated policy", func(c *businessCheck) { c.SourceQuote = "return true" }},
		{"unrelated assessment", func(c *businessCheck) { c.Outcome = "uncertain" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := valid
			tc.change(&c)
			add, remove, err := groundBusinessChecks(businessReply(t, c), g, drafts)
			if err == nil || len(add) != 0 || len(remove) != 0 || !slices.Equal(drafts, original) {
				t.Fatalf("unsupported refutation: add=%+v remove=%v err=%v", add, remove, err)
			}
		})
	}
	for _, status := range []string{knowledge.StatusCandidate, knowledge.StatusConflict} {
		t.Run(status, func(t *testing.T) {
			add, remove, err := groundBusinessChecks(businessReply(t, valid), routingReviewGroup(status), drafts)
			if err == nil || len(add) != 0 || len(remove) != 0 {
				t.Fatalf("unconfirmed policy removed a draft: add=%+v remove=%v err=%v", add, remove, err)
			}
		})
	}
}

func TestBusinessContextBudgetAndRoleIsolation(t *testing.T) {
	snap := knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: knowledge.Fact{
		ID: "settlement", Source: "contract.go", Scope: "pay.go", Claim: "Orders settle after capture.",
	}, Status: knowledge.StatusCandidate}})
	g := FileGroup{Diffs: []model.Diff{{NewPath: "pay.go", OldPath: "legacy.go", IsRenamed: true, Diff: "@@ -5 +5 @@\n+settle()\n"}}}
	g.business = selectBusinessContext(snap, g.Diffs)
	for _, first := range []string{"system", "user"} {
		messages := []llm.Message{llm.NewTextMessage(first, "original message")}
		a := New(Args{RepoDir: t.TempDir(), Template: template.Template{MaxTokens: 10000}})
		result, ok := a.appendBusinessContext(messages, g, businessReviewPrompt)
		if !ok || result[0].Role != "system" || !strings.Contains(result[0].ExtractText(), businessReviewPrompt) || messages[0].ExtractText() != "original message" {
			t.Fatalf("role separation mismatch: %v", result)
		}
		if last := result[len(result)-1]; last.Role != "user" || !strings.Contains(last.ExtractText(), "after capture") {
			t.Fatalf("missing scoped evidence: %v", last)
		}
		a.args.Template.MaxTokens = 2
		if result, ok = a.appendBusinessContext(messages, g, businessReviewPrompt); ok || len(result) != len(messages) || len(a.Warnings()) == 0 {
			t.Fatal("an over-budget evidence set should produce a diagnostic and leave ordinary review available")
		}
	}
	other := []model.Diff{{NewPath: "other.go", Diff: "+unrelated()"}}
	if got := selectBusinessContext(snap, other); len(got.rules) != 0 {
		t.Fatal("unrelated group inherited a rule")
	}
}

func TestBusinessRefutationsNeedIndependentEvidence(t *testing.T) {
	g := routingReviewGroup(knowledge.StatusCurrent)
	const validFinding = "Rush orders lose a queued express retry when the queue is full."
	// A finding written in Chinese that rush orders must not use the express queue.
	const falseFinding = "\u52a0\u6025\u8ba2\u5355\u4e0d\u80fd\u4f7f\u7528\u5feb\u901f\u961f\u5217\u3002"
	base := businessCheck{ID: "express-policy", Outcome: "refuted", Path: routingReviewPath, Line: 9, Quote: dispatchQuote,
		SourcePath: routingPolicy, SourceQuote: routingSource, Reason: "First assessor's proposed dismissal."}
	one, two := base, base
	one.Candidate, one.RefutedText = "c-0", validFinding
	two.Candidate, two.RefutedText = "c-1", falseFinding
	confirmed := two
	confirmed.Trigger, confirmed.Effect = "A rush order reaches this branch.", "The finding says express routing is forbidden."
	confirmed.Reason = "The branch and policy establish that this order is eligible for express routing."
	otherSource := confirmed
	otherSource.SourceQuote = "return true"
	partial := confirmed
	partial.RefutedText = strings.TrimSuffix(falseFinding, "\u3002")
	noTrigger := confirmed
	noTrigger.Trigger = ""
	for _, tc := range []struct {
		name   string
		reply  string
		remove bool
	}{
		{"independently disproven", businessReply(t, confirmed), true},
		{"both findings possible", `{"checks":[]}`, false},
		{"malformed assessment", "no verdict", false},
		{"fabricated source", businessReply(t, otherSource), false},
		{"partial original claim", businessReply(t, partial), false},
		{"missing triggering input", businessReply(t, noTrigger), false},
		{"duplicate verdict", businessReply(t, confirmed, confirmed), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := businessReply(t, one, two)
			client := &fakeAgentClient{responses: []*llm.ChatResponse{
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &first}}}},
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &tc.reply}}}},
			}}
			a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 10000}})
			for _, finding := range []string{validFinding, falseFinding} {
				a.args.CommentCollector.Add(model.LlmComment{Path: routingReviewPath, StartLine: 9, EndLine: 9, ExistingCode: dispatchQuote, Content: finding})
			}
			a.verifyBusinessFindings(t.Context(), g, "Review all queue operations.", g.Diffs[0].Diff)
			comments := a.args.CommentCollector.CommentsForPath(routingReviewPath)
			wantCount := 2
			if tc.remove {
				wantCount = 1
			}
			if client.calls != 2 || len(comments) != wantCount || comments[0].Content != validFinding {
				t.Fatalf("calls=%d comments=%+v", client.calls, comments)
			}
			tasks := a.session.GetOrCreateFileSession(fileGroupKey(g.Diffs)).TaskRecords[session.BusinessRefutationTask]
			if len(tasks) != 1 {
				t.Fatalf("missing independent session record: %d", len(tasks))
			}
			var request strings.Builder
			for _, m := range tasks[0].RequestMessages {
				request.WriteString(m.ExtractText())
			}
			if strings.Contains(request.String(), base.Reason) || !strings.Contains(request.String(), validFinding) ||
				!strings.Contains(request.String(), falseFinding) || !strings.Contains(request.String(), dispatchQuote) {
				t.Fatal("second assessment received the first verdict or lost an original finding or its changed code")
			}
		})
	}
}

func TestBusinessReviewSkipsUnneededOrOverBudgetAdjudication(t *testing.T) {
	g := routingReviewGroup(knowledge.StatusCurrent)
	const finding = "Express routing is forbidden for rush orders."
	proposal := businessCheck{ID: "express-policy", Outcome: "refuted", Path: routingReviewPath, Line: 9, Quote: dispatchQuote,
		SourcePath: routingPolicy, SourceQuote: routingSource, Candidate: "c-0", RefutedText: finding, Reason: "The rush order is permitted."}
	for _, tc := range []struct {
		name   string
		reply  string
		skip   bool
		budget int64
	}{
		{"no refutation proposed", `{"checks":[]}`, false, 0},
		{"user keeps every ordinary finding", businessReply(t, proposal), true, 0},
		{"budget consumed by first assessment", businessReply(t, proposal), false, 5000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeAgentClient{responses: []*llm.ChatResponse{
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &tc.reply}}}, Usage: &llm.UsageInfo{PromptTokens: 4500, CompletionTokens: 500, TotalTokens: 5000}},
			}}
			a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 10000},
				SkipFilter: tc.skip, MaxTokensBudget: tc.budget})
			a.args.CommentCollector.Add(model.LlmComment{Path: routingReviewPath, StartLine: 9, EndLine: 9, ExistingCode: dispatchQuote, Content: finding})
			a.verifyBusinessFindings(t.Context(), g, "Review routing.", g.Diffs[0].Diff)
			tasks := a.session.GetOrCreateFileSession(fileGroupKey(g.Diffs)).TaskRecords
			if client.calls != 1 || len(tasks[session.BusinessRefutationTask]) != 0 || len(a.args.CommentCollector.CommentsForPath(routingReviewPath)) != 1 {
				t.Fatalf("calls=%d recorded tasks=%v", client.calls, tasks)
			}
		})
	}
}

func TestRecordedDecisionReassessesRecurringFindingWithoutANomination(t *testing.T) {
	g := routingReviewGroup(knowledge.StatusCurrent)
	decision := g.business.rules[0]
	decision.Category, decision.ReviewID, decision.Claim = knowledge.CategoryDecision, "MR-48", ""
	decision.Finding, decision.Reason = "Rush orders cannot use this queue.", "Express routing is allowed for rush orders."
	g.business = selectBusinessContext(knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: decision, Status: knowledge.StatusHistorical}}), g.Diffs)
	const repeated = "Expedited dispatch rejects the order when it enters the express queue."
	const genuine = "The audit operation can fail when its destination is full."
	v := businessCheck{ID: decision.ID, Candidate: "c-0", Outcome: "refuted", Path: routingReviewPath, Quote: dispatchQuote, Line: 9,
		RefutedText: repeated, SourcePath: routingPolicy, SourceQuote: routingSource, Trigger: "Express dispatch of a rush order.",
		Effect: "The reviewer says routing rejects it.", Reason: "The unchanged rush guard permits this queue."}
	for _, tc := range []struct {
		name, answer string
		remove       bool
	}{
		{"corroborated decision", businessReply(t, v), true},
		{"prior decision alone", `{"checks":[]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			empty := `{"checks":[]}`
			client := &fakeAgentClient{responses: []*llm.ChatResponse{
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &empty}}}},
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &tc.answer}}}},
			}}
			a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 10000}})
			a.args.CommentCollector.Add(model.LlmComment{Path: routingReviewPath, StartLine: 9, EndLine: 9, ExistingCode: dispatchQuote, Content: repeated})
			a.args.CommentCollector.Add(model.LlmComment{Path: routingReviewPath, StartLine: 12, EndLine: 12, ExistingCode: "audit(order)", Content: genuine})
			a.verifyBusinessFindings(t.Context(), g, "", g.Diffs[0].Diff)
			got := a.args.CommentCollector.CommentsForPath(routingReviewPath)
			if client.calls != 2 || len(got) == 0 || (len(got) == 1) != tc.remove || got[len(got)-1].Content != genuine {
				t.Fatalf("calls=%d comments=%+v", client.calls, got)
			}
		})
	}
}

func TestInheritedWitnessRequiresBothSidesOfTheSameReplacement(t *testing.T) {
	const p = "billing.js"
	const expr = `invoice.channel == "desk"`
	old, newLine := expr+" ? 0 : 1", expr+" || amount > 0 ? 0 : 1"
	change := model.Diff{NewPath: p, Diff: "@@ -10,2 +10,2 @@\n context()\n-" + old + "\n+" + newLine}
	base := businessCheck{Path: p, Line: 11, Quote: newLine, SourcePath: p, SourceQuote: "-" + old, Inherited: expr}
	if !showsInheritedExpression(base, []model.Diff{change}) {
		t.Fatal("exact preserved expression in a single replacement was rejected")
	}
	pair := base
	pair.SourceQuote += "\n+" + newLine
	if !showsInheritedExpression(pair, []model.Diff{change}) {
		t.Fatal("explicit quotes from both sides of the same replacement were rejected")
	}
	for _, tc := range []struct {
		name string
		edit func(*businessCheck, *model.Diff)
	}{
		{"unmarked old quote", func(c *businessCheck, _ *model.Diff) { c.SourceQuote = old }},
		{"invented reviewed quote", func(c *businessCheck, _ *model.Diff) { c.SourceQuote += "\n+somethingElse()" }},
		{"another line", func(c *businessCheck, _ *model.Diff) { c.Line = 10 }},
		{"another file", func(c *businessCheck, _ *model.Diff) { c.SourcePath = "other.js" }},
		{"partial property name", func(c *businessCheck, _ *model.Diff) { c.Inherited = "invoice.chan" }},
		{"different value", func(c *businessCheck, _ *model.Diff) { c.Inherited = "amount > 0" }},
		{"changed leading guard", func(c *businessCheck, d *model.Diff) {
			c.Quote = "enabled && " + newLine
			d.Diff = strings.ReplaceAll(d.Diff, "+"+newLine, "+"+c.Quote)
		}},
		{"separate replacement", func(c *businessCheck, d *model.Diff) {
			d.Diff = "@@ -9,3 +9,3 @@\n-" + old + "\n+somethingElse()\n context()\n-another()\n+" + newLine
		}},
		{"ambiguous deleted block", func(c *businessCheck, d *model.Diff) {
			d.Diff = strings.ReplaceAll(d.Diff, "-"+old, "-guard()\n-"+old)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, d := base, change
			tc.edit(&c, &d)
			if showsInheritedExpression(c, []model.Diff{d}) {
				t.Fatal("unverified inherited expression accepted")
			}
		})
	}
}

func TestInheritedWitnessCannotDismissAnotherIssueOnTheSameLine(t *testing.T) {
	const path = "checkout.go"
	const expr = `order.Mode == "desk"`
	old, edited := "if "+expr+" {", "if "+expr+" || total/units > 0 {"
	const finding = "When " + expr + " is false and units are zero, the new division panics."
	f := knowledge.Fact{ID: "mode-decision", Scope: path, Source: "modes.go", Category: knowledge.CategoryDecision, ReviewID: "MR-5",
		Finding: "Change the comparison " + expr + ".", Reason: "Desk orders are permitted.",
		Citations: []knowledge.Citation{{Kind: "code", Path: "modes.go", Quote: `return mode == "desk"`}}}
	g := FileGroup{Diffs: []model.Diff{{NewPath: path, Diff: "@@ -7 +7 @@\n-" + old + "\n+" + edited}}}
	g.business = selectBusinessContext(knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: f, Status: knowledge.StatusHistorical}}), g.Diffs)
	empty := `{"checks":[]}`
	incorrect := businessReply(t, businessCheck{ID: f.ID, Candidate: "c-0", Outcome: "refuted", RefutedText: finding,
		Path: path, Line: 7, Quote: edited, SourcePath: path, SourceQuote: "-" + old, Inherited: expr,
		Trigger: "Zero units on a non-desk order.", Effect: "Panics during evaluation.", Reason: "The desk comparison was already present."})
	client := &fakeAgentClient{responses: []*llm.ChatResponse{
		{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &empty}}}},
		{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &incorrect}}}},
	}}
	a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 10000}})
	a.args.CommentCollector.Add(model.LlmComment{Path: path, StartLine: 7, EndLine: 7, ExistingCode: edited, Content: finding})
	a.verifyBusinessFindings(t.Context(), g, "", g.Diffs[0].Diff)
	if client.calls != 2 || len(a.args.CommentCollector.CommentsForPath(path)) != 1 {
		t.Fatal("a decision about permitted modes authorized withdrawal of a new arithmetic failure")
	}
}

func TestPriorDecisionBindsAnInheritedExpressionToTheDisputedIssue(t *testing.T) {
	const expr = `order.Mode == "desk"`
	check := businessCheck{Inherited: expr, RefutedText: "Use strict equality for the desk comparison."}
	if !priorFindingMatches("Use strict equality\nfor the desk comparison.", check) {
		t.Fatal("line wrapping changed the identity of a prior decision")
	}
	check.RefutedText = "Consider rewriting " + expr + " in the dispatch condition."
	if priorFindingMatches("Replace the comparison "+expr+".", check) {
		t.Fatal("a rephrased finding needs current independent evidence")
	}
	if priorFindingMatches("Desk mode is a configured value.", check) {
		t.Fatal("mere topic overlap attributed an unrelated decision to the finding")
	}
}

func TestBusinessClaimNeedsItsCurrentPremise(t *testing.T) {
	src := knowledge.Citation{Kind: "test", Path: "settlement_test.go", Quote: "require.Zero(t, order.Fee)"}
	old := knowledge.Citation{Kind: "discussion", Path: "other.go", Quote: "we usually waive the fee"}
	f := knowledge.Fact{ID: "fee", Scope: "settlement.go", Claim: "Waive the fee for this order state.", Citations: []knowledge.Citation{old}, Conditions: []knowledge.Condition{{Description: "pending settlement", Citation: &src}}}
	check := businessCheck{SourcePath: src.Path, SourceQuote: src.Quote}
	rule := knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: f, Status: knowledge.StatusCandidate}})[0]
	if !supportsBusinessCheck(rule, check) {
		t.Fatal("lost matching current condition citation")
	}
	if supportsBusinessCheck(rule, businessCheck{SourcePath: old.Path, SourceQuote: old.Quote}) ||
		supportsBusinessCheck(rule, businessCheck{SourcePath: src.Path, SourceQuote: "order always has a fee"}) {
		t.Fatal("unsupported conclusion accepted")
	}
	disputed := knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: f, Status: knowledge.StatusConflict}})[0]
	if supportsBusinessCheck(disputed, check) {
		t.Fatal("a conflicting rule cannot authorize a verdict")
	}
}

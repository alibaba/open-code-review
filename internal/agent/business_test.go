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
	if err != nil || len(add) != 0 || len(remove) != 1 || !remove[0] || !slices.Equal(drafts, original) {
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

func TestIndependentFailureOnSameBusinessLineSurvives(t *testing.T) {
	g := routingReviewGroup(knowledge.StatusCurrent)
	const unrelated = "An empty work queue causes division by zero during batching."
	draft := model.LlmComment{Path: routingReviewPath, StartLine: 9, EndLine: 9, ExistingCode: dispatchQuote, Content: unrelated}
	check := businessCheck{ID: g.business.rules[0].ID, Outcome: "refuted", Path: routingReviewPath, Line: 9, Quote: dispatchQuote, SourcePath: routingPolicy, SourceQuote: routingSource,
		Candidate: "c-0", RefutedText: unrelated, Reason: "Rush orders use express routing."}
	_, remove, err := groundBusinessChecks(businessReply(t, check), g, []model.LlmComment{draft})
	if err == nil || len(remove) != 0 {
		t.Fatal("an applicable routing rule cannot disprove an unrelated failure on the same line")
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

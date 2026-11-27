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
	"github.com/alibaba/open-code-review/internal/session"
)

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

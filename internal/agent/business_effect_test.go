// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
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

func TestUnlocatedBusinessEffectsPreserveOrdinaryReview(t *testing.T) {
	const file = "billing.js"
	const premise = "  return total - rejected;"
	const regularExpression = "function allocate(total, rejected) {\n" + premise + "\n}\n" +
		"function percentage(name, total) {\n  return /[(]/.test(name) ? total : 100;\n}\n"
	for _, tc := range []struct{ name, content string }{
		{"unavailable full file", ""},
		{"ambiguous premise", regularExpression + "function second(total, rejected) {\n" + premise + "\n}\n"},
		{"unavailable function boundaries", regularExpression},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeAgentClient{}
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

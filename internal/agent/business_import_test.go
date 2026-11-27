// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"encoding/json"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/llm"
)

func TestBusinessImportRequiresAReproducedOutcomeAndActualCallee(t *testing.T) {
	const line = "def display(order):"
	const decision = "This service prints the shipping label."
	q := importQuestion{Outcome: "violation", Input: "one shipping order", Observable: "printed title", Expected: "recipient", Before: "recipient", After: "warehouse"}
	code := knowledge.ImportedReviewCode{Imports: map[string]string{"format_label": "display"}, Implementation: knowledge.Citation{Path: "label.py", StartLine: 1, EndLine: 2, Quote: line + "\n    return order.warehouse"}}
	for _, tc := range []struct {
		name, via, anchor, selection, after string
		keep                                bool
	}{
		{"reproduced through imported alias", "format_label", "implementation-1", decision, q.After, true},
		{"unrelated same-module function", "other_label", "implementation-1", decision, q.After, false},
		{"line does not link to the import", "format_label", "implementation-2", decision, q.After, false},
		{"unselected contract", "format_label", "implementation-1", "Other warehouses print this.", q.After, false},
		{"one generic word", "format_label", "implementation-1", "This", q.After, false},
		{"outcome unchanged", "format_label", "implementation-1", decision, q.Expected, false},
		{"invented declaration", "format_label", "caller-1", decision, q.After, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]any{"linked": true, "via": tc.via, "selection_quote": tc.selection, "implementation_anchor": tc.anchor,
				"input": q.Input, "observable": q.Observable, "expected": q.Expected, "before": q.Before, "after": tc.after, "reason": "The label uses the stored warehouse."})
			if err != nil {
				t.Fatal(err)
			}
			reply := string(data)
			client := &fakeAgentClient{responses: []*llm.ChatResponse{{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}}}}}
			a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 16000}})
			if got := a.traceBusinessImport(t.Context(), "label", q, []string{decision}, "change label", code); got != tc.keep || client.calls != 1 {
				t.Fatalf("verified = %v, calls = %d", got, client.calls)
			}
		})
	}
}

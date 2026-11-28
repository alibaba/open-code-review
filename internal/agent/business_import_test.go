// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
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

func TestBusinessImportQuotesTheBaseRevision(t *testing.T) {
	dir := initPreviewRepo(t)
	const caller, legacy, proposed = "ship.py", "legacy.py", "preferred.py"
	const oldCaller = "from .legacy import display as legacy_label\n\ndef ship(order):\n    return legacy_label(order)\n\ndef pickup(order):\n    return order.pickup_label\n"
	const newCaller = "from .preferred import render as label\n\ndef ship(order):\n    return label(order)\n\ndef pickup(order):\n    return order.pickup_label\n"
	const oldModule = "def display(order):\n    # return order.canceled_label\n    return order.recipient\n\ndef different(order):\n    return order.other_label\n"
	const newModule = "def render(order):\n    return order.warehouse\n"
	commitIn(t, dir, legacy, oldModule, "add the legacy label")
	commitIn(t, dir, proposed, newModule, "add another available label")
	commitIn(t, dir, caller, oldCaller, "select the legacy label")
	ctx := t.Context()
	runner := gitcmd.New(1)
	output := func(args ...string) string {
		t.Helper()
		data, err := runner.Output(ctx, dir, args...)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	base := strings.TrimSpace(output("rev-parse", "HEAD"))
	commitIn(t, dir, caller, newCaller, "select the warehouse label")
	head := strings.TrimSpace(output("rev-parse", "HEAD"))
	tree := knowledge.Tree{Repo: dir, Git: runner, Commit: head}
	patch := output("diff", "--unified=6", base, head, "--", caller)
	change := model.Diff{OldPath: caller, NewPath: caller, NewFileContent: newCaller, Diff: patch}
	code, ok := tree.ReviewImport(ctx, base, change, 4, proposed)
	if !ok || code.BaseCaller == nil || code.BaseImplementation == nil || code.BaseImplementation.Path != legacy {
		t.Fatalf("did not resolve the actual legacy binding: %+v", code)
	}
	const decision = "This service prints the recipient's shipping label."
	q := importQuestion{Outcome: "violation", Path: caller, SourcePath: proposed, Input: "one shipping order",
		Observable: "printed title", Expected: "recipient", Before: "recipient", After: "warehouse", line: 4}
	for _, tc := range []struct {
		name, quote string
		valid       bool
	}{
		{"actual old definition", "return order.recipient", true},
		{"actual old call", "return legacy_label(order)", true},
		{"comment in old implementation", "# return order.canceled_label", false},
		{"wrong implementation in the old module", "return order.other_label", false},
		{"other entry in the same hunk", "return order.pickup_label", false},
		{"new implementation available at base", "return order.warehouse", false},
		{"missing old evidence", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]any{"linked": true, "via": "label", "selection_quote": decision,
				"implementation_anchor": "implementation-1", "base_quote": tc.quote, "input": q.Input, "observable": q.Observable,
				"expected": q.Expected, "before": q.Before, "after": q.After, "reason": "The selected label changed."})
			if err != nil {
				t.Fatal(err)
			}
			reply := string(data)
			client := &fakeAgentClient{responses: []*llm.ChatResponse{{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}}}}}
			a := New(Args{RepoDir: dir, LLMClient: client, Template: template.Template{MaxTokens: 16000}})
			if got := a.traceBusinessImport(t.Context(), "label", q, []string{decision}, patch, code); got != tc.valid || client.calls != 1 {
				t.Fatalf("verified = %t, want %t, calls = %d", got, tc.valid, client.calls)
			}
			request := a.session.GetOrCreateFileSession("label").TaskRecords[session.BusinessImportTask][0].RequestMessages[1].ExtractText()
			var payload struct {
				Source knowledge.ImportedReviewCode `json:"source"`
			}
			if err := json.Unmarshal([]byte(request), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Source.BaseCaller == nil || !strings.Contains(payload.Source.BaseCaller.Quote, "return legacy_label(order)") ||
				payload.Source.BaseImplementation == nil || payload.Source.BaseImplementation.Path != legacy ||
				payload.Source.Implementation.Path != proposed || payload.Source.BaseImports["legacy_label"] != "display" {
				t.Fatalf("wrong verification sources: %+v", payload.Source)
			}
		})
	}
}

func TestBusinessImportRequiresAnAcceptedQuoteWhenTheHelperChangesInTheSameModule(t *testing.T) {
	dir := initPreviewRepo(t)
	const caller, module = "ship.py", "labels.py"
	const header = "from .labels import legacy_label, new_label\n\ndef ship(order):\n"
	const oldCall, newCall = "    return legacy_label(order)\n", "    return new_label(order)\n"
	const helpers = "def legacy_label(order):\n    return order.recipient\n\ndef new_label(order):\n    return order.warehouse\n"
	commitIn(t, dir, module, helpers, "add both label formatters")
	commitIn(t, dir, caller, header+oldCall, "select the recipient label")
	ctx := t.Context()
	runner := gitcmd.New(1)
	git := func(args ...string) string {
		t.Helper()
		data, err := runner.Output(ctx, dir, args...)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	base := strings.TrimSpace(git("rev-parse", "HEAD"))
	commitIn(t, dir, caller, header+newCall, "select the warehouse label")
	head := strings.TrimSpace(git("rev-parse", "HEAD"))
	tree := knowledge.Tree{Repo: dir, Git: runner, Commit: head}
	patch := git("diff", base, head, "--", caller)
	change := model.Diff{OldPath: caller, NewPath: caller, NewFileContent: header + newCall, Diff: patch}
	code, ok := tree.ReviewImport(ctx, base, change, 4, module)
	if !ok {
		t.Fatal("did not resolve the imported label")
	}
	const decision = "This service prints the recipient's shipping label."
	q := importQuestion{Outcome: "violation", Path: caller, SourcePath: module, Input: "one shipping order",
		Observable: "printed title", Expected: "recipient", Before: "recipient", After: "warehouse", line: 4}
	for _, tc := range []struct {
		name, quote string
		valid       bool
	}{
		{"missing base quote", "", false},
		{"new result in the same file", "return order.warehouse", false},
		{"accepted result in the same file", "return order.recipient", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]any{"linked": true, "via": "new_label", "selection_quote": decision,
				"implementation_anchor": "implementation-4", "base_quote": tc.quote, "input": q.Input, "observable": q.Observable,
				"expected": q.Expected, "before": q.Before, "after": q.After, "reason": "The selected label changed."})
			if err != nil {
				t.Fatal(err)
			}
			reply := string(data)
			client := &fakeAgentClient{responses: []*llm.ChatResponse{{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}}}}}
			a := New(Args{RepoDir: dir, LLMClient: client, Template: template.Template{MaxTokens: 16000}})
			if got := a.traceBusinessImport(ctx, "label", q, []string{decision}, patch, code); got != tc.valid || client.calls != 1 {
				t.Fatalf("verified = %t, want %t, calls = %d", got, tc.valid, client.calls)
			}
			request := a.session.GetOrCreateFileSession("label").TaskRecords[session.BusinessImportTask][0].RequestMessages
			var payload struct {
				Source knowledge.ImportedReviewCode `json:"source"`
			}
			if err := json.Unmarshal([]byte(request[1].ExtractText()), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Source.BaseCaller == nil || !strings.Contains(payload.Source.BaseCaller.Quote, strings.TrimSpace(oldCall)) ||
				payload.Source.BaseImports["legacy_label"] != "legacy_label" || payload.Source.BaseImplementation != nil ||
				payload.Source.Implementation.Quote != strings.TrimSpace(helpers) || !strings.Contains(request[0].ExtractText(), "base_quote") {
				t.Fatalf("missing accepted side of the shared module: %+v", payload.Source)
			}
		})
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"path"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func reviewImportChange(caller, entry, patch string) model.Diff {
	return model.Diff{OldPath: caller, NewPath: caller, NewFileContent: entry, Diff: patch}
}

func TestReviewImportResolvesTheCallAgainstTheReviewedTree(t *testing.T) {
	repo := newDiscoveryRepo(t)
	const caller = "orders/send.py"
	const source = "orders/labels.py"
	const entry = "from .labels import display as format_label\n\ndef send(order):\n    return format_label(order)\n"
	const helper = "def display(order):\n    return order.title\n"
	repo.write(caller, entry)
	repo.write(source, helper)
	repo.write("unused/labels.py", helper)
	tree := repo.commit("use the local label formatter")
	trace, ok := tree.ReviewImport(t.Context(), tree.Commit, reviewImportChange(caller, entry, ""), 4, source)
	if !ok || trace.Imports["format_label"] != "display" || !strings.Contains(trace.Caller.Quote, "def send(order)") || trace.Implementation.Quote != strings.TrimSpace(helper) {
		t.Fatalf("missing actual local import: %+v", trace)
	}
	for _, tc := range []struct{ text, target string }{{entry, "unused/labels.py"}, {entry + "# a newer workspace\n", source}} {
		if _, ok := tree.ReviewImport(t.Context(), tree.Commit, reviewImportChange(caller, tc.text, ""), 4, tc.target); ok {
			t.Fatal("connected a different file or a different revision")
		}
	}
	tree.AllowPath = func(p string) bool { return p != source }
	if _, ok := tree.ReviewImport(t.Context(), tree.Commit, reviewImportChange(caller, entry, ""), 4, source); ok {
		t.Fatal("included a filtered dependency")
	}
	repo.write("orders/labels/__init__.py", helper)
	head := repo.commit("introduce ambiguous package path")
	patch := repo.git("diff", tree.Commit, head.Commit, "--", caller)
	if _, ok := head.ReviewImport(t.Context(), tree.Commit, reviewImportChange(caller, entry, patch), 4, source); ok {
		t.Fatal("matched a module with a competing package")
	}
}

func TestReviewCallNamesExcludeLiteralTextAndShadowedPythonImports(t *testing.T) {
	const source = "from .labels import display\n\ndef send(order):\n    return display(order)\n\ndef relay(display):\n    return display()\n\ndef advise():\n    return 'display()' # display()\n"
	if got := reviewCallNames("orders.py", source, 4); len(got) != 1 || got[0] != "display" {
		t.Fatalf("actual imported call: %v", got)
	}
	for _, line := range []int{7, 10} {
		if got := reviewCallNames("orders.py", source, line); len(got) != 0 {
			t.Fatalf("line %d: %v", line, got)
		}
	}
}

func TestReviewImportReadsTheAcceptedAndProposedModules(t *testing.T) {
	repo := newDiscoveryRepo(t)
	const caller, module = "orders/ship.py", "orders/labels.py"
	const entry = "from .labels import display\n\ndef ship(order):\n    return display(order)\n"
	const old = "def display(order):\n    return order.recipient\n"
	const changed = "def display(order):\n    return order.warehouse\n"
	const oldEntry = "from .labels import display as recipient_label\n\ndef ship(order):\n    return recipient_label(order)\n"
	repo.write(caller, oldEntry)
	beforeModule := repo.commit("refer to an unavailable recipient label")
	repo.write(module, old)
	base := repo.commit("add recipient labels")
	repo.write(caller, entry)
	repo.write(module, changed)
	head := repo.commit("switch shipping labels to the warehouse")
	patch := repo.git("diff", base.Commit, head.Commit, "--", caller)
	trace, ok := head.ReviewImport(t.Context(), base.Commit, reviewImportChange(caller, entry, patch), 4, module)
	if !ok || trace.BaseImplementation == nil || trace.BaseImplementation.Revision != base.Commit ||
		trace.BaseImplementation.Quote != strings.TrimSpace(old) || trace.Implementation.Quote != strings.TrimSpace(changed) ||
		trace.BaseCaller == nil || trace.BaseImports["recipient_label"] != "display" {
		t.Fatalf("lost the actual revisions: %+v", trace)
	}
	if !trace.CitesBaseQuote("return order.recipient") || trace.CitesBaseQuote("return order.warehouse") {
		t.Fatal("did not distinguish the executable lines of the two revisions")
	}
	if _, ok := head.ReviewImport(t.Context(), beforeModule.Commit, reviewImportChange(caller, entry, patch), 4, module); ok {
		t.Fatal("claimed to verify an absent earlier implementation")
	}
}

func TestReviewImportUsesTheActualBaseCallerBinding(t *testing.T) {
	repo := newDiscoveryRepo(t)
	const caller, legacy, proposed = "orders/ship.py", "orders/legacy.py", "orders/proposed.py"
	const baseCaller = "from .legacy import print_label as label\n\ndef ship(order):\n    return label(order)\ndef other(order):\n    return unrelated(order)\n"
	const headCaller = "from .proposed import render as label\n\ndef ship(order):\n    return label(order)\ndef other(order):\n    return unrelated(order)\n"
	const old = "def print_label(order):\n" +
		"    \"\"\"Historical printer behavior.\n    return docstring_label\n    \"\"\"\n" +
		"    # return commented_label\n" +
		"    if order.confirmed:\n        return order.recipient\n" +
		"    return order.pending_label\n\n" +
		"def unrelated(order):\n    return order.other_label\n"
	const next = "def render(order):\n    return order.warehouse\n"
	repo.write(caller, baseCaller)
	repo.write(legacy, old)
	base := repo.commit("select the legacy printer")
	repo.write(caller, headCaller)
	repo.write(proposed, next)
	head := repo.commit("rebind the label import")
	// The call stays on the same line. The old module is reachable only through the base import.
	patch := repo.git("diff", "--unified=6", base.Commit, head.Commit, "--", caller)
	trace, ok := head.ReviewImport(t.Context(), base.Commit, reviewImportChange(caller, headCaller, patch), 4, proposed)
	if !ok || trace.BaseCaller == nil || trace.BaseImplementation == nil ||
		trace.BaseImplementation.Path != legacy || trace.BaseImports["label"] != "print_label" || trace.Imports["label"] != "render" {
		t.Fatalf("lost the accepted binding across the import changed at line 1: %+v", trace)
	}
	for _, tc := range []struct {
		quote string
		valid bool
	}{
		{"return label(order)", true},
		{"return order.recipient", true},
		{"# return commented_label", false},
		{"return docstring_label", false},
		{"return order.other_label", false},
		{"return order.warehouse", false},
		{"return unrelated(order)", false},
		{"def print_label(order):", false},
	} {
		if got := trace.CitesBaseQuote(tc.quote); got != tc.valid {
			t.Errorf("base quote %q accepted = %v, want %v", tc.quote, got, tc.valid)
		}
	}
	head.AllowPath = func(p string) bool { return p != legacy }
	if _, ok := head.ReviewImport(t.Context(), base.Commit, reviewImportChange(caller, headCaller, patch), 4, proposed); ok {
		t.Fatal("supplied an excluded accepted module")
	}
	renamed := strings.Replace(headCaller, "def ship(", "def dispatch(", 1)
	repo.write(caller, renamed)
	newEntry := repo.commit("rename the entry point")
	patch = repo.git("diff", "--unified=6", base.Commit, newEntry.Commit, "--", caller)
	if _, ok := newEntry.ReviewImport(t.Context(), base.Commit, reviewImportChange(caller, renamed, patch), 4, proposed); ok {
		t.Fatal("paired calls from different entry points")
	}
	unresolved := "import external_printer\n" + strings.Replace(baseCaller, "return label(order)", "return external_printer.print(label(order))", 1)
	repo.write(caller, unresolved)
	unknownBase := repo.commit("use an unresolvable outer printer")
	repo.write(caller, headCaller)
	knownHead := repo.commit("replace the external printer")
	patch = repo.git("diff", "--unified=6", unknownBase.Commit, knownHead.Commit, "--", caller)
	if _, ok := knownHead.ReviewImport(t.Context(), unknownBase.Commit, reviewImportChange(caller, headCaller, patch), 4, proposed); ok {
		t.Fatal("treated a known nested import as the complete old invocation")
	}
}

func TestReviewImportKeepsTheOldResultBindingInAnUnchangedModule(t *testing.T) {
	const caller, module = "orders/ship.py", "orders/labels.py"
	const header = "from .labels import legacy_label, new_label\n\ndef ship(order):\n"
	const legacy = "    return legacy_label(order)\n"
	const proposed = "    return new_label(order)\n"
	const helpers = "def legacy_label(order):\n    return order.recipient\n\ndef new_label(order):\n    return order.warehouse\n"
	for _, tc := range []struct {
		name, before, after string
		linked              bool
	}{
		{"binding changes", legacy, proposed, true},
		{"assignment and return change", "    order = order.confirmed\n" + legacy, "    order = order.pending\n" + proposed, true},
		{"text inside a string is not a return", "    note = '''An old note.\n    return unrelated(order)\n    '''\n" + legacy, proposed, true},
		{"multiple previous returns", "    if order.pending: return legacy_label(order)\n" + legacy, proposed, false},
		{"multiple new returns", legacy, "    if order.pending: return new_label(order)\n" + proposed, false},
		{"unrelated unchanged return", "    order = order.confirmed\n" + legacy, proposed + legacy, false},
		{"new branch without a replacement", legacy, proposed + legacy, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newDiscoveryRepo(t)
			repo.write(module, helpers)
			repo.write(caller, header+tc.before)
			base := repo.commit("use the existing recipient label")
			entry := header + tc.after
			repo.write(caller, entry)
			head := repo.commit("change the chosen label")
			patch := repo.git("diff", base.Commit, head.Commit, "--", caller)
			line := 1 + strings.Count(entry[:strings.LastIndex(entry, proposed)], "\n")
			trace, ok := head.ReviewImport(t.Context(), base.Commit, reviewImportChange(caller, entry, patch), line, module)
			if ok != tc.linked {
				t.Fatalf("linked = %t, want %t, patch:\n%s", ok, tc.linked, patch)
			}
			if !ok {
				return
			}
			if trace.BaseCaller == nil || trace.BaseCaller.Revision != base.Commit || trace.BaseImplementation != nil ||
				trace.BaseImports["legacy_label"] != "legacy_label" || trace.Imports["new_label"] != "new_label" ||
				!trace.CitesBaseQuote(legacy) || !trace.CitesBaseQuote("return order.recipient") ||
				trace.CitesBaseQuote(proposed) || trace.CitesBaseQuote("return order.warehouse") || trace.CitesBaseQuote("return unrelated(order)") {
				t.Fatalf("wrong old binding or quote: %+v", trace)
			}
		})
	}
}

func TestReviewImportBaseQuoteUsesExecutableLinesInGoAndJavaScript(t *testing.T) {
	for _, tc := range []struct {
		path, header, close, quotedReturn string
	}{
		{"labels.go", "func choose() string {", "}", `return "legacy"`},
		{"labels.mjs", "export function choose() {", "}", `return "legacy";`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			const comment = "return fake;"
			quote := tc.header + "\n  /*\n    " + comment + "\n  */\n  " + tc.quotedReturn + "\n" + tc.close
			code := ImportedReviewCode{baseDefinition: Citation{Path: tc.path, StartLine: 5, EndLine: 10, Quote: quote}}
			if !code.CitesBaseQuote(tc.quotedReturn) || code.CitesBaseQuote(comment) || code.CitesBaseQuote(tc.header) {
				t.Fatal("confused the implementation's executable line with other source text")
			}
		})
	}
}

func TestReviewImportResolvesGoAndJavaScriptAliases(t *testing.T) {
	for _, tc := range []struct {
		name, caller, source, prefix, function, implementation, alias, exported string
	}{
		{
			name: "Go package alias", caller: "orders/dispatch.go", source: "policy/capacity.go",
			prefix:         "package orders\n\nimport amounts \"example.test/shipment/policy\"\n\n",
			function:       "func dispatch(order int) int {\n\treturn amounts.Capacity(order)\n}",
			implementation: "package policy\n\nfunc Capacity(order int) int { return order + 1 }\n",
			alias:          "amounts.Capacity", exported: "Capacity",
		},
		{
			name: "JavaScript named import alias", caller: "orders/dispatch.mjs", source: "policy/capacity.mjs",
			prefix:         "import { capacity as available } from '../policy/capacity.mjs';\nconst bracket = /[)]/u;\n\n",
			function:       "export function dispatch(order) {\n  return available(order);\n}",
			implementation: "export function capacity(order) {\n  return order + 1;\n}\n",
			alias:          "available", exported: "capacity",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newDiscoveryRepo(t)
			entry := tc.prefix + tc.function + "\n"
			decoy := "unrelated/" + path.Base(tc.source)
			repo.write(tc.caller, entry)
			repo.write(tc.source, tc.implementation)
			repo.write(decoy, tc.implementation)
			if path.Ext(tc.caller) == ".go" {
				repo.write("go.mod", "module example.test/shipment\n")
				repo.write("policy/support.go", "package policy\nfunc Other(order int) int { return order - 1 }\n")
			}
			tree := repo.commit("use the imported capacity function")
			line := strings.Count(tc.prefix, "\n") + 2
			trace, ok := tree.ReviewImport(t.Context(), tree.Commit, reviewImportChange(tc.caller, entry, ""), line, tc.source)
			if !ok || len(trace.Imports) != 1 || trace.Imports[tc.alias] != tc.exported || trace.Caller.Path != tc.caller ||
				trace.Caller.StartLine != line-1 || trace.Caller.EndLine != line+1 || trace.Caller.Quote != tc.function ||
				trace.Implementation.Path != tc.source || trace.Implementation.Quote != strings.TrimSuffix(tc.implementation, "\n") {
				t.Fatalf("incorrect import trace: %+v", trace)
			}
			if trace, ok := tree.ReviewImport(t.Context(), tree.Commit, reviewImportChange(tc.caller, entry, ""), line, decoy); ok {
				t.Fatalf("same-named declaration in another file was linked: %+v", trace)
			}

			before, beforeStatement, _ := strings.Cut(tc.function, "\n")
			indent := "  "
			terminator := ";"
			if path.Ext(tc.caller) == ".go" {
				indent, terminator = "\t", ""
			}
			oldCaller := tc.prefix + before + "\n" + indent + "order += 1" + terminator + "\n" + beforeStatement + "\n"
			repo.write(tc.caller, oldCaller)
			base := repo.commit("adjust the accepted capacity input")
			newReturn := strings.Replace(beforeStatement, "(order)", "(order * 2)", 1)
			newCaller := tc.prefix + before + "\n" + indent + "order += 2" + terminator + "\n" + newReturn + "\n"
			repo.write(tc.caller, newCaller)
			head := repo.commit("adjust both lines of the proposed capacity call")
			patch := repo.git("diff", base.Commit, head.Commit, "--", tc.caller)
			trace, ok = head.ReviewImport(t.Context(), base.Commit, reviewImportChange(tc.caller, newCaller, patch), line+1, tc.source)
			if !ok || trace.BaseCaller == nil || trace.BaseImplementation != nil || trace.BaseImports[tc.alias] != tc.exported ||
				!trace.CitesBaseQuote(strings.SplitN(beforeStatement, "\n", 2)[0]) || trace.CitesBaseQuote(strings.SplitN(newReturn, "\n", 2)[0]) {
				t.Fatalf("did not pair the two result calls of the same alias: %+v", trace)
			}
		})
	}
}

func TestReviewImportChecksJavaScriptLocalBindings(t *testing.T) {
	repo := newDiscoveryRepo(t)
	const caller, source = "orders/dispatch.mjs", "orders/policy.mjs"
	const imports = "import { capacity as available } from './policy.mjs';\n\n"
	repo.write(source, "export function capacity(order) { return order + 1; }\n")
	for _, tc := range []struct {
		name, signature, setup string
		linked                 bool
	}{
		{"repeated imported calls", "order", "  order = available(order);\n", true},
		{"parameter", "order, available", "", false},
		{"lexical declaration", "order", "  const available = (n) => n - 1;\n", false},
		{"local function", "order", "  function available(n) { return n - 1; }\n", false},
		{"destructured parameter", "order, {available}", "", false},
		{"escaped parameter", `order, \u0061vailable`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := imports + "export function dispatch(" + tc.signature + ") {\n" + tc.setup
			entry := prefix + "  return available(order);\n}\n"
			repo.write(caller, entry)
			tree := repo.commit(tc.name)
			line := strings.Count(prefix, "\n") + 1
			trace, ok := tree.ReviewImport(t.Context(), tree.Commit, reviewImportChange(caller, entry, ""), line, source)
			if ok != tc.linked || ok && trace.Imports["available"] != "capacity" {
				t.Fatalf("trace = %+v, linked = %v, want %v", trace, ok, tc.linked)
			}
		})
	}
}

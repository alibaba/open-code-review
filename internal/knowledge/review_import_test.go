// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"path"
	"strings"
	"testing"
)

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
	trace, ok := tree.ReviewImport(t.Context(), caller, entry, 4, source)
	if !ok || trace.Imports["format_label"] != "display" || !strings.Contains(trace.Caller.Quote, "def send(order)") || trace.Implementation.Quote != strings.TrimSpace(helper) {
		t.Fatalf("missing actual local import: %+v", trace)
	}
	for _, tc := range []struct{ text, target string }{{entry, "unused/labels.py"}, {entry + "# a newer workspace\n", source}} {
		if _, ok := tree.ReviewImport(t.Context(), caller, tc.text, 4, tc.target); ok {
			t.Fatal("connected a different file or a different revision")
		}
	}
	tree.AllowPath = func(p string) bool { return p != source }
	if _, ok := tree.ReviewImport(t.Context(), caller, entry, 4, source); ok {
		t.Fatal("included a filtered dependency")
	}
	repo.write("orders/labels/__init__.py", helper)
	tree = repo.commit("introduce ambiguous package path")
	if _, ok := tree.ReviewImport(t.Context(), caller, entry, 4, source); ok {
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
			trace, ok := tree.ReviewImport(t.Context(), tc.caller, entry, line, tc.source)
			if !ok || len(trace.Imports) != 1 || trace.Imports[tc.alias] != tc.exported || trace.Caller.Path != tc.caller ||
				trace.Caller.StartLine != line-1 || trace.Caller.EndLine != line+1 || trace.Caller.Quote != tc.function ||
				trace.Implementation.Path != tc.source || trace.Implementation.Quote != strings.TrimSuffix(tc.implementation, "\n") {
				t.Fatalf("incorrect import trace: %+v", trace)
			}
			if trace, ok := tree.ReviewImport(t.Context(), tc.caller, entry, line, decoy); ok {
				t.Fatalf("same-named declaration in another file was linked: %+v", trace)
			}
		})
	}
}

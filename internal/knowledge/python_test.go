// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"strings"
	"testing"
)

func TestPythonKeepsUnchangedPremisesAtTheirNewLines(t *testing.T) {
	ctx := t.Context()
	r := newLifecycleRepo(t)
	const p = "prices.py"
	const original = "TAX = 2\n\nclass Order:\n    def fee(self, mode):\n        if mode == 'pickup':\n            return 0\n        return TAX\n\ndef other_rate():\n    return 4\n"
	r.write(p, original)
	base := r.commit("set pickup fee")
	c, err := base.Cite(ctx, p, 5, 6)
	if err != nil {
		t.Fatal(err)
	}
	f := prepareFact(t, base, Fact{Category: CategoryBusinessRule, Authority: AuthorityConfirmed, Source: p, Scope: p, Claim: "Pickup orders have no fee.", Citations: []Citation{c}})
	decision, err := base.DecideWithCitation(ctx, p, p, "order-41", "Charge for pickup", "The pickup contract waives this fee.", 5, 6)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, code string
		current    bool
		line       int
	}{
		{"blank lines", "\n\n" + original, true, 7},
		{"layout within the function", strings.Replace(original, "        if mode", "\n        if mode", 1), true, 6},
		{"unreferenced sibling", strings.Replace(original, "return 4", "return 7", 1), true, 5},
		{"guard", strings.Replace(original, "'pickup'", "'delivery'", 1), false, 0},
		{"module setup", strings.Replace(original, "TAX = 2", "TAX = 5", 1), false, 0},
		{"new decorator", strings.Replace(original, "    def fee", "    @cached\n    def fee", 1), false, 0},
		{"malformed source", "class Order(\n" + original, false, 0},
	} {
		r.write(p, tc.code)
		head := r.commit(tc.name)
		got := checkedFacts(t, head, []Fact{f, decision})
		if tc.current {
			if got[0].Status != StatusCurrent || got[1].Status != StatusHistorical || got[0].Citations[0].CurrentStartLine != tc.line {
				t.Errorf("%s: claim %s (%s), decision %s (%s), relocated to %d", tc.name, got[0].Status, got[0].Detail, got[1].Status, got[1].Detail, got[0].Citations[0].CurrentStartLine)
			}
		} else if got[0].Status != StatusChanged || got[1].Status != StatusChanged {
			t.Errorf("%s: expected changed premises, got %s and %s", tc.name, got[0].Status, got[1].Status)
		}
	}
}

func TestPythonKeepsReferencedSiblingsAndStringContents(t *testing.T) {
	const original = "def fee():\n    return limit()\n\ndef limit():\n    return 4\n\nNOTICE = '''cost\n\nunits'''\n"
	before := pythonWitnessesFor(t.Context(), []byte(original))
	if before == nil || before.functions["fee"].start != 1 {
		t.Fatal("missing Python function")
	}
	for _, replacement := range []string{
		strings.Replace(original, "return 4", "return 9", 1),
		strings.Replace(original, "cost\n\nunits", "cost\nunits", 1),
	} {
		after := pythonWitnessesFor(t.Context(), []byte(replacement))
		if after == nil || before.shared == after.shared {
			t.Fatal("changed code or literal ignored")
		}
	}
}

func TestPythonImplicitTruthinessIsPartOfTheBusinessGuard(t *testing.T) {
	const cart = "class Cart:\n    def fee(self, mode):\n        if mode == 'pickup' and self:\n            return 0\n        return 7\n\n    def __len__(self):\n        return len(self.items)\n"
	first := pythonWitnessesFor(t.Context(), []byte(cart))
	second := pythonWitnessesFor(t.Context(), []byte(strings.Replace(cart, "len(self.items)", "sum(item.active for item in self.items)", 1)))
	if first == nil || second == nil || first.shared == second.shared {
		t.Fatal("changed implicit cart length must invalidate the pickup guard")
	}
}

func TestPythonMultilineSuiteKeepsLaterCitations(t *testing.T) {
	for name, setup := range map[string]string{
		"docstring":   "        \"\"\"A literal can contain apparent top-level statements.\ndef other():\n\"\"\"\n",
		"parentheses": "        choices = (\n2,\n3,\n)\n",
	} {
		t.Run(name, func(t *testing.T) {
			r := newLifecycleRepo(t)
			const p = "pricing.py"
			original := "class Cart:\n    def fee(self, pickup):\n" + setup + "        if pickup:\n            return 0\n        return 7\n\n    def weight(self):\n        return 4\n"
			line := strings.Count(original[:strings.Index(original, "            return 0")], "\n") + 1
			r.write(p, original)
			base := r.commit("record the pickup fee")
			citation, err := base.Cite(t.Context(), p, line, line)
			if err != nil {
				t.Fatal(err)
			}
			fact := prepareFact(t, base, Fact{Category: CategoryBusinessRule, Authority: AuthorityConfirmed, Source: p, Scope: p, Claim: "Pickup is free.", Citations: []Citation{citation}})
			moved := "\n" + strings.Replace(original, "return 4", "return 8", 1)
			r.write(p, moved)
			current := checkedFacts(t, r.commit("change weight"), []Fact{fact})[0]
			if current.Status != StatusCurrent || current.Citations[0].CurrentStartLine != line+1 {
				t.Fatalf("unchanged fee: %s (%s), line %d", current.Status, current.Detail, current.Citations[0].CurrentStartLine)
			}
			r.write(p, strings.Replace(moved, "if pickup:", "if not pickup:", 1))
			if changed := checkedFacts(t, r.commit("invert pickup guard"), []Fact{fact})[0]; changed.Status != StatusChanged {
				t.Fatalf("changed guard: %s (%s)", changed.Status, changed.Detail)
			}
		})
	}
}

func TestPythonDeclarationEndsAtItsNextSuiteStatement(t *testing.T) {
	const source = `def first():
    return 1
@(
    authorize
)
async def second():
    return 2
LIMIT = 10
class Outer:
    class Inner:
        def last(self):
            return 3
    RATE = 4
`
	parsed := pythonWitnessesFor(t.Context(), []byte(source))
	if parsed == nil {
		t.Fatal("expected valid Python")
	}
	for name, end := range map[string]int{"first": 2, "second": 7, "Outer.Inner.last": 12} {
		if got := parsed.functions[name].end; got != end {
			t.Errorf("%s ends at %d, want %d", name, got, end)
		}
	}
}

func TestPythonDetachedCommentDoesNotBorrowPrecedingFunction(t *testing.T) {
	const path = "pricing.py"
	const source = `def banner():
    return """Unindented string content follows.
# This line is part of the banner.
# The string ends here."""

# Pickup orders are free.
def fee(pickup):
    if pickup:
        return 0
    return 9
`
	r := newLifecycleRepo(t)
	r.write(path, source)
	base := r.commit("record the banner and pickup rule")
	ctx := t.Context()
	comment, err := base.DecideWithCitation(ctx, path, path, "pickup", "Charge for pickup", "The comment records a pickup waiver.", 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	literal, err := base.DecideWithCitation(ctx, path, path, "banner", "Change the banner", "The line is literal display text.", 3, 4)
	if err != nil {
		t.Fatal(err)
	}
	r.write(path, strings.Replace(source, "if pickup:", "if not pickup:", 1))
	checked := checkedFacts(t, r.commit("invert the fee condition"), []Fact{comment, literal})
	if checked[0].Status != StatusChanged || checked[1].Status != StatusHistorical || checked[1].Citations[0].CurrentStartLine != 3 {
		t.Fatalf("detached comment: %s; literal: %s at %d", checked[0].Status, checked[1].Status, checked[1].Citations[0].CurrentStartLine)
	}
}

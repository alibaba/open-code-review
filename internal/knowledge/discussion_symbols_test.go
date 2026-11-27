// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/config/rules"
)

const discussionTestStamp = "2025-02-03T15:00:00Z"

func namedDiscussion(p, name, revision string) Discussion {
	return Discussion{ID: "change-14/comment-2", Path: p, Revision: revision, UpdatedAt: discussionTestStamp,
		Body: "The deployment uses `" + name + "` for this queue."}
}

func oneDiscussionRecord(t *testing.T, tree Tree, n Discussion, opts DiscoverOptions) Fact {
	t.Helper()
	opts.Discussions = []Discussion{n}
	result := discoverWithoutRules(t, tree, opts)
	if len(result.DiscussionSources) != 1 {
		t.Fatalf("expected one original discussion, got %+v, limits %v", result.DiscussionSources, result.Limits)
	}
	return result.DiscussionSources[0]
}

func namedWitnessesIn(s Snapshot, p string) []Citation {
	var witnesses []Citation
	for _, f := range s.SelectForChange([]string{p}, nil) {
		for _, w := range f.DiscussionWitnesses() {
			if w.Symbol != "" && !slices.Contains(witnesses, w) {
				witnesses = append(witnesses, w)
			}
		}
	}
	return witnesses
}

func TestDiscussionSymbolWitnessUsesCurrentDefinition(t *testing.T) {
	for _, tc := range []struct {
		name, extension, mention, entry, implementation string
		line                                            int
	}{
		{
			name: "Go", extension: ".go", mention: "policy.BatchSize",
			entry:          "package delivery\n\nfunc dispatch(n int) int { return n }\n",
			implementation: "package policy\n\nfunc BatchSize(n int) int {\n\treturn n + 17\n}\n\nfunc BackupBatchSize(n int) int { return 900 }\n",
			line:           3,
		},
		{
			name: "Python", extension: ".py", mention: "DispatchPolicy.batch_size",
			entry: "def dispatch(n):\n    return n\n",
			implementation: "class DispatchPolicy:\n    @staticmethod\n    def batch_size(n):\n        return n + 17\n\n" +
				"    @staticmethod\n    def backup_batch_size(n):\n        return 900\n",
			line: 2,
		},
		{
			name: "JavaScript", extension: ".js", mention: "batchSize",
			entry: "export function dispatch(n) { return n; }\n",
			implementation: "const example = `\nfunction batchSize(n) { return 800; }\n`;\n\n" +
				"export const batchSize = (n) => {\n  return n + 17;\n};\n\nexport function backupBatchSize(n) { return 900; }\n",
			line: 5,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newDiscoveryRepo(t)
			caller, definition := "delivery/dispatch"+tc.extension, "shared/policy"+tc.extension
			r.write(caller, tc.entry)
			r.write(definition, tc.implementation)
			base := r.commit("publish dispatch policy")
			note := namedDiscussion(caller, tc.mention, base.Commit)
			raw := oneDiscussionRecord(t, base, note, DiscoverOptions{})
			if raw.DiscussionBindings == nil || len(raw.DiscussionBindings.Symbols) != 1 || raw.DiscussionBindings.Symbols[0].Path != definition {
				t.Fatalf("lost the implementation in the admitted cross-directory corpus: %+v", raw.DiscussionBindings)
			}
			r.write(definition, strings.ReplaceAll(tc.implementation, "+ 17", "+ 23"))
			head := r.commit("revise the selected helper")
			review := ReviewSnapshot(checkedFacts(t, head, []Fact{raw}))
			got := namedWitnessesIn(review, caller)
			if len(got) != 1 || got[0].Path != definition || got[0].StartLine != tc.line || got[0].Revision != head.Commit ||
				got[0].Reference != note.ID || got[0].Symbol != tc.mention || !strings.Contains(got[0].Quote, "+ 23") ||
				strings.Contains(got[0].Quote, "900") || strings.Contains(got[0].Quote, "800") {
				t.Fatalf("expected the exact new selected definition, got %+v", got)
			}
			if review.ForPaths([]string{definition}) != "" {
				t.Fatal("a helper witness changed where the original discussion applies")
			}
		})
	}
}

func TestDiscussionSymbolWitnessRechecksKnownCompetitorsAndDeletion(t *testing.T) {
	r := newDiscoveryRepo(t)
	const caller, implementation, competitor = "delivery/dispatch.js", "shared/primary.js", "shared/secondary.js"
	r.write(caller, "export function dispatch(n) { return n; }\n")
	r.write(implementation, "export function selectedBatch(n) { return n + 1; }\n")
	r.write(competitor, "export function backupBatch(n) { return n + 2; }\n")
	base := r.commit("define two available dispatch policies")
	note := namedDiscussion(caller, "selectedBatch", base.Commit)
	raw := oneDiscussionRecord(t, base, note, DiscoverOptions{})
	witnessesAt := func(tree Tree) []Citation {
		t.Helper()
		return namedWitnessesIn(ReviewSnapshot(checkedFacts(t, tree, []Fact{raw})), caller)
	}
	if got := witnessesAt(base); len(got) != 1 || got[0].Path != implementation {
		t.Fatalf("missing selected implementation: %+v", got)
	}
	r.write(competitor, "export function selectedBatch(n) { return n + 2; }\n")
	ambiguous := r.commit("add a competing admitted definition")
	if got := witnessesAt(ambiguous); len(got) != 0 {
		t.Fatalf("ambiguous symbol yielded a witness: %+v", got)
	}
	r.write(competitor, "export function backupBatch(n) { return n + 2; }\n")
	r.write(implementation, "/*\nfunction selectedBatch(n) { return n + 1; }\n*/\nexport function unrelated(n) { return n - 8; }\n")
	rewritten := r.commit("rewrite the selected module")
	if got := witnessesAt(rewritten); len(got) != 0 {
		t.Fatalf("a removed declaration survived as a comment: %+v", got)
	}
	if err := os.Remove(filepath.Join(r.dir, filepath.FromSlash(implementation))); err != nil {
		t.Fatal(err)
	}
	r.write("new/surrogate.js", "export function selectedBatch(n) { return n - 8; }\n")
	deleted := r.commit("remove original and add an unrelated translation")
	if got := witnessesAt(deleted); len(got) != 0 {
		t.Fatalf("a new file substituted for the removed definition: %+v", got)
	}
}

func TestDiscussionSymbolWitnessRequiresAcceptedIdentityAndPathPermission(t *testing.T) {
	r := newDiscoveryRepo(t)
	const caller, definition, unrelated = "delivery/worker.py", "lib/policy.py", "lib/new_policy.py"
	r.write(caller, "def dispatch(n):\n    return n\n")
	r.write(definition, "MAX_QUEUE = 'regional'\n")
	base := r.commit("define the regional mode")
	note := namedDiscussion(caller, "regional", base.Commit)
	check := func(raw Fact, tree Tree) []Citation {
		return namedWitnessesIn(ReviewSnapshot(checkedFacts(t, tree, []Fact{raw})), caller)
	}
	for _, opts := range []DiscoverOptions{
		{Scope: caller},
		{FileFilter: &rules.FileFilter{Exclude: []string{"lib/**"}}},
	} {
		raw := oneDiscussionRecord(t, base, note, opts)
		if got := check(raw, base); raw.DiscussionBindings != nil || len(got) != 0 {
			t.Fatalf("sync scope or exclude admitted outside definition: %+v", got)
		}
	}
	raw := oneDiscussionRecord(t, base, note, DiscoverOptions{})
	if got := check(raw, base); len(got) != 1 || got[0].Path != definition {
		t.Fatalf("expected the exact currently defined mode: %+v", got)
	}
	excluded := base
	excluded.AllowPath = func(p string) bool { return p != definition }
	if got := check(raw, excluded); len(got) != 0 {
		t.Fatalf("a review exclusion admitted a helper: %+v", got)
	}
	r.write(definition, "MAX_QUEUE = 'regional'\napi_key = 'placeholderCredential'\n")
	credential := r.commit("add credential-shaped text")
	if got := check(raw, credential); len(got) != 0 {
		t.Fatalf("sensitive file entered review: %+v", got)
	}
	r.write(definition, "MAX_QUEUE = 'central'\n")
	r.write(unrelated, "MAX_QUEUE = 'regional'\n")
	head := r.commit("introduce another regional configuration")
	resync := oneDiscussionRecord(t, head, note, DiscoverOptions{})
	if got := check(resync, head); len(got) != 0 {
		t.Fatalf("an old revision's wording acquired a new definition: %+v", got)
	}
}

func TestDiscussionSymbolWitnessUpgradesStoredEditionAndLinksExactDerivedSource(t *testing.T) {
	r := newDiscoveryRepo(t)
	const caller, impl = "orders/dispatch.go", "rules/selection.go"
	r.write(caller, "package orders\n\nfunc choose(v int) int { return v }\n")
	r.write(impl, "package rules\n\nfunc routeSelected(v int) int { return v + 1 }\n")
	tree := r.commit("define selection")
	note := namedDiscussion(caller, "routeSelected", tree.Commit)
	latest := oneDiscussionRecord(t, tree, note, DiscoverOptions{})
	older := latest
	older.DiscussionBindings = nil
	db := testStore(t)
	editions := []Fact{older, latest}
	if err := db.Save(t.Context(), editions); err != nil {
		t.Fatal(err)
	}
	if editions[0].ID != editions[1].ID {
		t.Fatal("a lookup capability changed the stored discussion identity")
	}
	derived := prepareFact(t, tree, Fact{Category: CategoryBusinessRule, Authority: AuthorityInferred, Source: caller, Scope: caller,
		Claim: "Orders use the selected route.", Citations: []Citation{{Kind: "code", Path: caller, StartLine: 3, EndLine: 3}, latest.Citations[0]}})
	rows := storedFacts(t, db)
	snapshot := ReviewSnapshot(checkedFacts(t, tree, append(rows, derived)))
	found := map[string]bool{}
	for _, f := range snapshot.SelectForChange([]string{caller}, nil) {
		witness := f.DiscussionWitnesses()
		if slices.ContainsFunc(witness, func(c Citation) bool { return c.Path == impl && c.Reference == note.ID && c.Symbol != "" }) {
			found[f.Category] = true
		}
	}
	if !found[CategoryBusinessRule] || !found[CategoryDiscussionSource] {
		t.Fatalf("same latest edition did not expose its raw and derived symbol witness: %v", found)
	}
	if text := snapshot.ForPaths([]string{caller}); !strings.Contains(text, impl) || strings.Contains(text, "discussion_bindings") {
		t.Fatalf("incorrect witness projection: %s", text)
	}
	earlier := tree
	earlier.Now = time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC)
	before := ReviewSnapshot(checkedFacts(t, earlier, rows))
	if len(namedWitnessesIn(before, caller)) != 0 {
		t.Fatal("a later-edited discussion entered an earlier replay")
	}
}

func TestNamedDiscussionCoveragePreservesCurrentLocationAndSource(t *testing.T) {
	wanted := Citation{Kind: "code", Path: "rules/selection.go", CurrentPath: "rules/current.go",
		StartLine: 5, EndLine: 6, CurrentStartLine: 8, CurrentEndLine: 9, Symbol: "choose", Reference: "note-2", Quote: "func choose() int {\n\treturn 2"}
	for _, tc := range []struct {
		name   string
		change func(*Citation)
		covers bool
	}{
		{"same current witness", func(c *Citation) {
			c.Path, c.StartLine, c.EndLine = "rules/current.go", 8, 9
			c.CurrentPath, c.CurrentStartLine, c.CurrentEndLine = "", 0, 0
		}, true},
		{"generic excerpt", func(c *Citation) { c.Symbol, c.Reference = "", "" }, false},
		{"other symbol", func(c *Citation) { c.Symbol = "alternate" }, false},
		{"other discussion", func(c *Citation) { c.Reference = "note-4" }, false},
		{"same excerpt at another line", func(c *Citation) { c.CurrentStartLine, c.CurrentEndLine = 10, 11 }, false},
		{"same excerpt at another path", func(c *Citation) { c.CurrentPath = "rules/prior.go" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shown := wanted
			tc.change(&shown)
			if got := coversDiscussionWitness(shown, wanted); got != tc.covers {
				t.Fatalf("coversDiscussionWitness(%+v) = %t, want %t", shown, got, tc.covers)
			}
		})
	}
}

func TestDiscussionExcerptCoverageUsesItsCurrentLine(t *testing.T) {
	wanted := Citation{Path: "orders.py", StartLine: 9, EndLine: 9, Quote: "    return result"}
	for _, tc := range []struct {
		name  string
		shown Citation
		want  bool
	}{
		{"enclosing definition", Citation{Path: "old.py", CurrentPath: wanted.Path, CurrentStartLine: 8, CurrentEndLine: 10,
			Quote: "def place(result):\n    return result\n"}, true},
		{"another definition with the same return", Citation{Path: wanted.Path, StartLine: 25, EndLine: 25, Quote: wanted.Quote}, false},
		{"other return in enclosing excerpt", Citation{Path: wanted.Path, StartLine: 8, EndLine: 10, Quote: "    return result\nelse:\n    return result"}, false},
		{"truncated excerpt", Citation{Path: wanted.Path, StartLine: 8, EndLine: 10, Quote: "def place(result):"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := coversDiscussionWitness(tc.shown, wanted); got != tc.want {
				t.Fatalf("coverage of %s:%d = %v, want %v", wanted.Path, wanted.StartLine, got, tc.want)
			}
		})
	}
}

func TestDiscussionUnquotableSameLineDeclarationsCountAsCompetitors(t *testing.T) {
	for _, tc := range []struct {
		name, extension, entry, declaration, competing string
	}{
		{"Python", ".py", "def dispatch():\n    return 1\n", "batch_size = 2\n", "label = 0; batch_size = 3\n"},
		{"Python unpacking", ".py", "def dispatch():\n    return 1\n", "batch_size = 2\n", "label, batch_size = 0, 3\n"},
		{"Go grouped binding", ".go", "package api\nfunc dispatch() int { return 1 }\n", "package policy\nvar batch_size = 2\n",
			"package alternate\nvar label, batch_size = 0, 3\n"},
		{"JavaScript statements", ".js", "function dispatch() { return 1; }\n", "export const batch_size = 2;\n",
			"const label = 0; export const batch_size = 3;\n"},
		{"JavaScript declarators", ".js", "function dispatch() { return 1; }\n", "export const batch_size = 2;\n",
			"export const label = 0, batch_size = 3;\n"},
		{"JavaScript destructuring", ".js", "function dispatch() { return 1; }\n", "export const batch_size = 2;\n",
			"export const [label, batch_size] = [0, 3];\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newDiscoveryRepo(t)
			entry, target, peer := "api/entry"+tc.extension, "rules/target"+tc.extension, "rules/alternate"+tc.extension
			r.write(entry, tc.entry)
			r.write(target, tc.declaration)
			r.write(peer, tc.competing)
			tree := r.commit("implement the same setting in separate modules")
			note := namedDiscussion(entry, "batch_size", tree.Commit)
			raw := oneDiscussionRecord(t, tree, note, DiscoverOptions{})
			review := ReviewSnapshot(checkedFacts(t, tree, []Fact{raw}))
			if got := namedWitnessesIn(review, entry); len(got) != 0 {
				t.Fatalf("unquotable competing declaration did not prevent a witness: %+v", got)
			}
		})
	}
}

func TestDiscussionRejectsSameNameBeyondDiscoverySample(t *testing.T) {
	r := newDiscoveryRepo(t)
	const caller, chosen, alternate = "api/dispatch.py", "policy/selected.py", "secondary/alternative.py"
	r.write(caller, "def dispatch(n):\n    return n\n")
	r.write(chosen, "def batch_size(n):\n    return n + 1\n")
	r.write(alternate, "def batch_size(n):\n    return n + 30\n")
	priority := []string{caller, chosen}
	for i := range discoveryFiles - len(priority) {
		p := fmt.Sprintf("api/utility%02d.py", i)
		r.write(p, fmt.Sprintf("def utility%02d(n):\n    return n\n", i))
		priority = append(priority, p)
	}
	tree := r.commit("add a same-named definition outside the preferred corpus")
	note := namedDiscussion(caller, "batch_size", tree.Commit)
	opts := DiscoverOptions{Paths: priority, Discussions: []Discussion{note}}
	result, err := tree.Discover(t.Context(), opts, func(_ context.Context, _ string, corpus string) (string, error) {
		if !strings.Contains(corpus, chosen) || strings.Contains(corpus, alternate) {
			t.Fatalf("fixture did not omit the alternative from discovery: %s", corpus)
		}
		return `{"records":[]}`, nil
	})
	if err != nil || len(result.DiscussionSources) != 1 || result.DiscussionSources[0].DiscussionBindings != nil {
		t.Fatalf("sample-only match was considered unique: %+v, %v", result.DiscussionSources, err)
	}
	opts.FileFilter = &rules.FileFilter{Exclude: []string{alternate}}
	raw := oneDiscussionRecord(t, tree, note, opts)
	if got := namedWitnessesIn(ReviewSnapshot(checkedFacts(t, tree, []Fact{raw})), caller); len(got) != 1 || got[0].Path != chosen {
		t.Fatalf("excluded alternative changed the authorized binding: %+v", got)
	}
}

func TestDiscussionRejectsNewSameNameInCommitAndWorkspace(t *testing.T) {
	r := newDiscoveryRepo(t)
	const caller, chosen, alternate = "api/dispatch.go", "policy/selected.go", "secondary/new.go"
	r.write(caller, "package api\n\nfunc dispatch() int { return 1 }\n")
	r.write(chosen, "package policy\n\nfunc BatchSize() int { return 2 }\n")
	base := r.commit("define a single batch size function")
	raw := oneDiscussionRecord(t, base, namedDiscussion(caller, "BatchSize", base.Commit), DiscoverOptions{})
	witness := func(tree Tree) []Citation {
		t.Helper()
		return namedWitnessesIn(ReviewSnapshot(checkedFacts(t, tree, []Fact{raw})), caller)
	}
	if got := witness(base); len(got) != 1 || got[0].Path != chosen {
		t.Fatalf("missing independently unique binding: %+v", got)
	}
	r.write(alternate, "package secondary\n\nfunc OtherSize() int { return 3 }\n")
	unrelated := r.commit("add an unrelated setting")
	if got := witness(unrelated); len(got) != 1 || got[0].Path != chosen {
		t.Fatalf("unrelated source blocked a known definition: %+v", got)
	}
	r.write(alternate, "package secondary\n\nfunc BatchSize() int { return 3 }\n")
	workspace := unrelated
	workspace.Workspace = true
	if got := witness(workspace); len(got) != 0 {
		t.Fatalf("a new worktree definition went unchecked: %+v", got)
	}
	head := r.commit("add another function named BatchSize")
	if got := witness(head); len(got) != 0 {
		t.Fatalf("new same-named definition from outside the old corpus was ignored: %+v", got)
	}
	head.AllowPath = func(p string) bool { return p != alternate }
	if got := witness(head); len(got) != 1 || got[0].Path != chosen {
		t.Fatalf("an excluded new file changed the permitted proof: %+v", got)
	}
}

func TestDiscussionNameLookupWithholdsMoreMatchesThanItsBudget(t *testing.T) {
	r := newDiscoveryRepo(t)
	const caller, chosen = "api/dispatch.js", "policy/selected.js"
	r.write(caller, "export function dispatch(n) { return n; }\n")
	r.write(chosen, "export function queuedSize(n) { return n + 1; }\n")
	for i := range maxDiscussionMatchFiles {
		r.write(fmt.Sprintf("support/ref%02d.js", i), "export function plausibleCaller(n) { return queuedSize(n); }\n")
	}
	base := r.commit("build a corpus with many references to the single declared name")
	raw := oneDiscussionRecord(t, base, namedDiscussion(caller, "queuedSize", base.Commit), DiscoverOptions{Paths: []string{caller, chosen}})
	if raw.DiscussionBindings != nil {
		t.Fatalf("a partial match scan was accepted as exhaustive: %+v", raw.DiscussionBindings)
	}
}

func TestDiscussionStaticImportDisambiguatesSameNameAndSurvivesUnrelatedAdditions(t *testing.T) {
	for _, tc := range []struct {
		name, caller, chosen, alternate, mention, entry, implement, rebind string
	}{
		{"Go", "api/dispatch.go", "policy/selected.go", "unrelated/other.go", "chosen.BatchSize",
			"package api\n\nimport chosen \"example.test/dispatch/policy\"\n\nfunc dispatch(n int) int { return chosen.BatchSize(n) }\n",
			"package policy\n\nfunc BatchSize(n int) int { return n + 1 }\n",
			"package api\n\nimport chosen \"other.test/dispatch/policy\"\n\nfunc dispatch(n int) int { return chosen.BatchSize(n) }\n"},
		{"Python", "api/dispatch.py", "api/policy.py", "unrelated/other.py", "chosen_size",
			"from .policy import batch_size as chosen_size\n\ndef dispatch(n):\n    return chosen_size(n)\n",
			"def batch_size(n):\n    return n + 1\n",
			"from .other import batch_size as chosen_size\n\ndef dispatch(n):\n    return chosen_size(n)\n"},
		{"JavaScript", "api/dispatch.js", "policy/selected.js", "unrelated/other.js", "selected.batchSize",
			"import * as selected from '../policy/selected.js';\n\nexport function dispatch(n) { return selected.batchSize(n); }\n",
			"export function batchSize(n) { return n + 1; }\n",
			"import * as selected from '../unrelated/other.js';\n\nexport function dispatch(n) { return selected.batchSize(n); }\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newDiscoveryRepo(t)
			if tc.name == "Go" {
				r.write("go.mod", "module example.test/dispatch\n")
			}
			r.write(tc.caller, tc.entry)
			r.write(tc.chosen, tc.implement)
			r.write(tc.alternate, strings.Replace(tc.implement, "+ 1", "+ 2", 1))
			base := r.commit("import one of two existing definitions")
			note := namedDiscussion(tc.caller, tc.mention, base.Commit)
			raw := oneDiscussionRecord(t, base, note, DiscoverOptions{})
			witness := func(tree Tree) []Citation {
				t.Helper()
				return namedWitnessesIn(ReviewSnapshot(checkedFacts(t, tree, []Fact{raw})), tc.caller)
			}
			if got := witness(base); len(got) != 1 || got[0].Path != tc.chosen || got[0].Symbol != tc.mention {
				t.Fatalf("static import did not identify its exact implementation: %+v", got)
			}
			r.write("newcopy/"+filepath.Base(tc.alternate), strings.Replace(tc.implement, "+ 1", "+ 3", 1))
			head := r.commit("add a further definition outside the import")
			if got := witness(head); len(got) != 1 || got[0].Path != tc.chosen {
				t.Fatalf("unrelated addition blurred a concrete import: %+v", got)
			}
			r.write(tc.caller, tc.rebind)
			changed := r.commit("change the imported module")
			if got := witness(changed); len(got) != 0 {
				t.Fatalf("stale discussion silently rebound to a new import: %+v", got)
			}
		})
	}
}

func TestGoDiscussionImportVerifiesUnsampledPackageSiblings(t *testing.T) {
	r := newDiscoveryRepo(t)
	const caller, definition, sibling = "api/dispatch.go", "policy/batch.go", "policy/support.go"
	const entry = "package api\nimport chosen \"example.test/dispatch/policy\"\nfunc dispatch(n int) int { return chosen.BatchSize(n) }\n"
	r.write("go.mod", "module example.test/dispatch\n")
	r.write(caller, entry)
	r.write(definition, "package policy\nfunc BatchSize(n int) int { return n + 1 }\n")
	r.write(sibling, "package policy\ntype Plan struct{}\nfunc (Plan) BatchSize() int { return 2 }\n")
	priority := []string{caller, definition}
	for i := range discoveryFiles - len(priority) {
		p := fmt.Sprintf("api/utility%02d.go", i)
		r.write(p, "package api\n")
		priority = append(priority, p)
	}
	base := r.commit("sample one definition in a multi-file imported package")
	note := namedDiscussion(caller, "chosen.BatchSize", base.Commit)
	discover := func(paths []string, admitted, omitted string, filter *rules.FileFilter) Fact {
		t.Helper()
		opts := DiscoverOptions{Paths: paths, Discussions: []Discussion{note}, FileFilter: filter}
		result, err := base.Discover(t.Context(), opts, func(_ context.Context, _ string, corpus string) (string, error) {
			if !strings.Contains(corpus, admitted) || strings.Contains(corpus, omitted) {
				t.Fatalf("fixture did not sample %s and omit %s: %s", admitted, omitted, corpus)
			}
			return `{"records":[]}`, nil
		})
		if err != nil || len(result.DiscussionSources) != 1 {
			t.Fatalf("missing discussion: %+v, %v", result.DiscussionSources, err)
		}
		return result.DiscussionSources[0]
	}
	raw := discover(priority, definition, sibling, nil)
	witness := func(tree Tree) []Citation {
		t.Helper()
		return namedWitnessesIn(ReviewSnapshot(checkedFacts(t, tree, []Fact{raw})), caller)
	}
	if got := witness(base); len(got) != 1 || got[0].Path != definition || got[0].Symbol != "chosen.BatchSize" {
		t.Fatalf("unsampled sibling prevented the package binding: %+v", got)
	}
	withoutDefinition := slices.Clone(priority)
	withoutDefinition[1] = sibling
	if got := discover(withoutDefinition, sibling, definition, nil); got.DiscussionBindings != nil {
		t.Fatalf("unsampled definition was admitted: %+v", got.DiscussionBindings)
	}
	if got := discover(priority, definition, sibling, &rules.FileFilter{Exclude: []string{sibling}}); got.DiscussionBindings != nil {
		t.Fatalf("uniqueness inferred through an excluded sibling: %+v", got.DiscussionBindings)
	}
	workspace := base
	workspace.Workspace = true
	if got := witness(workspace); len(got) != 1 || got[0].Path != definition {
		t.Fatalf("working package omitted its actual definition: %+v", got)
	}
	for _, tc := range []struct{ name, source string }{
		{"ambiguous", "package policy\nfunc BatchSize(n int) int { return 8 }\n"},
		{"unavailable body", "package policy\nfunc BatchSize(n int) int\n"},
		{"unparseable", "package policy\nfunc incomplete(\n"},
		{"credential-shaped text", "package policy\nvar api_key = \"placeholderCredential\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r.write(sibling, tc.source)
			if got := witness(workspace); len(got) != 0 {
				t.Fatalf("binding retained despite %s sibling: %+v", tc.name, got)
			}
		})
	}
	r.write(sibling, "package policy\nfunc BatchSize(n int) int { return 8 }\n")
	head := r.commit("add a competing package-level declaration")
	if got := witness(head); len(got) != 0 {
		t.Fatalf("commit binding retained despite two package members: %+v", got)
	}
}

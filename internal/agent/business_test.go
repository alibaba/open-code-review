// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
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

func TestReviewRetrievesLandedDiscussionForChangedField(t *testing.T) {
	const policy = "billing/policy.py"
	const processor = "billing/processor.py"
	const reference = "review:fee-calculation"
	snapshot := competingLandedDiscussions(policy, reference, "    return self.net_fee")
	file := func(path, patch, actualClass string) model.Diff {
		change := model.Diff{NewPath: path, Diff: patch}
		if path == policy {
			_, changed, _ := strings.Cut(patch, "\n+")
			change.NewFileContent = "from .processor import Processor\nclass LocalPolicy(Processor):\n" + changed + "\n"
		} else if patch != "" {
			lines := []string{"class " + actualClass + ":", "    def post(self, total):"}
			for _, h := range diff.ParseHunks(patch) {
				for len(lines) < h.NewStart-1 {
					lines = append(lines, "")
				}
				for _, l := range h.Lines {
					if l.Type != diff.HunkDeleted {
						lines = append(lines, l.Content)
					}
				}
			}
			change.NewFileContent = strings.Join(lines, "\n")
		}
		return change
	}
	const local = "@@ -3 +3 @@ class LocalPolicy(Processor):\n-    floor_fee = 0\n+    floor_fee = 1"
	const shared = "@@ -8,2 +8,4 @@ class Processor:\n+        if self.floor_fee > total:\n+            return None\n         self.net_fee = total\n         return total"
	for _, tc := range []struct {
		name, local, other string
		want               bool
	}{
		{"own file owns exact member", strings.ReplaceAll(local, "floor_fee", "net_fee"), "", true},
		{"unrelated file names landed member", local, "@@ -2 +2 @@\n-    return amount\n+    return self.net_fee", false},
		{"different receiver member with same suffix", local, strings.ReplaceAll(shared, "self.net_fee", "self.total_fee"), false},
		{"subclass field is read beside the landed member", local, shared, true},
		{"other class happens to use the same two members", local, strings.ReplaceAll(shared, "class Processor:", "class Unrelated:"), false},
		{"different receiver inside linked hunk", local, strings.ReplaceAll(shared, "self.net_fee", "peer.net_fee"), false},
		{"read and landed member occur in separate hunks", local, strings.ReplaceAll(shared, "         self.net_fee", "@@ -90,2 +92,2 @@ class Processor:\n         self.net_fee"), false},
		{"comment mentions the member in linked hunk", local, strings.ReplaceAll(shared, "self.net_fee = total", "# self.net_fee = total"), false},
		{"read only occurs in unchanged context", local, strings.ReplaceAll(shared, "+        if self.floor_fee", "         if self.floor_fee"), false},
		{"write does not read the overriding value", local, strings.ReplaceAll(shared, "if self.floor_fee > total:", "self.floor_fee = total"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := "Processor"
			if tc.name == "other class happens to use the same two members" {
				owner = "Unrelated"
			}
			other := file(processor, tc.other, owner)
			ctx := selectBusinessContext(snapshot, []model.Diff{file(policy, tc.local, ""), other})
			if got := strings.Contains(ctx.text, reference); got != tc.want || len(ctx.rules) == 0 || len(ctx.rules) == len(snapshot) {
				t.Fatalf("selected %d sources, landed discussion present=%t, want %t", len(ctx.rules), got, tc.want)
			}
			if got := selectBusinessContext(snapshot, []model.Diff{other}); strings.Contains(got.text, reference) {
				t.Fatal("a matching field admitted a discussion outside its file scope")
			}
		})
	}
}

func withHunkLabel(patch, label string) string {
	lines := strings.Split(patch, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "@@ ") {
			header, _, _ := strings.Cut(line, " @@")
			lines[i] = strings.TrimSpace(header + " @@ " + label)
		}
	}
	return strings.Join(lines, "\n")
}

func TestLandedRetrievalUsesParsedChangeLocations(t *testing.T) {
	const policy, processor = "policy.py", "processor.py"
	const submitted = "review:accepted-member"
	const local = "from .processor import Processor\nclass Local(Processor):\n    floor_fee: int = (\n        1\n    )\n    display_width = 1\n"
	const engine = "class Processor:\n    def preview(self):\n        return 7\n    def charge(self, total):\n        total += 1\n        self.net_fee = total\n        return total\n\nclass Other:\n    def charge(self, total):\n        total -= 1\n        self.net_fee = total\n        return total\n"
	tree := initPreviewRepo(t)
	commitIn(t, tree, policy, local, "introduce a multiline override")
	commitIn(t, tree, processor, engine, "define both processors")
	git := gitcmd.New(1)
	output := func(args ...string) string {
		t.Helper()
		out, err := git.Output(t.Context(), tree, args...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	base := output("rev-parse", "HEAD")
	gitDiff := func(file, src string) model.Diff {
		return model.Diff{NewPath: file, NewFileContent: src, Diff: output("diff", "--unified=3", base, "HEAD", "--", file)}
	}
	snapshot := competingLandedDiscussions(policy, submitted, "    return self.net_fee")
	for _, tc := range []struct {
		name, policyChange, engineChange string
		want                             bool
	}{
		{"multiline override read on the imported base", strings.Replace(local, "        1", "        2", 1), strings.Replace(engine, "total += 1", "if self.floor_fee > total: return 0", 1), true},
		{"ordinary override with equivalent hunk labels", strings.Replace(local, "(\n        1\n    )", "2", 1), strings.Replace(engine, "total += 1", "if self.floor_fee > total: return 3", 1), true},
		{"field read in another class", strings.Replace(local, "(\n        1\n    )", "3", 1), strings.Replace(engine, "total -= 1", "if self.floor_fee > total: return 0", 1), false},
		{"other attribute was changed", strings.Replace(local, "display_width = 1", "display_width = 2", 1), strings.Replace(engine, "total += 1", "if self.floor_fee > total: return 0", 1), false},
		{"another method owns the neighbor in the same hunk", strings.Replace(local, "(\n        1\n    )", "4", 1), strings.Replace(engine, "return 7", "return self.floor_fee", 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			commitIn(t, tree, policy, tc.policyChange, tc.name+" policy")
			commitIn(t, tree, processor, tc.engineChange, tc.name+" processor")
			sub, baseChange := gitDiff(policy, tc.policyChange), gitDiff(processor, tc.engineChange)
			if tc.name == "another method owns the neighbor in the same hunk" && (!strings.Contains(baseChange.Diff, "self.net_fee") || len(diff.ParseHunks(baseChange.Diff)) != 1) {
				t.Fatal("control must place both members in the same hunk")
			}
			for _, title := range []string{"class Processor:", "def charge(self, total):", ""} {
				current := baseChange
				current.Diff = withHunkLabel(current.Diff, title)
				ctx := selectBusinessContext(snapshot, []model.Diff{sub, current})
				if got := strings.Contains(ctx.text, submitted); got != tc.want {
					t.Errorf("label %q selected=%t, want %t", title, got, tc.want)
				}
			}
		})
	}
}

func TestLandedRetrievalAttributesDeletedPythonValues(t *testing.T) {
	const policy, worker = "policy.py", "worker.py"
	const reference = "review:accepted-route"
	const source = "from .worker import Worker\n" +
		"class Local(Worker):\n" +
		"    routes: tuple = (\n" +
		"        'priority',\n" +
		"        # temporary review note\n" +
		"        'standard',\n" +
		"    )\n" +
		"    display_width = 18\n\n" +
		"    def previous(self):\n        if not self.enabled:\n            return None\n        return self.last_route\n\n" +
		"class Other:\n" +
		"    routes = (\n" +
		"        'other-priority',\n" +
		"        'other-standard',\n" +
		"    )\n"
	const accepted = "class Worker:\n    def route(self, code):\n        available = True\n        self.last_route = code\n        return available\n"
	headWorker := strings.Replace(accepted, "available = True", "available = code in self.routes", 1)
	for _, tc := range []struct {
		name, deleted string
		want          bool
	}{
		{"active list element", "        'priority',\n", true},
		{"only a comment", "        # temporary review note\n", false},
		{"an entire adjacent assignment", "    display_width = 18\n", false},
		{"same field on unrelated class", "        'other-priority',\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := initPreviewRepo(t)
			commitIn(t, dir, policy, source, "introduce two route policies")
			commitIn(t, dir, worker, accepted, "introduce the shared worker")
			git := gitcmd.New(1)
			output := func(args ...string) string {
				t.Helper()
				s, err := git.Output(t.Context(), dir, args...)
				if err != nil {
					t.Fatal(err)
				}
				return strings.TrimSpace(string(s))
			}
			base := output("rev-parse", "HEAD")
			policyHead := strings.Replace(source, tc.deleted, "", 1)
			commitIn(t, dir, policy, policyHead, "remove one line")
			commitIn(t, dir, worker, headWorker, "consult the configured routes")
			facts := competingLandedDiscussions(policy, reference, "        return self.last_route")
			baseChange := model.Diff{NewPath: worker, NewFileContent: headWorker, Diff: output("diff", base, "HEAD", "--", worker)}
			for _, context := range []string{"--unified=0", "--unified=3"} {
				patch := output("diff", context, base, "HEAD", "--", policy)
				hunks := diff.ParseHunks(patch)
				if strings.Contains(patch, "self.last_route") {
					t.Fatal("the scope file itself must not expose the landed member in its diff")
				}
				if len(hunks) != 1 || hunks[0].OldCount != hunks[0].NewCount+1 || slices.ContainsFunc(hunks[0].Lines, func(l diff.HunkLine) bool { return l.Type == diff.HunkAdded }) {
					t.Fatalf("expected a pure deletion: %s", patch)
				}
				changes := []model.Diff{{NewPath: policy, NewFileContent: policyHead, Diff: patch}, baseChange}
				if got := strings.Contains(selectBusinessContext(facts, changes).text, reference); got != tc.want {
					t.Errorf("%s ranked=%t, want %t", context, got, tc.want)
				}
			}
		})
	}
}

func TestReviewRetrievesTheEditedEnclosingDeclaration(t *testing.T) {
	const ref = "review:billing-limit"
	for _, tc := range []struct{ file, decl, other, name, statement string }{
		{"ledger.go", "package ledger\nfunc applyBillingLimit() int {", "func otherTotal() int {", "applyBillingLimit", "    value := 1"},
		{"ledger.py", "def apply_billing_limit():", "def other_total():", "apply_billing_limit", "    value = 1"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			end := ""
			if strings.HasSuffix(tc.file, ".go") {
				end = "}\n"
			}
			body := "\n" + strings.Repeat("\n", 7) + tc.statement + "\n    return value\n" + end
			before := tc.decl + body + "\n" + tc.other + body
			repo := initPreviewRepo(t)
			commitIn(t, repo, tc.file, before, "introduce the two accounting functions")
			git := gitcmd.New(1)
			output := func(args ...string) string {
				t.Helper()
				out, err := git.Output(t.Context(), repo, args...)
				if err != nil {
					t.Fatal(err)
				}
				return strings.TrimSpace(string(out))
			}
			base := output("rev-parse", "HEAD")
			snap := competingLandedDiscussions(tc.file, ref, tc.decl)
			for i, want := range []bool{true, false} {
				oldSide := strings.SplitN(before, tc.other, 2)
				oldSide[i] = strings.Replace(oldSide[i], tc.statement, strings.Replace(tc.statement, "1", "2", 1), 1)
				head := strings.Join(oldSide, tc.other)
				commitIn(t, repo, tc.file, head, "change one amount")
				patch := output("diff", "--unified=1", base, "HEAD", "--", tc.file)
				for _, title := range []string{tc.name, ""} {
					change := model.Diff{NewPath: tc.file, NewFileContent: head, Diff: withHunkLabel(patch, title)}
					if got := strings.Contains(selectBusinessContext(snap, []model.Diff{change}).text, ref); got != want {
						t.Errorf("named %q and edited declaration %d: got %t want %t", title, i, got, want)
					}
				}
			}
		})
	}
}

func competingLandedDiscussions(file, reference, code string) knowledge.Snapshot {
	var facts []knowledge.Checked
	add := func(id, ref string, landing *knowledge.DiscussionLanding) {
		facts = append(facts, knowledge.Checked{Status: knowledge.StatusDiscussionSource, Fact: knowledge.Fact{
			ID: id, Category: knowledge.CategoryDiscussionSource, Scope: file, Source: file,
			Citations: []knowledge.Citation{{Kind: "discussion", Path: file, Reference: ref,
				ObservedAt: "2025-01-01T00:00:00Z", Quote: strings.Repeat("Accepted repository discussion. ", 40), Landing: landing}},
		}})
	}
	for _, ref := range []string{"a", "b", "c", "d", "e", "f"} {
		add(ref, ref, nil)
	}
	add("z", reference, &knowledge.DiscussionLanding{Quote: code})
	return knowledge.ReviewSnapshot(facts)
}

type landedReviewSpy struct{ evidence string }

func (p *landedReviewSpy) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if len(req.Messages) != 0 && strings.Contains(req.Messages[0].ExtractText(), businessReviewPrompt) {
		p.evidence = req.Messages[len(req.Messages)-1].ExtractText()
		reply := `{"checks":[]}`
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}}}, nil
	}
	return agentTaskDoneResponse(), nil
}

func TestReviewGroupKeepsLandedRetryDiscussionInItsOwnFile(t *testing.T) {
	const policy, queue = "policy.go", "queue.go"
	const ref = "review:retry-accounting"
	const quote = "    return cfg.MaxRetries"
	dir := initPreviewRepo(t)
	git := gitcmd.New(1)
	output := func(args ...string) string {
		t.Helper()
		out, err := git.Output(t.Context(), dir, args...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	original := "package jobs\ntype Config struct { MaxRetries int }\nfunc limit(cfg Config) int {\n" + quote + "\n}\n" + strings.Repeat("\n", 9) + "func delay() int { return 1 }\n"
	const oldQueue = "package jobs\nfunc queued(cfg Config) int { return 1 }\n"
	const newQueue = "package jobs\nfunc queued(cfg Config) int { return cfg.MaxRetries }\n"
	commitIn(t, dir, policy, original, "record the retry policy")
	commitIn(t, dir, queue, oldQueue, "introduce a separate queue")
	base := output("rev-parse", "HEAD")
	commitIn(t, dir, queue, newQueue, "consult retry count in the queue")
	for _, tc := range []struct {
		name, after string
		want        bool
	}{
		{"unrelated policy edit", strings.Replace(original, "return 1", "return 2", 1), false},
		{"policy itself changes the retry calculation", strings.Replace(original, quote, quote+" + 2", 1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			commitIn(t, dir, policy, tc.after, tc.name)
			diffs := []model.Diff{{NewPath: policy, NewFileContent: tc.after, Diff: output("diff", base, "HEAD", "--", policy)}, {NewPath: queue, NewFileContent: newQueue, Diff: output("diff", base, "HEAD", "--", queue)}}
			spy := &landedReviewSpy{}
			a := newManifestFlowAgentWithClient(t, diffs, nil, spy)
			a.args.Knowledge = competingLandedDiscussions(policy, ref, quote)
			complete, stop, err := a.executeGroupSubtask(t.Context(), FileGroup{Diffs: diffs})
			if !complete || stop != nil || err != nil || spy.evidence == "" || strings.Contains(spy.evidence, ref) != tc.want {
				t.Fatalf("review complete=%t stop=%v err=%v evidence=%t reference=%t want=%t", complete, stop, err, spy.evidence != "", strings.Contains(spy.evidence, ref), tc.want)
			}
		})
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

func TestDirectCalleeFindingUsesItsOwnPremise(t *testing.T) {
	const callee, caller, testFile = "sizing.py", "queue.py", "test_sizing.py"
	const limit = "LIMIT = 8"
	const missing = "if items is None:\n        return LIMIT"
	const presence = "assert width(values) is not None"
	const strong = "assert width(\n        values\n    ) == 8"
	const withSetup = "values = range(12)\n    " + presence
	const both = withSetup + "\n    " + strong
	const widthHeader = "def width( items ):"
	const siblingTest = "assert preview_width(12) == 8"
	const previewReturn = "return min(count, 8)"
	const reviewed = "return len(items)"
	const before = limit + "\nfrom .defaults import MAX_ITEMS\ndef width(items):\n    " + missing + "\n    return min(len(items), LIMIT, MAX_ITEMS)\n\n" +
		"def export_width(items):\n    if items is None:\n        return width(items)\n    count = len(items)\n" +
		"    if count < LIMIT:\n        return count\n    return min(count, LIMIT)\n\n" +
		"def preview_width(count):\n    " + previewReturn + "\n\ndef test_preview():\n    " + siblingTest + "\n"
	after := strings.Replace(before, "return min(count, LIMIT)", reviewed, 1)
	after = strings.Replace(after, "def width(items):", widthHeader, 1)
	const changedLine = 14
	const realFinding = "An export of twelve items now returns 12, above the limit of eight."
	const falseFinding = "Exporting with no items supplied now loses the fallback width of eight."
	dir := initPreviewRepo(t)
	commitIn(t, dir, "defaults.py", "MAX_ITEMS = 8\n", "declare the permitted batch size")
	commitIn(t, dir, callee, before, "apply the common limit to exports")
	commitIn(t, dir, caller, "from .sizing import width\n\ndef enqueue(items):\n    return width(items)\n", "queue batches using the accepted width")
	commitIn(t, dir, testFile, "from .sizing import width\n\ndef test_present():\n    "+both+"\n", "check batch presence and the cap")
	ctx, git := t.Context(), gitcmd.New(1)
	output := func(args ...string) string {
		t.Helper()
		b, err := git.Output(ctx, dir, args...)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	base := knowledge.Tree{Repo: dir, Git: git, Commit: strings.TrimSpace(output("rev-parse", "HEAD"))}
	commitIn(t, dir, callee, after, "return raw export count")
	head := knowledge.Tree{Repo: dir, Git: git, Commit: strings.TrimSpace(output("rev-parse", "HEAD"))}
	change := model.Diff{OldPath: callee, NewPath: callee, NewFileContent: after, Diff: output("diff", base.Commit, head.Commit, "--", callee)}
	run := func(t *testing.T, includeLimit, constantFirst, separateRoutine bool, falseCounterproof string, decisions ...knowledge.Fact) {
		t.Helper()
		citations := []knowledge.Citation{{Kind: "code", Path: callee, StartLine: 4, EndLine: 6},
			{Kind: "code", Path: caller, StartLine: 4, EndLine: 4}, {Kind: "test", Path: testFile, StartLine: 3, EndLine: 8},
			{Kind: "test", Path: testFile, StartLine: 1, EndLine: 1}}
		defaultLimit := knowledge.Citation{Kind: "code", Path: "defaults.py", StartLine: 1, EndLine: 1}
		citations = append(citations, defaultLimit)
		if constantFirst {
			citations[2], citations[4] = citations[4], citations[2]
		}
		if includeLimit {
			citations = append(citations, knowledge.Citation{Kind: "code", Path: callee, StartLine: 1, EndLine: 1},
				knowledge.Citation{Kind: "code", Path: callee, StartLine: 11, EndLine: 11})
		}
		if falseCounterproof == siblingTest || falseCounterproof == previewReturn {
			line := uniqueBusinessQuoteLine(before, falseCounterproof)
			citations = append(citations, knowledge.Citation{Kind: "code", Path: callee, StartLine: line, EndLine: line})
		}
		f, err := base.Prepare(ctx, knowledge.Fact{Origin: "discovery", Category: knowledge.CategoryBusinessRule, Authority: knowledge.AuthorityInferred,
			Source: callee, Scope: callee, Claim: "Batch widths are capped at eight.", Citations: citations})
		if err != nil {
			t.Fatal(err)
		}
		f.Citations[1].Symbol = "enqueue"
		facts := append([]knowledge.Fact{f}, decisions...)
		checked, err := head.CheckReview(ctx, base.Commit, facts)
		if err != nil {
			t.Fatal(err)
		}
		snapshot := knowledge.ReviewSnapshot(checked)
		if err := snapshot.DescribeReviewChanges(ctx, base, head, checked); err != nil || len(snapshot) != len(facts) {
			t.Fatalf("snapshot = %+v, err = %v", snapshot, err)
		}
		direct, ok := head.WithDirectCalleeEvidence(ctx, snapshot[0], after)
		if !ok || (direct.DirectAssertion() == nil) != constantFirst {
			t.Fatalf("unexpected primary witness: %+v", direct.Citations)
		}
		defaultCheck := businessCheck{Path: callee, SourcePath: "defaults.py", SourceQuote: "MAX_ITEMS = 8"}
		if assertion, bound := defaultCheck.directCalleePremise(ctx, head, direct, after); !bound || assertion != nil {
			t.Fatal("an imported literal used by the reviewed callee lost its independent binding")
		}
		check := businessCheck{ID: snapshot[0].ID, Path: callee, Line: changedLine, Quote: reviewed, Reason: realFinding,
			Outcome: "violation", SourcePath: callee, SourceQuote: limit, Severity: "medium"}
		var drafts []model.LlmComment
		var first, second string
		if includeLimit {
			if separateRoutine {
				check.SourceQuote = "return min(len(items), LIMIT, MAX_ITEMS)"
			}
			importOnly := check
			importOnly.SourcePath, importOnly.SourceQuote = testFile, "from .sizing import width"
			importOnly.Reason = "Queued batches now drop items beyond the eighth."
			weak := importOnly
			weak.SourceQuote, weak.Reason = withSetup, "The queue now rejects a twelve-item batch."
			weak.Quote, weak.Line = widthHeader, 3
			testHeader := weak
			testHeader.SourceQuote, testHeader.Reason = "def test_present():", "All queued batches now lack a result."
			ambiguous, partial := weak, weak
			ambiguous.SourceQuote, ambiguous.Reason = both, "Queued batches now return zero."
			partial.SourceQuote, partial.Reason = "assert width(", "Queued batches now return a negative value."
			sameRoutine := check
			sameRoutine.SourceQuote, sameRoutine.Reason = "count = len(items)", "The changed width causes the queue to retry indefinitely."
			first = businessReply(t, check, importOnly, weak, testHeader, ambiguous, partial, sameRoutine)
			second = `{"checks":[{"id":"e-0","verdict":"confirmed","input":"12 items","expected":"8","before":"8","after":"12","reason":"The export now returns the length of the batch.","output_path":"sizing.py","output_line":14,"output_quote":"return len(items)"},{"id":"e-1","verdict":"confirmed","input":"12 items","expected":"8","before":"8","after":"12","reason":"The numeric assertion fails.","output_path":"sizing.py","output_line":6,"output_quote":"return min(len(items), LIMIT, MAX_ITEMS)","assertion_quote":"assert width(\n        values\n    ) == 8","assertion_before_passes":true,"assertion_after_passes":false}]}`
		} else {
			if snapshot[0].ContextStatus() != knowledge.StatusCandidate {
				t.Fatalf("expected a current inferred rule, got %s", snapshot[0].ContextStatus())
			}
			for _, text := range []string{realFinding, falseFinding} {
				drafts = append(drafts, model.LlmComment{Path: callee, StartLine: changedLine, EndLine: changedLine, ExistingCode: reviewed, Content: text})
			}
			check.Outcome, check.SourcePath, check.SourceQuote, check.Reason = "refuted", testFile, presence, "The width test still passes."
			check.RefutedText, check.Candidate = realFinding, "c-0"
			fallback := check
			fallback.RefutedText, fallback.Candidate = falseFinding, "c-1"
			if len(decisions) != 0 {
				if !snapshot[1].IsDecision() || snapshot[1].ContextStatus() != knowledge.StatusHistorical {
					t.Fatal("explicit review decision lost its current implementation witness")
				}
				fallback.ID, fallback.SourcePath, fallback.SourceQuote = snapshot[1].ID, callee, missing
			}
			first = businessReply(t, check, fallback)
			check.Trigger, check.Effect = "Export twelve items.", "The finding reports a width above eight."
			fallback.Trigger, fallback.Effect = "Export with items set to None.", "The finding reports that the fallback width is lost."
			fallback.SourcePath, fallback.SourceQuote, fallback.Reason = callee, missing, "The unchanged branch returns the fallback limit."
			if falseCounterproof != "" {
				check.SourcePath, check.SourceQuote, check.Reason = callee, falseCounterproof, "The separately cited code still gives eight."
			}
			second = businessReply(t, check, fallback)
		}
		client := &fakeAgentClient{}
		for _, reply := range []string{first, second, `{"checks":[]}`} {
			client.responses = append(client.responses, &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}}})
		}
		a := New(Args{RepoDir: dir, LLMClient: client, KnowledgeTree: &head, Template: template.Template{MaxTokens: 16000}})
		for _, draft := range drafts {
			a.args.CommentCollector.Add(draft)
		}
		a.inputResolution.ResolvedBase = base.Commit
		g := FileGroup{Diffs: []model.Diff{change}}
		g.business = selectBusinessContext(snapshot, g.Diffs)
		a.verifyBusinessFindings(ctx, g, "", change.Diff)
		comments := a.args.CommentCollector.CommentsForPath(callee)
		tasks := a.session.GetOrCreateFileSession(fileGroupKey(g.Diffs)).TaskRecords
		if includeLimit {
			effects := tasks[session.BusinessEffectTask]
			if len(effects) != 1 {
				t.Fatalf("declaration effect checks = %d", len(effects))
			}
			payload := effects[0].RequestMessages[1].ExtractText()
			var request struct {
				Proposals    []effectInput        `json:"proposals"`
				Declarations []knowledge.Citation `json:"declarations"`
			}
			if err := json.Unmarshal([]byte(payload), &request); err != nil || len(request.Proposals) != 2 ||
				request.Proposals[0].Assertion != nil || request.Proposals[0].Premise != check.SourceQuote ||
				request.Proposals[1].Premise != withSetup || request.Proposals[1].Assertion == nil ||
				len(request.Proposals[1].Assertion.Predicates) != 1 || strings.TrimSpace(request.Proposals[1].Assertion.Predicates[0].Quote) != presence {
				t.Fatalf("the selected premises did not reach their own outcome checks: %s", payload)
			}
			if separateRoutine {
				if len(request.Proposals[0].Routines) != 2 {
					t.Fatalf("expected both current functions: %s", payload)
				}
			} else if len(request.Declarations) != 1 || request.Declarations[0].Quote != limit || request.Proposals[0].Declaration == nil {
				t.Fatalf("missing independently cited declaration: %s", payload)
			}
		} else if len(tasks[session.BusinessRefutationTask]) != 1 {
			t.Fatal("ordinary findings did not reach independent reassessment")
		}
		if !includeLimit && len(decisions) == 0 {
			if !slices.Equal(comments, drafts) {
				t.Fatalf("the inferred caller rule changed an ordinary finding: %+v", comments)
			}
		} else if len(comments) != 1 || !strings.Contains(comments[0].Content, realFinding) {
			t.Fatalf("expected just the verified 12-item regression; published %+v", comments)
		}
	}
	t.Run("a declaration after the test verifies the export", func(t *testing.T) { run(t, true, false, false, "") })
	t.Run("a test after a declaration still needs its own outcome", func(t *testing.T) { run(t, true, true, false, "") })
	t.Run("a different producing function still gets an outcome check", func(t *testing.T) { run(t, true, false, true, "") })
	t.Run("own implementation evidence cannot withdraw an ordinary finding", func(t *testing.T) { run(t, false, false, false, "") })
	t.Run("a secondary weak test cannot withdraw an ordinary finding", func(t *testing.T) { run(t, false, true, false, "") })
	t.Run("the None decision cannot dismiss a twelve-item export", func(t *testing.T) {
		witness, err := base.Cite(ctx, callee, 4, 5)
		if err != nil {
			t.Fatal(err)
		}
		decision := knowledge.Fact{ReviewID: "export-review", Revision: base.Commit, Scope: callee, Source: callee,
			Finding: falseFinding, Reason: "An export with no items calls width(None) and returns the fallback of eight.",
			Citations: []knowledge.Citation{witness}}
		// Both independent verdicts cite the same branch at the same reviewed
		// line. Only the None verdict has the recorded decision for its input.
		run(t, false, false, false, missing, decision)
	})
	t.Run("a colocated test is not production counterevidence", func(t *testing.T) { run(t, false, false, false, siblingTest) })
	t.Run("an unrelated production function cannot disprove an export result", func(t *testing.T) {
		run(t, false, false, false, previewReturn)
	})
}

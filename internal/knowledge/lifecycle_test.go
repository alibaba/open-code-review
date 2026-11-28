// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

type lifecycleRepo struct {
	t *testing.T
	p string
}

func TestReharvestRespectsRejectedBusinessClaim(t *testing.T) {
	a := Fact{ID: "rejected", Category: CategoryDomainConstraint, Source: "router.go", Scope: "submit.go",
		Claim: "One pending request per order.", Subject: "order", Predicate: "pending limit", Value: json.RawMessage("1"),
		Citations: []Citation{{Kind: "code", Path: "router.go", Digest: "first code witness"}}}
	b := a
	b.ID, b.Claim, b.Identifiers, b.Value = "reworded", "Each order can have one pending request.", []string{"Pending"}, json.RawMessage("1.0")
	otherState := b
	otherState.ID = "new-condition"
	otherState.Conditions = []Condition{{Description: "Only for card payments"}}
	otherEvidence := b
	otherEvidence.ID = "new-evidence"
	otherEvidence.Citations = []Citation{{Kind: "code", Path: "router.go", Digest: "independent new evidence"}}
	confirmed := b
	confirmed.ID = "human-reviewed"
	checked := []Checked{{Fact: a, Status: StatusRejected}, {Fact: b, Status: StatusCandidate}, {Fact: otherState, Status: StatusCandidate}, {Fact: otherEvidence, Status: StatusCandidate}, {Fact: confirmed, Status: StatusCurrent}}
	linkRejectedObservations(checked)
	if checked[1].Status != StatusConflict || !slices.Contains(checked[1].Conflicts, a.ID) ||
		checked[2].Status != StatusCandidate || checked[3].Status != StatusCandidate || checked[4].Status != StatusCurrent {
		t.Fatalf("incorrect feedback scope: %+v", checked)
	}
	successor := []Checked{{Fact: a, Status: StatusSuperseded}, {Fact: b, Status: StatusCandidate}}
	linkRejectedObservations(successor)
	if successor[1].Status != StatusCandidate {
		t.Fatal("replacement alone does not reject an equivalent successor")
	}
}

func TestReviewDoesNotLearnItsOwnProposedBehavior(t *testing.T) {
	ctx := t.Context()
	repo := newLifecycleRepo(t)
	repo.write("fee.py", "def fee(order):\n    return 4\n")
	base := repo.commit("accepted fixed fee")
	live := func(tree Tree, claim string) Fact {
		c, err := tree.Cite(ctx, "fee.py", 2, 2)
		if err != nil {
			t.Fatal(err)
		}
		f, err := tree.Prepare(ctx, Fact{Origin: "discovery", Category: CategoryBusinessRule, Authority: AuthorityInferred,
			Claim: claim, Source: "fee.py", Scope: "fee.py", Revision: tree.Commit, Citations: []Citation{c}})
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	initial := live(base, "Fee is four.")
	repo.write("fee.py", "def fee(order):\n    return 10\n")
	proposed := repo.commit("propose another fee")
	later := live(proposed, "Fee is ten.")
	db := testStore(t)
	if err := db.Save(ctx, []Fact{initial, later}); err != nil {
		t.Fatal(err)
	}
	loaded, err := db.List(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := proposed.CheckReview(ctx, base.Commit, loaded)
	if err != nil || len(got) != 1 || got[0].Claim != initial.Claim || got[0].Status != StatusChanged {
		t.Fatalf("proposal became its own business premise: %+v, %v", got, err)
	}
	got, err = proposed.CheckReview(ctx, "", loaded)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty accepted tree: %+v, %v", got, err)
	}
	got, err = proposed.CheckReview(ctx, proposed.Commit, loaded)
	if err != nil || len(got) != 2 {
		t.Fatalf("accepted after merge: %+v, %v", got, err)
	}
}

func newLifecycleRepo(t *testing.T) lifecycleRepo {
	r := lifecycleRepo{t, t.TempDir()}
	r.git("init", "-q")
	return r
}

func (r lifecycleRepo) git(args ...string) string {
	r.t.Helper()
	flags := []string{"-c", "user.name=Test", "-c", "user.email=test@example.org", "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}
	cmd := exec.CommandContext(r.t.Context(), "git", append(flags, args...)...)
	cmd.Dir = r.p
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %s (%v)", args, out, err)
	}
	return strings.TrimSpace(string(out))
}

func (r lifecycleRepo) write(p, text string) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.p, p), []byte(text), 0600); err != nil {
		r.t.Fatal(err)
	}
}

func (r lifecycleRepo) commit(message string) Tree {
	r.git("add", ".")
	r.git("commit", "-qm", message)
	return Tree{Repo: r.p, Commit: r.git("rev-parse", "HEAD"), Git: gitcmd.New(1)}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "knowledge.db"), "repo", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func prepareFact(t *testing.T, tree Tree, f Fact) Fact {
	t.Helper()
	f, err := tree.Prepare(t.Context(), f)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func checkedFacts(t *testing.T, tree Tree, facts []Fact) []Checked {
	t.Helper()
	got, err := tree.Check(t.Context(), facts)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestConfirmedPremiseTracksFormattingMovesAndChangedCaller(t *testing.T) {
	r := newLifecycleRepo(t)
	r.write("entry.go", "package payment\n\nfunc validate(amount int) bool {\n\treturn amount > 0\n}\n")
	r.write("charge.go", "package payment\n\nfunc charge(amount int) int {\n\treturn amount\n}\n")
	tree := r.commit("initial")
	ctx := t.Context()
	fact := prepareFact(t, tree, Fact{Category: CategoryBusinessRule, Authority: AuthorityInferred, Scope: "charge.go", Source: "charge.go",
		Claim: "Charge is reached with a positive amount.", Identifiers: []string{"charge"},
		Citations: []Citation{{Kind: "code", Path: "charge.go", StartLine: 3, EndLine: 5}},
		Conditions: []Condition{{Description: "The entry point validates the amount before charging.", Citation: &Citation{
			Kind: "code", Path: "entry.go", StartLine: 4, EndLine: 4,
		}}},
	})
	s := testStore(t)
	facts := []Fact{fact}
	if err := s.Save(ctx, facts); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Feedback(ctx, tree, facts[0].ID, Feedback{Action: FeedbackConfirm, Reason: "Checked the call path", Actor: "maintainer"}); err != nil {
		t.Fatal(err)
	}
	load := func() []Fact {
		t.Helper()
		facts, err := s.List(ctx, "", "amount")
		if err != nil || len(facts) != 1 || len(facts[0].Feedback) != 1 {
			t.Fatalf("load knowledge and feedback: %+v, %v", facts, err)
		}
		return facts
	}
	if got := checkedFacts(t, tree, load()); got[0].Status != StatusCurrent ||
		ReviewSnapshot(got)[0].CurrentSchema() || got[0].Authority != AuthorityConfirmed {
		t.Fatalf("anchored confirmation: %+v", got)
	}
	r.git("mv", "entry.go", "validation.go")
	r.write("validation.go", "package payment\n\nfunc unrelated() {}\n\nfunc validate( amount int ) bool { return amount > 0 }\n")
	moved := r.commit("move and format the caller")
	got := checkedFacts(t, moved, load())
	if got[0].Status != StatusCurrent || got[0].Conditions[0].Citation.CurrentPath != "validation.go" ||
		got[0].Conditions[0].Citation.Path != "entry.go" {
		t.Fatalf("relocated current anchor: %+v", got[0])
	}
	r.write("validation.go", "package payment\n\nfunc unrelated() {}\n\nfunc validate(amount int) bool { return true }\n")
	current := r.commit("remove the caller guard")
	got = checkedFacts(t, current, load())
	if got[0].Status != StatusChanged || !strings.Contains(got[0].Detail, "validation.go") {
		t.Fatalf("stale caller condition retained: %+v", got)
	}
	if review := ReviewSnapshot(got).ForChange([]string{"charge.go"}, []string{"charge"}); strings.Contains(review, fact.Claim) {
		t.Fatalf("stale premise entered review: %s", review)
	}
}

func TestDecisionsRemainReviewLocalAndReconcilable(t *testing.T) {
	r := newLifecycleRepo(t)
	r.write("charge.go", "package payment\nfunc charge() { println(5) }\n")
	r.write("entry.go", "package payment\nfunc validate(amount int) bool { return amount > 0 }\n")
	original := r.commit("initial")
	ctx := t.Context()
	one, err := original.Decide(ctx, "entry.go", "charge.go", "PR-3", "Check input validation", "The caller enforces positive values")
	if err != nil {
		t.Fatal(err)
	}
	two, err := original.DecideWithCitation(ctx, "entry.go", "charge.go", "PR-3", "check  input validation", "This path accepts negative values", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	facts := []Fact{one, two}
	if err := s.Save(ctx, facts); err != nil {
		t.Fatal(err)
	}
	if got, err := s.List(ctx, "PR-4", ""); err != nil || len(got) != 0 {
		t.Fatalf("decision leaked into another review: %+v, %v", got, err)
	}
	r.write("entry.go", "package payment\n\nfunc validate( amount int ) bool {\n    return amount > 0\n}\n")
	target := r.commit("format the caller")
	got := checkedFacts(t, target, facts)
	for _, c := range got {
		if c.Status != StatusConflict || len(c.Conflicts) != 1 || c.CurrentSchema() {
			t.Fatalf("contradictory rationale: %+v", got)
		}
	}
	context := ReviewSnapshot(got)
	if outside := context.ForChange([]string{"entry.go"}, []string{"charge"}); outside != "" {
		t.Fatalf("decision escaped its exact reviewed file: %s", outside)
	}
	inside := context.ForPaths([]string{"charge.go"})
	if !strings.Contains(inside, one.Reason) || !strings.Contains(inside, two.Reason) || !strings.Contains(inside, `"review_context":"conflict"`) {
		t.Fatalf("group lacks conflicting historical leads: %s", inside)
	}
	if _, err := s.Feedback(ctx, target, facts[1].ID, Feedback{Action: FeedbackReject, Reason: "This path uses the same caller"}); err != nil {
		t.Fatal(err)
	}
	stored, err := s.List(ctx, "PR-3", "")
	if err != nil {
		t.Fatal(err)
	}
	got = checkedFacts(t, target, stored)
	if !slices.ContainsFunc(got, func(c Checked) bool { return c.Status == StatusRejected && c.ID == facts[1].ID }) ||
		!slices.ContainsFunc(got, func(c Checked) bool { return c.Status == StatusHistorical && c.ID == facts[0].ID }) {
		t.Fatalf("rejected rationale still conflicts with the historical lead: %+v", got)
	}
	if _, err := s.Feedback(ctx, target, facts[0].ID, Feedback{Action: FeedbackConfirm, Reason: "Free-text decision"}); err == nil {
		t.Fatal("hash-matched rationale accepted as confirmed")
	}
}

func TestRuleConflictsRespectConditionalScopeAndReplacements(t *testing.T) {
	r := newLifecycleRepo(t)
	r.write("currency.go", "package payment\n\nfunc currency(kind string) string {\n\tif kind == \"refund\" { return \"EUR\" }; return \"USD\"\n}\n")
	tree := r.commit("currency rules")
	citation := Citation{Kind: "code", Path: "currency.go", StartLine: 3, EndLine: 5}
	ctx := t.Context()
	rule := func(scope, kind, value string) Fact {
		f := Fact{Category: CategoryDomainConstraint, Authority: AuthorityInferred, Scope: scope, Source: citation.Path,
			Claim: "Currency is " + value + " for " + kind, Subject: "payment", Predicate: "currency",
			Value: json.RawMessage(value), Citations: []Citation{citation}, Identifiers: []string{"currency"}}
		if kind != "" {
			f.Conditions = []Condition{{Description: "Transaction kind is " + kind, Citation: &citation,
				Key: "transaction.kind", Equals: json.RawMessage(kind)}}
		}
		return prepareFact(t, tree, f)
	}
	sale := rule(".", `"sale"`, `"USD"`)
	refund := rule("currency.go", `"refund"`, `"EUR"`)
	facts := []Fact{sale, refund}
	s := testStore(t)
	if err := s.Save(ctx, facts); err != nil {
		t.Fatal(err)
	}
	for _, c := range checkedFacts(t, tree, facts) {
		if c.Status != StatusCandidate {
			t.Fatalf("disjoint conditions conflict: %+v", c)
		}
	}
	other := rule("currency.go", `"refund"`, `"USD"`)
	replacement := []Fact{other}
	if err := s.Save(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	facts = append(facts, replacement...)
	got := checkedFacts(t, tree, facts)
	if got[0].Status != StatusCandidate || got[1].Status != StatusConflict || got[2].Status != StatusConflict {
		t.Fatalf("overlapping facts: %+v", got)
	}
	text := ReviewSnapshot(got).ForPaths([]string{"currency.go"})
	if !strings.Contains(text, "Transaction kind is") || !strings.Contains(text, facts[2].ID) {
		t.Fatalf("conditional context absent from conflict lead: %s", text)
	}
	if _, err := s.Feedback(ctx, tree, facts[1].ID, Feedback{
		Action: FeedbackReplace, Reason: "Reviewed the updated refund behavior", ReplacementID: facts[2].ID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(ctx, facts); err != nil {
		t.Fatal(err)
	}
	stored, err := s.List(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	got = checkedFacts(t, tree, stored)
	if !slices.ContainsFunc(got, func(c Checked) bool { return c.ID == facts[1].ID && c.Status == StatusSuperseded }) ||
		!slices.ContainsFunc(got, func(c Checked) bool { return c.ID == facts[2].ID && c.Status == StatusCurrent }) {
		t.Fatalf("same-revision reimport revived a replaced rule: %+v", got)
	}
}

func TestConditionKeysPreserveCase(t *testing.T) {
	r := newLifecycleRepo(t)
	r.write("rates.py", `def first(order):
    if order.Mode == "on":
        return 1

def second(order):
    if order.mode == "off":
        return 2

def third(order):
    if order.Mode == "off":
        return 2
`)
	tree := r.commit("case-sensitive order attributes")
	rule := func(line int, key, guard, value string) Fact {
		t.Helper()
		citation := Citation{Kind: "code", Path: "rates.py", StartLine: line, EndLine: line + 1}
		return prepareFact(t, tree, Fact{
			Category: CategoryBusinessRule, Authority: AuthorityInferred, Source: "rates.py", Scope: "rates.py",
			Claim: "The order rate is " + value, Subject: "order", Predicate: "rate", Value: json.RawMessage(value),
			Citations: []Citation{citation}, Conditions: []Condition{{Description: key + " equals " + guard,
				Key: key, Equals: json.RawMessage(guard), Citation: &citation}},
		})
	}
	first := rule(2, "order.Mode", `"on"`, "1")
	for _, pair := range []struct {
		second Fact
		status string
	}{
		{rule(6, "order.mode", `"off"`, "2"), StatusConflict},
		{rule(10, "order.Mode", `"off"`, "2"), StatusCandidate},
	} {
		for _, c := range checkedFacts(t, tree, []Fact{first, pair.second}) {
			if c.Status != pair.status {
				t.Errorf("guard %q: got %s, want %s", pair.second.Conditions[0].Key, c.Status, pair.status)
			}
		}
	}
}

func TestExpiredRuleAndCompactEvidence(t *testing.T) {
	r := newLifecycleRepo(t)
	r.write("evidence.md", strings.Repeat("x", 1600)+"\n")
	r.write("charge.go", "package payment\n\nfunc charge(amount int) int { return amount }\n")
	tree := r.commit("add evidence")
	tree.Now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	past := tree.Now.Add(-time.Hour).Format(time.RFC3339)
	future := tree.Now.Add(time.Hour).Format(time.RFC3339)
	citations := []Citation{{Kind: "code", Path: "charge.go", StartLine: 3, EndLine: 3}}
	for range 7 {
		citations = append(citations, Citation{Kind: "documentation", Path: "evidence.md", StartLine: 1, EndLine: 1})
	}
	f := Fact{Scope: "charge.go", Source: "charge.go", Category: CategoryBusinessRule, Authority: AuthorityInferred, Claim: "Amounts flow through unchanged.",
		Identifiers: []string{"charge"}, Citations: citations, ExpiresAt: past}
	expired := prepareFact(t, tree, f)
	f.ExpiresAt = future
	active := prepareFact(t, tree, f)
	got := checkedFacts(t, tree, []Fact{expired, active})
	if got[0].Status != StatusExpired || got[1].Status != StatusCandidate {
		t.Fatalf("expiry: %+v", got)
	}
	text := ReviewSnapshot(got).ForChange([]string{"different.go"}, []string{"charge"})
	if !strings.Contains(text, got[1].ID) || strings.Contains(text, got[0].ID) ||
		!strings.Contains(text, `"additional_citations":6`) || !strings.Contains(text, "[excerpt]") {
		t.Fatalf("large multi-source lead was not compactly rendered: %s", text)
	}
}

func TestNonGoPremiseRequiresUnchangedContext(t *testing.T) {
	r := newLifecycleRepo(t)
	r.write("dispatch.py", "def dispatch(order):\n    if order.ready:\n        publish(order)\n")
	base := r.commit("dispatch ready orders")
	facts := []Fact{prepareFact(t, base, Fact{
		Scope: "dispatch.py", Source: "dispatch.py", Category: CategoryBusinessRule, Authority: AuthorityInferred,
		Claim:     "Ready orders get published.",
		Citations: []Citation{{Kind: "code", Path: "dispatch.py", StartLine: 3, EndLine: 3}},
	})}
	s := testStore(t)
	ctx := t.Context()
	if err := s.Save(ctx, facts); err != nil {
		t.Fatal(err)
	}
	confirm := Feedback{Action: FeedbackConfirm, Reason: "Checked the current dispatcher"}
	if _, err := s.Feedback(ctx, base, facts[0].ID, confirm); err != nil {
		t.Fatal(err)
	}
	r.write("dispatch.py", "def dispatch(order):\n    if order.ready and not order.suppressed:\n        publish(order)\n")
	target := r.commit("add suppression")
	saved, err := s.List(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	current := checkedFacts(t, target, saved)
	if current[0].Status != StatusChanged {
		t.Fatalf("same selected line after a guard change: %+v", current[0])
	}
	if _, err := s.Feedback(ctx, target, facts[0].ID, confirm); err == nil {
		t.Fatal("accepted confirmation of a changed citation context")
	}
}

func TestReplayWithholdsLaterDecisionsDiscussionsAndFeedback(t *testing.T) {
	r := newLifecycleRepo(t)
	r.write("queue.go", "package dispatch\n\nfunc send() {}\n")
	base := r.commit("add queue")
	plain := Fact{
		Scope: "queue.go", Source: "queue.go", Category: CategoryBusinessRule, Authority: AuthorityInferred,
		Claim:     "The dispatcher invokes the queue directly.",
		Citations: []Citation{{Kind: "code", Path: "queue.go", StartLine: 3, EndLine: 3}},
	}
	now := time.Now()
	ctx := t.Context()
	note := "Note about the queue"
	fromDiscussion := plain
	fromDiscussion.Citations = append(slices.Clone(plain.Citations), Citation{
		Kind: "discussion", Path: "queue.go", Reference: "comment-2", Digest: Digest([]byte(note)),
		Quote: note, StartLine: 1, EndLine: 1, ObservedAt: now.Format(time.RFC3339Nano),
	})
	decision, err := base.Decide(ctx, "queue.go", "queue.go", "PR-2", "Check dispatch", "The queue is synchronous here")
	if err != nil {
		t.Fatal(err)
	}
	facts := []Fact{prepareFact(t, base, plain), prepareFact(t, base, fromDiscussion), decision}
	s := testStore(t)
	if err := s.Save(ctx, facts); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Feedback(ctx, base, facts[0].ID, Feedback{Action: FeedbackConfirm, Reason: "The evidence agrees"}); err != nil {
		t.Fatal(err)
	}
	stored, err := s.List(ctx, "PR-2", "")
	if err != nil {
		t.Fatal(err)
	}
	statuses := func(at time.Time) map[string]string {
		t.Helper()
		target := base
		target.Now = at
		result := make(map[string]string)
		for _, c := range checkedFacts(t, target, stored) {
			result[c.ID] = c.Status
		}
		return result
	}
	early := statuses(now.Add(-time.Hour))
	late := statuses(now.Add(time.Hour))
	if early[facts[0].ID] != StatusCandidate || early[facts[1].ID] != StatusUnverified || early[facts[2].ID] != StatusUnverified {
		t.Fatalf("future history entered the earlier comparison: %v", early)
	}
	if late[facts[0].ID] != StatusCurrent || late[facts[1].ID] != StatusCandidate || late[facts[2].ID] != StatusHistorical {
		t.Fatalf("causally available history missing from the later comparison: %v", late)
	}
}

func TestSchemaRecheckAccountsForEquivalentLiterals(t *testing.T) {
	r := newLifecycleRepo(t)
	r.write("amount.json", `{"properties":{"amount":{"const":1.0}}}`)
	r.write("related.json", `{"properties":{"amount":{"enum":[1e0,2]}}}`)
	base := r.commit("add related contracts")
	load := func(p string) Fact {
		t.Helper()
		found, err := base.Learn(t.Context(), p, "src", "amount")
		if err != nil {
			t.Fatal(err)
		}
		return found[0]
	}
	facts := []Fact{load("amount.json"), load("related.json")}
	r.write("amount.json", `{"properties":{"amount":{"const":1e0}}}`)
	target := r.commit("normalize numeric notation")
	got := checkedFacts(t, target, facts)
	if got[0].Status != StatusVerified || got[1].Status != StatusVerified {
		t.Fatalf("equivalent JSON values looked changed or inconsistent: %+v", got)
	}
}

func TestEquivalentRuleLeadKeepsRicherProvenance(t *testing.T) {
	r := newLifecycleRepo(t)
	r.write("queue.go", "package dispatch\n\nfunc retryLimit() int { return 1 }\n")
	tree := r.commit("add retry limit")
	cite := Citation{Kind: "code", Path: "queue.go", StartLine: 3, EndLine: 3}
	plain := Fact{Scope: "queue.go", Source: "queue.go", Category: CategoryDomainConstraint, Authority: AuthorityInferred,
		Claim: "Retry limit is one.", Subject: "queue", Predicate: "limit", Value: json.RawMessage("1"), Citations: []Citation{cite}}
	richer := plain
	note := "One retry is sufficient under the controlled retry interval"
	richer.Citations = append(slices.Clone(plain.Citations), Citation{Kind: "discussion", Reference: "note-3",
		ObservedAt: time.Now().Format(time.RFC3339Nano), Digest: Digest([]byte(note)), Quote: note, StartLine: 1, EndLine: 1})
	richer.Value = json.RawMessage("1e0")
	conditional := plain
	conditional.Conditions = []Condition{{Description: "Only for the main queue", Citation: &cite}}
	facts := []Fact{prepareFact(t, tree, plain), prepareFact(t, tree, richer), prepareFact(t, tree, conditional)}
	s := testStore(t)
	if err := s.Save(t.Context(), facts); err != nil {
		t.Fatal(err)
	}
	got, err := s.List(t.Context(), "", "")
	if err != nil || len(got) != 3 {
		t.Fatalf("equivalent lead lost its separate provenance in storage: %+v (%v)", got, err)
	}
	text := ReviewSnapshot(checkedFacts(t, tree, got)).ForPaths([]string{"queue.go"})
	if strings.Contains(text, facts[0].ID) || !strings.Contains(text, facts[1].ID) || !strings.Contains(text, facts[2].ID) {
		t.Fatalf("review repeated an equivalent lead or dropped a distinct condition: %s", text)
	}
}

func TestDiscoveryReceiptAndLegacyDatabase(t *testing.T) {
	ctx := t.Context()
	name := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", name)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `CREATE TABLE facts (fact_rowid INTEGER PRIMARY KEY, repo TEXT NOT NULL, id TEXT NOT NULL, review TEXT NOT NULL,
 body TEXT NOT NULL, recorded_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, revoked_at TEXT, UNIQUE(repo,id));
 CREATE TABLE fact_revisions (fact_row INTEGER NOT NULL, revision TEXT NOT NULL, PRIMARY KEY(fact_row,revision));
 CREATE VIRTUAL TABLE fact_search USING fts5(text, tokenize='trigram');
 INSERT INTO facts(repo,id,review,body) VALUES('repo','legacy','','{"scope":"charge.go","source":"contract.json","id":"legacy"}');
 INSERT INTO fact_revisions(fact_row,revision) VALUES(1,'old-revision');`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	read, err := Open(ctx, name, "repo", false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := read.List(ctx, "", "")
	read.Close()
	if err != nil || len(got) != 1 || got[0].Revision != "old-revision" {
		t.Fatalf("read pre-migration database: %+v, %v", got, err)
	}
	s, err := Open(ctx, name, "repo", true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.SaveDiscovery(ctx, "scan", []Fact{{Value: json.RawMessage("invalid")}}); err == nil {
		t.Fatal("invalid scan write succeeded")
	}
	if cached, err := s.Scanned(ctx, "scan"); err != nil || cached {
		t.Fatalf("failed scan left a receipt: %t, %v", cached, err)
	}
	if err := s.SaveDiscovery(ctx, "scan", []Fact{{Claim: "First committed scan"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDiscovery(ctx, "scan", []Fact{{Claim: "Second scan of the same input"}}); err != nil {
		t.Fatal(err)
	}
	got, err = s.List(ctx, "", "")
	if err != nil || len(got) != 2 || slices.ContainsFunc(got, func(f Fact) bool { return strings.Contains(f.Claim, "Second") }) {
		t.Fatalf("scan receipt was not atomic or idempotent: %+v, %v", got, err)
	}
}

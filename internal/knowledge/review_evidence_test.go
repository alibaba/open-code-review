// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestForReviewOrdersLandedEvidenceFromAcceptedSource(t *testing.T) {
	const local = "workers/sites/local.py"
	const driver = "workers/driver.py"
	const boundary = "        return self.batch_cursor"
	const localBase = `from ..driver import Driver as Base

class Local(Base):
    batch_limit: int = 4

    def cursor(self):
` + boundary + "\n"
	const driverBase = `class Driver:
    def process(self, batch):
        if batch is None:
            return
        pages = tuple(batch)
        if not pages:
            return
        self.batch_cursor = pages[-1]
`
	repo := newDiscoveryRepo(t)
	repo.write(local, localBase)
	repo.write(driver, driverBase)
	base := repo.commit("accept cursor tracking")
	const merged = "2025-02-04T15:00:00Z"
	general := Discussion{ID: "review/1", Path: local, Revision: base.Commit, UpdatedAt: discussionTestStamp, Body: "The worker can be deployed."}
	landed := Discussion{ID: "review/2", Path: local, Revision: base.Commit, UpdatedAt: discussionTestStamp, Body: "A completed batch retains its last cursor.",
		landing: &DiscussionLanding{OriginalRevision: base.Commit, OriginalStart: 7, OriginalEnd: 7, LandedStart: 7, Quote: boundary, MergedAt: merged}}
	discovered := discoverWithoutRules(t, base, DiscoverOptions{Paths: []string{local}, Discussions: []Discussion{general, landed}})
	db := testStore(t)
	if err := db.Save(t.Context(), discovered.DiscussionObservations()); err != nil {
		t.Fatal(err)
	}
	records := storedFacts(t, db)
	if len(records) != 2 {
		t.Fatalf("expected both dated discussions, got %+v", records)
	}
	localHead := strings.Replace(localBase, "= 4", "= 2", 1)
	driverHead := strings.Replace(driverBase, "if not pages:", "if len(pages) < self.batch_limit:", 1)
	repo.write(local, localHead)
	repo.write(driver, driverHead)
	head := repo.commit("apply the local batch limit")
	change := func(file, content string) model.Diff {
		return model.Diff{NewPath: file, Diff: repo.git("diff", "--unified=2", base.Commit, head.Commit, "--", file), NewFileContent: content}
	}
	localChange, driverChange := change(local, localHead), change(driver, driverHead)
	paths := []string{local, driver}
	names := []string{"batch_cursor", "batch_limit"}
	snapshot := ReviewSnapshot(checkedFacts(t, head, records))
	for _, tc := range []struct {
		name    string
		changes []model.Diff
		first   Discussion
	}{
		{"imported base reads the override beside the cursor", []model.Diff{localChange, driverChange}, landed},
		{"only the override changes", []model.Diff{localChange}, general},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, text := snapshot.ForReview(paths, names, tc.changes)
			if len(got) != 2 || got[0].Citations[0].Reference != tc.first.ID {
				t.Fatalf("unexpected first source for %s: %s\n%s\n%s", tc.name, text, localChange.Diff, driverChange.Diff)
			}
			if strings.Index(text, tc.first.Body) < 0 || strings.Index(text, tc.first.Body) >= strings.Index(text, got[1].Citations[0].Quote) {
				t.Fatalf("the reviewer did not receive the selected sources in order: %s", text)
			}
		})
	}
	before, err := time.Parse(time.RFC3339, merged)
	if err != nil {
		t.Fatal(err)
	}
	head.Now = before.Add(-time.Nanosecond)
	early := ReviewSnapshot(checkedFacts(t, head, records))
	if got, text := early.ForReview(paths, names, []model.Diff{localChange, driverChange}); len(got) != 1 || strings.Contains(text, landed.Body) {
		t.Fatalf("before the merge only the general discussion was available: %s", text)
	}
	repo.write(local, strings.Replace(localHead, boundary, "        return None", 1))
	replaced := repo.commit("remove cursor access in the local worker")
	checked := ReviewSnapshot(checkedFacts(t, replaced, records))
	if _, text := checked.ForReview(paths, names, []model.Diff{driverChange}); strings.Contains(text, landed.Body) {
		t.Fatalf("a replaced landed line survived current-source validation: %s", text)
	}
}

func TestWithDirectCalleeEvidenceKeepsAnIndependentCurrentWitness(t *testing.T) {
	const callee = "work/sizing.py"
	const caller = "work/queue.py"
	const content = `DEFAULT_WIDTH = 8

async def width(items):
    return min(len(items), DEFAULT_WIDTH)
`
	repo := newDiscoveryRepo(t)
	repo.write(callee, content)
	repo.write(caller, "from .sizing import width\n\nasync def enqueue(items):\n    return await width(items)\n")
	const testPath = "work/test_sizing.py"
	repo.write(testPath, "from .sizing import width\n\nasync def test_width_limit():\n    assert await width(range(12)) == 8\n")
	const indirectTest = "work/test_queue.py"
	repo.write(indirectTest, "from .queue import enqueue\n\nasync def test_queue_limit():\n    assert await enqueue(range(12)) == 8\n")
	tree := repo.commit("limit pending batch width")
	rule := prepareFact(t, tree, Fact{Category: CategoryBusinessRule, Authority: AuthorityInferred, Scope: "work", Source: callee,
		Claim: "Batch width is limited to eight items.", Citations: []Citation{
			{Kind: "code", Path: callee, StartLine: 4, EndLine: 4},
			{Kind: "code", Path: caller, StartLine: 4, EndLine: 4},
			{Kind: "code", Path: callee, StartLine: 1, EndLine: 1},
		}, Conditions: []Condition{{Description: "An oversized batch is capped.", Citation: &Citation{Kind: "test", Path: testPath, StartLine: 4, EndLine: 4}}}})
	rule.AcceptedCall = &AcceptedCall{CallerPath: caller, Caller: "enqueue", CalleePath: callee, Callee: "width"}
	snapshot := ReviewSnapshot(checkedFacts(t, tree, []Fact{rule}))
	if len(snapshot) != 1 || len(snapshot[0].Citations) != 3 {
		t.Fatalf("expected one rule with three current citations, got %+v", snapshot)
	}
	fact := snapshot[0]
	direct := fact.Citations[2]
	result, ok := tree.WithDirectCalleeEvidence(t.Context(), fact, strings.ReplaceAll(content, "\n", "\r\n"))
	if !ok || len(result.Citations) != 3 || result.Citations[0] != direct || fact.Citations[0] == direct {
		t.Fatalf("expected the independent module constant first and the stored record unchanged: %+v", result)
	}
	if !slices.Equal(result.Citations[1:], fact.Citations[:2]) {
		t.Fatal("selecting the independent witness dropped the original call or changed its order")
	}
	witness := *fact.Conditions[0].Citation
	callOnly := fact
	callOnly.Citations = fact.Citations[:2:2]
	callOnly.Conditions = nil
	indirect := prepareFact(t, tree, Fact{Category: CategoryBusinessRule, Authority: AuthorityInferred, Source: indirectTest, Scope: fact.Scope,
		Claim: fact.Claim, Citations: []Citation{{Kind: "test", Path: indirectTest, StartLine: 4, EndLine: 4}}})
	indirect = ReviewSnapshot(checkedFacts(t, tree, []Fact{indirect}))[0]
	indirect.AcceptedCall = fact.AcceptedCall
	withTest := callOnly
	withTest.Conditions = fact.Conditions
	if got, ok := tree.WithDirectCalleeEvidence(t.Context(), withTest, content); !ok || got.Citations[0] != witness {
		t.Fatalf("an independent, current test in a condition was not displayed: %+v", got)
	}
	stale := direct
	stale.CurrentPath = ""
	missing := callOnly
	missing.Citations = append(missing.Citations, stale)
	outside := fact
	outside.Scope = caller
	constantOnly := fact
	constantOnly.Conditions = nil
	noCall := fact
	noCall.AcceptedCall = nil
	for _, tc := range []struct {
		name    string
		fact    Fact
		content string
	}{
		{"call pair alone", callOnly, content},
		{"test only exercises the caller", indirect, content},
		{"independent citation has no current match", missing, content},
		{"callee lies outside the rule scope", outside, content},
		{"no accepted invocation", noCall, content},
		{"reviewed bytes differ from the repository", constantOnly, strings.Replace(content, "= 8", "= 4", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := tree.WithDirectCalleeEvidence(t.Context(), tc.fact, tc.content); ok {
				t.Fatalf("unexpected independent witness: %+v", got.Citations)
			}
		})
	}
	duplicate := content + "\nasync def width(items):\n    return 0\n"
	repo.write(callee, duplicate)
	ambiguous := repo.commit("add a second implementation with the same name")
	if _, ok := ambiguous.WithDirectCalleeEvidence(t.Context(), constantOnly, duplicate); ok {
		t.Fatal("an ambiguous declaration authorized the module constant")
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func discoverWithoutRules(t *testing.T, tree Tree, opts DiscoverOptions) DiscoveryResult {
	t.Helper()
	result, err := tree.Discover(t.Context(), opts, func(context.Context, string, string) (string, error) {
		return `{"records":[]}`, nil
	})
	if err != nil || len(result.Facts) != 0 {
		t.Fatalf("empty extraction: %+v, %v", result, err)
	}
	return result
}

func storedFacts(t *testing.T, db *Store) []Fact {
	t.Helper()
	facts, err := db.List(t.Context(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

func TestDirectDiscussionSourcesWithoutExtractedRule(t *testing.T) {
	ctx := t.Context()
	repo := newDiscoveryRepo(t)
	const path = "jobs/dispatch.py"
	repo.write(path, "def dispatch(event):\n    return event.name\n")
	repo.write("jobs/worker.py", "def start(): return 1\n")
	repo.write("notes.md", "A reference manual\n")
	repo.write("private.py", "def private(): return True\n")
	base := repo.commit("implement dispatcher")
	const before = "2024-01-17T10:00:00Z"
	const after = "2024-04-17T10:00:00Z"
	cutoff := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	base.Now = cutoff
	base.AllowPath = func(p string) bool { return p != "private.py" }
	one := Discussion{ID: "mr:12/note:1", Body: "Deliver the event before the worker closes.\r\nPreserve its displayed name.", Path: path,
		UpdatedAt: before, Revision: base.Commit, URL: "https://example.org/p/merge_requests/12#note_1"}
	two := Discussion{ID: "mr:12/note:2", Body: "Deliver the event only after the worker closes.", Path: path, UpdatedAt: before}
	notes := []Discussion{one, two, one,
		{ID: "future", Body: "Later note", Path: path, UpdatedAt: after},
		{ID: "undated", Body: "Undated", Path: path}, {ID: "unscoped", Body: "No file named", UpdatedAt: before},
		{ID: "manual", Body: "Manual only", Path: "notes.md", UpdatedAt: before},
		{ID: "private", Body: "Private file", Path: "private.py", UpdatedAt: before},
		{ID: "unknown", Body: "Missing file", Path: "missing.py", UpdatedAt: before},
		{ID: "pasted", Body: "api_key = 'actualCredential123'", Path: path, UpdatedAt: before},
	}
	result := discoverWithoutRules(t, base, DiscoverOptions{Paths: []string{path}, Discussions: notes, AsOf: cutoff})
	if len(result.DiscussionSources) != 2 {
		t.Fatalf("expected just two independently scoped discussions, got %+v", result.DiscussionSources)
	}
	db := testStore(t)
	for range 2 {
		if err := db.Save(ctx, result.DiscussionSources); err != nil {
			t.Fatal(err)
		}
	}
	facts := storedFacts(t, db)
	if len(facts) != 2 {
		t.Fatalf("duplicate imports persisted %d facts", len(facts))
	}
	sourceBodies := map[string]string{}
	for _, c := range checkedFacts(t, base, facts) {
		f, ok := c.ReviewFact()
		if !ok || c.Status != StatusDiscussionSource || f.Authority != AuthorityHistorical || f.Claim != "" || f.IsDecision() || f.CurrentSchema() || len(f.Citations) != 1 || f.Citations[0].live() {
			t.Fatalf("original discussion gained rule or code authority: %+v", c)
		}
		visible := f.ReviewDiscussions()
		if len(visible) != 1 || visible[0].ObservedAt != before || visible[0].Path != path || len(f.DiscussionWitnesses()) != 1 {
			t.Fatalf("missing separate dated discussion and current file: %+v", f)
		}
		sourceBodies[visible[0].Reference] = f.DiscussionBody
		if visible[0].Reference == one.ID && (visible[0].Revision != base.Commit || visible[0].URL != one.URL || visible[0].Quote != strings.ReplaceAll(one.Body, "\r\n", "\n")) {
			t.Fatalf("original scoped words or permalink lost: %+v", visible[0])
		}
	}
	if sourceBodies[one.ID] != one.Body || sourceBodies[two.ID] != two.Body {
		t.Fatalf("SQLite changed the supplied text: %v", sourceBodies)
	}
	view := func(head Tree) Snapshot {
		head.Now = cutoff
		checked, err := head.CheckReview(ctx, base.Commit, facts)
		if err != nil {
			t.Fatal(err)
		}
		snapshot := ReviewSnapshot(checked)
		if err := snapshot.DescribeReviewChanges(ctx, base, head, checked); err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	repo.write(path, "def dispatch(event):\n    return event.label\n")
	review := view(repo.commit("change dispatch output"))
	inside := review.ForPaths([]string{path})
	for _, part := range []string{one.ID, two.ID, one.URL, before, "scope_witnesses", "return event.label"} {
		if !strings.Contains(inside, part) {
			t.Fatalf("review lost %q: %s", part, inside)
		}
	}
	if strings.Contains(inside, "return event.name") || strings.Contains(inside, "discussion_body") || review.ForPaths([]string{"jobs/worker.py"}) != "" {
		t.Fatal("discussion escaped its exact file, old implementation, or display budget")
	}
	repo.write(path, "def unrelated(event):\n    return event.name\n")
	if got := view(repo.commit("replace dispatcher")).ForPaths([]string{path}); got != "" {
		t.Fatalf("an unrelated function sharing a return expression inherited an old discussion: %s", got)
	}
	onlyScope := DiscoverOptions{Discussions: []Discussion{notes[5]}, Scope: "jobs", AsOf: cutoff}
	if got := discoverWithoutRules(t, base, onlyScope).DiscussionSources; len(got) != 0 {
		t.Fatalf("pathless comment acquired a directory scope: %+v", got)
	}
	onlyScope.Scope = path
	if got := discoverWithoutRules(t, base, onlyScope).DiscussionSources; len(got) != 1 || got[0].Scope != path || got[0].Citations[0].Path != "" {
		t.Fatalf("explicit single-file binding changed the original citation metadata: %+v", got)
	}
}

func TestDiscussionImportIsIndependentOfModelSamplingAndOriginalTree(t *testing.T) {
	repo := newDiscoveryRepo(t)
	repo.write("jobs.go", "package jobs\nfunc Previous() int { return 0 }\n")
	old := repo.commit("former job")
	repo.write("jobs.go", "package jobs\nfunc Register() int { return 1 }\n")
	repo.write("options.yaml", "enable_tasks: true\n")
	base := repo.commit("replace job")
	repo.write("jobs.go", "package jobs\nfunc Register() int { return 2 }\n")
	later := repo.commit("proposal")
	const date = "2024-01-01T00:00:00Z"
	var discussions []Discussion
	for i := range discoveryComments + 2 {
		discussions = append(discussions, Discussion{ID: fmt.Sprintf("note-%02d", i), Body: fmt.Sprintf("The runner keeps slot %d.", i), Path: "jobs.go", UpdatedAt: date})
	}
	discussions = append([]Discussion{
		{ID: "nonancestor", Body: "Only on proposed revision", Path: "jobs.go", Revision: later.Commit, UpdatedAt: date},
		{ID: "reused-path", Body: "The removed job contract", Path: "jobs.go", Revision: old.Commit, UpdatedAt: date},
		{ID: "absent-path", Body: "Did not exist in original tree", Path: "options.yaml", Revision: old.Commit, UpdatedAt: date},
	}, discussions...)
	result, err := base.Discover(t.Context(), DiscoverOptions{Discussions: discussions}, func(_ context.Context, _, corpus string) (string, error) {
		for _, retired := range []string{"removed job contract", "Did not exist in original tree", "Only on proposed revision"} {
			if strings.Contains(corpus, retired) {
				t.Fatalf("extractor could reintroduce an inadmissible scope: %s", retired)
			}
		}
		return `{"records":[]}`, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceKinds["discussion"] != discoveryComments || len(result.DiscussionSources) != discoveryComments+2 || !result.Truncated {
		t.Fatalf("lost supplied input to model sampling, or rebound old/future path: %d sampled, %d retained (%v)", result.SourceKinds["discussion"], len(result.DiscussionSources), result.Limits)
	}
	for _, f := range result.DiscussionSources {
		if !strings.HasPrefix(f.discussionRef(), "note-") {
			t.Fatalf("accepted an unverified original file: %+v", f)
		}
	}
	valid := Discussion{ID: "known", Body: strings.Repeat("long original words ", 350), Path: "jobs.go", Revision: base.Commit, UpdatedAt: date}
	got := discoverWithoutRules(t, later, DiscoverOptions{Discussions: []Discussion{valid}}).DiscussionSources
	if len(got) != 1 || got[0].DiscussionBody != valid.Body || len(got[0].Citations[0].Quote) > maxCitationQuote ||
		!strings.HasPrefix(valid.Body, got[0].Citations[0].Quote) || got[0].Citations[0].Revision != base.Commit {
		t.Fatalf("changed function with the same entry, original revision or complete long input lost: %+v", got)
	}
	projected := ReviewSnapshot(checkedFacts(t, later, got)).SelectForChange([]string{"jobs.go"}, nil)
	if len(projected) != 1 || len(projected[0].ReviewDiscussions()) != 1 || strings.Contains(projected.ForPaths([]string{"jobs.go"}), valid.Body) {
		t.Fatal("failed to expose a bounded exact excerpt independently of the stored full body")
	}
}

func TestDiscussionEditLifecycleAndDerivedFeedback(t *testing.T) {
	ctx := t.Context()
	repo := newLifecycleRepo(t)
	repo.write("orders.py", "def finalize(order):\n    return order.confirm()\n")
	tree := repo.commit("confirm order")
	baseTime := time.Now().UTC().Add(-5 * time.Hour)
	first := Discussion{ID: "thread-note", Body: "Orders close upon confirmation.", Path: "orders.py", UpdatedAt: baseTime.Format(time.RFC3339Nano)}
	next := first
	next.Body, next.UpdatedAt = "Amendment: orders close upon dispatch.", baseTime.Add(time.Hour).Format(time.RFC3339Nano)
	next.ExpiresAt = baseTime.Add(3 * time.Hour).Format(time.RFC3339Nano)
	second := Discussion{ID: "other-thread", Body: "Keep a separate fulfillment record.", Path: first.Path, UpdatedAt: first.UpdatedAt}
	opts := DiscoverOptions{Discussions: []Discussion{next, first, second}}
	raw := discoverWithoutRules(t, tree, opts).DiscussionSources
	if len(raw) != 3 {
		t.Fatalf("expected both editions plus an independent source, got %d", len(raw))
	}
	db := testStore(t)
	var firstFact, nextFact Fact
	for _, f := range raw {
		if f.DiscussionBody == first.Body {
			firstFact = f
		}
		if f.DiscussionBody == next.Body {
			nextFact = f
		}
	}
	rule := func(note Fact, claim string) Fact {
		return prepareFact(t, tree, Fact{Category: CategoryBusinessRule, Authority: AuthorityInferred, Claim: claim, Scope: first.Path, Source: first.Path,
			Citations: []Citation{{Kind: "code", Path: first.Path, StartLine: 2, EndLine: 2}, note.Citations[0]}})
	}
	rules := []Fact{rule(firstFact, first.Body), rule(nextFact, next.Body)}
	if err := db.Save(ctx, raw); err != nil {
		t.Fatal(err)
	}
	if err := db.Save(ctx, rules); err != nil {
		t.Fatal(err)
	}
	if facts, err := db.List(ctx, "", "Amendment"); err != nil || !slices.ContainsFunc(facts, func(f Fact) bool { return f.discussionRef() == next.ID && f.DiscussionBody == next.Body }) {
		t.Fatalf("amended source not searchable: %+v %v", facts, err)
	}
	evaluate := func(at time.Time) map[string]Checked {
		tree.Now = at
		items := checkedFacts(t, tree, storedFacts(t, db))
		byID := map[string]Checked{}
		for _, c := range items {
			byID[c.ID] = c
		}
		return byID
	}
	sourceID := discussionSourceID(first.ID)
	atFirst := evaluate(baseTime.Add(30 * time.Minute))
	if atFirst[sourceID].DiscussionBody != first.Body || atFirst[rules[0].ID].Status != StatusCandidate {
		t.Fatalf("future edit leaked into prior review: %+v", atFirst)
	}
	atEdit := evaluate(baseTime.Add(2 * time.Hour))
	if atEdit[sourceID].DiscussionBody != next.Body || atEdit[rules[0].ID].Status != StatusSuperseded || atEdit[rules[1].ID].Status != StatusCandidate {
		t.Fatalf("latest eligible source did not invalidate an old derivation: %+v", atEdit)
	}
	if _, err := db.Feedback(ctx, tree, rules[0].ID, Feedback{Action: FeedbackConfirm, Reason: "confirm outdated premise"}); err == nil {
		t.Fatal("confirmation bypassed the original discussion's known edit")
	}
	atExpiry := evaluate(baseTime.Add(4 * time.Hour))
	if atExpiry[sourceID].Status != StatusExpired || atExpiry[rules[1].ID].Status != StatusSuperseded || atExpiry[discussionSourceID(second.ID)].Status != StatusDiscussionSource {
		t.Fatalf("expiry affected the wrong records: %+v", atExpiry)
	}
	if _, err := db.Feedback(ctx, tree, rules[0].ID, Feedback{Action: FeedbackReplace, Reason: "replacement has expired", ReplacementID: rules[1].ID}); err == nil {
		t.Fatal("replacement used an expired original source")
	}
	if err := db.Revoke(ctx, sourceID); err != nil {
		t.Fatal(err)
	}
	at := baseTime.Add(30 * time.Minute)
	historical, err := db.ListAt(ctx, "", "", at)
	if err != nil {
		t.Fatal(err)
	}
	tree.Now = at
	view := ReviewSnapshot(checkedFacts(t, tree, historical)).ForPaths([]string{first.Path})
	if !strings.Contains(view, first.Body) || strings.Contains(view, next.Body) || !strings.Contains(view, rules[0].ID) {
		t.Fatalf("a later withdrawal hid the original source or its then-current rule: %s", view)
	}
	if err := db.Save(ctx, raw); err != nil {
		t.Fatal(err)
	}
	if got := storedFacts(t, db); len(got) != 1 || got[0].discussionRef() != second.ID {
		t.Fatalf("resync revived a withdrawn original or its derived rule: %+v", got)
	}
	// Databases created before precise revoke timestamps used SQLite's UTC
	// calendar format. The last instant before the stored second is eligible.
	legacy := time.Date(2026, 1, 3, 9, 0, 0, 0, time.UTC)
	if _, err := db.db.ExecContext(ctx, "UPDATE facts SET revoked_at=? WHERE repo=? AND id=?", legacy.Format(time.DateTime), db.repo, sourceID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at      time.Time
		visible bool
	}{{legacy.Add(-time.Nanosecond), true}, {legacy, false}} {
		list, err := db.ListAt(ctx, "", "", tc.at)
		if err != nil || slices.ContainsFunc(list, func(f Fact) bool { return f.ID == rules[0].ID }) != tc.visible {
			t.Fatalf("legacy source withdrawal at %v: %v, %v", tc.at, list, err)
		}
	}
}

func TestIneligibleEditedDiscussionRetiresOnlyTheEarlierAcceptedSource(t *testing.T) {
	ctx := t.Context()
	repo := newLifecycleRepo(t)
	repo.write("orders.py", "def submit(order):\n    return order.price\n")
	repo.write("removed.py", "def previous(): return 1\n")
	repo.write("manual.md", "Instructions for shipping.\n")
	base := repo.commit("accept existing implementation")
	repo.git("rm", "removed.py")
	head := repo.commit("remove old integration")
	date := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	original := Discussion{ID: "review-17-note-2", Path: "orders.py", Body: "Price includes postage.", UpdatedAt: date.Format(time.RFC3339)}
	for _, path := range []string{"removed.py", "manual.md", ".env", "orders.py"} {
		t.Run(path, func(t *testing.T) {
			db := testStore(t)
			old := discoverWithoutRules(t, base, DiscoverOptions{AsOf: date, Discussions: []Discussion{original}}).DiscussionSources
			rules := []Fact{prepareFact(t, base, Fact{Category: CategoryBusinessRule, Authority: AuthorityInferred, Origin: "discovery",
				Claim: original.Body, Scope: original.Path, Source: original.Path, Citations: []Citation{
					{Kind: "code", Path: original.Path, StartLine: 2, EndLine: 2}, old[0].Citations[0]}})}
			if err := db.Save(ctx, append(old, rules...)); err != nil {
				t.Fatal(err)
			}
			edit := original
			edit.Path, edit.Body, edit.Revision, edit.UpdatedAt = path, "Reassigned to a different integration.", head.Commit, date.Add(time.Hour).Format(time.RFC3339)
			if path == original.Path {
				edit.Body = "api_key = 'neverStoreThisCredential'"
			}
			options := DiscoverOptions{AsOf: date.Add(2 * time.Hour), Discussions: []Discussion{original, edit}}
			checkInput := func(tree Tree, o DiscoverOptions, expected int) DiscoveryResult {
				t.Helper()
				result, err := tree.Discover(ctx, o, func(_ context.Context, _, corpus string) (string, error) {
					var rows []discoverySource
					if err := json.Unmarshal([]byte(corpus), &rows); err != nil {
						t.Fatal(err)
					}
					var comments int
					for _, row := range rows {
						if row.Kind == "discussion" {
							comments++
							if row.Text != original.Body {
								t.Fatalf("unavailable edition entered the extractor: %+v", row)
							}
						}
					}
					if comments != expected {
						t.Fatalf("sent %d outdated discussions to the extractor, expected %d", comments, expected)
					}
					return `{"records":[]}`, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				return result
			}
			if result := checkInput(base, options, 1); len(result.discussionNotices) != 0 {
				t.Fatal("an edit from a future Git revision retired current evidence")
			}
			beforeEdit := options
			beforeEdit.AsOf = date
			if result := checkInput(head, beforeEdit, 1); len(result.discussionNotices) != 0 {
				t.Fatal("an edit later than the cutoff retired current evidence")
			}
			result := checkInput(head, options, 0)
			if len(result.DiscussionSources) != 1 || len(result.discussionNotices) != 1 {
				t.Fatalf("lost original edition or retained the new body: %+v", result)
			}
			if err := db.SaveDiscovery(ctx, "updated", result.DiscussionObservations()); err != nil {
				t.Fatal(err)
			}
			facts := storedFacts(t, db)
			body, _ := json.Marshal(facts)
			if strings.Contains(string(body), edit.Body) || path != original.Path && strings.Contains(string(body), path) {
				t.Fatal("stored the unavailable new body or path")
			}
			ruleID, err := stableFactID(rules[0])
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Feedback(ctx, head, ruleID, Feedback{Action: FeedbackConfirm, Reason: "check the old quotation"}); err == nil {
				t.Fatal("could confirm an earlier inference after its source moved outside the evidence")
			}
			for _, tc := range []struct {
				accepted string
				at       time.Time
				active   bool
			}{
				{head.Commit, date, true}, {base.Commit, options.AsOf, true}, {head.Commit, options.AsOf, false},
			} {
				tree := head
				tree.Now = tc.at
				items, err := tree.CheckReview(ctx, tc.accepted, facts)
				if err != nil {
					t.Fatal(err)
				}
				var raw, derived bool
				for _, c := range items {
					raw = raw || c.Status == StatusDiscussionSource
					derived = derived || c.Category == CategoryBusinessRule && c.Status == StatusCandidate
				}
				if raw != tc.active || derived != tc.active {
					t.Fatalf("accepted %s, cutoff %v: raw=%t, derived=%t, expected active=%t", tc.accepted, tc.at, raw, derived, tc.active)
				}
			}
		})
	}
}

func TestDiscussionEditionTieWithUnavailableCopy(t *testing.T) {
	ctx := t.Context()
	repo := newLifecycleRepo(t)
	repo.write("queue.py", "def next_item(queue):\n    return queue.first()\n")
	base := repo.commit("take first item")
	stamp := time.Date(2025, 4, 1, 12, 0, 0, 0, time.UTC)
	valid := Discussion{ID: "note-41", Path: "queue.py", Body: "Take the first queued item.", UpdatedAt: stamp.Format(time.RFC3339)}
	raw := discoverWithoutRules(t, base, DiscoverOptions{Discussions: []Discussion{valid}, AsOf: stamp}).DiscussionSources
	rule := prepareFact(t, base, Fact{Category: CategoryBusinessRule, Authority: AuthorityInferred, Origin: "discovery", Scope: valid.Path,
		Source: valid.Path, Claim: valid.Body, Citations: []Citation{{Kind: "code", Path: valid.Path, StartLine: 2, EndLine: 2}, raw[0].Citations[0]}})
	db := testStore(t)
	if err := db.Save(ctx, append(raw, rule)); err != nil {
		t.Fatal(err)
	}
	repo.write("docs.md", "Separate integration.\n")
	head := repo.commit("import the conflicting discussion exports")
	moved := valid
	moved.Path, moved.Body, moved.Revision = "docs.md", "Handled by another integration.", head.Commit
	moved.UpdatedAt = stamp.In(time.FixedZone("export", -4*60*60)).Format(time.RFC3339)
	imported := discoverWithoutRules(t, head, DiscoverOptions{Scope: valid.Path, AsOf: stamp, Discussions: []Discussion{moved, valid}})
	if imported.SourceKinds["discussion"] != 0 || len(imported.DiscussionSources) != 1 || len(imported.discussionNotices) != 1 {
		t.Fatalf("expected an audited tie excluded from extraction: %+v", imported)
	}
	if err := db.Save(ctx, imported.DiscussionObservations()); err != nil {
		t.Fatal(err)
	}
	facts := storedFacts(t, db)
	base.Now, head.Now = stamp, stamp
	if checked := checkedFacts(t, base, facts); len(ReviewSnapshot(checked)) == 0 {
		t.Fatal("a notice observed only on a later Git revision affected the original base")
	}
	for range 2 {
		if got := ReviewSnapshot(checkedFacts(t, head, facts)).ForPaths([]string{valid.Path}); got != "" {
			t.Errorf("an eligible copy or its inferred rule won a tied withdrawal: %s", got)
		}
		slices.Reverse(facts)
	}
	later := valid
	head.Now = stamp.Add(time.Hour)
	later.Body, later.UpdatedAt = "Take the first item after approval.", head.Now.Format(time.RFC3339)
	if err := db.Save(ctx, discoverWithoutRules(t, head, DiscoverOptions{AsOf: head.Now, Discussions: []Discussion{later}}).DiscussionSources); err != nil {
		t.Fatal(err)
	}
	got := ReviewSnapshot(checkedFacts(t, head, storedFacts(t, db))).ForPaths([]string{valid.Path})
	if !strings.Contains(got, later.Body) || strings.Contains(got, valid.Body) {
		t.Fatalf("a strictly later admissible edition did not resolve the tie: %s", got)
	}
}

func TestDiscussionExpiryForUnscopedDerivedRules(t *testing.T) {
	ctx := t.Context()
	repo := newDiscoveryRepo(t)
	repo.write("queue/read.py", "def next_item(queue):\n    return queue.first()\n")
	base := repo.commit("return first queued item")
	repo.write("other.txt", "unaccepted change\n")
	proposal := repo.commit("propose a different queue policy")
	now := time.Date(2025, 4, 1, 12, 0, 0, 0, time.UTC)
	stamp := func(hours int) string { return now.Add(time.Duration(hours) * time.Hour).Format(time.RFC3339) }
	old := Discussion{ID: "note-2", Body: "Old queue agreement.", UpdatedAt: stamp(-3)}
	recent := old
	recent.Body, recent.UpdatedAt, recent.ExpiresAt = "Keep queue order for this release.", stamp(-2), "2025-04-01T09:00:00-04:00"
	longer := Discussion{ID: "note-3", Body: "Read only the first item.", UpdatedAt: stamp(-2), ExpiresAt: stamp(2)}
	unused := Discussion{ID: "note-4", Body: "Temporary archive window.", UpdatedAt: stamp(-1), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339)}
	future, foreign := recent, recent
	future.Body, future.UpdatedAt, future.ExpiresAt = "A future agreement.", stamp(3), stamp(4)
	foreign.Body, foreign.UpdatedAt, foreign.Revision, foreign.ExpiresAt = "An unaccepted agreement.", stamp(-1), proposal.Commit, stamp(0)
	opts := DiscoverOptions{Scope: "queue", AsOf: now, Discussions: []Discussion{old, recent, longer, unused, future, foreign}}
	var rows []discoverySource
	read := func(body string) {
		t.Helper()
		if err := json.Unmarshal([]byte(body), &rows); err != nil {
			t.Fatal(err)
		}
	}
	result, err := base.Discover(ctx, opts, func(_ context.Context, _, corpus string) (string, error) {
		read(corpus)
		var code discoveryQuote
		var notes []discoveryQuote
		for _, row := range rows {
			if row.Kind == "code" && row.Path == "queue/read.py" {
				code = discoveryQuote{Source: row.ID, Quote: "return queue.first()"}
			}
			if row.Kind == "discussion" && row.Reference != unused.ID {
				if row.Text != recent.Body && row.Text != longer.Body {
					t.Fatalf("wrong edition in historical input: %+v", row)
				}
				notes = append(notes, discoveryQuote{Source: row.ID, Quote: row.Text})
			}
		}
		if len(notes) != 2 || code.Source == "" {
			t.Fatalf("missing original discussions or running code: %+v", rows)
		}
		b, err := json.Marshal(map[string]any{"records": []discoveryRule{
			{Category: CategoryBusinessRule, Claim: "Read in the agreed order for this release.", Scope: "queue/read.py", Citations: append(notes, code)},
			{Category: CategoryBusinessRule, Claim: "Only the first item is returned.", Scope: "queue/read.py", Citations: []discoveryQuote{code}},
		}})
		return string(b), err
	})
	if err != nil || len(result.Facts) != 2 || len(result.DiscussionSources) != 0 {
		t.Fatalf("unscoped import: %+v %v", result, err)
	}
	until, _ := time.Parse(time.RFC3339, result.Facts[0].ExpiresAt)
	if !until.Equal(now.Add(time.Hour)) || result.Facts[1].ExpiresAt != "" {
		t.Errorf("expected only the cited earliest expiry, got %q and %q", result.Facts[0].ExpiresAt, result.Facts[1].ExpiresAt)
	}
	db := testStore(t)
	legacy := slices.Clone(result.Facts)
	legacy[0].ExpiresAt = ""
	if err := db.Save(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	amended := recent
	amended.UpdatedAt, amended.ExpiresAt = stamp(2), stamp(3)
	for _, tc := range []struct {
		tree Tree
		note Discussion
	}{{base, amended}, {proposal, recent}} {
		notes := discoverWithoutRules(t, tc.tree, DiscoverOptions{AsOf: now.Add(3 * time.Hour), Discussions: []Discussion{tc.note}})
		if err := db.Save(ctx, notes.DiscussionObservations()); err != nil {
			t.Fatal(err)
		}
	}
	proposal.Now = now.Add(3 * time.Hour)
	migrated, err := proposal.CheckReview(ctx, base.Commit, storedFacts(t, db))
	if err != nil || len(migrated) != 2 {
		t.Fatalf("legacy evidence: %+v %v", migrated, err)
	}
	for _, item := range migrated {
		if item.ExpiresAt != "" || item.Status != StatusCandidate {
			t.Fatalf("a different edition or proposal-side import retired the accepted rule: %+v", item)
		}
	}
	saved := append(result.Facts, result.DiscussionObservations()...)
	if err := db.Save(ctx, saved); err != nil {
		t.Fatal(err)
	}
	if saved[0].ID != legacy[0].ID {
		t.Fatal("learning the original edition's deadline changed the old rule's ID")
	}
	for _, tc := range []struct {
		at     time.Time
		status string
	}{
		{now.Add(59 * time.Minute), StatusCandidate}, {now.Add(time.Hour), StatusExpired},
	} {
		base.Now = tc.at
		for _, c := range checkedFacts(t, base, storedFacts(t, db)) {
			want := StatusCandidate
			if c.ID == legacy[0].ID {
				want = tc.status
			}
			if c.Status != want {
				t.Errorf("claim %q at %v: got %s, want %s", c.Claim, tc.at, c.Status, want)
			}
		}
	}
	opts.Scope, opts.AsOf = "", now.Add(time.Hour)
	_, err = base.Discover(ctx, opts, func(_ context.Context, _, corpus string) (string, error) {
		read(corpus)
		var notes []string
		for _, row := range rows {
			if row.Kind == "discussion" {
				notes = append(notes, row.Text)
			}
		}
		if !slices.Equal(notes, []string{longer.Body}) {
			t.Errorf("expired, future or superseded source leaked into new inference: %q", notes)
		}
		return `{"records":[]}`, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiscussionConflictingEditionsAtSamplingBoundary(t *testing.T) {
	repo := newLifecycleRepo(t)
	repo.write("rules.go", "package rules\nfunc Apply(n int) int { return n }\n")
	base := repo.commit("accept rule")
	repo.write("rules.go", "package rules\nfunc Apply(n int) int { return n+1 }\n")
	side := repo.commit("future proposal")
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	note := func(id, body string, hours int) Discussion {
		return Discussion{ID: id, Body: body, Path: "rules.go", UpdatedAt: start.Add(time.Duration(hours) * time.Hour).Format(time.RFC3339)}
	}
	var notes []Discussion
	for i := range discoveryComments - 1 {
		notes = append(notes, note(fmt.Sprintf("active-%d", i), "An independently dated observation.", 3))
	}
	notes = append(notes, note("ambiguous", "Free capacity is required.", 2), note("ambiguous", "Reserved capacity is required.", 2))
	wrongBranch := note("fallback", "Only the proposed contract.", 4)
	wrongBranch.Revision = side.Commit
	prior := note("fallback", "The accepted revision stays in scope.", 1)
	prior.Revision = base.Commit
	notes = append(notes, wrongBranch, prior)
	result, err := base.Discover(t.Context(), DiscoverOptions{Discussions: notes, AsOf: start.Add(5 * time.Hour)}, func(_ context.Context, _, corpus string) (string, error) {
		var rows []discoverySource
		if err := json.Unmarshal([]byte(corpus), &rows); err != nil {
			t.Fatal(err)
		}
		included := false
		for _, row := range rows {
			if row.Kind != "discussion" {
				continue
			}
			if row.Reference == "ambiguous" || row.Revision == side.Commit {
				t.Fatalf("model received an arbitrary edition: %+v", row)
			}
			included = included || row.Reference == prior.ID && row.Text == prior.Body
		}
		if !included {
			t.Fatal("a later edition on another branch displaced the accepted one")
		}
		return `{"records":[]}`, nil
	})
	if err != nil || len(result.DiscussionSources) != discoveryComments+2 {
		t.Fatalf("expected both tied editions in the audit store: %d, %v", len(result.DiscussionSources), err)
	}
	checked := checkedFacts(t, base, result.DiscussionSources)
	if len(checked) != discoveryComments || slices.ContainsFunc(checked, func(c Checked) bool { return c.discussionRef() == "ambiguous" }) {
		t.Fatalf("retrieval picked a winner for equal-time inconsistent exports: %+v", checked)
	}
}

func TestDiscussionSelectionCoversEveryQuoteAndScopeWitness(t *testing.T) {
	repo := newLifecycleRepo(t)
	const path = "dispatch.py"
	repo.write(path, "def route(message):\n    return message.queue\n")
	tree := repo.commit("route messages")
	first := Discussion{ID: "policy-note", Path: path, UpdatedAt: "2025-01-01T00:00:00Z", Body: "The team records all message routes."}
	var clauses []string
	for i := range 60 {
		clauses = append(clauses, fmt.Sprintf("For route %d, record the sender unless the queue is offline.", i+1))
	}
	for _, tc := range []struct {
		name        string
		body        string
		fullWitness bool
		expected    int
	}{
		{"complete coverage", first.Body, true, 1},
		{"missing entry point", first.Body, false, 2},
		{"missing later quotation", strings.Join(clauses, "\n"), true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			note := first
			note.Body = tc.body
			sources := discoverWithoutRules(t, tree, DiscoverOptions{Discussions: []Discussion{note}}).DiscussionSources
			if len(sources) != 1 {
				t.Fatalf("missing independent source: %+v", sources)
			}
			raw := sources[0]
			if tc.name == "missing later quotation" && len(raw.Citations) != 2 {
				t.Fatalf("test did not exercise distinct source ranges: %+v", raw.Citations)
			}
			firstLine := 2
			if tc.fullWitness {
				firstLine = 1
			}
			claim := prepareFact(t, tree, Fact{Category: CategoryBusinessRule, Authority: AuthorityInferred, Claim: "Route messages using their queue.", Source: path, Scope: path,
				Citations: []Citation{{Kind: "code", Path: path, StartLine: firstLine, EndLine: 2}, raw.Citations[0]}})
			selected := ReviewSnapshot(checkedFacts(t, tree, []Fact{raw, claim})).SelectForChange([]string{path}, nil)
			if len(selected) != tc.expected {
				t.Fatalf("wrong source pruning, got %d rows for %d quotes, want %d: %s", len(selected), len(raw.Citations), tc.expected, selected.ForPaths([]string{path}))
			}
		})
	}
}

func TestDiscussionSourceReplacementAndRejection(t *testing.T) {
	ctx := t.Context()
	repo := newLifecycleRepo(t)
	repo.write("mail.py", "def send(message):\n    return message.address\n")
	repo.write("receipt.py", "def receipt(message):\n    return message.id\n")
	tree := repo.commit("deliver mail")
	original := Discussion{ID: "decision-one", Path: "mail.py", Body: "The delivery queue owns the address.", UpdatedAt: "2025-01-01T00:00:00Z"}
	replacement := original
	replacement.ID, replacement.Body = "decision-two", "The message owns the address."
	otherFile := original
	otherFile.ID, otherFile.Path = "receipt-note", "receipt.py"
	raw := discoverWithoutRules(t, tree, DiscoverOptions{Discussions: []Discussion{original, replacement, otherFile}}).DiscussionSources
	db := testStore(t)
	if err := db.Save(ctx, raw); err != nil {
		t.Fatal(err)
	}
	f := prepareFact(t, tree, Fact{Category: CategoryBusinessRule, Authority: AuthorityInferred, Claim: original.Body, Source: "mail.py", Scope: "mail.py",
		Citations: []Citation{{Kind: "code", Path: "mail.py", StartLine: 2, EndLine: 2}, raw[0].Citations[0]}})
	derived := []Fact{f}
	if err := db.Save(ctx, derived); err != nil {
		t.Fatal(err)
	}
	origID, replacementID := discussionSourceID(original.ID), discussionSourceID(replacement.ID)
	for _, feedback := range []Feedback{
		{Action: FeedbackReplace, ReplacementID: replacementID},
		{Action: FeedbackReplace, Reason: "a different file", ReplacementID: discussionSourceID(otherFile.ID)},
	} {
		if _, err := db.Feedback(ctx, tree, origID, feedback); err == nil {
			t.Fatalf("accepted an unreasoned or cross-file replacement: %+v", feedback)
		}
	}
	if _, err := db.Feedback(ctx, tree, origID, Feedback{Action: FeedbackConfirm, Reason: "the quote is present"}); err == nil {
		t.Fatal("an original discussion alone became a confirmed claim")
	}
	if _, err := db.Feedback(ctx, tree, origID, Feedback{Action: FeedbackReplace, Reason: "corrected by later decision", ReplacementID: replacementID}); err != nil {
		t.Fatal(err)
	}
	statuses := map[string]Checked{}
	for _, c := range checkedFacts(t, tree, storedFacts(t, db)) {
		statuses[c.ID] = c
	}
	if statuses[origID].Status != StatusSuperseded || statuses[derived[0].ID].Status != StatusSuperseded || statuses[replacementID].Status != StatusDiscussionSource || statuses[replacementID].Authority != AuthorityHistorical {
		t.Fatalf("replacement granted authority or left old sources active: %+v", statuses)
	}
	if _, err := db.Feedback(ctx, tree, derived[0].ID, Feedback{Action: FeedbackConfirm, Reason: "old quotation is still recorded"}); err == nil {
		t.Fatal("a replaced source could confirm its dependent claim")
	}
	if _, err := db.Feedback(ctx, tree, replacementID, Feedback{Action: FeedbackReject, Reason: "this proposal was declined"}); err != nil {
		t.Fatal(err)
	}
	if got := ReviewSnapshot(checkedFacts(t, tree, storedFacts(t, db))).ForPaths([]string{"mail.py"}); got != "" {
		t.Fatalf("rejected or replaced discussions were shown as active: %s", got)
	}
}

func TestUneditedDiscussionCannotRebindToReplacementDuringResync(t *testing.T) {
	ctx := t.Context()
	repo := newLifecycleRepo(t)
	repo.write("engine.py", "def route(message):\n    return None\n")
	repo.write("options.yaml", "run_mode: scheduled\n")
	base := repo.commit("accept routing")
	note := Discussion{ID: "design/113", Body: "The dispatcher runs on the scheduled route.", Path: "engine.py", UpdatedAt: "2024-02-01T00:00:00Z"}
	opts := DiscoverOptions{Discussions: []Discussion{note}}
	db := testStore(t)
	record := func(tree Tree) Fact {
		t.Helper()
		items := discoverWithoutRules(t, tree, opts).DiscussionSources
		if len(items) != 1 {
			t.Fatalf("expected one supplied note at import, got %+v", items)
		}
		if err := db.Save(ctx, items); err != nil {
			t.Fatal(err)
		}
		return items[0]
	}
	original := record(base)
	claim := prepareFact(t, base, Fact{Category: CategoryBusinessRule, Authority: AuthorityInferred, Claim: "Dispatch uses the scheduled route.", Scope: "engine.py", Source: "options.yaml",
		Citations: []Citation{{Kind: "config", Path: "options.yaml", StartLine: 1, EndLine: 1}, original.Citations[0]}})
	if err := db.Save(ctx, []Fact{claim}); err != nil {
		t.Fatal(err)
	}
	repo.write("engine.py", "def invoice(amount):\n    return None\n")
	replaced := repo.commit("reuse filename in a different subsystem")
	record(replaced)
	if got := ReviewSnapshot(checkedFacts(t, replaced, storedFacts(t, db))).ForPaths([]string{"engine.py"}); got != "" {
		t.Fatalf("resync revived an unchanged external note or its derived claim after the named entry disappeared: %s", got)
	}
	repo.write("engine.py", "def route(message):\n    return message.ready\n")
	restored := repo.commit("restore the routing entry")
	if got := ReviewSnapshot(checkedFacts(t, restored, storedFacts(t, db))).ForPaths([]string{"engine.py"}); !strings.Contains(got, note.Body) || !strings.Contains(got, "return message.ready") {
		t.Fatalf("independently identifiable current entry is missing: %s", got)
	}
}

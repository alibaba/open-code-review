// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestHistoricalSchemaInheritsParentWithdrawalIntervals(t *testing.T) {
	ctx := t.Context()
	repo := newLifecycleRepo(t)
	repo.write("ship.go", "package ship\n")
	var trees []Tree
	for _, value := range []int{10, 20, 25, 30} {
		repo.write("policy.json", fmt.Sprintf(`{"properties":{"window":{"const":%d}}}`, value))
		trees = append(trees, repo.commit(fmt.Sprintf("use dispatch window %d", value)))
	}
	for _, during := range []bool{false, true} {
		name := "child first derived after relearn"
		if during {
			name = "child first derived during withdrawal"
		}
		t.Run(name, func(t *testing.T) {
			db := testStore(t)
			learn := func(tree Tree) Fact {
				t.Helper()
				facts, err := tree.Learn(ctx, "policy.json", "ship.go", "window")
				if err != nil || len(facts) != 1 {
					t.Fatalf("learn: %+v, %v", facts, err)
				}
				if err := db.Save(ctx, facts); err != nil {
					t.Fatal(err)
				}
				return facts[0]
			}
			withdraw := func(f Fact, at time.Time) {
				t.Helper()
				if err := db.Revoke(ctx, f.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := db.db.ExecContext(ctx, "UPDATE facts SET revoked_at=? WHERE id=?", at.Format(time.RFC3339Nano), f.ID); err != nil {
					t.Fatal(err)
				}
			}
			original, independent := learn(trees[0]), learn(trees[2])
			before := time.Date(2025, 4, 1, 10, 0, 0, 0, time.UTC)
			// Both original grants existed before the simulated withdrawal events.
			if _, err := db.db.ExecContext(ctx, "UPDATE facts SET recorded_at=? WHERE id IN (?,?)", before.Add(-time.Hour).Format(time.RFC3339Nano), original.ID, independent.ID); err != nil {
				t.Fatal(err)
			}
			oneParentOff, allOff, recovered, withdrawnAgain := before.Add(time.Hour), before.Add(2*time.Hour), before.Add(3*time.Hour), before.Add(4*time.Hour)
			refresh := func(tree Tree, parents int) Fact {
				t.Helper()
				tree.Now = before
				previous, err := db.ListAt(ctx, "", "", before)
				if err != nil {
					t.Fatal(err)
				}
				rules, err := tree.RefreshSchemas(ctx, previous, "ship.go")
				if err != nil || len(rules) != 1 || len(rules[0].schemaParents) != parents {
					t.Fatalf("historical refresh expected %d accepted parents: %+v, %v", parents, rules, err)
				}
				if err := db.Save(ctx, rules); err != nil {
					t.Fatal(err)
				}
				return rules[0]
			}
			withdraw(original, oneParentOff)
			parent := refresh(trees[1], 1)
			withdraw(independent, allOff)
			var child Fact
			if during {
				child = refresh(trees[3], 3)
			}
			if got := learn(trees[1]); got.ID != parent.ID {
				t.Fatal("explicit learn changed the parent identity")
			}
			if _, err := db.db.ExecContext(ctx, "UPDATE schema_parents SET learned_at=? WHERE fact_row=(SELECT rowid FROM facts WHERE id=?)", recovered.Format(time.RFC3339Nano), parent.ID); err != nil {
				t.Fatal(err)
			}
			withdraw(parent, withdrawnAgain)
			if !during {
				child = refresh(trees[3], 3)
			}
			for _, tc := range []struct {
				name    string
				when    time.Time
				visible bool
			}{
				{"originally authorized", before, true}, {"independent parent still active", oneParentOff, true},
				{"all parents temporarily unavailable", allOff, false}, {"one parent explicitly relearned", recovered, true},
				{"last parent directly withdrawn", withdrawnAgain, false}, {"live", time.Time{}, false},
			} {
				facts, err := db.ListAt(ctx, "", "", tc.when)
				if err != nil {
					t.Fatal(err)
				}
				if got := slices.ContainsFunc(facts, func(f Fact) bool { return f.ID == child.ID }); got != tc.visible {
					t.Errorf("%s: child visible=%v, want %v", tc.name, got, tc.visible)
				}
			}
			withdraw(child, withdrawnAgain.Add(time.Hour))
			learn(trees[3])
			facts, err := db.List(ctx, "", "")
			if err != nil || slices.ContainsFunc(facts, func(f Fact) bool { return f.ID == child.ID }) {
				t.Errorf("directly withdrawn child became current: %+v, %v", facts, err)
			}
		})
	}
}

func TestSchemaWithdrawalMigrationRetainsClosedHistory(t *testing.T) {
	ctx := t.Context()
	repo := newLifecycleRepo(t)
	repo.write("ship.go", "package ship\n")
	repo.write("terms.json", `{"properties":{"days":{"const":7}}}`)
	tree := repo.commit("accept dispatch terms")
	dbfile := filepath.Join(t.TempDir(), "older.db")
	open := func(write bool) *Store {
		t.Helper()
		s, err := Open(ctx, dbfile, "repo", write)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	db := open(true)
	facts, err := tree.Learn(ctx, "terms.json", "ship.go", "days")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Save(ctx, facts); err != nil {
		t.Fatal(err)
	}
	begin := time.Date(2025, 4, 1, 10, 0, 0, 101, time.UTC)
	end := begin.Add(4 * time.Nanosecond)
	_, err = db.db.ExecContext(ctx, `DROP TABLE schema_parent_additions; DROP TABLE schema_parents;
 INSERT INTO schema_withdrawals SELECT rowid,?,? FROM facts WHERE id=?`, begin.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano), facts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, write := range []bool{false, true} {
		db = open(write)
		for _, tc := range []struct {
			at    time.Time
			count int
		}{{begin.Add(-time.Nanosecond), 1}, {begin, 0}, {end.Add(-time.Nanosecond), 0}, {end, 1}} {
			got, err := db.ListAt(ctx, "", "", tc.at)
			if err != nil || len(got) != tc.count {
				t.Errorf("write=%t at %s: %d records, %v", write, tc.at, len(got), err)
			}
		}
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLegacySchemaReplacementDoesNotBridgeWithdrawal(t *testing.T) {
	ctx := t.Context()
	filename := filepath.Join(t.TempDir(), "legacy.db")
	writer, err := Open(ctx, filename, "repo", true)
	if err != nil {
		t.Fatal(err)
	}
	repo := newLifecycleRepo(t)
	repo.write("worker.go", "package worker\nconst maxPending = 8\n")
	tree := repo.commit("bound pending work")
	root := Fact{Revision: tree.Commit, Source: "policy.json", Scope: "worker.go", Property: "limit", Keyword: "const", Value: []byte("1")}
	replacement := root
	replacement.Value = []byte("2")
	code := prepareFact(t, tree, Fact{Category: CategoryBusinessRule, Authority: AuthorityInferred, Source: root.Scope, Scope: root.Scope,
		Claim: "A worker has at most eight pending jobs.", Citations: []Citation{{Kind: "code", Path: root.Scope, StartLine: 2, EndLine: 2}}})
	facts := []Fact{root, replacement, code}
	if err := writer.Save(ctx, facts); err != nil {
		t.Fatal(err)
	}
	withdrawn := time.Date(2025, 4, 1, 10, 0, 0, 0, time.UTC)
	learned := withdrawn.Add(time.Hour)
	// Legacy stores wrote whole-second timestamps and had no mapping tables.
	_, err = writer.db.ExecContext(ctx, `DROP TABLE schema_parent_additions; DROP TABLE schema_parents; DROP TABLE schema_withdrawals;
 UPDATE facts SET recorded_at=?1;
 UPDATE facts SET recorded_at=?2, revoked_at=?3 WHERE id=?4`,
		learned.Format(time.DateTime), withdrawn.Add(-time.Hour).Format(time.DateTime), withdrawn.Format(time.DateTime), facts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, write := range []bool{false, true} {
		store, err := Open(ctx, filename, "repo", write)
		if err != nil {
			t.Fatal(err)
		}
		if store.schemaParents != write {
			t.Fatalf("write=%t: unexpected parent table availability", write)
		}
		for _, tc := range []struct {
			at                time.Time
			root, replacement bool
		}{
			{withdrawn.Add(-time.Nanosecond), true, true},
			{withdrawn, false, false},
			{learned.Add(time.Second - time.Nanosecond), false, false},
			{learned.Add(time.Second), false, true},
			{time.Time{}, false, true},
		} {
			rows, err := store.ListAt(ctx, "", "", tc.at)
			if err != nil {
				t.Fatal(err)
			}
			for i, want := range []bool{tc.root, tc.replacement, true} {
				if got := slices.ContainsFunc(rows, func(f Fact) bool { return f.ID == facts[i].ID }); got != want {
					t.Errorf("write=%t cutoff=%s %s %s: visible=%t, want %t", write, tc.at, facts[i].Source, facts[i].Value, got, want)
				}
			}
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExistingSchemaChildGainsAnotherParent(t *testing.T) {
	ctx := t.Context()
	repo := newLifecycleRepo(t)
	repo.write("consumer.go", "package consumer\n")
	var revisions []Tree
	for _, price := range []int{10, 20, 30} {
		repo.write("plan.json", fmt.Sprintf(`{"properties":{"price":{"const":%d}}}`, price))
		revisions = append(revisions, repo.commit(fmt.Sprintf("set price to %d", price)))
	}
	for _, tc := range []struct {
		name          string
		explicitChild bool
		startWithB    bool
	}{{"child initially derived from A", false, false}, {"child first learned explicitly", true, false}, {"child initially derived only from B", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			db := testStore(t)
			learn := func(tree Tree) Fact {
				t.Helper()
				facts, err := db.Learn(ctx, tree, "plan.json", "consumer.go", "price")
				if err != nil || len(facts) != 1 || facts[0].Status != StatusVerified {
					t.Fatalf("learn: %+v, %v", facts, err)
				}
				return facts[0].Fact
			}
			refresh := func() Fact {
				t.Helper()
				mappings, err := db.List(ctx, "", "")
				if err != nil {
					t.Fatal(err)
				}
				facts, err := revisions[2].RefreshSchemas(ctx, mappings, "consumer.go")
				if err != nil || len(facts) != 1 {
					t.Fatalf("refresh: %+v, %v", facts, err)
				}
				if err := db.Save(ctx, facts); err != nil {
					t.Fatal(err)
				}
				return facts[0]
			}
			first := learn(revisions[0])
			if tc.explicitChild {
				learn(revisions[2])
			}
			var child Fact
			if !tc.startWithB {
				child = refresh()
			}
			visible := func(cutoff time.Time, want bool) {
				t.Helper()
				facts, err := db.ListAt(ctx, "", "", cutoff)
				if err != nil {
					t.Fatal(err)
				}
				if got := slices.ContainsFunc(facts, func(f Fact) bool { return f.ID == child.ID }); got != want {
					t.Errorf("at %s child visible=%t, want %t", cutoff, got, want)
				}
			}
			initial := time.Now().UTC().Add(-time.Nanosecond)
			if err := db.Revoke(ctx, first.ID); err != nil {
				t.Fatal(err)
			}
			gap := time.Now().UTC()
			visible(gap, tc.explicitChild)
			second := learn(revisions[1])
			afterLearn := time.Now().UTC()
			got := refresh()
			if tc.startWithB {
				child = got
				if !slices.Equal(got.schemaParents, []string{second.ID}) {
					t.Fatalf("unexpected first parents: %v", got.schemaParents)
				}
			}
			if got.ID != child.ID || !slices.Contains(got.schemaParents, second.ID) {
				t.Fatalf("expected same child with second mapping: %+v", got)
			}
			if got := refresh(); got.ID != child.ID {
				t.Fatal("repeated refresh changed the child identity")
			}
			visible(initial, true)
			visible(gap, tc.explicitChild)
			visible(afterLearn, true)
			visible(time.Time{}, true)
			if err := db.Revoke(ctx, second.ID); err != nil {
				t.Fatal(err)
			}
			visible(time.Time{}, tc.explicitChild)
			visible(afterLearn, true)
			if !tc.explicitChild {
				// A later explicit learn supplies a new independent grant, but it
				// must not alter the gap that preceded the second parent.
				learn(revisions[2])
				visible(time.Time{}, true)
				visible(gap, false)
			}
			if err := db.Revoke(ctx, child.ID); err != nil {
				t.Fatal(err)
			}
			visible(time.Time{}, false)
		})
	}
}

func TestSchemaParentsAreReadableBeforeAdditionalParentMigration(t *testing.T) {
	ctx := t.Context()
	filename := filepath.Join(t.TempDir(), "previous.db")
	writer, err := Open(ctx, filename, "repo", true)
	if err != nil {
		t.Fatal(err)
	}
	parent := []Fact{{Revision: Digest([]byte("first")), Source: "terms.json", Scope: "billing.go", Property: "limit", Keyword: "const", Value: []byte("1")}}
	if err := writer.Save(ctx, parent); err != nil {
		t.Fatal(err)
	}
	child := []Fact{{Revision: Digest([]byte("next")), Source: "terms.json", Scope: "billing.go", Property: "limit", Keyword: "const", Value: []byte("2"), schemaParents: []string{parent[0].ID}}}
	if err := writer.Save(ctx, child); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.db.ExecContext(ctx, "DROP TABLE schema_parent_additions"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, writable := range []bool{false, true} {
		store, err := Open(ctx, filename, "repo", writable)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := store.List(ctx, "", "")
		if err != nil || len(rows) != 2 {
			t.Fatalf("writable=%t: %+v, %v", writable, rows, err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

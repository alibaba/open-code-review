// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"slices"
	"testing"
	"time"
)

func TestReviewReplyEdits(t *testing.T) {
	const path, outside = "pkg/queue.py", "archive/queue.py"
	const quote = "def ready(item):"
	repo := newDiscoveryRepo(t)
	for _, p := range []string{path, outside} {
		repo.write(p, quote+"\n    return item.enabled\n")
	}
	tree := repo.commit("add queues")
	published := time.Date(2025, 8, 6, 15, 0, 0, 0, time.UTC)
	cutoff := published.Add(4 * time.Hour)
	originalAt := published.Add(-2 * time.Hour).Format(time.RFC3339)
	parent := Discussion{ID: "first", ReviewID: "queue-review", Body: "Disabled items can enter the queue.",
		PublishedAt: originalAt, UpdatedAt: originalAt, PublishedRevision: tree.Commit,
		OriginalRevision: tree.Commit, StartLine: 1, EndLine: 1, SourceQuote: quote}
	reply := Discussion{ID: "reply", ReviewID: parent.ReviewID, ReplyTo: parent.ID, Body: "The caller excludes disabled items.",
		PublishedAt: published.Format(time.RFC3339), UpdatedAt: published.Add(-40 * time.Minute).Format(time.RFC3339), PublishedRevision: tree.Commit}
	setup := func(t *testing.T, p string) (func(string, ...Discussion), func(time.Time, ...string)) {
		t.Helper()
		ctx := t.Context()
		db := testStore(t)
		importNotes := func(scope string, notes ...Discussion) {
			t.Helper()
			result := discoverWithoutRules(t, tree, DiscoverOptions{Scope: scope, AsOf: cutoff, Discussions: notes})
			if err := db.Save(ctx, result.DiscussionObservations()); err != nil {
				t.Fatal(err)
			}
		}
		wantReplies := func(at time.Time, want ...string) {
			t.Helper()
			facts, err := db.ListAt(ctx, parent.ReviewID, "", at)
			if err != nil {
				t.Fatal(err)
			}
			current := tree
			current.Now = at
			checked, err := current.CheckReview(ctx, tree.Commit, facts)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, f := range ReviewSnapshot(checked) {
				if f.ReviewReply != nil {
					got = append(got, f.Reason)
				}
			}
			if !slices.Equal(got, want) {
				t.Fatalf("replies at %s = %q, want %q", at.Format(time.RFC3339), got, want)
			}
		}
		original, correction := parent, reply
		original.Path, correction.Path = p, p
		importNotes("", original, correction)
		wantReplies(cutoff, correction.Body)
		return importNotes, wantReplies
	}

	t.Run("scoped import retires a reply moved away", func(t *testing.T) {
		importNotes, wantReplies := setup(t, path)
		moved := reply
		moved.Path, moved.UpdatedAt = outside, published.Add(2*time.Hour).Format(time.RFC3339)
		importNotes("pkg", moved)
		wantReplies(published.Add(time.Hour), reply.Body)
		wantReplies(cutoff)
	})

	t.Run("scoped reimport preserves an unchanged reply outside scope", func(t *testing.T) {
		importNotes, wantReplies := setup(t, outside)
		original, correction := parent, reply
		original.Path, correction.Path = outside, outside
		importNotes("pkg", original, correction)
		wantReplies(cutoff, correction.Body)
	})

	t.Run("last draft edit is visible at publication", func(t *testing.T) {
		importNotes, wantReplies := setup(t, path)
		latest := reply
		latest.Path, latest.Body = path, "The caller also excludes archived items."
		latest.UpdatedAt = published.Add(-15 * time.Minute).Format(time.RFC3339)
		importNotes("", latest)
		wantReplies(published.Add(-time.Second))
		wantReplies(published, latest.Body)
	})

	t.Run("same edition stays visible until the added deadline", func(t *testing.T) {
		importNotes, wantReplies := setup(t, path)
		withDeadline := reply
		deadline := cutoff.Add(time.Hour)
		withDeadline.Path, withDeadline.ExpiresAt = path, deadline.Format(time.RFC3339)
		importNotes("", withDeadline)
		wantReplies(deadline.Add(-time.Second), reply.Body)
		wantReplies(deadline)
	})
}

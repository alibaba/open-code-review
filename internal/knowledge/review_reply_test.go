// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestReviewReplyAcrossPushesAndImports(t *testing.T) {
	ctx := t.Context()
	repo := newLifecycleRepo(t)
	repo.write("base.py", "DEFAULT = True\n")
	base := repo.commit("base without the proposed file")
	const path = "orders.py"
	const selection = "def pending(order):\n    return order.ready"
	repo.write(path, selection+"\n")
	original := repo.commit("propose queue")
	repo.write(path, "# callers pass ready orders\n"+selection+"\n")
	submitted := repo.commit("before publishing the reply")
	repo.write(path, "# callers pass ready orders\ndef pending(order):\n    # orders are filtered by the caller\n    return order.ready\n")
	current := repo.commit("insert a comment inside the reviewed range")
	root := Discussion{ID: "note-17", ReviewID: "change-4", Path: path, Body: "Pending orders can be lost.",
		OriginalRevision: original.Commit, StartLine: 1, EndLine: 2, SourceQuote: selection,
		PublishedRevision: original.Commit, PublishedAt: "2025-08-06T14:01:00Z", UpdatedAt: "2025-08-06T14:01:00Z"}
	reply := Discussion{ID: "note-21", ReplyTo: root.ID, ReviewID: root.ReviewID, Path: path, Body: "The caller queues only ready orders.",
		PublishedRevision: submitted.Commit, PublishedAt: "2025-08-06T15:00:00Z", UpdatedAt: "2025-08-06T14:20:00Z"}
	db := testStore(t)
	asOf := func(at string) time.Time {
		v, err := time.Parse(time.RFC3339, at)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	end := asOf("2025-08-07T00:00:00Z")
	save := func(notes ...Discussion) {
		t.Helper()
		result := discoverWithoutRules(t, base, DiscoverOptions{AsOf: end, Discussions: notes})
		if err := db.Save(ctx, result.DiscussionObservations()); err != nil {
			t.Fatal(err)
		}
	}
	read := func(tree Tree, id string, at time.Time) []Checked {
		t.Helper()
		facts, err := db.ListAt(ctx, id, "", at)
		if err != nil {
			t.Fatal(err)
		}
		tree.Now = at
		checked, err := tree.CheckReview(ctx, base.Commit, facts)
		if err != nil {
			t.Fatal(err)
		}
		return slices.DeleteFunc(checked, func(c Checked) bool { return c.ReviewReply == nil })
	}
	save(reply)
	if got := read(current, root.ReviewID, end); len(got) != 0 {
		t.Fatalf("orphaned reply entered review: %+v", got)
	}
	save(root)
	if got := read(current, "another-review", end); len(got) != 0 {
		t.Fatalf("reply crossed reviews: %+v", got)
	}
	if got := read(current, root.ReviewID, asOf("2025-08-06T14:59:00Z")); len(got) != 0 {
		t.Fatalf("unpublished draft entered review: %+v", got)
	}
	if got := read(original, root.ReviewID, end); len(got) != 0 {
		t.Fatalf("used a reply from a later commit: %+v", got)
	}
	got := read(current, root.ReviewID, end)
	if len(got) != 1 || got[0].Status != StatusHistorical || got[0].Revision != submitted.Commit ||
		got[0].Finding != root.Body || got[0].Reason != reply.Body || got[0].ReviewReply.OriginalRevision != original.Commit ||
		got[0].ReviewReply.SubmittedStartLine != 2 || got[0].ReviewReply.SubmittedEndLine != 3 ||
		len(got[0].Citations) != 1 || got[0].Citations[0].CurrentStartLine != 2 || got[0].Citations[0].CurrentEndLine != 4 ||
		!strings.Contains(got[0].Citations[0].Quote, "filtered by") {
		t.Fatalf("wrong review reply or evidence: %+v", got)
	}
	id := got[0].ID
	save(root, reply)
	if repeat := read(current, root.ReviewID, end); len(repeat) != 1 || repeat[0].ID != id {
		t.Fatalf("reimport duplicated or replaced the reply: %+v", repeat)
	}
	repo.write(path, "def pending(order):\n    return order.cancelled\n")
	changed := repo.commit("replace the discussed operation")
	if after := read(changed, root.ReviewID, end); len(after) != 1 || after[0].Status != StatusChanged || len(ReviewSnapshot(after)) != 0 {
		t.Fatalf("stale reply remained eligible: %+v", after)
	}
	edited := reply
	edited.UpdatedAt, edited.Body = "2025-08-06T17:00:00Z", "The queue now accepts pending orders."
	save(edited)
	if now := read(current, root.ReviewID, end); len(now) != 1 || now[0].Reason != edited.Body || now[0].ID != id {
		t.Fatalf("edited reply not selected: %+v", now)
	}
	if then := read(current, root.ReviewID, asOf("2025-08-06T16:00:00Z")); len(then) != 1 || then[0].Reason != reply.Body {
		t.Fatalf("later edit leaked into older review: %+v", then)
	}
	conflict := edited
	conflict.Body = "A contradictory edition at exactly the same time."
	save(conflict)
	if ambiguous := read(current, root.ReviewID, end); len(ambiguous) != 0 {
		t.Fatalf("ambiguous edition overrode an earlier one: %+v", ambiguous)
	}
	redacted := reply
	redacted.ID = "another-reply"
	save(redacted)
	secret := "ghp_" + strings.Repeat("x", 36)
	redacted.Body, redacted.UpdatedAt = "Use "+secret, "2025-08-06T18:00:00Z"
	save(redacted)
	if previous := read(current, root.ReviewID, asOf("2025-08-06T17:59:00Z")); len(previous) != 1 || previous[0].Reason != reply.Body {
		t.Fatalf("lost the earlier public reply: %+v", previous)
	}
	if visible := read(current, root.ReviewID, end); len(visible) != 0 {
		t.Fatalf("kept an earlier reply after its text became ineligible: %+v", visible)
	}
	all, err := db.ListAt(ctx, root.ReviewID, "", end)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range all {
		if strings.Contains(f.DiscussionBody, secret) {
			t.Fatal("stored the text of an ineligible edit")
		}
	}
}

func TestReviewReplyContextUsesEachSubmissionsCommit(t *testing.T) {
	ctx := t.Context()
	repo := newLifecycleRepo(t)
	const path, quote = "queue.go", "func pending() bool { return ready() }"
	repo.write(path, "package queue\n"+quote+"\n")
	first := repo.commit("queue ready orders")
	repo.write(path, "package queue\n// Called after validation.\n"+quote+"\n")
	second := repo.commit("document caller")
	const review = "https://github.com/example/store/pull/7"
	root := Discussion{ID: "first", ReviewID: review, Path: path, Body: "A queued order could still be pending.",
		PublishedAt: "2025-08-06T14:01:00Z", UpdatedAt: "2025-08-06T14:01:00Z", PublishedRevision: first.Commit,
		OriginalRevision: first.Commit, StartLine: 3, EndLine: 3, SourceQuote: quote}
	correction := Discussion{ID: "reply", ReviewID: review, ReplyTo: root.ID, Path: path, Body: "Only ready orders are queued.",
		PublishedAt: "2025-08-06T14:05:00Z", UpdatedAt: "2025-08-06T14:05:00Z", PublishedRevision: first.Commit}
	ack := correction
	ack.ID, ack.Body, ack.PublishedRevision = "ack", "Thanks for explaining the caller.", second.Commit
	ack.PublishedAt, ack.UpdatedAt = "2025-08-06T15:00:00Z", "2025-08-06T15:00:00Z"
	check := func(at string, notes ...Discussion) []Checked {
		t.Helper()
		now, err := time.Parse(time.RFC3339, at)
		if err != nil {
			t.Fatal(err)
		}
		second.Now = now
		db := testStore(t)
		result := discoverWithoutRules(t, first, DiscoverOptions{AsOf: now, Discussions: notes})
		if err := db.Save(ctx, result.DiscussionObservations()); err != nil {
			t.Fatal(err)
		}
		facts, err := db.ListAt(ctx, "https://github.com/Example/Store/pull/7", "", now)
		if err != nil {
			t.Fatal(err)
		}
		got, err := second.CheckReview(ctx, first.Commit, facts)
		if err != nil {
			t.Fatal(err)
		}
		return slices.DeleteFunc(got, func(c Checked) bool { return c.ReviewReply == nil })
	}
	const end = "2025-08-06T18:00:00Z"
	truncated := root
	truncated.StartLine, truncated.EndLine, truncated.ReportedOriginalLine = 2, 2, 4
	if found := check(end, truncated, correction); len(found) != 1 || found[0].Status != StatusHistorical || found[0].ReviewReply.ReportedOriginalLine != 4 {
		t.Fatalf("did not preserve the verified truncated selection and reported line: %+v", found)
	}
	truncated.StartLine, truncated.EndLine = 1, 1
	if missing := check(end, truncated, correction); len(missing) != 1 || missing[0].Status != StatusChanged {
		t.Fatalf("a matching line elsewhere was used for a truncated selection: %+v", missing)
	}
	got := check(end, ack, root, correction)
	if len(got) != 2 {
		t.Fatalf("wanted both replies, got %+v", got)
	}
	for _, c := range got {
		if c.Status != StatusHistorical || c.Citations[0].CurrentStartLine != 3 {
			t.Fatalf("did not locate the original selection in the current file: %+v", c)
		}
		switch c.ReviewReply.ID {
		case correction.ID:
			if c.Revision != first.Commit || !strings.Contains(c.ReviewReply.LaterNotes, ack.Body) {
				t.Fatalf("later acknowledgement replaced the correction: %+v", c)
			}
		case ack.ID:
			if c.Revision != second.Commit || !strings.Contains(c.ReviewReply.EarlierNotes, correction.Body) {
				t.Fatalf("missing the correction in the later turn: %+v", c)
			}
		}
	}
	if early := check("2025-08-06T14:59:00Z", ack, root, correction); len(early) != 1 || early[0].ReviewReply.LaterNotes != "" {
		t.Fatalf("early review saw later acknowledgement: %+v", early)
	}
	third, fourth := ack, ack
	third.ID, third.PublishedAt, third.Body = "third", "2025-08-06T16:00:00Z", "The batch caller also checks readiness."
	fourth.ID, fourth.PublishedAt, fourth.Body = "fourth", "2025-08-06T17:00:00Z", "This last reply concerns a separate queue."
	turns := check(end, fourth, third, ack, root, correction)
	if !slices.ContainsFunc(turns, func(c Checked) bool {
		notes := c.ReviewReply.LaterNotes
		return c.ReviewReply.ID == correction.ID && strings.Contains(notes, ack.Body) && strings.Contains(notes, third.Body) && !strings.Contains(notes, fourth.Body)
	}) {
		t.Fatalf("early correction lost its adjacent replies: %+v", turns)
	}
	edited := root
	edited.Body, edited.UpdatedAt = "A different finding.", "2025-08-06T16:00:00Z"
	if changed := check(end, edited, correction); len(changed) != 0 {
		t.Fatalf("reply was assigned to a later edit of its parent: %+v", changed)
	}
	second.ProposalRepliesOnly = true
	if imported := check(end, root, correction, ack); len(imported) != 1 || imported[0].ReviewReply.ID != ack.ID {
		t.Fatalf("auto-selected an already accepted historical reply: %+v", imported)
	}
	manual, err := second.DecideWithCitation(ctx, path, path, review, "A different finding.", "Locally recorded decision.", 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	manual.ObservedAt = correction.PublishedAt
	if auto, err := second.CheckReview(ctx, first.Commit, []Fact{manual}); err != nil || len(auto) != 0 {
		t.Fatalf("auto-selected a manual decision for a source PR: %+v, %v", auto, err)
	}
	second.ProposalRepliesOnly = false
	if explicit, err := second.CheckReview(ctx, first.Commit, []Fact{manual}); err != nil || len(explicit) != 1 || explicit[0].Status != StatusHistorical {
		t.Fatalf("lost the explicit manual decision: %+v, %v", explicit, err)
	}
	db := testStore(t)
	result := discoverWithoutRules(t, first, DiscoverOptions{AsOf: second.Now, Discussions: []Discussion{root, correction, ack}})
	if err := db.Save(ctx, result.DiscussionObservations()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Feedback(ctx, second, reviewNoteID(review, correction.ID), Feedback{Action: FeedbackReject, Reason: "The queue accepts unready orders."}); err != nil {
		t.Fatal(err)
	}
	second.Now = time.Now().Add(time.Second)
	facts, err := db.ListAt(ctx, review, "", second.Now)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := second.Check(ctx, facts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(checked, func(c Checked) bool {
		return c.ReviewReply != nil && c.ReviewReply.ID == ack.ID && c.ReviewReply.EarlierNotes == ""
	}) {
		t.Fatalf("the rejected reply still influenced its neighbor: %+v", checked)
	}
}

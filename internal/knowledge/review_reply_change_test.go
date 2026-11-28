// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/diff"
)

func TestReplyTracksChangesToTheSubmittedExplanation(t *testing.T) {
	repo := newLifecycleRepo(t)
	const file = "labels.py"
	const call = "    return print_label(unit)"
	const before = "def label():\n    unit = 'kg'\n" + call + "\n"
	repo.write(file, before)
	submitted := repo.commit("issue labels in kilograms")
	repo.write(file, strings.Replace(before, "'kg'", "'lb'", 1))
	current := repo.commit("issue labels in pounds")
	current.Now = time.Date(2025, 8, 7, 0, 0, 0, 0, time.UTC)
	root := Discussion{ID: "report", ReviewID: "labels-4", Path: file, Body: "A pounds label goes to the kilogram printer.",
		OriginalRevision: submitted.Commit, StartLine: 3, EndLine: 3, SourceQuote: call, PublishedRevision: submitted.Commit,
		PublishedAt: "2025-08-06T09:00:00Z", UpdatedAt: "2025-08-06T09:00:00Z"}
	reply := Discussion{ID: "answer", ReviewID: root.ReviewID, ReplyTo: root.ID, Path: file, Body: "The input is fixed to kg.",
		PublishedRevision: submitted.Commit, PublishedAt: "2025-08-06T10:00:00Z", UpdatedAt: "2025-08-06T10:00:00Z"}
	db := testStore(t)
	result := discoverWithoutRules(t, submitted, DiscoverOptions{AsOf: current.Now, Discussions: []Discussion{root, reply}})
	if err := db.Save(t.Context(), result.DiscussionObservations()); err != nil {
		t.Fatal(err)
	}
	facts, err := db.ListAt(t.Context(), root.ReviewID, "", current.Now)
	if err != nil {
		t.Fatal(err)
	}
	got, err := current.CheckReview(t.Context(), submitted.Commit, facts)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got {
		if c.ReviewReply == nil {
			continue
		}
		change := c.ReviewReply.Change
		if c.Status != StatusHistorical || change == nil || len(change.Hunks) != 1 || change.Submitted.Revision != submitted.Commit ||
			change.Current.Revision != current.Commit || !strings.Contains(change.Current.Quote, call) {
			t.Fatalf("lost the observed contract change: %+v", change)
		}
		h := change.Hunks[0]
		if h.Submitted.StartLine != 2 || h.Current.StartLine != 2 || h.Submitted.Quote != "    unit = 'kg'" || h.Current.Quote != "    unit = 'lb'" {
			t.Fatalf("wrong changed evidence: %+v", h)
		}
		if c.Fact.promptRecord().ReviewReply.Change != nil || c.ReviewReply.Change == nil {
			t.Fatal("ordinary prompt serialization leaked or mutated the paired function bodies")
		}
		return
	}
	t.Fatal("no historical reply was recovered")
}

func TestReplyChangesRequireCurrentExecutableEvidence(t *testing.T) {
	const original = "def cost(item):\n    if item.paid:\n        tally(item)\n        audit(item)\n    return total(item)\n"
	const file = "billing.py"
	replaced := func(a, b, n int) diff.Hunk { return diff.Hunk{OldStart: a, OldCount: n, NewStart: b, NewCount: n} }
	for _, tc := range []struct {
		name, current string
		line          int
		hunk          diff.Hunk
		want          bool
	}{
		{"trailing comment", strings.Replace(original, "tally(item)", "tally(item)  # capture the item", 1), 5, replaced(3, 3, 1), false},
		{"equivalent reindent", strings.ReplaceAll(original, "        ", "            "), 5, replaced(3, 3, 2), false},
		{"different guard scope", strings.Replace(original, "        audit(item)", "    audit(item)", 1), 5, replaced(4, 4, 1), true},
		{"new executable step", strings.Replace(original, "    return", "    settle(item)\n    return", 1), 6, diff.Hunk{OldStart: 4, NewStart: 5, NewCount: 1}, true},
		{"removed executable step", strings.Replace(original, "        audit(item)\n", "", 1), 4, diff.Hunk{OldStart: 4, OldCount: 1, NewStart: 3}, true},
		{"another function", original + "\ndef other():\n    settle(None)\n", 5, diff.Hunk{OldStart: 5, NewStart: 6, NewCount: 3}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := changedReplyFunction(file, "before", "after", original, tc.current, 5, tc.line, []diff.Hunk{tc.hunk})
			if (got != nil) != tc.want {
				t.Fatalf("unexpected reason to revisit the reply: %+v", got)
			}
			if got != nil && got.Hunks[0].LayoutChanged != (tc.name == "different guard scope") {
				t.Fatalf("misclassified the scope change: %+v", got.Hunks)
			}
		})
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

// compareEvalCase labels which before/after findings describe the same
// underlying issue. Comments are identified by StartLine, which Compare
// deliberately ignores; keep those values unique within one case.
type compareEvalCase struct {
	name            string
	before          []model.LlmComment
	after           []model.LlmComment
	manifest        *RunManifest
	same            [][2]int
	knownSplit      bool
	wantNotReviewed int
}

func bucketLines(comments []model.LlmComment) map[int]bool {
	lines := make(map[int]bool, len(comments))
	for _, c := range comments {
		lines[c.StartLine] = true
	}
	return lines
}

func compareEvalCases() []compareEvalCase {
	return []compareEvalCase{
		{
			name:   "line drift persists",
			before: []model.LlmComment{cmt("a.go", 10, "bug", "x := 1", "unused")},
			after:  []model.LlmComment{cmt("a.go", 30, "bug", "x := 1", "unused")},
			same:   [][2]int{{0, 0}},
		},
		{
			name:   "reindented snippet persists",
			before: []model.LlmComment{cmt("a.go", 10, "bug", "x := 1\ny := 2", "unused")},
			after:  []model.LlmComment{cmt("a.go", 30, "bug", "\t\tx := 1\n\t\ty := 2", "unused")},
			same:   [][2]int{{0, 0}},
		},
		{
			name:   "reworded content with same snippet persists",
			before: []model.LlmComment{cmt("a.go", 10, "bug", "x := 1", "this variable is unused")},
			after:  []model.LlmComment{cmt("a.go", 30, "bug", "x := 1", "x is never read")},
			same:   [][2]int{{0, 0}},
		},
		{
			name:   "category case change persists",
			before: []model.LlmComment{cmt("a.go", 10, "Bug", "x := 1", "unused")},
			after:  []model.LlmComment{cmt("a.go", 30, "bug", "x := 1", "unused")},
			same:   [][2]int{{0, 0}},
		},
		{
			name:   "snippet range expansion persists",
			before: []model.LlmComment{cmt("db.go", 10, "bug", "rows, err := db.Query(query)", "handle the error")},
			after:  []model.LlmComment{cmt("db.go", 30, "bug", "rows, err := db.Query(query)\nif err != nil {\n    return err\n}", "handle the error")},
			same:   [][2]int{{0, 0}},
		},
		{
			name:   "snippet range contraction persists",
			before: []model.LlmComment{cmt("db.go", 10, "bug", "rows, err := db.Query(query)\nif err != nil {\n    return err\n}", "handle the error")},
			after:  []model.LlmComment{cmt("db.go", 30, "bug", "rows, err := db.Query(query)", "handle the error")},
			same:   [][2]int{{0, 0}},
		},
		{
			name:       "category drift is a known split",
			before:     []model.LlmComment{cmt("a.go", 10, "bug", "x := 1", "unused")},
			after:      []model.LlmComment{cmt("a.go", 30, "style", "x := 1", "unused")},
			same:       [][2]int{{0, 0}},
			knownSplit: true,
		},
		{
			name:       "empty snippet reworded is a known split",
			before:     []model.LlmComment{cmt("a.go", 10, "bug", "", "missing error check")},
			after:      []model.LlmComment{cmt("a.go", 30, "bug", "", "the error is dropped")},
			same:       [][2]int{{0, 0}},
			knownSplit: true,
		},
		{
			name:   "genuine new finding is new",
			before: []model.LlmComment{cmt("a.go", 10, "bug", "x := 1", "unused")},
			after:  []model.LlmComment{cmt("a.go", 30, "bug", "y := 2", "also unused")},
		},
		{
			name:   "short generic snippet is not merged",
			before: []model.LlmComment{cmt("a.go", 10, "bug", "return nil", "error is swallowed")},
			after:  []model.LlmComment{cmt("a.go", 30, "bug", "if err != nil {\n    return nil\n}", "error is swallowed")},
		},
		{
			name:   "shared line with different finding text is not merged",
			before: []model.LlmComment{cmt("a.go", 10, "style", "log.Printf(\"starting server\")", "use structured logging")},
			after:  []model.LlmComment{cmt("a.go", 30, "style", "log.Printf(\"starting server\")\ndef cancel()", "missing context cancellation")},
		},
		{
			name:     "renamed file persists",
			before:   []model.LlmComment{cmt("old/a.go", 10, "bug", "x := 1", "unused")},
			after:    []model.LlmComment{cmt("new/a.go", 30, "bug", "x := 1", "unused")},
			manifest: reviewedManifest(CoverageItem{Path: "new/a.go", OldPath: "old/a.go"}),
			same:     [][2]int{{0, 0}},
		},
		{
			name:            "skipped file is not reviewed",
			before:          []model.LlmComment{cmt("skipped.go", 10, "bug", "x := 1", "unused")},
			manifest:        reviewedManifest(CoverageItem{Path: "a.go"}),
			wantNotReviewed: 1,
		},
	}
}

// TestCompareEvalCorpus guards the conservative matching contract on a labeled
// corpus. Required same-issue pairs must persist, known limitations must stay
// visible as splits, and unrelated findings must never be merged.
func TestCompareEvalCorpus(t *testing.T) {
	var requiredPairs, requiredCorrect, knownSplits, knownSplitsObserved, falseMerges, expectedNew, newCorrect, notReviewed int

	for _, tc := range compareEvalCases() {
		got := Compare(tc.before, tc.after, tc.manifest)
		persisting := bucketLines(got.Persisting)
		newBucket := bucketLines(got.New)

		sameAfter := make(map[int]bool, len(tc.same))
		caseRequired, caseRequiredCorrect := 0, 0
		for _, pair := range tc.same {
			sameAfter[pair[1]] = true
			matched := persisting[tc.after[pair[1]].StartLine]
			if tc.knownSplit {
				knownSplits++
				if !matched {
					knownSplitsObserved++
				}
				continue
			}
			requiredPairs++
			caseRequired++
			if matched {
				requiredCorrect++
				caseRequiredCorrect++
			} else {
				t.Errorf("%s: required same-issue pair %v was split into resolved + new", tc.name, pair)
			}
		}

		caseFalseMerges := 0
		for i, c := range tc.after {
			if !sameAfter[i] && persisting[c.StartLine] {
				caseFalseMerges++
			}
		}
		falseMerges += caseFalseMerges

		caseExpectedNew := len(tc.after) - len(sameAfter)
		caseNewCorrect := 0
		for i, c := range tc.after {
			if !sameAfter[i] && newBucket[c.StartLine] {
				caseNewCorrect++
			}
		}
		if caseNewCorrect != caseExpectedNew {
			t.Errorf("%s: new = %d/%d, want all unrelated findings in new", tc.name, caseNewCorrect, caseExpectedNew)
		}
		expectedNew += caseExpectedNew
		newCorrect += caseNewCorrect
		notReviewed += len(got.NotReviewed)

		if len(got.NotReviewed) != tc.wantNotReviewed {
			t.Errorf("%s: not_reviewed = %d, want %d", tc.name, len(got.NotReviewed), tc.wantNotReviewed)
		}
		if caseFalseMerges != 0 {
			t.Errorf("%s: %d unrelated finding(s) were merged as persisting", tc.name, caseFalseMerges)
		}

		t.Logf("%s: required_correct=%d/%d known_split=%v new=%d/%d not_reviewed=%d",
			tc.name, caseRequiredCorrect, caseRequired, tc.knownSplit, caseNewCorrect, caseExpectedNew, len(got.NotReviewed))
	}

	if knownSplitsObserved != knownSplits {
		t.Errorf("known split behavior changed: observed %d/%d; update the corpus if the matcher now handles this case", knownSplitsObserved, knownSplits)
	}
	t.Logf("TOTAL: required=%d/%d known_splits=%d/%d false_merge=%d new=%d/%d not_reviewed=%d",
		requiredCorrect, requiredPairs, knownSplitsObserved, knownSplits, falseMerges, newCorrect, expectedNew, notReviewed)
}

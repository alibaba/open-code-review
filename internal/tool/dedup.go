// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"fmt"

	"github.com/alibaba/open-code-review/internal/model"
)

// severityRank orders severities so DedupByLocation can keep the more
// important comment when two candidates land on the same location.
// Unknown/empty severities sort last (rank 0).
var severityRank = map[string]int{
	"critical": 4,
	"high":     3,
	"medium":   2,
	"low":      1,
}

// DedupByLocation collapses comments that target the same file and
// overlapping line range into a single comment. This is a cheap,
// deterministic, no-LLM-call safety net — distinct from the LLM-based
// DEDUP_TASK used by `ocr scan` — intended to catch same-location repeats
// that slip through when the agent re-discovers an issue via a different
// tool-call path (e.g. re-reading a file segment, a second code_search hit).
//
// Two comments are considered the same location when they share Path and
// their [StartLine, EndLine] ranges overlap. Among comments in the same
// group, the one with the highest severity is kept; ties keep the first
// one encountered (stable order is preserved for everything else).
func DedupByLocation(comments []model.LlmComment) []model.LlmComment {
	if len(comments) < 2 {
		return comments
	}

	// groupsByPath maps a path to indices (positions in `out`) of the
	// group-representative comments seen so far for that file. Within a
	// path we do a linear overlap scan since the number of comments per
	// file is small.
	groupsByPath := make(map[string][]int)
	out := make([]model.LlmComment, 0, len(comments))
	outGroupBest := make([]int, 0, len(comments)) // parallel to `out`: rank of the kept comment

	for _, c := range comments {
		reps := groupsByPath[c.Path]
		merged := false
		for _, repPos := range reps {
			rep := out[repPos]
			if overlaps(rep.StartLine, rep.EndLine, c.StartLine, c.EndLine) {
				rank := severityRank[c.Severity]
				if rank > outGroupBest[repPos] {
					out[repPos] = c
					outGroupBest[repPos] = rank
				}
				merged = true
				break
			}
		}
		if !merged {
			groupsByPath[c.Path] = append(groupsByPath[c.Path], len(out))
			out = append(out, c)
			outGroupBest = append(outGroupBest, severityRank[c.Severity])
		}
	}
	return out
}

// overlaps reports whether two 1-indexed inclusive line ranges intersect.
// A zero-width range (start==end==0, i.e. unset) never matches anything,
// not even another unset range — the safe default is to never merge
// comments we can't place a line number on.
func overlaps(aStart, aEnd, bStart, bEnd int) bool {
	if (aStart == 0 && aEnd == 0) || (bStart == 0 && bEnd == 0) {
		return false
	}
	return aStart <= bEnd && bStart <= aEnd
}

// DedupSummary renders a one-line human-readable summary of a dedup pass,
// used for the same "[ocr] ... N → M comments" style logging the scan
// pipeline already emits.
func DedupSummary(before, after int) string {
	if before == after {
		return ""
	}
	return fmt.Sprintf("%d → %d comments", before, after)
}
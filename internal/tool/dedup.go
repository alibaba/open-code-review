// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/alibaba/open-code-review/internal/model"
)

// severityRank orders severities so DedupByLocation can keep the more
// important comment when two candidates are judged duplicates of one
// another. Unknown/empty severities sort last (rank 0).
var severityRank = map[string]int{
	"critical": 4,
	"high":     3,
	"medium":   2,
	"low":      1,
}

// contentSimilarityThreshold is the minimum Jaccard similarity of
// significant words (see tokenize) two comments' Content must share, in
// addition to an overlapping line range, before DedupByLocation treats them
// as the same finding. Calibrated against real near-duplicate pairs (two
// paraphrasings of one finding typically score well above this) while
// staying far below the score two topically-unrelated findings on the same
// line would produce (near zero — see isDuplicateFinding's package doc
// example of a SQL-injection finding vs. an ignored-error finding on the
// same range).
const contentSimilarityThreshold = 0.30

// existingCodeSimilarityThreshold is looser than contentSimilarityThreshold:
// two comments quoting nearly the same existing_code snippet are anchored to
// the same statement, which is itself decent (if not sufficient on its own)
// evidence of the same finding, so this alternate path uses a lower bar.
const existingCodeSimilarityThreshold = 0.60

// DedupByLocation collapses comments that (a) target the same file with an
// overlapping line range AND (b) describe the same underlying finding, into
// a single comment. This is a cheap, deterministic, no-LLM-call safety net —
// distinct from the LLM-based DEDUP_TASK used by `ocr scan` — intended to
// catch same-location, same-finding repeats that slip through when the agent
// re-discovers an issue via a different tool-call path (e.g. re-reading a
// file segment, a second code_search hit).
//
// Location overlap alone is NOT sufficient: two independent, equally valid
// findings can legitimately share a line range (e.g. unsafe SQL construction
// and a separately-ignored database error on the same statement). Merging
// on location alone would silently drop one of them from the final review
// output. So a pair is only merged when their Content (or, failing that,
// their ExistingCode snippet) is textually similar enough to indicate the
// same finding — see contentSimilarityThreshold / existingCodeSimilarityThreshold.
// When in doubt, both comments are kept.
//
// Among comments judged duplicates of one another, the one with the highest
// severity is kept; ties keep the first one encountered (stable order is
// preserved for everything else).
func DedupByLocation(comments []model.LlmComment) []model.LlmComment {
	if len(comments) < 2 {
		return comments
	}

	// groupsByPath maps a path to indices (positions in `out`) of the
	// group-representative comments seen so far for that file. Within a
	// path we do a linear scan since the number of comments per file is
	// small.
	groupsByPath := make(map[string][]int)
	out := make([]model.LlmComment, 0, len(comments))
	outGroupBest := make([]int, 0, len(comments)) // parallel to `out`: rank of the kept comment

	for _, c := range comments {
		reps := groupsByPath[c.Path]
		merged := false
		for _, repPos := range reps {
			rep := out[repPos]
			if isDuplicateFinding(rep, c) {
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

// isDuplicateFinding reports whether a and b are two reports of the same
// underlying finding: an overlapping line range is necessary but not
// sufficient, since distinct findings routinely share a line (a single
// statement can be both a SQL-injection risk and silently swallow an error).
// The pair is treated as one finding only when, in addition to the location
// overlap, their Content is similar enough (contentSimilarityThreshold), or
// — as a fallback for cases where the two write-ups share little vocabulary
// but quote the same code — their ExistingCode snippets are near-identical
// (existingCodeSimilarityThreshold).
func isDuplicateFinding(a, b model.LlmComment) bool {
	if !overlaps(a.StartLine, a.EndLine, b.StartLine, b.EndLine) {
		return false
	}
	if jaccard(tokenize(a.Content), tokenize(b.Content)) >= contentSimilarityThreshold {
		return true
	}
	if strings.TrimSpace(a.ExistingCode) != "" && strings.TrimSpace(b.ExistingCode) != "" {
		if jaccard(tokenize(a.ExistingCode), tokenize(b.ExistingCode)) >= existingCodeSimilarityThreshold {
			return true
		}
	}
	return false
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

// tokenWordPattern extracts "word-like" runs: letters, digits, underscore
// and dot, so identifiers such as config.asdf or skip_DB_connection survive
// as single tokens instead of being split apart.
var tokenWordPattern = regexp.MustCompile(`[A-Za-z0-9_.]+`)

// stopWords are common English function words filtered out before comparing
// two comments' vocabulary, so shared connective tissue ("the", "before",
// "can") doesn't inflate the similarity score between unrelated findings.
var stopWords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "is": {}, "are": {}, "was": {}, "were": {},
	"be": {}, "been": {}, "being": {}, "to": {}, "of": {}, "in": {}, "on": {},
	"at": {}, "by": {}, "for": {}, "with": {}, "or": {}, "and": {}, "but": {},
	"if": {}, "this": {}, "that": {}, "these": {}, "those": {}, "it": {},
	"its": {}, "as": {}, "will": {}, "would": {}, "could": {}, "should": {},
	"can": {}, "not": {}, "no": {}, "so": {}, "than": {}, "then": {},
	"there": {}, "here": {}, "from": {}, "into": {}, "do": {}, "does": {},
	"did": {}, "done": {}, "has": {}, "have": {}, "had": {}, "may": {},
	"might": {}, "must": {}, "shall": {}, "we": {}, "you": {}, "your": {},
	"they": {}, "them": {}, "he": {}, "she": {}, "his": {}, "her": {}, "i": {},
}

// tokenize lowercases s and returns the set of significant (non-stopword)
// word-like tokens it contains, deduplicated.
func tokenize(s string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, w := range tokenWordPattern.FindAllString(strings.ToLower(s), -1) {
		if _, stop := stopWords[w]; stop {
			continue
		}
		out[w] = struct{}{}
	}
	return out
}

// jaccard returns the Jaccard similarity |a ∩ b| / |a ∪ b| of two token
// sets. Two empty sets are dissimilar (0), matching the "when in doubt,
// don't merge" default.
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for w := range a {
		if _, ok := b[w]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
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
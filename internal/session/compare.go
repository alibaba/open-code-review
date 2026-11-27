// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/model"
)

// CompareResult groups two sessions' findings into four disjoint buckets.
// Every bucket is non-nil and sorted, so both the text and the JSON outlet
// render the same order regardless of the order LoadComments replayed.
type CompareResult struct {
	New        []model.LlmComment `json:"new"`
	Persisting []model.LlmComment `json:"persisting"`
	Resolved   []model.LlmComment `json:"resolved"`
	// NotReviewed holds before-findings whose file the after run never looked
	// at. They are not resolved: nobody re-checked them.
	NotReviewed []model.LlmComment `json:"not_reviewed"`
}

// Compare buckets the findings of an earlier run (before) against a later one
// (after). It first matches findings with the exact findingKey identity used by
// earlier versions. For findings that do not match exactly, it then tries a
// conservative snippet containment rule: same path and category, one snippet
// is a contiguous run of whole lines inside the other, the shorter snippet is
// at least minContainedSnippetChars long, and the finding text is identical.
// Snippet containment handles a finding whose selected range grew or shrank
// without treating arbitrary substring overlap as the same issue.
//
// Matching is a multiset match: N identical findings before and M after yield
// min(N,M) persisting plus the remainder on whichever side is longer.
// Persisting carries the after copy, which has the current line numbers.
//
// afterManifest supplies both the paths the after run actually reviewed and
// any old-to-new path pairs recorded for renamed files. When it is nil - a
// legacy session that recorded no manifest - every unmatched before-finding is
// reported as resolved and paths are matched as recorded.
func Compare(before, after []model.LlmComment, afterManifest *RunManifest) CompareResult {
	reviewed := ReviewedPaths(afterManifest)
	renamed := renamedPaths(afterManifest)

	used := make([]bool, len(after))
	exact := make(map[findingIdentity][]int, len(after))
	groups := make(map[findingGroupKey][]int, len(after))
	for i, c := range after {
		key := findingKey(c, c.Path)
		exact[key] = append(exact[key], i)
		group := findingGroupKey{path: key.path, category: key.category}
		groups[group] = append(groups[group], i)
	}

	var res CompareResult
	for _, c := range before {
		path := normalizePath(c.Path)
		if newPath, ok := renamed[path]; ok {
			path = newPath
		}
		key := findingKey(c, path)
		group := findingGroupKey{path: key.path, category: key.category}

		matched := firstUnused(exact[key], used)
		if matched < 0 {
			matched = matchContainedSnippet(c, after, groups[group], used)
		}
		if matched >= 0 {
			used[matched] = true
			res.Persisting = append(res.Persisting, after[matched])
			continue
		}
		// A path-less finding cannot be attributed to any reviewed file, so it
		// falls out here as not-reviewed rather than being claimed as fixed.
		if reviewed != nil && !reviewed[key.path] {
			res.NotReviewed = append(res.NotReviewed, c)
			continue
		}
		res.Resolved = append(res.Resolved, c)
	}

	// Whatever the before side never claimed is new.
	for i, c := range after {
		if !used[i] {
			res.New = append(res.New, c)
		}
	}

	res.New = sortFindings(res.New)
	res.Persisting = sortFindings(res.Persisting)
	res.Resolved = sortFindings(res.Resolved)
	res.NotReviewed = sortFindings(res.NotReviewed)
	return res
}

func firstUnused(indices []int, used []bool) int {
	for _, idx := range indices {
		if !used[idx] {
			return idx
		}
	}
	return -1
}

// matchContainedSnippet chooses the most similar unchecked candidate in the
// same path/category group. Length difference is the primary tie-break and
// line distance is only a secondary one; line numbers are still not identity,
// so pure line drift does not prevent a match.
func matchContainedSnippet(before model.LlmComment, after []model.LlmComment, indices []int, used []bool) int {
	beforeText := normalizeSnippet(before.Content)
	if beforeText == "" {
		return -1
	}
	best := -1
	bestLengthDelta := 0
	bestLineDelta := 0
	for _, idx := range indices {
		if used[idx] {
			continue
		}
		candidate := after[idx]
		if normalizeSnippet(candidate.Content) != beforeText {
			continue
		}
		if !lineBlockContains(candidate.ExistingCode, before.ExistingCode) &&
			!lineBlockContains(before.ExistingCode, candidate.ExistingCode) {
			continue
		}
		lengthDelta := absInt(len(normalizeSnippet(candidate.ExistingCode)) - len(normalizeSnippet(before.ExistingCode)))
		lineDelta := absInt(candidate.StartLine - before.StartLine)
		if best < 0 || lengthDelta < bestLengthDelta ||
			(lengthDelta == bestLengthDelta && lineDelta < bestLineDelta) {
			best = idx
			bestLengthDelta = lengthDelta
			bestLineDelta = lineDelta
		}
	}
	return best
}

// minContainedSnippetChars avoids treating a short, generic line such as
// "return nil" as the identity of a larger block. The issue's motivating
// example (a Query call followed by its error check) is comfortably above it.
const minContainedSnippetChars = 24

// lineBlockContains reports whether inner is a contiguous run of complete
// normalized lines in outer. Partial-line overlap is deliberately excluded:
// it is too easy for a common expression to appear inside unrelated code.
func lineBlockContains(outer, inner string) bool {
	if utf8.RuneCountInString(normalizeSnippet(inner)) < minContainedSnippetChars {
		return false
	}
	outerLines := normalizedLines(outer)
	innerLines := normalizedLines(inner)
	if len(innerLines) == 0 || len(innerLines) >= len(outerLines) {
		return false
	}
	for start := 0; start+len(innerLines) <= len(outerLines); start++ {
		matches := true
		for offset := range innerLines {
			if outerLines[start+offset] != innerLines[offset] {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func normalizedLines(s string) []string {
	raw := strings.Split(s, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = normalizeSnippet(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// findingKey identifies a finding across runs by what it is about, not where it
// sits: path, category and the offending snippet. Line numbers are deliberately
// absent so a finding that merely drifted down the file still matches.
//
// ponytail: deliberately separate from sarifFingerprints in cmd/opencodereview,
// which keys on the raw snippet and falls back to StartLine. That fingerprint is
// the identity of a published GitHub Code Scanning alert - changing how it
// normalizes would re-open every alert downstream - so the two stay apart even
// though they read the same fields.
func findingKey(c model.LlmComment, path string) findingIdentity {
	return findingIdentity{
		path:     normalizePath(path),
		category: normalizeCategory(c.Category),
		body:     findingBody(c),
	}
}

func findingBody(c model.LlmComment) string {
	body := normalizeSnippet(c.ExistingCode)
	if body == "" {
		// ponytail: no snippet recorded, so the prose is all the identity there
		// is - a reworded finding then reads as resolved + new. A StartLine
		// fallback would be worse: it re-breaks the line drift this key exists
		// to survive.
		body = normalizeSnippet(c.Content)
	}
	return body
}

func normalizeCategory(category string) string {
	return strings.ToLower(strings.TrimSpace(category))
}

type findingIdentity struct {
	path     string
	category string
	body     string
}

type findingGroupKey struct {
	path     string
	category string
}

// normalizeSnippet collapses every whitespace run to a single space so that
// re-indentation and line rewrapping do not change a finding's identity.
// Semantically the same trick as internal/diff.normalizeLine, minus the diff
// markers, which never reach a persisted comment.
func normalizeSnippet(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// sortFindings orders a bucket by (path, start line, category, content,
// snippet) and guarantees a non-nil slice so empty buckets encode as [] rather
// than null. The snippet is the last tie-break because the New bucket is
// drained from a map: two findings that differ only in their snippet sit under
// different keys, so without it their relative order would follow Go's random
// map iteration and make --json output undiffable between runs.
func sortFindings(findings []model.LlmComment) []model.LlmComment {
	if findings == nil {
		return []model.LlmComment{}
	}
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.StartLine != b.StartLine {
			return a.StartLine < b.StartLine
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		if a.Content != b.Content {
			return a.Content < b.Content
		}
		return a.ExistingCode < b.ExistingCode
	})
	return findings
}

// ReviewedPaths returns the paths a run actually decided on, or nil when the run
// recorded no manifest - nil tells Compare to treat every unmatched
// before-finding as resolved.
//
// Completed and Reused are the only partitions that mean "the LLM's verdict on
// this file is current": Reused carries a checkpoint forward from a resume
// chain, which is still a verdict. Selected is deliberately not used - it is
// the intended set, so an interrupted run would report every file it never got
// to as clean. Failed and Waived items are selected but undecided for the same
// reason.
func ReviewedPaths(m *RunManifest) map[string]bool {
	if m == nil {
		return nil
	}
	cov := m.Coverage
	paths := make(map[string]bool, len(cov.Completed)+len(cov.Reused))
	for _, items := range [][]CoverageItem{cov.Completed, cov.Reused} {
		for _, item := range items {
			paths[normalizePath(item.Path)] = true
		}
	}
	return paths
}

// renamedPaths returns normalized old-to-new path mappings for files whose
// verdict is current in the after run. Failed and waived files are excluded for
// the same reason ReviewedPaths excludes them: the later run did not decide
// whether their earlier findings still apply.
func renamedPaths(m *RunManifest) map[string]string {
	if m == nil {
		return nil
	}
	cov := m.Coverage
	paths := make(map[string]string)
	for _, items := range [][]CoverageItem{cov.Completed, cov.Reused} {
		for _, item := range items {
			oldPath := normalizePath(item.OldPath)
			newPath := normalizePath(item.Path)
			if oldPath != "" && newPath != "" && oldPath != newPath {
				paths[oldPath] = newPath
			}
		}
	}
	return paths
}

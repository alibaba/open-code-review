// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"sort"
	"strings"

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
// (after). Matching is a multiset match on findingKey, so N identical findings
// before and M after yield min(N,M) persisting plus the remainder on whichever
// side is longer. Persisting carries the after copy, which has the current line
// numbers.
//
// afterManifest supplies both the paths the after run actually reviewed and
// any old-to-new path pairs recorded for renamed files. When it is nil - a
// legacy session that recorded no manifest - every unmatched before-finding is
// reported as resolved and paths are matched as recorded.
func Compare(before, after []model.LlmComment, afterManifest *RunManifest) CompareResult {
	reviewed := ReviewedPaths(afterManifest)
	renamed := renamedPaths(afterManifest)

	pool := make(map[string][]model.LlmComment, len(after))
	for _, c := range after {
		k := findingKey(c, c.Path)
		pool[k] = append(pool[k], c)
	}

	var res CompareResult
	for _, c := range before {
		path := normalizePath(c.Path)
		if newPath, ok := renamed[path]; ok {
			path = newPath
		}
		k := findingKey(c, path)
		if matches := pool[k]; len(matches) > 0 {
			res.Persisting = append(res.Persisting, matches[0])
			pool[k] = matches[1:]
			continue
		}
		// A path-less finding cannot be attributed to any reviewed file, so it
		// falls out here as not-reviewed rather than being claimed as fixed.
		if reviewed != nil && !reviewed[path] {
			res.NotReviewed = append(res.NotReviewed, c)
			continue
		}
		res.Resolved = append(res.Resolved, c)
	}
	// Whatever the before side never claimed is new. Map iteration order is
	// random; sortFindings below makes that unobservable.
	for _, rest := range pool {
		res.New = append(res.New, rest...)
	}

	res.New = sortFindings(res.New)
	res.Persisting = sortFindings(res.Persisting)
	res.Resolved = sortFindings(res.Resolved)
	res.NotReviewed = sortFindings(res.NotReviewed)
	return res
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
func findingKey(c model.LlmComment, path string) string {
	body := normalizeSnippet(c.ExistingCode)
	if body == "" {
		// ponytail: no snippet recorded, so the prose is all the identity there
		// is - a reworded finding then reads as resolved + new. A StartLine
		// fallback would be worse: it re-breaks the line drift this key exists
		// to survive.
		body = normalizeSnippet(c.Content)
	}
	return normalizePath(path) + "|" + strings.ToLower(strings.TrimSpace(c.Category)) + "|" + body
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

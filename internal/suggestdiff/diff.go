// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package suggestdiff provides line-level diff computation between code snippets,
// used for CLI rendering of review suggestions with ANSI color codes.
package suggestdiff

import "strings"

// DiffLineType marks a line as context, added, or deleted.
type DiffLineType int

const (
	DiffContext DiffLineType = iota
	DiffAdded
	DiffDeleted
)

// DiffLine is a single line in the diff result.
type DiffLine struct {
	Type    DiffLineType
	Content string
}

// sameLine reports whether two lines count as unchanged for diff purposes.
//
// Leading and trailing whitespace is ignored: ExistingCode is a model-quoted
// excerpt whose indentation cannot be trusted, so requiring byte equality would
// render an entire block as rewritten whenever the model re-indented its quote.
// This is the same tolerance diff.normalizeLine applies when matching
// ExistingCode against the real file to resolve line numbers.
//
// Case is deliberately NOT folded. Identifiers differ by case in essentially
// every language — in Go it is the exported/unexported boundary — so folding it
// made a `foo` -> `Foo` suggestion render as a single unchanged context line
// showing the OLD text, i.e. as though the suggestion changed nothing.
func sameLine(a, b string) bool {
	return strings.TrimSpace(a) == strings.TrimSpace(b)
}

// ComputeLineDiff returns a line-level diff between oldLines and newLines.
// Uses Myers-style LCS to find common subsequences, then emits context/added/deleted lines.
func ComputeLineDiff(oldLines, newLines []string) []DiffLine {
	m, n := len(oldLines), len(newLines)
	if m == 0 && n == 0 {
		return nil
	}

	// LCS DP table
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if sameLine(oldLines[i-1], newLines[j-1]) {
				lcs[i][j] = lcs[i-1][j-1] + 1
			} else {
				lcs[i][j] = max(lcs[i-1][j], lcs[i][j-1])
			}
		}
	}

	// Backtrack to produce diff
	var result []DiffLine
	i, j := m, n
	back := make([]DiffLine, 0, max(m, n)*2)
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && sameLine(oldLines[i-1], newLines[j-1]) {
			back = append(back, DiffLine{Type: DiffContext, Content: oldLines[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || lcs[i][j-1] >= lcs[i-1][j]) {
			back = append(back, DiffLine{Type: DiffAdded, Content: newLines[j-1]})
			j--
		} else {
			back = append(back, DiffLine{Type: DiffDeleted, Content: oldLines[i-1]})
			i--
		}
	}

	// Reverse
	for idx := len(back) - 1; idx >= 0; idx-- {
		result = append(result, back[idx])
	}
	return result
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

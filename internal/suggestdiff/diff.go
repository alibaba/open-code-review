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

type diffStep uint8

const (
	stepMatch diffStep = iota
	stepAdd
	stepDelete
)

// ComputeLineDiff returns a line-level diff between oldLines and newLines.
// Uses LCS alignment to find common subsequences, then emits context/added/deleted lines.
func ComputeLineDiff(oldLines, newLines []string) []DiffLine {
	m, n := len(oldLines), len(newLines)
	if m == 0 && n == 0 {
		return nil
	}

	// Backtracking needs only the chosen direction, not every LCS score.
	steps := make([][]diffStep, m)
	lcs := make([]int, n+1)
	trimmedNew := make([]string, n)
	for j, line := range newLines {
		trimmedNew[j] = strings.TrimSpace(line)
	}
	for i, line := range oldLines {
		steps[i] = make([]diffStep, n)
		trimmedOld := strings.TrimSpace(line)
		diagonal := 0
		for j, newLine := range trimmedNew {
			above := lcs[j+1]
			if strings.EqualFold(trimmedOld, newLine) {
				lcs[j+1] = diagonal + 1
				steps[i][j] = stepMatch
			} else if lcs[j] >= above {
				lcs[j+1] = lcs[j]
				steps[i][j] = stepAdd
			} else {
				steps[i][j] = stepDelete
			}
			diagonal = above
		}
	}

	// Backtrack to produce diff
	var result []DiffLine
	i, j := m, n
	back := make([]DiffLine, 0, max(m, n)*2)
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && steps[i-1][j-1] == stepMatch {
			if oldLines[i-1] == newLines[j-1] {
				back = append(back, DiffLine{Type: DiffContext, Content: oldLines[i-1]})
			} else {
				// Fuzzy equality is only for alignment; preserve raw changes in the rendered diff.
				back = append(back, DiffLine{Type: DiffAdded, Content: newLines[j-1]})
				back = append(back, DiffLine{Type: DiffDeleted, Content: oldLines[i-1]})
			}
			i--
			j--
		} else if j > 0 && (i == 0 || steps[i-1][j-1] == stepAdd) {
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

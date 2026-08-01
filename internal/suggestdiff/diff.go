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
			if strings.EqualFold(strings.TrimSpace(oldLines[i-1]), strings.TrimSpace(newLines[j-1])) {
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
		if i > 0 && j > 0 && strings.EqualFold(strings.TrimSpace(oldLines[i-1]), strings.TrimSpace(newLines[j-1])) {
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

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"strings"

	"github.com/alibaba/open-code-review/internal/model"
)

// ResolveLineNumbers populates StartLine/EndLine on each comment by matching
// the ExistingCode against the corresponding file's diff hunks (primary), or
// falling back to scanning the full new-file content line-by-line.
func ResolveLineNumbers(comments []model.LlmComment, diffs []model.Diff) []model.LlmComment {
	if len(comments) == 0 || len(diffs) == 0 {
		return comments
	}

	// Build lookup: newPath -> *Diff
	diffByPath := make(map[string]*model.Diff, len(diffs))
	for i := range diffs {
		d := &diffs[i]
		if d.NewPath != "/dev/null" && d.NewPath != "" {
			diffByPath[d.NewPath] = d
		}
		if d.OldPath != "/dev/null" && d.OldPath != "" {
			diffByPath[d.OldPath] = d
		}
	}

	result := make([]model.LlmComment, len(comments))
	copy(result, comments)

	for i := range result {
		cm := &result[i]
		if cm.StartLine > 0 || cm.EndLine > 0 {
			continue
		}
		if cm.ExistingCode == "" {
			continue
		}
		d, ok := diffByPath[cm.Path]
		if !ok {
			continue
		}

		// Primary: try matching from deleted/context lines in diff hunks
		if resolveFromHunk(d, cm) {
			continue
		}

		// Fallback: scan the new file content for consecutive matches
		resolveFromFileContent(d, cm)
	}

	return result
}

// ResolveComment attempts to resolve StartLine/EndLine for a single comment
// by matching ExistingCode against the diff. Returns true on success.
func ResolveComment(cm *model.LlmComment, d *model.Diff) bool {
	if cm.StartLine > 0 || cm.EndLine > 0 {
		return true
	}
	if cm.ExistingCode == "" {
		return false
	}
	if resolveFromHunk(d, cm) {
		return true
	}
	return resolveFromFileContent(d, cm)
}

// RelocateAcrossFiles handles the comment whose ExistingCode belongs to a
// different file than the one it was filed against.
//
// The reviewing Agent reads related files through file_read_diff, so it can
// describe code from a file other than the one under review and still file the
// comment against the file under review — typically a declaration/implementation
// split, where the comment lands on the header and its code lives in the source
// file. ResolveComment then fails, and the LLM re-location that follows is given
// only the wrong file's diff and a prompt that demands a code block back, so it
// answers with whatever token in that diff looks closest. That overwrites the
// one piece of evidence pointing at the real code, and the comment ends up
// looking located while pointing at an unrelated line.
//
// So this runs first, and without a model: ExistingCode is a verbatim excerpt,
// which makes finding its true home plain string matching over the diffs that
// are already in memory. On a unique hit the comment is re-filed — Path,
// StartLine and EndLine all move together — and it returns that path.
//
// Zero hits and multiple hits both decline, leaving cm untouched: the same
// boilerplate can legitimately appear in several files, and guessing between
// them would trade one wrong location for another. Callers should treat a
// false return as "still unlocated" rather than as an error.
//
// cm.Path is skipped because its own file has already been tried, and probing
// happens on a copy so a failed candidate cannot leave line numbers behind.
func RelocateAcrossFiles(cm *model.LlmComment, diffs []model.Diff) (string, bool) {
	if cm == nil || cm.ExistingCode == "" || len(diffs) == 0 {
		return "", false
	}

	type hit struct {
		path       string
		start, end int
	}
	var hits []hit

	for i := range diffs {
		d := &diffs[i]
		if d.NewPath == cm.Path || d.OldPath == cm.Path {
			continue
		}
		probe := *cm
		probe.StartLine, probe.EndLine = 0, 0
		if !ResolveComment(&probe, d) {
			continue
		}
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		hits = append(hits, hit{path: path, start: probe.StartLine, end: probe.EndLine})
		if len(hits) > 1 {
			// Ambiguous already; no verdict can come from looking further.
			return "", false
		}
	}

	if len(hits) != 1 {
		return "", false
	}
	cm.Path = hits[0].path
	cm.StartLine = hits[0].start
	cm.EndLine = hits[0].end
	return hits[0].path, true
}

// indexedLine pairs a normalized line with its absolute file line number.
type indexedLine struct {
	lineNum int
	content string
}

// resolveFromHunk tries to find startLine/endLine by matching ExistingCode
// against hunk lines. It tries the new-side first (context + added lines →
// new-file line numbers), then falls back to old-side (context + deleted →
// old-file line numbers).
func resolveFromHunk(d *model.Diff, cm *model.LlmComment) bool {
	hunks := ParseHunks(d.Diff)
	if len(hunks) == 0 {
		return false
	}

	targetLines := splitAndNormalize(cm.ExistingCode)
	if len(targetLines) == 0 {
		return false
	}

	for i := range hunks {
		newSide := extractSideLines(&hunks[i], true)
		if start, end, ok := matchConsecutive(newSide, targetLines); ok {
			cm.StartLine = start
			cm.EndLine = end
			return true
		}
	}

	for i := range hunks {
		oldSide := extractSideLines(&hunks[i], false)
		if start, end, ok := matchConsecutive(oldSide, targetLines); ok {
			cm.StartLine = start
			cm.EndLine = end
			return true
		}
	}

	return false
}

// extractSideLines extracts one side of the diff from a hunk.
// When newSide is true, returns context+added lines with new-file line numbers.
// When newSide is false, returns context+deleted lines with old-file line numbers.
func extractSideLines(hunk *Hunk, newSide bool) []indexedLine {
	var result []indexedLine
	oldLine := hunk.OldStart
	newLine := hunk.NewStart

	for _, l := range hunk.Lines {
		switch l.Type {
		case HunkContext:
			if newSide {
				result = append(result, indexedLine{newLine, normalizeLine(l.Content)})
			} else {
				result = append(result, indexedLine{oldLine, normalizeLine(l.Content)})
			}
			oldLine++
			newLine++
		case HunkAdded:
			if newSide {
				result = append(result, indexedLine{newLine, normalizeLine(l.Content)})
			}
			newLine++
		case HunkDeleted:
			if !newSide {
				result = append(result, indexedLine{oldLine, normalizeLine(l.Content)})
			}
			oldLine++
		}
	}
	return result
}

// matchConsecutive scans sideLines for a consecutive run matching all targetLines.
func matchConsecutive(sideLines []indexedLine, targetLines []string) (startLine, endLine int, found bool) {
	if len(targetLines) == 0 || len(sideLines) < len(targetLines) {
		return 0, 0, false
	}
	for i := 0; i <= len(sideLines)-len(targetLines); i++ {
		matched := true
		for j, target := range targetLines {
			if sideLines[i+j].content != target {
				matched = false
				break
			}
		}
		if matched {
			return sideLines[i].lineNum, sideLines[i+len(targetLines)-1].lineNum, true
		}
	}
	return 0, 0, false
}

// resolveFromFileContent scans the new file content line-by-line for consecutive
// matches of the normalized existing_code.
func resolveFromFileContent(d *model.Diff, cm *model.LlmComment) bool {
	if d.NewFileContent == "" {
		return false
	}

	fileLines := strings.Split(d.NewFileContent, "\n")
	targetLines := splitAndNormalize(cm.ExistingCode)
	if len(targetLines) == 0 {
		return false
	}

	// Normalize file lines the same way as target: skip blanks so that
	// blank lines in the source don't break the sliding-window match.
	// "Consecutive" here means adjacent non-blank lines.
	normalizedFileLines := make([]string, 0, len(fileLines))
	fileLineNums := make([]int, 0, len(fileLines))
	for i, line := range fileLines {
		n := normalizeLine(strings.TrimRight(line, "\r"))
		if n == "" {
			continue
		}
		normalizedFileLines = append(normalizedFileLines, n)
		fileLineNums = append(fileLineNums, i+1)
	}

	if len(normalizedFileLines) < len(targetLines) {
		return false
	}

	for i := 0; i <= len(normalizedFileLines)-len(targetLines); i++ {
		matched := true
		for j, target := range targetLines {
			if normalizedFileLines[i+j] != target {
				matched = false
				break
			}
		}
		if matched {
			cm.StartLine = fileLineNums[i]
			cm.EndLine = fileLineNums[i+len(targetLines)-1]
			return true
		}
	}

	return false
}

// splitAndNormalize splits code text into lines and normalizes each one.
func splitAndNormalize(code string) []string {
	raw := strings.Split(code, "\n")
	result := make([]string, 0, len(raw))
	for _, line := range raw {
		n := normalizeLine(line)
		if n == "" {
			continue
		}
		result = append(result, n)
	}
	return result
}

// normalizeLine removes leading/trailing whitespace and strips any leading
// '+' or '-' diff marker.
func normalizeLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "+")
	s = strings.TrimPrefix(s, "-")
	return strings.TrimSpace(s)
}

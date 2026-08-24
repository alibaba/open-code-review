// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"strconv"
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

		candidates := ResolveCommentCandidates(cm, d)
		if len(candidates) == 1 {
			ApplyCandidate(cm, candidates[0])
		}
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

// CommentLocationCandidate is one place where ExistingCode matched.
type CommentLocationCandidate struct {
	ID        string
	Path      string
	StartLine int
	EndLine   int
	Snippet   string
	Context   string
}

// ResolveCommentCandidates returns locations that match cm.ExistingCode in d,
// using ResolveComment's existing search order while keeping ambiguity visible:
// new-side hunks, old-side hunks, then full new-file content.
func ResolveCommentCandidates(cm *model.LlmComment, d *model.Diff) []CommentLocationCandidate {
	return assignCandidateIDs(commentCandidates(cm, d))
}

func commentCandidates(cm *model.LlmComment, d *model.Diff) []CommentLocationCandidate {
	if cm == nil || d == nil || cm.ExistingCode == "" {
		return nil
	}
	if cm.StartLine > 0 || cm.EndLine > 0 {
		return []CommentLocationCandidate{{
			Path:      candidatePath(d),
			StartLine: cm.StartLine,
			EndLine:   cm.EndLine,
			Snippet:   cm.ExistingCode,
		}}
	}

	targetLines := splitAndNormalize(cm.ExistingCode)
	if len(targetLines) == 0 {
		return nil
	}

	hunks := ParseHunks(d.Diff)
	if candidates := collectHunkCandidates(hunks, targetLines, d, true); len(candidates) > 0 {
		return candidates
	}

	if candidates := collectHunkCandidates(hunks, targetLines, d, false); len(candidates) > 0 {
		return candidates
	}

	return fileContentCandidates(d, targetLines)
}

// ApplyCandidate mutates cm to point at c.
func ApplyCandidate(cm *model.LlmComment, c CommentLocationCandidate) {
	if cm == nil {
		return
	}
	if c.Path != "" {
		cm.Path = c.Path
	}
	cm.StartLine = c.StartLine
	cm.EndLine = c.EndLine
}

// RelocationCandidates returns candidate anchors across the reviewed diff set.
func RelocationCandidates(cm *model.LlmComment, diffs []model.Diff) []CommentLocationCandidate {
	if cm == nil || cm.ExistingCode == "" || len(diffs) == 0 {
		return nil
	}
	var candidates []CommentLocationCandidate
	for i := range diffs {
		d := &diffs[i]
		probe := *cm
		probe.StartLine, probe.EndLine = 0, 0
		candidates = append(candidates, commentCandidates(&probe, d)...)
	}
	return assignCandidateIDs(candidates)
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

func candidatePath(d *model.Diff) string {
	if d == nil {
		return ""
	}
	if d.NewPath != "" && d.NewPath != "/dev/null" {
		return d.NewPath
	}
	if d.OldPath != "" && d.OldPath != "/dev/null" {
		return d.OldPath
	}
	return ""
}

func collectHunkCandidates(hunks []Hunk, targetLines []string, d *model.Diff, newSide bool) []CommentLocationCandidate {
	var candidates []CommentLocationCandidate
	for i := range hunks {
		sideLines := extractSideLines(&hunks[i], newSide)
		candidates = append(candidates, candidatesFromIndexedLines(sideLines, targetLines, d)...)
	}
	return candidates
}

func candidatesFromIndexedLines(sideLines []indexedLine, targetLines []string, d *model.Diff) []CommentLocationCandidate {
	if len(targetLines) == 0 || len(sideLines) < len(targetLines) {
		return nil
	}

	var candidates []CommentLocationCandidate
	for i := 0; i <= len(sideLines)-len(targetLines); i++ {
		if !indexedLinesMatch(sideLines[i:], targetLines) {
			continue
		}
		contextStart := max(0, i-3)
		contextEnd := min(len(sideLines), i+len(targetLines)+3)
		candidates = append(candidates, CommentLocationCandidate{
			Path:      candidatePath(d),
			StartLine: sideLines[i].lineNum,
			EndLine:   sideLines[i+len(targetLines)-1].lineNum,
			Snippet:   snippetFromIndexedLines(sideLines[i : i+len(targetLines)]),
			Context:   snippetFromIndexedLines(sideLines[contextStart:contextEnd]),
		})
	}
	return candidates
}

func indexedLinesMatch(sideLines []indexedLine, targetLines []string) bool {
	if len(sideLines) < len(targetLines) {
		return false
	}
	for i, target := range targetLines {
		if sideLines[i].content != target {
			return false
		}
	}
	return true
}

func snippetFromIndexedLines(lines []indexedLine) string {
	if len(lines) == 0 {
		return ""
	}
	parts := make([]string, 0, len(lines))
	for _, l := range lines {
		parts = append(parts, l.content)
	}
	return strings.Join(parts, "\n")
}

func assignCandidateIDs(candidates []CommentLocationCandidate) []CommentLocationCandidate {
	for i := range candidates {
		candidates[i].ID = candidateID(i)
	}
	return candidates
}

func candidateID(i int) string {
	return strconv.Itoa(i + 1)
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

func fileContentCandidates(d *model.Diff, targetLines []string) []CommentLocationCandidate {
	if d == nil || d.NewFileContent == "" || len(targetLines) == 0 {
		return nil
	}

	fileLines := strings.Split(d.NewFileContent, "\n")
	sideLines := make([]indexedLine, 0, len(fileLines))
	for i, line := range fileLines {
		n := normalizeLine(strings.TrimRight(line, "\r"))
		if n == "" {
			continue
		}
		sideLines = append(sideLines, indexedLine{lineNum: i + 1, content: n})
	}

	return candidatesFromIndexedLines(sideLines, targetLines, d)
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

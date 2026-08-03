package diff

import (
	"strings"

	"github.com/alibaba/open-code-review/internal/hashline"
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

		// Fast path: verified hashline anchor (O(1), unambiguous).
		if resolveFromAnchor(d, cm) {
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
	if resolveFromAnchor(d, cm) {
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

// resolveFromAnchor resolves the comment position from a hashline anchor
// ("12#KT" or "12#KT-18#MQ") verified against the new file content.
//
// Two-factor validation, following the hashline protocol:
//   - The hash is the checksum: both endpoint anchors must verify against the
//     current 3-line context window in NewFileContent.
//   - ExistingCode, when present, acts as a text hint that can veto a
//     hash collision: the first non-blank line of existing_code must appear
//     somewhere within the anchored range (normalized comparison).
func resolveFromAnchor(d *model.Diff, cm *model.LlmComment) bool {
	if cm.Anchor == "" || d.NewFileContent == "" {
		return false
	}
	start, end, ok := hashline.ResolveSpec(cm.Anchor, d.NewFileContent)
	if !ok {
		return false
	}

	// textHint veto: if existing_code is present, its first significant line
	// must be found inside the anchored range.
	if hint := firstSignificantLine(cm.ExistingCode); hint != "" {
		fileLines := strings.Split(d.NewFileContent, "\n")
		found := false
		for ln := start; ln <= end && ln <= len(fileLines); ln++ {
			if normalizeLine(fileLines[ln-1]) == hint {
				found = true
				break
			}
		}
		if !found {
			cm.LocMethod = "anchor_hint_veto"
			return false
		}
	}

	cm.StartLine = start
	cm.EndLine = end
	cm.LocMethod = "anchor"
	return true
}

// firstSignificantLine returns the first normalized non-blank line of code.
func firstSignificantLine(code string) string {
	for _, line := range strings.Split(code, "\n") {
		if n := normalizeLine(line); n != "" {
			return n
		}
	}
	return ""
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
			cm.LocMethod = "hunk"
			return true
		}
	}

	for i := range hunks {
		oldSide := extractSideLines(&hunks[i], false)
		if start, end, ok := matchConsecutive(oldSide, targetLines); ok {
			cm.StartLine = start
			cm.EndLine = end
			cm.LocMethod = "hunk"
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
			cm.LocMethod = "file"
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

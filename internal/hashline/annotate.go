package hashline

import (
	"strconv"
	"strings"

	"github.com/alibaba/open-code-review/internal/model"
)

// AnnotateDiff renders the unified diff of d with hashline anchors on every
// line that exists in the new file (added and context lines). Deleted lines
// and hunk/file headers are passed through unchanged.
//
// Anchor hashes are computed against d.NewFileContent (the authoritative
// post-change file), so an anchor copied from the annotated diff verifies
// against the same file content used by ResolveSpec.
//
// Output line format for new-side lines:
//
//	12#KT:+added line content
//	13#MQ: context line content
//
// If NewFileContent is empty or a hunk's new-side line numbers fall outside
// the file (malformed diff), the original diff text is returned unmodified.
func AnnotateDiff(d *model.Diff) string {
	if d == nil || d.Diff == "" || d.NewFileContent == "" {
		if d == nil {
			return ""
		}
		return d.Diff
	}
	newLines := strings.Split(d.NewFileContent, "\n")

	var out strings.Builder
	lines := strings.Split(d.Diff, "\n")
	newLine := 0 // 1-based new-file line of the *next* new-side hunk line
	inHunk := false

	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		if m := hunkHeaderRe.FindStringSubmatch(line); m != nil {
			newLine, _ = strconv.Atoi(m[1])
			inHunk = true
			out.WriteString(line)
			continue
		}
		if !inHunk || line == "" || strings.HasPrefix(line, "\\") ||
			strings.HasPrefix(line, "diff --git ") {
			if strings.HasPrefix(line, "diff --git ") {
				inHunk = false
			}
			out.WriteString(line)
			continue
		}
		switch line[0] {
		case '+', ' ':
			idx := newLine - 1
			// Only annotate when the diff line content actually matches the
			// new file content at that position; otherwise pass through
			// (defensive against malformed diffs / stale file content).
			if idx >= 0 && idx < len(newLines) &&
				NormalizeHashInput(line[1:]) == NormalizeHashInput(newLines[idx]) {
				out.WriteString(FormatAnchor(newLines, idx))
				out.WriteByte(':')
			}
			out.WriteString(line)
			newLine++
		case '-':
			out.WriteString(line)
		default:
			// Header-ish line inside hunk region (shouldn't happen) — pass through.
			out.WriteString(line)
		}
	}
	return out.String()
}

var hunkHeaderRe = mustHunkRe()

func mustHunkRe() *hunkRe { return &hunkRe{} }

// hunkRe is a tiny allocation-free matcher for "@@ -a[,b] +c[,d] @@" headers
// that extracts the new-file start line. Using a hand-rolled matcher avoids a
// regexp dependency in the hot path.
type hunkRe struct{}

// FindStringSubmatch mimics regexp: returns nil or [full, newStart].
func (h *hunkRe) FindStringSubmatch(line string) []string {
	if !strings.HasPrefix(line, "@@ -") {
		return nil
	}
	plus := strings.Index(line, " +")
	if plus < 0 {
		return nil
	}
	rest := line[plus+2:]
	end := strings.IndexAny(rest, ", @")
	if end < 0 {
		return nil
	}
	numStr := rest[:end]
	if numStr == "" {
		return nil
	}
	for _, c := range numStr {
		if c < '0' || c > '9' {
			return nil
		}
	}
	return []string{line, numStr}
}

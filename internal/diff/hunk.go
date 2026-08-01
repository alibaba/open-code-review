package diff

import (
	"regexp"
	"strconv"
	"strings"
)

// HunkLineType represents the type of a line in a diff hunk.
type HunkLineType int

const (
	HunkContext HunkLineType = iota // ' ' prefix: unchanged context line
	HunkAdded                       // '+' prefix: added line
	HunkDeleted                     // '-' prefix: removed line
)

// HunkLine is a single line within a hunk.
type HunkLine struct {
	Type    HunkLineType
	Content string // content without the leading +/-/ marker
}

// Hunk represents one @@ ... @@ block in a unified diff.
type Hunk struct {
	OldStart int        // starting line in the old file (1-indexed)
	OldCount int        // number of lines in the old file
	NewStart int        // starting line in the new file (1-indexed)
	NewCount int        // number of lines in the new file
	Lines    []HunkLine // all lines in sequence
}

var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// ParseHunks parses raw unified diff text for a single file into a slice of Hunks.
// Lines before the first @@ header (file-level headers like "diff --git", "---", "+++") are ignored.
func ParseHunks(rawDiffText string) []Hunk {
	lines := strings.Split(rawDiffText, "\n")
	var hunks []Hunk
	var current *Hunk

	for _, line := range lines {
		if m := hunkHeaderRe.FindStringSubmatch(line); m != nil {
			// Flush previous hunk
			if current != nil {
				hunks = append(hunks, *current)
			}
			oldStart, _ := strconv.Atoi(m[1])
			oldCount := 1
			if m[2] != "" {
				oldCount, _ = strconv.Atoi(m[2])
			}
			newStart, _ := strconv.Atoi(m[3])
			newCount := 1
			if m[4] != "" {
				newCount, _ = strconv.Atoi(m[4])
			}
			current = &Hunk{
				OldStart: oldStart,
				OldCount: oldCount,
				NewStart: newStart,
				NewCount: newCount,
			}
			continue
		}

		if current == nil {
			continue // skip file-level headers and preamble
		}

		// Skip metadata lines that can appear inside hunks
		if strings.HasPrefix(line, "\\ No newline at end of file") {
			continue
		}
		// Stop processing if we hit another file's diff header
		if strings.HasPrefix(line, "diff --git ") {
			break
		}

		switch {
		case strings.HasPrefix(line, "+"):
			current.Lines = append(current.Lines, HunkLine{
				Type:    HunkAdded,
				Content: line[1:],
			})
		case strings.HasPrefix(line, "-"):
			current.Lines = append(current.Lines, HunkLine{
				Type:    HunkDeleted,
				Content: line[1:],
			})
		default:
			// Context line (' ' prefix) or other — treat as context
			content := line
			if len(content) > 0 && content[0] == ' ' {
				content = content[1:]
			}
			current.Lines = append(current.Lines, HunkLine{
				Type:    HunkContext,
				Content: content,
			})
		}
	}

	// Flush last hunk
	if current != nil {
		hunks = append(hunks, *current)
	}

	return hunks
}

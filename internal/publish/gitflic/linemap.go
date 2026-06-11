package gitflic

import "github.com/open-code-review/open-code-review/internal/diff"

// oldLineFor maps a line number in the new version of a file to the
// corresponding line in the old version, using the file's diff hunks.
// Lines added by the diff have no old counterpart, so they are anchored to
// the closest preceding old line — GitFlic only needs a plausible old-side
// position to render the code comment next to the insertion point.
// The returned value is always >= 1.
func oldLineFor(hunks []diff.Hunk, newLine int) int {
	delta := 0 // cumulative (new - old) line count shift from preceding hunks
	for _, h := range hunks {
		if newLine < h.NewStart {
			break
		}
		if newLine < h.NewStart+h.NewCount {
			return oldLineInHunk(h, newLine)
		}
		delta += h.NewCount - h.OldCount
	}
	return clampLine(newLine - delta)
}

// oldLineInHunk walks the hunk lines tracking both line counters until it
// reaches newLine.
func oldLineInHunk(h diff.Hunk, newLine int) int {
	oldLn, newLn := h.OldStart, h.NewStart
	lastOld := h.OldStart - 1 // last old line seen before the current position
	for _, l := range h.Lines {
		switch l.Type {
		case diff.HunkContext:
			if newLn == newLine {
				return clampLine(oldLn)
			}
			lastOld = oldLn
			oldLn++
			newLn++
		case diff.HunkDeleted:
			lastOld = oldLn
			oldLn++
		case diff.HunkAdded:
			if newLn == newLine {
				return clampLine(lastOld)
			}
			newLn++
		}
	}
	return clampLine(lastOld)
}

func clampLine(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

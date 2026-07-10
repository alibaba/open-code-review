package diff

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/open-code-review/open-code-review/internal/model"
)

// rawHunk is a single @@ block captured verbatim from a unified diff.
type rawHunk struct {
	header                                 string   // exact "@@ -a,b +c,d @@[ ctx]" line
	oldStart, oldCount, newStart, newCount int      // parsed header ranges
	body                                   []string // lines after the header, verbatim
}

// text renders the hunk back to unified-diff text (header + body).
func (h rawHunk) text() string {
	if len(h.body) == 0 {
		return h.header
	}
	return h.header + "\n" + strings.Join(h.body, "\n")
}

// selectionFile is one file section parsed from a caller-supplied selection patch.
type selectionFile struct {
	oldPath     string
	newPath     string
	isBinary    bool
	isSubmodule bool
	hunks       []rawHunk
}

// displayPath returns the most meaningful path for error messages.
func (sf selectionFile) displayPath() string {
	if sf.newPath != "" && sf.newPath != "/dev/null" {
		return sf.newPath
	}
	return sf.oldPath
}

// ApplySelection narrows the canonical BASE..HEAD diff set down to only the
// hunks present in selectionPatch, which must be a standard unified git diff
// whose files and hunks are an exact subset of canonical.
//
// The returned diffs preserve each file's NewFileContent (read at the true
// --to/HEAD ref) and the original hunk headers, so downstream line resolution
// still yields true-HEAD line numbers. Only the per-file Diff text and the
// insertion/deletion counts are narrowed to the selected hunks, and files with
// no selected hunk are dropped entirely.
//
// It fails closed: any invented/renamed/mutated hunk, malformed patch, path
// traversal, mismatched header/ranges, ambiguous match, duplicate selection, or
// unsupported binary/submodule shape is rejected with an error rather than
// silently widening or narrowing scope.
func ApplySelection(canonical []model.Diff, selectionPatch string) ([]model.Diff, error) {
	if strings.TrimSpace(selectionPatch) == "" {
		return nil, fmt.Errorf("selected diff is empty")
	}

	blocks, err := splitSelectionFiles(selectionPatch)
	if err != nil {
		return nil, err
	}

	type canonEntry struct {
		diff     *model.Diff
		preamble string
		hunks    []rawHunk
	}
	entries := make([]*canonEntry, len(canonical))
	byNew := make(map[string]*canonEntry)
	byOld := make(map[string]*canonEntry)
	for i := range canonical {
		d := &canonical[i]
		pre, hunks := splitFileHunks(d.Diff)
		e := &canonEntry{diff: d, preamble: pre, hunks: hunks}
		entries[i] = e
		if d.NewPath != "" && d.NewPath != "/dev/null" {
			byNew[d.NewPath] = e
		}
		if d.OldPath != "" && d.OldPath != "/dev/null" {
			byOld[d.OldPath] = e
		}
	}

	// chosen[entry] = set of selected canonical hunk indices.
	chosen := make(map[*canonEntry]map[int]bool)

	for _, block := range blocks {
		sf, perr := parseSelectionFile(block)
		if perr != nil {
			return nil, perr
		}
		if verr := validateSelectionPath(sf.oldPath); verr != nil {
			return nil, verr
		}
		if verr := validateSelectionPath(sf.newPath); verr != nil {
			return nil, verr
		}

		var e *canonEntry
		if sf.newPath != "" && sf.newPath != "/dev/null" {
			e = byNew[sf.newPath]
		}
		if e == nil && sf.oldPath != "" && sf.oldPath != "/dev/null" {
			e = byOld[sf.oldPath]
		}
		if e == nil {
			return nil, fmt.Errorf("selected diff references %q which is not part of the canonical diff", sf.displayPath())
		}
		if !pathsMatch(sf, e.diff) {
			return nil, fmt.Errorf("selected diff path mismatch for %q: canonical change is %s -> %s",
				sf.displayPath(), e.diff.OldPath, e.diff.NewPath)
		}
		if e.diff.IsBinary || sf.isBinary {
			return nil, fmt.Errorf("selected diff includes binary file %q, which cannot be hunk-selected", sf.displayPath())
		}
		if sf.isSubmodule {
			return nil, fmt.Errorf("selected diff includes submodule change %q, which cannot be hunk-selected", sf.displayPath())
		}
		if len(sf.hunks) == 0 {
			return nil, fmt.Errorf("selected diff for %q contains no hunks", sf.displayPath())
		}

		set := chosen[e]
		if set == nil {
			set = make(map[int]bool)
			chosen[e] = set
		}
		for _, sh := range sf.hunks {
			idx, merr := matchCanonicalHunk(e.hunks, sh)
			if merr != nil {
				return nil, fmt.Errorf("selected diff for %q: %w", sf.displayPath(), merr)
			}
			if set[idx] {
				return nil, fmt.Errorf("selected diff for %q lists the same hunk (@@ -%d,%d +%d,%d @@) more than once",
					sf.displayPath(), sh.oldStart, sh.oldCount, sh.newStart, sh.newCount)
			}
			set[idx] = true
		}
	}

	var result []model.Diff
	for _, e := range entries {
		set := chosen[e]
		if len(set) == 0 {
			continue
		}
		nd := *e.diff

		parts := make([]string, 0, len(set)+1)
		if e.preamble != "" {
			parts = append(parts, e.preamble)
		}
		var insertions, deletions int64
		for i := range e.hunks {
			if !set[i] {
				continue
			}
			h := e.hunks[i]
			parts = append(parts, h.text())
			for _, bl := range h.body {
				if len(bl) == 0 {
					continue
				}
				switch bl[0] {
				case '+':
					insertions++
				case '-':
					deletions++
				}
			}
		}
		nd.Diff = strings.Join(parts, "\n")
		nd.Insertions = insertions
		nd.Deletions = deletions
		result = append(result, nd)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("selected diff matched no reviewable hunks")
	}
	return result, nil
}

// splitSelectionFiles splits a multi-file unified diff into per-file text
// blocks, each beginning with a "diff --git" header.
func splitSelectionFiles(patch string) ([]string, error) {
	patch = strings.TrimRight(patch, "\n")
	lines := strings.Split(patch, "\n")
	var blocks []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			blocks = append(blocks, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			cur = []string{line}
			continue
		}
		if len(cur) == 0 {
			if strings.TrimSpace(line) == "" {
				continue
			}
			return nil, fmt.Errorf("selected diff: unexpected content before first 'diff --git' header: %q", line)
		}
		cur = append(cur, line)
	}
	flush()
	if len(blocks) == 0 {
		return nil, fmt.Errorf("selected diff: no file sections found (expected a 'diff --git' header)")
	}
	return blocks, nil
}

// parseSelectionFile parses one "diff --git" file block into a selectionFile.
func parseSelectionFile(block string) (selectionFile, error) {
	var sf selectionFile
	lines := strings.Split(block, "\n")
	m := diffHeaderRe.FindStringSubmatch(lines[0])
	if m == nil {
		return sf, fmt.Errorf("selected diff: malformed file header: %q", lines[0])
	}
	sf.oldPath = m[1]
	sf.newPath = m[2]

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "rename from "):
			sf.oldPath = strings.TrimPrefix(line, "rename from ")
		case strings.HasPrefix(line, "rename to "):
			sf.newPath = strings.TrimPrefix(line, "rename to ")
		case line == "--- /dev/null":
			sf.oldPath = "/dev/null"
		case line == "+++ /dev/null":
			sf.newPath = "/dev/null"
		case binaryRe.MatchString(line):
			sf.isBinary = true
		case strings.HasPrefix(line, "Subproject commit "):
			sf.isSubmodule = true
		case strings.HasPrefix(line, "index ") && strings.Contains(line, " 160000"):
			sf.isSubmodule = true
		}
	}

	_, hunks := splitFileHunks(block)
	sf.hunks = hunks
	return sf, nil
}

// splitFileHunks splits a single file's unified diff into the preamble
// (everything before the first @@ header) and the sequence of raw hunks.
func splitFileHunks(fileDiff string) (preamble string, hunks []rawHunk) {
	lines := strings.Split(fileDiff, "\n")
	var pre []string
	var cur *rawHunk
	seenHunk := false
	flush := func() {
		if cur != nil {
			cur.body = trimTrailingEmpty(cur.body)
			hunks = append(hunks, *cur)
			cur = nil
		}
	}
	for _, line := range lines {
		if seenHunk && strings.HasPrefix(line, "diff --git ") {
			break // start of the next file
		}
		if hm := hunkHeaderRe.FindStringSubmatch(line); hm != nil {
			flush()
			seenHunk = true
			oldStart, _ := strconv.Atoi(hm[1])
			oldCount := 1
			if hm[2] != "" {
				oldCount, _ = strconv.Atoi(hm[2])
			}
			newStart, _ := strconv.Atoi(hm[3])
			newCount := 1
			if hm[4] != "" {
				newCount, _ = strconv.Atoi(hm[4])
			}
			cur = &rawHunk{
				header:   line,
				oldStart: oldStart,
				oldCount: oldCount,
				newStart: newStart,
				newCount: newCount,
			}
			continue
		}
		if cur == nil {
			pre = append(pre, line)
			continue
		}
		cur.body = append(cur.body, line)
	}
	flush()
	return strings.Join(pre, "\n"), hunks
}

// matchCanonicalHunk finds the unique canonical hunk that exactly equals sel
// (same four header ranges and identical body). It fails closed on no match,
// a header-only match (mutated/partial body), or multiple matches (ambiguous).
func matchCanonicalHunk(canon []rawHunk, sel rawHunk) (int, error) {
	matches := make([]int, 0, 1)
	for i, ch := range canon {
		if ch.oldStart == sel.oldStart && ch.oldCount == sel.oldCount &&
			ch.newStart == sel.newStart && ch.newCount == sel.newCount &&
			equalBody(ch.body, sel.body) {
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		for _, ch := range canon {
			if ch.oldStart == sel.oldStart && ch.newStart == sel.newStart {
				return -1, fmt.Errorf("hunk @@ -%d,%d +%d,%d @@ does not match the canonical hunk content (modified or partial hunk)",
					sel.oldStart, sel.oldCount, sel.newStart, sel.newCount)
			}
		}
		return -1, fmt.Errorf("hunk @@ -%d,%d +%d,%d @@ is not present in the canonical diff",
			sel.oldStart, sel.oldCount, sel.newStart, sel.newCount)
	default:
		return -1, fmt.Errorf("hunk @@ -%d,%d +%d,%d @@ matches multiple canonical hunks (ambiguous)",
			sel.oldStart, sel.oldCount, sel.newStart, sel.newCount)
	}
}

// equalBody reports whether two hunk bodies are byte-for-byte identical.
func equalBody(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// trimTrailingEmpty drops trailing empty strings, which only ever arise from a
// trailing newline in the source text (never from a real diff line, which
// always carries a leading '+', '-', ' ' or '\' marker).
func trimTrailingEmpty(lines []string) []string {
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// validateSelectionPath rejects absolute paths and '..' traversal segments.
func validateSelectionPath(p string) error {
	if p == "" || p == "/dev/null" {
		return nil
	}
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("selected diff path %q must be repository-relative, not absolute", p)
	}
	if p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, "/../") || strings.HasSuffix(p, "/..") {
		return fmt.Errorf("selected diff path %q contains a path traversal ('..') segment", p)
	}
	return nil
}

// pathsMatch verifies the selection's declared old/new paths agree with the
// canonical change, guarding against path swapping or rename mismatch.
func pathsMatch(sf selectionFile, d *model.Diff) bool {
	if sf.newPath != "" && sf.newPath != "/dev/null" && d.NewPath != "/dev/null" {
		if sf.newPath != d.NewPath {
			return false
		}
	}
	if sf.oldPath != "" && sf.oldPath != "/dev/null" && d.OldPath != "/dev/null" {
		if sf.oldPath != d.OldPath {
			return false
		}
	}
	return true
}

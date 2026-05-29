package diff

import (
	"strings"

	"github.com/open-code-review/open-code-review/internal/model"
)

// MergeDiffs merges multiple slices of model.Diff (one per commit) into a single
// combined diff set. Files that appear in multiple commits have their diff text
// concatenated and their insertions/deletions summed.
func MergeDiffs(diffSets ...[]model.Diff) []model.Diff {
	if len(diffSets) == 0 {
		return nil
	}
	if len(diffSets) == 1 {
		return diffSets[0]
	}

	// Collect diffs by NewPath, preserving order of first appearance.
	type entry struct {
		diff       model.Diff
		diffTexts  []string
		seenCount  int
	}
	fileOrder := make([]string, 0)
	fileMap := make(map[string]*entry)

	for _, diffs := range diffSets {
		for _, d := range diffs {
			key := d.NewPath
			if key == "/dev/null" {
				key = d.OldPath
			}
			if e, ok := fileMap[key]; ok {
				e.diffTexts = append(e.diffTexts, d.Diff)
				e.diff.Insertions += d.Insertions
				e.diff.Deletions += d.Deletions
				e.seenCount++
				// Merge flags: if any commit marks file as new/deleted, propagate.
				if d.IsNew {
					e.diff.IsNew = true
				}
				if d.IsDeleted {
					e.diff.IsDeleted = true
				}
				if d.IsBinary {
					e.diff.IsBinary = true
				}
			} else {
				fileOrder = append(fileOrder, key)
				e := &entry{
					diff:      d,
					diffTexts: []string{d.Diff},
					seenCount: 1,
				}
				fileMap[key] = e
			}
		}
	}

	result := make([]model.Diff, 0, len(fileOrder))
	for _, key := range fileOrder {
		e := fileMap[key]
		if e.seenCount > 1 {
			e.diff.Diff = strings.Join(e.diffTexts, "\n\n")
		}
		result = append(result, e.diff)
	}

	return result
}

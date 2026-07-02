package agent

import (
	"context"
	"fmt"

	allowedext "github.com/open-code-review/open-code-review/internal/config/allowlist"
	"github.com/open-code-review/open-code-review/internal/model"
)

// ExcludeReason / DiffPreview / DiffPreviewEntry are now type aliases of
// the mode-agnostic preview types in internal/model. Kept for backwards
// compatibility with existing call sites; internal/scan returns the same
// model.Preview shape directly.
type ExcludeReason = model.ExcludeReason
type DiffPreview = model.Preview
type DiffPreviewEntry = model.PreviewEntry

// Re-export the constants so callers can keep writing agent.ExcludeBinary.
const (
	ExcludeNone        = model.ExcludeNone
	ExcludeUserRule    = model.ExcludeUserRule
	ExcludeExtension   = model.ExcludeExtension
	ExcludeDefaultPath = model.ExcludeDefaultPath
	ExcludeDeleted     = model.ExcludeDeleted
	ExcludeBinary      = model.ExcludeBinary
)

// whyExcluded applies the filter algorithm as shouldReview but
// returns the specific reason a file is excluded.
func (a *Agent) whyExcluded(d model.Diff) ExcludeReason {
	if d.IsBinary {
		return ExcludeBinary
	}

	path := effectivePath(d)
	f := a.args.FileFilter

	if f != nil && f.IsUserExcluded(path) {
		return ExcludeUserRule
	}

	if f != nil && f.HasInclude() && f.IsUserIncluded(path) {
		return ExcludeNone
	}

	ext := a.extFromPath(path)
	if ext != "" && !allowedext.IsAllowedExt(ext) {
		return ExcludeExtension
	}

	if allowedext.IsExcludedPath(path) {
		return ExcludeDefaultPath
	}

	return ExcludeNone
}

// Preview loads diffs and applies the filter algorithm, returning structured
// preview data without dispatching any LLM calls.
func (a *Agent) Preview(ctx context.Context) (*DiffPreview, error) {
	if err := a.loadDiffs(ctx); err != nil {
		return nil, fmt.Errorf("load diffs: %w", err)
	}

	result := &DiffPreview{
		TotalInsertions: a.totalInsertions,
		TotalDeletions:  a.totalDeletions,
		TotalFiles:      len(a.diffs),
	}

	for _, d := range a.diffs {
		path := effectivePath(d)
		entry := DiffPreviewEntry{
			Path:       path,
			Insertions: d.Insertions,
			Deletions:  d.Deletions,
			Status:     diffStatus(d),
		}

		reason := a.whyExcluded(d)
		if reason == ExcludeNone && d.IsDeleted {
			reason = ExcludeDeleted
		}

		entry.WillReview = reason == ExcludeNone
		entry.ExcludeReason = reason

		if entry.WillReview {
			result.ReviewableCount++
		} else {
			result.ExcludedCount++
		}

		result.Entries = append(result.Entries, entry)
	}

	return result, nil
}

func effectivePath(d model.Diff) string {
	if d.NewPath == "/dev/null" {
		return d.OldPath
	}
	return d.NewPath
}

func diffStatus(d model.Diff) string {
	switch {
	case d.IsBinary:
		return "binary"
	case d.IsNew:
		return "added"
	case d.IsDeleted:
		return "deleted"
	case d.IsRenamed:
		return "renamed"
	case d.OldPath != d.NewPath && d.OldPath != "" && d.OldPath != "/dev/null":
		return "renamed"
	default:
		return "modified"
	}
}

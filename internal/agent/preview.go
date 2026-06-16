package agent

import (
	"context"
	"fmt"

	allowedext "github.com/open-code-review/open-code-review/internal/config/allowlist"
	"github.com/open-code-review/open-code-review/internal/model"
)

// ExcludeReason describes why a file was excluded from review.
type ExcludeReason string

const (
	ExcludeNone        ExcludeReason = ""
	ExcludeUserRule    ExcludeReason = "user_exclude"
	ExcludeExtension   ExcludeReason = "unsupported_ext"
	ExcludeDefaultPath ExcludeReason = "default_path"
	ExcludeDeleted     ExcludeReason = "deleted"
	ExcludeBinary      ExcludeReason = "binary"
)

// DiffPreviewEntry is one file's preview record.
type DiffPreviewEntry struct {
	Path          string        `json:"path"`
	Status        string        `json:"status"`
	Insertions    int64         `json:"insertions"`
	Deletions     int64         `json:"deletions"`
	WillReview    bool          `json:"will_review"`
	ExcludeReason ExcludeReason `json:"exclude_reason,omitempty"`
}

// DiffPreview is the full preview result.
type DiffPreview struct {
	Entries         []DiffPreviewEntry `json:"files"`
	TotalInsertions int64              `json:"total_insertions"`
	TotalDeletions  int64              `json:"total_deletions"`
	TotalFiles      int                `json:"total_files"`
	ReviewableCount int                `json:"reviewable_count"`
	ExcludedCount   int                `json:"excluded_count"`
}

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

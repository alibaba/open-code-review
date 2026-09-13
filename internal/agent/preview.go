// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"fmt"

	"github.com/alibaba/open-code-review/internal/model"
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
	ExcludeTooLarge    = model.ExcludeTooLarge
)

// Preview loads diffs and applies the same selection the real run applies
// before dispatch, returning structured preview data without dispatching any
// LLM calls. Given the run's Args.Template, its will_review set is the set a
// fresh, non-resumed, unbudgeted run registers as selected coverage; a zero
// Template.MaxTokens leaves the per-file size ceiling disabled, exactly as it
// is for a run configured that way.
//
// It builds none of the review runtime — no session, manifest, or runner — so
// previewing cannot open session persistence. Going through New instead would
// auto-create a session and leave an unfinalized JSONL file under the OCR home.
func Preview(ctx context.Context, args Args) (*DiffPreview, error) {
	return (&Agent{args: args}).preview(ctx)
}

func (a *Agent) preview(ctx context.Context) (*DiffPreview, error) {
	if err := a.loadDiffs(ctx); err != nil {
		return nil, fmt.Errorf("load diffs: %w", err)
	}

	result := &DiffPreview{
		TotalInsertions: a.totalInsertions,
		TotalDeletions:  a.totalDeletions,
		TotalFiles:      len(a.diffs),
		// Non-nil so an empty diff marshals as `"files":[]`, not `"files":null`.
		Entries: make([]DiffPreviewEntry, 0, len(a.diffs)),
	}

	for _, dec := range a.selectFiles(a.diffs) {
		d := dec.Diff
		entry := DiffPreviewEntry{
			Path:          effectivePath(d),
			Insertions:    d.Insertions,
			Deletions:     d.Deletions,
			Status:        diffStatus(d),
			WillReview:    dec.selected(),
			ExcludeReason: dec.Reason,
		}

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

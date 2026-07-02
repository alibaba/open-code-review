package scan

import (
	"context"
	"fmt"

	"github.com/open-code-review/open-code-review/internal/model"
)

// Preview enumerates files and applies the standard reviewability filter
// without dispatching any LLM calls. Returns a *model.Preview ready for
// cmd/opencodereview.outputPreviewText to render.
//
// Preview is read-only with respect to the Agent: it does not mutate
// a.items. (Earlier versions did, which made a subsequent Run on the same
// Agent silently observe the preview's enumeration instead of re-running
// it.) Callers that want to reuse the enumeration should call Run once.
func (a *Agent) Preview(ctx context.Context) (*model.Preview, error) {
	provider := NewProvider(a.args.RepoDir, a.args.Paths, a.args.GitRunner, a.args.MaxFileSizeBytes)
	items, err := provider.Enumerate(ctx)
	if err != nil {
		return nil, fmt.Errorf("enumerate files: %w", err)
	}

	// Pre-allocate Entries to a non-nil empty slice so JSON marshalling
	// emits `"files":[]` rather than `"files":null` when there is nothing
	// to review — important for downstream API consumers expecting an array.
	result := &model.Preview{
		TotalFiles: len(items),
		Entries:    make([]model.PreviewEntry, 0, len(items)),
	}

	for _, it := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry := model.PreviewEntry{
			Path:       it.Path,
			Status:     "scan",
			Insertions: int64(it.LineCount),
		}
		reason := a.whyExcluded(it)
		entry.WillReview = reason == model.ExcludeNone
		entry.ExcludeReason = reason
		if entry.WillReview {
			result.ReviewableCount++
			result.TotalInsertions += entry.Insertions
		} else {
			result.ExcludedCount++
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}

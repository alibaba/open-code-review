package scan

import (
	"context"
	"fmt"

	"github.com/open-code-review/open-code-review/internal/model"
)

// Preview enumerates files and applies the standard reviewability filter
// without dispatching any LLM calls. Returns a *model.Preview ready for
// cmd/opencodereview.outputPreviewText to render.
func (a *Agent) Preview(ctx context.Context) (*model.Preview, error) {
	provider := NewProvider(a.args.RepoDir, a.args.Paths, a.args.GitRunner)
	items, err := provider.Enumerate(ctx)
	if err != nil {
		return nil, fmt.Errorf("enumerate files: %w", err)
	}
	a.items = items

	result := &model.Preview{
		TotalFiles: len(a.items),
	}

	for _, it := range a.items {
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

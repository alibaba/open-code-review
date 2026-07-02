package scan

import (
	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/reviewfilter"
)

// ExcludeReason applies the native full-scan reviewability order.
func ExcludeReason(item model.ScanItem, fileFilter *rules.FileFilter) model.ExcludeReason {
	return reviewfilter.Filter{FileFilter: fileFilter}.ExcludeReason(model.Diff{
		NewPath:  item.Path,
		IsBinary: item.IsBinary,
	})
}

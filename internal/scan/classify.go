package scan

import (
	"strings"

	allowedext "github.com/open-code-review/open-code-review/internal/config/allowlist"
	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/model"
)

// ExcludeReason applies the native full-scan reviewability order.
func ExcludeReason(item model.ScanItem, fileFilter *rules.FileFilter) model.ExcludeReason {
	if item.IsBinary {
		return model.ExcludeBinary
	}
	if fileFilter != nil && fileFilter.IsUserExcluded(item.Path) {
		return model.ExcludeUserRule
	}
	if fileFilter != nil && fileFilter.HasInclude() && fileFilter.IsUserIncluded(item.Path) {
		return model.ExcludeNone
	}
	if fileFilter != nil && fileFilter.HasInclude() {
		return model.ExcludeUserRule
	}
	extension := scanExtension(item.Path)
	if extension != "" && !allowedext.IsAllowedExt(extension) {
		return model.ExcludeExtension
	}
	if allowedext.IsExcludedPath(item.Path) {
		return model.ExcludeDefaultPath
	}
	return model.ExcludeNone
}

func scanExtension(path string) string {
	baseName := path
	if index := strings.LastIndex(path, "/"); index >= 0 {
		baseName = path[index+1:]
	}
	dot := strings.LastIndex(baseName, ".")
	if dot <= 0 {
		return ""
	}
	return strings.ToLower(baseName[dot:])
}

// extFromPath is retained for native package compatibility.
func extFromPath(path string) string {
	return scanExtension(path)
}

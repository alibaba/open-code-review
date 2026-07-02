// Package reviewfilter classifies diff entries using OCR's deterministic
// include, exclude, extension, and path policies.
package reviewfilter

import (
	"strings"

	allowedext "github.com/open-code-review/open-code-review/internal/config/allowlist"
	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/model"
)

// Filter applies the user-configured and built-in review filters.
type Filter struct {
	FileFilter *rules.FileFilter
}

// ExcludeReason returns why a change is excluded, or model.ExcludeNone.
func (f Filter) ExcludeReason(change model.Diff) model.ExcludeReason {
	if change.IsBinary {
		return model.ExcludeBinary
	}

	path := EffectivePath(change)
	if f.FileFilter != nil && f.FileFilter.IsUserExcluded(path) {
		return model.ExcludeUserRule
	}
	if f.FileFilter != nil && f.FileFilter.HasInclude() && f.FileFilter.IsUserIncluded(path) {
		return model.ExcludeNone
	}
	if f.FileFilter != nil && f.FileFilter.HasInclude() {
		return model.ExcludeUserRule
	}

	extension := extensionFromPath(path)
	if extension != "" && !allowedext.IsAllowedExt(extension) {
		return model.ExcludeExtension
	}
	if allowedext.IsExcludedPath(path) {
		return model.ExcludeDefaultPath
	}
	return model.ExcludeNone
}

// EffectivePath returns the path to display and filter for a change.
func EffectivePath(change model.Diff) string {
	if change.NewPath == "/dev/null" {
		return change.OldPath
	}
	return change.NewPath
}

// Status returns the stable protocol status for a change.
func Status(change model.Diff) string {
	switch {
	case change.IsBinary:
		return "binary"
	case change.IsNew:
		return "added"
	case change.IsDeleted:
		return "deleted"
	case change.IsRenamed:
		return "renamed"
	case change.OldPath != change.NewPath && change.OldPath != "" && change.OldPath != "/dev/null":
		return "renamed"
	default:
		return "modified"
	}
}

func extensionFromPath(path string) string {
	basename := path
	if index := strings.LastIndex(path, "/"); index >= 0 {
		basename = path[index+1:]
	}
	dot := strings.LastIndex(basename, ".")
	if dot <= 0 {
		return ""
	}
	return strings.ToLower(basename[dot:])
}

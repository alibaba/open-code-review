package reviewfilter

import (
	"testing"

	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/model"
)

func TestExcludeReasonPreservesNativeOrdering(t *testing.T) {
	tests := []struct {
		name       string
		fileFilter *rules.FileFilter
		change     model.Diff
		want       model.ExcludeReason
	}{
		{
			name:   "binary takes precedence",
			change: model.Diff{NewPath: "main.go", IsBinary: true},
			want:   model.ExcludeBinary,
		},
		{
			name:       "user exclude",
			fileFilter: &rules.FileFilter{Exclude: []string{"generated/**"}},
			change:     model.Diff{NewPath: "generated/main.go"},
			want:       model.ExcludeUserRule,
		},
		{
			name:       "explicit include overrides extension allowlist",
			fileFilter: &rules.FileFilter{Include: []string{"docs/**"}},
			change:     model.Diff{NewPath: "docs/notes.unsupported"},
			want:       model.ExcludeNone,
		},
		{
			name:       "include list excludes non-matching files",
			fileFilter: &rules.FileFilter{Include: []string{"docs/**"}},
			change:     model.Diff{NewPath: "src/main.go"},
			want:       model.ExcludeUserRule,
		},
		{
			name:   "unsupported extension",
			change: model.Diff{NewPath: "notes.unsupported"},
			want:   model.ExcludeExtension,
		},
		{
			name:   "default excluded path",
			change: model.Diff{NewPath: "internal/main_test.go"},
			want:   model.ExcludeDefaultPath,
		},
		{
			name:   "reviewable",
			change: model.Diff{NewPath: "internal/main.go"},
			want:   model.ExcludeNone,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filter := Filter{FileFilter: test.fileFilter}
			if got := filter.ExcludeReason(test.change); got != test.want {
				t.Fatalf("ExcludeReason() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestEffectivePathUsesOldPathForDeletion(t *testing.T) {
	change := model.Diff{OldPath: "internal/old.go", NewPath: "/dev/null", IsDeleted: true}
	if got := EffectivePath(change); got != "internal/old.go" {
		t.Fatalf("EffectivePath() = %q, want internal/old.go", got)
	}
}

func TestStatusClassifiesNativeDiffStates(t *testing.T) {
	tests := []struct {
		name   string
		change model.Diff
		want   string
	}{
		{name: "binary", change: model.Diff{IsBinary: true}, want: "binary"},
		{name: "added", change: model.Diff{IsNew: true}, want: "added"},
		{name: "deleted", change: model.Diff{IsDeleted: true}, want: "deleted"},
		{name: "renamed flag", change: model.Diff{IsRenamed: true}, want: "renamed"},
		{
			name:   "renamed paths",
			change: model.Diff{OldPath: "old.go", NewPath: "new.go"},
			want:   "renamed",
		},
		{name: "modified", change: model.Diff{OldPath: "main.go", NewPath: "main.go"}, want: "modified"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Status(test.change); got != test.want {
				t.Fatalf("Status() = %q, want %q", got, test.want)
			}
		})
	}
}

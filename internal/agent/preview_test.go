package agent

import (
	"testing"

	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/model"
)

func TestWhyExcluded_BinaryFile(t *testing.T) {
	agent := New(Args{})
	tests := []struct {
		name     string
		diff     model.Diff
		expected ExcludeReason
	}{
		{
			name: "binary file returns ExcludeBinary",
			diff: model.Diff{
				NewPath:  "image.png",
				IsBinary: true,
			},
			expected: ExcludeBinary,
		},
		{
			name: "non-binary go file returns ExcludeNone",
			diff: model.Diff{
				NewPath: "main.go",
			},
			expected: ExcludeNone,
		},
		{
			name: "binary file with valid extension still excluded",
			diff: model.Diff{
				NewPath:  "document.pdf",
				IsBinary: true,
			},
			expected: ExcludeBinary,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agent.whyExcluded(tt.diff)
			if got != tt.expected {
				t.Errorf("whyExcluded() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWhyExcluded_UserExcludePattern(t *testing.T) {
	agent := New(Args{
		FileFilter: &rules.FileFilter{
			Exclude: []string{"vendor/**", "*.gen.go"},
		},
	})

	tests := []struct {
		name     string
		diff     model.Diff
		expected ExcludeReason
	}{
		{
			name: "file matching exclude pattern",
			diff: model.Diff{
				NewPath: "vendor/foo/bar.go",
			},
			expected: ExcludeUserRule,
		},
		{
			name: "generated file excluded",
			diff: model.Diff{
				NewPath: "api.gen.go",
			},
			expected: ExcludeUserRule,
		},
		{
			name: "regular file not excluded",
			diff: model.Diff{
				NewPath: "main.go",
			},
			expected: ExcludeNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agent.whyExcluded(tt.diff)
			if got != tt.expected {
				t.Errorf("whyExcluded() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWhyExcluded_ExtensionFilter(t *testing.T) {
	agent := New(Args{})

	tests := []struct {
		name     string
		diff     model.Diff
		expected ExcludeReason
	}{
		{
			name: "unsupported extension txt",
			diff: model.Diff{
				NewPath: "README.txt",
			},
			expected: ExcludeExtension,
		},
		{
			name: "unsupported extension md",
			diff: model.Diff{
				NewPath: "docs/guide.md",
			},
			expected: ExcludeExtension,
		},
		{
			name: "supported extension go",
			diff: model.Diff{
				NewPath: "main.go",
			},
			expected: ExcludeNone,
		},
		{
			name: "supported extension java",
			diff: model.Diff{
				NewPath: "src/Main.java",
			},
			expected: ExcludeNone,
		},
		{
			name: "supported extension ts",
			diff: model.Diff{
				NewPath: "app.ts",
			},
			expected: ExcludeNone,
		},
		{
			name: "file without extension",
			diff: model.Diff{
				NewPath: "Makefile",
			},
			expected: ExcludeNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agent.whyExcluded(tt.diff)
			if got != tt.expected {
				t.Errorf("whyExcluded() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWhyExcluded_DefaultPathFilter(t *testing.T) {
	agent := New(Args{})

	tests := []struct {
		name     string
		diff     model.Diff
		expected ExcludeReason
	}{
		{
			name: "test file excluded by default path",
			diff: model.Diff{
				NewPath: "foo_test.go",
			},
			expected: ExcludeDefaultPath,
		},
		{
			name: "java test file excluded",
			diff: model.Diff{
				NewPath: "src/test/java/com/example/FooTest.java",
			},
			expected: ExcludeDefaultPath,
		},
		{
			name: "regular source file not excluded",
			diff: model.Diff{
				NewPath: "src/main/java/com/example/Foo.java",
			},
			expected: ExcludeNone,
		},
		{
			name: "go source file not excluded",
			diff: model.Diff{
				NewPath: "handler.go",
			},
			expected: ExcludeNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agent.whyExcluded(tt.diff)
			if got != tt.expected {
				t.Errorf("whyExcluded() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWhyExcluded_UserIncludePattern(t *testing.T) {
	agent := New(Args{
		FileFilter: &rules.FileFilter{
			Include: []string{"src/**/*.go", "pkg/**/*.go", "**/*.supportedext"},
		},
	})

	tests := []struct {
		name     string
		diff     model.Diff
		expected ExcludeReason
	}{
		// --- Files matching include patterns bypass default-path checks ---
		{
			name: "file matching first include pattern is reviewed",
			diff: model.Diff{
				NewPath: "src/foo/bar.go",
			},
			expected: ExcludeNone,
		},
		{
			name: "file matching second include pattern is reviewed",
			diff: model.Diff{
				NewPath: "pkg/util/helper.go",
			},
			expected: ExcludeNone,
		},
		{
			name: "include pattern bypasses default-path exclusion for test files",
			diff: model.Diff{
				NewPath: "src/foo/bar_test.go",
			},
			// IMPORTANT: Even though *_test.go is excluded by IsExcludedPath,
			// matching an include pattern returns ExcludeNone before that check.
			expected: ExcludeNone,
		},
		// --- Include is additive, NOT exclusive ---
		// When include patterns are configured, files that do NOT match them
		// still fall through to the default checks. If extension is valid and
		// path is not default-excluded, they are still reviewed.
		{
			name: "non-included file with valid extension still reviewed (additive semantics)",
			diff: model.Diff{
				NewPath: "vendor/baz.go",
			},
			// .go is a supported extension and vendor/baz.go does not hit
			// IsExcludedPath, so it falls through to ExcludeNone.
			expected: ExcludeNone,
		},
		{
			name: "non-included file in non-excluded directory still reviewed",
			diff: model.Diff{
				NewPath: "internal/handler.go",
			},
			expected: ExcludeNone,
		},
		{
			name: "include check overrides extension exclusion",
			diff: model.Diff{
				NewPath: "internal/test.supportedext",
			},
			expected: ExcludeNone,
		},
		{
			name: "unsupported extension even if path looks like include dir",
			diff: model.Diff{
				NewPath: "src/notes.txt",
			},
			expected: ExcludeExtension,
		},
		// --- Default-path exclusion still applies to non-included files ---
		{
			name: "non-included test file excluded by default path",
			diff: model.Diff{
				NewPath: "internal/handler_test.go",
			},
			// Does not match include patterns, falls through.
			// IsExcludedPath matches *_test.go → ExcludeDefaultPath.
			expected: ExcludeDefaultPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agent.whyExcluded(tt.diff)
			if got != tt.expected {
				t.Errorf("whyExcluded() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestWhyExcluded_IncludeBypassesDefaultPath verifies that a file matching
// an include pattern is reviewed even when it would normally be excluded by
// the default-path filter (e.g. test files, generated code patterns).
func TestWhyExcluded_IncludeBypassesDefaultPath(t *testing.T) {
	agent := New(Args{
		FileFilter: &rules.FileFilter{
			Include: []string{"**/*_test.go"},
		},
	})

	tests := []struct {
		name     string
		diff     model.Diff
		expected ExcludeReason
	}{
		{
			name: "test file explicitly included overrides default-path exclusion",
			diff: model.Diff{
				NewPath: "foo_test.go",
			},
			expected: ExcludeNone,
		},
		{
			name: "non-test file still reviewed via default checks",
			diff: model.Diff{
				NewPath: "main.go",
			},
			expected: ExcludeNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agent.whyExcluded(tt.diff)
			if got != tt.expected {
				t.Errorf("whyExcluded() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestWhyExcluded_IncludeAndExcludeInteraction verifies priority: user
// exclude patterns take precedence over include patterns.
func TestWhyExcluded_IncludeAndExcludeInteraction(t *testing.T) {
	agent := New(Args{
		FileFilter: &rules.FileFilter{
			Include: []string{"src/**/*.go"},
			Exclude: []string{"src/generated/**"},
		},
	})

	tests := []struct {
		name     string
		diff     model.Diff
		expected ExcludeReason
	}{
		{
			name: "included file is reviewed",
			diff: model.Diff{
				NewPath: "src/handler.go",
			},
			expected: ExcludeNone,
		},
		{
			name: "file matching both include and exclude is excluded (exclude wins)",
			diff: model.Diff{
				NewPath: "src/generated/api.go",
			},
			expected: ExcludeUserRule,
		},
		{
			name: "file outside include with valid ext still reviewed (additive)",
			diff: model.Diff{
				NewPath: "lib/utils.go",
			},
			expected: ExcludeNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agent.whyExcluded(tt.diff)
			if got != tt.expected {
				t.Errorf("whyExcluded() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestWhyExcluded_PriorityOrder(t *testing.T) {
	agent := New(Args{
		FileFilter: &rules.FileFilter{
			Exclude: []string{"vendor/**"},
		},
	})

	// Binary check should happen first, even if excluded by user pattern
	diff := model.Diff{
		NewPath:  "vendor/image.png",
		IsBinary: true,
	}

	got := agent.whyExcluded(diff)
	if got != ExcludeBinary {
		t.Errorf("whyExcluded() = %v, want %v (binary should be checked first)", got, ExcludeBinary)
	}
}

func TestShouldReview(t *testing.T) {
	agent := New(Args{})

	tests := []struct {
		name     string
		diff     model.Diff
		expected bool
	}{
		{
			name: "binary file should not be reviewed",
			diff: model.Diff{
				NewPath:  "image.png",
				IsBinary: true,
			},
			expected: false,
		},
		{
			name: "regular go file should be reviewed",
			diff: model.Diff{
				NewPath: "main.go",
			},
			expected: true,
		},
		{
			name: "test file should not be reviewed",
			diff: model.Diff{
				NewPath: "main_test.go",
			},
			expected: false,
		},
		{
			name: "unsupported extension should not be reviewed",
			diff: model.Diff{
				NewPath: "README.md",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agent.shouldReview(tt.diff)
			if got != tt.expected {
				t.Errorf("shouldReview() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEffectivePath(t *testing.T) {
	tests := []struct {
		name     string
		diff     model.Diff
		expected string
	}{
		{
			name: "normal new path",
			diff: model.Diff{
				OldPath: "old.go",
				NewPath: "new.go",
			},
			expected: "new.go",
		},
		{
			name: "new path is dev/null (deleted file)",
			diff: model.Diff{
				OldPath: "deleted.go",
				NewPath: "/dev/null",
			},
			expected: "deleted.go",
		},
		{
			name: "renamed file uses new path",
			diff: model.Diff{
				OldPath: "old_name.go",
				NewPath: "new_name.go",
			},
			expected: "new_name.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectivePath(tt.diff)
			if got != tt.expected {
				t.Errorf("effectivePath() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDiffStatus(t *testing.T) {
	tests := []struct {
		name     string
		diff     model.Diff
		expected string
	}{
		{
			name: "binary file",
			diff: model.Diff{
				IsBinary: true,
			},
			expected: "binary",
		},
		{
			name: "new file",
			diff: model.Diff{
				IsNew: true,
			},
			expected: "added",
		},
		{
			name: "deleted file",
			diff: model.Diff{
				IsDeleted: true,
			},
			expected: "deleted",
		},
		{
			name: "renamed file",
			diff: model.Diff{
				OldPath: "old.go",
				NewPath: "new.go",
			},
			expected: "renamed",
		},
		{
			name: "modified file",
			diff: model.Diff{
				OldPath: "main.go",
				NewPath: "main.go",
			},
			expected: "modified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffStatus(tt.diff)
			if got != tt.expected {
				t.Errorf("diffStatus() = %v, want %v", got, tt.expected)
			}
		})
	}
}

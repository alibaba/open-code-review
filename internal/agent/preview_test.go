// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/model"
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
		{
			name: "rust integration test excluded by default path",
			diff: model.Diff{
				NewPath: "tests/integration.rs",
			},
			expected: ExcludeDefaultPath,
		},
		{
			name: "rust tests.rs module excluded by default path",
			diff: model.Diff{
				NewPath: "crates/core/src/tests.rs",
			},
			expected: ExcludeDefaultPath,
		},
		{
			name: "rust _tests.rs file excluded by default path",
			diff: model.Diff{
				NewPath: "crates/core/src/parser_tests.rs",
			},
			expected: ExcludeDefaultPath,
		},
		{
			name: "rust benchmark file excluded by default path",
			diff: model.Diff{
				NewPath: "benches/throughput.rs",
			},
			expected: ExcludeDefaultPath,
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
				// Was vendor/baz.go, chosen only as a path outside the include
				// patterns that nothing else excluded. #1494 added **/vendor/**
				// to the default list, so it needs a path that is still only
				// non-included and not excluded on any other ground.
				NewPath: "cmd/baz.go",
			},
			// .go is a supported extension and cmd/baz.go does not hit
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

// Credential paths are excluded before either user rule is consulted, so
// neither an include nor an exclude can change the outcome or the reason.
func TestWhyExcluded_SecretPath(t *testing.T) {
	tests := []struct {
		name     string
		filter   *rules.FileFilter
		path     string
		expected ExcludeReason
	}{
		{
			name:     "secret path excluded with no user config",
			path:     ".env",
			expected: ExcludeSecret,
		},
		{
			name:     "nested secret path excluded",
			path:     "foo/bar/.env",
			expected: ExcludeSecret,
		},
		{
			name:     "extensionless key excluded",
			path:     "id_rsa",
			expected: ExcludeSecret,
		},
		{
			name:     "ssh directory contents excluded",
			path:     "foo/.ssh/id_ed25519",
			expected: ExcludeSecret,
		},
		{
			name:     "include cannot override a secret path",
			filter:   &rules.FileFilter{Include: []string{"**/.env"}},
			path:     ".env",
			expected: ExcludeSecret,
		},
		{
			name:     "user exclude does not change the reason",
			filter:   &rules.FileFilter{Exclude: []string{"**/.env"}},
			path:     ".env",
			expected: ExcludeSecret,
		},
		{
			name:     "env template with explicit include stays reviewable",
			filter:   &rules.FileFilter{Include: []string{"**/.env.example"}},
			path:     ".env.example",
			expected: ExcludeNone,
		},
		{
			// Still not reviewable, but for the pre-existing extension reason.
			name:     "env template without include keeps unsupported_ext",
			path:     ".env.example",
			expected: ExcludeExtension,
		},
		{
			name:     "public key is not a secret path",
			path:     "id_rsa.pub",
			expected: ExcludeExtension,
		},
		{
			name:     "dockerfile unchanged",
			path:     "Dockerfile",
			expected: ExcludeNone,
		},
		{
			name:     "makefile unchanged",
			path:     "Makefile",
			expected: ExcludeNone,
		},
		{
			name:     "ordinary unsupported extension unchanged",
			path:     "src/notes.txt",
			expected: ExcludeExtension,
		},
		{
			name:     "ordinary user exclude still reports user_exclude",
			filter:   &rules.FileFilter{Exclude: []string{"vendor/**"}},
			path:     "vendor/foo/bar.go",
			expected: ExcludeUserRule,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := New(Args{FileFilter: tt.filter})
			got := agent.whyExcluded(model.Diff{NewPath: tt.path})
			if got != tt.expected {
				t.Errorf("whyExcluded(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestWhyExcluded_SecretRename(t *testing.T) {
	agent := New(Args{
		FileFilter: &rules.FileFilter{
			Include: []string{"**/.env.example"},
		},
	})

	diff := model.Diff{
		OldPath: ".env",
		NewPath: ".env.example",
	}

	if got := agent.whyExcluded(diff); got != ExcludeSecret {
		t.Fatalf("whyExcluded() = %q, want %q", got, ExcludeSecret)
	}
}

// TestSelectFilesDeletionGate covers the one gate selectFiles adds over the
// static ones: a deletion is never selected, however reviewable its path looks.
// The static gates themselves are covered by the whyExcluded tables above.
func TestSelectFilesDeletionGate(t *testing.T) {
	agent := New(Args{})

	diffs := []model.Diff{
		{OldPath: "main.go", NewPath: "main.go"},
		{OldPath: "gone.go", NewPath: "/dev/null", IsDeleted: true},
	}

	decisions := agent.selectFiles(diffs)
	if !decisions[0].selected() {
		t.Errorf("main.go reason = %q, want selected", decisions[0].Reason)
	}
	if decisions[1].selected() || decisions[1].Reason != ExcludeDeleted {
		t.Errorf("gone.go = (selected=%v, reason=%q), want (false, %q)",
			decisions[1].selected(), decisions[1].Reason, ExcludeDeleted)
	}
}

func TestSelectFiles_Starlark(t *testing.T) {
	agent := New(Args{})
	for _, path := range []string{
		"BUILD", "src/library/BUILD", "WORKSPACE", "legacy/WORKSPACE",
		"src/library/BUILD.bazel", "legacy/WORKSPACE.bazel", "MODULE.bazel",
		"rules/library.bzl", "config.star",
	} {
		t.Run(path, func(t *testing.T) {
			decisions := agent.selectFiles([]model.Diff{{NewPath: path}})
			if !decisions[0].selected() {
				t.Errorf("selectFiles(%q) excluded file: %s", path, decisions[0].Reason)
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

// gitDiff returns the diff git produces between two versions of a file, so
// the generated-file tests run against real hunks.
func gitDiff(t *testing.T, oldContent, newContent string) string {
	t.Helper()
	dir := t.TempDir()
	oldPath, newPath := filepath.Join(dir, "old"), filepath.Join(dir, "new")
	if err := os.WriteFile(oldPath, []byte(oldContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte(newContent), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "diff", "--no-index", "--no-color", oldPath, newPath).Output()
	var exitErr *exec.ExitError
	if err != nil && !(errors.As(err, &exitErr) && exitErr.ExitCode() == 1) {
		t.Fatalf("git diff --no-index: %v", err)
	}
	return string(out)
}

func modifiedDiff(t *testing.T, path, oldContent, newContent string) model.Diff {
	return model.Diff{OldPath: path, NewPath: path, NewFileContent: newContent, Diff: gitDiff(t, oldContent, newContent)}
}

func TestWhyExcluded_GeneratedContent(t *testing.T) {
	const (
		deepcopyV1 = "// Code generated by deepcopy-gen. DO NOT EDIT.\n\npackage v1\n\nfunc (in *A) DeepCopy() *A { out := new(A); out.X = in.X; return out }\n"
		deepcopyV2 = "// Code generated by deepcopy-gen. DO NOT EDIT.\n\npackage v1\n\nfunc (in *A) DeepCopy() *A { out := new(A); out.Y = in.Y; return out }\n"
		handAuth   = "package auth\n\nfunc Allowed(u string) bool { return u == \"admin\" }\n"
		markedAuth = "// Code generated by example. DO NOT EDIT.\npackage auth\n\nfunc Allowed(u string) bool { return true }\n"
	)

	tests := []struct {
		name     string
		filter   *rules.FileFilter
		diff     func(t *testing.T) model.Diff
		expected ExcludeReason
	}{
		{
			name: "existing generated file stays generated",
			diff: func(t *testing.T) model.Diff {
				return modifiedDiff(t, "api/v1/zz_generated.deepcopy.go", deepcopyV1, deepcopyV2)
			},
			expected: ExcludeGenerated,
		},
		{
			name: "generator rewrites its own header",
			diff: func(t *testing.T) model.Diff {
				return modifiedDiff(t, "api/v1/zz_generated.deepcopy.go",
					"// Code generated by controller-gen v0.15. DO NOT EDIT.\npackage v1\n",
					"// Code generated by controller-gen v0.16. DO NOT EDIT.\npackage v1\n")
			},
			expected: ExcludeGenerated,
		},
		{
			name: "marker prepended to a hand-written file",
			diff: func(t *testing.T) model.Diff {
				return modifiedDiff(t, "auth/auth.go", handAuth, markedAuth)
			},
			expected: ExcludeNone,
		},
		{
			name: "marker prepended while one is removed from the body",
			diff: func(t *testing.T) model.Diff {
				return modifiedDiff(t, "auth/auth.go",
					"package auth\n\nconst ExampleMarker = \"@generated\"\n\nfunc Allowed(u string) bool { return u == \"admin\" }\n",
					"// @generated\npackage auth\n\nfunc Allowed(u string) bool { return true }\n")
			},
			expected: ExcludeNone,
		},
		{
			name: "code moved below an existing marker comment",
			diff: func(t *testing.T) model.Diff {
				return modifiedDiff(t, "auth/auth.go",
					"package auth\n\n// @generated\nfunc Allowed(u string) bool { return u == \"admin\" }\n",
					"// @generated\npackage auth\n\nfunc Allowed(u string) bool { return true }\n")
			},
			expected: ExcludeNone,
		},
		{
			name: "new file is reviewed even with a marker",
			diff: func(t *testing.T) model.Diff {
				return model.Diff{OldPath: "/dev/null", NewPath: "hook.go", IsNew: true, NewFileContent: markedAuth, Diff: gitDiff(t, "", markedAuth)}
			},
			expected: ExcludeNone,
		},
		{
			name: "renamed file is reviewed even with a marker",
			diff: func(t *testing.T) model.Diff {
				d := modifiedDiff(t, "hook.go", deepcopyV1, deepcopyV2)
				d.OldPath, d.IsRenamed = "api/v1/zz_generated.deepcopy.go", true
				return d
			},
			expected: ExcludeNone,
		},
		{
			name: "diff that does not fit the new content",
			diff: func(t *testing.T) model.Diff {
				d := modifiedDiff(t, "api/v1/zz_generated.deepcopy.go", deepcopyV1, deepcopyV2)
				d.NewFileContent = markedAuth
				return d
			},
			expected: ExcludeNone,
		},
		{
			name: "default path keeps its own reason",
			diff: func(t *testing.T) model.Diff {
				return modifiedDiff(t, "api/v1/types_test.go", deepcopyV1, deepcopyV2)
			},
			expected: ExcludeDefaultPath,
		},
		{
			name:   "include rule brings a generated file back",
			filter: &rules.FileFilter{Include: []string{"api/**/zz_generated.*.go"}},
			diff: func(t *testing.T) model.Diff {
				return modifiedDiff(t, "api/v1/zz_generated.deepcopy.go", deepcopyV1, deepcopyV2)
			},
			expected: ExcludeNone,
		},
		{
			name:   "user exclude wins over the generated reason",
			filter: &rules.FileFilter{Exclude: []string{"api/**"}},
			diff: func(t *testing.T) model.Diff {
				return modifiedDiff(t, "api/v1/zz_generated.deepcopy.go", deepcopyV1, deepcopyV2)
			},
			expected: ExcludeUserRule,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := New(Args{FileFilter: tt.filter})
			if got := agent.whyExcluded(tt.diff(t)); got != tt.expected {
				t.Errorf("whyExcluded() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSelectFiles_GeneratedContent(t *testing.T) {
	agent := New(Args{})
	decisions := agent.selectFiles([]model.Diff{
		modifiedDiff(t, "mocks/store.go",
			"// Code generated by MockGen. DO NOT EDIT.\npackage mocks\n\nvar V = 1\n",
			"// Code generated by MockGen. DO NOT EDIT.\npackage mocks\n\nvar V = 2\n"),
		modifiedDiff(t, "store.go", "package store\n\nvar V = 1\n", "package store\n\nvar V = 2\n"),
	})
	if got := decisions[0].Reason; got != ExcludeGenerated {
		t.Errorf("generated file reason = %q, want %q", got, ExcludeGenerated)
	}
	if decisions[0].selected() || decisions[0].retained() {
		t.Error("a generated file must leave the selected and retained sets")
	}
	if !decisions[1].selected() {
		t.Error("the hand-written file must stay selected")
	}
	if !ExcludeGenerated.IsExclusion() {
		t.Error("generated must count as an exclusion reason")
	}
}

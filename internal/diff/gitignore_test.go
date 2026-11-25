// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestExcludedDirs(t *testing.T) {
	dirs := ExcludedDirs()
	if len(dirs) == 0 {
		t.Fatal("ExcludedDirs should return non-empty list")
	}
	if !slices.Contains(dirs, ".git/") {
		t.Error("ExcludedDirs should include .git/")
	}

	for _, d := range dirs {
		if d == "pkgs/" {
			t.Error("ExcludedDirs must not include pkgs/")
		}
	}

	dirs2 := ExcludedDirs()
	dirs[0] = "MUTATED"
	if dirs2[0] == "MUTATED" {
		t.Error("ExcludedDirs should return a copy, not the original slice")
	}
}

func TestProviderDirPrefix(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"vendor/pkg/keep.go", "vendor/"},
		{"vendor", "vendor/"},
		{"target/.pnpm/pkg/index.js", "target/"},
		{"src/vendor/keep.go", ""},
		{"main.go", ""},
		{"pkgs/foo/default.nix", ""},
		{"pkgs", ""},
	}
	for _, tc := range tests {
		if got := ProviderDirPrefix(tc.path); got != tc.want {
			t.Errorf("ProviderDirPrefix(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestLoadGitignorePatterns(t *testing.T) {
	t.Run("valid gitignore", func(t *testing.T) {
		dir := t.TempDir()
		content := "*.log\n# comment\n\nnode_modules/\n*.tmp\n"
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		patterns := LoadGitignorePatterns(dir)
		want := []string{"*.log", "node_modules/", "*.tmp"}
		if len(patterns) != len(want) {
			t.Fatalf("got %d patterns %v, want %d %v", len(patterns), patterns, len(want), want)
		}
		for i := range want {
			if patterns[i] != want[i] {
				t.Errorf("patterns[%d] = %q, want %q", i, patterns[i], want[i])
			}
		}
	})

	t.Run("missing gitignore", func(t *testing.T) {
		dir := t.TempDir()
		patterns := LoadGitignorePatterns(dir)
		if patterns != nil {
			t.Errorf("expected nil for missing .gitignore, got %v", patterns)
		}
	})
}

func TestLoadGitignoreAtRef(t *testing.T) {
	repo := initBareRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("*.log\n out\nsrc\n!Src/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "commit", "--allow-empty", "-qm", "No ignore file")
	runGitTest(t, repo, "add", ".gitignore")
	runGitTest(t, repo, "commit", "-qm", "Regular ignore file")
	blob := gitOut(t, repo, "rev-parse", "HEAD:.gitignore")
	runGitTest(t, repo, "update-index", "--cacheinfo", "120000,"+blob+",.gitignore")
	runGitTest(t, repo, "commit", "-qm", "Symlink instead of rules")
	t.Setenv("GIT_GLOB_PATHSPECS", "1")
	for _, setting := range []string{"false", "true"} {
		runGitTest(t, repo, "config", "core.ignoreCase", setting)
		patterns, err := LoadGitignoreAtRef(t.Context(), repo, "HEAD~1", nil)
		if err != nil || !patterns.Matches("test.log") || patterns.Matches("out/main.go") ||
			!patterns.Matches(" out/main.go") || patterns.Matches("src/main.go") != (setting == "false") {
			t.Fatalf("regular .gitignore with core.ignoreCase=%s: patterns=%+v err=%v", setting, patterns, err)
		}
	}
	for _, ref := range []string{"HEAD~2", "HEAD"} {
		patterns, err := LoadGitignoreAtRef(t.Context(), repo, ref, nil)
		if err != nil || len(patterns.lines) != 0 {
			t.Fatalf("%s: patterns=%+v err=%v", ref, patterns, err)
		}
	}
	if _, err := LoadGitignoreAtRef(t.Context(), repo, "absent-revision", nil); err == nil {
		t.Fatal("missing revision treated as absent .gitignore")
	}
}

func TestIsPathExcluded(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		patterns []string
		want     bool
	}{
		{"hardcoded dir .git", ".git", nil, true},
		{"hardcoded dir prefix", ".git/config", nil, true},
		{"node_modules dir pattern", "node_modules/foo.js", []string{"node_modules/"}, true},
		{"gitignore pattern match", "debug.log", []string{"*.log"}, true},
		{"Git tree path separator", "out/main.go", []string{"/out/main.go"}, true},
		{"literal backslash in Git tree name", `out\main.go`, []string{"/out/main.go"}, false},
		{"escaped literal backslash", `out\main.go`, []string{`/out\\main.go`}, true},
		{"no match", "main.go", []string{"*.log"}, false},
		{"no patterns", "main.go", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPathExcluded(".", tt.relPath, tt.patterns)
			if got != tt.want {
				t.Errorf("IsPathExcluded(%q, %v) = %v, want %v", tt.relPath, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestIsPathExcluded_DirectoryPatterns(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		pattern string
		want    bool
	}{
		{"path pattern", "docs/generated/file.go", "docs/generated/", true},
		{"path pattern is root relative", "nested/docs/generated/file.go", "docs/generated/", false},
		{"globstar at root", "generated/file.go", "**/generated/", true},
		{"globstar nested", "src/generated/file.go", "**/generated/", true},
		{"component glob", "src/build-cache/file.go", "build*/", true},
		{"root anchored", "generated/file.go", "/generated/", true},
		{"root anchored does not match nested", "src/generated/file.go", "/generated/", false},
		{"file name is not a directory", "src/generated", "generated/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPathExcluded(".", tt.relPath, []string{tt.pattern})
			if got != tt.want {
				t.Errorf("IsPathExcluded(%q, %q) = %v, want %v",
					tt.relPath, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestIsPathExcluded_NegatedDirectoryPatterns(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		patterns []string
		want     bool
	}{
		{
			name:     "root directory re-included",
			relPath:  "src/a.go",
			patterns: []string{"/*/", "!/src/"},
			want:     false,
		},
		{
			name:     "re-including parent does not re-include ignored child",
			relPath:  "src/build/out.go",
			patterns: []string{"build/", "/*/", "!/src/"},
			want:     true,
		},
		{
			name:     "nested ignored directory stays excluded",
			relPath:  "ui/node_modules/x.js",
			patterns: []string{"node_modules/", "/*/", "!/ui/"},
			want:     true,
		},
		{
			name:     "slashless parent negation does not re-include child",
			relPath:  "src/vendor/lib.go",
			patterns: []string{"vendor/", "!src/"},
			want:     true,
		},
		{
			name:     "file-style ignored directory stays excluded",
			relPath:  "tools/build/x.go",
			patterns: []string{"build", "!tools/"},
			want:     true,
		},
		{
			name:     "relative directory re-included",
			relPath:  "src/a.go",
			patterns: []string{"src/", "!src/"},
			want:     false,
		},
		{
			name:     "negated directory does not re-include files",
			relPath:  "src/a.go",
			patterns: []string{"*", "!*/"},
			want:     true,
		},
		{
			name:     "file negation cannot bypass an excluded parent",
			relPath:  "src/a.go",
			patterns: []string{"src/", "!src/a.go"},
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPathExcluded(".", tt.relPath, tt.patterns)
			if got != tt.want {
				t.Errorf("IsPathExcluded(%q, %v) = %v, want %v", tt.relPath, tt.patterns, got, tt.want)
			}
		})
	}
}

// Expectations below follow git's own verdicts, verified with
// `git check-ignore --no-index`: a trailing
// /** ignores the prefix directory's contents but not the directory itself,
// and a slash-containing pattern stays relative to the .gitignore location
// even without a leading "/".
func TestIsPathExcluded_GlobstarAncestorRules(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		patterns []string
		want     bool
	}{
		{
			name:     "trailing globstar ignores the contents",
			relPath:  "src/a.go",
			patterns: []string{"src/**"},
			want:     true,
		},
		{
			name: "trailing globstar does not exclude the directory itself, " +
				"so a file negation can re-include",
			relPath:  "src/a.go",
			patterns: []string{"src/**", "!src/*.go"},
			want:     false,
		},
		{
			name: "file negation only reaches the globstar prefix's direct " +
				"children",
			relPath:  "src/sub/b.go",
			patterns: []string{"src/**", "!src/*.go"},
			want:     true,
		},
		{
			name:     "mid-pattern globstar with trailing globstar stays excluded",
			relPath:  "a/b/x.go",
			patterns: []string{"a/**/b/**"},
			want:     true,
		},
		{
			name: "trailing globstar after a mid-pattern globstar does not " +
				"exclude the inner directory itself",
			relPath:  "a/b/x.go",
			patterns: []string{"a/**/b/**", "!a/b/*.go"},
			want:     false,
		},
		{
			name:     "directory-only trailing globstar does not exclude the prefix",
			relPath:  "src/a.go",
			patterns: []string{"src/**/"},
			want:     false,
		},
		{
			name:     "directory-only trailing globstar ignores deeper directories",
			relPath:  "src/sub/x.go",
			patterns: []string{"src/**/"},
			want:     true,
		},
		{
			name:     "root-relative rule does not exclude an unrelated subtree",
			relPath:  "src/docs/generated/a.go",
			patterns: []string{"docs/generated"},
			want:     false,
		},
		{
			name:     "root-relative rule control at the repository root",
			relPath:  "docs/generated/a.go",
			patterns: []string{"docs/generated"},
			want:     true,
		},
		{
			name:     "anchored root-relative rule behaves the same",
			relPath:  "src/docs/generated/a.go",
			patterns: []string{"/docs/generated"},
			want:     false,
		},
		{
			name:     "anchored root-relative rule control",
			relPath:  "docs/generated/a.go",
			patterns: []string{"/docs/generated"},
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPathExcluded(".", tt.relPath, tt.patterns)
			if got != tt.want {
				t.Errorf("IsPathExcluded(%q, %v) = %v, want %v", tt.relPath, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestIsPathExcluded_AncestorPatternTargets(test *testing.T) {
	cases := []struct {
		name     string
		relPath  string
		patterns []string
		want     bool
	}{
		{
			name:     "slashless globstar negation matches nested basename",
			relPath:  "src/cache/a.go",
			patterns: []string{"cache", "!**cache"},
			want:     false,
		},
		{
			name:     "slashless globstar exclusion matches nested basename",
			relPath:  "src/cache-data/a.go",
			patterns: []string{"cache**"},
			want:     true,
		},
		{
			name:     "anchored globstar does not match nested basename",
			relPath:  "src/cache/a.go",
			patterns: []string{"cache", "!/**cache"},
			want:     true,
		},
		{
			name:     "anchored globstar negation matches root directory",
			relPath:  "cache/a.go",
			patterns: []string{"cache", "!/**cache"},
			want:     false,
		},
		{
			name:     "path globstar still matches full ancestor path",
			relPath:  "src/cache/a.go",
			patterns: []string{"**/cache"},
			want:     true,
		},
		{
			name:     "path wildcard does not cross directory separators",
			relPath:  "src/ok/nested/a.go",
			patterns: []string{"src/*", "!src/ok/", "!src/ok/nested/*.go"},
			want:     false,
		},
		{
			name:     "path wildcard still excludes sibling directory",
			relPath:  "src/other/a.go",
			patterns: []string{"src/*", "!src/ok/", "!src/ok/nested/*.go"},
			want:     true,
		},
		{
			name:     "anchored wildcard does not cross directory separators",
			relPath:  "src/inner/a.go",
			patterns: []string{"/*", "!/src/", "!*.go"},
			want:     false,
		},
		{
			name:     "question mark does not match a directory separator",
			relPath:  "src/ok/a.go",
			patterns: []string{"/src?ok"},
			want:     false,
		},
	}
	for _, testCase := range cases {
		test.Run(testCase.name, func(test *testing.T) {
			got := IsPathExcluded(".", testCase.relPath, testCase.patterns)
			if got != testCase.want {
				test.Errorf("IsPathExcluded(%q, %v) = %v, want %v", testCase.relPath, testCase.patterns, got, testCase.want)
			}
		})
	}
}

// allowListGitignore is the "ignore everything, then re-include" idiom from
// github/gitignore's Go.AllowList.gitignore. Repositories using it are the
// reason negation patterns cannot be discarded: the leading `*` matches every
// basename, so without honouring the `!` lines every file in the repository
// looks excluded and a review silently covers nothing.
var allowListGitignore = []string{
	"*",
	"!/.github/**/*",
	"!/.gitignore",
	"!/.tool-versions",
	"!/.golangci.yml",
	"!Taskfile.yml",
	"!*.go",
	"!go.sum",
	"!go.mod",
	"!README.md",
	"!LICENSE",
	"!scripts/*",
	"!*/",
}

func TestIsPathExcluded_AllowListGitignore(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		want    bool
	}{
		{"go file at root", "main.go", false},
		{"go file nested", "internal/diff/git.go", false},
		{"go test file nested", "internal/diff/git_test.go", false},
		{"go.mod", "go.mod", false},
		{"go.sum", "go.sum", false},
		{"readme", "README.md", false},
		{"license", "LICENSE", false},
		{"root-anchored dotfile", ".golangci.yml", false},
		{"root-anchored tool-versions", ".tool-versions", false},
		{"doublestar workflow", ".github/workflows/ci.yml", false},
		{"script by dir glob", "scripts/build.sh", false},
		{"taskfile", "Taskfile.yml", false},

		// Still excluded: nothing re-includes these. A negated directory-only
		// pattern (`!*/`) must not re-include a file, or the trailing entry
		// would readmit everything below the root.
		{"build artifact at root", "coverage.out", true},
		{"build artifact nested", "internal/diff/coverage.out", true},
		{"binary at root", "ocr", true},
		{"unrelated yaml nested", "internal/testdata/fixture.yaml", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPathExcluded(".", tt.relPath, allowListGitignore)
			if got != tt.want {
				t.Errorf("IsPathExcluded(%q) = %v, want %v", tt.relPath, got, tt.want)
			}
		})
	}
}

// TestIsPathExcluded_LastMatchWins pins ordering semantics: gitignore resolves
// a path by the LAST pattern that matches it, not the first.
func TestIsPathExcluded_LastMatchWins(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		patterns []string
		want     bool
	}{
		{"negation after exclusion re-includes", "important.log", []string{"*.log", "!important.log"}, false},
		{"exclusion after negation re-excludes", "important.log", []string{"!important.log", "*.log"}, true},
		{"negation of unmatched path is inert", "main.go", []string{"!important.log"}, false},
		{"hardcoded dirs are not negatable", ".git/config", []string{"!.git/config"}, true},
		{"blocklist still works", "debug.log", []string{"*.log"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPathExcluded(".", tt.relPath, tt.patterns)
			if got != tt.want {
				t.Errorf("IsPathExcluded(%q, %v) = %v, want %v", tt.relPath, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestMatchGitignorePattern(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		pattern string
		want    bool
	}{
		{"basename glob match", "src/debug.log", "*.log", true},
		{"basename glob no match", "src/main.go", "*.log", false},
		{"directory pattern", "vendor/pkg/file.go", "vendor/", true},
		{"directory pattern nested", "a/vendor/b", "vendor/", true},
		{"directory pattern no match", "vendor_extra/file.go", "vendor/", false},
		{"full path glob", "docs/api.md", "docs/*.md", true},
		{"full path no match", "src/api.md", "docs/*.md", false},
		{"negated class includes letter", "pkg/buildX", "build[!0-9]", true},
		{"negated class excludes digit", "pkg/build1", "build[!0-9]", false},
		{"anchored negated class cannot match slash", "pkg/foo/bar", "/pkg/foo[!0-9]bar", false},
		{"globstar negated class cannot match slash", "pkg/foo/bar", "**/foo[!0-9]bar", false},
		{"globstar negated class matches component", "pkg/sub/fooXbar", "**/foo[!0-9]bar", true},
		{"globstar retries a later directory", "pkg/a/sub/a/test.go", "pkg/**/a/*.go", true},
		{"named classes can share a bracket", "buildA", "build[[:digit:][:upper:]]", true},
		{"named punctuation includes brackets", "build]", "build[[:punct:]]", true},
		{"named punctuation excludes letters", "buildA", "build[[:punct:]]", false},
		{"unknown named class", "build7", "build[[:invalid:]]", false},
		{"escaped bracket before named class", "[7", `\[[[:digit:]]`, true},
		{"directory braces are literal", "src/a.go", "{src,lib}/", false},
		{"globstar with literal braces", "pkg/{src,lib}/a.go", "**/{src,lib}/*.go", true},
		{"globstar without brace expansion", "pkg/lib/a.go", "**/{src,lib}/*.go", false},
		{"escaped braces stay literal", "pkg/{a}/b.go", `**/\{a\}/`, true},
		{"braces in character class", "pkg/}/b.go", "**/[{}]/", true},
		{"negation pattern", "important.log", "!important.log", false},
		{"literal path is root relative", "src/generated/api.go", "generated/api.go", false},
		{"path suffix respects component boundary", "othersrc/main.go", "src/main.go", false},
		{"path suffix at root is not a suffix match", "src/main.go", "rc/main.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchGitignorePattern(tt.relPath, tt.pattern)
			if got != tt.want {
				t.Errorf("MatchGitignorePattern(%q, %q) = %v, want %v", tt.relPath, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestIsRangeMode(t *testing.T) {
	p := &Provider{mode: ModeRange}
	if !p.IsRangeMode() {
		t.Error("expected IsRangeMode() = true for ModeRange")
	}
	p.mode = ModeCommit
	if p.IsRangeMode() {
		t.Error("expected IsRangeMode() = false for ModeCommit")
	}
}

func TestIsCommitMode(t *testing.T) {
	p := &Provider{mode: ModeCommit}
	if !p.IsCommitMode() {
		t.Error("expected IsCommitMode() = true for ModeCommit")
	}
	p.mode = ModeWorkspace
	if p.IsCommitMode() {
		t.Error("expected IsCommitMode() = false for ModeWorkspace")
	}
}

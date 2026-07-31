package diff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExcludedDirs(t *testing.T) {
	dirs := ExcludedDirs()
	if len(dirs) == 0 {
		t.Fatal("ExcludedDirs should return non-empty list")
	}
	found := false
	for _, d := range dirs {
		if d == ".git/" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ExcludedDirs should include .git/")
	}

	dirs2 := ExcludedDirs()
	dirs[0] = "MUTATED"
	if dirs2[0] == "MUTATED" {
		t.Error("ExcludedDirs should return a copy, not the original slice")
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
		{"negation pattern", "important.log", "!important.log", false},
		{"path suffix match", "src/generated/api.go", "generated/api.go", true},
		// The suffix has to begin on a path component. "othersrc" ends in
		// "src", which would complete the pattern on a plain string suffix
		// check and exclude a directory git never matched.
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

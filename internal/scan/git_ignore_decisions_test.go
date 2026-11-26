// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestScanKeepsGitSelectedFiles(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		for _, limited := range []bool{false, true} {
			name := "nested_negation"
			if tracked {
				name = "tracked_after_ignore"
			}
			if limited {
				name += "/runner"
			}
			t.Run(name, func(t *testing.T) {
				repo := initTestRepo(t)
				writeFile(t, repo, "src/.gitignore", []byte("!keep.go\n"))
				writeFile(t, repo, "tracked.go", []byte("package tracked\n"))
				gitCommit(t, repo, "initial source")
				writeFile(t, repo, ".gitignore", []byte("*.go\n"))
				writeFile(t, repo, "src/keep.go", []byte("package keep\n"))
				writeFile(t, repo, "src/ignored.go", []byte("package ignored\n"))
				writeFile(t, repo, "vendor/.gitignore", []byte("!keep.go\n"))
				writeFile(t, repo, "vendor/keep.go", []byte("package vendor\n"))

				cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
				cmd.Dir = repo
				output, err := cmd.Output()
				if err != nil {
					t.Fatal(err)
				}
				gitPaths := strings.Split(string(output), "\x00")
				if !slices.Contains(gitPaths, "src/keep.go") || !slices.Contains(gitPaths, "tracked.go") || slices.Contains(gitPaths, "src/ignored.go") {
					t.Fatalf("fixture does not exercise Git's ignore decisions: %q", gitPaths)
				}
				var runner *gitcmd.Runner
				if limited {
					runner = gitcmd.New(1)
				}
				selected := "src/keep.go"
				if tracked {
					selected = "tracked.go"
				}
				items, err := NewProvider(repo, []string{selected}, runner, 0).Enumerate(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if got := itemPaths(items); !slices.Equal(got, []string{selected}) {
					t.Fatalf("Git-selected file disappeared from scan: got %v, want %s", got, selected)
				}
				if items[0].Content == "" || items[0].LineCount != 1 || items[0].IsBinary {
					t.Fatalf("source payload was not retained: %+v", items[0])
				}
				all, err := NewProvider(repo, nil, runner, 0).Enumerate(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				paths := itemPaths(all)
				if slices.Contains(paths, "src/ignored.go") || slices.Contains(paths, "vendor/keep.go") {
					t.Fatalf("ignored or hard-excluded file was admitted: %v", paths)
				}
			})
		}
	}
}

func TestScanPreviewRetainsGitNegationsAndUserExclusions(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, ".gitignore", []byte("*.go\n"))
	writeFile(t, repo, "src/.gitignore", []byte("!keep.go\n!other.go\n"))
	writeFile(t, repo, "src/keep.go", []byte("package keep\n"))
	writeFile(t, repo, "src/other.go", []byte("package other\n"))
	writeFile(t, repo, "src/ignored.go", []byte("package ignored\n"))
	tpl, err := template.LoadScanDefault()
	if err != nil {
		t.Fatal(err)
	}
	preview, err := Preview(t.Context(), Args{
		RepoDir: repo, Paths: []string{"src"}, Template: *tpl,
		FileFilter: &rules.FileFilter{Exclude: []string{"src/other.go", "**/.gitignore"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var reviewed []string
	for _, entry := range preview.Entries {
		if entry.WillReview {
			reviewed = append(reviewed, entry.Path)
		}
	}
	if !slices.Equal(reviewed, []string{"src/keep.go"}) || preview.ReviewableCount != 1 {
		t.Fatalf("preview selection = %v, want only src/keep.go: %+v", reviewed, preview)
	}
}

func TestScanPlainDirectoryStillAppliesRootIgnore(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, ".gitignore", []byte("ignored.go\n"))
	writeFile(t, repo, "keep.go", []byte("package keep\n"))
	writeFile(t, repo, "ignored.go", []byte("package ignored\n"))
	writeFile(t, repo, "vendor/keep.go", []byte("package vendor\n"))
	items, err := NewProvider(repo, nil, nil, 0).Enumerate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := itemPaths(items); !slices.Equal(got, []string{".gitignore", "keep.go"}) {
		t.Fatalf("plain directory ignore behavior changed: %v", got)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func comparisonGit(test *testing.T, repo string, args ...string) {
	test.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		test.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func comparisonReaders(test *testing.T) (FileReader, FileReader) {
	test.Helper()
	repo := setupTestRepo(test)
	writeTestFile(test, repo, "version.txt", "old-marker\n")
	writeTestFile(test, repo, "removed.txt", "removed-marker\n")
	writeTestFile(test, repo, "before.txt", "renamed-marker\n")
	comparisonGit(test, repo, "add", ".")
	comparisonGit(test, repo, "commit", "-m", "old version")
	old := getHeadCommit(test, repo)
	writeTestFile(test, repo, "version.txt", "new-marker\n")
	comparisonGit(test, repo, "rm", "removed.txt")
	comparisonGit(test, repo, "mv", "before.txt", "after.txt")
	comparisonGit(test, repo, "add", ".")
	comparisonGit(test, repo, "commit", "-m", "new version")
	current := FileReader{RepoDir: repo, Mode: ModeCommit, Ref: getHeadCommit(test, repo), Runner: gitcmd.New(4)}
	previous := current
	previous.Ref = old
	writeTestFile(test, repo, "version.txt", "workspace-marker\n")
	return current, previous
}

func comparisonProviders(current *FileReader, access *CodeAccess) []Provider {
	reader := NewFileRead(current)
	reader.Access = access
	finder := NewFileFind(current)
	finder.Access = access
	search := NewCodeSearch(current)
	search.Access = access
	return []Provider{reader, finder, search}
}

func comparisonArgs(name string) map[string]any {
	switch name {
	case "file_read":
		return map[string]any{"file_path": "version.txt"}
	case "file_find":
		return map[string]any{"query_name": ".txt"}
	default:
		return map[string]any{"search_text": "marker", "file_patterns": []any{"*.txt"}}
	}
}

func TestCodeAccessSides(test *testing.T) {
	current, previous := comparisonReaders(test)
	access, err := NewComparisonCodeAccess(current, previous)
	if err != nil {
		test.Fatal(err)
	}
	baseline := comparisonProviders(&current, nil)
	for index, provider := range comparisonProviders(&current, access) {
		test.Run(provider.Tool().Name(), func(test *testing.T) {
			args := comparisonArgs(provider.Tool().Name())
			want, err := baseline[index].Execute(context.Background(), args)
			if err != nil {
				test.Fatal(err)
			}
			for _, explicit := range []bool{false, true} {
				if explicit {
					args["side"] = "new"
				}
				got, err := provider.Execute(context.Background(), args)
				if err != nil || got != want {
					test.Fatalf("default/new output changed: %q, %v; want %q", got, err, want)
				}
			}
			args["side"] = "old"
			got, err := provider.Execute(context.Background(), args)
			if err != nil || !strings.HasPrefix(got, "SIDE: old\nREVISION: "+previous.Ref+"\n") {
				test.Fatalf("old source missing: %q, %v", got, err)
			}
			if strings.Contains(got, "new-marker") || strings.Contains(got, "workspace-marker") || strings.Contains(got, "after.txt") {
				test.Fatalf("old side leaked new content: %s", got)
			}
			if provider.Tool() != FileRead && (!strings.Contains(got, "removed.txt") || !strings.Contains(got, "before.txt")) {
				test.Fatalf("old paths missing: %s", got)
			}
			for _, invalid := range []any{nil, "", "OLD", "HEAD", true, float64(1)} {
				args["side"] = invalid
				if _, err := provider.Execute(context.Background(), args); err == nil {
					test.Fatalf("accepted invalid side %#v", invalid)
				}
			}
			args["side"] = "old"
			cancelled, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := provider.Execute(cancelled, args); !errors.Is(err, context.Canceled) {
				test.Fatalf("cancellation lost: %v", err)
			}
			for _, side := range []any{"old", "new", nil} {
				args["side"] = side
				if _, err := baseline[index].Execute(context.Background(), args); err == nil {
					test.Fatalf("current-only provider accepted side %#v", side)
				}
			}
		})
	}
	reader := NewFileRead(&current)
	reader.Access = access
	for _, name := range []string{"removed.txt", "before.txt", "after.txt"} {
		for _, side := range []string{"old", "new"} {
			_, err := reader.Execute(context.Background(), map[string]any{"file_path": name, "side": side})
			exists := (side == "new") == (name == "after.txt")
			if (err == nil) != exists {
				test.Fatalf("%s/%s: exists=%t, error=%v", side, name, exists, err)
			}
			if err != nil && side == "old" && !strings.Contains(err.Error(), previous.Ref) {
				test.Fatalf("old error lost source: %v", err)
			}
		}
	}
}

func TestCodeAccessConcurrentSides(test *testing.T) {
	current, previous := comparisonReaders(test)
	access, err := NewComparisonCodeAccess(current, previous)
	if err != nil {
		test.Fatal(err)
	}
	providers := comparisonProviders(&current, access)
	var workers sync.WaitGroup
	for iteration := 0; iteration < 12; iteration++ {
		for _, provider := range providers {
			workers.Add(1)
			go func() {
				defer workers.Done()
				side := "new"
				if iteration%2 == 0 {
					side = "old"
				}
				args := comparisonArgs(provider.Tool().Name())
				args["side"] = side
				got, err := provider.Execute(context.Background(), args)
				if err != nil {
					test.Error(err)
					return
				}
				want := side + "-marker"
				if provider.Tool() == FileFind {
					want = "after.txt"
					if side == "old" {
						want = "before.txt"
					}
				}
				if !strings.Contains(got, want) {
					test.Errorf("%s/%s: %q lacks %q", provider.Tool().Name(), side, got, want)
				}
			}()
		}
	}
	workers.Wait()
}

func TestCodeAccessEmptyAndMissingTrees(test *testing.T) {
	current, previous := comparisonReaders(test)
	for _, empty := range []bool{true, false} {
		test.Run(fmt.Sprint(empty), func(test *testing.T) {
			old := previous
			old.Ref = strings.Repeat("f", 40)
			if empty {
				old.Mode, old.Ref = ModeEmpty, ""
			}
			access, err := NewComparisonCodeAccess(current, old)
			if err != nil {
				test.Fatal(err)
			}
			for _, provider := range comparisonProviders(&current, access) {
				args := comparisonArgs(provider.Tool().Name())
				args["side"] = "old"
				got, err := provider.Execute(context.Background(), args)
				if !empty || provider.Tool() == FileRead {
					if err == nil || !strings.Contains(err.Error(), "SIDE: old") {
						test.Fatalf("missing tree/file did not fail with source: %q %v", got, err)
					}
				} else if err != nil || !strings.HasPrefix(got, "SIDE: old\nSOURCE: empty-tree\n") {
					test.Fatalf("empty tree: %q %v", got, err)
				}
			}
		})
	}
	if _, err := (&FileReader{Mode: ModeEmpty}).Read(context.Background(), "version.txt"); err == nil {
		test.Fatal("empty-tree Read succeeded")
	}
	for _, pair := range [][2]FileReader{{{}, previous}, {current, {}}, {{Mode: ModeWorkspace, Ref: "HEAD"}, previous}, {current, {Mode: ModeWorkspace, Ref: "HEAD"}}} {
		if _, err := NewComparisonCodeAccess(pair[0], pair[1]); err == nil {
			test.Fatal("accepted unpinned reader")
		}
	}
	if err := os.Remove(filepath.Join(current.RepoDir, "version.txt")); err != nil {
		test.Fatal(err)
	}
	if _, err := current.Read(context.Background(), "version.txt"); err != nil {
		test.Fatalf("pinned read depended on worktree: %v", err)
	}
}

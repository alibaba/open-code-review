// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestWorkspaceDiffUsesLiveTree(test *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		for _, unborn := range []bool{false, true} {
			name := format + "/committed"
			if unborn {
				name = format + "/unborn"
			}
			test.Run(name, func(test *testing.T) {
				repo := test.TempDir()
				runGitTest(test, repo, "init", "-q", "--object-format="+format)
				runGitTest(test, repo, "config", "user.name", "Test")
				runGitTest(test, repo, "config", "user.email", "test@example.com")
				runGitTest(test, repo, "config", "commit.gpgsign", "false")
				if !unborn {
					writeCommit(test, repo, "version.go", "package example\nconst Value = 1\n", "base")
				}
				path := filepath.Join(repo, "version.go")
				write := func(content string) {
					test.Helper()
					if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
						test.Fatal(err)
					}
				}
				write("package example\nconst Value = 2\n")
				runGitTest(test, repo, "add", "version.go")
				write("package example\nconst Value = 3\n")
				indexBefore, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
				if err != nil {
					test.Fatal(err)
				}
				provider := NewWorkspaceProvider(repo, gitcmd.New(2))
				diffs, err := provider.GetDiff(context.Background())
				if err != nil || len(diffs) != 1 {
					test.Fatalf("workspace diff: %v %v", diffs, err)
				}
				indexAfterDiff, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
				if err != nil || !bytes.Equal(indexBefore, indexAfterDiff) {
					test.Fatalf("first diff modified the index: %v\nbefore: %x\nafter: %x", err, indexBefore, indexAfterDiff)
				}
				if !strings.Contains(diffs[0].Diff, "+const Value = 3") || strings.Contains(diffs[0].Diff, "Value = 2") || !strings.Contains(diffs[0].NewFileContent, "Value = 3") {
					test.Fatalf("diff or new content used the index: %+v", diffs[0])
				}
				resolution := provider.ResolveInput(context.Background())
				if resolution.BaseIsEmpty != unborn || resolution.ResolvedHead != "" || resolution.ExactRange != "" {
					test.Fatalf("incorrect workspace identity: %+v", resolution)
				}
				if !unborn && resolution.ResolvedBase != gitOut(test, repo, "rev-parse", "HEAD") {
					test.Fatalf("base not pinned: %+v", resolution)
				}
				if unborn {
					if err := os.Remove(path); err != nil {
						test.Fatal(err)
					}
				} else {
					write("package example\nconst Value = 1\n")
				}
				diffs, err = provider.GetDiff(context.Background())
				if err != nil || len(diffs) != 0 {
					test.Fatalf("empty workspace fell back to staged content: %v %v", diffs, err)
				}
				indexAfter, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
				if err != nil || !bytes.Equal(indexBefore, indexAfter) {
					test.Fatalf("empty diff modified the index: %v\nbefore: %x\nafter: %x", err, indexBefore, indexAfter)
				}
			})
		}
	}
}

func TestWorkspaceBaseRemainsFrozen(test *testing.T) {
	for _, unborn := range []bool{false, true} {
		repo := initBareRepo(test)
		if !unborn {
			writeCommit(test, repo, "version.go", "package example\nconst Value = 1\n", "base")
		}
		provider := NewWorkspaceProvider(repo, nil)
		base, err := provider.ResolveWorkspaceComparison(context.Background())
		if err != nil {
			test.Fatal(err)
		}
		writeCommit(test, repo, "version.go", "package example\nconst Value = 2\n", "advance")
		if err := os.WriteFile(filepath.Join(repo, "version.go"), []byte("package example\nconst Value = 3\n"), 0o644); err != nil {
			test.Fatal(err)
		}
		for _, selected := range []*Provider{provider, NewWorkspaceProviderWithBase(repo, base, nil)} {
			diffs, err := selected.GetDiff(context.Background())
			if err != nil || len(diffs) != 1 || strings.Contains(diffs[0].Diff, "Value = 2") {
				test.Fatalf("base moved after resolution: %v %v", diffs, err)
			}
			if selected.ResolveInput(context.Background()) != base {
				test.Fatal("manifest resolution moved after loading the diff")
			}
		}
	}
}

func TestWorkspaceBaseRejectsInvalidInputs(test *testing.T) {
	repo := initBareRepo(test)
	for _, base := range []InputResolution{{}, {ResolvedHead: "head", BaseIsEmpty: true}, {ExactRange: "base..head", BaseIsEmpty: true}, {ResolvedBase: "base", BaseIsEmpty: true}} {
		if _, err := NewWorkspaceProviderWithBase(repo, base, nil).GetDiff(context.Background()); err == nil {
			test.Fatalf("invalid base accepted: %+v", base)
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewWorkspaceProvider(repo, nil).ResolveWorkspaceComparison(cancelled); !errors.Is(err, context.Canceled) {
		test.Fatalf("cancellation lost: %v", err)
	}
	branch := gitOut(test, repo, "symbolic-ref", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, ".git", filepath.FromSlash(branch)), []byte(strings.Repeat("f", 40)+"\n"), 0o644); err != nil {
		test.Fatal(err)
	}
	if _, err := NewWorkspaceProvider(repo, nil).GetDiff(context.Background()); err == nil {
		test.Fatal("missing HEAD object treated as an unborn repository")
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "HEAD"), []byte(strings.Repeat("f", 40)+"\n"), 0o644); err != nil {
		test.Fatal(err)
	}
	if _, err := NewWorkspaceProvider(repo, nil).GetDiff(context.Background()); err == nil {
		test.Fatal("invalid detached HEAD treated as an unborn repository")
	}
	if _, err := NewWorkspaceProvider(repo, &gitcmd.Runner{}).GetDiff(context.Background()); err == nil {
		test.Fatal("Git runner failure treated as an unborn repository")
	}
}

func TestWorkspaceShallowHeadComparison(test *testing.T) {
	repo := initBareRepo(test)
	writeCommit(test, repo, "version.go", "package example\nconst Value = 1\n", "root")
	writeCommit(test, repo, "version.go", "package example\nconst Value = 2\n", "head")
	shallow := initBareRepo(test)
	runGitTest(test, shallow, "-c", "protocol.file.allow=always", "fetch", "--depth=1", filepath.ToSlash(repo), "HEAD")
	runGitTest(test, shallow, "checkout", "--detach", "FETCH_HEAD")
	if err := os.WriteFile(filepath.Join(shallow, "version.go"), []byte("package example\nconst Value = 3\n"), 0o644); err != nil {
		test.Fatal(err)
	}
	provider := NewWorkspaceProvider(shallow, nil)
	diffs, err := provider.GetDiff(context.Background())
	if err != nil || len(diffs) != 1 {
		test.Fatalf("shallow workspace diff: %v %v", diffs, err)
	}
	if !strings.Contains(diffs[0].Diff, "-const Value = 2") || !strings.Contains(diffs[0].Diff, "+const Value = 3") || provider.ResolveInput(context.Background()).ResolvedBase != gitOut(test, shallow, "rev-parse", "HEAD") {
		test.Fatalf("shallow workspace did not use HEAD as its base: %+v", diffs[0])
	}
}

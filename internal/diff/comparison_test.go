// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestResolveCommitComparison(test *testing.T) {
	repo := initBareRepo(test)
	writeCommit(test, repo, "base.txt", "base\n", "root")
	root := gitOut(test, repo, "rev-parse", "HEAD")
	resolution, err := NewCommitProvider(repo, "HEAD", gitcmd.New(1)).ResolveCommitComparison(context.Background())
	if err != nil || resolution.ResolvedHead != root || !resolution.BaseIsEmpty || resolution.ResolvedBase != "" {
		test.Fatalf("root comparison: %+v %v", resolution, err)
	}
	writeCommit(test, repo, "base.txt", "next\n", "next")
	parent := gitOut(test, repo, "rev-parse", "HEAD")
	resolution, err = NewCommitProvider(repo, "HEAD", nil).ResolveCommitComparison(context.Background())
	if err != nil || resolution.ResolvedBase != root || resolution.BaseIsEmpty || resolution.ExactRange != root+".."+parent {
		test.Fatalf("ordinary comparison: %+v %v", resolution, err)
	}
	branch := gitOut(test, repo, "branch", "--show-current")
	runGitTest(test, repo, "checkout", "-b", "side", root)
	writeCommit(test, repo, "side.txt", "side\n", "side")
	runGitTest(test, repo, "checkout", branch)
	runGitTest(test, repo, "merge", "--no-ff", "side", "-m", "merge")
	resolution, err = NewCommitProvider(repo, "HEAD", nil).ResolveCommitComparison(context.Background())
	if err != nil || resolution.ResolvedBase != parent || resolution.BaseIsEmpty {
		test.Fatalf("merge comparison: %+v %v", resolution, err)
	}
	for _, ref := range []string{"missing-ref", strings.Repeat("f", 40)} {
		if _, err := NewCommitProvider(repo, ref, nil).ResolveCommitComparison(context.Background()); err == nil {
			test.Fatalf("resolved missing commit %s", ref)
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewCommitProvider(repo, "HEAD", nil).ResolveCommitComparison(cancelled); err == nil {
		test.Fatal("cancelled comparison succeeded")
	}
}

func TestResolveCommitComparisonRejectsShallowParent(test *testing.T) {
	repo := initBareRepo(test)
	writeCommit(test, repo, "base.txt", "base\n", "root")
	root := gitOut(test, repo, "rev-parse", "HEAD")
	writeCommit(test, repo, "base.txt", "next\n", "next")
	head := gitOut(test, repo, "rev-parse", "HEAD")
	shallow := initBareRepo(test)
	runGitTest(test, shallow, "-c", "protocol.file.allow=always", "fetch", "--depth=1", filepath.ToSlash(repo), "HEAD")
	_, err := NewCommitProvider(shallow, "FETCH_HEAD", nil).ResolveCommitComparison(context.Background())
	if err == nil || !strings.Contains(err.Error(), "parent") {
		test.Fatalf("shallow boundary treated as empty tree: %v", err)
	}
	runGitTest(test, shallow, "-c", "protocol.file.allow=always", "fetch", "--depth=1", filepath.ToSlash(repo), root)
	if gitOut(test, shallow, "cat-file", "-t", root) != "commit" {
		test.Fatal("parent object is not available")
	}
	_, err = NewCommitProvider(shallow, head, nil).ResolveCommitComparison(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Git history") {
		test.Fatalf("accepted a parent hidden by the shallow boundary: %v", err)
	}
	runGitTest(test, shallow, "-c", "protocol.file.allow=always", "fetch", "--unshallow", filepath.ToSlash(repo), "HEAD")
	resolution, err := NewCommitProvider(shallow, head, nil).ResolveCommitComparison(context.Background())
	if err != nil || resolution.ResolvedBase != root || resolution.BaseIsEmpty {
		test.Fatalf("deepened comparison: %+v %v", resolution, err)
	}
}

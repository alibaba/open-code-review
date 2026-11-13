// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/vcs"
)

func reviewGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com", "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func reviewCommit(t *testing.T, dir, path, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	reviewGit(t, dir, "add", "--", path)
	reviewGit(t, dir, "commit", "-m", "fixture")
	return reviewGit(t, dir, "rev-parse", "HEAD")
}

func reviewRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	reviewGit(t, dir, "init", "-b", "main")
	return dir
}

func TestResolvedReviewPinsChangesAndDistinguishesComparisons(t *testing.T) {
	dir := reviewRepo(t)
	root := reviewCommit(t, dir, "base.go", "package base\n")
	reviewGit(t, dir, "checkout", "-b", "feature")
	head := reviewCommit(t, dir, "feature.go", "package feature\n")
	reviewGit(t, dir, "checkout", "main")
	main := reviewCommit(t, dir, "main.go", "package main\n")
	r := New(dir, gitcmd.New(2))
	ctx := context.Background()
	merged, err := r.ResolveReview(ctx, vcs.ReviewSpec{Mode: vcs.MergeBase, Base: "main", Head: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	direct, err := r.ResolveReview(ctx, vcs.ReviewSpec{Mode: vcs.TwoRevisions, Base: "main", Head: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Base != root || direct.Base != main || merged.Head != head || merged.Content.Revision != head {
		t.Fatalf("wrong endpoints: %+v / %+v", merged, direct)
	}
	before, err := r.Changes(ctx, merged)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Included) != 1 || before.Included[0].NewPath != "feature.go" {
		t.Fatalf("merge-base changes: %+v", before)
	}
	compared, err := r.Changes(ctx, direct)
	if err != nil {
		t.Fatal(err)
	}
	if len(compared.Included) != 2 {
		t.Fatalf("direct changes: %+v", compared)
	}
	reviewGit(t, dir, "checkout", "feature")
	reviewCommit(t, dir, "late.go", "package late\n")
	after, err := r.Changes(ctx, merged)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("moving branch changed resolved input: %+v", after)
	}
	// The direct comparison must keep main as its base even though main is not
	// an ancestor of feature; recomputing a merge-base would drop the deletion.
	afterDirect, err := r.Changes(ctx, direct)
	if err != nil || !reflect.DeepEqual(compared, afterDirect) {
		t.Fatalf("direct comparison changed: %+v, %v", afterDirect, err)
	}
}

func TestRootCommitMergeCommitAndWorkspace(t *testing.T) {
	ctx := context.Background()
	dir := reviewRepo(t)
	r := New(dir, nil)
	unborn, err := r.ResolveReview(ctx, vcs.ReviewSpec{Mode: vcs.Workspace})
	if err != nil || unborn.Base != "" || !unborn.Content.IsWorkspace() {
		t.Fatalf("unborn: %+v, %v", unborn, err)
	}
	root := reviewCommit(t, dir, "base.go", "package base\n")
	// A workspace resolved before its first commit must still compare against
	// the empty tree if another process creates HEAD before Changes runs.
	firstWorkspace, err := r.Changes(ctx, unborn)
	if err != nil || len(firstWorkspace.Included) != 1 {
		t.Fatalf("unborn workspace changed with HEAD: %+v, %v", firstWorkspace, err)
	}
	initial, err := r.ResolveReview(ctx, vcs.ReviewSpec{Mode: vcs.Commit, Head: root})
	if err != nil || initial.Base != "" || initial.Head != root {
		t.Fatalf("initial: %+v, %v", initial, err)
	}
	changes, err := r.Changes(ctx, initial)
	if err != nil || len(changes.Included) != 1 || !changes.Included[0].IsNew {
		t.Fatalf("initial changes: %+v, %v", changes, err)
	}
	reviewGit(t, dir, "checkout", "-b", "feature")
	reviewCommit(t, dir, "feature.go", "package feature\n")
	reviewGit(t, dir, "checkout", "main")
	parent := reviewCommit(t, dir, "main.go", "package main\n")
	reviewGit(t, dir, "merge", "--no-ff", "feature", "-m", "merge")
	merge, err := r.ResolveReview(ctx, vcs.ReviewSpec{Mode: vcs.Commit, Head: "HEAD"})
	if err != nil || merge.Base != parent {
		t.Fatalf("merge: %+v, %v", merge, err)
	}
	changes, err = r.Changes(ctx, merge)
	if err != nil || len(changes.Included) != 1 || changes.Included[0].NewPath != "feature.go" {
		t.Fatalf("merge changes: %+v, %v", changes, err)
	}
	workspace, err := r.ResolveReview(ctx, vcs.ReviewSpec{Mode: vcs.Workspace})
	if err != nil {
		t.Fatal(err)
	}
	reviewCommit(t, dir, "main.go", "package main\n// updated\n")
	changes, err = r.Changes(ctx, workspace)
	if err != nil || len(changes.Included) != 1 || changes.Included[0].NewPath != "main.go" {
		t.Fatalf("workspace lost pinned base: %+v, %v", changes, err)
	}
}

func TestReviewRejectsUnsupportedAndInvalidInput(t *testing.T) {
	r := New(reviewRepo(t), nil)
	ctx := context.Background()
	if _, err := r.ResolveReview(ctx, vcs.ReviewSpec{Mode: "unknown"}); !errors.Is(err, vcs.ErrUnsupported) {
		t.Fatalf("unsupported: %v", err)
	}
	for _, spec := range []vcs.ReviewSpec{{Mode: vcs.Commit}, {Mode: vcs.MergeBase, Head: "HEAD"}, {Mode: vcs.Workspace, Head: "HEAD"}, {Mode: vcs.Commit, Head: "--help"}, {Mode: vcs.Commit, Head: "missing"}} {
		if _, err := r.ResolveReview(ctx, spec); err == nil {
			t.Fatalf("accepted %+v", spec)
		}
	}
	if _, err := r.Changes(ctx, vcs.ResolvedReview{Provider: "svn"}); !errors.Is(err, vcs.ErrUnsupported) {
		t.Fatalf("foreign provider: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.ResolveReview(canceled, vcs.ReviewSpec{Mode: vcs.Workspace}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
}

func TestWorkspaceRetainsStagedChangesCanceledByWorkingTree(t *testing.T) {
	dir := reviewRepo(t)
	reviewCommit(t, dir, "main.go", "package main\n// old\n")
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n// staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reviewGit(t, dir, "add", "main.go")
	if err := os.WriteFile(file, []byte("package main\n// old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := New(dir, nil)
	resolved, err := r.ResolveReview(context.Background(), vcs.ReviewSpec{Mode: vcs.Workspace})
	if err != nil {
		t.Fatal(err)
	}
	changes, err := r.Changes(context.Background(), resolved)
	if err != nil || len(changes.Included) != 1 || !strings.Contains(changes.Included[0].Diff, "+// staged") {
		t.Fatalf("lost index change: %+v, %v", changes, err)
	}
}

func TestMergeBaseDoesNotFallBackForUnrelatedHistories(t *testing.T) {
	dir := reviewRepo(t)
	base := reviewCommit(t, dir, "base.go", "package base\n")
	reviewGit(t, dir, "checkout", "--orphan", "unrelated")
	reviewGit(t, dir, "rm", "-f", "--", "base.go")
	head := reviewCommit(t, dir, "other.go", "package other\n")
	r := New(dir, nil)
	ctx := context.Background()
	if _, err := r.ResolveReview(ctx, vcs.ReviewSpec{Mode: vcs.MergeBase, Base: base, Head: head}); err == nil {
		t.Fatal("unrelated histories silently accepted")
	}
	if _, err := r.ResolveReview(ctx, vcs.ReviewSpec{Mode: vcs.TwoRevisions, Base: "missing-base", Head: head}); err == nil {
		t.Fatal("invalid base silently accepted")
	}
	direct, err := r.ResolveReview(ctx, vcs.ReviewSpec{Mode: vcs.TwoRevisions, Base: base, Head: head})
	if err != nil {
		t.Fatal(err)
	}
	changes, err := r.Changes(ctx, direct)
	if err != nil || len(changes.Included) != 2 {
		t.Fatalf("explicit direct comparison: %+v, %v", changes, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.ResolveReview(canceled, vcs.ReviewSpec{Mode: vcs.Commit, Head: head}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled resolution: %v", err)
	}
}

func TestMalformedResolvedInputsFailBeforeReading(t *testing.T) {
	t.Setenv("PATH", "")
	r := New(t.TempDir(), nil)
	for _, resolved := range []vcs.ResolvedReview{
		{Provider: "git", Spec: vcs.ReviewSpec{Mode: "unknown"}},
		{Provider: "git", Spec: vcs.ReviewSpec{Mode: vcs.Workspace}, Head: "r2"},
		{Provider: "git", Spec: vcs.ReviewSpec{Mode: vcs.Workspace}, Content: vcs.ContentRef{Revision: "r2"}},
		{Provider: "git", Spec: vcs.ReviewSpec{Mode: vcs.Commit, Head: "r2"}},
		{Provider: "git", Spec: vcs.ReviewSpec{Mode: vcs.Commit, Head: "r2"}, Head: "r2", Content: vcs.ContentRef{Revision: "other"}},
		{Provider: "git", Spec: vcs.ReviewSpec{Mode: vcs.TwoRevisions, Base: "r1", Head: "r2"}, Head: "r2", Content: vcs.ContentRef{Revision: "r2"}},
	} {
		_, err := r.Changes(context.Background(), resolved)
		if err == nil || errors.Is(err, exec.ErrNotFound) {
			t.Fatalf("malformed input reached Git: %+v, %v", resolved, err)
		}
	}
}

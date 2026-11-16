// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package git

import (
	"context"
	"errors"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestCurrentBranchNamedDetachedAndCanceled(t *testing.T) {
	dir := reviewRepo(t)
	reviewCommit(t, dir, "main.go", "package main\n")
	r := New(dir, nil)
	branch, err := r.CurrentBranch(context.Background())
	if err != nil || branch != "main" {
		t.Fatalf("named branch: %q, %v", branch, err)
	}
	reviewGit(t, dir, "checkout", "--detach", "HEAD")
	r.Runner = gitcmd.New(1)
	branch, err = r.CurrentBranch(context.Background())
	if err != nil || branch != "HEAD" {
		t.Fatalf("detached branch metadata: %q, %v", branch, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	branch, err = r.CurrentBranch(ctx)
	if branch != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled branch query: %q, %v", branch, err)
	}
}

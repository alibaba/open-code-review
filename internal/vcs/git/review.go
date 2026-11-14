// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package git implements repository operations with the Git command-line client.
package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/vcs"
)

type Repository struct {
	RepoDir string
	Runner  *gitcmd.Runner
}

func New(repoDir string, runner *gitcmd.Runner) *Repository {
	return &Repository{RepoDir: repoDir, Runner: runner}
}

func (r *Repository) CurrentBranch(ctx context.Context) (string, error) {
	runner := r.Runner
	if runner == nil {
		runner = gitcmd.New(1)
	}
	out, err := runner.Output(ctx, r.RepoDir, "rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(string(out)), err
}

func (r *Repository) resolveCommit(ctx context.Context, ref string) (string, error) {
	if ref == "" || strings.HasPrefix(ref, "-") {
		return "", fmt.Errorf("invalid revision %q", ref)
	}
	head := diff.NewCommitProvider(r.RepoDir, ref, r.Runner).ResolveInput(ctx).ResolvedHead
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if head == "" {
		return "", fmt.Errorf("resolve commit %q", ref)
	}
	return head, nil
}

// ResolveReview resolves moving names once, before any diff or context read.
func (r *Repository) ResolveReview(ctx context.Context, spec vcs.ReviewSpec) (vcs.ResolvedReview, error) {
	if err := spec.Validate(); err != nil {
		return vcs.ResolvedReview{}, err
	}
	resolved := vcs.ResolvedReview{Provider: "git", Spec: spec}
	workspace := diff.NewWorkspaceProvider(r.RepoDir, r.Runner)
	resolved.RepositoryID = workspace.RemoteIdentity(ctx)
	if spec.Mode == vcs.Workspace {
		base, err := workspace.ResolveWorkspaceComparison(ctx)
		resolved.Base, resolved.BaseIsEmpty = base.ResolvedBase, base.BaseIsEmpty
		return resolved, err
	}
	head, err := r.resolveCommit(ctx, spec.Head)
	if err != nil {
		return vcs.ResolvedReview{}, err
	}
	resolved.Head = head
	resolved.Content = vcs.ContentRef{Revision: head}
	if spec.Mode == vcs.Commit {
		comparison, err := diff.NewCommitProvider(r.RepoDir, head, r.Runner).ResolveCommitComparison(ctx)
		if err != nil {
			return vcs.ResolvedReview{}, err
		}
		resolved.Base, resolved.BaseIsEmpty = comparison.ResolvedBase, comparison.BaseIsEmpty
		return resolved, nil
	}
	base, err := r.resolveCommit(ctx, spec.Base)
	if err != nil {
		return vcs.ResolvedReview{}, err
	}
	if spec.Mode == vcs.MergeBase {
		base = diff.NewProvider(r.RepoDir, base, head, r.Runner).MergeBase(ctx)
		if err := ctx.Err(); err != nil {
			return vcs.ResolvedReview{}, err
		}
		if base == "" {
			return vcs.ResolvedReview{}, fmt.Errorf("resolve merge-base between %q and %q", spec.Base, spec.Head)
		}
	}
	resolved.Base = base
	return resolved, nil
}

// Changes consumes resolved endpoints; it never re-resolves the original names.
func (r *Repository) Changes(ctx context.Context, review vcs.ResolvedReview) (vcs.Changes, error) {
	if review.Provider != "git" {
		return vcs.Changes{}, fmt.Errorf("%w: Git cannot read %q review", vcs.ErrUnsupported, review.Provider)
	}
	if err := review.Spec.Validate(); err != nil {
		return vcs.Changes{}, err
	}
	var provider *diff.Provider
	switch review.Spec.Mode {
	case vcs.Workspace:
		if review.Head != "" || !review.Content.IsWorkspace() {
			return vcs.Changes{}, fmt.Errorf("workspace review has historical content")
		}
		provider = diff.NewWorkspaceProviderWithBase(r.RepoDir, diff.InputResolution{ResolvedBase: review.Base, BaseIsEmpty: review.BaseIsEmpty}, r.Runner)
	default:
		if review.Head == "" || review.Content.Revision != review.Head {
			return vcs.Changes{}, fmt.Errorf("review has no resolved content revision")
		}
		if review.Base == "" {
			if review.Spec.Mode != vcs.Commit {
				return vcs.Changes{}, fmt.Errorf("comparison has no resolved base")
			}
			provider = diff.NewCommitProvider(r.RepoDir, review.Head, r.Runner)
		} else {
			provider = diff.NewDirectProvider(r.RepoDir, review.Base, review.Head, r.Runner)
		}
	}
	set, err := provider.GetDiffSet(ctx)
	if err != nil {
		return vcs.Changes{}, err
	}
	changes := vcs.Changes{Included: set.Included, Excluded: set.Excluded}
	next := 0
	set.ForEachInOrder(func(_ model.Diff, excluded bool) {
		if excluded {
			changes.ExcludedAt = append(changes.ExcludedAt, next)
		}
		next++
	})
	return changes, nil
}

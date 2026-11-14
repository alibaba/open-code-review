// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/vcs"
	gitvcs "github.com/alibaba/open-code-review/internal/vcs/git"
)

// ReviewSpec translates the existing command inputs to repository semantics.
func ReviewSpec(from, to, commit string) vcs.ReviewSpec {
	if commit != "" {
		return vcs.ReviewSpec{Mode: vcs.Commit, Head: commit}
	}
	if from != "" && to != "" {
		return vcs.ReviewSpec{Mode: vcs.MergeBase, Base: from, Head: to}
	}
	return vcs.ReviewSpec{Mode: vcs.Workspace}
}

func (a *Agent) reviewSpec() vcs.ReviewSpec {
	if a.args.ResolvedReview != nil {
		return a.args.ResolvedReview.Spec
	}
	if a.resolvedReview.Provider != "" {
		return a.resolvedReview.Spec
	}
	return ReviewSpec(a.args.From, a.args.To, a.args.Commit)
}

func (a *Agent) loadRepositoryChanges(ctx context.Context) (vcs.Changes, error) {
	source := a.args.Repository
	if source == nil {
		source = gitvcs.New(a.args.RepoDir, a.args.GitRunner)
	}
	var review vcs.ResolvedReview
	if a.args.ResolvedReview != nil {
		review = *a.args.ResolvedReview
	} else {
		spec := ReviewSpec(a.args.From, a.args.To, a.args.Commit)
		requested := spec
		// Compatibility for callers that still pass the old Git endpoint seal.
		if s := a.args.SealedInput; s != nil && s.ResolvedHead != "" {
			switch spec.Mode {
			case vcs.Commit:
				spec.Head = s.ResolvedHead
			case vcs.MergeBase:
				if s.ResolvedBase != "" {
					spec.Base, spec.Head = s.ResolvedBase, s.ResolvedHead
				}
			}
		}
		var err error
		review, err = source.ResolveReview(ctx, spec)
		if err != nil {
			return vcs.Changes{}, err
		}
		// A legacy endpoint seal changes where we read, not what was requested.
		review.Spec = requested
		if s := a.args.SealedInput; s != nil && spec.Mode == vcs.Workspace {
			if s.ResolvedHead != "" || s.ExactRange != "" {
				return vcs.Changes{}, fmt.Errorf("workspace comparison cannot have a resolved head or range")
			}
			review.Base, review.BaseIsEmpty = s.ResolvedBase, s.BaseIsEmpty
		}
	}
	set, err := source.Changes(ctx, review)
	if err != nil {
		return vcs.Changes{}, err
	}
	a.resolvedReview = review
	a.inputResolution = inputResolutionForReview(review)
	a.repoRemoteIdentity = review.RepositoryID
	if review.Provider != "git" && review.RepositoryID != "" {
		a.repoRemoteIdentity = review.Provider + "\x00" + review.RepositoryID
	}
	return set, nil
}

func inputResolutionForReview(review vcs.ResolvedReview) diff.InputResolution {
	resolution := diff.InputResolution{ResolvedBase: review.Base, ResolvedHead: review.Head, BaseIsEmpty: review.BaseIsEmpty}
	// Keep existing Git manifests and resume identities byte-for-byte compatible.
	if review.Provider == "git" && review.Base != "" && review.Head != "" {
		resolution.ExactRange = review.Base + ".." + review.Head
	}
	return resolution
}

func repositoryBranch(parent context.Context, args Args) string {
	source := args.Repository
	if source == nil {
		source = gitvcs.New(args.RepoDir, args.GitRunner)
	}
	if metadata, ok := source.(vcs.BranchSource); ok {
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		branch, _ := metadata.CurrentBranch(ctx)
		return branch
	}
	return ""
}

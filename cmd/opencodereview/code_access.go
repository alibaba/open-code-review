// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/alibaba/open-code-review/internal/vcs"
)

type reviewCodeTools struct {
	reader     *tool.FileReader
	access     *tool.CodeAccess
	resolution *diff.InputResolution
}

func prepareReviewCodeTools(ctx context.Context, repoDir string, opts reviewOptions, runner *gitcmd.Runner, resolution *diff.InputResolution, source vcs.Repository, review *vcs.ResolvedReview) (*reviewCodeTools, error) {
	mode := tool.ParseReviewMode(opts.from, opts.to, opts.commit)
	if resolution == nil {
		var err error
		resolution, err = agent.ResolveComparisonInput(ctx, agent.Args{
			RepoDir: repoDir, From: opts.from, To: opts.to, Commit: opts.commit, GitRunner: runner,
			Repository: source, ResolvedReview: review,
		})
		if err != nil {
			return nil, err
		}
	}
	reader := &tool.FileReader{
		RepoDir: repoDir, Mode: mode, Ref: fileReadRef(mode, opts, resolution), Runner: runner,
	}
	if source != nil {
		reader.Source = source
		reader.Content = vcs.ContentRef{Revision: reader.Ref}
		if review != nil {
			reader.Content = review.Content
		}
	}
	setup := &reviewCodeTools{reader: reader, resolution: resolution}
	if resolution == nil || (resolution.ResolvedBase == "" && !resolution.BaseIsEmpty) || (resolution.ResolvedBase != "" && resolution.BaseIsEmpty) {
		return nil, fmt.Errorf("code access requires a resolved base or an empty tree")
	}
	previous := *reader
	previous.Mode = tool.ModeCommit
	previous.Ref = resolution.ResolvedBase
	previous.Content = vcs.ContentRef{Revision: previous.Ref}
	if resolution.BaseIsEmpty {
		previous.Mode = tool.ModeEmpty
	}
	var err error
	if mode != tool.ModeWorkspace {
		if resolution.ResolvedHead == "" {
			return nil, fmt.Errorf("comparison code access requires resolved comparison endpoints")
		}
		setup.access, err = tool.NewComparisonCodeAccess(*reader, previous)
	} else {
		if resolution.ResolvedHead != "" || resolution.ExactRange != "" {
			return nil, fmt.Errorf("workspace code access cannot have a resolved head or range")
		}
		setup.access, err = tool.NewWorkspaceCodeAccess(*reader, previous)
	}
	if err != nil {
		return nil, err
	}
	return setup, nil
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type reviewCodeTools struct {
	reader     *tool.FileReader
	access     *tool.CodeAccess
	resolution *diff.InputResolution
	profile    string
}

func prepareReviewCodeTools(ctx context.Context, repoDir string, opts reviewOptions, runner *gitcmd.Runner, state *session.ResumeState, resolution *diff.InputResolution) (*reviewCodeTools, error) {
	mode := tool.ParseReviewMode(opts.from, opts.to, opts.commit)
	profile := session.CodeAccessCurrentOnly
	if state != nil {
		var err error
		profile, err = state.CodeAccessProfile()
		if err != nil {
			return nil, err
		}
	} else if mode != tool.ModeWorkspace {
		profile = session.CodeAccessComparisonSides
	}
	if mode == tool.ModeWorkspace && profile == session.CodeAccessComparisonSides {
		return nil, fmt.Errorf("comparison code access requires a commit or range review")
	}
	if resolution == nil && mode != tool.ModeWorkspace {
		var err error
		resolution, err = agent.ResolveComparisonInput(ctx, agent.Args{
			RepoDir: repoDir, From: opts.from, To: opts.to, Commit: opts.commit, GitRunner: runner,
		})
		if err != nil {
			return nil, err
		}
	}
	reader := &tool.FileReader{
		RepoDir: repoDir, Mode: mode, Ref: fileReadRef(mode, opts, resolution), Runner: runner,
	}
	setup := &reviewCodeTools{reader: reader, resolution: resolution, profile: profile}
	if profile == session.CodeAccessComparisonSides {
		if resolution == nil || resolution.ResolvedHead == "" || (resolution.ResolvedBase == "" && !resolution.BaseIsEmpty) {
			return nil, fmt.Errorf("comparison code access requires resolved comparison endpoints")
		}
		previous := *reader
		previous.Mode = tool.ModeCommit
		previous.Ref = resolution.ResolvedBase
		if resolution.BaseIsEmpty {
			previous.Mode = tool.ModeEmpty
		}
		var err error
		setup.access, err = tool.NewComparisonCodeAccess(*reader, previous)
		if err != nil {
			return nil, err
		}
	}
	return setup, nil
}

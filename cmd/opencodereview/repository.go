// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/config/rules"
	gitvcs "github.com/alibaba/open-code-review/internal/vcs/git"
)

// prepareRepositoryReview freezes historical content before preview, resume
// admission, rule sniffing, and file tools can observe a moving revision name.
func prepareRepositoryReview(ctx context.Context, cc *commonContext, opts reviewOptions) error {
	if cc.Repository == nil {
		cc.Repository = gitvcs.New(cc.RepoDir, cc.GitRunner)
	}
	resolved, err := cc.Repository.ResolveReview(ctx, agent.ReviewSpec(opts.from, opts.to, opts.commit))
	if err != nil {
		return fmt.Errorf("resolve review input: %w", err)
	}
	resolver, filter, err := rules.NewResolver(cc.RepoDir, opts.rulePath, rules.ResolverOptions{
		Files:   cc.Repository,
		Content: resolved.Content,
	})
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}
	cc.Review = &resolved
	cc.Resolver, cc.FileFilter = resolver, filter
	applyCLIExcludes(cc, splitPaths(opts.excludes))
	return nil
}

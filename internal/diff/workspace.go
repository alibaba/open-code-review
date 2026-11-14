// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func NewWorkspaceProviderWithBase(repoDir string, base InputResolution, runner *gitcmd.Runner) *Provider {
	provider := NewWorkspaceProvider(repoDir, runner)
	provider.workspaceBase = &base
	return provider
}

func (provider *Provider) ResolveWorkspaceComparison(ctx context.Context) (InputResolution, error) {
	if err := ctx.Err(); err != nil {
		return InputResolution{}, err
	}
	if base := provider.workspaceBase; base != nil {
		if base.ResolvedHead != "" || base.ExactRange != "" || (base.ResolvedBase == "" && !base.BaseIsEmpty) || (base.ResolvedBase != "" && base.BaseIsEmpty) {
			return InputResolution{}, fmt.Errorf("workspace comparison requires a resolved base or an empty tree")
		}
		return *base, nil
	}
	out, stderr, err := provider.runGitSplit(ctx, "rev-parse", "--verify", "--quiet", "--end-of-options", "HEAD^{commit}")
	if err == nil {
		base := InputResolution{ResolvedBase: strings.TrimSpace(out)}
		provider.workspaceBase = &base
		return base, nil
	}
	branch, _, branchErr := provider.runGitSplit(ctx, "symbolic-ref", "--quiet", "HEAD")
	branch = strings.TrimSpace(branch)
	if branchErr == nil && strings.HasPrefix(branch, "refs/heads/") {
		_, refStderr, refErr := provider.runGitSplit(ctx, "show-ref", "--verify", "--quiet", "--", branch)
		var exitErr *exec.ExitError
		if errors.As(refErr, &exitErr) && exitErr.ExitCode() == 1 && refStderr == "" {
			base := InputResolution{BaseIsEmpty: true}
			provider.workspaceBase = &base
			return base, nil
		}
	}
	if ctx.Err() != nil {
		return InputResolution{}, ctx.Err()
	}
	return InputResolution{}, gitFailure("resolve workspace HEAD", stderr, err)
}

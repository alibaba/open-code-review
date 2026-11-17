// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"fmt"
	"strings"
)

func (provider *Provider) ResolveCommitComparison(ctx context.Context) (InputResolution, error) {
	head := provider.resolveCommit(ctx, provider.commit)
	if head == "" {
		return InputResolution{}, fmt.Errorf("cannot resolve commit %q", provider.commit)
	}
	content, err := provider.runGit(ctx, "cat-file", "commit", head)
	if err != nil {
		return InputResolution{}, fmt.Errorf("read commit %s: %w", head, err)
	}
	resolution := InputResolution{ResolvedHead: head, BaseIsEmpty: true}
	for _, line := range strings.Split(content, "\n") {
		if line == "" {
			break
		}
		if parent, ok := strings.CutPrefix(line, "parent "); ok {
			if provider.resolveCommit(ctx, parent) != parent {
				return InputResolution{}, fmt.Errorf("comparison parent %s is unavailable", parent)
			}
			parents := provider.commitParents(ctx, head)
			if len(parents) == 0 || parents[0] != parent {
				return InputResolution{}, fmt.Errorf("comparison parent %s is not visible in Git history; deepen the clone before reviewing this commit", parent)
			}
			resolution.ResolvedBase = parent
			resolution.ExactRange = parent + ".." + head
			resolution.BaseIsEmpty = false
			break
		}
	}
	return resolution, nil
}

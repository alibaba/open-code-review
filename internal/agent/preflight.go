// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"fmt"
	"io"

	"github.com/alibaba/open-code-review/internal/stdout"
)

// EstimatePreflight computes the rough token projection for the work this run
// can actually dispatch, without creating a session or issuing any LLM request.
// It is intended for command-layer admission checks that must be able to reject
// a run before agent.New persists session_start.
//
// Admission is intentionally narrower than Run's legacy headline estimate:
// path/extension filters, the max_tokens oversized-diff filter, and reusable
// resume checkpoints are all applied first. A warning may conservatively count
// work that will later be skipped, but confirm/abort must not reject a run for
// work that runtime will never dispatch.
func EstimatePreflight(ctx context.Context, args Args) (Estimate, error) {
	// Selection filters report excluded paths through the package progress writer.
	// A preflight is an internal probe: the caller owns the one user-facing
	// prompt/warning, so suppress those duplicate progress lines. This runs on
	// the command goroutine before any workers exist, which is the safe usage
	// contract for stdout.Swap.
	restore := stdout.Swap(io.Discard)
	defer restore()

	a := &Agent{args: args}
	if err := a.loadDiffs(ctx); err != nil {
		return Estimate{}, fmt.Errorf("load diffs: %w", err)
	}
	a.diffs = a.filterDiffs(a.diffs)
	a.diffs = a.filterLargeDiffs(a.diffs)

	if args.Resume != nil {
		mode := a.reviewMode()
		pending := a.diffs[:0]
		for _, d := range a.diffs {
			if !d.IsDeleted {
				if _, ok := args.Resume.ReusableItem(reviewItemFingerprint(mode, d)); ok {
					continue
				}
			}
			pending = append(pending, d)
		}
		a.diffs = pending
	}

	return estimateDiffCost(a.diffs), nil
}

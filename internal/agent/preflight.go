// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"fmt"
	"io"

	"github.com/alibaba/open-code-review/internal/stdout"
)

// EstimatePreflight computes the same rough token projection that Run prints
// before a budgeted review, without creating a session or issuing any LLM
// request. It is intended for command-layer admission checks that must be able
// to reject a run before agent.New persists session_start.
//
// Keep the selection order in sync with Run's estimate: load the diff, apply
// path/extension filters, then estimate. Run deliberately prints its estimate
// before filterLargeDiffs, so this helper does the same rather than presenting a
// different number at the admission boundary.
func EstimatePreflight(ctx context.Context, args Args) (Estimate, error) {
	// filterDiffs reports excluded paths through the package progress writer.
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
	return estimateDiffCost(a.diffs), nil
}

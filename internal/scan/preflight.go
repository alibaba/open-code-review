// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"fmt"
	"io"

	"github.com/alibaba/open-code-review/internal/stdout"
)

// EstimatePreflight computes the same rough token projection that Run prints
// before dispatch, without creating a session or issuing any LLM request. It is
// intended for command-layer admission checks that can reject an expensive scan
// before NewAgent persists session_start.
func EstimatePreflight(ctx context.Context, args Args) (Estimate, error) {
	// Enumeration filters print progress in the normal run. A preflight is an
	// internal probe and the command layer owns the user-facing budget message,
	// so suppress those duplicate lines. The call happens before any scan worker
	// is started, which satisfies stdout.Swap's single-goroutine contract.
	restore := stdout.Swap(io.Discard)
	defer restore()

	provider := NewProvider(args.RepoDir, args.Paths, args.GitRunner, args.MaxFileSizeBytes)
	items, err := provider.Enumerate(ctx)
	if err != nil {
		return Estimate{}, fmt.Errorf("enumerate files: %w", err)
	}

	a := &Agent{args: args, items: items}
	a.items = a.filterScanItems(a.items)
	a.items = a.filterLargeScans(a.items)
	return estimateCost(a.items, a.planEnabled(), a.dedupEnabled(), a.summaryEnabled()), nil
}

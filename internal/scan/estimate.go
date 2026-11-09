// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"fmt"

	"github.com/alibaba/open-code-review/internal/estimate"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

// Estimate is a pre-run, order-of-magnitude projection of scan cost.
type Estimate struct {
	Files        int
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
}

// estimateFileTokens projects the input+output token cost of reviewing a
// single file (PLAN_TASK + MAIN_TASK rounds). Excludes the run-level dedup/
// summary phases. Returns 0 for files that are skipped before dispatch
// (binary / empty). Used both by the aggregate estimate and by the
// per-file budget look-ahead in dispatch.
func estimateFileTokens(it model.ScanItem, planEnabled bool, params estimate.Parameters) int64 {
	if it.IsBinary || it.Content == "" {
		return 0
	}
	fileTokens := int64(llm.CountTokens(it.Content))

	input, output := params.FileTokens(fileTokens, planEnabled)
	return input + output
}

// estimateCost projects token usage using the effective runtime phase toggles
// (template field present AND not disabled by a --no-* flag).
func estimateCost(items []model.ScanItem, planEnabled, dedupEnabled, summaryEnabled bool, params estimate.Parameters) Estimate {
	params = params.WithDefaults()
	var est Estimate
	var allCommentsApprox int64

	for i := range items {
		it := &items[i]
		if it.IsBinary || it.Content == "" {
			continue // skipped before dispatch
		}
		est.Files++
		fileTokens := int64(llm.CountTokens(it.Content))
		input, output := params.FileTokens(fileTokens, planEnabled)
		est.InputTokens += input
		est.OutputTokens += output

		// Rough comment yield used to size dedup/summary inputs downstream.
		allCommentsApprox += 3
	}

	// DEDUP_TASK: one call per batch; approximate as a single pass over all
	// comments (batches partition them, so total dedup input ≈ all comments).
	if dedupEnabled && allCommentsApprox > 0 {
		est.InputTokens += allCommentsApprox*120 + params.PromptOverheadTokens
		est.OutputTokens += allCommentsApprox * 20
	}

	// PROJECT_SUMMARY_TASK: one call over all comments.
	if summaryEnabled && allCommentsApprox > 0 {
		est.InputTokens += allCommentsApprox*120 + params.PromptOverheadTokens
		est.OutputTokens += 2000
	}

	est.TotalTokens = est.InputTokens + est.OutputTokens
	return est
}

// String renders a one-line human-readable estimate. Money is intentionally
// omitted — pricing varies per provider/model and we don't want to imply a
// precise dollar figure.
func (e Estimate) String() string {
	return fmt.Sprintf("~%d file(s), est. %s input + %s output ≈ %s total tokens (rough; actual reported after run)",
		e.Files, humanTokens(e.InputTokens), humanTokens(e.OutputTokens), humanTokens(e.TotalTokens))
}

// humanTokens formats a token count as e.g. "1.2M" / "850K" / "420".
func humanTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

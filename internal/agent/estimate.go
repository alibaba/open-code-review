// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"fmt"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

// Cost-estimation heuristics for the diff-review path. These mirror
// internal/scan/estimate.go deliberately: scan and review are two paths over
// the same underlying review model, and their estimates should be comparable.
// The values are intentionally re-declared here (rather than imported from
// scan) to avoid a new agent→scan cross-package dependency; keep them in sync
// if scan's change.
//
// As in scan, these are rough — their job is to give the user an
// order-of-magnitude warning before a large review, not to be
// billing-accurate. Real usage is always reported from the API response after
// the run. The estimate also cannot account for agent tool-use inflation
// because tool use can multiply the prompt substantially, so it is a floor.
const (
	// promptOverheadTokens approximates the fixed prompt scaffolding per LLM
	// call (system prompt + template wrappers + tool definitions).
	// KEEP IN SYNC with internal/scan/estimate.go.
	promptOverheadTokens = 2000
	// avgMainRoundsPerFile is the assumed number of MAIN_TASK tool-use rounds
	// for a typical file. Observed ~6 on real repos; round up.
	// KEEP IN SYNC with internal/scan/estimate.go.
	avgMainRoundsPerFile = 7
	// avgOutputTokensPerRound approximates completion tokens per round.
	// KEEP IN SYNC with internal/scan/estimate.go.
	avgOutputTokensPerRound = 700
)

// Estimate is a pre-run, order-of-magnitude projection of review cost. It
// mirrors scan.Estimate so callers can treat the two paths uniformly.
type Estimate struct {
	Files        int
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
}

// estimateDiffFileTokens projects the input+output token cost of reviewing a
// single diff (PLAN + MAIN_TASK rounds). It mirrors scan.estimateFileTokens
// but counts tokens of the diff text (d.Diff) rather than whole-file content,
// since the diff path reviews patches, not full files. Returns 0 for deleted
// files (they are skipped before dispatch and must not trip the gate). Used
// both by the aggregate estimate (estimateDiffCost) and by the per-file budget
// look-ahead in dispatchSubtasks.
func estimateDiffFileTokens(d model.Diff) int64 {
	if d.IsDeleted || d.Diff == "" {
		return 0
	}
	diffTokens := int64(llm.CountTokens(d.Diff))

	// PLAN phase input/output (small JSON output).
	total := diffTokens + promptOverheadTokens
	total += 400
	// MAIN_TASK: diff content carried across rounds + per-round overhead.
	total += (diffTokens + promptOverheadTokens) * avgMainRoundsPerFile
	total += avgOutputTokensPerRound * avgMainRoundsPerFile
	return total
}

// estimateDiffCost projects token usage for reviewing the given diffs. It is
// the diff-path analogue of scan.estimateCost and is used for the pre-review
// scale warning.
func estimateDiffCost(diffs []model.Diff) Estimate {
	var est Estimate
	for _, d := range diffs {
		if d.IsDeleted || d.Diff == "" {
			continue
		}
		est.Files++
		diffTokens := int64(llm.CountTokens(d.Diff))
		est.InputTokens += diffTokens + promptOverheadTokens
		est.OutputTokens += 400
		est.InputTokens += (diffTokens + promptOverheadTokens) * avgMainRoundsPerFile
		est.OutputTokens += avgOutputTokensPerRound * avgMainRoundsPerFile
	}
	est.TotalTokens = est.InputTokens + est.OutputTokens
	return est
}

// String renders a one-line human-readable estimate, matching scan.Estimate's
// format so diff and scan warnings read consistently. Money is intentionally
// omitted — pricing varies per provider/model and we don't imply a precise
// dollar figure.
func (e Estimate) String() string {
	return fmt.Sprintf("~%d file(s), est. %s input + %s output ≈ %s total tokens (rough; agent tool-use inflates this — actual reported after run)",
		e.Files, humanTokens(e.InputTokens), humanTokens(e.OutputTokens), humanTokens(e.TotalTokens))
}

// humanTokens formats a token count as e.g. "1.2M" / "850K" / "420".
// Mirrors internal/scan/estimate.go; kept here so agent has no scan import.
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

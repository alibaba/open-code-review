// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"fmt"

	"github.com/alibaba/open-code-review/internal/estimate"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
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
// both by the aggregate estimate (estimateDiffCost) and by the per-group budget
// look-ahead in dispatchSubtasks, which sums this over a group's diffs.
func estimateDiffFileTokens(d model.Diff, params estimate.Parameters) int64 {
	if d.IsDeleted || d.Diff == "" {
		return 0
	}
	diffTokens := int64(llm.CountTokens(d.Diff))

	input, output := params.FileTokens(diffTokens, true)
	return input + output
}

// estimateDiffCost projects token usage for reviewing the given diffs. It is
// the diff-path analogue of scan.estimateCost and is used for the pre-review
// scale warning.
func estimateDiffCost(diffs []model.Diff, params estimate.Parameters) Estimate {
	var est Estimate
	for _, d := range diffs {
		if d.IsDeleted || d.Diff == "" {
			continue
		}
		est.Files++
		diffTokens := int64(llm.CountTokens(d.Diff))
		input, output := params.FileTokens(diffTokens, true)
		est.InputTokens += input
		est.OutputTokens += output
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

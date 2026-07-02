package scan

import (
	"fmt"

	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/model"
)

// Cost-estimation heuristics. These are deliberately rough — their job is
// to give the user an order-of-magnitude warning before a large scan, not
// to be billing-accurate. Real usage is always reported from the API
// response after the run.
const (
	// promptOverheadTokens approximates the fixed prompt scaffolding per LLM
	// call (system prompt + template wrappers + tool definitions).
	promptOverheadTokens = 2000
	// avgMainRoundsPerFile is the assumed number of MAIN_TASK tool-use
	// rounds for a typical file. Observed ~6 on real repos; round up.
	avgMainRoundsPerFile = 7
	// avgOutputTokensPerRound approximates completion tokens per round.
	avgOutputTokensPerRound = 700
)

// Estimate is a pre-run, order-of-magnitude projection of scan cost.
type Estimate struct {
	Files        int
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
}

// estimateCost projects token usage for reviewing the given items under the
// supplied scan template. planEnabled / dedupEnabled / summaryEnabled
// reflect the effective runtime toggles (template field present AND not
// disabled by a --no-* flag).
// estimateFileTokens projects the input+output token cost of reviewing a
// single file (PLAN_TASK + MAIN_TASK rounds). Excludes the run-level dedup/
// summary phases. Returns 0 for files that are skipped before dispatch
// (binary / empty). Used both by the aggregate estimate and by the
// per-file budget look-ahead in dispatch.
func estimateFileTokens(it model.ScanItem, planEnabled bool) int64 {
	if it.IsBinary || it.Content == "" {
		return 0
	}
	fileTokens := int64(llm.CountTokens(it.Content))

	var total int64
	if planEnabled {
		total += fileTokens + promptOverheadTokens // PLAN input
		total += 400                               // PLAN output (small JSON)
	}
	// MAIN_TASK: file content carried across rounds + per-round overhead.
	total += (fileTokens + promptOverheadTokens) * avgMainRoundsPerFile
	total += avgOutputTokensPerRound * avgMainRoundsPerFile
	return total
}

func estimateCost(items []model.ScanItem, planEnabled, dedupEnabled, summaryEnabled bool) Estimate {
	var est Estimate
	var allCommentsApprox int64

	for i := range items {
		it := &items[i]
		if it.IsBinary || it.Content == "" {
			continue // skipped before dispatch
		}
		est.Files++
		// Per-file cost folds into InputTokens for the aggregate; we don't
		// split input/output here since the look-ahead only needs the total.
		// Recompute the input/output split inline to keep the headline
		// numbers meaningful.
		fileTokens := int64(llm.CountTokens(it.Content))
		if planEnabled {
			est.InputTokens += fileTokens + promptOverheadTokens
			est.OutputTokens += 400
		}
		est.InputTokens += (fileTokens + promptOverheadTokens) * avgMainRoundsPerFile
		est.OutputTokens += avgOutputTokensPerRound * avgMainRoundsPerFile

		// Rough comment yield used to size dedup/summary inputs downstream.
		allCommentsApprox += 3
	}

	// DEDUP_TASK: one call per batch; approximate as a single pass over all
	// comments (batches partition them, so total dedup input ≈ all comments).
	if dedupEnabled && allCommentsApprox > 0 {
		est.InputTokens += allCommentsApprox*120 + promptOverheadTokens
		est.OutputTokens += allCommentsApprox * 20
	}

	// PROJECT_SUMMARY_TASK: one call over all comments.
	if summaryEnabled && allCommentsApprox > 0 {
		est.InputTokens += allCommentsApprox*120 + promptOverheadTokens
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

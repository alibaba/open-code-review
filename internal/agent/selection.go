// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	allowedext "github.com/alibaba/open-code-review/internal/config/allowlist"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
)

// fileDecision is one changed file's pre-dispatch outcome: the diff, the
// reason it was excluded (ExcludeNone when it is selected for review), and the
// raw diff's token count when the size gate actually counted it.
type fileDecision struct {
	Diff       model.Diff
	Reason     ExcludeReason
	DiffTokens int
}

// selected reports whether the file enters the review's selected set — the
// same set registerCoverage seals and preview reports as will_review.
func (d fileDecision) selected() bool { return d.Reason == ExcludeNone }

// retained reports whether the file stays in the agent's working diff set.
// Deletions are retained although they are never dispatched: they carry no new
// content to review but still belong to the change-file list the prompts show.
func (d fileDecision) retained() bool {
	return d.Reason == ExcludeNone || d.Reason == ExcludeDeleted
}

// selectFiles is the review's one deterministic pre-dispatch selection: for
// every changed file it applies the static path/extension gates, the deletion
// check and the per-file diff-size ceiling, returning one decision per input
// diff in input order.
//
// It is pure — no output, no mutation, no git or LLM access — so `--preview`
// and the real run consume the same answer instead of each deriving their own,
// which is what let the two drift (#782). Everything that cannot be known
// before dispatch stays out: aggregate budget exhaustion, resume reuse and
// provider failures are execution outcomes, not selection.
func (a *Agent) selectFiles(diffs []model.Diff) []fileDecision {
	limit := llmloop.PromptTokenLimit(a.args.Template.MaxTokens)
	decisions := make([]fileDecision, 0, len(diffs))
	for _, d := range diffs {
		dec := fileDecision{Diff: d, Reason: a.whyExcluded(d)}
		switch {
		case dec.Reason != ExcludeNone:
		case d.IsDeleted:
			dec.Reason = ExcludeDeleted
		case limit > 0:
			// Counted only when a limit is configured, so the tokenizer pass is
			// skipped entirely for max_tokens=0.
			dec.DiffTokens = llm.CountTokens(d.Diff)
			if dec.DiffTokens > limit {
				dec.Reason = ExcludeTooLarge
			}
		}
		decisions = append(decisions, dec)
	}
	return decisions
}

// whyExcluded applies the static gates — binary, user exclude/include, then
// the extension allowlist and default-path patterns — and returns the specific
// reason a file is excluded, or ExcludeNone when it survives all of them. It
// answers only what the path and the diff header say; the size gate lives in
// selectFiles because it needs the resolved token limit.
func (a *Agent) whyExcluded(d model.Diff) ExcludeReason {
	if d.IsBinary {
		return ExcludeBinary
	}

	path := effectivePath(d)
	f := a.args.FileFilter

	if f != nil && f.IsUserExcluded(path) {
		return ExcludeUserRule
	}

	if f != nil && f.HasInclude() && f.IsUserIncluded(path) {
		return ExcludeNone
	}

	ext := a.extFromPath(path)
	if ext != "" && !allowedext.IsAllowedExt(ext) {
		return ExcludeExtension
	}

	if allowedext.IsExcludedPath(path) {
		return ExcludeDefaultPath
	}

	return ExcludeNone
}

// selectionCounts is the run-level rollup of one selection pass.
type selectionCounts struct {
	Selected int // files that enter the selected set
	TooLarge int // files dropped by the per-file diff-size ceiling
}

// summarizeSelection returns the diffs the run keeps working with, plus the
// counts Run reports.
func summarizeSelection(decisions []fileDecision) ([]model.Diff, selectionCounts) {
	var counts selectionCounts
	kept := make([]model.Diff, 0, len(decisions))
	for _, dec := range decisions {
		if dec.retained() {
			kept = append(kept, dec.Diff)
		}
		switch {
		case dec.selected():
			counts.Selected++
		case dec.Reason == ExcludeTooLarge:
			counts.TooLarge++
		}
	}
	return kept, counts
}

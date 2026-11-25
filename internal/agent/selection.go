// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"strings"

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
// Deletions and unchanged files are retained although they are never
// dispatched: they carry no new content to review but still belong to the
// change-file list the prompts show, so a reviewer of another file still sees
// that this one moved or went away.
func (d fileDecision) retained() bool {
	return d.Reason == ExcludeNone || d.Reason == ExcludeDeleted || d.Reason == ExcludeUnchanged
}

// dispatchable reports whether a retained diff has new content to review.
// Every step after selection that sends files to the model or counts them as
// reviewed uses this, so a deleted or unchanged file can stay in the prompt
// context without being dispatched, registered for coverage or resumed.
func dispatchable(d model.Diff) bool {
	return !d.IsDeleted && !hasNoHunk(d)
}

// hasNoHunk reports whether a git diff section consists of headers only: git
// writes no "@@" hunk for a pure rename, a mode-only change or an empty new
// file. Only text that starts with the "diff --git" line the parser keeps is
// judged; anything else says nothing about hunks and is not treated as
// unchanged.
func hasNoHunk(d model.Diff) bool {
	if !strings.HasPrefix(d.Diff, "diff --git ") {
		return false
	}
	return !strings.Contains(d.Diff, "\n@@")
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
		case hasNoHunk(d):
			dec.Reason = ExcludeUnchanged
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

// whyExcluded applies the static gates — binary, the built-in secret paths,
// user exclude/include, then the extension allowlist and default-path patterns
// — and returns the specific reason a file is excluded, or ExcludeNone when it
// survives all of them. It answers only what the path and the diff header say;
// the size gate lives in selectFiles because it needs the resolved token limit.
func (a *Agent) whyExcluded(d model.Diff) ExcludeReason {
	if d.IsBinary {
		return ExcludeBinary
	}

	path := effectivePath(d)
	f := a.args.FileFilter

	// Ahead of both user rules: no include glob can admit a credential path,
	// and no user exclude can claim it under a different reason.
	if allowedext.IsSecretPath(d.OldPath) || allowedext.IsSecretPath(d.NewPath) {
		return ExcludeSecret
	}

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
	Selected  int // files that enter the selected set
	TooLarge  int // files dropped by the per-file diff-size ceiling
	Unchanged int // renamed or mode-changed files with no hunk to review
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
		case dec.Reason == ExcludeUnchanged:
			counts.Unchanged++
		}
	}
	return kept, counts
}

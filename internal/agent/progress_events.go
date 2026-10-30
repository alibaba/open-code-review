// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"github.com/alibaba/open-code-review/internal/progress"
)

// This file publishes the structured half of a review run's progress. The
// plain [ocr] lines the pipeline has always printed are emitted separately, at
// the point where the fact becomes known; these functions publish the same
// facts in a shape a live dashboard can render as rows and counters.
//
// The key is always fileGroupKey, never FileGroup.Label: Label is free-form
// text an LLM produced, while the key is derived from the paths and is the
// same string the tool loop reports tool calls under. Keying the dashboard on
// it is what lets a `▶ file_read` line be attributed to the row that caused
// it.

// publishPlanned reports the run's coverage denominator and its cost estimate.
// It is published once selection has settled, before any dispatch, so the
// dashboard's bar starts at a real total rather than growing out of an unknown
// one, and the estimate is on screen for the whole run rather than scrolling
// away in the log.
//
// A review does not group its work into batches, so it publishes no strategy
// and no group count.
func publishPlanned(files int, est Estimate) {
	progress.Publish(progress.Event{
		Kind:  progress.KindPlanned,
		Files: files,
		Plan: &progress.Plan{
			InputTokens:  est.InputTokens,
			OutputTokens: est.OutputTokens,
		},
	})
}

// publishGroupStart announces a group entering the concurrent work pool.
func publishGroupStart(g FileGroup) {
	paths := make([]string, 0, len(g.Diffs))
	for _, d := range g.Diffs {
		paths = append(paths, d.NewPath)
	}
	progress.Publish(progress.Event{
		Kind:  progress.KindGroupStart,
		Group: fileGroupKey(g.Diffs),
		Paths: paths,
	})
}

// publishGroupDone reports a group that completed every file it was given.
// findings is passed in rather than counted here: the caller has just read
// every file's comments, and CommentsForPath allocates a fresh slice per call,
// so recounting would be a second pass over the whole comment store.
func (a *Agent) publishGroupDone(g FileGroup, findings int) {
	publishGroupDoneFiles(fileGroupKey(g.Diffs), len(g.Diffs), findings)
}

// publishGroupDoneFiles reports the files of a group that completed. It takes
// the file count explicitly because a group that stopped short accounts for
// its files in two events: the ones that finished, and the ones that did not.
func publishGroupDoneFiles(key string, files, findings int) {
	progress.Publish(progress.Event{
		Kind:     progress.KindGroupDone,
		Group:    key,
		Files:    files,
		Findings: findings,
	})
}

// publishGroupFailed reports a group that ended in an error. The file count is
// the whole group, since none of its files completed.
func publishGroupFailed(key string, files int, reason string) {
	progress.Publish(progress.Event{
		Kind:  progress.KindGroupFailed,
		Group: key,
		Files: files,
		Err:   reason,
	})
}

// publishRoundStart announces an LLM round within a group.
func publishRoundStart(key string, round int) {
	progress.Publish(progress.Event{
		Kind:  progress.KindRoundStart,
		Group: key,
		Round: round,
	})
}

// publishBudgetReached reports that dispatch has stopped because the run hit
// a budget. The dashboard promotes this into its status line, because it is
// the one event that changes what the finished report will say.
//
// The event carries no text: the run already printed the budget line where
// the fact became known, and the plain-text sink would print it a second
// time here. Piped runs are promised the same bytes they always had.
func publishBudgetReached() {
	progress.Publish(progress.Event{Kind: progress.KindBudgetReached})
}

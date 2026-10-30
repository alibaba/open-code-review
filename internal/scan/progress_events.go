// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/progress"
)

// This file publishes the structured half of a scan run's progress, mirroring
// what internal/agent does for review. The plain [ocr] lines are emitted where
// the facts become known; these carry the same facts in the shape a live
// dashboard renders as rows and counters.
//
// A scan's unit of work is a single file, so its key is simply the path —
// there is no grouping step to derive a composite key from.

// publishPlanned reports the run's coverage denominator, its cost estimate
// and how the work is grouped, published once selection has settled so the
// dashboard's bar starts at a real total and the estimate is on screen for
// the whole run.
func publishPlanned(files int, est Estimate, strategy string, groups int) {
	progress.Publish(progress.Event{
		Kind:  progress.KindPlanned,
		Files: files,
		Plan: &progress.Plan{
			InputTokens:  est.InputTokens,
			OutputTokens: est.OutputTokens,
			Strategy:     strategy,
			Groups:       groups,
		},
	})
}

// publishItemStart announces a file entering the concurrent work pool. The
// batch is the group of files the run dispatches together, so the row can say
// which part of the scan it is working on.
func publishItemStart(it model.ScanItem, batch string) {
	progress.Publish(progress.Event{
		Kind:  progress.KindGroupStart,
		Group: it.Path,
		Paths: []string{it.Path},
		Plan:  &progress.Plan{Batch: batch},
	})
}

// publishItemDone reports a file that finished, with the findings it produced.
func publishItemDone(it model.ScanItem, findings int) {
	progress.Publish(progress.Event{
		Kind:     progress.KindGroupDone,
		Group:    it.Path,
		Files:    1,
		Findings: findings,
	})
}

// publishItemFailed reports a file that ended in an error or was cut short.
func publishItemFailed(it model.ScanItem, reason string) {
	progress.Publish(progress.Event{
		Kind:  progress.KindGroupFailed,
		Group: it.Path,
		Files: 1,
		Err:   reason,
	})
}

// publishBudgetReached reports that dispatch stopped because the run hit a
// budget. The dashboard promotes this into its status line.
func publishBudgetReached() {
	progress.Publish(progress.Event{Kind: progress.KindBudgetReached})
}

// publishReusedUpfront reports the resume total before any dispatch starts.
//
// The resume line has already told the reader how many files are carried over,
// so the bar has to agree with it immediately. Publishing per file as each
// batch reached its reused items made the bar climb one at a time while the
// log already showed the final count — two surfaces disagreeing about the
// same fact.
func (a *Agent) publishReusedUpfront() {
	if a.args.Resume == nil {
		return
	}
	reused, findings := 0, 0
	for _, it := range a.items {
		// The cached method, not the package-level function: it looks the
		// fingerprint up in the map initScanFingerprints already built,
		// instead of re-hashing every file's bytes for a lookup that
		// initResumeInfo has already made the same way.
		item, ok := a.resumeItem(a.scanItemFingerprint(it))
		if !ok {
			continue
		}
		reused++
		findings += len(item.Comments)
	}
	progress.Publish(progress.Event{
		Kind:     progress.KindReused,
		Files:    reused,
		Findings: findings,
	})
}

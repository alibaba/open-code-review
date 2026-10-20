// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
)

var errFocusedReviewPanic = errors.New("focused review panicked")

func (a *Agent) reviewFocusCount() int {
	return min(3, max(1, a.args.IntraGroupConcurrency))
}

func reviewFocuses(count int) []string {
	if count == 2 {
		return []string{
			"Security and authorization, including trust boundaries, validation, isolation and sensitive data.",
			"Correctness, compatibility, data integrity, error handling, concurrency and performance.",
		}
	}
	return []string{
		"Security and authorization, including trust boundaries, validation, isolation and sensitive data.",
		"Correctness, compatibility, data integrity and error handling.",
		"Concurrency, resource lifecycle, performance and other substantive defects outside the other focuses.",
	}
}

func (a *Agent) runFocusedReview(ctx context.Context, messages []llm.Message, groupKey string) (bool, llmloop.MainLoopStop, *subtaskStop, error) {
	type result struct {
		completed bool
		stop      llmloop.MainLoopStop
		err       error
	}
	focuses := reviewFocuses(a.reviewFocusCount())
	results := make([]result, len(focuses))
	prompts := make([][]llm.Message, len(focuses))
	for i, focus := range focuses {
		prompts[i] = append(slices.Clone(messages), llm.NewTextMessage("user",
			"This is a focused part of a parallel first-round review. Your focus: "+focus+
				" Other reviewers cover the remaining concerns. Investigate this focus across the entire diff and its dependencies; "+
				"the plan is guidance, not a coverage limit. Follow the existing review rules and report only substantiated findings. "+
				"Include the actual file path in every code_comment. Finish with task_done when this focus is covered."))
		if stop := a.checkPromptBudget(ctx, prompts[i], groupKey, 1); stop != nil {
			return false, llmloop.StopNone, stop, nil
		}
	}
	var wg sync.WaitGroup
	for i := range focuses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if p := recover(); p != nil {
					results[i].err = fmt.Errorf("%w (focus %d): %v", errFocusedReviewPanic, i+1, p)
				}
			}()
			key := fmt.Sprintf("%s [focus %d]", groupKey, i+1)
			if a.args.CommentWorkerPool != nil {
				defer a.args.CommentWorkerPool.AwaitKey(key)
			}
			results[i].completed, results[i].stop, results[i].err =
				a.runner.RunMainTaskWithFallbackPath(ctx, prompts[i], key, groupKey)
		}()
	}
	wg.Wait()
	budgetStopped := false
	for _, r := range results {
		if r.stop == llmloop.StopTokenBudget {
			budgetStopped = true
		}
	}
	if budgetStopped && a.budgetExceeded.CompareAndSwap(false, true) {
		a.recordWarning("token_budget_reached", groupKey,
			fmt.Sprintf("stopped focused review of group %q: used %d tokens exceeds budget %d", groupKey, a.runner.TotalTokensUsed(), a.args.MaxTokensBudget))
	}
	if err := ctx.Err(); err != nil {
		return false, llmloop.StopNone, nil, err
	}
	// A successful sibling cannot turn an incomplete focus into full coverage.
	for _, r := range results {
		if r.err != nil {
			return false, r.stop, nil, r.err
		}
	}
	if budgetStopped {
		return false, llmloop.StopTokenBudget, nil, nil
	}
	for _, r := range results {
		if !r.completed {
			return false, r.stop, nil, nil
		}
	}
	return true, llmloop.StopNone, nil, nil
}

func (a *Agent) removeExactGroupDuplicates(g FileGroup, baseline map[string]int) {
	for _, d := range g.Diffs {
		seen := make(map[model.LlmComment]struct{})
		remove := make(map[int]struct{})
		for i, comment := range a.args.CommentCollector.CommentsForPath(d.NewPath) {
			// Reasoning can differ even when the public finding is identical.
			comment.Thinking = ""
			if _, exists := seen[comment]; exists && i >= baseline[d.NewPath] {
				remove[i] = struct{}{}
			}
			seen[comment] = struct{}{}
		}
		a.args.CommentCollector.RemoveByPathAndIndices(d.NewPath, remove)
	}
}

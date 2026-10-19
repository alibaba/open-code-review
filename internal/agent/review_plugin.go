// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/plugin"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type PluginReviewReport = session.PluginReviewReport

var errReviewPlugin = errors.New("review plugin failed")

func (a *Agent) PluginReviewReports() []PluginReviewReport {
	a.pluginReviewMu.Lock()
	out := append([]PluginReviewReport(nil), a.pluginReviewReports...)
	a.pluginReviewMu.Unlock()
	// Stages are appended in package order within each group.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Group < out[j].Group })
	return out
}

func requiredStop(class session.FailureClass, reason string) *subtaskStop {
	return &subtaskStop{required: true, class: class, reason: reason, checkpoint: reason}
}

func (a *Agent) executePluginReview(ctx context.Context, g FileGroup) (bool, *subtaskStop, error) {
	p := a.args.ReviewPlugin
	groupKey := fileGroupKey(g.Diffs)
	paths := make([]string, 0, len(g.Diffs))
	for _, d := range g.Diffs {
		paths = append(paths, d.NewPath)
	}
	slices.Sort(paths)
	client, err := plugin.StartReview(ctx, *p, a.args.RepoDir)
	if err != nil {
		return false, nil, fmt.Errorf("%w: %w", errReviewPlugin, err)
	}
	defer client.Close()
	stages := p.Manifest.Review.Stages
	severities, categories := tool.CodeCommentClassifications()
	for i, stage := range stages {
		baseline := make(map[string]int, len(g.Diffs))
		for _, d := range g.Diffs {
			baseline[d.NewPath] = len(a.args.CommentCollector.CommentsForPath(d.NewPath))
		}
		report := PluginReviewReport{Plugin: p.Manifest.ID, Group: groupKey, Stage: stage}
		if budget := a.args.MaxTokensBudget; budget > 0 {
			used := a.runner.TotalTokensUsed()
			report.TokenLimit = min(budget, used+max(int64(1), (budget-used)/int64(len(stages)-i)))
		}
		var stageStop *subtaskStop
		var stageErr, completionErr, transportErr error
		var accepted *plugin.ReviewResponse
		begin, beginErr := client.Call(plugin.ReviewRequest{Method: "begin", Stage: stage, Paths: paths, Severities: severities, Categories: categories})
		report.Report = begin.Report
		if beginErr != nil {
			stageErr = fmt.Errorf("%w: %v", errReviewPlugin, beginErr)
		} else if budget := a.args.MaxTokensBudget; budget > 0 && a.runner.TotalTokensUsed() >= budget {
			if a.budgetExceeded.CompareAndSwap(false, true) {
				a.recordWarning("token_budget_reached", g.Diffs[0].NewPath,
					fmt.Sprintf("required %s stage not started: used %d tokens reaches budget %d", stage, a.runner.TotalTokensUsed(), budget))
			}
			stageStop = requiredStop(session.FailureBudget, "required "+stage+" stage not started: aggregate token budget exhausted")
		} else {
			messages := a.buildMainTaskMessages(a.resolveGroupSystemRule(g.Diffs), a.buildChangeFilesExceptGroup(g.Diffs), buildConcatenatedDiffs(g.Diffs), "", "")
			messages = append(messages, llm.NewTextMessage("system", begin.Task.Instructions), llm.NewTextMessage("user", begin.Task.Input))
			if stop := a.checkPromptBudget(ctx, messages, groupKey, i+1); stop != nil {
				stop.required = true
				stageStop = stop
			} else {
				completed, stop, runErr := a.runner.RunMainTaskWithOptions(ctx, messages, groupKey, llmloop.TaskOptions{
					Tools:      pluginReviewTools(a.args.MainToolDefs, begin.Task.Completion),
					TokenLimit: report.TokenLimit, FrozenMessageCount: len(messages),
					Complete: func(args map[string]any) ([]model.LlmComment, error) {
						response, callErr := client.Call(plugin.ReviewRequest{Method: "submit", Stage: stage, Arguments: args})
						if json.Valid(response.Report) {
							report.Report = response.Report
						}
						if callErr != nil {
							transportErr = callErr
							return nil, fmt.Errorf("%w: %v", llmloop.ErrCompletionUnavailable, callErr)
						}
						if !*response.Accepted {
							completionErr = fmt.Errorf("%s", response.Feedback)
							return nil, completionErr
						}
						if err := validatePluginComments(response.Comments, paths); err != nil {
							transportErr = err
							return nil, fmt.Errorf("%w: %v", llmloop.ErrCompletionUnavailable, err)
						}
						completionErr = nil
						accepted = &response
						return response.Comments, nil
					},
				})
				stageErr = runErr
				if transportErr != nil {
					stageErr = fmt.Errorf("%w: %v", errReviewPlugin, transportErr)
				}
				if ctx.Err() != nil {
					stageErr = ctx.Err()
				}
				if stop == llmloop.StopTokenBudget || (a.args.MaxTokensBudget > 0 && a.runner.TotalTokensUsed() > a.args.MaxTokensBudget) {
					if a.budgetExceeded.CompareAndSwap(false, true) {
						a.recordWarning("token_budget_reached", g.Diffs[0].NewPath,
							fmt.Sprintf("required %s stage reached aggregate token budget: used %d tokens, budget %d", stage, a.runner.TotalTokensUsed(), a.args.MaxTokensBudget))
					}
				}
				if !completed && stageErr == nil {
					class, reason := classifyMainLoopStop(stop)
					if completionErr != nil {
						reason += "; last completion report rejected: " + completionErr.Error()
					}
					stageStop = requiredStop(class, "required "+stage+" stage incomplete: "+reason)
				}
			}
		}
		if ctx.Err() != nil {
			stageErr = ctx.Err()
		}
		if a.args.CommentWorkerPool != nil {
			a.args.CommentWorkerPool.AwaitKey(groupKey)
		}
		// Accepted findings retain the normal filter even when required work is
		// incomplete. A filter result cannot close the plugin's remaining work.
		if accepted != nil && len(accepted.Comments) > 0 && stageErr == nil {
			a.executeGroupReviewFilter(ctx, g, baseline)
		}
		if stageErr != nil {
			report.Error = "required plugin stage failed; see session error"
			if completionErr != nil {
				report.Error += "; last completion report rejected: " + completionErr.Error()
			}
		} else if stageStop != nil {
			report.Error = stageStop.reason
		} else if accepted == nil || !*accepted.Complete {
			reason := "required plugin stage declared incomplete"
			if accepted != nil && accepted.Feedback != "" {
				reason = accepted.Feedback
			}
			stageStop = requiredStop(session.FailureUnknown, reason)
			report.Error = reason
		} else {
			report.Complete = true
		}
		a.pluginReviewMu.Lock()
		a.pluginReviewReports = append(a.pluginReviewReports, report)
		a.pluginReviewMu.Unlock()
		if err := a.session.RecordPluginReview(report); err != nil {
			stageErr = errors.Join(stageErr, fmt.Errorf("persist plugin report: %w", err))
		}
		if stageErr != nil {
			return false, nil, stageErr
		}
		if stageStop != nil {
			return false, stageStop, nil
		}
	}
	return true, nil, nil
}

func pluginReviewTools(defs []llm.ToolDef, completion plugin.ReviewCompletion) []llm.ToolDef {
	out := make([]llm.ToolDef, 0, len(defs))
	for _, def := range defs {
		if def.Function.Name != "task_done" && def.Function.Name != "code_comment" {
			out = append(out, def)
		}
	}
	return append(out, llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: completion.Description, Parameters: completion.Parameters}})
}

func validatePluginComments(comments []model.LlmComment, paths []string) error {
	severities, categories := tool.CodeCommentClassifications()
	for i, comment := range comments {
		switch {
		case !slices.Contains(paths, comment.Path):
			return fmt.Errorf("plugin finding %d: path %q is not selected", i+1, comment.Path)
		case strings.TrimSpace(comment.Content) == "":
			return fmt.Errorf("plugin finding %d: content must not be empty", i+1)
		case !slices.Contains(severities, comment.Severity):
			return fmt.Errorf("plugin finding %d: unsupported severity %q", i+1, comment.Severity)
		case !slices.Contains(categories, comment.Category):
			return fmt.Errorf("plugin finding %d: unsupported category %q", i+1, comment.Category)
		}
	}
	return nil
}

func (a *Agent) reviewRounds() int {
	if a.args.ReviewPlugin != nil {
		return len(a.args.ReviewPlugin.Manifest.Review.Stages)
	}
	return a.args.Template.ReviewRounds()
}

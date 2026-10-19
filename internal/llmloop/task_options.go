// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// TaskOptions scopes a tool set and completion contract to one conversation.
// Complete validates a successful task_done before optionally publishing findings
// through the normal code_comment pipeline. Returning an error asks the model to
// repair its report within the existing request budget.
type TaskOptions struct {
	Tools    []llm.ToolDef
	Complete func(map[string]any) ([]model.LlmComment, error)
	// FrozenMessageCount preserves the initial task inputs verbatim during
	// compression. Zero keeps the ordinary two-message prefix. This is scoped
	// to the conversation, including its asynchronous compression jobs.
	FrozenMessageCount int
	// TokenLimit is the soft end of this task's share of aggregate usage.
	// Zero uses the run budget alone. Closing still obeys the global budget gate.
	TokenLimit int64
}

type taskOptionsKey struct{}
type acceptedCommentsKey struct{}

// ErrCompletionUnavailable stops a task when its validator cannot run. Invalid
// model submissions return ordinary errors and remain eligible for correction.
var ErrCompletionUnavailable = errors.New("completion handler unavailable")

const scopedCompletionPrompt = "Submit the required task_done report now. Address every required item and any validation errors. Do not invent missing evidence; record incomplete work as required by the task. Only task_done is available. No further investigation is allowed."

const scopedClosingRequests = 3 // Two ordinary requests and the existing grace.

// completionDue reserves submission and correction requests within a scoped
// task's share. Account for output carried into subsequent prompts as well as
// tool definitions. This remains an estimate: tool feedback, concurrent groups
// and asynchronous compression can consume additional allowance.
func (r *Runner) completionDue(ctx context.Context, roundsLeft int, lastPromptTokens int64, messages []llm.Message) bool {
	opts, _ := ctx.Value(taskOptionsKey{}).(*TaskOptions)
	if opts == nil {
		return false
	}
	if roundsLeft < scopedClosingRequests {
		return true
	}
	limit := r.deps.MaxTokensBudget
	if opts.TokenLimit > 0 && (limit <= 0 || opts.TokenLimit < limit) {
		limit = opts.TokenLimit
	}
	if limit <= 0 {
		return false
	}
	tools, _ := json.Marshal(opts.Tools)
	prompt := max(lastPromptTokens, int64(CountMessagesTokens(messages)+llm.CountTokens(string(tools))))
	// Close before another investigation request would consume the reserve.
	const requests = scopedClosingRequests + 1
	output := llm.CompletionTokenBudget(r.deps.LLMClient, r.deps.Template.CompletionTokenLimit())
	if int64(output) > limit/(requests*(requests+1)/2) {
		return true // The output allowance alone exceeds the share; avoid overflow.
	}
	reserve := requests*prompt + requests*(requests+1)/2*int64(output)
	return limit-r.TotalTokensUsed() <= reserve
}

func completionContext(ctx context.Context) context.Context {
	opts, _ := ctx.Value(taskOptionsKey{}).(*TaskOptions)
	if opts == nil {
		return ctx
	}
	closing := *opts
	closing.Tools = nil
	for _, def := range opts.Tools {
		if def.Function.Name == "task_done" {
			closing.Tools = append(closing.Tools, def)
		}
	}
	return context.WithValue(ctx, taskOptionsKey{}, &closing)
}

// RunMainTaskWithOptions uses the shared runner without changing other groups'
// tools, counters, request identity, or completion requirements.
func (r *Runner) RunMainTaskWithOptions(ctx context.Context, messages []llm.Message, taskKey string, opts TaskOptions) (bool, MainLoopStop, error) {
	return r.RunMainTask(context.WithValue(ctx, taskOptionsKey{}, &opts), messages, taskKey)
}

func (r *Runner) taskToolDefs(ctx context.Context) []llm.ToolDef {
	if opts, _ := ctx.Value(taskOptionsKey{}).(*TaskOptions); opts != nil {
		return opts.Tools
	}
	return r.deps.MainToolDefs
}

func (r *Runner) completeTask(ctx context.Context, taskKey string, args map[string]any, rec *session.TaskRecord, thinking string) tool.TaskCheckpoint {
	opts, _ := ctx.Value(taskOptionsKey{}).(*TaskOptions)
	if opts == nil || opts.Complete == nil {
		return tool.Complete()
	}
	comments, err := opts.Complete(args)
	if err != nil {
		if errors.Is(err, ErrCompletionUnavailable) {
			return tool.Fail(err.Error())
		}
		message := fmt.Sprintf("Error: completion report rejected: %v", err)
		if rec != nil {
			rec.AddToolResult("task_done", "", message)
		}
		return tool.Of(message)
	}
	if len(comments) > 0 {
		data, err := json.Marshal(map[string]any{"comments": comments})
		if err != nil {
			return tool.Fail(fmt.Sprintf("encode accepted comments: %v", err))
		}
		// This internal publication is authorized by the completion contract;
		// the model cannot bypass the validator by calling code_comment directly.
		publishCtx := context.WithValue(ctx, taskOptionsKey{}, (*TaskOptions)(nil))
		publishCtx = context.WithValue(publishCtx, acceptedCommentsKey{}, true)
		cp := r.executeToolCall(publishCtx, taskKey, llm.ToolCall{
			Type: "function", Function: llm.FunctionCall{Name: "code_comment", Arguments: string(data)},
		}, rec, thinking)
		if cp.Failed || cp.Data != tool.CommentSucceed {
			return tool.Fail("accepted comments could not be published: " + cp.Data)
		}
	}
	return tool.Complete()
}

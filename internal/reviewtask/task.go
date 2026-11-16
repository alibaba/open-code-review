// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package reviewtask

import (
	"context"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type Deps struct {
	LLMClient         llm.LLMClient
	Model             string
	Template          template.Template
	Tools             *tool.Registry
	MainToolDefs      []llm.ToolDef
	CommentCollector  *tool.CommentCollector
	CommentWorkerPool *CommentWorkerPool
	Session           *session.SessionHistory
	// DiffLookup is consulted by the code_comment tool path to resolve
	// line numbers against the file's diff (or against full file content
	// in scan mode — scan adapters return a synthetic Diff whose
	// NewFileContent is the whole file and Diff is empty).
	DiffLookup func(path string) *model.Diff

	// AllDiffs returns every diff this run reviews, for re-filing a comment
	// whose ExistingCode belongs to a different file than the one it was filed
	// against (diff.RelocateAcrossFiles). It is the reviewed set rather than
	// every parsed diff on purpose: re-filing a comment onto a path the run
	// excluded would point the reader at a file this review never covered.
	// When nil, cross-file re-filing is skipped and only same-file resolution
	// applies.
	AllDiffs func() []model.Diff

	// NewRequestMeta builds the retry-report identity for one logical LLM
	// request. Non-nil only for review: the retry report describes ocr review,
	// and this Runner is shared with scan (internal/scan.Agent calls RunMainTask),
	// so main_task, memory compression and re-location all run under both modes.
	//
	// The gate has to be this field rather than a Provider string, because an
	// empty provider is a legitimate value for an unnamed endpoint — it cannot
	// double as "identity disabled". Leaving it nil (what scan does) keeps every
	// request exactly as it was before request identity existed.
	//
	// requestNo must be the RequestNo of the session.TaskRecord already created
	// for this request, so the report joins against the session JSONL.
	NewRequestMeta func(filePath string, taskType session.TaskType, requestNo int) llm.RequestMeta

	// MaxTokensBudget, when > 0, is the run's aggregate token budget
	// (input+output across every LLM call on this Runner). RunMainTask checks
	// it before each round and stops with StopTokenBudget once the running
	// total is over it, so a single long conversation cannot outrun a budget
	// that the caller's dispatch gate only evaluates between subtasks. 0 =
	// unlimited, which is also what scan and every existing caller get by
	// default.
	MaxTokensBudget int64
}

type Runner struct {
	*llmloop.Runner
	deps              Deps
	toolFailureStreak toolFailureStreakState
}

func NewRunner(deps Deps) *Runner {
	runner := &Runner{deps: deps}
	messages := make([]llmloop.MessageTemplate, 0, len(deps.Template.MemoryCompressionTask.Messages))
	for _, message := range deps.Template.MemoryCompressionTask.Messages {
		messages = append(messages, llmloop.MessageTemplate{Role: message.Role, Content: message.Content})
	}
	runner.Runner = llmloop.NewRunner(llmloop.Deps{
		LLMClient:       deps.LLMClient,
		Model:           deps.Model,
		Session:         deps.Session,
		NewRequestMeta:  deps.NewRequestMeta,
		MaxTokensBudget: deps.MaxTokensBudget,
	}, llmloop.Task{
		ToolDefs: deps.MainToolDefs,
		ExecuteTool: func(ctx context.Context, key string, call llm.ToolCall, record *session.TaskRecord, thinking string) llmloop.ToolOutcome {
			checkpoint := runner.executeToolCall(ctx, key, call, record, thinking)
			action := llmloop.Continue
			if checkpoint.Failed {
				action = llmloop.Fail
			} else if checkpoint.Completed {
				action = llmloop.Complete
			}
			content := checkpoint.Data
			if action == llmloop.Complete {
				content = "Task completed successfully."
			}
			return llmloop.ToolOutcome{Content: content, Action: action}
		},
		MaxRounds:            deps.Template.MaxToolRequestTimes,
		MaxContextTokens:     deps.Template.MaxTokens,
		MaxCompletionTokens:  deps.Template.CompletionTokenLimit(),
		EmptyResponseMessage: "You did not successfully call any tools. Please try again or use task_done if finished.",
		Compression: llmloop.CompressionPolicy{
			Messages:        messages,
			SummaryTemplate: "\n\n<previous_review_summary>\n{{summary}}\n</previous_review_summary>",
		},
		Grace: llmloop.GracePolicy{
			Tools: graceRoundToolDefs(deps.MainToolDefs),
			Messages: []llm.Message{llm.NewTextMessage("user",
				"Your review budget is exhausted. This is your FINAL round. You may ONLY:\n"+
					"- Call code_comment to submit any findings you have identified but not yet reported.\n"+
					"- Call task_done if you have nothing more to report.\n"+
					"No other tools are available. Do not attempt further analysis.")},
		},
	})
	return runner
}

func (r *Runner) RunMainTask(ctx context.Context, messages []llm.Message, taskKey string) (bool, llmloop.MainLoopStop, error) {
	result, err := r.Run(ctx, messages, taskKey)
	if result.StopReason == llmloop.StopMaxRounds || result.StopReason == llmloop.StopTokenBudget {
		r.recordGraceRoundLimit(taskKey, result.StopReason, result.Completed)
	}
	return result.Completed, result.StopReason, err
}

// CollectPendingComments awaits any async comment-processing workers and
// returns the aggregated comments from the collector. Safe to call once
// per session at the end.
func (r *Runner) CollectPendingComments() []model.LlmComment {
	if r.deps.CommentWorkerPool != nil {
		r.deps.CommentWorkerPool.Await()
	}
	return r.deps.CommentCollector.Comments()
}

// WarningReviewLimitReached is the warning type RunMainTask records when a
// declared budget — the tool-request rounds (StopMaxRounds) or the aggregate
// token budget (StopTokenBudget) — spends the main loop's last round and sends
// the task into its grace round. Callers publish warnings in their JSON and
// terminal output, and the warning keeps a spent limit visible even when the
// grace round completes the task: by design the completion alone would look
// like an untruncated review.
const WarningReviewLimitReached = "review_limit_reached"

// recordGraceRoundLimit records which declared budget reached its grace round,
// and the round's verdict on the task. The verdict is part of the message
// because completion and its absence send a reader — or a JSON consumer — to
// opposite conclusions about the same limit.
func (r *Runner) recordGraceRoundLimit(taskKey string, stop llmloop.MainLoopStop, completed bool) {
	outcome := "the grace round did not complete the task"
	if completed {
		outcome = "the grace round completed the task"
	}
	r.RecordWarning(WarningReviewLimitReached, taskKey, stop.Reason()+"; "+outcome)
}

// graceRoundToolDefs returns the subset of tool definitions containing only
// code_comment and task_done.
func graceRoundToolDefs(defs []llm.ToolDef) []llm.ToolDef {
	out := make([]llm.ToolDef, 0, 2)
	for _, d := range defs {
		if d.Function.Name == "code_comment" || d.Function.Name == "task_done" {
			out = append(out, d)
		}
	}
	return out
}

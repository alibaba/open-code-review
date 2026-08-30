// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/google/uuid"
)

// Deps bundles all per-call dependencies the Runner needs. Both
// internal/agent (diff review) and internal/scan (full-file scan) build a
// Deps from their own state and hand it to NewRunner.
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
	// and this Runner is shared with scan (internal/scan.Agent calls RunPerFile),
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
}

// requestCtx returns ctx carrying the identity of one logical LLM request, or
// ctx unchanged when identity is disabled (scan) or the meta is unusable.
//
// Callers must invoke it after AppendTaskRecord and pass that record's
// RequestNo — the fixed order is AppendTaskRecord -> requestCtx ->
// CompletionsWithCtx -> SetResponse/SetError.
func (r *Runner) requestCtx(ctx context.Context, filePath string, taskType session.TaskType, requestNo int) context.Context {
	if r.deps.NewRequestMeta == nil {
		return ctx
	}
	return llm.WithRequestMeta(ctx, r.deps.NewRequestMeta(filePath, taskType, requestNo))
}

// Runner is a per-session (across files) executor of the LLM tool-use
// loop. Token counters and warnings are aggregated across every RunPerFile
// call; background memory compression is scoped to each RunPerFile
// conversation (see compressionState).
type Runner struct {
	deps                  Deps
	totalInputTokens      int64 // atomically updated
	totalOutputTokens     int64
	totalCacheReadTokens  int64
	totalCacheWriteTokens int64
	warningsMu            sync.Mutex
	warnings              []AgentWarning
	toolCallsMu           sync.Mutex
	toolCalls             map[string]int64
	// bg tracks every background goroutine that can still issue an LLM
	// request after RunPerFile returned. WaitBackground joins them so a
	// retry-report Freeze at the run boundary cannot observe an
	// un-finalized request. See WaitBackground.
	bg sync.WaitGroup
}

// NewRunner returns a Runner bound to the given dependencies.
func NewRunner(deps Deps) *Runner {
	return &Runner{deps: deps}
}

// WaitBackground blocks until every background job started by this Runner has
// returned. Background memory compression is the only such job, and
// cancelPendingCompression cancels it without waiting — its goroutine can
// therefore still be inside an LLM request after RunPerFile returned. Callers
// that freeze a retry report at the run boundary must join here first:
// RetryCollector.Freeze rejects any request that has not been finalized and
// discards the whole report, which would otherwise be an intermittent race.
//
// Every pending job has already been cancelled by the time the last
// RunPerFile returns (cancelPendingCompression runs as a deferred call on
// every exit, and triggerAsyncCompression refuses to start a second job while
// one is pending), so this normally returns quickly — but the wait length
// ultimately depends on the LLM client honouring context cancellation, and no
// additional deadline is imposed here: the job already carries its own
// timeout.
// Both diff-review and scan modes call this prior to session finalization so
// background compression goroutines are joined before session_end is written.
func (r *Runner) WaitBackground() {
	r.bg.Wait()
}

// TotalInputTokens returns the accumulated input/prompt tokens from all LLM calls.
func (r *Runner) TotalInputTokens() int64 { return atomic.LoadInt64(&r.totalInputTokens) }

// TotalOutputTokens returns the accumulated completion tokens from all LLM calls.
func (r *Runner) TotalOutputTokens() int64 { return atomic.LoadInt64(&r.totalOutputTokens) }

// TotalCacheReadTokens returns the accumulated cache read tokens.
func (r *Runner) TotalCacheReadTokens() int64 { return atomic.LoadInt64(&r.totalCacheReadTokens) }

// TotalCacheWriteTokens returns the accumulated cache write tokens.
func (r *Runner) TotalCacheWriteTokens() int64 { return atomic.LoadInt64(&r.totalCacheWriteTokens) }

// TotalTokensUsed returns input + output.
func (r *Runner) TotalTokensUsed() int64 {
	return r.TotalInputTokens() + r.TotalOutputTokens()
}

// Warnings returns a copy of the accumulated warnings.
func (r *Runner) Warnings() []AgentWarning {
	r.warningsMu.Lock()
	defer r.warningsMu.Unlock()
	out := make([]AgentWarning, len(r.warnings))
	copy(out, r.warnings)
	return out
}

// RecordWarning adds a non-fatal warning.
func (r *Runner) RecordWarning(warningType, file, message string) {
	r.warningsMu.Lock()
	r.warnings = append(r.warnings, AgentWarning{
		File:    file,
		Message: message,
		Type:    warningType,
	})
	r.warningsMu.Unlock()
}

// ToolCalls returns a snapshot of the per-tool call counts.
func (r *Runner) ToolCalls() map[string]int64 {
	r.toolCallsMu.Lock()
	defer r.toolCallsMu.Unlock()
	out := make(map[string]int64, len(r.toolCalls))
	for k, v := range r.toolCalls {
		out[k] = v
	}
	return out
}

func (r *Runner) recordToolCall(name string) {
	r.toolCallsMu.Lock()
	if r.toolCalls == nil {
		r.toolCalls = make(map[string]int64)
	}
	r.toolCalls[name]++
	r.toolCallsMu.Unlock()
}

// RecordUsage adds the prompt/completion/cache tokens reported by an LLM
// response to the runner's aggregate counters. Used by callers (plan phase
// in agent / future scan phases) that perform their own LLM calls outside
// RunPerFile.
func (r *Runner) RecordUsage(u *llm.UsageInfo) {
	if u == nil {
		return
	}
	atomic.AddInt64(&r.totalInputTokens, u.PromptTokens)
	atomic.AddInt64(&r.totalOutputTokens, u.CompletionTokens)
	atomic.AddInt64(&r.totalCacheReadTokens, u.CacheReadTokens)
	atomic.AddInt64(&r.totalCacheWriteTokens, u.CacheWriteTokens)
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

// MainLoopStop classifies why RunPerFile stopped without an explicit task_done
// and without a Go error. It lets the caller attribute a precise, honest failure
// classification instead of guessing from free text: only a configured limit
// (max tool-request rounds) is a budget stop; the empty-round and compression
// exits are genuine but unclassifiable, so they map to the unknown catch-all.
// StopNone means the loop returned via task_done (completed) or via an error.
type MainLoopStop int

const (
	// StopNone — RunPerFile completed via task_done or returned an error; the
	// stop cause carries no additional meaning.
	StopNone MainLoopStop = iota
	// StopMaxRounds — the configured MaxToolRequestTimes round budget was
	// exhausted before task_done. This is a declared budget limit.
	StopMaxRounds
	// StopEmptyRounds — the model returned no usable tool result for too many
	// consecutive rounds. Not a declared budget; unclassifiable.
	StopEmptyRounds
	// StopCompression — context compression exceeded its threshold, so the loop
	// could not continue. Token/context driven but not a declared budget.
	StopCompression
)

// String names the stop for diagnostics — telemetry attributes, log lines and
// test failure messages. Without it a MainLoopStop formats as a bare integer,
// which tells the reader nothing about which exit fired.
func (s MainLoopStop) String() string {
	switch s {
	case StopNone:
		return "none"
	case StopMaxRounds:
		return "max_rounds"
	case StopEmptyRounds:
		return "empty_rounds"
	case StopCompression:
		return "compression"
	default:
		return fmt.Sprintf("MainLoopStop(%d)", int(s))
	}
}

// Reason is the safe, human-facing sentence for a stop, and the single source of
// truth for it: the diff-review manifest reason and the scan subtask warning
// both render this string, so the same stop can never read differently in the
// two commands' output. Each return is a static literal — never error text, a
// path or a provider payload — because callers persist it into machine-readable
// output.
//
// Under --format json the [ocr] progress lines that would say which exit fired
// are discarded by stdout.Quiet(), so this is the only stop diagnostic that
// survives an ephemeral CI runner. Every stop must therefore read distinctly,
// including one added to the enum after this was written: the default names the
// unrecognized value instead of silently reusing the StopNone catch-all, so a
// new constant announces itself in the artifact rather than hiding behind a
// message that says nothing.
func (s MainLoopStop) Reason() string {
	switch s {
	case StopNone:
		return "main task stopped before completing"
	case StopMaxRounds:
		return "reached the maximum tool-request rounds without finishing"
	case StopEmptyRounds:
		return "stopped after repeated rounds without a usable tool result"
	case StopCompression:
		return "stopped because context compression exceeded its threshold"
	default:
		return fmt.Sprintf("main task stopped for an unrecognized reason (stop=%d)", int(s))
	}
}

// RunPerFile drives the main LLM conversation loop for a single file.
// It sends messages with the configured tool definitions, executes any
// tool calls returned by the model, and collects review comments until
// task_done is called or limits are reached. Token usage and warnings
// are aggregated on the Runner across all files. The returned bool is true
// only when the model explicitly calls task_done with a successful state. The
// MainLoopStop return classifies a non-completed, non-error stop at its trigger
// point so the caller never has to infer the cause from text or context state.
func (r *Runner) RunPerFile(ctx context.Context, messages []llm.Message, newPath string) (bool, MainLoopStop, error) {
	// Every round of this loop re-sends the growing conversation, so each
	// request is a prefix extension of the previous one — exactly what
	// provider prompt caches reuse. Scope the affinity key to this file's
	// main-task conversation so every round routes to the same cache node.
	ctx = llm.ContextWithSessionKey(ctx,
		llm.SessionTaskKey(r.deps.Session.SessionID, string(session.MainTask), newPath))

	toolReqCount := r.deps.Template.MaxToolRequestTimes
	const maxConsecutiveEmptyRounds = 3
	consecutiveEmptyRounds := 0
	sessionID := uuid.NewString()

	// Async compression is owned by this conversation alone; the deferred
	// cancel aborts any job still in flight when the conversation ends.
	st := &compressionState{}
	defer r.cancelPendingCompression(st)

	// stop defaults to StopMaxRounds: if the for-loop exits because toolReqCount
	// reached zero, the run stopped on the round budget. The empty-round and
	// compression breaks overwrite it at their trigger points.
	stop := StopMaxRounds
	for toolReqCount > 0 {
		select {
		case <-ctx.Done():
			return false, StopNone, ctx.Err()
		default:
		}

		toolReqCount--

		fs := r.deps.Session.GetOrCreateFileSession(newPath)
		rec := fs.AppendTaskRecord(session.MainTask, append([]llm.Message(nil), messages...))
		startTime := time.Now()

		// Scoped to this round: ctx itself must stay identity-free so each
		// iteration's meta replaces the previous one instead of nesting.
		reqCtx := r.requestCtx(ctx, newPath, session.MainTask, rec.RequestNo)

		_, llmSpan := telemetry.StartLLMSpan(ctx, r.deps.Model)
		resp, err := r.deps.LLMClient.CompletionsWithCtx(reqCtx, llm.ChatRequest{
			Model:     r.deps.Model,
			Messages:  messages,
			Tools:     r.deps.MainToolDefs,
			MaxTokens: r.deps.Template.CompletionTokenLimit(),
			SessionID: sessionID,
		})
		duration := time.Since(startTime)
		if err != nil {
			rec.SetError(err, duration)
			telemetry.RecordLLMResult(llmSpan, duration, 0, err)
			llmSpan.End()
			telemetry.RecordLLMRequest(ctx, r.deps.Model, duration, 0, "error")
			return false, StopNone, fmt.Errorf("LLM completion error: %w", err)
		}
		rec.SetResponse(resp, duration)
		totalTokens := int64(0)
		if resp.Usage != nil {
			totalTokens = resp.Usage.TotalTokens
			atomic.AddInt64(&r.totalInputTokens, resp.Usage.PromptTokens)
			atomic.AddInt64(&r.totalOutputTokens, resp.Usage.CompletionTokens)
			atomic.AddInt64(&r.totalCacheReadTokens, resp.Usage.CacheReadTokens)
			atomic.AddInt64(&r.totalCacheWriteTokens, resp.Usage.CacheWriteTokens)
		}
		telemetry.RecordLLMResult(llmSpan, duration, totalTokens, nil)
		llmSpan.End()
		telemetry.RecordLLMRequest(ctx, r.deps.Model, duration, totalTokens, "ok")

		content := resp.VisibleContent()
		calls := resp.ToolCalls()

		if len(calls) == 0 {
			fmt.Fprintf(stdout.Writer(), "[ocr] No tool calls parsed for %s, retrying...\n", newPath)
			messages = append(messages, llm.NewTextMessage("user", "You did not successfully call any tools. Please try again or use task_done if finished."))
			native := resp.Native()
			reasoning := resp.ReasoningContent()
			if content != "" || native.Payload != nil || reasoning != "" {
				messages = append(messages[:len(messages)-1], llm.NewToolCallMessage(content, nil, native, reasoning), messages[len(messages)-1])
			}
			continue
		}

		var results []tool.ToolCallResult
		taskCompleted := false
		hasValidResult := false

		// Capture the model's native reasoning content for this turn. Models
		// without a reasoning channel leave it empty.
		// Reasoning is turn-level, so all tool calls in this turn share it.
		thinking := resp.ReasoningContent()
		for _, call := range calls {
			cp := r.executeToolCall(ctx, newPath, call, rec, thinking)
			if cp.Failed {
				return false, StopNone, fmt.Errorf("task failed: %s", cp.Data)
			} else if cp.Completed {
				results = append(results, tool.ToolCallResult{
					ToolCallID: call.ID,
					Name:       call.Function.Name,
					Result:     "Task completed successfully.",
				})
				taskCompleted = true
			} else if cp.Data != "" {
				results = append(results, tool.ToolCallResult{
					ToolCallID: call.ID,
					Name:       call.Function.Name,
					Result:     cp.Data,
				})
				hasValidResult = true
			} else {
				results = append(results, tool.ToolCallResult{
					ToolCallID: call.ID,
					Name:       call.Function.Name,
					Result:     "Error: Tool execution returned no result.",
				})
			}
		}

		if taskCompleted {
			return true, StopNone, nil
		}
		if !hasValidResult {
			consecutiveEmptyRounds++
			if consecutiveEmptyRounds >= maxConsecutiveEmptyRounds {
				fmt.Fprintf(stdout.Writer(), "[ocr] Too many empty retries for %s, stopping.\n", newPath)
				stop = StopEmptyRounds
				break
			}
			fmt.Fprintf(stdout.Writer(), "[ocr] No valid tool results for %s, retrying...\n", newPath)
		} else {
			consecutiveEmptyRounds = 0
		}

		succeed := r.addNextMessage(ctx, content, calls, resp.Native(), thinking, results, &messages, newPath, st)
		if !succeed {
			fmt.Fprintf(stdout.Writer(), "[ocr] Context compression exceeded threshold for %s, stopping.\n", newPath)
			stop = StopCompression
			break
		}
	}

	if stop == StopMaxRounds {
		fmt.Fprintf(stdout.Writer(), "[ocr] Max tool requests reached for %s.\n", newPath)
		r.runGraceRound(ctx, messages, newPath, sessionID)
	}
	return false, stop, nil
}

// runGraceRound performs one final LLM call after the tool-request budget is
// exhausted, giving the model a chance to submit any findings it identified
// but did not yet report via code_comment.
func (r *Runner) runGraceRound(ctx context.Context, messages []llm.Message, newPath string, sessionID string) {
	graceDefs := graceRoundToolDefs(r.deps.MainToolDefs)
	if len(graceDefs) == 0 {
		return
	}

	messages = append(messages, llm.NewTextMessage("user",
		"Your tool-call budget is exhausted. This is your FINAL round. You may ONLY:\n"+
			"- Call code_comment to submit any findings you have identified but not yet reported.\n"+
			"- Call task_done if you have nothing more to report.\n"+
			"No other tools are available. Do not attempt further analysis."))

	if ctx.Err() != nil {
		fmt.Fprintf(stdout.Writer(), "[ocr] Grace round skipped for %s: context cancelled\n", newPath)
		return
	}

	fs := r.deps.Session.GetOrCreateFileSession(newPath)
	rec := fs.AppendTaskRecord(session.MainTask, messages)
	startTime := time.Now()
	reqCtx := r.requestCtx(ctx, newPath, session.MainTask, rec.RequestNo)

	_, llmSpan := telemetry.StartLLMSpan(ctx, r.deps.Model)
	resp, err := r.deps.LLMClient.CompletionsWithCtx(reqCtx, llm.ChatRequest{
		Model:     r.deps.Model,
		Messages:  messages,
		Tools:     graceDefs,
		MaxTokens: r.deps.Template.CompletionTokenLimit(),
		SessionID: sessionID,
	})
	duration := time.Since(startTime)
	if err != nil {
		rec.SetError(err, duration)
		telemetry.RecordLLMResult(llmSpan, duration, 0, err)
		llmSpan.End()
		telemetry.RecordLLMRequest(ctx, r.deps.Model, duration, 0, "error")
		fmt.Fprintf(stdout.Writer(), "[ocr] Grace round LLM error for %s: %v\n", newPath, err)
		return
	}

	rec.SetResponse(resp, duration)
	totalTokens := int64(0)
	if resp.Usage != nil {
		totalTokens = resp.Usage.TotalTokens
		atomic.AddInt64(&r.totalInputTokens, resp.Usage.PromptTokens)
		atomic.AddInt64(&r.totalOutputTokens, resp.Usage.CompletionTokens)
		atomic.AddInt64(&r.totalCacheReadTokens, resp.Usage.CacheReadTokens)
		atomic.AddInt64(&r.totalCacheWriteTokens, resp.Usage.CacheWriteTokens)
	}
	telemetry.RecordLLMResult(llmSpan, duration, totalTokens, nil)
	llmSpan.End()
	telemetry.RecordLLMRequest(ctx, r.deps.Model, duration, totalTokens, "ok")

	calls := resp.ToolCalls()
	if len(calls) == 0 {
		return
	}

	thinking := resp.ReasoningContent()
	for _, call := range calls {
		r.executeToolCall(ctx, newPath, call, rec, thinking)
	}
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

// executeToolCall dispatches a single tool call from the LLM response and
// records the result in session history. code_comment handling includes
// optional async dispatch through CommentWorkerPool plus line-number
// resolution / re-location.
func (r *Runner) executeToolCall(ctx context.Context, newPath string, call llm.ToolCall, rec *session.TaskRecord, thinking string) tool.TaskCheckpoint {
	t := tool.OfName(call.Function.Name)

	if !t.IsKnown() {
		p, ok := r.deps.Tools.Get(call.Function.Name)
		if !ok {
			return tool.Of(tool.NotAvailableMsg)
		}
		r.recordToolCall(call.Function.Name)
		dynArgs, err := parseToolArgs(call.Function.Arguments)
		if err != nil {
			return tool.Of(fmt.Sprintf("Error parsing tool arguments for %s: %v", call.Function.Name, err))
		}
		telemetry.PrintToolCallStarted(call.Function.Name, dynArgs)
		_, toolSpan := telemetry.StartToolSpan(ctx, call.Function.Name)
		startTime := time.Now()
		result, err := p.Execute(ctx, dynArgs)
		dur := time.Since(startTime)
		if err != nil {
			telemetry.RecordToolResult(toolSpan, call.Function.Name, dur.Milliseconds(), err)
			toolSpan.End()
			telemetry.RecordToolCall(ctx, call.Function.Name, dur, false)
			telemetry.PrintToolCallError(call.Function.Name, err)
			return tool.Of(fmt.Sprintf("Error executing tool %s: %v", call.Function.Name, err))
		}
		telemetry.RecordToolResult(toolSpan, call.Function.Name, dur.Milliseconds(), nil)
		toolSpan.End()
		telemetry.RecordToolCall(ctx, call.Function.Name, dur, true)
		telemetry.PrintToolCallFinished(call.Function.Name, dur)
		if rec != nil {
			rec.AddToolResult(call.Function.Name, call.Function.Arguments, result)
		}
		return tool.Of(result)
	}

	if t == tool.TaskDone {
		args, err := parseToolArgs(call.Function.Arguments)
		if err != nil {
			return tool.Of(fmt.Sprintf("Error parsing tool arguments for %s: %v", t.Name(), err))
		}
		rawState, hasState := args["state"]
		if !hasState {
			return tool.Complete()
		}
		state, ok := rawState.(string)
		if !ok {
			return tool.Of("Error: task_done state must be DONE or FAILED.")
		}
		switch state {
		case "DONE":
			return tool.Complete()
		case "FAILED":
			return tool.Fail("task_done reported FAILED")
		default:
			return tool.Of(fmt.Sprintf("Error: invalid task_done state %q; expected DONE or FAILED.", state))
		}
	}

	p := lookupTool(r.deps.Tools, t)
	if p == nil {
		return tool.Of(tool.NotAvailableMsg)
	}

	r.recordToolCall(t.Name())

	args, err := parseToolArgs(call.Function.Arguments)
	if err != nil {
		return tool.Of(fmt.Sprintf("Error parsing tool arguments for %s: %v", t.Name(), err))
	}

	startTime := time.Now()

	if t == tool.CodeComment {
		telemetry.PrintToolCallStarted(t.Name(), args)
		_, toolSpan := telemetry.StartToolSpan(ctx, t.Name())

		comments, errMsg := tool.ParseCommentsWithPath(args, newPath)
		if errMsg != "" {
			dur := time.Since(startTime)
			telemetry.RecordToolResult(toolSpan, t.Name(), dur.Milliseconds(), fmt.Errorf("%s", errMsg))
			toolSpan.End()
			telemetry.RecordToolCall(ctx, t.Name(), dur, false)
			return tool.Of(errMsg)
		}

		// Batched comments share the turn's thinking.
		if thinking != "" {
			for i := range comments {
				if comments[i].Thinking == "" {
					comments[i].Thinking = thinking
				}
			}
		}

		resolveAndCollect := func(rctx context.Context) {
			for i := range comments {
				cm := &comments[i]
				var d *model.Diff
				if r.deps.DiffLookup != nil {
					d = r.deps.DiffLookup(cm.Path)
				}
				// Resolution order: the comment's own file, then a cross-file
				// search, then the LLM. The cross-file search precedes the LLM
				// because it needs the Agent's original ExistingCode, which the
				// LLM step overwrites; and it runs even when d is nil, since a
				// comment filed against a path this run holds no diff for is
				// exactly the case that search can still place.
				located := d != nil && diff.ResolveComment(cm, d)
				if !located && r.deps.AllDiffs != nil {
					from := cm.Path
					if to, ok := diff.RelocateAcrossFiles(cm, r.deps.AllDiffs()); ok {
						located = true
						r.RecordWarning("comment_refiled", to, fmt.Sprintf(
							"comment filed against %s describes code in %s; re-filed", from, to))
					}
				}
				if d != nil {
					if !located && r.deps.Template.ReLocationTask != nil {
						// rlStart stays ahead of prompt construction, which is
						// where it sat when ReLocateComment built the messages
						// itself — moving it would silently change what
						// TaskRecord.Duration measures.
						rlStart := time.Now()
						msgs := diff.BuildReLocationMessages(cm, d, r.deps.Template.ReLocationTask)
						if len(msgs) > 0 {
							fs := r.deps.Session.GetOrCreateFileSession(cm.Path)
							rlRec := fs.AppendTaskRecord(session.ReLocationTask, msgs)
							// FilePath is cm.Path so it cannot drift from the file
							// session opened above — that join is what the report
							// needs. It equals newPath whenever newPath is set,
							// because the path arg is overridden with it further
							// up, but reading it from the comment keeps the two
							// aligned without depending on that.
							rlCtx := llm.ContextWithSessionKey(rctx,
								llm.SessionTaskKey(r.deps.Session.SessionID, string(session.ReLocationTask), cm.Path))
							reqCtx := r.requestCtx(rlCtx, cm.Path, session.ReLocationTask, rlRec.RequestNo)
							_, resp := diff.ReLocateComment(reqCtx, cm, d, r.deps.LLMClient, msgs, r.deps.Model, r.deps.Template.CompletionTokenLimit())
							if resp != nil {
								rlRec.SetResponse(resp, time.Since(rlStart))
								if resp.Usage != nil {
									atomic.AddInt64(&r.totalInputTokens, resp.Usage.PromptTokens)
									atomic.AddInt64(&r.totalOutputTokens, resp.Usage.CompletionTokens)
									atomic.AddInt64(&r.totalCacheReadTokens, resp.Usage.CacheReadTokens)
									atomic.AddInt64(&r.totalCacheWriteTokens, resp.Usage.CacheWriteTokens)
								}
							} else {
								rlRec.SetError(fmt.Errorf("re-location LLM call failed"), time.Since(rlStart))
							}
						}
					}
				}
				r.deps.CommentCollector.Add(*cm)
			}
		}

		if r.deps.CommentWorkerPool != nil {
			if rec != nil {
				rec.AddToolResult(t.Name(), call.Function.Arguments, "(async)")
			}
			pool := r.deps.CommentWorkerPool
			asyncCtx := context.WithoutCancel(ctx)
			toolName := t.Name()
			pool.SubmitFor(newPath, func() ([]model.LlmComment, error) {
				defer func() {
					dur := time.Since(startTime)
					telemetry.RecordToolResult(toolSpan, toolName, dur.Milliseconds(), nil)
					toolSpan.End()
					telemetry.PrintToolCallFinished(toolName, dur)
				}()
				resolveAndCollect(asyncCtx)
				return []model.LlmComment{}, nil
			})
			telemetry.RecordToolCall(asyncCtx, toolName, time.Since(startTime), true)
			return tool.Of(tool.CommentSucceed)
		}

		resolveAndCollect(ctx)
		dur := time.Since(startTime)
		telemetry.RecordToolResult(toolSpan, t.Name(), dur.Milliseconds(), nil)
		toolSpan.End()
		telemetry.RecordToolCall(ctx, t.Name(), dur, true)
		telemetry.PrintToolCallFinished(t.Name(), dur)
		if rec != nil {
			rec.AddToolResult(t.Name(), call.Function.Arguments, tool.CommentSucceed)
		}
		return tool.Of(tool.CommentSucceed)
	}

	// Synchronous path for all other tools
	telemetry.PrintToolCallStarted(t.Name(), args)
	_, toolSpan := telemetry.StartToolSpan(ctx, t.Name())
	result, err := p.Execute(ctx, args)
	dur := time.Since(startTime)
	ok := err == nil
	telemetry.RecordToolResult(toolSpan, t.Name(), dur.Milliseconds(), err)
	toolSpan.End()
	telemetry.RecordToolCall(ctx, t.Name(), dur, ok)

	if err != nil {
		telemetry.PrintToolCallError(t.Name(), err)
		return tool.Of(fmt.Sprintf("Error executing tool %s: %v", t.Name(), err))
	}
	telemetry.PrintToolCallFinished(t.Name(), dur)
	if rec != nil {
		rec.AddToolResult(t.Name(), call.Function.Arguments, result)
	}
	return tool.Of(result)
}

// addNextMessage extends the conversation with the assistant message and
// tool responses, applying three-zone compression at the soft (60%) and
// warning (80%) MaxTokens thresholds. Returns false when even after
// synchronous compression the conversation is still over the warning
// threshold — caller should stop the loop in that case.
func (r *Runner) addNextMessage(ctx context.Context, assistantContent string, toolCalls []llm.ToolCall, native llm.NativeTurn, reasoningContent string, results []tool.ToolCallResult, messages *[]llm.Message, filePath string, st *compressionState) bool {
	maxAllowed := r.deps.Template.MaxTokens
	softLimit := int(float64(maxAllowed) * tokenSoftThreshold)
	warnLimit := PromptTokenLimit(maxAllowed)

	r.tryApplyPendingCompression(st, messages)

	// A conversation can already be over the warning threshold before this
	// round's messages are appended (e.g. an oversized initial prompt).
	if CountMessagesTokens(*messages) > warnLimit {
		r.cancelPendingCompression(st)
		var err error
		if *messages, err = r.runCompression(ctx, *messages, filePath); err != nil {
			// Compression failed; continue with over-limit messages — the
			// post-append check below will retry.
			fmt.Fprintf(stdout.Writer(), "[ocr] Memory compression failed: %v\n", err)
		}
	}

	if len(toolCalls) > 0 {
		*messages = append(*messages, llm.NewToolCallMessage(assistantContent, toolCalls, native, reasoningContent))
	} else if assistantContent != "" || native.Payload != nil {
		*messages = append(*messages, llm.NewToolCallMessage(assistantContent, nil, native, reasoningContent))
	}

	for _, rs := range results {
		*messages = append(*messages, llm.NewToolResultMessage(rs.ToolCallID, rs.Result))
	}

	finalCount := CountMessagesTokens(*messages)
	if finalCount > warnLimit {
		r.cancelPendingCompression(st)
		var err error
		if *messages, err = r.runCompression(ctx, *messages, filePath); err != nil {
			fmt.Fprintf(stdout.Writer(), "[ocr] Memory compression failed: %v\n", err)
		}
		finalCount = CountMessagesTokens(*messages)
	}

	// Trigger async compression only after all appends for this update, so
	// a job is never started and then immediately cancelled by the same
	// call (#384), and never started when we are about to return false.
	if finalCount > softLimit && finalCount < warnLimit {
		r.triggerAsyncCompression(ctx, st, *messages, filePath)
	}

	return finalCount < warnLimit
}

// parseToolArgs unmarshals a tool call's raw JSON arguments, always
// returning a non-nil map on success: some OpenAI-compatible gateways send
// "arguments": null, which unmarshals to a nil map and would panic on the
// first write (#382). An equivalent inline guard exists in internal/llm's
// buildAnthropicParams; keep the two in sync.
func parseToolArgs(raw string) (map[string]any, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, err
	}
	if args == nil {
		args = make(map[string]any)
	}
	return args, nil
}

// lookupTool returns the provider for a given tool from the registry, or
// nil when not registered.
func lookupTool(reg *tool.Registry, t tool.Tool) tool.Provider {
	p, ok := reg.Get(t.Name())
	if !ok {
		return nil
	}
	return p
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
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

// ProgressEvent is the intentionally small liveness signal exposed by the
// MCP adapter. It carries no prompt, response, token, provider, or credential
// data.
type ProgressEvent struct {
	Event string `json:"event"`
	Phase string `json:"phase"`
	Path  string `json:"path,omitempty"`
}

type ProgressFunc func(ProgressEvent)

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
	Progress          ProgressFunc
	TokenAccounting   *llm.TokenAccounting
	ContextBudget     int64
	MaxTokensBudget   int64
	// DiffLookup is consulted by the code_comment tool path to resolve
	// line numbers against the file's diff (or against full file content
	// in scan mode — scan adapters return a synthetic Diff whose
	// NewFileContent is the whole file and Diff is empty).
	DiffLookup func(path string) *model.Diff

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
	deps        Deps
	accounting  *llm.TokenAccounting
	client      *llm.AccountingClient
	warningsMu  sync.Mutex
	warnings    []AgentWarning
	toolCallsMu sync.Mutex
	toolCalls   map[string]int64
	// bg tracks every background goroutine that can still issue an LLM
	// request after RunPerFile returned. WaitBackground joins them so a
	// retry-report Freeze at the run boundary cannot observe an
	// un-finalized request. See WaitBackground.
	bg sync.WaitGroup
}

// NewRunner returns a Runner bound to the given dependencies.
func NewRunner(deps Deps) *Runner {
	accounting := deps.TokenAccounting
	if accounting == nil {
		accounting = llm.NewTokenAccounting(llm.TokenAccountingOptions{
			ContextBudget:   deps.ContextBudget,
			AggregateBudget: deps.MaxTokensBudget,
		})
	}
	return &Runner{
		deps:       deps,
		accounting: accounting,
		client:     llm.NewAccountingClient(deps.LLMClient, accounting),
	}
}

func (r *Runner) emitProgress(phase, path string) {
	if r.deps.Progress != nil {
		r.deps.Progress(ProgressEvent{Event: "ocr_progress", Phase: phase, Path: path})
	}
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
// Scan does not currently call this because it freezes no retry report; its
// analogous session-finalization race is outside this change.
func (r *Runner) WaitBackground() {
	r.bg.Wait()
}

// TotalInputTokens returns the accumulated input/prompt tokens from all LLM calls.
func (r *Runner) TotalInputTokens() int64 { return r.accounting.Snapshot().InputTokens }

// TotalOutputTokens returns the accumulated completion tokens from all LLM calls.
func (r *Runner) TotalOutputTokens() int64 { return r.accounting.Snapshot().OutputTokens }

// TotalCacheReadTokens returns the accumulated cache read tokens.
func (r *Runner) TotalCacheReadTokens() int64 { return r.accounting.Snapshot().CacheReadTokens }

// TotalCacheWriteTokens returns the accumulated cache write tokens.
func (r *Runner) TotalCacheWriteTokens() int64 { return r.accounting.Snapshot().CacheWriteTokens }

// TotalTokensUsed returns the provider total when input/output are not
// separately available, otherwise input + output.
func (r *Runner) TotalTokensUsed() int64 {
	return r.accounting.Snapshot().TotalTokens
}

// LLMClient returns the shared accounting seam for phase-specific callers.
func (r *Runner) LLMClient() llm.LLMClient { return r.client }

// TokenAccounting returns the shared aggregate store used by every phase.
func (r *Runner) TokenAccounting() *llm.TokenAccounting { return r.accounting }

// SetTokenBudget applies the resolved positive cap after template/CLI
// resolution. Zero preserves unlimited behavior.
func (r *Runner) SetTokenBudget(maxTokens int64) {
	if r != nil && r.accounting != nil {
		r.accounting.SetAggregateBudget(maxTokens)
	}
}

// UsageStatus reports the aggregate certainty level of completed requests.
func (r *Runner) UsageStatus() llm.UsageStatus {
	return r.accounting.Snapshot().UsageStatus
}

// UnknownUsageRequests reports requests for which neither provider usage nor
// a local complete-request estimate was available.
func (r *Runner) UnknownUsageRequests() int64 {
	return r.accounting.Snapshot().UnknownRequests
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
	r.accounting.RecordExternal(u)
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
	// StopBudgetExceeded means the shared aggregate token gate rejected the
	// next request. Existing findings remain valid and callers classify the
	// item as a controlled budget stop.
	StopBudgetExceeded
)

// RunPerFile drives the main LLM conversation loop for a single file.
// It sends messages with the configured tool definitions, executes any
// tool calls returned by the model, and collects review comments until
// task_done is called or limits are reached. Token usage and warnings
// are aggregated on the Runner across all files. The returned bool is true
// only when the model explicitly calls task_done with a successful state. The
// MainLoopStop return classifies a non-completed, non-error stop at its trigger
// point so the caller never has to infer the cause from text or context state.
func (r *Runner) RunPerFile(ctx context.Context, messages []llm.Message, newPath string) (bool, MainLoopStop, error) {
	toolReqCount := r.deps.Template.MaxToolRequestTimes
	const maxConsecutiveEmptyRounds = 3
	consecutiveEmptyRounds := 0
	sessionID := uuid.NewString()
	contextResults := make(map[string]string)

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

		req := llm.ChatRequest{
			Model:     r.deps.Model,
			Messages:  messages,
			Tools:     r.deps.MainToolDefs,
			MaxTokens: r.deps.Template.CompletionTokenLimit(),
			SessionID: sessionID,
		}
		if err := r.fitRequest(ctx, &req, &messages, newPath, st); err != nil {
			switch {
			case errors.Is(err, llm.ErrTokenBudgetExceeded):
				r.RecordWarning("token_budget_reached", newPath, "aggregate token budget exceeded before main-task dispatch")
				return false, StopBudgetExceeded, nil
			case errors.Is(err, llm.ErrContextBudgetExceeded):
				return false, StopCompression, nil
			default:
				return false, StopNone, err
			}
		}

		fs := r.deps.Session.GetOrCreateFileSession(newPath)
		rec := fs.AppendTaskRecord(session.MainTask, append([]llm.Message(nil), messages...))
		startTime := time.Now()

		// Scoped to this round: ctx itself must stay identity-free so each
		// iteration's meta replaces the previous one instead of nesting.
		reqCtx := r.requestCtx(ctx, newPath, session.MainTask, rec.RequestNo)

		_, llmSpan := telemetry.StartLLMSpan(ctx, r.deps.Model)
		resp, err := r.client.CompletionsWithCtx(reqCtx, req)
		duration := time.Since(startTime)
		if err != nil {
			if errors.Is(err, llm.ErrTokenBudgetExceeded) {
				rec.SetError(err, duration)
				telemetry.RecordLLMResult(llmSpan, duration, 0, err)
				llmSpan.End()
				r.RecordWarning("token_budget_reached", newPath, "aggregate token budget exceeded before main-task dispatch")
				return false, StopBudgetExceeded, nil
			}
			if errors.Is(err, llm.ErrContextBudgetExceeded) {
				rec.SetError(err, duration)
				telemetry.RecordLLMResult(llmSpan, duration, 0, err)
				llmSpan.End()
				return false, StopCompression, nil
			}
			rec.SetError(err, duration)
			telemetry.RecordLLMResult(llmSpan, duration, 0, err)
			llmSpan.End()
			telemetry.RecordLLMRequest(ctx, r.deps.Model, duration, 0, "error")
			return false, StopNone, fmt.Errorf("LLM completion error: %w", err)
		}
		rec.SetResponse(resp, duration)
		r.emitProgress("llm_completed", newPath)
		totalTokens := int64(0)
		if resp.Usage != nil {
			totalTokens = resp.Usage.TotalTokens
			if totalTokens == 0 {
				totalTokens = resp.Usage.PromptTokens + resp.Usage.CompletionTokens
			}
		}
		telemetry.RecordLLMResult(llmSpan, duration, totalTokens, nil)
		llmSpan.End()
		telemetry.RecordLLMRequest(ctx, r.deps.Model, duration, totalTokens, "ok")

		content := resp.Content()
		calls := resp.ToolCalls()
		responseItems := resp.ResponseItems()

		if len(calls) == 0 {
			fmt.Fprintf(stdout.Writer(), "[ocr] No tool calls parsed for %s, retrying...\n", newPath)
			messages = append(messages, llm.NewTextMessage("user", "You did not successfully call any tools. Please try again or use task_done if finished."))
			if content != "" {
				assistant := llm.NewAssistantMessage(content, resp.Phase())
				assistant.ResponseItems = responseItems
				messages = append(messages[:len(messages)-1], assistant, messages[len(messages)-1])
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
			t := tool.OfName(call.Function.Name)
			if t == tool.FileRead || t == tool.FileFind || t == tool.FileReadDiff || t == tool.CodeSearch {
				key := call.Function.Name + "\x00" + call.Function.Arguments
				if cached, duplicate := contextResults[key]; duplicate {
					results = append(results, tool.ToolCallResult{
						ToolCallID: call.ID,
						Name:       call.Function.Name,
						Result:     "No new evidence: this exact context query already ran. Earlier result:\n" + cached,
					})
					continue
				}
			}
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
				if t == tool.FileRead || t == tool.FileFind || t == tool.FileReadDiff || t == tool.CodeSearch {
					if isUsableContextResult(cp.Data) {
						contextResults[call.Function.Name+"\x00"+call.Function.Arguments] = cp.Data
					}
				}
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

		succeed := r.addNextMessage(ctx, content, resp.Phase(), responseItems, calls, results, &messages, newPath, st)
		if !succeed {
			if r.accounting.Snapshot().BudgetExceeded {
				return false, StopBudgetExceeded, nil
			}
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

func isUsableContextResult(result string) bool {
	trimmed := strings.TrimSpace(result)
	return trimmed != "" && !strings.HasPrefix(trimmed, "Error") && !strings.HasPrefix(trimmed, "code_search timed out.")
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

	resp, err := r.client.CompletionsWithCtx(ctx, llm.ChatRequest{
		Model:     r.deps.Model,
		Messages:  messages,
		Tools:     graceDefs,
		MaxTokens: r.deps.Template.CompletionTokenLimit(),
		SessionID: sessionID,
	})
	if err != nil {
		fmt.Fprintf(stdout.Writer(), "[ocr] Grace round LLM error for %s: %v\n", newPath, err)
		return
	}

	calls := resp.ToolCalls()
	if len(calls) == 0 {
		return
	}

	fs := r.deps.Session.GetOrCreateFileSession(newPath)
	rec := fs.AppendTaskRecord(session.MainTask, append([]llm.Message(nil), messages...))
	rec.SetResponse(resp, 0)
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
			if rec != nil {
				rec.AddToolError(call.Function.Name, call.Function.Arguments, tool.NotAvailableMsg, 0)
			}
			return tool.Of(tool.NotAvailableMsg)
		}
		r.recordToolCall(call.Function.Name)
		dynArgs, err := parseToolArgs(call.Function.Arguments)
		if err != nil {
			message := fmt.Sprintf("Error parsing tool arguments for %s: %v", call.Function.Name, err)
			if rec != nil {
				rec.AddToolError(call.Function.Name, call.Function.Arguments, message, 0)
			}
			return tool.Of(message)
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
			message := fmt.Sprintf("Error executing tool %s: %v", call.Function.Name, err)
			if rec != nil {
				rec.AddToolError(call.Function.Name, call.Function.Arguments, message, dur)
			}
			return tool.Of(message)
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
			message := fmt.Sprintf("Error parsing tool arguments for %s: %v", t.Name(), err)
			if rec != nil {
				rec.AddToolError(t.Name(), call.Function.Arguments, message, 0)
			}
			return tool.Of(message)
		}
		rawState, hasState := args["state"]
		if !hasState {
			return tool.Complete()
		}
		state, ok := rawState.(string)
		if !ok {
			message := "Error: task_done state must be DONE or FAILED."
			if rec != nil {
				rec.AddToolError(t.Name(), call.Function.Arguments, message, 0)
			}
			return tool.Of(message)
		}
		switch state {
		case "DONE":
			return tool.Complete()
		case "FAILED":
			return tool.Fail("task_done reported FAILED")
		default:
			message := fmt.Sprintf("Error: invalid task_done state %q; expected DONE or FAILED.", state)
			if rec != nil {
				rec.AddToolError(t.Name(), call.Function.Arguments, message, 0)
			}
			return tool.Of(message)
		}
	}

	p := lookupTool(r.deps.Tools, t)
	if p == nil {
		if rec != nil {
			rec.AddToolError(t.Name(), call.Function.Arguments, tool.NotAvailableMsg, 0)
		}
		return tool.Of(tool.NotAvailableMsg)
	}

	r.recordToolCall(t.Name())

	args, err := parseToolArgs(call.Function.Arguments)
	if err != nil {
		message := fmt.Sprintf("Error parsing tool arguments for %s: %v", t.Name(), err)
		if rec != nil {
			rec.AddToolError(t.Name(), call.Function.Arguments, message, 0)
		}
		return tool.Of(message)
	}

	// Always inject the current file path for code_comment.
	// The model sometimes hallucinates a path, so we override it.
	if t == tool.CodeComment && newPath != "" {
		args["path"] = newPath
	}

	startTime := time.Now()

	if t == tool.CodeComment {
		telemetry.PrintToolCallStarted(t.Name(), args)
		_, toolSpan := telemetry.StartToolSpan(ctx, t.Name())

		comments, errMsg := tool.ParseComments(args)
		if errMsg != "" {
			dur := time.Since(startTime)
			telemetry.RecordToolResult(toolSpan, t.Name(), dur.Milliseconds(), fmt.Errorf("%s", errMsg))
			toolSpan.End()
			telemetry.RecordToolCall(ctx, t.Name(), dur, false)
			if rec != nil {
				rec.AddToolError(t.Name(), call.Function.Arguments, errMsg, dur)
			}
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
				if d != nil {
					if !diff.ResolveComment(cm, d) && r.deps.Template.ReLocationTask != nil {
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
							reqCtx := r.requestCtx(rctx, cm.Path, session.ReLocationTask, rlRec.RequestNo)
							_, resp := diff.ReLocateComment(reqCtx, cm, d, r.client, msgs, r.deps.Model, r.deps.Template.CompletionTokenLimit())
							if resp != nil {
								rlRec.SetResponse(resp, time.Since(rlStart))
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
		message := fmt.Sprintf("Error executing tool %s: %v", t.Name(), err)
		if rec != nil {
			rec.AddToolError(t.Name(), call.Function.Arguments, message, dur)
		}
		return tool.Of(message)
	}
	telemetry.PrintToolCallFinished(t.Name(), dur)
	if rec != nil {
		rec.AddToolResult(t.Name(), call.Function.Arguments, result)
	}
	return tool.Of(result)
}

// addNextMessage extends the conversation with the assistant message and
// tool responses, applying three-zone compression at the soft (60%) and
// effective preflight MaxTokens thresholds. Returns false when even after
// synchronous compression the conversation is still over the warning
// threshold — caller should stop the loop in that case.
func (r *Runner) addNextMessage(ctx context.Context, assistantContent, assistantPhase string, assistantResponseItems []json.RawMessage, toolCalls []llm.ToolCall, results []tool.ToolCallResult, messages *[]llm.Message, filePath string, st *compressionState) bool {
	maxAllowed := r.deps.Template.MaxTokens
	softLimit := int(float64(maxAllowed) * tokenSoftThreshold)
	warnLimit := PromptTokenLimit(maxAllowed)
	didSyncCompression := false

	r.tryApplyPendingCompression(st, messages)

	// A conversation can already be over the warning threshold before this
	// round's messages are appended (e.g. an oversized initial prompt).
	if r.messageInputTokens(*messages) > int64(warnLimit) {
		r.cancelPendingCompression(st)
		var err error
		if *messages, err = r.runCompression(ctx, *messages, filePath, true); err != nil {
			// Compression failed; continue with over-limit messages — the
			// post-append check below will retry.
			fmt.Fprintf(stdout.Writer(), "[ocr] Memory compression failed: %v\n", err)
		}
		didSyncCompression = true
	}

	if len(toolCalls) > 0 {
		assistant := llm.NewToolCallMessageWithPhase(assistantContent, toolCalls, assistantPhase)
		assistant.ResponseItems = assistantResponseItems
		*messages = append(*messages, assistant)
	} else if assistantContent != "" {
		assistant := llm.NewAssistantMessage(assistantContent, assistantPhase)
		assistant.ResponseItems = assistantResponseItems
		*messages = append(*messages, assistant)
	}

	for _, rs := range results {
		*messages = append(*messages, llm.NewToolResultMessage(rs.ToolCallID, rs.Result))
	}

	finalCount := int(r.messageInputTokens(*messages))
	if finalCount > warnLimit {
		r.cancelPendingCompression(st)
		var err error
		if *messages, err = r.runCompression(ctx, *messages, filePath, true); err != nil {
			fmt.Fprintf(stdout.Writer(), "[ocr] Memory compression failed: %v\n", err)
		}
		didSyncCompression = true
		finalCount = int(r.messageInputTokens(*messages))
	}

	// Trigger async compression only after all appends for this update, so
	// a job is never started and then immediately cancelled by the same
	// call (#384), and never started when we are about to return false.
	if !didSyncCompression && finalCount > softLimit && finalCount < warnLimit {
		r.triggerAsyncCompression(ctx, st, *messages, filePath)
	}

	return finalCount <= warnLimit
}

func (r *Runner) messageInputTokens(messages []llm.Message) int64 {
	if r == nil || r.accounting == nil {
		return int64(CountMessagesTokens(messages))
	}
	return r.accounting.Estimate(llm.ChatRequest{
		Model:    r.deps.Model,
		Messages: messages,
		Tools:    r.deps.MainToolDefs,
	}).InputTokens
}

func (r *Runner) requestFits(req llm.ChatRequest) bool {
	max := r.accounting.Snapshot().ContextBudget
	if max <= 0 {
		return true
	}
	// The configured context budget and provider output limit are independent:
	// this gate applies the single safety margin to input only.
	est := r.accounting.Estimate(req)
	return est.Status == llm.UsageStatusUnknown || est.InputTokens <= llm.EffectivePreflightCeiling(max)
}

// fitRequest applies the same complete-request estimate used by the
// AccountingClient before dispatch. It gives compression one chance to reduce
// the active conversation; the provider seam remains the final guard.
func (r *Runner) fitRequest(ctx context.Context, req *llm.ChatRequest, messages *[]llm.Message, filePath string, st *compressionState) error {
	if r.requestFits(*req) {
		return nil
	}
	r.cancelPendingCompression(st)
	rebuilt, err := r.runCompression(ctx, *messages, filePath, true)
	if err != nil {
		return err
	}
	*messages = rebuilt
	req.Messages = rebuilt
	if r.requestFits(*req) {
		return nil
	}
	return fmt.Errorf("%w: complete request remains over the preflight ceiling", llm.ErrContextBudgetExceeded)
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

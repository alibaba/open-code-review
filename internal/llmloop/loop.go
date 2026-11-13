// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/google/uuid"
)

type Deps struct {
	LLMClient       llm.LLMClient
	Model           string
	Session         *session.SessionHistory
	NewRequestMeta  func(taskKey string, taskType session.TaskType, requestNo int) llm.RequestMeta
	MaxTokensBudget int64
}

// RequestContext must be called after AppendTaskRecord: request identity joins
// the retry report to the existing session record by its RequestNo.
func (r *Runner) RequestContext(ctx context.Context, taskKey string, taskType session.TaskType, requestNo int) context.Context {
	if r.deps.NewRequestMeta == nil {
		return ctx
	}
	return llm.WithRequestMeta(ctx, r.deps.NewRequestMeta(taskKey, taskType, requestNo))
}

// Runner is a per-session (across subtasks) executor of the LLM tool-use
// loop. Token counters and warnings are aggregated across every Run
// call; background memory compression is scoped to each Run
// conversation (see compressionState).
type Runner struct {
	deps                  Deps
	task                  Task
	totalInputTokens      int64 // atomically updated
	totalOutputTokens     int64
	totalCacheReadTokens  int64
	totalCacheWriteTokens int64
	warningsMu            sync.Mutex
	warnings              []AgentWarning
	toolCallsMu           sync.Mutex
	toolCalls             map[string]int64
	toolCallSequence      int64
	toolFailures          []ToolFailureDetail
	// bg tracks every background goroutine that can still issue an LLM
	// request after Run returned. WaitBackground joins them so a
	// retry-report Freeze at the run boundary cannot observe an
	// un-finalized request. See WaitBackground.
	bg sync.WaitGroup
}

// ToolFailureDetail describes one failed registered-tool invocation.
type ToolFailureDetail struct {
	ToolCallNumber int64  `json:"tool_call_number"`
	ToolName       string `json:"tool_name"`
	// FilePath carries the failing subtask's key, which is a file path only in
	// scan; a review group of several files reports its group key here. The
	// field reaches --format json as file_path, so a consumer must not assume
	// it parses as a single path.
	FilePath string `json:"file_path,omitempty"`
	// Arguments is the raw tool-call argument string returned by the LLM.
	Arguments string `json:"arguments"`
	Error     string `json:"error"`
}

// NewRunner returns a Runner bound to the given dependencies.
func NewRunner(deps Deps, task Task) *Runner {
	return &Runner{deps: deps, task: task}
}

// WaitBackground blocks until every background job started by this Runner has
// returned. Background memory compression is the only such job, and
// cancelPendingCompression cancels it without waiting — its goroutine can
// therefore still be inside an LLM request after Run returned. Callers
// that freeze a retry report at the run boundary must join here first:
// RetryCollector.Freeze rejects any request that has not been finalized and
// discards the whole report, which would otherwise be an intermittent race.
//
// Every pending job has already been cancelled by the time the last
// Run returns (cancelPendingCompression runs as a deferred call on
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

// ToolFailures returns failed tool calls ordered by their global call number.
func (r *Runner) ToolFailures() []ToolFailureDetail {
	r.toolCallsMu.Lock()
	defer r.toolCallsMu.Unlock()

	out := append([]ToolFailureDetail(nil), r.toolFailures...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].ToolCallNumber < out[j].ToolCallNumber
	})
	return out
}

func (r *Runner) RecordToolCall(name string) int64 {
	r.toolCallsMu.Lock()
	defer r.toolCallsMu.Unlock()
	if r.toolCalls == nil {
		r.toolCalls = make(map[string]int64)
	}
	r.toolCalls[name]++
	r.toolCallSequence++
	return r.toolCallSequence
}

func (r *Runner) RecordToolFailure(number int64, name, taskKey, errMsg string,
	rec *session.TaskRecord, rawArguments string, duration time.Duration) {
	detail := ToolFailureDetail{
		ToolCallNumber: number,
		ToolName:       name,
		FilePath:       taskKey,
		Arguments:      rawArguments,
		Error:          errMsg,
	}

	r.toolCallsMu.Lock()
	r.toolFailures = append(r.toolFailures, detail)
	r.toolCallsMu.Unlock()

	if rec != nil {
		rec.AddToolFailure(name, rawArguments, errMsg, duration)
	}
}

// RecordUsage adds the prompt/completion/cache tokens reported by an LLM
// response to the runner's aggregate counters. Used by callers (plan phase
// in agent / future scan phases) that perform their own LLM calls outside
// Run.
func (r *Runner) RecordUsage(u *llm.UsageInfo) {
	if u == nil {
		return
	}
	atomic.AddInt64(&r.totalInputTokens, u.PromptTokens)
	atomic.AddInt64(&r.totalOutputTokens, u.CompletionTokens)
	atomic.AddInt64(&r.totalCacheReadTokens, u.CacheReadTokens)
	atomic.AddInt64(&r.totalCacheWriteTokens, u.CacheWriteTokens)
}

// MainLoopStop classifies why Run stopped without an explicit completion
// and without a Go error. It lets the caller attribute a precise, honest failure
// classification instead of guessing from free text: only a configured limit
// (max tool-request rounds) is a budget stop; the empty-round and compression
// exits are genuine but unclassifiable, so they map to the unknown catch-all.
// StopNone means the loop returned via a completion outcome in a normal round or via an
// error. A completion carried by the grace round is the exception: it completes
// the run while the stop keeps naming the budget exit that reached that round,
// so the completed flag, not the stop, is what separates success from failure.
type MainLoopStop int

const (
	// StopNone — Run completed via a completion outcome in a normal round, or
	// returned an error; the stop cause carries no additional meaning.
	StopNone MainLoopStop = iota
	// StopMaxRounds — the configured MaxToolRequestTimes round budget was
	// exhausted before completion. This is a declared budget limit.
	StopMaxRounds
	// StopEmptyRounds — the model returned no usable tool result for too many
	// consecutive rounds. Not a declared budget; unclassifiable.
	StopEmptyRounds
	// StopCompression — context compression exceeded its threshold, so the loop
	// could not continue. Token/context driven but not a declared budget.
	StopCompression
	// StopTokenBudget — the run's aggregate token budget (Deps.MaxTokensBudget)
	// was already exceeded when the next round was about to start. This is a
	// declared budget limit, like StopMaxRounds.
	StopTokenBudget
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
	case StopTokenBudget:
		return "token_budget"
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
	case StopTokenBudget:
		return "reached the aggregate token budget before finishing"
	default:
		return fmt.Sprintf("main task stopped for an unrecognized reason (stop=%d)", int(s))
	}
}

func (r *Runner) Run(ctx context.Context, messages []llm.Message, taskKey string) (RunResult, error) {
	completed, stop, err := r.runMainTask(ctx, messages, taskKey)
	return RunResult{Completed: completed, StopReason: stop}, err
}

func (r *Runner) runMainTask(ctx context.Context, messages []llm.Message, taskKey string) (bool, MainLoopStop, error) {
	// Every round of this loop re-sends the growing conversation, so each
	// request is a prefix extension of the previous one — exactly what
	// provider prompt caches reuse. Scope the affinity key to this subtask's
	// main-task conversation so every round routes to the same cache node.
	ctx = llm.ContextWithSessionKey(ctx,
		llm.SessionTaskKey(r.deps.Session.SessionID, string(session.MainTask), taskKey))

	toolReqCount := r.task.MaxRounds
	const maxConsecutiveEmptyRounds = 3
	consecutiveEmptyRounds := 0
	sessionID := uuid.NewString()
	if closer, ok := r.deps.LLMClient.(interface{ CloseSession(string) }); ok {
		defer closer.CloseSession(sessionID)
	}

	// Async compression is owned by this conversation alone; the deferred
	// cancel aborts any job still in flight when the conversation ends.
	st := &compressionState{}
	defer r.cancelPendingCompression(st)

	// stop defaults to StopMaxRounds: if the for-loop exits because toolReqCount
	// reached zero, the run stopped on the round budget. The empty-round,
	// compression and token-budget breaks overwrite it at their trigger points.
	stop := StopMaxRounds
	for toolReqCount > 0 {
		select {
		case <-ctx.Done():
			return false, StopNone, ctx.Err()
		default:
		}

		// The aggregate budget is checked here, before the request that would
		// spend past it, rather than only at subtask dispatch: a conversation
		// grows with every round and re-sends its whole history, so the one
		// long group is exactly the spender a between-subtasks gate never sees.
		if r.tokenBudgetExceeded() {
			stop = StopTokenBudget
			break
		}

		toolReqCount--

		fs := r.deps.Session.GetOrCreateFileSession(taskKey)
		rec := fs.AppendTaskRecord(session.MainTask, append([]llm.Message(nil), messages...))
		startTime := time.Now()

		// Scoped to this round: ctx itself must stay identity-free so each
		// iteration's meta replaces the previous one instead of nesting.
		reqCtx := r.RequestContext(ctx, taskKey, session.MainTask, rec.RequestNo)

		_, llmSpan := telemetry.StartLLMSpan(ctx, r.deps.Model)
		request := llm.ChatRequest{
			Model:     r.deps.Model,
			Messages:  messages,
			Tools:     r.task.ToolDefs,
			MaxTokens: r.task.MaxCompletionTokens,
			SessionID: sessionID,
		}
		resp, err := r.deps.LLMClient.CompletionsWithCtx(reqCtx, request)
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
			fmt.Fprintf(stdout.Writer(), "[ocr] No tool calls parsed for %s, retrying...\n", taskKey)
			messages = append(messages, llm.NewTextMessage("user", r.task.EmptyResponseMessage))
			native := resp.Native()
			reasoning := resp.ReasoningContent()
			if content != "" || native.Payload != nil || reasoning != "" {
				messages = append(messages[:len(messages)-1], llm.NewToolCallMessage(content, nil, native, reasoning), messages[len(messages)-1])
			}
			continue
		}

		var results []ToolCallResult
		taskCompleted := false
		hasValidResult := false

		// Capture the model's native reasoning content for this turn. Models
		// without a reasoning channel leave it empty.
		// Reasoning is turn-level, so all tool calls in this turn share it.
		thinking := resp.ReasoningContent()
		for _, call := range calls {
			cp := r.executeTool(ctx, request.Tools, taskKey, call, rec, thinking)
			if cp.Action == Fail {
				return false, StopNone, fmt.Errorf("task failed: %s", cp.Content)
			} else if cp.Action == Complete {
				results = append(results, ToolCallResult{
					ToolCallID: call.ID,
					Name:       call.Function.Name,
					Result:     cp.Content,
				})
				taskCompleted = true
			} else if cp.Content != "" {
				results = append(results, ToolCallResult{
					ToolCallID: call.ID,
					Name:       call.Function.Name,
					Result:     cp.Content,
				})
				hasValidResult = true
			} else {
				results = append(results, ToolCallResult{
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
				fmt.Fprintf(stdout.Writer(), "[ocr] Too many empty retries for %s, stopping.\n", taskKey)
				stop = StopEmptyRounds
				break
			}
			fmt.Fprintf(stdout.Writer(), "[ocr] No valid tool results for %s, retrying...\n", taskKey)
		} else {
			consecutiveEmptyRounds = 0
		}

		succeed := r.addNextMessage(ctx, content, calls, resp.Native(), thinking, results, &messages, taskKey, st)
		if !succeed {
			fmt.Fprintf(stdout.Writer(), "[ocr] Context compression exceeded threshold for %s, stopping.\n", taskKey)
			stop = StopCompression
			break
		}
	}

	// The grace round after a spent budget is the last chance to finish the
	// task, so a completion outcome it carries counts. The caller records
	// the subtask as incomplete otherwise, and a resume re-runs a file the
	// model already declared done.
	completed := false
	switch stop {
	case StopMaxRounds:
		fmt.Fprintf(stdout.Writer(), "[ocr] Max tool requests reached for %s.\n", taskKey)
		completed = r.runGraceRound(ctx, messages, taskKey, sessionID)
	case StopTokenBudget:
		fmt.Fprintf(stdout.Writer(), "[ocr] Token budget exceeded (used %d > budget %d) for %s.\n",
			r.TotalTokensUsed(), r.deps.MaxTokensBudget, taskKey)
		completed = r.runGraceRound(ctx, messages, taskKey, sessionID)
	}
	return completed, stop, nil
}

// Check the current request before dispatch so restricted rounds cannot invoke
// providers or completion actions that were only available in earlier rounds.
func (r *Runner) executeTool(ctx context.Context, definitions []llm.ToolDef, taskKey string, call llm.ToolCall, rec *session.TaskRecord, thinking string) ToolOutcome {
	for _, definition := range definitions {
		if definition.Type == "function" && definition.Function.Name == call.Function.Name {
			return r.task.ExecuteTool(ctx, taskKey, call, rec, thinking)
		}
	}
	return ToolOutcome{Content: tool.NotAvailableMsg}
}

// tokenBudgetExceeded reports whether the run's aggregate token usage is past
// Deps.MaxTokensBudget. A zero budget never trips.
func (r *Runner) tokenBudgetExceeded() bool {
	budget := r.deps.MaxTokensBudget
	return budget > 0 && r.TotalTokensUsed() > budget
}

func (r *Runner) runGraceRound(ctx context.Context, messages []llm.Message, taskKey string, sessionID string) bool {
	graceDefs := r.task.Grace.Tools
	if len(graceDefs) == 0 {
		return false
	}

	messages = append(messages, r.task.Grace.Messages...)

	if ctx.Err() != nil {
		fmt.Fprintf(stdout.Writer(), "[ocr] Grace round skipped for %s: context cancelled\n", taskKey)
		return false
	}

	fs := r.deps.Session.GetOrCreateFileSession(taskKey)
	rec := fs.AppendTaskRecord(session.MainTask, messages)
	startTime := time.Now()
	reqCtx := r.RequestContext(ctx, taskKey, session.MainTask, rec.RequestNo)

	_, llmSpan := telemetry.StartLLMSpan(ctx, r.deps.Model)
	request := llm.ChatRequest{
		Model:     r.deps.Model,
		Messages:  messages,
		Tools:     graceDefs,
		MaxTokens: r.task.MaxCompletionTokens,
		SessionID: sessionID,
	}
	resp, err := r.deps.LLMClient.CompletionsWithCtx(reqCtx, request)
	duration := time.Since(startTime)
	if err != nil {
		rec.SetError(err, duration)
		telemetry.RecordLLMResult(llmSpan, duration, 0, err)
		llmSpan.End()
		telemetry.RecordLLMRequest(ctx, r.deps.Model, duration, 0, "error")
		fmt.Fprintf(stdout.Writer(), "[ocr] Grace round LLM error for %s: %v\n", taskKey, err)
		return false
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
		return false
	}

	// Any explicit failure keeps the round incomplete, even if another tool
	// in the same response reported completion.
	completed := false
	failed := false
	thinking := resp.ReasoningContent()
	for _, call := range calls {
		checkpoint := r.executeTool(ctx, request.Tools, taskKey, call, rec, thinking)
		completed = completed || checkpoint.Action == Complete
		failed = failed || checkpoint.Action == Fail
	}
	return completed && !failed
}

// addNextMessage extends the conversation with the assistant message and
// tool responses, applying three-zone compression at the soft (60%) and
// warning (80%) MaxTokens thresholds. Returns false when even after
// synchronous compression the conversation is still over the warning
// threshold — caller should stop the loop in that case.
func (r *Runner) addNextMessage(ctx context.Context, assistantContent string, toolCalls []llm.ToolCall, native llm.NativeTurn, reasoningContent string, results []ToolCallResult, messages *[]llm.Message, taskKey string, st *compressionState) bool {
	maxAllowed := r.task.MaxContextTokens
	softLimit := int(float64(maxAllowed) * tokenSoftThreshold)
	warnLimit := PromptTokenLimit(maxAllowed)

	r.tryApplyPendingCompression(st, messages)

	// A conversation can already be over the warning threshold before this
	// round's messages are appended (e.g. an oversized initial prompt).
	if CountMessagesTokens(*messages) > warnLimit {
		r.cancelPendingCompression(st)
		var err error
		if *messages, err = r.runCompression(ctx, *messages, taskKey); err != nil {
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
		if *messages, err = r.runCompression(ctx, *messages, taskKey); err != nil {
			fmt.Fprintf(stdout.Writer(), "[ocr] Memory compression failed: %v\n", err)
		}
		finalCount = CountMessagesTokens(*messages)
	}

	// Trigger async compression only after all appends for this update, so
	// a job is never started and then immediately cancelled by the same
	// call (#384), and never started when we are about to return false.
	if finalCount > softLimit && finalCount < warnLimit {
		r.triggerAsyncCompression(ctx, st, *messages, taskKey)
	}

	return finalCount < warnLimit
}

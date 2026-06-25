package llmloop

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/open-code-review/open-code-review/internal/config/template"
	"github.com/open-code-review/open-code-review/internal/diff"
	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/session"
	"github.com/open-code-review/open-code-review/internal/stdout"
	"github.com/open-code-review/open-code-review/internal/telemetry"
	"github.com/open-code-review/open-code-review/internal/tool"
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
}

// Runner is a per-session (across files) executor of the LLM tool-use
// loop. Token counters, warnings, and the optional background compression
// job are aggregated across every RunPerFile call.
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
	compressionMu         sync.Mutex
	pendingJob            *compressionJob
}

// NewRunner returns a Runner bound to the given dependencies.
func NewRunner(deps Deps) *Runner {
	return &Runner{deps: deps}
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

// RunPerFile drives the main LLM conversation loop for a single file.
// It sends messages with the configured tool definitions, executes any
// tool calls returned by the model, and collects review comments until
// task_done is called or limits are reached. Token usage and warnings
// are aggregated on the Runner across all files.
func (r *Runner) RunPerFile(ctx context.Context, messages []llm.Message, newPath string) error {
	toolReqCount := r.deps.Template.MaxToolRequestTimes
	const maxConsecutiveEmptyRounds = 3
	consecutiveEmptyRounds := 0

	for toolReqCount > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		toolReqCount--

		fs := r.deps.Session.GetOrCreateFileSession(newPath)
		rec := fs.AppendTaskRecord(session.MainTask, append([]llm.Message(nil), messages...))
		startTime := time.Now()

		resp, err := r.deps.LLMClient.CompletionsWithCtx(ctx, llm.ChatRequest{
			Model:     r.deps.Model,
			Messages:  messages,
			Tools:     r.deps.MainToolDefs,
			MaxTokens: r.deps.Template.MaxTokens,
		})
		duration := time.Since(startTime)
		if err != nil {
			rec.SetError(err, duration)
			telemetry.RecordLLMRequest(ctx, r.deps.Model, duration, 0, "error")
			return fmt.Errorf("LLM completion error: %w", err)
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
		telemetry.RecordLLMRequest(ctx, r.deps.Model, duration, totalTokens, "ok")

		content := resp.Content()
		calls := resp.ToolCalls()

		if len(calls) == 0 {
			fmt.Fprintf(stdout.Writer(), "[ocr] No tool calls parsed for %s, retrying...\n", newPath)
			messages = append(messages, llm.NewTextMessage("user", "You did not successfully call any tools. Please try again or use task_done if finished."))
			if content != "" {
				messages = append(messages[:len(messages)-1], llm.NewTextMessage("assistant", content), messages[len(messages)-1])
			}
			continue
		}

		var results []tool.ToolCallResult
		taskCompleted := false
		hasValidResult := false

		for _, call := range calls {
			cp := r.executeToolCall(ctx, newPath, call, rec)
			if cp.Completed {
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
			break
		}
		if !hasValidResult {
			consecutiveEmptyRounds++
			if consecutiveEmptyRounds >= maxConsecutiveEmptyRounds {
				fmt.Fprintf(stdout.Writer(), "[ocr] Too many empty retries for %s, stopping.\n", newPath)
				break
			}
			fmt.Fprintf(stdout.Writer(), "[ocr] No valid tool results for %s, retrying...\n", newPath)
		} else {
			consecutiveEmptyRounds = 0
		}

		succeed := r.addNextMessage(ctx, content, calls, results, &messages, newPath)
		if !succeed {
			fmt.Fprintf(stdout.Writer(), "[ocr] Context compression exceeded threshold for %s, stopping.\n", newPath)
			break
		}
	}

	if toolReqCount <= 0 {
		fmt.Fprintf(stdout.Writer(), "[ocr] Max tool requests reached for %s.\n", newPath)
	}
	return nil
}

// executeToolCall dispatches a single tool call from the LLM response and
// records the result in session history. code_comment handling includes
// optional async dispatch through CommentWorkerPool plus line-number
// resolution / re-location.
func (r *Runner) executeToolCall(ctx context.Context, newPath string, call llm.ToolCall, rec *session.TaskRecord) tool.TaskCheckpoint {
	t := tool.OfName(call.Function.Name)
	if !t.IsKnown() {
		return tool.Of(tool.NotAvailableMsg)
	}

	if t == tool.TaskDone {
		return tool.Complete()
	}

	p := lookupTool(r.deps.Tools, t)
	if p == nil {
		return tool.Of(tool.NotAvailableMsg)
	}

	r.recordToolCall(t.Name())

	var args map[string]any
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return tool.Of(fmt.Sprintf("Error parsing tool arguments for %s: %v", t.Name(), err))
	}

	// Inject current file path as default for code_comment when not provided.
	if t == tool.CodeComment && newPath != "" {
		if _, ok := args["path"]; !ok {
			args["path"] = newPath
		}
	}

	startTime := time.Now()

	if t == tool.CodeComment {
		telemetry.PrintToolCallStarted(t.Name(), args)

		comments, errMsg := tool.ParseComments(args)
		if errMsg != "" {
			telemetry.RecordToolCall(ctx, t.Name(), time.Since(startTime), false)
			return tool.Of(errMsg)
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
						rlStart := time.Now()
						_, resp, msgs := diff.ReLocateComment(rctx, cm, d, r.deps.LLMClient, r.deps.Template.ReLocationTask, r.deps.Model, r.deps.Template.MaxTokens)
						if msgs != nil {
							fs := r.deps.Session.GetOrCreateFileSession(cm.Path)
							rlRec := fs.AppendTaskRecord(session.ReLocationTask, msgs)
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
			pool.Submit(func() ([]model.LlmComment, error) {
				resolveAndCollect(asyncCtx)
				telemetry.PrintToolCallFinished(toolName, time.Since(startTime))
				return []model.LlmComment{}, nil
			})
			telemetry.RecordToolCall(asyncCtx, toolName, time.Since(startTime), true)
			return tool.Of(tool.CommentSucceed)
		}

		resolveAndCollect(ctx)
		dur := time.Since(startTime)
		telemetry.RecordToolCall(ctx, t.Name(), dur, true)
		telemetry.PrintToolCallFinished(t.Name(), dur)
		if rec != nil {
			rec.AddToolResult(t.Name(), call.Function.Arguments, tool.CommentSucceed)
		}
		return tool.Of(tool.CommentSucceed)
	}

	// Synchronous path for all other tools
	telemetry.PrintToolCallStarted(t.Name(), args)
	result, err := p.Execute(ctx, args)
	dur := time.Since(startTime)
	ok := err == nil
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
func (r *Runner) addNextMessage(ctx context.Context, assistantContent string, toolCalls []llm.ToolCall, results []tool.ToolCallResult, messages *[]llm.Message, filePath string) bool {
	maxAllowed := r.deps.Template.MaxTokens
	softLimit := int(float64(maxAllowed) * tokenSoftThreshold)
	warnLimit := int(float64(maxAllowed) * tokenWarningThreshold)

	r.tryApplyPendingCompression(messages)

	tokenCount := CountMessagesTokens(*messages)

	if tokenCount > warnLimit {
		r.cancelPendingCompression()
		*messages, _ = r.runCompression(ctx, *messages, filePath)
		tokenCount = CountMessagesTokens(*messages)
	}

	if tokenCount > softLimit && r.pendingJob == nil {
		r.triggerAsyncCompression(ctx, *messages, filePath)
	}

	if len(toolCalls) > 0 {
		*messages = append(*messages, llm.NewToolCallMessage(assistantContent, toolCalls))
	} else if assistantContent != "" {
		*messages = append(*messages, llm.NewTextMessage("assistant", assistantContent))
	}

	for _, rs := range results {
		*messages = append(*messages, llm.NewToolResultMessage(rs.ToolCallID, rs.Result))
	}

	finalCount := CountMessagesTokens(*messages)
	if finalCount > warnLimit {
		r.cancelPendingCompression()
		*messages, _ = r.runCompression(ctx, *messages, filePath)
	}

	return CountMessagesTokens(*messages) < warnLimit
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

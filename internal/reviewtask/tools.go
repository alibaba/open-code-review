// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package reviewtask

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/tool"
	"strings"
	"time"
)

// executeToolCall dispatches a single tool call from the LLM response and
// records the result in session history. code_comment handling includes
// optional async dispatch through CommentWorkerPool plus line-number
// resolution / re-location.
//
// taskKey is the calling subtask's key (see RunMainTask). It labels the
// session/telemetry records and the worker-pool submission, and stands in as a
// code_comment's path when the model omitted one.
func (r *Runner) executeToolCall(ctx context.Context, taskKey string, call llm.ToolCall, rec *session.TaskRecord, thinking string) tool.TaskCheckpoint {
	t := tool.OfName(call.Function.Name)

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

	toolName := call.Function.Name
	p, found := r.deps.Tools.Get(toolName)
	if !found {
		return r.toolFailureResult(taskKey, toolName, tool.NotAvailableMsg)
	}

	toolCallNumber := r.RecordToolCall(toolName)
	callStarted := time.Now()

	args, err := parseToolArgs(call.Function.Arguments)
	if err != nil {
		errMsg := fmt.Sprintf("Error parsing tool arguments for %s: %v", toolName, err)
		telemetry.PrintToolCallStarted(toolName, nil)
		telemetry.PrintToolCallError(toolName, fmt.Errorf("%s", errMsg))
		r.RecordToolFailure(toolCallNumber, toolName, taskKey, errMsg,
			rec, call.Function.Arguments, time.Since(callStarted))
		return r.toolFailureResult(taskKey, toolName, errMsg)
	}

	startTime := time.Now()

	if t == tool.CodeComment {
		telemetry.PrintToolCallStarted(t.Name(), args)
		_, toolSpan := telemetry.StartToolSpan(ctx, t.Name())

		comments, repair, errMsg := tool.ParseCommentsWithPath(args, taskKey)
		if repair != nil {
			// The model sees a plain success, so this warning is the only record
			// that its `comments` violated the array schema — without it the
			// repair would absorb an unbounded number of them unobserved.
			r.RecordWarning("comment_args_repaired", taskKey, repair.Message())
		}
		// Report failure so the model can retry instead of assuming its findings were saved.
		if errMsg == "" && len(comments) == 0 {
			errMsg = tool.CommentNoneUsableMsg
		}
		if errMsg != "" {
			dur := time.Since(startTime)
			toolErr := fmt.Errorf("%s", errMsg)
			telemetry.RecordToolResult(toolSpan, t.Name(), dur.Milliseconds(), toolErr)
			toolSpan.End()
			telemetry.RecordToolCall(ctx, t.Name(), dur, false)
			r.RecordToolFailure(toolCallNumber, toolName, taskKey, errMsg,
				rec, call.Function.Arguments, dur)
			telemetry.PrintToolCallError(t.Name(), toolErr)
			return r.toolFailureResult(taskKey, toolName, errMsg)
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
				// The model may have named a line. ResolveComment treats any
				// positive line as already resolved, which would skip the file
				// match. Hold the hint aside so a verbatim existing_code still
				// wins, then put the hint back only if every resolver misses
				// and the line actually falls inside the file. Scan's synthetic
				// diff has no unified diff text, so this hint is the only
				// anchor left when the snippet is not a whole-line match.
				hintStart, hintEnd := cm.StartLine, cm.EndLine
				cm.StartLine, cm.EndLine = 0, 0
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
							// needs. Reading it from the comment rather than from
							// taskKey is what makes this correct for a grouped
							// review: taskKey is only the comment's fallback path,
							// so a group of several files resolves each of its
							// comments under the file the comment names.
							rlCtx := llm.ContextWithSessionKey(rctx,
								llm.SessionTaskKey(r.deps.Session.SessionID, string(session.ReLocationTask), cm.Path))
							reqCtx := r.RequestContext(rlCtx, cm.Path, session.ReLocationTask, rlRec.RequestNo)
							relocated, resp := diff.ReLocateComment(reqCtx, cm, d, r.deps.LLMClient, msgs, r.deps.Model, r.deps.Template.CompletionTokenLimit())
							if relocated {
								located = true
							}
							if resp != nil {
								rlRec.SetResponse(resp, time.Since(rlStart))
								r.RecordUsage(resp.Usage)
							} else {
								rlRec.SetError(fmt.Errorf("re-location LLM call failed"), time.Since(rlStart))
							}
						}
					}
				}
				if !located {
					if start, end, ok := lineHintInFile(d, hintStart, hintEnd); ok {
						cm.StartLine, cm.EndLine = start, end
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
			pool.SubmitFor(taskKey, func() ([]model.LlmComment, error) {
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
			r.resetToolFailureStreak(taskKey, t.Name())
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
		r.resetToolFailureStreak(taskKey, t.Name())
		return tool.Of(tool.CommentSucceed)
	}

	// Synchronous path for all other tools
	telemetry.PrintToolCallStarted(toolName, args)
	_, toolSpan := telemetry.StartToolSpan(ctx, toolName)
	result, err := p.Execute(ctx, args)
	dur := time.Since(startTime)
	ok := err == nil
	telemetry.RecordToolResult(toolSpan, toolName, dur.Milliseconds(), err)
	toolSpan.End()
	telemetry.RecordToolCall(ctx, toolName, dur, ok)

	if err != nil {
		r.RecordToolFailure(toolCallNumber, toolName, taskKey, err.Error(),
			rec, call.Function.Arguments, dur)
		telemetry.PrintToolCallError(toolName, err)
		return r.toolFailureResult(taskKey, toolName, fmt.Sprintf("Error executing tool %s: %v", toolName, err))
	}
	telemetry.PrintToolCallFinished(toolName, dur)
	if rec != nil {
		rec.AddToolResult(toolName, call.Function.Arguments, result)
	}
	r.resetToolFailureStreak(taskKey, toolName)
	return tool.Of(result)
}

// parseToolArgs unmarshals a tool call's raw JSON arguments, always
// returning a non-nil map on success: some OpenAI-compatible gateways send
// "arguments": null, which unmarshals to a nil map and would panic on the
// first write (#382). An equivalent inline guard exists in internal/llm's
// buildAnthropicParams; keep the two in sync.
//
// When raw fails to parse whole, this retries against just its first balanced
// top-level JSON value (see extractTopLevelJSON) before giving up: a model
// that appends non-JSON content after otherwise-valid arguments would
// otherwise have the whole call rejected and retry the same finding
// unbounded. A raw string with no balanced value at all still fails with its
// original error, unchanged from before.
func parseToolArgs(raw string) (map[string]any, error) {
	var args map[string]any
	err := json.Unmarshal([]byte(raw), &args)
	if err != nil {
		span, ok := extractTopLevelJSON(raw)
		if !ok {
			return nil, err
		}
		var spanArgs map[string]any
		if spanErr := json.Unmarshal([]byte(span), &spanArgs); spanErr != nil {
			return nil, err
		}
		args = spanArgs
	}
	if args == nil {
		args = make(map[string]any)
	}
	return args, nil
}

// lineHintInFile reports whether the model-supplied range is a real anchor.
// A missing end line means a single-line comment. A range past the end of
// NewFileContent is not anchorable. An empty body has no lines: a scanned
// empty file, a deleted diff, a binary diff, and a failed read all stay
// unanchored. Only a nil diff, where nothing was loaded to disprove the
// line, keeps a positive range.
func lineHintInFile(d *model.Diff, start, end int) (int, int, bool) {
	if start <= 0 {
		return 0, 0, false
	}
	if end <= 0 {
		end = start
	}
	if end < start {
		return 0, 0, false
	}
	if d == nil {
		return start, end, true
	}
	n := contentLineCount(d.NewFileContent)
	if start > n || end > n {
		return 0, 0, false
	}
	return start, end, true
}

func contentLineCount(content string) int {
	if content == "" {
		return 0
	}
	n := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		n++
	}
	return n
}

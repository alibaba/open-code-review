// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors
package llmloop

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/alibaba/open-code-review/internal/tool"
)

// Repeated-call guard.
//
// A model can get stuck issuing the same tool call -- the same name and the
// same arguments -- whose result does not change. The loop otherwise runs it
// until MAX_TOOL_REQUEST_TIMES or the token budget stops it, and every round
// resends the whole conversation, so one stuck task can spend a run's entire
// budget: a one-file review was seen alternating two identical code_search
// calls 144 times, 5M tokens, before the budget ended it. Greedy decoding
// makes this deterministic, but any model can do it.
//
// The guard keeps each task's last repeatWindow calls. When the call being
// made has already appeared repeatWarnAt times in that window it is not
// executed; the model is told it is repeating itself and what to do instead.
// At repeatFailAt the task fails, so the budget goes to the other tasks. A
// window rather than a consecutive-streak counter, because the loop seen in
// practice alternated two calls: each repeated, neither consecutively.
//
// The window is per task and a different call clears nothing: a review that
// legitimately re-reads one file a couple of times between other calls stays
// well under the threshold, and the failure-streak counter in
// tool_failure_streak.go is unaffected.
const (
	repeatWindow = 12
	repeatWarnAt = 3
	repeatFailAt = 6
)

type toolRepeatState struct {
	mu     sync.Mutex
	recent map[string][]string // taskKey -> the last repeatWindow call keys, oldest first
	warned map[string]int      // taskKey -> warnings recorded, for the run's warning list
}

// toolCallKey identifies one call by tool name and canonical arguments. The
// parsed map is re-encoded so key order and whitespace in the raw JSON do not
// make two identical calls look different.
func toolCallKey(toolName string, args map[string]any) string {
	canon, err := json.Marshal(args)
	if err != nil {
		canon = []byte(fmt.Sprintf("%v", args))
	}
	return toolName + "\x00" + string(canon)
}

// repeatCount returns how many times callKey appears in the task's window,
// plus one for the call being considered, without recording anything.
func (r *Runner) repeatCount(taskKey, callKey string) int {
	r.toolRepeat.mu.Lock()
	defer r.toolRepeat.mu.Unlock()
	n := 1
	for _, k := range r.toolRepeat.recent[taskKey] {
		if k == callKey {
			n++
		}
	}
	return n
}

// recordToolRepeat appends a call to the task's window. Called for a call that
// ran and succeeded, and for one the guard refused -- both are attempts the
// model made. A call that ran and failed, or ran and returned nothing, is not
// recorded: repeated failures are the failure streak's business
// (tool_failure_streak.go) and repeated empty results the empty-rounds stop's,
// and counting them here too would end a task those were about to steer.
func (r *Runner) recordToolRepeat(taskKey, callKey string) {
	r.toolRepeat.mu.Lock()
	defer r.toolRepeat.mu.Unlock()
	if r.toolRepeat.recent == nil {
		r.toolRepeat.recent = make(map[string][]string)
	}
	window := append(r.toolRepeat.recent[taskKey], callKey)
	if len(window) > repeatWindow {
		window = window[len(window)-repeatWindow:]
	}
	r.toolRepeat.recent[taskKey] = window
}

// toolRepeatResult decides, before a tool runs, whether the call is a repeat
// the loop should refuse. It returns a checkpoint to hand the model instead of
// executing, or ok=false when the call should run normally -- in which case
// the caller records it with recordToolRepeat once it has succeeded.
func (r *Runner) toolRepeatResult(taskKey, toolName string, args map[string]any) (tool.TaskCheckpoint, bool) {
	key := toolCallKey(toolName, args)
	n := r.repeatCount(taskKey, key)
	if n < repeatWarnAt {
		return tool.TaskCheckpoint{}, false
	}
	r.recordToolRepeat(taskKey, key)
	if n >= repeatFailAt {
		r.RecordWarning("tool_call_loop", taskKey, fmt.Sprintf(
			"%s was called with identical arguments %d times within the last %d calls; the task was stopped",
			toolName, n, repeatWindow))
		return tool.Fail(fmt.Sprintf(
			"tool call loop: %s called with identical arguments %d times within the last %d calls",
			toolName, n, repeatWindow)), true
	}
	r.toolRepeat.mu.Lock()
	if r.toolRepeat.warned == nil {
		r.toolRepeat.warned = make(map[string]int)
	}
	first := r.toolRepeat.warned[taskKey] == 0
	r.toolRepeat.warned[taskKey]++
	r.toolRepeat.mu.Unlock()
	if first {
		r.RecordWarning("tool_call_repeated", taskKey, fmt.Sprintf(
			"%s was called with identical arguments %d times within the last %d calls; the repeat was not executed",
			toolName, n, repeatWindow))
	}
	return tool.Of(fmt.Sprintf(
		"You have already called %s with exactly these arguments %d times; its result is unchanged and is not repeated here. "+
			"Use the result you already have. If it did not answer your question, call a different tool or different arguments, "+
			"or call task_done if you have nothing further to report. Repeating this call again will end the task.",
		toolName, n-1)), true
}

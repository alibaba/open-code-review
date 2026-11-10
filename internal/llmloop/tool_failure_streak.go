// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"fmt"
	"sync"

	"github.com/alibaba/open-code-review/internal/tool"
)

// A tool call that fails carries no memory of that on its own: executeToolCall
// returns an error string as the tool result and the next round starts fresh,
// so a model whose call keeps failing for the same reason (most commonly the
// trailing-content case parseToolArgs now recovers most of, but not all of —
// e.g. a still-invalid comments payload) can resend the identical finding,
// reworded, with no cap. One review run reattempted a single finding 6 times
// before this existed. toolFailureResult tracks consecutive failures per
// (taskKey, toolName) pair and escalates the response instead of repeating the
// same retryable error forever.

// toolFailureKey identifies one (task, tool) pair for the consecutive-failure
// counter in Runner.toolFailureStreak.
type toolFailureKey struct {
	taskKey  string
	toolName string
}

// toolFailureStreakState guards the consecutive-failure counters keyed by
// toolFailureKey. It is a separate lock from toolCallsMu because it tracks a
// different concern (retry escalation, not call counts or the audit trail)
// and is read and written on every tool call, not just failing ones.
type toolFailureStreakState struct {
	mu     sync.Mutex
	counts map[toolFailureKey]int
}

// recordToolFailureStreak increments and returns the consecutive-failure count
// for one (taskKey, toolName) pair.
func (r *Runner) recordToolFailureStreak(taskKey, toolName string) int {
	r.toolFailureStreak.mu.Lock()
	defer r.toolFailureStreak.mu.Unlock()
	if r.toolFailureStreak.counts == nil {
		r.toolFailureStreak.counts = make(map[toolFailureKey]int)
	}
	key := toolFailureKey{taskKey, toolName}
	r.toolFailureStreak.counts[key]++
	return r.toolFailureStreak.counts[key]
}

// resetToolFailureStreak clears the consecutive-failure count for one
// (taskKey, toolName) pair after it succeeds.
func (r *Runner) resetToolFailureStreak(taskKey, toolName string) {
	r.toolFailureStreak.mu.Lock()
	defer r.toolFailureStreak.mu.Unlock()
	delete(r.toolFailureStreak.counts, toolFailureKey{taskKey, toolName})
}

// toolFailureResult turns one failed execution of toolName within taskKey into
// the tool-call result sent back to the model, escalating its wording as the
// pair keeps failing.
//
// The 1st and 2nd failures still read as a plain retryable error, so a model
// that can actually fix its own mistake gets the chance to; the 2nd names the
// repeat explicitly. From the 3rd failure on, the result reads as accepted
// instead of erroring, and keeps reading that way for any further attempt on
// the same pair: this drops exactly one finding, which is no worse than the
// partial outcomes RunMainTask already tolerates elsewhere (StopMaxRounds,
// StopEmptyRounds) — it does not fail the task, the file, or the review, and
// it stops inviting another attempt rather than granting the pair more retry
// budget than the three already spent.
func (r *Runner) toolFailureResult(taskKey, toolName, errMsg string) tool.TaskCheckpoint {
	switch r.recordToolFailureStreak(taskKey, toolName) {
	case 1:
		return tool.Of(errMsg)
	case 2:
		return tool.Of(fmt.Sprintf(
			"%s\nThis is the second consecutive failure calling %s for this task. "+
				"Fix your arguments, or call task_done if you have nothing further to report.",
			errMsg, toolName))
	default:
		return tool.Of(fmt.Sprintf(
			"%s has failed repeatedly for this task and has been skipped; it will not be retried further. "+
				"Move on to your next finding, or call task_done if you have nothing further to report.",
			toolName))
	}
}

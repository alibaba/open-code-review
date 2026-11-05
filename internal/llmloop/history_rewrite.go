// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"strings"

	"github.com/alibaba/open-code-review/internal/llm"
)

const (
	// toolRetryNudgePrefix matches any retry hint this loop (or earlier
	// versions of it) appended to the history after a round that parsed to no
	// tool calls. The re-anchor keeps the prefix so retries appended after it
	// strip the same way.
	toolRetryNudgePrefix = "You did not successfully call any tools"

	// toolRetryReanchor is the single instruction left in the history after
	// failed retries are stripped: it names the native tool-call format and
	// makes explicit that plain text answers are discarded, so the model
	// corrects instead of repeating its prose.
	toolRetryReanchor = toolRetryNudgePrefix + ". Respond NOW with a native tool call: use task_done if the review is finished, or code_comment to file a finding. Plain text answers are discarded."
)

// stripFailedToolRounds removes every trailing [assistant prose, user retry
// hint] pair from the conversation history and returns the shortened slice.
// Rounds that did produce tool calls (assistant ToolCalls plus their tool
// results) are kept, as is the opening system/user context.
//
// The retry loop replays a prose answer as an assistant message and appends a
// retry hint. Rendered through the model's chat template, that transcript
// shows the model's own prose as if it were a valid prior answer — an
// in-context example of the wrong format. Small local models complete the
// repeating [prose, hint] pattern instead of correcting, locking into dozens
// of identical failed rounds. Stripping the failed pairs keeps at most one in
// the history, so a slip costs one round instead of a hundred.
func stripFailedToolRounds(messages []llm.Message) []llm.Message {
	// Keep at least the opening context: never strip below two messages.
	for len(messages) > 2 &&
		isRetryHint(messages[len(messages)-1]) &&
		isFailedAssistantTurn(messages[len(messages)-2]) {
		messages = messages[:len(messages)-2]
	}
	return messages
}

// isRetryHint reports whether msg is a user retry hint appended by the loop
// after a round that parsed to no tool calls.
func isRetryHint(msg llm.Message) bool {
	return msg.Role == "user" && strings.HasPrefix(msg.ExtractText(), toolRetryNudgePrefix)
}

// isFailedAssistantTurn reports whether msg is an assistant turn without tool
// calls — the prose answer that triggered a retry hint.
func isFailedAssistantTurn(msg llm.Message) bool {
	return msg.Role == "assistant" && len(msg.ToolCalls) == 0
}

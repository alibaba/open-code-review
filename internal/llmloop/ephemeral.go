// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/alibaba/open-code-review/internal/llm"
)

// Large tool results are sent once, for the round that consumes them.
//
// The main loop re-sends its whole growing conversation on every round, so a
// single chunk of read context stays in every subsequent request unless
// something removes it. Provider prompt caches make the repeat cheap but not
// free, and the replay is not the mechanism this run relies on for safety.
//
// A result is therefore projected out of the request the round after the model
// has answered with it, and replaced by a compact, deterministic receipt. The
// original message is left untouched in the conversation: async memory
// compression still summarizes the history the run actually had, and a
// resumed or inspected session still shows what was read.
const (
	// DefaultContextResultThreshold is the tool-result size, in tokens, at or
	// above which a result is treated as review context rather than as a
	// status line. Callers override it through Deps.ContextResultThreshold.
	DefaultContextResultThreshold = 2048
	// MinContextResultThreshold floors the override so a tiny configured
	// budget cannot turn every status message into a receipt.
	MinContextResultThreshold = 256
)

// ContextStats is the accounting for one conversation: how much raw context
// was sent, how many payloads were replaced by a receipt, and how big the
// requests got. The token counts are this package's own measurement of what it
// handed to the client; they are not provider-reported usage and are never
// added to it.
//
// Re-sends of the same content are not counted here. A re-send of a chunk is a
// second tool call producing a fresh payload under a fresh call id, which this
// ledger cannot distinguish from a first read; the chunk store can, and reports
// it as chunk refetches.
type ContextStats struct {
	// RawTokensSent is the token count of context payloads sent for the first
	// time.
	RawTokensSent int64
	// ReceiptsIssued counts the payloads replaced by a receipt.
	ReceiptsIssued int64
	// MaxEstimatedRequestTokens is the largest estimated input size of any
	// request this conversation issued, counted over messages only - tool
	// definitions are a fixed per-request term, not a per-conversation one.
	MaxEstimatedRequestTokens int64
	// LastEstimatedRequestTokens is the estimate for the most recent request.
	LastEstimatedRequestTokens int64
}

// contextLedger tracks, per conversation, which tool results have already been
// sent raw and what they cost. One ledger belongs to one RunMainTask call: the
// Runner is shared across subtasks, and a shared ledger would let one subtask's
// receipt strip another subtask's payload.
type contextLedger struct {
	threshold int

	mu       sync.Mutex
	sent     map[string]int
	tokens   map[string]int
	rawFirst int64
	receipts int64
	maxReq   int64
	lastReq  int64
}

func newContextLedger(threshold int) *contextLedger {
	if threshold <= 0 {
		threshold = DefaultContextResultThreshold
	}
	if threshold < MinContextResultThreshold {
		threshold = MinContextResultThreshold
	}
	return &contextLedger{
		threshold: threshold,
		sent:      make(map[string]int),
		tokens:    make(map[string]int),
	}
}

// prepare returns the messages to send for one request: the conversation with
// every already-consumed large tool result replaced by its receipt.
//
// It also records what this request sends, which is the only place the run
// learns how often a payload actually reaches a provider. The input slice is
// never mutated - the caller keeps the real history.
func (l *contextLedger) prepare(messages []llm.Message) []llm.Message {
	if l == nil {
		return messages
	}

	out := make([]llm.Message, len(messages))
	copy(out, messages)

	var rawThisRequest, replaced int64
	for i := range out {
		m := &out[i]
		if m.Role != "tool" || m.ToolCallID == "" {
			continue
		}
		if _, isString := m.Content.(string); !isString {
			// Provider-native content blocks are left alone: rewriting them
			// would mean reshaping an SDK-validated payload.
			continue
		}
		text := m.ExtractText()
		tokens := llm.CountTokens(text)
		if tokens < l.threshold {
			continue
		}

		l.mu.Lock()
		seen := l.sent[m.ToolCallID]
		l.tokens[m.ToolCallID] = tokens
		if seen == 0 {
			l.sent[m.ToolCallID] = 1
			rawThisRequest += int64(tokens)
		} else {
			m.Content = receiptFor(m.ToolCallID, tokens, seen+1)
			replaced++
		}
		l.mu.Unlock()
	}

	estimated := CountMessagesTokens(out)
	l.mu.Lock()
	l.rawFirst += rawThisRequest
	l.receipts += replaced
	if int64(estimated) > l.maxReq {
		l.maxReq = int64(estimated)
	}
	l.lastReq = int64(estimated)
	l.mu.Unlock()

	return out
}

// stats returns the ledger's accounting.
func (l *contextLedger) stats() ContextStats {
	if l == nil {
		return ContextStats{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return ContextStats{
		RawTokensSent:              l.rawFirst,
		ReceiptsIssued:             l.receipts,
		MaxEstimatedRequestTokens:  l.maxReq,
		LastEstimatedRequestTokens: l.lastReq,
	}
}

// fold adds a finished ledger into the run-wide counters.
func (l *contextLedger) fold(r *Runner) {
	s := l.stats()
	atomic.AddInt64(&r.rawContextTokensSent, s.RawTokensSent)
	atomic.AddInt64(&r.contextReceipts, s.ReceiptsIssued)
	atomic.CompareAndSwapInt64(&r.maxEstimatedRequestTokens, 0, s.MaxEstimatedRequestTokens)
	for {
		cur := atomic.LoadInt64(&r.maxEstimatedRequestTokens)
		if s.MaxEstimatedRequestTokens <= cur || atomic.CompareAndSwapInt64(&r.maxEstimatedRequestTokens, cur, s.MaxEstimatedRequestTokens) {
			break
		}
	}
	atomic.StoreInt64(&r.lastEstimatedRequestTokens, s.LastEstimatedRequestTokens)
}

// receiptFor renders the compact stand-in for a payload already read. The
// wording states what happened (fetched previously, raw content omitted) rather
// than what the model is supposed to have concluded from it: a receipt is a
// record of transport, not a claim about the model's reasoning.
func receiptFor(toolCallID string, tokens, timesSent int) string {
	var b strings.Builder
	b.WriteString("[context receipt] ")
	b.WriteString(fmt.Sprintf("~%d tokens were fetched previously; raw content omitted to keep this request bounded", tokens))
	b.WriteString(fmt.Sprintf(" (call %s, sent %d times)", toolCallID, timesSent))
	b.WriteString(". Re-issue the same tool call if you need this content again.")
	return b.String()
}

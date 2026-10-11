// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chunk

// Budget is the single place the context budget is derived from. It exists so
// no caller carries a magic number: the chunk ceiling and the per-read ceiling
// both come from the same arithmetic, and the provider request ceiling that
// feeds it is named rather than inlined at each use site.
//
// chunk_budget = request_limit - fixed_overhead - conversation_reserve - output_reserve
//
// Every term is an estimate in the same token currency as the rest of the
// codebase's accounting (cl100k_base). The result is deliberately
// conservative: it under-fills rather than risking a request the provider would
// refuse.
type Budget struct {
	// RequestLimit is the safe input ceiling for a single request.
	RequestLimit int
	// FixedOverhead is the measured cost of everything the request carries
	// besides the review context: system prompt, tool definitions, checklist,
	// plan. Measured by rendering the prompt with an empty context rather than
	// assumed, so a template edit moves the number.
	FixedOverhead int
	// ConversationReserve is the headroom kept for the conversation the
	// context is read inside: prior rounds, findings, receipts.
	ConversationReserve int
	// OutputReserve is the headroom kept for the model's own reply.
	OutputReserve int
}

// conversationReserveFraction and outputReserveFraction bound the two reserves
// relative to the request limit. They are fractions rather than constants
// because the same run may be configured for a 32k provider ceiling or a 200k
// one, and a fixed reserve tuned for one is wrong for the other.
const (
	conversationReserveFraction = 0.30
	outputReserveFraction       = 0.20
)

// DeriveBudget builds a Budget from the request limit, the measured fixed
// overhead and the configured completion ceiling. maxCompletionTokens is
// capped by outputReserveFraction of the request limit: a completion ceiling
// larger than the request limit can afford must not be allowed to erase the
// context budget entirely.
func DeriveBudget(requestLimit, fixedOverhead, maxCompletionTokens int) Budget {
	if requestLimit <= 0 {
		requestLimit = minUsableBudget
	}
	if fixedOverhead < 0 {
		fixedOverhead = 0
	}
	b := Budget{
		RequestLimit:        requestLimit,
		FixedOverhead:       fixedOverhead,
		ConversationReserve: int(float64(requestLimit) * conversationReserveFraction),
		OutputReserve:       min(int(float64(requestLimit)*outputReserveFraction), maxCompletionTokens),
	}
	if b.OutputReserve < 0 {
		b.OutputReserve = 0
	}
	return b
}

// Chunk returns the maximum token count of a single chunk. It returns 0 when
// the request limit leaves no usable room: a caller must treat that as "the
// context cannot be sharded into anything reviewable" rather than as a licence
// to send unbounded content.
func (b Budget) Chunk() int {
	n := b.RequestLimit - b.FixedOverhead - b.ConversationReserve - b.OutputReserve
	if n < minUsableBudget {
		return 0
	}
	return n
}

// Read returns the maximum token count of a single tool result serving review
// context. It is the chunk ceiling: one chunk is by construction within budget,
// so returning one chunk is within budget too, and a multi-chunk read stops at
// the same line. A zero value means no context may be served at all.
func (b Budget) Read() int { return b.Chunk() }

// Manifest returns the token ceiling for the chunk index rendered into the
// first user message. The index is read once and then lives in the frozen
// prefix of the conversation for the whole run, so it gets a quarter of the
// chunk budget: enough to name a large change, small enough that the frozen
// prefix never becomes the thing that pushes a request over its limit.
func (b Budget) Manifest() int {
	n := b.Chunk() / 4
	if n < minUsableBudget {
		return minUsableBudget
	}
	return n
}

// Usable reports whether the budget leaves room for at least one chunk.
func (b Budget) Usable() bool { return b.Chunk() > 0 }

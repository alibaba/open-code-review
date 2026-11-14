// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/stdout"
)

// Compression thresholds, as fractions of MaxTokens.
const (
	tokenSoftThreshold    = 0.60 // async background compression
	tokenWarningThreshold = 0.80 // immediate sync compression
)

// PromptTokenLimit returns tokenWarningThreshold (80%) of maxTokens. It is
// shared by the agent and scan pre-flight gates, their large-input filters, and
// computeActiveZoneSize so the threshold has a single definition. Non-positive
// input is not special-cased — each caller decides what that means.
func PromptTokenLimit(maxTokens int) int {
	return int(float64(maxTokens) * tokenWarningThreshold)
}

// round groups consecutive messages starting with an assistant message
// followed by zero or more tool result messages.
type round struct {
	assistantIdx int
	toolIdxs     []int
}

// partitionResult describes how messages should be split for compression.
type partitionResult struct {
	frozenEnd   int
	compressEnd int
	rounds      []round
	activeCount int
}

// compressionJob tracks an in-flight background compression operation.
type compressionJob struct {
	done        chan struct{}
	rebuilt     []llm.Message
	cancel      context.CancelFunc
	snapshotLen int // message count when the snapshot was taken
}

// compressionState is the async-compression bookkeeping for a single
// conversation (one RunMainTask call). The Runner is shared by concurrent
// per-subtask goroutines, so this state must not live on the Runner: a shared
// slot lets one subtask apply, cancel, or replace another subtask's
// compression job (#384).
type compressionState struct {
	mu         sync.Mutex
	pendingJob *compressionJob
}

// messageTokens counts visible text plus the Native replay payload and
// tool calls that ExtractText() does not see.
func messageTokens(m llm.Message) int {
	return llm.CountTokens(m.ExtractText()) + m.EstimatedTokens()
}

// CountMessagesTokens returns the rough token count of msgs by summing the
// per-message token count. Exported because both review and scan top
// layers may want it for pre-flight checks.
func CountMessagesTokens(msgs []llm.Message) int {
	var total int
	for _, m := range msgs {
		total += messageTokens(m)
	}
	return total
}

// groupIntoRounds parses messages[start:] into logical
// (assistant + tool_results) pairs.
func groupIntoRounds(messages []llm.Message, start int) []round {
	var rounds []round
	i := start
	for i < len(messages) {
		if messages[i].Role == "assistant" {
			r := round{assistantIdx: i}
			i++
			for i < len(messages) && messages[i].Role == "tool" {
				r.toolIdxs = append(r.toolIdxs, i)
				i++
			}
			rounds = append(rounds, r)
		} else {
			i++
		}
	}
	return rounds
}

// computeActiveZoneSize returns how many trailing rounds fit within the
// remaining token budget after accounting for the frozen zone and the
// compressed summary.
func computeActiveZoneSize(rounds []round, messages []llm.Message, maxTokens int, reservedTokens int) int {
	budget := PromptTokenLimit(maxTokens) - reservedTokens
	if budget <= 0 {
		return 0
	}

	count := 0
	tokensUsed := 0
	for i := len(rounds) - 1; i >= 0; i-- {
		roundTokens := messageTokens(messages[rounds[i].assistantIdx])
		for _, ti := range rounds[i].toolIdxs {
			roundTokens += messageTokens(messages[ti])
		}
		if tokensUsed+roundTokens > budget {
			break
		}
		tokensUsed += roundTokens
		count++
	}
	return count
}

// partitionMessages divides messages into frozen, compress, and active zones.
// Frozen zone is always messages[0:2]. Active zone preserves the K most
// recent complete rounds based on available token budget.
func partitionMessages(messages []llm.Message, maxTokens int, prevSummaryTokenEstimate int) partitionResult {
	result := partitionResult{frozenEnd: 2}
	if len(messages) <= 2 {
		result.compressEnd = len(messages)
		return result
	}

	result.rounds = groupIntoRounds(messages, 2)
	if len(result.rounds) == 0 {
		result.compressEnd = len(messages)
		return result
	}

	result.activeCount = computeActiveZoneSize(result.rounds, messages, maxTokens, prevSummaryTokenEstimate)
	if result.activeCount >= len(result.rounds) {
		// Everything fits — no compression needed.
		result.compressEnd = len(messages)
		result.activeCount = 0
		return result
	}

	// compressEnd = index after the last round NOT in active zone.
	activeStartIdx := len(result.rounds) - result.activeCount
	lastCompressRound := result.rounds[activeStartIdx-1]
	if len(lastCompressRound.toolIdxs) > 0 {
		result.compressEnd = lastCompressRound.toolIdxs[len(lastCompressRound.toolIdxs)-1] + 1
	} else {
		result.compressEnd = lastCompressRound.assistantIdx + 1
	}

	return result
}

// StripMarkdownFences removes ```json and ``` wrappers some models add
// around structured outputs. Exposed so callers (e.g. agent's review-filter
// post-step) that parse LLM JSON output can reuse the same heuristic.
func StripMarkdownFences(s string) string { return stripMarkdownFences(s) }

// stripMarkdownFences is the package-private workhorse used by the
// internal compression code paths.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			s = s[nl+1:]
		} else {
			s = strings.TrimPrefix(s, "```json")
			s = strings.TrimPrefix(s, "```")
		}
	}
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	return s
}

// buildMessageXML serializes msgs into the <message><content> form expected
// by the MEMORY_COMPRESSION_TASK prompt template.
func buildMessageXML(msgs []llm.Message) string {
	var sb strings.Builder
	for i, m := range msgs {
		sb.WriteString(fmt.Sprintf("<message id=\"%d\" role=\"%s\">\n", i, m.Role))
		sb.WriteString("    <content>\n")
		sb.WriteString(fmt.Sprintf("      %s\n", m.ExtractText()))
		sb.WriteString("    </content>\n")
		if m.ReasoningContent != "" {
			sb.WriteString("    <reasoning>\n")
			sb.WriteString(fmt.Sprintf("      %s\n", m.ReasoningContent))
			sb.WriteString("    </reasoning>\n")
		}
		sb.WriteString("</message>")
		if i < len(msgs)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// errCompressionInputTooLarge reports that the compression request could not
// be brought under its input budget, so it was not sent.
var errCompressionInputTooLarge = errors.New("compression input exceeds budget")

const truncationSuffix = "... (truncated)"

// truncateToByteBudget shortens text to at most maxBytes, backing off to a
// rune boundary so the result stays valid UTF-8. The explicit suffix lets the
// summarizer see that content was elided rather than assume it saw everything.
func truncateToByteBudget(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	keep := maxBytes - len(truncationSuffix)
	if keep <= 0 {
		return truncationSuffix
	}
	for keep > 0 && !utf8.RuneStart(text[keep]) {
		keep--
	}
	return text[:keep] + truncationSuffix
}

// renderedSize is the byte length of the compression request built from
// templates once {{context}} expands to contextXML. Measuring the rendered
// request rather than ExtractText() accounts for reasoning, XML markup and the
// template's own text.
func renderedSize(templates []MessageTemplate, contextXML string) int {
	size := 0
	for _, m := range templates {
		size += len(strings.ReplaceAll(m.Content, "{{context}}", contextXML))
	}
	return size
}

// fitCompressionInput returns a copy of msgs whose rendered compression request
// stays within budgetBytes. It repeatedly shortens the largest content or
// reasoning field, so small messages keep their full text, and copies each
// message so ToolCalls, Native and the untouched field survive. Byte length is
// used instead of a token count because tokenizing multi-hundred-KB single-line
// tool results is slow, and the bound only needs a cheap, deterministic ceiling.
// If markup and template text alone exceed the budget, it returns
// errCompressionInputTooLarge so the caller can skip the request.
func fitCompressionInput(msgs []llm.Message, templates []MessageTemplate, budgetBytes int) ([]llm.Message, error) {
	if budgetBytes <= 0 {
		return msgs, nil
	}
	excess := renderedSize(templates, buildMessageXML(msgs)) - budgetBytes
	if excess <= 0 {
		return msgs, nil
	}

	out := copyMessages(msgs)
	for excess > 0 {
		idx, reasoning, largest := -1, false, 0
		for i := range out {
			if n := len(out[i].ExtractText()); n > largest {
				idx, reasoning, largest = i, false, n
			}
			if n := len(out[i].ReasoningContent); n > largest {
				idx, reasoning, largest = i, true, n
			}
		}
		if largest <= len(truncationSuffix) {
			return nil, fmt.Errorf("%w: %d bytes over a %d-byte budget",
				errCompressionInputTooLarge, excess, budgetBytes)
		}

		target := max(0, largest-excess)
		if reasoning {
			out[idx].ReasoningContent = truncateToByteBudget(out[idx].ReasoningContent, target)
		} else {
			out[idx].Content = truncateToByteBudget(out[idx].ExtractText(), target)
		}
		excess = renderedSize(templates, buildMessageXML(out)) - budgetBytes
	}
	return out, nil
}

// copyMessages creates a shallow copy of a message slice.
func copyMessages(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	return out
}

// runCompression performs three-zone memory compression on the given
// messages, summarizing the compress zone while preserving the active zone
// intact. Returns rebuilt as [frozen] + [compressed_summary appended to
// the user prompt] + [active].
func (r *Runner) runCompression(ctx context.Context, msgs []llm.Message, taskKey string) ([]llm.Message, error) {
	if len(r.task.Compression.Messages) == 0 || len(msgs) <= 2 {
		return msgs[:min(len(msgs), 2)], nil
	}

	part := partitionMessages(msgs, r.task.MaxContextTokens, 0)
	if part.compressEnd <= part.frozenEnd {
		return msgs, nil
	}

	// MaxContextTokens × ~4 bytes/token: the request that summarizes the history
	// must itself fit the context window it is meant to relieve.
	compressZone, err := fitCompressionInput(msgs[part.frozenEnd:part.compressEnd],
		r.task.Compression.Messages, r.task.MaxContextTokens*4)
	if err != nil {
		return msgs, fmt.Errorf("memory compression: %w", err)
	}
	contextXML := buildMessageXML(compressZone)

	compressionMsgs := make([]llm.Message, 0, len(r.task.Compression.Messages))
	for _, m := range r.task.Compression.Messages {
		content := strings.ReplaceAll(m.Content, "{{context}}", contextXML)
		compressionMsgs = append(compressionMsgs, llm.NewTextMessage(m.Role, content))
	}

	// The task record is created before the request, not after it, because the
	// retry report keys request identity on RequestNo and that number only
	// exists once the record does. The visible consequence is that the
	// llm_request line reaches the session JSONL before the response: a run
	// killed mid-request now leaves an llm_request with no response, which
	// resume ignores (applyResumeLine has no case for it).
	fs := r.deps.Session.GetOrCreateFileSession(taskKey)
	rec := fs.AppendTaskRecord(session.MemoryCompressionTask, compressionMsgs)

	ctx = llm.ContextWithSessionKey(ctx,
		llm.SessionTaskKey(r.deps.Session.SessionID, string(session.MemoryCompressionTask), taskKey))

	startTime := time.Now()
	reqCtx := r.RequestContext(ctx, taskKey, session.MemoryCompressionTask, rec.RequestNo)
	resp, err := r.deps.LLMClient.CompletionsWithCtx(reqCtx, llm.ChatRequest{
		Model:     r.deps.Model,
		Messages:  compressionMsgs,
		MaxTokens: r.task.MaxCompletionTokens,
	})
	duration := time.Since(startTime)

	if err != nil {
		rec.SetError(err, duration)
		// Return msgs unchanged: truncating to frozenEnd would discard all
		// conversation context, which is worse than staying over the token
		// limit temporarily.
		return msgs, fmt.Errorf("memory compression: %w", err)
	}
	rec.SetResponse(resp, duration)
	r.RecordUsage(resp.Usage)

	rawSummary := stripMarkdownFences(resp.Content())
	if rawSummary == "" {
		// Empty summary: keep the original conversation rather than dropping
		// everything below the frozen zone.
		return msgs, nil
	}

	rebuilt := make([]llm.Message, 2)
	copy(rebuilt, msgs[:2])

	userMsg := rebuilt[1]
	currentText := userMsg.ExtractText()
	rebuilt[1] = llm.NewTextMessage(userMsg.Role, currentText+strings.ReplaceAll(r.task.Compression.SummaryTemplate, "{{summary}}", rawSummary))

	for i := part.compressEnd; i < len(msgs); i++ {
		rebuilt = append(rebuilt, msgs[i])
	}

	return rebuilt, nil
}

// triggerAsyncCompression kicks off a background compression job for the
// conversation owning st. A no-op when a job is already pending — the
// check-and-set happens under st.mu so concurrent callers cannot replace
// (and thereby leak) an in-flight job.
func (r *Runner) triggerAsyncCompression(ctx context.Context, st *compressionState, messages []llm.Message, taskKey string) {
	st.mu.Lock()
	if st.pendingJob != nil {
		st.mu.Unlock()
		return
	}
	msgSnapshot := copyMessages(messages)
	asyncCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	job := &compressionJob{done: make(chan struct{}), cancel: cancel, snapshotLen: len(messages)}
	st.pendingJob = job
	st.mu.Unlock()

	// Registered before the goroutine starts so WaitBackground can never miss
	// a job that was launched but has not run yet.
	r.bg.Add(1)
	go func() {
		defer r.bg.Done()
		defer cancel()
		rebuilt, err := r.runCompression(asyncCtx, msgSnapshot, taskKey)

		st.mu.Lock()
		defer st.mu.Unlock()

		if st.pendingJob != job {
			return // cancelled or superseded
		}
		if err != nil {
			// Still the owner, so this is a genuine failure rather than a
			// deliberate cancel (cancelPendingCompression cancels and clears
			// pendingJob under the lock, so cancelled jobs fail the ownership
			// check above and die silently). Abandon the job rather than
			// applying a truncated/unmodified snapshot over live messages.
			fmt.Fprintf(stdout.Writer(), "[ocr] Memory compression failed: %v\n", err)
			st.pendingJob = nil
			close(job.done)
			return
		}
		job.rebuilt = rebuilt
		close(job.done)
	}()
}

// tryApplyPendingCompression checks whether a background compression has
// completed and swaps the rebuilt messages into place. Returns true if
// applied.
func (r *Runner) tryApplyPendingCompression(st *compressionState, messages *[]llm.Message) bool {
	st.mu.Lock()
	job := st.pendingJob
	st.mu.Unlock()

	if job == nil {
		return false
	}

	select {
	case <-job.done:
		applied := false
		st.mu.Lock()
		if st.pendingJob == job && job.rebuilt != nil {
			rebuilt := job.rebuilt
			// Preserve any messages appended after the snapshot was taken —
			// the background job only compressed messages[:snapshotLen].
			if job.snapshotLen < len(*messages) {
				rebuilt = append(rebuilt, (*messages)[job.snapshotLen:]...)
			}
			*messages = rebuilt
			applied = true
		}
		if st.pendingJob == job {
			st.pendingJob = nil
		}
		st.mu.Unlock()
		return applied
	default:
		return false
	}
}

// cancelPendingCompression aborts the conversation's in-flight background
// compression, if any.
func (r *Runner) cancelPendingCompression(st *compressionState) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.pendingJob != nil {
		st.pendingJob.cancel()
		st.pendingJob = nil
	}
}

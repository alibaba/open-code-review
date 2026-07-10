package llmloop

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/session"
	"github.com/open-code-review/open-code-review/internal/stdout"
)

// Compression thresholds, as fractions of MaxTokens.
const (
	tokenSoftThreshold    = 0.60 // async background compression
	tokenWarningThreshold = 0.80 // immediate sync compression
)

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

// CountMessagesTokens returns the rough token count of msgs by summing the
// per-message text token count. Exported because both review and scan top
// layers may want it for pre-flight checks.
func CountMessagesTokens(msgs []llm.Message) int {
	var total int
	for _, m := range msgs {
		total += llm.CountTokens(m.ExtractText())
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
	budget := int(float64(maxTokens)*tokenWarningThreshold) - reservedTokens
	if budget <= 0 {
		return 0
	}

	count := 0
	tokensUsed := 0
	for i := len(rounds) - 1; i >= 0; i-- {
		roundTokens := llm.CountTokens(messages[rounds[i].assistantIdx].ExtractText())
		for _, ti := range rounds[i].toolIdxs {
			roundTokens += llm.CountTokens(messages[ti].ExtractText())
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
		sb.WriteString("</message>")
		if i < len(msgs)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
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
func (r *Runner) runCompression(ctx context.Context, msgs []llm.Message, filePath string) ([]llm.Message, error) {
	if len(r.deps.Template.MemoryCompressionTask.Messages) == 0 || len(msgs) <= 2 {
		return msgs[:min(len(msgs), 2)], nil
	}

	part := partitionMessages(msgs, r.deps.Template.MaxTokens, 0)
	if part.compressEnd <= part.frozenEnd {
		return msgs, nil
	}

	contextXML := buildMessageXML(msgs[part.frozenEnd:part.compressEnd])

	compressionMsgs := make([]llm.Message, 0, len(r.deps.Template.MemoryCompressionTask.Messages))
	for _, m := range r.deps.Template.MemoryCompressionTask.Messages {
		content := strings.ReplaceAll(m.Content, "{{context}}", contextXML)
		compressionMsgs = append(compressionMsgs, llm.NewTextMessage(m.Role, content))
	}

	startTime := time.Now()
	resp, err := r.deps.LLMClient.CompletionsWithCtx(ctx, llm.ChatRequest{
		Model:     r.deps.Model,
		Messages:  compressionMsgs,
		MaxTokens: r.deps.Template.MaxTokens,
	})
	duration := time.Since(startTime)

	fs := r.deps.Session.GetOrCreateFileSession(filePath)
	rec := fs.AppendTaskRecord(session.MemoryCompressionTask, compressionMsgs)
	if err != nil {
		rec.SetError(err, duration)
		fmt.Fprintf(stdout.Writer(), "[ocr] Memory compression failed: %v\n", err)
		// Return msgs unchanged: truncating to frozenEnd would discard all
		// conversation context, which is worse than staying over the token
		// limit temporarily.
		return msgs, fmt.Errorf("memory compression: %w", err)
	}
	rec.SetResponse(resp, duration)
	if resp.Usage != nil {
		atomic.AddInt64(&r.totalInputTokens, resp.Usage.PromptTokens)
		atomic.AddInt64(&r.totalOutputTokens, resp.Usage.CompletionTokens)
		atomic.AddInt64(&r.totalCacheReadTokens, resp.Usage.CacheReadTokens)
		atomic.AddInt64(&r.totalCacheWriteTokens, resp.Usage.CacheWriteTokens)
	}

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
	rebuilt[1] = llm.NewTextMessage(userMsg.Role, currentText+"\n\n<previous_review_summary>\n"+rawSummary+"\n</previous_review_summary>")

	for i := part.compressEnd; i < len(msgs); i++ {
		rebuilt = append(rebuilt, msgs[i])
	}

	return rebuilt, nil
}

// triggerAsyncCompression kicks off a background compression job.
func (r *Runner) triggerAsyncCompression(ctx context.Context, messages []llm.Message, filePath string) {
	msgSnapshot := copyMessages(messages)

	asyncCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)

	job := &compressionJob{done: make(chan struct{}), cancel: cancel, snapshotLen: len(messages)}
	r.compressionMu.Lock()
	r.pendingJob = job
	r.compressionMu.Unlock()

	go func() {
		defer cancel()
		rebuilt, err := r.runCompression(asyncCtx, msgSnapshot, filePath)

		r.compressionMu.Lock()
		defer r.compressionMu.Unlock()

		if r.pendingJob != job {
			return // cancelled or superseded
		}
		if err != nil {
			// Compression failed — abandon the job rather than applying a
			// truncated/unmodified snapshot over live messages.
			r.pendingJob = nil
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
func (r *Runner) tryApplyPendingCompression(messages *[]llm.Message) bool {
	r.compressionMu.Lock()
	job := r.pendingJob
	r.compressionMu.Unlock()

	if job == nil {
		return false
	}

	select {
	case <-job.done:
		applied := false
		r.compressionMu.Lock()
		if r.pendingJob == job && job.rebuilt != nil {
			rebuilt := job.rebuilt
			// Preserve any messages appended after the snapshot was taken —
			// the background job only compressed messages[:snapshotLen].
			if job.snapshotLen < len(*messages) {
				rebuilt = append(rebuilt, (*messages)[job.snapshotLen:]...)
			}
			*messages = rebuilt
			applied = true
		}
		if r.pendingJob == job {
			r.pendingJob = nil
		}
		r.compressionMu.Unlock()
		return applied
	default:
		return false
	}
}

// cancelPendingCompression aborts any in-flight background compression.
func (r *Runner) cancelPendingCompression() {
	r.compressionMu.Lock()
	defer r.compressionMu.Unlock()

	if r.pendingJob != nil {
		r.pendingJob.cancel()
		r.pendingJob = nil
	}
}

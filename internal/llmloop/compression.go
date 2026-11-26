// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	markdowntext "github.com/yuin/goldmark/text"
)

// Compression thresholds, as fractions of MaxTokens.
const (
	tokenSoftThreshold    = 0.60 // async background compression
	tokenWarningThreshold = 0.80 // immediate sync compression
)

// PromptTokenLimit returns tokenWarningThreshold (80%) of maxTokens. It is
// shared by the agent and scan pre-flight gates, their large-input filters, and
// partitionMessages so the threshold has a single definition. Non-positive
// input is not special-cased — each caller decides what that means.
func PromptTokenLimit(maxTokens int) int {
	return int(float64(maxTokens) * tokenWarningThreshold)
}

// partitionResult describes how messages should be split for compression.
type partitionResult struct {
	frozenEnd   int
	compressEnd int
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

// partitionMessages divides messages into frozen, compress, and active zones.
// Frozen zone is always messages[0:2]. Active zone preserves the K most
// recent complete rounds based on available token budget.
func partitionMessages(messages []llm.Message, maxTokens int, summaryTokenReserve int) partitionResult {
	result := partitionResult{frozenEnd: min(2, len(messages)), compressEnd: len(messages)}
	remaining := PromptTokenLimit(maxTokens) - CountMessagesTokens(messages[:result.frozenEnd]) - summaryTokenReserve
	for index := len(messages) - 1; index >= result.frozenEnd; index-- {
		remaining -= messageTokens(messages[index])
		if remaining <= 0 {
			break
		}
		if messages[index].Role == "assistant" {
			result.activeCount++
			result.compressEnd = index
		} else if index == result.frozenEnd {
			result.compressEnd = result.frozenEnd
		}
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
	if before, ok := strings.CutSuffix(s, "```"); ok {
		s = strings.TrimSpace(before)
	}
	return s
}

var summaryMarkers = regexp.MustCompile("</?think>")
var summaryMarkdown = goldmark.DefaultParser()

func summaryControlText(source string) []byte {
	controls := []byte(source)
	markdown := []byte(strings.ReplaceAll(source, "<", "x"))
	doc := summaryMarkdown.Parse(markdowntext.NewReader(markdown))
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch node := node.(type) {
			case *ast.Text:
				if node.Parent().Kind() == ast.KindCodeSpan {
					clear(controls[node.Segment.Start:node.Segment.Stop])
				}
			case *ast.FencedCodeBlock, *ast.CodeBlock:
				lines := node.Lines()
				for index := 0; index < lines.Len(); index++ {
					line := lines.At(index)
					clear(controls[line.Start:line.Stop])
				}
			}
		}
		return ast.WalkContinue, nil
	})
	return controls
}

func unwrapSummaryFence(text string) string {
	trimmed := strings.TrimSpace(text)
	backticks := strings.HasPrefix(trimmed, "```")
	if !backticks && !strings.HasPrefix(trimmed, "~~~") {
		return trimmed
	}
	source := []byte(text)
	doc := summaryMarkdown.Parse(markdowntext.NewReader(source))
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		if node.Kind() != ast.KindFencedCodeBlock {
			return trimmed
		}
		lines := node.Lines()
		content := bytes.TrimSpace(lines.Value(source))
		if len(content) == 0 {
			continue
		}
		if backticks && node == doc.FirstChild() && node.NextSibling() == nil {
			last := lines.At(lines.Len() - 1)
			if len(bytes.TrimSpace(source[last.Stop:])) != 0 {
				header, _, _ := strings.Cut(trimmed, "\n")
				switch strings.TrimSpace(strings.TrimLeft(header, "`")) {
				case "", "text", "plaintext", "markdown", "md":
					return string(content)
				}
			}
		}
		return trimmed
	}
	return ""
}

func compressionSummary(text string) string {
	text = unwrapSummaryFence(text)
	if !strings.Contains(text, "<think>") && !strings.Contains(text, "</think>") {
		return text
	}
	var visible strings.Builder
	var start, depth int
	for _, match := range summaryMarkers.FindAllIndex(summaryControlText(text), -1) {
		prefix := match[0]
		for prefix > 0 && text[prefix-1] == '\\' {
			prefix--
		}
		if (match[0]-prefix)%2 != 0 {
			continue
		}
		if text[match[0]:match[1]] == "<think>" {
			if depth == 0 {
				visible.WriteString(text[start:match[0]])
			}
			depth++
		} else {
			if depth == 0 {
				if start != 0 || !strings.HasPrefix(strings.TrimLeft(text[match[1]:], " \t\r"), "\n") {
					return ""
				}
			} else {
				depth--
			}
			start = match[1]
		}
	}
	if depth != 0 {
		return ""
	}
	visible.WriteString(text[start:])
	return unwrapSummaryFence(visible.String())
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

// contextOccurrences counts how many times a template list expands {{context}}.
// It is used to translate a byte reduction in a single message field into the
// corresponding reduction of the fully rendered request without re-rendering.
func contextOccurrences(templates []MessageTemplate) int {
	n := 0
	for _, m := range templates {
		n += strings.Count(m.Content, "{{context}}")
	}
	return n
}

// fitCompressionInput returns a copy of msgs whose rendered compression request
// stays within budgetBytes. It repeatedly shortens the largest content or
// reasoning field, so small messages keep their full text, and copies each
// message so ToolCalls, Native and the untouched field survive. Byte length is
// used instead of a token count because tokenizing multi-hundred-KB single-line
// tool results is slow, and the bound only needs a cheap, deterministic ceiling.
// If markup and template text alone exceed the budget, it returns
// errCompressionInputTooLarge so the caller can skip the request.
//
// The rendered size is measured once and then updated incrementally: a cut that
// removes d bytes from a field shrinks the rendered request by d * ctxCount,
// where ctxCount is the number of {{context}} expansions. This avoids re-rendering
// every message and template on each pass (O(n^2 * L) worst case), so a round
// with many oversized tool results stays linear in the total field bytes.
func fitCompressionInput(msgs []llm.Message, templates []MessageTemplate, budgetBytes int) ([]llm.Message, error) {
	if budgetBytes <= 0 {
		return msgs, nil
	}
	ctxCount := contextOccurrences(templates)
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

		// Each byte removed from a field shrinks the rendered request by ctxCount
		// bytes (every {{context}} expansion shrinks), so the field-level cut must
		// be sized against excess/ctxCount, not the raw excess. Using the raw
		// excess over-truncates whenever the template references {{context}} more
		// than once, silently discarding preservable history.
		target := max(0, largest-(excess+ctxCount-1)/ctxCount)
		if reasoning {
			before := len(out[idx].ReasoningContent)
			out[idx].ReasoningContent = truncateToByteBudget(out[idx].ReasoningContent, target)
			excess -= (before - len(out[idx].ReasoningContent)) * ctxCount
		} else {
			before := len(out[idx].ExtractText())
			out[idx].Content = truncateToByteBudget(out[idx].ExtractText(), target)
			excess -= (before - len(out[idx].ExtractText())) * ctxCount
		}
	}
	return out, nil
}

// smallestCompressionInput returns a copy of msgs with every content and
// reasoning field longer than the truncation marker replaced by the marker:
// the shortest input fitCompressionInput can produce.
func smallestCompressionInput(msgs []llm.Message) []llm.Message {
	out := copyMessages(msgs)
	for i := range out {
		if len(out[i].ExtractText()) > len(truncationSuffix) {
			out[i].Content = truncationSuffix
		}
		if len(out[i].ReasoningContent) > len(truncationSuffix) {
			out[i].ReasoningContent = truncationSuffix
		}
	}
	return out
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
		return msgs, nil
	}

	limit := PromptTokenLimit(r.task.MaxContextTokens)
	summaryOccurrences := strings.Count(r.task.Compression.SummaryTemplate, "{{summary}}")
	if summaryOccurrences == 0 {
		return msgs, fmt.Errorf("memory compression: summary template has no summary placeholder")
	}
	frameTokens := llm.CountTokens(strings.ReplaceAll(r.task.Compression.SummaryTemplate, "{{summary}}", ""))
	availableTokens := limit - CountMessagesTokens(msgs[:2]) - frameTokens - 1
	summaryLimit := min(r.task.MaxContextTokens-limit, availableTokens/summaryOccurrences)
	if r.task.MaxCompletionTokens > 0 {
		summaryLimit = min(summaryLimit, r.task.MaxCompletionTokens)
	}
	if summaryLimit <= 0 {
		return msgs, fmt.Errorf("memory compression: fixed prompt leaves no room for a summary")
	}
	part := partitionMessages(msgs, r.task.MaxContextTokens, frameTokens+summaryLimit*summaryOccurrences)
	if part.compressEnd <= part.frozenEnd {
		return msgs, nil
	}
	originalTokens := CountMessagesTokens(msgs)
	if part.activeCount == 0 && originalTokens < limit {
		return msgs, nil
	}

	// MaxContextTokens × ~4 bytes/token bounds the request that summarizes the
	// history, so the summarization request itself fits the window it relieves.
	// The byte budget is a cheap ceiling; the actual token count is validated
	// below so dense token streams (digits, CJK, code at 2.4–3.9 bytes/token)
	// cannot still overflow the model's limit.
	//
	// The byte budget is only a heuristic, not proof that a request fits: CJK
	// and other dense content can tokenize well past 4 bytes/token, so a
	// request under the byte ceiling can still exceed MaxContextTokens. Rather
	// than rejecting it outright, scale the byte budget down by the observed
	// token density and refit. The scale factor is < 1 whenever the token
	// count is over budget, so the budget strictly shrinks each pass and
	// fitCompressionInput eventually either fits or reports the markup/template
	// alone is too large — there is no unbounded loop.
	//
	// The scaled budget is itself an estimate: template text and XML markup
	// cannot shrink, so scaling the whole rendered size can undershoot what
	// truncation is able to reach. The budget is therefore never lowered below
	// the size of the smallest marked request; only when that smallest request
	// is still over the token limit is compression skipped.
	zone := msgs[part.frozenEnd:part.compressEnd]
	minBudget := renderedSize(r.task.Compression.Messages, buildMessageXML(smallestCompressionInput(zone)))
	var (
		byteBudget      = r.task.MaxContextTokens * 4
		compressZone    []llm.Message
		compressionMsgs []llm.Message
		contextXML      string
		requestTokens   int
	)
	for {
		var err error
		compressZone, err = fitCompressionInput(zone, r.task.Compression.Messages, byteBudget)
		if err != nil {
			return msgs, fmt.Errorf("memory compression: %w", err)
		}
		contextXML = buildMessageXML(compressZone)

		compressionMsgs = make([]llm.Message, 0, len(r.task.Compression.Messages))
		for _, m := range r.task.Compression.Messages {
			content := strings.ReplaceAll(m.Content, "{{context}}", contextXML)
			compressionMsgs = append(compressionMsgs, llm.NewTextMessage(m.Role, content))
		}

		requestTokens = CountMessagesTokens(compressionMsgs)
		if requestTokens <= r.task.MaxContextTokens {
			break
		}
		if byteBudget <= minBudget {
			return msgs, fmt.Errorf("memory compression: %w: %d tokens over a %d-token window",
				errCompressionInputTooLarge, requestTokens, r.task.MaxContextTokens)
		}
		// Shrink the byte budget in proportion to the observed token density so
		// the next fit targets a request that actually stays under the window.
		// requestTokens > MaxContextTokens guarantees the factor is < 1. The
		// product is computed in 64 bits so it cannot overflow on 32-bit builds.
		scaled := int(int64(byteBudget) * int64(r.task.MaxContextTokens) / int64(requestTokens))
		byteBudget = max(scaled, minBudget)
	}
	generationLimit := summaryLimit
	if r.task.MaxCompletionTokens > summaryLimit {
		for index := len(compressionMsgs) - 1; index >= 0; index-- {
			original := compressionMsgs[index]
			if original.Role != "user" && original.Role != "system" {
				continue
			}
			instruction := fmt.Sprintf("\n\nKeep the visible summary within %d tokens.", summaryLimit)
			compressionMsgs[index] = llm.NewTextMessage(original.Role, original.ExtractText()+instruction)
			if expanded := min(r.task.MaxCompletionTokens, limit-CountMessagesTokens(compressionMsgs)); expanded > summaryLimit {
				generationLimit = expanded
			} else {
				compressionMsgs[index] = original
			}
			break
		}
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
		MaxTokens: generationLimit,
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

	if len(resp.Choices) > 0 {
		switch reason := resp.Choices[0].FinishReason; reason {
		case "", "stop", "end_turn", "stop_sequence":
		default:
			return msgs, fmt.Errorf("memory compression: unfinished summary (finish_reason=%q)", reason)
		}
	}
	rawSummary := compressionSummary(resp.RawOutputText())
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
	rebuiltTokens := CountMessagesTokens(rebuilt)
	if rebuiltTokens >= limit {
		return msgs, fmt.Errorf("memory compression: rebuilt context exceeds its threshold")
	}
	if rebuiltTokens >= originalTokens {
		return msgs, nil
	}
	if llm.CountTokens(rawSummary) > summaryLimit {
		return msgs, fmt.Errorf("memory compression: visible summary exceeds its reservation")
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
	r.bg.Go(func() {
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
			fmt.Fprintf(stdout.Writer(), "[ocr] Memory compression failed: %v\n", r.progressError(err))
			st.pendingJob = nil
			close(job.done)
			return
		}
		job.rebuilt = rebuilt
		close(job.done)
	})
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

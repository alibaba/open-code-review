// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

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

// copyMessages creates a shallow copy of a message slice.
func copyMessages(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	return out
}

// maxCompressionContextTokens bounds how much conversation content a single
// memory-compression call may attempt to summarize. The compress zone's size
// is otherwise unbounded: it is whatever accumulated between the frozen and
// active zones, which can include many rounds of tool-call output (e.g. broad
// code_search/file_read results). Without this cap, a single compression
// request can end up needing far more tokens than the model's real context
// window, so the LLM call itself fails and the whole review aborts (see
// alibaba/open-code-review#838 — three-zone compression can let live/compress
// content overfill past its intended threshold). This is independent of the
// per-file MaxTokens template setting: it is a hard backstop sized well under
// real-world model context windows (even the smallest current-generation
// models offer >= 128K tokens), so it only engages in pathological cases.
const maxCompressionContextTokens = 200_000

// boundCompressZoneStart returns the start index of the largest trailing
// slice of msgs[frozenEnd:compressEnd] whose combined token count fits within
// limit. The compress zone summarizes its oldest content first (closest to
// the frozen zone) and its newest content last (closest to the active zone
// it hands off to), so trimming from the old end discards the least
// contextually relevant material when a single call cannot fit everything.
// Always includes at least the single newest message in the zone, even if
// that message alone exceeds limit, so the caller never ends up with an
// empty range to summarize.
func boundCompressZoneStart(msgs []llm.Message, frozenEnd, compressEnd, limit int) int {
	tokens := 0
	i := compressEnd
	for i > frozenEnd {
		t := messageTokens(msgs[i-1])
		if i < compressEnd && tokens+t > limit {
			break
		}
		tokens += t
		i--
	}
	return i
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

	compressStart := boundCompressZoneStart(msgs, part.frozenEnd, part.compressEnd, maxCompressionContextTokens)
	contextXML := buildMessageXML(msgs[compressStart:part.compressEnd])
	if compressStart > part.frozenEnd {
		contextXML = fmt.Sprintf(
			"<truncation_notice>%d earlier message(s) omitted: accumulated conversation history exceeded the %d-token compression size limit and could not be summarized in a single request.</truncation_notice>\n%s",
			compressStart-part.frozenEnd, maxCompressionContextTokens, contextXML)
	}

	compressionMsgs := make([]llm.Message, 0, len(r.task.Compression.Messages))
	for _, m := range r.task.Compression.Messages {
		content := strings.ReplaceAll(m.Content, "{{context}}", contextXML)
		compressionMsgs = append(compressionMsgs, llm.NewTextMessage(m.Role, content))
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

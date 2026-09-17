// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/acp/internal/contract"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

const progressLogLimit = 32 << 10
const progressInterval = 500 * time.Millisecond

type progressLog struct {
	text      string
	truncated bool
	dirty     bool
}

func (l *progressLog) append(message string) {
	message = strings.TrimRight(message, "\r\n")
	if message == "" {
		return
	}
	l.text += strings.ToValidUTF8(message, "?") + "\n"
	if len(l.text) > progressLogLimit {
		start := len(l.text) - progressLogLimit
		for start < len(l.text) && !utf8.RuneStart(l.text[start]) {
			start++
		}
		l.text = strings.Clone(l.text[start:])
		l.truncated = true
	}
	l.dirty = true
}

func (l *progressLog) content() []acp.ToolCallContent {
	return []acp.ToolCallContent{acp.ToolContent(acp.TextBlock(l.markdown()))}
}

func (l *progressLog) markdown() string {
	var b strings.Builder
	if l.truncated {
		b.WriteString("Earlier OCR log output was truncated.\n\n")
	}
	// Explicit plain text prevents client language guessing for process logs.
	code := l.text
	longest, run := 2, 0
	for _, r := range code {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	fmt.Fprintf(&b, "%stext\n%s", fence, code)
	if !strings.HasSuffix(code, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(fence + "\n")
	return b.String()
}

// A short escaped activity label remains visible while the log is collapsed.
func progressTitle(operation, message string, elapsed time.Duration) string {
	message = strings.ToValidUTF8(message, "?")
	lines := strings.Split(strings.TrimSpace(message), "\n")
	message = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[len(lines)-1]), "[ocr]"))
	var b strings.Builder
	for _, r := range message {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			r = ' '
		}
		if b.Len()+utf8.RuneLen(r) > 160 {
			b.WriteString("...")
			break
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return operation + " · Running · " + elapsed.Truncate(time.Second).String()
	}
	return operation + " · " + markdownLabel(b.String()) + " · " + elapsed.Truncate(time.Second).String()
}

// shellArgument formats an argv element for copying into a POSIX shell. The
// runner still executes the original argv directly, without a shell.
func shellArgument(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-", r)
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func executionCommandLine(binary string, request orchestrator.Request) string {
	parts := []string{shellArgument(binary)}
	for _, arg := range request.Args {
		parts = append(parts, shellArgument(arg))
	}
	return strings.Join(parts, " ")
}

func executionCommand(binary string, request orchestrator.Request) string {
	var b strings.Builder
	b.WriteString("Command:\n\nPOSIX shell syntax:\n\n")
	writeCodeBlock(&b, "command.sh", executionCommandLine(binary, request))
	b.WriteByte('\n')
	return b.String()
}

// collectProgress uses one native tool call for the entire OCR process. Log
// snapshots replace prior content at a bounded rate, not once per stderr line.
func (a *Agent) collectProgress(ctx context.Context, cancel context.CancelFunc, sessionID acp.SessionId, request orchestrator.Request) (orchestrator.Outcome, error) {
	id := nextToolCallID("ocr-execution")
	// Generic tools have a collapsible header in Zed. Execute titles instead
	// render as command previews, without the same activity-label interaction.
	operation := "OCR review"
	if len(request.Args) > 0 && request.Args[0] == "scan" {
		operation = "OCR scan"
	}
	started := time.Now()
	activity := ""
	title := progressTitle(operation, activity, 0)
	var details strings.Builder
	details.WriteString(executionCommand(a.binary, request))
	details.WriteString("Working directory:\n\n")
	writeCodeBlock(&details, "", request.CWD)
	executionDetails := acp.ToolContent(acp.TextBlock(details.String()))
	content := func(log *progressLog) []acp.ToolCallContent {
		blocks := []acp.ToolCallContent{executionDetails}
		if log.text != "" || log.truncated {
			blocks = append(blocks, log.content()...)
		}
		return blocks
	}
	if err := a.notify(ctx, sessionID, acp.StartToolCall(id, title, acp.WithStartKind(acp.ToolKindOther), acp.WithStartStatus(acp.ToolCallStatusInProgress), acp.WithStartContent([]acp.ToolCallContent{executionDetails}))); err != nil {
		return orchestrator.Outcome{}, err
	}
	events, outcomes := a.runner.Run(ctx, request)
	ticker := time.NewTicker(progressInterval)
	defer ticker.Stop()
	var log progressLog
	first := true
	var sendErr error
	flush := func() {
		if sendErr != nil || ctx.Err() != nil {
			return
		}
		nextTitle := progressTitle(operation, activity, time.Since(started))
		if !log.dirty && nextTitle == title {
			return
		}
		options := []acp.ToolCallUpdateOpt{acp.WithUpdateTitle(nextTitle)}
		if log.dirty {
			options = append(options, acp.WithUpdateContent(content(&log)))
		}
		sendErr = a.notify(ctx, sessionID, acp.UpdateToolCall(id, options...))
		title = nextTitle
		log.dirty = false
		if sendErr != nil {
			cancel()
		}
	}
	for events != nil {
		select {
		case event, open := <-events:
			if !open {
				events = nil
				continue
			}
			log.append(event.Message)
			log.truncated = log.truncated || event.Truncated
			if event.Kind == orchestrator.EventProgress && strings.TrimSpace(event.Message) != "" {
				activity = event.Message
			}
			if first && log.dirty {
				first = false
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
	flush()
	outcome := <-outcomes
	// The bounded stderr tail is authoritative when best-effort events dropped.
	if outcome.Diagnostics != "" {
		log.text = ""
		log.append(outcome.Diagnostics)
	}
	for _, warning := range outcome.Warnings {
		log.append(warning.Message)
		log.truncated = log.truncated || warning.Truncated
	}
	status := acp.ToolCallStatusCompleted
	kind := outcome.Kind
	if ctx.Err() == context.DeadlineExceeded {
		kind = orchestrator.OutcomeTimedOut
	} else if ctx.Err() != nil {
		kind = orchestrator.OutcomeCancelled
	}
	if outcome.Err != nil && kind == orchestrator.OutcomeCompleted {
		kind = orchestrator.OutcomeFailed
	}
	if sendErr != nil {
		kind = orchestrator.OutcomeFailed
		log.append("OCR progress delivery failed: " + sendErr.Error())
	}
	if kind != orchestrator.OutcomeCompleted || outcome.Err != nil {
		status = acp.ToolCallStatusFailed
	}
	if kind != orchestrator.OutcomeCompleted {
		log.append("OCR execution: " + string(kind))
	}
	if outcome.Err != nil {
		log.append(outcome.Err.Error())
	}
	// After cleanup, make one bounded terminal attempt even if progress failed.
	// A disconnected client may never receive it; preserve the original failure.
	finish, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer stop()
	finalContent := content(&log)
	if stats := executionStats(outcome.Result); stats != "" {
		finalContent = append(finalContent, acp.ToolContent(acp.TextBlock(stats)))
	}
	finalTitle := progressTitle(operation, executionResultLabel(kind, outcome.Result), time.Since(started))
	if err := a.notify(finish, sessionID, acp.UpdateToolCall(id, acp.WithUpdateTitle(finalTitle), acp.WithUpdateStatus(status), acp.WithUpdateContent(finalContent))); sendErr == nil {
		sendErr = err
	}
	return outcome, sendErr
}

// Execution failures take precedence over any usable result returned by OCR.
func executionResultLabel(kind orchestrator.OutcomeKind, result *orchestrator.Result) string {
	if kind == orchestrator.OutcomeCompleted && result != nil {
		var status string
		if result.Review != nil {
			status = reviewStatus(result.Review.Status, result.Review.Manifest)
		} else if result.Scan != nil {
			status = result.Scan.Status
		}
		if status != "" && status != contract.StatusSuccess && status != contract.StatusComplete {
			return resultStatus(status)
		}
	}
	return outcomeLabel(kind)
}

func outcomeLabel(kind orchestrator.OutcomeKind) string {
	switch kind {
	case orchestrator.OutcomeCompleted:
		return "Completed"
	case orchestrator.OutcomeCancelled:
		return "Cancelled"
	case orchestrator.OutcomeTimedOut:
		return "Timed out"
	default:
		return "Failed"
	}
}

// executionStats shows aggregate CLI facts, not inferred per-invocation tool calls.
func executionStats(result *orchestrator.Result) string {
	if result == nil {
		return ""
	}
	var summary *contract.Summary
	var calls *contract.ToolCalls
	if result.Review != nil {
		summary, calls = result.Review.Summary, result.Review.ToolCalls
	} else if result.Scan != nil {
		summary, calls = result.Scan.Summary, result.Scan.ToolCalls
	}
	var b strings.Builder
	if calls != nil {
		fmt.Fprintf(&b, "**Tool usage:** %d calls", calls.Total)
		names := make([]string, 0, len(calls.ByTool))
		for name := range calls.ByTool {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(&b, "\n\n%s × %d", markdownLabel(name), calls.ByTool[name])
		}
	}
	if summary != nil {
		var usage []string
		if summary.TotalTokens > 0 {
			usage = append(usage, fmt.Sprintf("%d total", summary.TotalTokens))
		}
		if summary.InputTokens != nil {
			usage = append(usage, fmt.Sprintf("%d input", *summary.InputTokens))
		}
		if summary.OutputTokens != nil {
			usage = append(usage, fmt.Sprintf("%d output", *summary.OutputTokens))
		}
		if summary.CacheReadTokens > 0 {
			usage = append(usage, fmt.Sprintf("%d cache read", summary.CacheReadTokens))
		}
		if summary.CacheWriteTokens > 0 {
			usage = append(usage, fmt.Sprintf("%d cache write", summary.CacheWriteTokens))
		}
		if len(usage) > 0 {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString("**Tokens (cumulative):** " + strings.Join(usage, " · "))
		}
	}
	return b.String()
}

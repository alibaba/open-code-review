// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
func progressTitle(message string) string {
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
		return "OCR progress"
	}
	return "OCR progress · " + markdownLabel(b.String())
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
	b.WriteString("Command:\n\n")
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
	title := "OCR progress"
	if err := a.update(ctx, sessionID, executionCommand(a.binary, request)); err != nil {
		return orchestrator.Outcome{}, err
	}
	var directory strings.Builder
	directory.WriteString("Working directory:\n\n")
	writeCodeBlock(&directory, "", request.CWD)
	workingDirectory := acp.ToolContent(acp.TextBlock(directory.String()))
	content := func(log *progressLog) []acp.ToolCallContent {
		return append([]acp.ToolCallContent{workingDirectory}, log.content()...)
	}
	if err := a.notify(ctx, sessionID, acp.StartToolCall(id, title, acp.WithStartKind(acp.ToolKindOther), acp.WithStartStatus(acp.ToolCallStatusInProgress), acp.WithStartContent([]acp.ToolCallContent{workingDirectory}))); err != nil {
		return orchestrator.Outcome{}, err
	}
	events, outcomes := a.runner.Run(ctx, request)
	ticker := time.NewTicker(progressInterval)
	defer ticker.Stop()
	var log progressLog
	first := true
	var sendErr error
	flush := func() {
		if !log.dirty || sendErr != nil || ctx.Err() != nil {
			return
		}
		sendErr = a.notify(ctx, sessionID, acp.UpdateToolCall(id, acp.WithUpdateTitle(title), acp.WithUpdateContent(content(&log))))
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
			if strings.TrimSpace(event.Message) != "" {
				title = progressTitle(event.Message)
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
	if err := a.notify(finish, sessionID, acp.UpdateToolCall(id, acp.WithUpdateTitle("OCR progress · "+string(kind)), acp.WithUpdateStatus(status), acp.WithUpdateContent(content(&log)))); sendErr == nil {
		sendErr = err
	}
	return outcome, sendErr
}

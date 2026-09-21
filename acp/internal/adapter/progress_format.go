// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
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
	writeFencedBlock(&b, "text", l.text)
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

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package telemetry

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/alibaba/open-code-review/internal/stdout"
)

// Event emits a structured event as a span with immediate end.
func Event(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	if !IsEnabled() || ctx == nil {
		return
	}

	opts := []trace.SpanStartOption{trace.WithAttributes(attrs...)}
	_, span := otel.GetTracerProvider().Tracer(serviceName).Start(ctx, "event."+name, opts...)
	defer span.End()
}

// Eventf is like Event but includes a message attribute.
func Eventf(ctx context.Context, name string, msg string, attrs ...attribute.KeyValue) {
	Event(ctx, name, append(attrs, attribute.String("message", msg))...)
}

// ErrorEvent emits an error event with error status.
func ErrorEvent(ctx context.Context, name string, err error, extraAttrs ...attribute.KeyValue) {
	if !IsEnabled() || ctx == nil || err == nil {
		return
	}

	attrs := append(extraAttrs, attribute.String("error", err.Error()))
	_, span := otel.GetTracerProvider().Tracer(serviceName).Start(ctx, "event."+name,
		trace.WithAttributes(attrs...))
	span.SetStatus(codes.Error, err.Error())
	span.RecordError(err)
	span.End()
}

// PhaseEvent records a review phase completion with duration and optional error.
func PhaseEvent(ctx context.Context, phase string, filePath string, dur time.Duration, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("phase", phase),
		attribute.String("file.path", filePath),
		attribute.Int64("duration_ms", dur.Milliseconds()),
	}
	if err != nil {
		ErrorEvent(ctx, "phase.completed", err, attrs...)
	} else {
		Event(ctx, "phase.completed", attrs...)
	}
}

// FormatDuration returns a human-readable duration string for console output.
func FormatDuration(dur time.Duration) string {
	return dur.Round(time.Millisecond).String()
}

// TraceSummary carries the metrics printed by PrintTraceSummary.
type TraceSummary struct {
	FilesReviewed     int64
	CommentsGenerated int64
	InputTokens       int64
	OutputTokens      int64
	TotalTokens       int64
	CacheReadTokens   int64
	CacheWriteTokens  int64
	Duration          time.Duration
	SessionID         string
}

// PrintTraceSummary prints a one-line summary of the review to stdout.
// If SessionID is non-empty, an "[ocr] Session: <id>" line follows the summary.
func PrintTraceSummary(s TraceSummary) {
	elapsed := s.Duration.Round(time.Second).String()
	if s.InputTokens > 0 || s.OutputTokens > 0 {
		base := fmt.Sprintf("[ocr] Summary: %d file(s) reviewed, %d comment(s), ~%d token(s) used (input: ~%d, output: ~%d)",
			s.FilesReviewed, s.CommentsGenerated, s.TotalTokens, s.InputTokens, s.OutputTokens)
		if s.CacheReadTokens > 0 || s.CacheWriteTokens > 0 {
			base += fmt.Sprintf(", cache(read: ~%d, write: ~%d)", s.CacheReadTokens, s.CacheWriteTokens)
		}
		fmt.Fprintf(stdout.Writer(), "%s, %s elapsed\n", base, elapsed)
	} else {
		fmt.Fprintf(stdout.Writer(), "[ocr] Summary: %d file(s) reviewed, %d comment(s), ~%d token(s) used, %s elapsed\n",
			s.FilesReviewed, s.CommentsGenerated, s.TotalTokens, elapsed)
	}
	if s.SessionID != "" {
		fmt.Fprintf(stdout.Writer(), "[ocr] Session: %s\n", s.SessionID)
	}
}

// PrintToolCallStarted prints a line when a tool begins execution.
// Args are summarized as key-value pairs (path, search terms, etc.).
// Example: [ocr]   ▶ file_read "internal/config/rules/loader.go"
func PrintToolCallStarted(toolName string, args map[string]any) {
	summary := summarizeArgs(args)
	if summary != "" {
		fmt.Fprintf(stdout.Writer(), "[ocr]   ▶ %s %s\n", toolName, summary)
	} else {
		fmt.Fprintf(stdout.Writer(), "[ocr]   ▶ %s\n", toolName)
	}
}

// PrintToolCallFinished prints a line when a tool finishes successfully.
// Example: [ocr]   ✔ file_read "internal/config/rules/loader.go" (12ms)
func PrintToolCallFinished(toolName string, dur time.Duration) {
	fmt.Fprintf(stdout.Writer(), "[ocr]   ✔ %s (%s)\n", toolName, FormatDuration(dur))
}

// PrintToolCallError prints a line when a tool fails.
// Example: [ocr]   ✘ file_read "internal/config/rules/loader.go" failed: permission denied
func PrintToolCallError(toolName string, err error) {
	fmt.Fprintf(os.Stderr, "[ocr]   ✘ %s failed: %v\n", toolName, err)
}

// summarizeArgs extracts a concise key=value summary from tool arguments for console display.
// It picks the most human-readable fields depending on the argument keys.
func summarizeArgs(args map[string]any) string {
	parts := make([]string, 0, len(args))
	for k, v := range args {
		s := fmt.Sprint(v)
		switch k {
		case "path":
			return strconv.Quote(s)
		case "search", "query", "pattern":
			return strconv.Quote(s)
		default:
			if len(s) <= 50 {
				parts = append(parts, fmt.Sprintf("%s=%s", k, s))
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

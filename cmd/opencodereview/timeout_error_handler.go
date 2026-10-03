// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TimeoutErrorInfo holds detailed information about a timeout error.
type TimeoutErrorInfo struct {
	// IsTimeout indicates this is a context deadline exceeded error
	IsTimeout bool
	// ElapsedSeconds is how long the operation ran before timing out
	ElapsedSeconds float64
	// TimeoutMinutes is the configured timeout limit
	TimeoutMinutes int
	// SessionID for resuming the review
	SessionID string
}

// isTimeoutError checks if an error is a context deadline exceeded error.
func isTimeoutError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}

// analyzeTimeoutError extracts details from a timeout error and returns
// information useful for user guidance.
func analyzeTimeoutError(runErr error, startTime time.Time, timeoutMinutes int) TimeoutErrorInfo {
	info := TimeoutErrorInfo{
		IsTimeout:      isTimeoutError(runErr),
		ElapsedSeconds: time.Since(startTime).Seconds(),
		TimeoutMinutes: timeoutMinutes,
	}
	return info
}

// formatTimeoutErrorMessage creates a user-friendly error message with
// recovery suggestions.
func formatTimeoutErrorMessage(info TimeoutErrorInfo) string {
	var sb strings.Builder

	sb.WriteString("⏱️  Review task timeout: A task exceeded its configured ")
	sb.WriteString(fmt.Sprintf("%d minute", info.TimeoutMinutes))
	if info.TimeoutMinutes > 1 {
		sb.WriteString("s")
	}
	sb.WriteString(" limit")

	// Add elapsed time if available
	if info.ElapsedSeconds > 0 {
		minutes := int(info.ElapsedSeconds / 60)
		seconds := int(info.ElapsedSeconds) % 60
		sb.WriteString(fmt.Sprintf(" (ran for %dm%02ds)", minutes, seconds))
	}
	sb.WriteString("\n")

	sb.WriteString("\n📋 Suggestions to recover:\n")

	// Suggestion 1: Increase timeout
	newTimeout := info.TimeoutMinutes + 5
	sb.WriteString("  1. Increase timeout limit:\n")
	sb.WriteString(fmt.Sprintf("     $ ocr review --timeout %d\n\n", newTimeout))

	// Suggestion 2: Reduce concurrency
	sb.WriteString("  2. Reduce concurrent tasks (use less memory/time per task):\n")
	sb.WriteString("     $ ocr review --concurrency 4\n\n")

	// Suggestion 3: Exclude large files
	sb.WriteString("  3. Exclude large or auto-generated files:\n")
	sb.WriteString("     $ ocr review --exclude '**/dist/**,**/build/**,**/*.min.js'\n\n")

	// Suggestion 4: Resume
	if info.SessionID != "" {
		sb.WriteString("  4. Re-run your original command with --resume to continue where it stopped:\n")
		sb.WriteString(fmt.Sprintf("     $ ocr review <original args> --resume %s\n\n", info.SessionID))
	}

	sb.WriteString("📖 Learn more: https://open-codereview.ai/docs\n")

	return sb.String()
}

// enhanceTimeoutError wraps a timeout error with helpful context.
func enhanceTimeoutError(runErr error, startTime time.Time, timeoutMinutes int, sessionID string) error {
	if !isTimeoutError(runErr) {
		return runErr
	}

	info := analyzeTimeoutError(runErr, startTime, timeoutMinutes)
	info.SessionID = sessionID

	return fmt.Errorf("%s: %w", formatTimeoutErrorMessage(info), runErr)
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "context.DeadlineExceeded",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "wrapped deadline error",
			err:      fmt.Errorf("operation failed: %w", context.DeadlineExceeded),
			expected: true,
		},
		{
			name:     "error with deadline text",
			err:      fmt.Errorf("context deadline exceeded"),
			expected: true,
		},
		{
			name:     "non-timeout error",
			err:      fmt.Errorf("some other error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTimeoutError(tt.err)
			if result != tt.expected {
				t.Errorf("isTimeoutError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestAnalyzeTimeoutError(t *testing.T) {
	startTime := time.Now().Add(-2*time.Minute - 30*time.Second)
	err := context.DeadlineExceeded

	info := analyzeTimeoutError(err, startTime, 15)

	if !info.IsTimeout {
		t.Errorf("IsTimeout = %v, want true", info.IsTimeout)
	}

	// Allow some margin for test execution time
	if info.ElapsedSeconds < 149 || info.ElapsedSeconds > 151 {
		t.Errorf("ElapsedSeconds = %v, want ~150", info.ElapsedSeconds)
	}

	if info.TimeoutMinutes != 15 {
		t.Errorf("TimeoutMinutes = %v, want 15", info.TimeoutMinutes)
	}
}

func TestFormatTimeoutErrorMessage(t *testing.T) {
	info := TimeoutErrorInfo{
		IsTimeout:       true,
		ElapsedSeconds:  150.5,
		TimeoutMinutes:  15,
		TotalInputTokens: 180000,
		TotalOutputTokens: 45000,
		LastFile:        "src/processor.go",
		SessionID:       "session-abc123",
	}

	msg := formatTimeoutErrorMessage(info)

	// Check for key components
	checks := []string{
		"⏱️  LLM timeout",
		"15 minute",
		"180000 input tokens",
		"45000 output tokens",
		"src/processor.go",
		"--timeout",
		"--concurrency",
		"--exclude",
		"--resume session-abc123",
		"https://open-codereview.ai/docs/troubleshooting",
	}

	for _, check := range checks {
		if !strings.Contains(msg, check) {
			t.Errorf("formatTimeoutErrorMessage missing %q\n\nFull message:\n%s", check, msg)
		}
	}
}

func TestFormatTimeoutErrorMessage_MinimalInfo(t *testing.T) {
	info := TimeoutErrorInfo{
		IsTimeout:      true,
		ElapsedSeconds: 0,
		TimeoutMinutes: 15,
	}

	msg := formatTimeoutErrorMessage(info)

	// Should still contain suggestions even with minimal info
	if !strings.Contains(msg, "⏱️  LLM timeout") {
		t.Errorf("Missing timeout indicator in message:\n%s", msg)
	}
	if !strings.Contains(msg, "--timeout") {
		t.Errorf("Missing timeout suggestion in message:\n%s", msg)
	}
}

func TestEnhanceTimeoutError_NonTimeout(t *testing.T) {
	regularErr := fmt.Errorf("some other error")
	startTime := time.Now()

	result := enhanceTimeoutError(regularErr, startTime, 15, "")

	if result != regularErr {
		t.Errorf("enhanceTimeoutError should return original error for non-timeout, got %v", result)
	}
}

func TestEnhanceTimeoutError_Timeout(t *testing.T) {
	startTime := time.Now().Add(-2 * time.Minute)
	timeoutErr := context.DeadlineExceeded

	result := enhanceTimeoutError(timeoutErr, startTime, 15, "session-xyz")

	errMsg := result.Error()
	if !strings.Contains(errMsg, "⏱️  LLM timeout") {
		t.Errorf("Enhanced error missing timeout indicator:\n%s", errMsg)
	}
	if !strings.Contains(errMsg, "session-xyz") {
		t.Errorf("Enhanced error missing session ID:\n%s", errMsg)
	}
}

func TestFormatTimeoutErrorMessage_PluralSingular(t *testing.T) {
	// Test singular "minute"
	info := TimeoutErrorInfo{
		IsTimeout:      true,
		TimeoutMinutes: 1,
	}
	msg := formatTimeoutErrorMessage(info)
	if !strings.Contains(msg, "1 minute ") {
		t.Errorf("Should use singular 'minute' for timeout=1, got:\n%s", msg)
	}

	// Test plural "minutes"
	info.TimeoutMinutes = 15
	msg = formatTimeoutErrorMessage(info)
	if !strings.Contains(msg, "15 minutes ") {
		t.Errorf("Should use plural 'minutes' for timeout=15, got:\n%s", msg)
	}
}

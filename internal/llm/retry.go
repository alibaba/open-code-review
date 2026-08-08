// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	openai "github.com/openai/openai-go/v3"
)

const (
	llmMaxAttempts    = 3
	llmRetryBaseDelay = time.Second
)

func withLLMRetry(ctx context.Context, call func(context.Context) (*ChatResponse, error)) (*ChatResponse, error) {
	var lastErr error
	for attempt := 1; attempt <= llmMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		response, err := call(ctx)
		if err == nil {
			return response, nil
		}
		lastErr = err
		if attempt == llmMaxAttempts || !retryableLLMError(ctx, err) {
			return nil, err
		}
		if err := waitForLLMRetry(ctx, llmRetryBaseDelay*time.Duration(1<<(attempt-1))); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func waitForLLMRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func retryableLLMError(ctx context.Context, err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ctx.Err() == nil
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	var networkErr net.Error
	if errors.As(err, &networkErr) && (networkErr.Timeout() || networkErr.Temporary()) {
		return true
	}

	var openAIError *openai.Error
	if errors.As(err, &openAIError) {
		return retryableHTTPStatus(openAIError.StatusCode)
	}
	var anthropicError *anthropic.Error
	if errors.As(err, &anthropicError) {
		return retryableHTTPStatus(anthropicError.StatusCode)
	}
	return false
}

func retryableHTTPStatus(status int) bool {
	return status == 408 || status == 429 || status >= 500
}

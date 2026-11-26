// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	openai "github.com/openai/openai-go/v3"
)

// restoredBody replays bytes already peeked from an HTTP response body while
// closing the underlying body when the consumer is done.
type restoredBody struct {
	io.Reader
	io.Closer
}

// isTerminalQuotaToken reports whether a structured provider error type or code
// represents an exhausted budget or billing quota that cannot succeed on an
// immediate retry. Transient rate limits (rate_limit_error, rate_limit_exceeded,
// overloaded_error, etc.) return false so SDK backoff retries stay intact.
func isTerminalQuotaToken(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "budget_exceeded", "insufficient_quota":
		return true
	default:
		return false
	}
}

func decodeJSONString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// isTerminalQuotaPayload inspects a JSON error body for structured terminal
// budget/quota identifiers in either nested ({"error":{"type":...,"code":...}})
// or flat ({"type":...,"code":...}) form.
func isTerminalQuotaPayload(body []byte) bool {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		return false
	}
	var envelope struct {
		Type  json.RawMessage `json:"type"`
		Code  json.RawMessage `json:"code"`
		Error *struct {
			Type json.RawMessage `json:"type"`
			Code json.RawMessage `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return false
	}
	if envelope.Error != nil {
		if isTerminalQuotaToken(decodeJSONString(envelope.Error.Type)) ||
			isTerminalQuotaToken(decodeJSONString(envelope.Error.Code)) {
			return true
		}
	}
	return isTerminalQuotaToken(decodeJSONString(envelope.Type)) ||
		isTerminalQuotaToken(decodeJSONString(envelope.Code))
}

// nonRetryableQuotaMiddleware marks HTTP 429 (and any retry-forced 4xx)
// responses whose JSON error payload reports an exhausted budget or billing
// quota with x-should-retry: false so the SDK stops after the first attempt
// instead of burning retries and backoff. The response body is restored
// losslessly for the SDK's own error decoder.
func nonRetryableQuotaMiddleware() func(*http.Request, func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	return func(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		resp, err := next(req)
		if err != nil || resp == nil || resp.Body == nil {
			return resp, err
		}
		if resp.Header.Get("x-should-retry") == "false" {
			return resp, nil
		}
		shouldCheck := resp.StatusCode == http.StatusTooManyRequests ||
			(resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.Header.Get("x-should-retry") == "true")
		if !shouldCheck {
			return resp, nil
		}

		orig := resp.Body
		peeked, readErr := io.ReadAll(io.LimitReader(orig, maxErrorBodyBytes+1))
		if int64(len(peeked)) <= maxErrorBodyBytes && readErr == nil {
			resp.Body = &restoredBody{Reader: bytes.NewReader(peeked), Closer: orig}
		} else {
			resp.Body = &restoredBody{Reader: io.MultiReader(bytes.NewReader(peeked), orig), Closer: orig}
		}

		if readErr == nil && int64(len(peeked)) <= maxErrorBodyBytes && isTerminalQuotaPayload(peeked) {
			resp.Header.Set("x-should-retry", "false")
		}
		return resp, nil
	}
}

// IsBudgetExceededError reports whether err is a terminal provider or gateway
// budget/quota exhaustion error (such as LiteLLM's budget_exceeded or OpenAI's
// insufficient_quota) that cannot succeed on subsequent calls in the same run.
func IsBudgetExceededError(err error) bool {
	if err == nil {
		return false
	}
	var custom interface{ IsBudgetExceeded() bool }
	if errors.As(err, &custom) && custom.IsBudgetExceeded() {
		return true
	}
	var openAI *openai.Error
	if errors.As(err, &openAI) {
		if isTerminalQuotaToken(openAI.Type) || isTerminalQuotaToken(openAI.Code) ||
			isTerminalQuotaPayload([]byte(openAI.RawJSON())) {
			return true
		}
	}
	var anthropicErr *anthropic.Error
	if errors.As(err, &anthropicErr) {
		if isTerminalQuotaToken(string(anthropicErr.Type())) ||
			isTerminalQuotaPayload([]byte(anthropicErr.RawJSON())) {
			return true
		}
	}
	return false
}

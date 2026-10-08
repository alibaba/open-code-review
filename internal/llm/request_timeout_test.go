// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
)

func TestRequestTimeoutAndSDKRetryBehavior(t *testing.T) {
	for _, protocol := range []string{"openai", "responses", "anthropic"} {
		for _, scenario := range []struct {
			name           string
			wantAttempts   int32
			wantError      string
			requestTimeout time.Duration
			taskTimeout    time.Duration
			cancelAfter    time.Duration
		}{
			{name: "headers", wantAttempts: 1, wantError: "request", requestTimeout: 100 * time.Millisecond},
			{name: "body", wantAttempts: 1, wantError: "request", requestTimeout: 100 * time.Millisecond},
			{name: "task_during_request", wantAttempts: 1, wantError: "task", taskTimeout: 100 * time.Millisecond},
			{name: "cancel_during_request", wantAttempts: 1, wantError: "cancel", cancelAfter: 100 * time.Millisecond},
			{name: "request_during_backoff", wantAttempts: 1, wantError: "request", requestTimeout: 100 * time.Millisecond},
			{name: "task_during_backoff", wantAttempts: 1, wantError: "task", taskTimeout: 100 * time.Millisecond},
			{name: "cancel_during_backoff", wantAttempts: 1, wantError: "cancel", cancelAfter: 100 * time.Millisecond},
			{name: "sdk_retry_success", wantAttempts: 2},
			{name: "sdk_retry_limit", wantAttempts: 6, wantError: "provider"},
			{name: "permanent_error", wantAttempts: 1, wantError: "provider"},
		} {
			t.Run(protocol+"/"+scenario.name, func(t *testing.T) {
				t.Parallel()
				var attempts atomic.Int32
				release := make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					attempt := attempts.Add(1)
					writer.Header().Set("Content-Type", "application/json")
					switch {
					case scenario.name == "permanent_error":
						writer.WriteHeader(http.StatusBadRequest)
						fmt.Fprint(writer, `{"error":{"message":"invalid"}}`)
						return
					case strings.HasSuffix(scenario.name, "_backoff"):
						writer.Header().Set("Retry-After-Ms", "3000")
						writer.WriteHeader(http.StatusTooManyRequests)
						fmt.Fprint(writer, `{"error":{"message":"wait"}}`)
						return
					case scenario.name == "sdk_retry_limit" || scenario.name == "sdk_retry_success" && attempt == 1:
						writer.Header().Set("Retry-After-Ms", "0")
						writer.WriteHeader(http.StatusServiceUnavailable)
						fmt.Fprint(writer, `{"error":{"message":"busy"}}`)
						return
					case attempt == 1:
						if scenario.name == "body" {
							writer.WriteHeader(http.StatusOK)
							writer.(http.Flusher).Flush()
						}
						select {
						case <-request.Context().Done():
						case <-release:
						}
						return
					}
					body := openAIOKBody
					if protocol == "responses" {
						body = responsesOKBody
					}
					if protocol == "anthropic" {
						body = anthropicOKBody
					}
					fmt.Fprint(writer, body)
				}))
				defer server.Close()
				defer close(release)
				collector := NewRetryCollector()
				cfg := ClientConfig{URL: server.URL, APIKey: "test-key", Model: "test-model", Timeout: 5 * time.Second, retryCollector: collector}
				if scenario.requestTimeout > 0 {
					cfg.Timeout = scenario.requestTimeout
				}
				var client LLMClient
				switch protocol {
				case "openai":
					client = NewOpenAIClient(cfg)
				case "responses":
					client = NewOpenAIResponsesClient(cfg)
				case "anthropic":
					client = NewAnthropicClient(cfg)
				}
				ctx, cancel := context.WithCancel(metaCtx(testMeta()))
				defer cancel()
				if scenario.taskTimeout > 0 {
					var deadlineCancel context.CancelFunc
					ctx, deadlineCancel = context.WithTimeout(ctx, scenario.taskTimeout)
					defer deadlineCancel()
				}
				if scenario.cancelAfter > 0 {
					timer := time.AfterFunc(scenario.cancelAfter, cancel)
					defer timer.Stop()
				}
				_, err := ping(ctx, client)
				switch scenario.wantError {
				case "":
					if err != nil {
						t.Fatalf("ping: %v", err)
					}
				case "request":
					if !errors.Is(err, ErrRequestTimeout) || !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("error = %v, want request timeout", err)
					}
				case "task":
					if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrRequestTimeout) || !strings.Contains(err.Error(), "task deadline exhausted") {
						t.Fatalf("error = %v, want task deadline", err)
					}
				case "cancel":
					if !errors.Is(err, context.Canceled) || errors.Is(err, ErrRequestTimeout) {
						t.Fatalf("error = %v, want cancellation", err)
					}
				case "provider":
					if err == nil || errors.Is(err, ErrRequestTimeout) {
						t.Fatalf("error = %v, want provider error", err)
					}
				}
				if got := attempts.Load(); got != scenario.wantAttempts {
					t.Fatalf("attempts = %d, want %d", got, scenario.wantAttempts)
				}
				report := freezeOne(t, collector)
				if len(report.Attempts) != int(scenario.wantAttempts) {
					t.Fatalf("unexpected report: %+v", report)
				}
				wantOutcome := OutcomeFailed
				if scenario.wantError == "" {
					wantOutcome = OutcomeRecovered
				}
				if scenario.wantError == "cancel" {
					wantOutcome = OutcomeCancelled
				}
				if report.Outcome != wantOutcome {
					t.Fatalf("outcome = %s, want %s", report.Outcome, wantOutcome)
				}
				if scenario.name == "headers" || scenario.name == "body" {
					if report.Attempts[0].ErrorClass != ErrorClassTimeout {
						t.Fatalf("timeout not recorded: %+v", report)
					}
				}
			})
		}
	}
}

func TestRequestTimeoutStreamingNoReplay(t *testing.T) {
	var attempts atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		if attempts.Add(1) == 1 {
			fmt.Fprint(writer, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"discarded\"}}]}\n\n")
			writer.(http.Flusher).Flush()
			select {
			case <-request.Context().Done():
			case <-release:
			}
			return
		}
		fmt.Fprint(writer, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	defer close(release)
	collector := NewRetryCollector()
	client := NewOpenAIClient(ClientConfig{URL: server.URL, Model: "test-model", APIKey: "test-key", Timeout: 100 * time.Millisecond, ExtraBody: map[string]any{"stream": true}, retryCollector: collector})
	response, err := ping(metaCtx(testMeta()), client)
	if !errors.Is(err, ErrRequestTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want request timeout", err)
	}
	if response != nil {
		t.Fatalf("unexpected partial response: %+v", response)
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", attempts.Load())
	}
	report := freezeOne(t, collector)
	if report.Outcome != OutcomeFailed || report.Attempts[0].ErrorClass != ErrorClassTimeout {
		t.Fatalf("report = %+v", report)
	}
}

func TestRequestTimeoutBedrockNoReplay(t *testing.T) {
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "credentials"))
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "")
	var attempts atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("request is not signed")
		}
		if attempts.Add(1) == 1 {
			select {
			case <-request.Context().Done():
			case <-release:
			}
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, anthropicOKBody)
	}))
	defer server.Close()
	defer close(release)
	collector := NewRetryCollector()
	client := NewAnthropicBedrockClient(ClientConfig{Model: "test-model", AWSRegion: "us-east-1", Timeout: 100 * time.Millisecond, retryCollector: collector})
	client.sdk.Messages.Options = append(client.sdk.Messages.Options, option.WithBaseURL(server.URL))
	_, err := ping(metaCtx(testMeta()), client)
	if !errors.Is(err, ErrRequestTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want request timeout", err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", attempts.Load())
	}
	if report := freezeOne(t, collector); report.Outcome != OutcomeFailed {
		t.Fatalf("report = %+v", report)
	}
}

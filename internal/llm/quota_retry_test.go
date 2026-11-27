// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestBudgetExceededDoesNotRetry(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		respHeaders map[string]string
		build       func(url string) LLMClient
		wantCode    int
	}{
		{
			name: "openai_chat_streaming_budget_exceeded",
			body: `{"error":{"message":"Budget has been exceeded! Current cost: 10.0, Max budget: 10.0","type":"budget_exceeded","code":"429"}}`,
			build: func(url string) LLMClient {
				return NewOpenAIClient(ClientConfig{
					URL:       url + "/v1",
					APIKey:    "test-key",
					Model:     "gpt-test",
					ExtraBody: map[string]any{"stream": true},
				})
			},
		},
		{
			name: "openai_chat_streaming_flat_budget_exceeded",
			body: `{"message":"Budget has been exceeded! Current cost: 10.0, Max budget: 10.0","type":"budget_exceeded","param":null,"code":"429"}`,
			build: func(url string) LLMClient {
				return NewOpenAIClient(ClientConfig{
					URL:       url + "/v1",
					APIKey:    "test-key",
					Model:     "gpt-test",
					ExtraBody: map[string]any{"stream": true},
				})
			},
		},
		{
			name: "openai_chat_nonstreaming_budget_exceeded",
			body: `{"error":{"message":"Budget has been exceeded","type":"budget_exceeded","code":"429"}}`,
			build: func(url string) LLMClient {
				return NewOpenAIClient(ClientConfig{
					URL:    url + "/v1",
					APIKey: "test-key",
					Model:  "gpt-test",
				})
			},
		},
		{
			name: "openai_chat_nonstreaming_flat_budget_exceeded",
			body: `{"message":"Budget has been exceeded","type":"budget_exceeded","code":"429"}`,
			build: func(url string) LLMClient {
				return NewOpenAIClient(ClientConfig{
					URL:    url + "/v1",
					APIKey: "test-key",
					Model:  "gpt-test",
				})
			},
		},
		{
			name:        "openai_chat_flat_budget_exceeded_with_preset_no_retry_header",
			body:        `{"message":"Budget has been exceeded","type":"budget_exceeded","code":"429"}`,
			respHeaders: map[string]string{"x-should-retry": "false"},
			build: func(url string) LLMClient {
				return NewOpenAIClient(ClientConfig{
					URL:    url + "/v1",
					APIKey: "test-key",
					Model:  "gpt-test",
				})
			},
		},
		{
			name: "openai_chat_overrides_retry_codes_429",
			body: `{"error":{"message":"Budget has been exceeded","type":"budget_exceeded","code":"429"}}`,
			build: func(url string) LLMClient {
				return NewOpenAIClient(ClientConfig{
					URL:        url + "/v1",
					APIKey:     "test-key",
					Model:      "gpt-test",
					RetryCodes: []int{429},
				})
			},
		},
		{
			name: "openai_responses_insufficient_quota",
			body: `{"error":{"message":"You exceeded your current quota","type":"insufficient_quota","code":"insufficient_quota"}}`,
			build: func(url string) LLMClient {
				return NewOpenAIResponsesClient(ClientConfig{
					URL:    url + "/v1",
					APIKey: "test-key",
					Model:  "gpt-test",
				})
			},
		},
		{
			name: "openai_responses_flat_insufficient_quota",
			body: `{"message":"You exceeded your current quota","type":"insufficient_quota","code":"insufficient_quota"}`,
			build: func(url string) LLMClient {
				return NewOpenAIResponsesClient(ClientConfig{
					URL:    url + "/v1",
					APIKey: "test-key",
					Model:  "gpt-test",
				})
			},
		},
		{
			name: "anthropic_budget_exceeded",
			body: `{"type":"error","error":{"type":"budget_exceeded","message":"Budget has been exceeded"}}`,
			build: func(url string) LLMClient {
				return NewAnthropicClient(ClientConfig{
					URL:    url,
					APIKey: "test-key",
					Model:  "claude-test",
				})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After-Ms", "0")
				for k, v := range tc.respHeaders {
					w.Header().Set(k, v)
				}
				w.WriteHeader(http.StatusTooManyRequests)
				if requests.Add(1) == 1 {
					_, _ = fmt.Fprint(w, tc.body)
					return
				}
				_, _ = fmt.Fprint(w, `{"error":{"message":"unexpected retry","type":"rate_limit_error"}}`)
			}))
			defer server.Close()

			client := tc.build(server.URL)
			_, err := ping(context.Background(), client)
			if got := requests.Load(); got != 1 {
				t.Fatalf("got %d HTTP attempts, want 1", got)
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !IsBudgetExceededError(err) {
				t.Fatalf("IsBudgetExceededError(%v) = false, want true", err)
			}
		})
	}
}

func TestSpoofedTerminalQuotaHeaderIgnoredOnTransientRateLimit(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			w.Header().Set("Retry-After-Ms", "0")
			w.Header().Set(terminalQuotaHeader, "true")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"message":"Rate limit reached","type":"rate_limit_error","code":"rate_limit_exceeded"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"id":"chatcmpl-1","object":"chat.completion","model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	client := openAIClient(server.URL, nil, nil)
	resp, err := ping(context.Background(), client)
	if err != nil {
		t.Fatalf("expected transient rate limit to retry and succeed, got error: %v", err)
	}
	if resp == nil || resp.Content() != "ok" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("got %d HTTP attempts, want 2", got)
	}
}

func TestTransientRateLimitStillRetries(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			w.Header().Set("Retry-After-Ms", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"error":{"message":"Rate limit reached","type":"rate_limit_error","code":"rate_limit_exceeded"}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"id":"chatcmpl-1","object":"chat.completion","model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	client := openAIClient(server.URL, nil, nil)
	resp, err := ping(context.Background(), client)
	if err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}
	if resp == nil || resp.Content() != "ok" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("got %d HTTP attempts, want 2", got)
	}
}

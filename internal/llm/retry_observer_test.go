// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- helpers ---

const (
	anthropicOKBody = `{
		"id":"msg_test","type":"message","role":"assistant","model":"claude-test",
		"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn",
		"usage":{"input_tokens":1,"output_tokens":1}
	}`
	openAIOKBody = `{
		"id":"chatcmpl-test","object":"chat.completion","model":"gpt-test",
		"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
	}`
	responsesOKBody = `{
		"id":"resp_test","object":"response","model":"gpt-test","status":"completed",
		"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
	}`
)

// finalizeForTest stands in for the client return boundary, which P3 adds.
//
// It exists so P2 can assert on a frozen report at all: Freeze refuses to build
// one while any logical request is unfinalized. P3 must delete this helper and
// let the client boundary finalize instead — leaving it in place would keep the
// Freeze-level tests green even if P3 forgot to wire one of the three clients.
func finalizeForTest(c *RetryCollector, m RequestMeta, reqErr error) {
	c.Finalize(m, reqErr, false)
}

// attemptsFor returns a copy of the attempts recorded for m.
//
// It takes the collector's lock rather than reading entries directly: a test
// that leaves a request in flight would otherwise race, and that failure would
// show up as a flake in an unrelated test rather than here.
func attemptsFor(t *testing.T, c *RetryCollector, m RequestMeta) []AttemptRecord {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entries[m]
	if e == nil {
		return nil
	}
	return append([]AttemptRecord(nil), e.attempts...)
}

func ping(ctx context.Context, client LLMClient) (*ChatResponse, error) {
	return client.CompletionsWithCtx(ctx, ChatRequest{
		Messages:  []Message{{Role: "user", Content: "ping"}},
		MaxTokens: 64,
	})
}

// metaCtx attaches identity the way the review path will in P4.
func metaCtx(m RequestMeta) context.Context {
	return WithRequestMeta(context.Background(), m)
}

func anthropicClient(url string, c *RetryCollector, extra map[string]string) *AnthropicClient {
	return NewAnthropicClient(ClientConfig{
		URL:            url + "/v1/messages",
		APIKey:         "test-key",
		Model:          "claude-test",
		AuthHeader:     "x-api-key",
		ExtraHeaders:   extra,
		retryCollector: c,
	})
}

// --- attempt-level observation ---

// The canonical retry: one 429 with a server hint, then success. Everything the
// observer is responsible for is visible in this one sequence.
func TestObserverRecordsRateLimitedThenSuccess(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("request-id", fmt.Sprintf("req_%d", requests.Load()+1))
		if requests.Add(1) == 1 {
			w.Header().Set("Retry-After-Ms", "40")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
			return
		}
		_, _ = fmt.Fprint(w, anthropicOKBody)
	}))
	defer server.Close()

	c := NewRetryCollector()
	m := testMeta()
	if _, err := ping(metaCtx(m), anthropicClient(server.URL, c, nil)); err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}

	got := attemptsFor(t, c, m)
	if len(got) != 2 {
		t.Fatalf("got %d attempts, want 2", len(got))
	}

	first, second := got[0], got[1]
	if first.Number != 1 || first.Outcome != AttemptError {
		t.Errorf("attempt 1 = number %d outcome %s, want 1/error", first.Number, first.Outcome)
	}
	if first.ErrorClass != ErrorClassRateLimited || first.FailurePhase != FailurePhaseHTTP {
		t.Errorf("attempt 1 classified as %s/%s, want rate_limited/http", first.ErrorClass, first.FailurePhase)
	}
	if first.StatusCode != http.StatusTooManyRequests {
		t.Errorf("attempt 1 status_code = %d, want 429", first.StatusCode)
	}
	if first.RequestID != "req_1" {
		t.Errorf("attempt 1 request_id = %q, want req_1", first.RequestID)
	}
	if first.RetryAfterMS != 40 {
		t.Errorf("attempt 1 retry_after_ms = %d, want 40", first.RetryAfterMS)
	}
	if first.ObservedBackoffMS != 0 {
		t.Errorf("attempt 1 observed_backoff_ms = %d, want 0 (no predecessor)", first.ObservedBackoffMS)
	}

	if second.Number != 2 || second.Outcome != AttemptSuccess {
		t.Errorf("attempt 2 = number %d outcome %s, want 2/success", second.Number, second.Outcome)
	}
	if second.ErrorClass != "" || second.FailurePhase != "" {
		t.Errorf("attempt 2 carries error fields: %+v", second)
	}
	if second.RequestID != "req_2" {
		t.Errorf("attempt 2 request_id = %q, want req_2", second.RequestID)
	}
	// The SDK honors Retry-After-Ms verbatim, and the sleep happens around the
	// middleware, so the measured gap must cover it. Asserted with slack because
	// it is a real elapsed time, not a computed one.
	if second.ObservedBackoffMS < 30 {
		t.Errorf("attempt 2 observed_backoff_ms = %d, want >= 30 (server asked for 40)", second.ObservedBackoffMS)
	}

	finalizeForTest(c, m, nil)
	rep, err := c.Freeze("test-run-id")
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if rep == nil {
		t.Fatal("Freeze returned no report, want one")
	}
	if rep.TotalRequests != 1 || rep.RetriedRequests != 1 || rep.TotalRetries != 1 {
		t.Errorf("aggregates = total %d retried %d retries %d, want 1/1/1",
			rep.TotalRequests, rep.RetriedRequests, rep.TotalRetries)
	}
	if rep.RecoveredRequests != 1 || rep.FailedRequests != 0 {
		t.Errorf("recovered %d failed %d, want 1/0", rep.RecoveredRequests, rep.FailedRequests)
	}
	if len(rep.Requests) != 1 || rep.Requests[0].Outcome != OutcomeRecovered {
		t.Fatalf("requests = %+v, want one recovered", rep.Requests)
	}
}

// Retries exhaust at WithMaxRetries(5), so a permanently overloaded provider
// produces six attempts under one logical request, numbered without a gap.
func TestObserverRecordsExhaustedRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After-Ms", "0") // keep the test fast; 0 is a valid hint
		w.WriteHeader(529)
		_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`)
	}))
	defer server.Close()

	c := NewRetryCollector()
	m := testMeta()
	_, err := ping(metaCtx(m), anthropicClient(server.URL, c, nil))
	if err == nil {
		t.Fatal("CompletionsWithCtx succeeded, want an error")
	}

	got := attemptsFor(t, c, m)
	if len(got) != 6 {
		t.Fatalf("got %d attempts, want 6 (1 + WithMaxRetries(5))", len(got))
	}
	for i, a := range got {
		if a.Number != i+1 {
			t.Errorf("attempt %d numbered %d", i+1, a.Number)
		}
		if a.Outcome != AttemptError || a.ErrorClass != ErrorClassOverloaded || a.StatusCode != 529 {
			t.Errorf("attempt %d = %+v, want overloaded 529 error", i+1, a)
		}
	}

	finalizeForTest(c, m, err)
	rep, freezeErr := c.Freeze("test-run-id")
	if freezeErr != nil {
		t.Fatalf("Freeze: %v", freezeErr)
	}
	if rep.TotalRetries != 5 || rep.FailedRequests != 1 || rep.RecoveredRequests != 0 {
		t.Errorf("aggregates = retries %d failed %d recovered %d, want 5/1/0",
			rep.TotalRetries, rep.FailedRequests, rep.RecoveredRequests)
	}
	if rep.Requests[0].Outcome != OutcomeFailed {
		t.Errorf("outcome = %s, want failed", rep.Requests[0].Outcome)
	}
}

// Statuses the SDK does not retry produce exactly one attempt. 402 is the case
// that pins the coarse provider bucket: it is told apart by status_code rather
// than by an enum that duplicates HTTP.
func TestObserverClassifiesTerminalStatuses(t *testing.T) {
	cases := []struct {
		status int
		want   ErrorClass
	}{
		{http.StatusUnauthorized, ErrorClassAuthentication},
		{http.StatusForbidden, ErrorClassAuthentication},
		{http.StatusPaymentRequired, ErrorClassProvider},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"nope"}}`)
			}))
			defer server.Close()

			c := NewRetryCollector()
			m := testMeta()
			_, err := ping(metaCtx(m), anthropicClient(server.URL, c, nil))
			if err == nil {
				t.Fatal("CompletionsWithCtx succeeded, want an error")
			}
			if n := requests.Load(); n != 1 {
				t.Errorf("server saw %d requests, want 1 (no retry)", n)
			}

			got := attemptsFor(t, c, m)
			if len(got) != 1 {
				t.Fatalf("got %d attempts, want 1", len(got))
			}
			if got[0].ErrorClass != tc.want || got[0].FailurePhase != FailurePhaseHTTP {
				t.Errorf("classified as %s/%s, want %s/http", got[0].ErrorClass, got[0].FailurePhase, tc.want)
			}
			if got[0].StatusCode != tc.status {
				t.Errorf("status_code = %d, want %d", got[0].StatusCode, tc.status)
			}
		})
	}
}

// Both SDKs consult x-should-retry before the status code and do not exclude
// 2xx, so a server can make them retry a successful response. The extra attempt
// is real and is reported, but nothing failed: the outcome is succeeded, and the
// summary legitimately shows a retry with zero recovered and zero failed.
func TestObserverRecordsRetryDirectiveOnSuccess(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			w.Header().Set("x-should-retry", "true")
			w.Header().Set("Retry-After-Ms", "0")
		}
		_, _ = fmt.Fprint(w, anthropicOKBody)
	}))
	defer server.Close()

	c := NewRetryCollector()
	m := testMeta()
	if _, err := ping(metaCtx(m), anthropicClient(server.URL, c, nil)); err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}

	got := attemptsFor(t, c, m)
	if len(got) != 2 {
		t.Fatalf("got %d attempts, want 2", len(got))
	}
	if got[0].Outcome != AttemptSuccess || got[1].Outcome != AttemptSuccess {
		t.Fatalf("attempts = %+v, want both success", got)
	}
	if got[0].SDKRetryDirective == nil || !*got[0].SDKRetryDirective {
		t.Errorf("attempt 1 sdk_retry_directive = %v, want true", got[0].SDKRetryDirective)
	}
	if got[1].SDKRetryDirective != nil {
		t.Errorf("attempt 2 sdk_retry_directive = %v, want absent", *got[1].SDKRetryDirective)
	}

	finalizeForTest(c, m, nil)
	rep, err := c.Freeze("test-run-id")
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if rep.RetriedRequests != 1 || rep.TotalRetries != 1 {
		t.Errorf("retried %d retries %d, want 1/1", rep.RetriedRequests, rep.TotalRetries)
	}
	if rep.RecoveredRequests != 0 || rep.FailedRequests != 0 {
		t.Errorf("recovered %d failed %d, want 0/0", rep.RecoveredRequests, rep.FailedRequests)
	}
	if rep.Requests[0].Outcome != OutcomeSucceeded {
		t.Errorf("outcome = %s, want succeeded", rep.Requests[0].Outcome)
	}
}

// A transport failure gives the observer no response at all: no status, no
// diagnostics, and a class derived from the Go error alone.
func TestObserverRecordsTransportFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close() // nothing is listening, so every attempt fails to connect

	c := NewRetryCollector()
	m := testMeta()
	// A refused connection sends no Retry-After, so the SDK falls back to its own
	// exponential backoff and five retries take ~15s. The deadline cuts the run
	// short during the first sleep; the attempts already recorded are the point.
	ctx, cancel := context.WithTimeout(metaCtx(m), 300*time.Millisecond)
	defer cancel()
	if _, err := ping(ctx, anthropicClient(url, c, nil)); err == nil {
		t.Fatal("CompletionsWithCtx succeeded, want a connection error")
	}

	got := attemptsFor(t, c, m)
	if len(got) == 0 {
		t.Fatal("no attempts recorded")
	}
	for i, a := range got {
		if a.StatusCode != 0 {
			t.Errorf("attempt %d status_code = %d, want 0 (no response)", i+1, a.StatusCode)
		}
		if a.ErrorClass != ErrorClassNetwork || a.FailurePhase != FailurePhaseTransport {
			t.Errorf("attempt %d = %s/%s, want network/transport", i+1, a.ErrorClass, a.FailurePhase)
		}
	}
}

// X-Stainless-Retry-Count is an SDK implementation detail. Overriding it through
// ExtraHeaders makes the SDK stop maintaining it (it only refreshes the header
// when it still reads "0"), so every attempt carries the same bogus value —
// which must change nothing about numbering, collection or Freeze.
func TestObserverIgnoresOverriddenRetryCountHeader(t *testing.T) {
	var seen []string
	var mu sync.Mutex
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("X-Stainless-Retry-Count"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			w.Header().Set("Retry-After-Ms", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
			return
		}
		_, _ = fmt.Fprint(w, anthropicOKBody)
	}))
	defer server.Close()

	c := NewRetryCollector()
	m := testMeta()
	extra := map[string]string{"X-Stainless-Retry-Count": "7"}
	if _, err := ping(metaCtx(m), anthropicClient(server.URL, c, extra)); err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for i, v := range seen {
		if v != "7" {
			t.Fatalf("request %d sent retry-count %q, want the overridden 7", i+1, v)
		}
	}

	got := attemptsFor(t, c, m)
	if len(got) != 2 || got[0].Number != 1 || got[1].Number != 2 {
		t.Fatalf("attempts = %+v, want two numbered 1 and 2", got)
	}
	finalizeForTest(c, m, nil)
	if _, err := c.Freeze("test-run-id"); err != nil {
		t.Fatalf("Freeze: %v", err)
	}
}

// Requests without identity — scan and llm test — are dropped whole. The
// assertion needs a second, identified request in the same collector: with only
// unidentified traffic there are no entries, Freeze returns (nil, nil), and
// total_requests cannot be observed at all.
func TestObserverDropsRequestsWithoutIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"nope"}}`)
	}))
	defer server.Close()

	c := NewRetryCollector()
	client := anthropicClient(server.URL, c, nil)

	// No RequestMeta on the context: this is what scan looks like.
	if _, err := ping(context.Background(), client); err == nil {
		t.Fatal("CompletionsWithCtx succeeded, want an error")
	}
	c.mu.Lock()
	entries := len(c.entries)
	c.mu.Unlock()
	if entries != 0 {
		t.Fatalf("collector holds %d entries after an unidentified request, want 0", entries)
	}

	m := testMeta()
	_, err := ping(metaCtx(m), client)
	if err == nil {
		t.Fatal("CompletionsWithCtx succeeded, want an error")
	}
	finalizeForTest(c, m, err)

	rep, freezeErr := c.Freeze("test-run-id")
	if freezeErr != nil {
		t.Fatalf("Freeze: %v", freezeErr)
	}
	if rep.TotalRequests != 1 || len(rep.Requests) != 1 {
		t.Errorf("total_requests %d, listed %d, want 1/1 — the unidentified request must not appear",
			rep.TotalRequests, len(rep.Requests))
	}
}

// --- the other two mount points ---

// The observer is mounted on all three clients, so the OpenAI Chat Completions
// and Responses constructors get the same 429-then-success check. The bodies
// differ; the observed sequence must not.
func TestObserverMountedOnOpenAIClients(t *testing.T) {
	cases := []struct {
		name    string
		okBody  string
		errBody string
		build   func(url string, c *RetryCollector) LLMClient
	}{
		{
			name:    "chat_completions",
			okBody:  openAIOKBody,
			errBody: `{"error":{"message":"slow down","type":"rate_limit_error"}}`,
			build: func(url string, c *RetryCollector) LLMClient {
				return NewOpenAIClient(ClientConfig{
					URL: url + "/v1", APIKey: "test-key", Model: "gpt-test", retryCollector: c,
				})
			},
		},
		{
			name:    "responses",
			okBody:  responsesOKBody,
			errBody: `{"error":{"message":"slow down","type":"rate_limit_error"}}`,
			build: func(url string, c *RetryCollector) LLMClient {
				return NewOpenAIResponsesClient(ClientConfig{
					URL: url + "/v1", APIKey: "test-key", Model: "gpt-test", retryCollector: c,
				})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if requests.Add(1) == 1 {
					// OpenAI reports its identifier under a different header
					// than Anthropic; the shared observer reads both.
					w.Header().Set("x-request-id", "req_openai")
					w.Header().Set("Retry-After-Ms", "0")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = fmt.Fprint(w, tc.errBody)
					return
				}
				_, _ = fmt.Fprint(w, tc.okBody)
			}))
			defer server.Close()

			c := NewRetryCollector()
			m := testMeta()
			if _, err := ping(metaCtx(m), tc.build(server.URL, c)); err != nil {
				t.Fatalf("CompletionsWithCtx: %v", err)
			}

			got := attemptsFor(t, c, m)
			if len(got) != 2 {
				t.Fatalf("got %d attempts, want 2", len(got))
			}
			if got[0].ErrorClass != ErrorClassRateLimited || got[0].StatusCode != http.StatusTooManyRequests {
				t.Errorf("attempt 1 = %+v, want rate_limited 429", got[0])
			}
			if got[0].RequestID != "req_openai" {
				t.Errorf("attempt 1 request_id = %q, want req_openai", got[0].RequestID)
			}
			if got[1].Outcome != AttemptSuccess {
				t.Errorf("attempt 2 = %+v, want success", got[1])
			}
		})
	}
}

// A nil collector must leave the request path exactly as it was: no middleware,
// no observation, and no change in behavior.
func TestNilCollectorMountsNoObserver(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, anthropicOKBody)
	}))
	defer server.Close()

	client := anthropicClient(server.URL, nil, nil)
	if _, err := ping(metaCtx(testMeta()), client); err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
}

// --- concurrency ---

// With --concurrency > 1 several files share one collector. Distinct metas must
// stay in distinct entries, and the frozen report must be stable. Run under
// -race, this is also the data-race assertion.
func TestObserverConcurrentRequests(t *testing.T) {
	var requests sync.Map
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Test-Request")
		n, _ := requests.LoadOrStore(key, new(atomic.Int32))
		w.Header().Set("Content-Type", "application/json")
		if n.(*atomic.Int32).Add(1) == 1 {
			w.Header().Set("Retry-After-Ms", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
			return
		}
		_, _ = fmt.Fprint(w, anthropicOKBody)
	}))
	defer server.Close()

	c := NewRetryCollector()
	const n = 8
	metas := make([]RequestMeta, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		m := testMeta()
		m.FilePath = fmt.Sprintf("file_%d.go", i)
		metas[i] = m
		client := anthropicClient(server.URL, c, map[string]string{"X-Test-Request": m.FilePath})
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ping(metaCtx(m), client); err != nil {
				t.Errorf("%s: %v", m.FilePath, err)
			}
		}()
	}
	wg.Wait()

	for _, m := range metas {
		if got := attemptsFor(t, c, m); len(got) != 2 {
			t.Errorf("%s recorded %d attempts, want 2", m.FilePath, len(got))
		}
		finalizeForTest(c, m, nil)
	}

	rep, err := c.Freeze("test-run-id")
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if rep.TotalRequests != n || rep.RecoveredRequests != n || rep.TotalRetries != n {
		t.Errorf("aggregates = total %d recovered %d retries %d, want %d each",
			rep.TotalRequests, rep.RecoveredRequests, rep.TotalRetries, n)
	}
	for i := 1; i < len(rep.Requests); i++ {
		if rep.Requests[i-1].LogicalRequestID >= rep.Requests[i].LogicalRequestID {
			t.Fatalf("requests not sorted by logical_request_id at %d", i)
		}
	}
}

// --- header parsing units ---

// Priority and units follow the SDKs' parseRetryAfterHeader exactly: Retry-After-Ms
// first and in milliseconds, then Retry-After as seconds, then Retry-After as an
// RFC1123 date. A hint that has already passed is not a negative wait.
func TestParseRetryAfterMS(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		headers map[string]string
		want    int64
	}{
		{name: "absent", headers: nil, want: 0},
		{name: "milliseconds", headers: map[string]string{"Retry-After-Ms": "1500"}, want: 1500},
		{name: "seconds", headers: map[string]string{"Retry-After": "3"}, want: 3000},
		{name: "fractional seconds", headers: map[string]string{"Retry-After": "0.5"}, want: 500},
		{
			name:    "milliseconds win over seconds",
			headers: map[string]string{"Retry-After-Ms": "250", "Retry-After": "9"},
			want:    250,
		},
		{
			name:    "unparsable milliseconds fall through to seconds",
			headers: map[string]string{"Retry-After-Ms": "soon", "Retry-After": "2"},
			want:    2000,
		},
		{
			name:    "rfc1123 date",
			headers: map[string]string{"Retry-After": now.Add(4 * time.Second).Format(time.RFC1123)},
			want:    4000,
		},
		{
			name:    "past rfc1123 date floors at zero",
			headers: map[string]string{"Retry-After": now.Add(-time.Hour).Format(time.RFC1123)},
			want:    0,
		},
		{name: "unparsable", headers: map[string]string{"Retry-After": "tomorrow"}, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range tc.headers {
				h.Set(k, v)
			}
			if got := parseRetryAfterMS(h, now); got != tc.want {
				t.Errorf("parseRetryAfterMS = %d, want %d", got, tc.want)
			}
		})
	}
}

// Only the two values the SDKs act on are recorded. Anything else is absent
// rather than coerced to false, because the SDK ignores it too and a false would
// claim the server said something it did not.
func TestParseRetryDirective(t *testing.T) {
	cases := []struct {
		value string
		want  *bool
	}{
		{value: "", want: nil},
		{value: "true", want: boolPtr(true)},
		{value: "false", want: boolPtr(false)},
		{value: "yes", want: nil},
	}
	for _, tc := range cases {
		t.Run("x-should-retry="+tc.value, func(t *testing.T) {
			h := http.Header{}
			if tc.value != "" {
				h.Set("x-should-retry", tc.value)
			}
			got := parseRetryDirective(h)
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("got %v, want absent", *got)
			case tc.want != nil && got == nil:
				t.Errorf("got absent, want %v", *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Errorf("got %v, want %v", *got, *tc.want)
			}
		})
	}
}

func TestResponseRequestID(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{name: "anthropic", headers: map[string]string{"request-id": "req_a"}, want: "req_a"},
		{name: "openai", headers: map[string]string{"x-request-id": "req_o"}, want: "req_o"},
		{
			name:    "anthropic wins when both present",
			headers: map[string]string{"request-id": "req_a", "x-request-id": "req_o"},
			want:    "req_a",
		},
		{name: "absent", headers: nil, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range tc.headers {
				h.Set(k, v)
			}
			if got := responseRequestID(h); got != tc.want {
				t.Errorf("responseRequestID = %q, want %q", got, tc.want)
			}
		})
	}
}

func boolPtr(v bool) *bool { return &v }

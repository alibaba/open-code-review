// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/packages/ssestream"
)

// The four blind spots share one shape: the observer sees HTTP 200, the failure
// surfaces afterwards, and nothing downstream would notice the omission —
// validateReport does not require an error attempt under outcome failed. So each
// path gets its own server and its own test rather than a shared fixture; a
// single table would let one missing correction hide behind another's coverage.

func openAIClient(url string, c *RetryCollector, extraBody map[string]any) *OpenAIClient {
	return NewOpenAIClient(ClientConfig{
		URL:            url + "/v1",
		APIKey:         "test-key",
		Model:          "gpt-test",
		ExtraBody:      extraBody,
		retryCollector: c,
	})
}

func responsesClient(url string, c *RetryCollector) *OpenAIResponsesClient {
	return NewOpenAIResponsesClient(ClientConfig{
		URL:            url + "/v1",
		APIKey:         "test-key",
		Model:          "gpt-test",
		retryCollector: c,
	})
}

// freezeOne freezes c and returns the single listed request.
func freezeOne(t *testing.T, c *RetryCollector) RequestReport {
	t.Helper()
	rep, err := c.Freeze("test-run-id")
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if rep == nil {
		t.Fatal("Freeze returned no report, want one")
	}
	if len(rep.Requests) != 1 {
		t.Fatalf("Freeze listed %d requests, want 1", len(rep.Requests))
	}
	return rep.Requests[0]
}

// --- blind spot 1: truncated body after HTTP 200 ---

// A body shorter than its Content-Length fails during the read that happens
// after the SDK's retry loop, so the SDK never retries it and the observer only
// ever saw the 200. This is also the three-attempt sequence from the design's
// timing diagram: 429, truncated 200, then the client's own re-call. It pins
// numbering across two SDK calls under one logical request, which is why the
// correction cannot assert "the first EOF corrects attempt 1".
func TestBoundaryCorrectsTruncatedResponse(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch requests.Add(1) {
		case 1:
			w.Header().Set("Retry-After-Ms", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"error":{"message":"slow down","type":"rate_limit_error"}}`)
		case 2:
			// Headers, then a pause, then fewer bytes than announced. The pause
			// is what makes attempt 3's observed_backoff_ms measurable: the
			// attempt ends when headers arrive, so the gap covers the stalled
			// body read plus the client-side re-call.
			w.Header().Set("Content-Length", "4096")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			time.Sleep(60 * time.Millisecond)
			_, _ = fmt.Fprint(w, openAIOKBody[:20])
		default:
			_, _ = fmt.Fprint(w, openAIOKBody)
		}
	}))
	defer server.Close()

	c := NewRetryCollector()
	m := testMeta()
	if _, err := ping(metaCtx(m), openAIClient(server.URL, c, nil)); err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}

	got := attemptsFor(t, c, m)
	if len(got) != 3 {
		t.Fatalf("got %d attempts, want 3 (429, truncated 200, re-call)", len(got))
	}
	for i, a := range got {
		if a.Number != i+1 {
			t.Errorf("attempt %d numbered %d — numbering must not restart with the second SDK call", i+1, a.Number)
		}
	}
	if got[0].ErrorClass != ErrorClassRateLimited {
		t.Errorf("attempt 1 = %s, want rate_limited", got[0].ErrorClass)
	}
	if got[1].Outcome != AttemptError || got[1].ErrorClass != ErrorClassNetwork || got[1].FailurePhase != FailurePhaseResponseDecode {
		t.Errorf("attempt 2 = %s %s/%s, want error network/response_decode",
			got[1].Outcome, got[1].ErrorClass, got[1].FailurePhase)
	}
	if got[1].StatusCode != http.StatusOK {
		t.Errorf("attempt 2 status_code = %d, want the observed 200 kept", got[1].StatusCode)
	}
	if got[2].Outcome != AttemptSuccess {
		t.Errorf("attempt 3 = %+v, want success", got[2])
	}
	// The re-call is not an SDK backoff, but the field only ever claimed to be a
	// measured interval. Fixed here so it is not "fixed" into a zero later.
	if got[2].ObservedBackoffMS < 30 {
		t.Errorf("attempt 3 observed_backoff_ms = %d, want >= 30 (the server stalled 60ms before truncating)",
			got[2].ObservedBackoffMS)
	}

	if req := freezeOne(t, c); req.Outcome != OutcomeRecovered {
		t.Errorf("outcome = %s, want recovered", req.Outcome)
	}
}

// The truncation correction is applied before the EOF branch consults ctx, so a
// request that ends cancelled still shows why the 200 was not usable. Nothing
// would flag the alternative: a cancelled request is not required to carry an
// error attempt, so the report would simply claim the truncated 200 was fine.
//
// The cancellation is placed in the re-call rather than between the two SDK
// calls: a body read that is racing a cancel returns whichever arrives first, so
// asserting on the gap itself could only ever be flaky.
func TestBoundaryKeepsTruncationCorrectionWhenRecallIsCancelled(t *testing.T) {
	var requests atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Length", "4096")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, openAIOKBody[:20])
			return
		}
		// Hold the re-call open until the test is done with it. An HTTP/1 server
		// does not cancel r.Context() while a handler is running, so waiting on
		// that instead would block Close forever.
		<-release
	}))
	defer server.Close()
	defer close(release)

	c := NewRetryCollector()
	m := testMeta()
	ctx, cancel := context.WithCancel(metaCtx(m))
	defer cancel()
	stop := time.AfterFunc(200*time.Millisecond, cancel)
	defer stop.Stop()

	_, err := ping(ctx, openAIClient(server.URL, c, nil))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CompletionsWithCtx err = %v, want context.Canceled", err)
	}

	got := attemptsFor(t, c, m)
	if len(got) < 1 {
		t.Fatal("no attempts recorded")
	}
	if got[0].Outcome != AttemptError || got[0].ErrorClass != ErrorClassNetwork || got[0].FailurePhase != FailurePhaseResponseDecode {
		t.Errorf("attempt 1 = %s %s/%s, want error network/response_decode kept through the cancellation",
			got[0].Outcome, got[0].ErrorClass, got[0].FailurePhase)
	}

	if req := freezeOne(t, c); req.Outcome != OutcomeCancelled {
		t.Errorf("outcome = %s, want cancelled", req.Outcome)
	}
}

// --- blind spot 2: any other decode failure after HTTP 200 ---

// A 200 whose body is not JSON at all fails in the SDK's post-retry-loop decode
// and arrives wrapped ("error parsing response json: %w"), so errors.As finds the
// *json.SyntaxError. The class is unknown rather than network: only the JSON
// error type is known here, and inferring more would mean reading the message.
func TestBoundaryCorrectsDecodeFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Content-Type must stay JSON: with anything else the SDK reports a
		// destination-type mismatch instead, which is a different error entirely.
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, "not json at all")
	}))
	defer server.Close()

	c := NewRetryCollector()
	m := testMeta()
	_, err := ping(metaCtx(m), anthropicClient(server.URL, c, nil))
	if err == nil {
		t.Fatal("CompletionsWithCtx succeeded, want a decode error")
	}

	got := attemptsFor(t, c, m)
	if len(got) != 1 {
		t.Fatalf("got %d attempts, want 1 (a decode failure is never retried)", len(got))
	}
	if got[0].Outcome != AttemptError || got[0].ErrorClass != ErrorClassUnknown || got[0].FailurePhase != FailurePhaseResponseDecode {
		t.Errorf("attempt 1 = %s %s/%s, want error unknown/response_decode",
			got[0].Outcome, got[0].ErrorClass, got[0].FailurePhase)
	}

	if req := freezeOne(t, c); req.Outcome != OutcomeFailed {
		t.Errorf("outcome = %s, want failed", req.Outcome)
	}
}

// --- blind spot 3: a stream that fails after it opened ---

// The stream opens with HTTP 200 and then ends without any choice reaching a
// finish_reason. OCR detects that itself, which is why the three integrity
// conditions carry a dedicated error type: a bare fmt.Errorf would be
// indistinguishable from every other error under "classify by Go type only".
func TestBoundaryCorrectsMidStreamFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\","+
			"\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":"+
			"{\"role\":\"assistant\",\"content\":\"ok\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	c := NewRetryCollector()
	m := testMeta()
	_, err := ping(metaCtx(m), openAIClient(server.URL, c, map[string]any{"stream": true}))
	if err == nil {
		t.Fatal("CompletionsWithCtx succeeded, want an incomplete-stream error")
	}
	var integrityErr *streamIntegrityError
	if !errors.As(err, &integrityErr) {
		t.Fatalf("err = %T, want *streamIntegrityError", err)
	}

	got := attemptsFor(t, c, m)
	if len(got) != 1 {
		t.Fatalf("got %d attempts, want 1", len(got))
	}
	if got[0].Outcome != AttemptError || got[0].ErrorClass != ErrorClassProvider || got[0].FailurePhase != FailurePhaseStream {
		t.Errorf("attempt 1 = %s %s/%s, want error provider/stream",
			got[0].Outcome, got[0].ErrorClass, got[0].FailurePhase)
	}

	if req := freezeOne(t, c); req.Outcome != OutcomeFailed {
		t.Errorf("outcome = %s, want failed", req.Outcome)
	}
}

// A stream that never opened needs no correction: stream.Err() carries the
// non-2xx *apierror.Error, the observer already recorded the attempt as an error,
// and ReviseLastAttempt's precondition makes the wrapper a no-op. Asserted so the
// precondition is not later "simplified" away on the streaming path.
func TestBoundaryKeepsHTTPClassOnStreamThatNeverOpened(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"error":{"message":"nope","type":"permission_error"}}`)
	}))
	defer server.Close()

	c := NewRetryCollector()
	m := testMeta()
	if _, err := ping(metaCtx(m), openAIClient(server.URL, c, map[string]any{"stream": true})); err == nil {
		t.Fatal("CompletionsWithCtx succeeded, want a 403 error")
	}

	got := attemptsFor(t, c, m)
	if len(got) != 1 {
		t.Fatalf("got %d attempts, want 1", len(got))
	}
	if got[0].ErrorClass != ErrorClassAuthentication || got[0].FailurePhase != FailurePhaseHTTP {
		t.Errorf("attempt 1 = %s/%s, want authentication/http kept, not rewritten to a stream failure",
			got[0].ErrorClass, got[0].FailurePhase)
	}
}

// --- blind spot 4: a Responses object that is not a success ---

// The Responses API answers HTTP 200 with the failure inside the object, so the
// SDK returns a nil Go error. Missing this correction is silent: the request is
// listed as failed with a single success attempt and no error_class at all —
// self-consistent counts over a record that misstates what happened.
func TestBoundaryCorrectsResponsesStatus(t *testing.T) {
	for _, status := range []string{"failed", "cancelled", "queued", "in_progress"} {
		t.Run(status, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"id":"resp_test","object":"response","model":"gpt-test",`+
					`"status":%q,"output":[]}`, status)
			}))
			defer server.Close()

			c := NewRetryCollector()
			m := testMeta()
			_, err := ping(metaCtx(m), responsesClient(server.URL, c))
			if err == nil {
				t.Fatalf("CompletionsWithCtx succeeded, want an error for status=%s", status)
			}

			got := attemptsFor(t, c, m)
			if len(got) != 1 {
				t.Fatalf("got %d attempts, want 1", len(got))
			}
			if got[0].Outcome != AttemptError || got[0].ErrorClass != ErrorClassProvider || got[0].FailurePhase != FailurePhaseResponseStatus {
				t.Errorf("attempt 1 = %s %s/%s, want error provider/response_status",
					got[0].Outcome, got[0].ErrorClass, got[0].FailurePhase)
			}
			if got[0].StatusCode != http.StatusOK {
				t.Errorf("attempt 1 status_code = %d, want the observed 200 kept", got[0].StatusCode)
			}

			if req := freezeOne(t, c); req.Outcome != OutcomeFailed {
				t.Errorf("outcome = %s, want failed", req.Outcome)
			}
		})
	}
}

// --- the counter-example: corrections must not overwrite an HTTP class ---

// A 5xx whose error body is corrupt makes the SDK return the raw JSON error
// instead of an *apierror.Error, so the boundary sees a decode failure for a
// request whose attempts are all classified from their status code. The status
// code is the stronger fact; a corrupt body only costs diagnostic richness.
func TestBoundaryKeepsHTTPClassOnCorruptErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After-Ms", "0")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, "not json at all")
	}))
	defer server.Close()

	c := NewRetryCollector()
	m := testMeta()
	_, err := ping(metaCtx(m), anthropicClient(server.URL, c, nil))
	if err == nil {
		t.Fatal("CompletionsWithCtx succeeded, want a 500 error")
	}

	got := attemptsFor(t, c, m)
	if len(got) != 6 {
		t.Fatalf("got %d attempts, want 6 (1 + WithMaxRetries(5))", len(got))
	}
	last := got[len(got)-1]
	if last.ErrorClass != ErrorClassProvider || last.FailurePhase != FailurePhaseHTTP {
		t.Errorf("last attempt = %s/%s, want provider/http — a corrupt error body must not rewrite it to unknown/response_decode",
			last.ErrorClass, last.FailurePhase)
	}
	if last.StatusCode != http.StatusInternalServerError {
		t.Errorf("last attempt status_code = %d, want 500", last.StatusCode)
	}

	if req := freezeOne(t, c); req.Outcome != OutcomeFailed {
		t.Errorf("outcome = %s, want failed", req.Outcome)
	}
}

// --- outcome is decided at Finalize, not read off the last attempt ---

// Cancelling while the SDK sleeps between attempts produces no new attempt, so
// the sequence still ends in a 429 error while the request outcome is cancelled.
// This is the case that makes inferring the outcome from the last attempt wrong.
func TestBoundaryCancelDuringBackoffIsCancelled(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		// The SDK honors a server hint verbatim, so this is long enough that the
		// cancel below always lands inside the wait rather than in an attempt.
		w.Header().Set("Retry-After-Ms", "3000")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
	}))
	defer server.Close()

	c := NewRetryCollector()
	m := testMeta()
	ctx, cancel := context.WithCancel(metaCtx(m))
	defer cancel()
	stop := time.AfterFunc(100*time.Millisecond, cancel)
	defer stop.Stop()

	_, err := ping(ctx, anthropicClient(server.URL, c, nil))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CompletionsWithCtx err = %v, want context.Canceled", err)
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("server saw %d requests, want 1 (the cancel lands in the backoff)", n)
	}

	got := attemptsFor(t, c, m)
	if len(got) != 1 {
		t.Fatalf("got %d attempts, want 1 — a cancelled wait must not invent an attempt", len(got))
	}
	if got[0].ErrorClass != ErrorClassRateLimited {
		t.Errorf("attempt 1 = %s, want the 429 classification untouched", got[0].ErrorClass)
	}

	rep, freezeErr := c.Freeze("test-run-id")
	if freezeErr != nil {
		t.Fatalf("Freeze: %v", freezeErr)
	}
	if rep.Requests[0].Outcome != OutcomeCancelled {
		t.Errorf("outcome = %s, want cancelled even though the last attempt is an HTTP error",
			rep.Requests[0].Outcome)
	}
	if rep.CancelledRequests != 1 || rep.FailedRequests != 0 || rep.RecoveredRequests != 0 {
		t.Errorf("cancelled %d failed %d recovered %d, want 1/0/0",
			rep.CancelledRequests, rep.FailedRequests, rep.RecoveredRequests)
	}
}

func TestBoundaryRetryAfterOutlivingAttemptTimeoutIsFailed(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After-Ms", "300")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
	}))
	defer server.Close()

	c := NewRetryCollector()
	m := testMeta()
	client := NewAnthropicClient(ClientConfig{
		URL:        server.URL + "/v1/messages",
		APIKey:     "test-key",
		Model:      "claude-test",
		AuthHeader: "x-api-key",
		// Reaches the SDK as WithRequestTimeout, which is per attempt and not a
		// budget for the logical request.
		Timeout:        200 * time.Millisecond,
		retryCollector: c,
	})

	_, err := ping(metaCtx(m), client)
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("CompletionsWithCtx err = %v, want the HTTP 429 error", err)
	}
	if n := requests.Load(); n != 6 {
		t.Errorf("server saw %d requests, want 6 — the hint is honored in full, so the SDK retries to MaxRetries", n)
	}

	got := attemptsFor(t, c, m)
	if len(got) != 6 {
		t.Fatalf("got %d attempts, want 6 — every retry is a real attempt", len(got))
	}
	for i, a := range got {
		if a.ErrorClass != ErrorClassRateLimited || a.FailurePhase != FailurePhaseHTTP {
			t.Errorf("attempt %d = %s/%s, want rate_limited/http kept — the boundary correction must no-op here",
				i+1, a.ErrorClass, a.FailurePhase)
		}
		if a.StatusCode != http.StatusTooManyRequests || a.RetryAfterMS != 300 {
			t.Errorf("attempt %d status %d retry_after_ms %d, want 429/300",
				i+1, a.StatusCode, a.RetryAfterMS)
		}
	}

	rep, freezeErr := c.Freeze("test-run-id")
	if freezeErr != nil {
		t.Fatalf("Freeze: %v", freezeErr)
	}
	if rep.Requests[0].Outcome != OutcomeFailed {
		t.Errorf("outcome = %s, want failed — an attempt deadline is not a user abort",
			rep.Requests[0].Outcome)
	}
	if rep.FailedRequests != 1 {
		t.Errorf("failed_requests = %d, want 1", rep.FailedRequests)
	}
}

// A parent deadline is not a user abort: only context.Canceled reaches
// parentCancelled, so an expired parent context is failed. Conflating the two
// would let the SDK's own per-attempt timeout report the run as cancelled.
func TestBoundaryDeadlineExceededIsFailed(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // outlive the parent deadline; see the note in the test above
	}))
	defer server.Close()
	defer close(release)

	c := NewRetryCollector()
	m := testMeta()
	ctx, cancel := context.WithTimeout(metaCtx(m), 150*time.Millisecond)
	defer cancel()

	_, err := ping(ctx, anthropicClient(server.URL, c, nil))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CompletionsWithCtx err = %v, want context.DeadlineExceeded", err)
	}

	got := attemptsFor(t, c, m)
	if len(got) != 1 {
		t.Fatalf("got %d attempts, want 1", len(got))
	}
	if got[0].ErrorClass != ErrorClassTimeout || got[0].FailurePhase != FailurePhaseContext {
		t.Errorf("attempt 1 = %s/%s, want timeout/context", got[0].ErrorClass, got[0].FailurePhase)
	}

	if req := freezeOne(t, c); req.Outcome != OutcomeFailed {
		t.Errorf("outcome = %s, want failed — DeadlineExceeded is not a cancellation", req.Outcome)
	}
}

// A request that fails before any HTTP attempt (here: unparsable tool call
// arguments) has no entry to finalize, so it is absent from the report rather
// than listed with an empty attempt list. As with the no-identity case, the
// assertion needs a second, real request: with nothing but zero-attempt requests
// the collector has no entries and Freeze returns (nil, nil).
func TestBoundarySkipsRequestWithoutAttempt(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	client := anthropicClient(server.URL, c, nil)

	empty := testMeta()
	empty.FilePath = "never-sent.go"
	_, err := client.CompletionsWithCtx(metaCtx(empty), ChatRequest{
		Messages: []Message{{
			Role:      "assistant",
			ToolCalls: []ToolCall{{ID: "call_1", Function: FunctionCall{Name: "read", Arguments: "{not json"}}},
		}},
	})
	if err == nil {
		t.Fatal("CompletionsWithCtx succeeded, want a parameter-building error")
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("server saw %d requests, want 0", n)
	}

	m := testMeta()
	if _, err := ping(metaCtx(m), client); err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}

	rep, freezeErr := c.Freeze("test-run-id")
	if freezeErr != nil {
		t.Fatalf("Freeze: %v", freezeErr)
	}
	if rep.TotalRequests != 1 || len(rep.Requests) != 1 {
		t.Errorf("total_requests %d, listed %d, want 1/1 — a request with no attempt must not appear",
			rep.TotalRequests, len(rep.Requests))
	}
	if rep.Requests[0].FilePath == empty.FilePath {
		t.Errorf("the zero-attempt request was listed")
	}
}

// --- classification units ---

// The false return is the contract: an error the boundary cannot recognize is
// left alone rather than bucketed as unknown, because the only remaining way to
// tell errors apart there is their message text.
func TestClassifyBoundaryError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantClass  ErrorClass
		wantPhase  FailurePhase
		recognized bool
	}{
		{name: "nil"},
		{
			name: "cancelled", err: fmt.Errorf("wrapped: %w", context.Canceled),
			wantClass: ErrorClassCancelled, wantPhase: FailurePhaseContext, recognized: true,
		},
		{
			name: "deadline", err: fmt.Errorf("wrapped: %w", context.DeadlineExceeded),
			wantClass: ErrorClassTimeout, wantPhase: FailurePhaseContext, recognized: true,
		},
		{
			name: "truncated body", err: fmt.Errorf("error reading response body: %w", io.ErrUnexpectedEOF),
			wantClass: ErrorClassNetwork, wantPhase: FailurePhaseResponseDecode, recognized: true,
		},
		{
			name: "json syntax", err: fmt.Errorf("error parsing response json: %w", &json.SyntaxError{}),
			wantClass: ErrorClassUnknown, wantPhase: FailurePhaseResponseDecode, recognized: true,
		},
		{
			name: "json type", err: fmt.Errorf("error parsing response json: %w", &json.UnmarshalTypeError{}),
			wantClass: ErrorClassUnknown, wantPhase: FailurePhaseResponseDecode, recognized: true,
		},
		{name: "opaque", err: errors.New("something went wrong")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			class, phase, recognized := classifyBoundaryError(tc.err)
			if recognized != tc.recognized {
				t.Fatalf("recognized = %v, want %v", recognized, tc.recognized)
			}
			if class != tc.wantClass || phase != tc.wantPhase {
				t.Errorf("got %s/%s, want %s/%s", class, phase, tc.wantClass, tc.wantPhase)
			}
		})
	}
}

// Unlike the boundary classifier this one always answers, because the caller
// already knows the failure came from an established stream. The fallback is
// unknown/stream rather than classifyAttempt's network/transport default: after
// HTTP 200 the error could just as easily come from decoding or the provider, so
// naming the transport would be an unverified claim.
func TestClassifyStreamError(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantClass ErrorClass
		wantPhase FailurePhase
	}{
		{
			name: "integrity", err: &streamIntegrityError{reason: "contained no choices"},
			wantClass: ErrorClassProvider, wantPhase: FailurePhaseStream,
		},
		{
			name: "sse stream error", err: fmt.Errorf("wrapped: %w", &ssestream.StreamError{}),
			wantClass: ErrorClassProvider, wantPhase: FailurePhaseStream,
		},
		{
			name: "cancelled keeps the context phase", err: fmt.Errorf("wrapped: %w", context.Canceled),
			wantClass: ErrorClassCancelled, wantPhase: FailurePhaseContext,
		},
		{
			name: "deadline keeps the context phase", err: fmt.Errorf("wrapped: %w", context.DeadlineExceeded),
			wantClass: ErrorClassTimeout, wantPhase: FailurePhaseContext,
		},
		{
			name: "opaque", err: errors.New("something went wrong"),
			wantClass: ErrorClassUnknown, wantPhase: FailurePhaseStream,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			class, phase := classifyStreamError(tc.err)
			if class != tc.wantClass || phase != tc.wantPhase {
				t.Errorf("got %s/%s, want %s/%s", class, phase, tc.wantClass, tc.wantPhase)
			}
		})
	}
}

// The three integrity conditions keep their exact rendered messages: the type
// exists to make them classifiable, not to change what a user reads.
func TestStreamIntegrityErrorMessage(t *testing.T) {
	err := &streamIntegrityError{reason: "contained no choices"}
	if got, want := err.Error(), "OpenAI streaming response contained no choices"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// A panic must still finalize, or the entry stays unfinalized and Freeze drops
// the whole run's report — one panicking file would erase every other file's
// retry records. No hostile server input makes the three clients panic, which is
// why the sentinel path is asserted here rather than through httptest.
func TestFinalizeRequestWithPanicSentinel(t *testing.T) {
	c := NewRetryCollector()
	m := testMeta()
	ctx := metaCtx(m)
	c.RecordAttempt(m, AttemptRecord{StatusCode: http.StatusOK}, time.Time{}, time.Time{})

	finalizeRequest(ctx, c, errRequestPanicked)

	if req := freezeOne(t, c); req.Outcome != OutcomeFailed {
		t.Errorf("outcome = %s, want failed", req.Outcome)
	}
}

// A nil collector and a context without identity are both no-ops, so no call
// site in the three clients needs to guard either.
func TestBoundaryHelpersAreInertWithoutCollectorOrMeta(t *testing.T) {
	m := testMeta()
	reviseAttempt(metaCtx(m), nil, ErrorClassNetwork, FailurePhaseResponseDecode)
	finalizeRequest(metaCtx(m), nil, errors.New("boom"))

	c := NewRetryCollector()
	reviseAttempt(context.Background(), c, ErrorClassNetwork, FailurePhaseResponseDecode)
	finalizeRequest(context.Background(), c, errors.New("boom"))

	c.mu.Lock()
	entries := len(c.entries)
	c.mu.Unlock()
	if entries != 0 {
		t.Errorf("collector holds %d entries, want 0", entries)
	}
}

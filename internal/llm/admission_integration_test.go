// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func gatedOpenAIClient(url string, limit int, c *RetryCollector) *OpenAIClient {
	return NewOpenAIClient(ClientConfig{
		URL:            url + "/v1",
		APIKey:         "test-key",
		Model:          "gpt-test",
		MaxInFlight:    limit,
		retryCollector: c,
	})
}

func scopedMetaCtx(m RequestMeta) context.Context {
	return ContextWithAdmissionScope(WithRequestMeta(context.Background(), m))
}

// TestAdmission_ClientCapsInFlight drives mixed concurrent requests through a
// real OpenAI client with max_in_flight=2 and asserts the server never sees
// more than 2 simultaneous attempts.
func TestAdmission_ClientCapsInFlight(t *testing.T) {
	var cur, max atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := cur.Add(1)
		for {
			m := max.Load()
			if n <= m || max.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond) // force overlap
		cur.Add(-1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, openAIOKBody)
	}))
	defer server.Close()

	client := gatedOpenAIClient(server.URL, 2, nil)
	const n = 8
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m := testMeta()
			m.FilePath = fmt.Sprintf("file_%d.go", i)
			if _, err := ping(scopedMetaCtx(m), client); err != nil {
				t.Errorf("request %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	if got := max.Load(); got > 2 {
		t.Fatalf("server observed %d concurrent attempts, want <= 2", got)
	}
	if got := max.Load(); got < 2 {
		t.Fatalf("server observed %d concurrent attempts — test never exercised the cap", got)
	}
}

// pingBody sends a chat request whose first message content identifies the
// request, so one shared client (one shared gate) can drive distinguishable
// requests.
func pingBody(ctx context.Context, client LLMClient, content string) (*ChatResponse, error) {
	return client.CompletionsWithCtx(ctx, ChatRequest{
		Messages:  []Message{{Role: "user", Content: content}},
		MaxTokens: 64,
	})
}

// bodyContent extracts messages[0].content for request identity.
func bodyContent(t *testing.T, r *http.Request) string {
	t.Helper()
	var body struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return ""
	}
	if len(body.Messages) == 0 {
		return ""
	}
	return body.Messages[0].Content
}

// TestAdmission_RetryReleasesBeforeBackoff asserts a retryable response frees
// the permit before the SDK's backoff sleep: while request A backs off, request
// B reaches the server through the SAME gate. The arrival log makes the
// assertion order-based, not timing-based. Both retryable statuses the issue
// names are covered: 429 and 529.
func TestAdmission_RetryReleasesBeforeBackoff(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{
			name:   "429 rate limited",
			status: http.StatusTooManyRequests,
			body:   `{"error":{"message":"slow down","type":"rate_limit_error"}}`,
		},
		{
			name:   "529 overloaded",
			status: 529,
			body:   `{"error":{"message":"overloaded","type":"overloaded_error"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			arrivals := []string{}
			aFirstDone := make(chan struct{})
			var aAttempts atomic.Int32

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id := bodyContent(t, r)
				mu.Lock()
				arrivals = append(arrivals, id)
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")

				if id == "A" {
					if aAttempts.Add(1) == 1 {
						w.Header().Set("Retry-After-Ms", "400")
						w.WriteHeader(tc.status)
						_, _ = fmt.Fprint(w, tc.body)
						close(aFirstDone)
						return
					}
				}
				_, _ = fmt.Fprint(w, openAIOKBody)
			}))
			defer server.Close()

			c := NewRetryCollector()
			client := NewOpenAIClient(ClientConfig{
				URL:            server.URL + "/v1",
				APIKey:         "test-key",
				Model:          "gpt-test",
				MaxInFlight:    1,
				retryCollector: c,
			})

			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				if _, err := pingBody(ContextWithAdmissionScope(context.Background()), client, "A"); err != nil {
					t.Errorf("A: %v", err)
				}
			}()
			go func() {
				defer wg.Done()
				<-aFirstDone // A's retryable attempt has released its permit; A now sleeps in backoff.
				if _, err := pingBody(ContextWithAdmissionScope(context.Background()), client, "B"); err != nil {
					t.Errorf("B: %v", err)
				}
			}()
			wg.Wait()

			mu.Lock()
			defer mu.Unlock()
			// Order must be: A(retryable), B, A(success). If the permit were
			// held through backoff, B could only arrive after A's second attempt.
			want := []string{"A", "B", "A"}
			if len(arrivals) != len(want) {
				t.Fatalf("arrivals = %v, want %v", arrivals, want)
			}
			for i := range want {
				if arrivals[i] != want[i] {
					t.Fatalf("arrivals = %v, want %v (permit not released before backoff)", arrivals, want)
				}
			}
		})
	}
}

// TestAdmission_StreamingBodyHoldsPermit asserts a streaming response keeps
// its permit until the stream has been fully consumed: streaming request B
// reaches the server only after streaming request A's SSE stream ended, via
// the same single-permit gate.
func TestAdmission_StreamingBodyHoldsPermit(t *testing.T) {
	var mu sync.Mutex
	events := []string{}
	// aArrived is closed when the server sees A's request, i.e. after A has
	// acquired the permit — so B provably queues behind A's held permit with
	// no sleep-based race for who acquires first.
	aArrived := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := bodyContent(t, r)
		if id == "A" {
			close(aArrived)
			writeOpenAISSE(t, w,
				`{"id":"s","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant","content":"hel"},"finish_reason":null}]}`,
				`{"id":"s","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":null}]}`,
				`{"id":"s","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			)
			mu.Lock()
			events = append(events, "A-stream-sent")
			mu.Unlock()
			return
		}
		mu.Lock()
		events = append(events, "B-arrived")
		mu.Unlock()
		writeOpenAISSE(t, w,
			`{"id":"s","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":null}]}`,
			`{"id":"s","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		)
	}))
	defer server.Close()

	// One client: one gate, and stream:true applies to both requests.
	client := NewOpenAIClient(ClientConfig{
		URL:            server.URL + "/v1",
		APIKey:         "test-key",
		Model:          "gpt-test",
		MaxInFlight:    1,
		ExtraBody:      map[string]any{"stream": true},
		retryCollector: NewRetryCollector(),
	})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := pingBody(ContextWithAdmissionScope(context.Background()), client, "A"); err != nil {
			t.Errorf("A: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		<-aArrived // A owns the single permit from here until its stream ends
		if _, err := pingBody(ContextWithAdmissionScope(context.Background()), client, "B"); err != nil {
			t.Errorf("B: %v", err)
		}
	}()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 || events[0] != "A-stream-sent" || events[1] != "B-arrived" {
		t.Fatalf("events = %v, want [A-stream-sent B-arrived] (stream must hold its permit)", events)
	}
}

// TestAdmission_DeadlineWhileQueued documents the deadline interaction: gate
// waiting consumes the request's timeout budget, so a queued request whose
// deadline expires fails without any transport attempt and without leaking a
// permit.
func TestAdmission_DeadlineWhileQueued(t *testing.T) {
	releaseA := make(chan struct{})
	aArrived := make(chan struct{})
	var bArrivals atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bodyContent(t, r) == "A" {
			close(aArrived) // A holds the permit from here until releaseA
			<-releaseA
		} else {
			bArrivals.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, openAIOKBody)
	}))
	defer server.Close()

	client := gatedOpenAIClient(server.URL, 1, nil)

	aDone := make(chan error, 1)
	go func() {
		_, err := pingBody(ContextWithAdmissionScope(context.Background()), client, "A")
		aDone <- err
	}()
	// A provably owns the single permit once the server has seen its request.
	<-aArrived

	ctx, cancel := context.WithTimeout(ContextWithAdmissionScope(context.Background()), 80*time.Millisecond)
	defer cancel()
	_, err := pingBody(ctx, client, "B")
	if err == nil {
		t.Fatal("queued request unexpectedly succeeded before the permit freed")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if got := bArrivals.Load(); got != 0 {
		t.Fatalf("B reached the transport %d times despite never being admitted", got)
	}

	close(releaseA)
	if err := <-aDone; err != nil {
		t.Fatalf("A: %v", err)
	}
}

// TestAdmission_UngatedWithoutScope asserts scan-style and llm-test-style
// requests bypass the gate entirely: with max_in_flight=1, three unscoped
// requests still overlap at the server.
func TestAdmission_UngatedWithoutScope(t *testing.T) {
	const n = 3
	arrived := make(chan struct{}, n)
	release := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived <- struct{}{}
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, openAIOKBody)
	}))
	defer server.Close()

	client := gatedOpenAIClient(server.URL, 1, nil)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ping(context.Background(), client); err != nil {
				t.Errorf("unscoped request: %v", err)
			}
		}()
	}
	// All n must be at the server simultaneously; if any were gated, this
	// drain would hang waiting for the single permit.
	for i := 0; i < n; i++ {
		select {
		case <-arrived:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d unscoped requests reached the server; the gate held one back", i)
		}
	}
	close(release) // unblock the handlers, then join
	wg.Wait()
}

// TestAdmission_TimingExcludesGateWait asserts the mount ordering keeps
// duration_to_headers_ms measuring transport only: request B queues behind a
// slow A, so B's wall-clock wait is large, but its duration_to_headers_ms
// stays small.
func TestAdmission_TimingExcludesGateWait(t *testing.T) {
	var mu sync.Mutex
	arrivals := []string{}
	aFirst := make(chan struct{})
	var aAttempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := bodyContent(t, r)
		mu.Lock()
		arrivals = append(arrivals, id)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if id == "A" {
			if aAttempts.Add(1) == 1 {
				close(aFirst)
				time.Sleep(250 * time.Millisecond) // slow transport: B queues at the gate meanwhile
			}
		}
		_, _ = fmt.Fprint(w, openAIOKBody)
	}))
	defer server.Close()

	c := NewRetryCollector()
	client := NewOpenAIClient(ClientConfig{
		URL:            server.URL + "/v1",
		APIKey:         "test-key",
		Model:          "gpt-test",
		MaxInFlight:    1,
		retryCollector: c,
	})

	aDone := make(chan error, 1)
	go func() {
		_, err := pingBody(ContextWithAdmissionScope(context.Background()), client, "A")
		aDone <- err
	}()
	<-aFirst

	bMeta := testMeta()
	bMeta.FilePath = "b.go"
	bCtx := ContextWithAdmissionScope(WithRequestMeta(context.Background(), bMeta))
	startedAt := time.Now()
	if _, err := pingBody(bCtx, client, "B"); err != nil {
		t.Fatalf("B: %v", err)
	}
	queuedFor := time.Since(startedAt)
	if err := <-aDone; err != nil {
		t.Fatalf("A: %v", err)
	}

	attempts := attemptsFor(t, c, bMeta)
	if len(attempts) != 1 {
		t.Fatalf("B recorded %d attempts, want 1", len(attempts))
	}
	// B waited at the gate for most of A's transport...
	if queuedFor < 150*time.Millisecond {
		t.Fatalf("B queued for only %v — test never exercised admission waiting", queuedFor)
	}
	// ...yet its headers duration measures only its own transport.
	if got := time.Duration(attempts[0].DurationToHeadersMS) * time.Millisecond; got >= 150*time.Millisecond {
		t.Fatalf("B duration_to_headers_ms = %v includes admission waiting (queued %v)", got, queuedFor)
	}
}

// TestAdmission_LimitOneForegroundAndBackgroundBothComplete exercises the
// limit=1 deadlock edge: a foreground request completes while a
// compression-style background request (WithoutCancel context) queues behind
// it — both must finish and the gate must drain.
func TestAdmission_LimitOneForegroundAndBackgroundBothComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, openAIOKBody)
	}))
	defer server.Close()

	client := gatedOpenAIClient(server.URL, 1, nil)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := ping(ContextWithAdmissionScope(context.Background()), client); err != nil {
			t.Errorf("foreground: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		// Compression-style: the parent is cancelled, the child is not.
		parent, cancel := context.WithCancel(ContextWithAdmissionScope(context.Background()))
		bg := context.WithoutCancel(parent)
		cancel()
		if _, err := ping(bg, client); err != nil {
			t.Errorf("background: %v", err)
		}
	}()
	wg.Wait()
}

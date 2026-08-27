// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	oteltrace "go.opentelemetry.io/otel/trace"
)

// recordingRawWriter collects records for assertions.
type recordingRawWriter struct {
	mu      sync.Mutex
	records []RawRecord
	closed  bool
}

func (w *recordingRawWriter) Write(rec RawRecord) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.records = append(w.records, rec)
}

func (w *recordingRawWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

func (w *recordingRawWriter) one(t *testing.T) RawRecord {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.records) != 1 {
		t.Fatalf("recorded %d records, want 1", len(w.records))
	}
	return w.records[0]
}

func TestRawLoggingEnabled(t *testing.T) {
	t.Setenv(rawLoggingEnv, "")
	if RawLoggingEnabled() {
		t.Error("unset env reported enabled")
	}
	t.Setenv(rawLoggingEnv, "true")
	if RawLoggingEnabled() {
		t.Error("non-\"1\" value reported enabled")
	}
	t.Setenv(rawLoggingEnv, "1")
	if !RawLoggingEnabled() {
		t.Error("\"1\" reported disabled")
	}
}

// rawRequest builds an *http.Request with the given body and context for
// middleware tests.
func rawRequest(t *testing.T, ctx context.Context, body string, headers map[string]string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://llm.example.com/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestRawMiddleware_CapturesRawBodiesAndMeta(t *testing.T) {
	meta := RequestMeta{
		Provider:  "openai",
		Model:     "gpt-test",
		FilePath:  "pkg/foo.go",
		TaskType:  "main_task",
		RequestNo: 3,
	}
	ctx := WithRequestMeta(context.Background(), meta)
	reqBody := `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`
	respBody := `{"id":"chatcmpl-1","choices":[]}`
	req := rawRequest(t, ctx, reqBody, map[string]string{"Content-Type": "application/json"})

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	resp, err := mw(req, func(r *http.Request) (*http.Response, error) {
		// The SDK must still be able to read the request body after capture.
		got, _ := io.ReadAll(r.Body)
		if string(got) != reqBody {
			t.Errorf("request body after capture = %q, want %q", got, reqBody)
		}
		return jsonResponse(respBody), nil
	})
	if err != nil {
		t.Fatalf("middleware: %v", err)
	}
	// The SDK must still be able to read the response body after capture.
	gotResp, _ := io.ReadAll(resp.Body)
	if string(gotResp) != respBody {
		t.Errorf("response body after capture = %q, want %q", gotResp, respBody)
	}

	rec := tw.one(t)
	if rec.RequestID == "" {
		t.Error("request_id empty")
	}
	if rec.Timestamp == "" {
		t.Error("timestamp empty")
	}
	if rec.FilePath != "pkg/foo.go" || rec.TaskType != "main_task" || rec.RequestNo != 3 {
		t.Errorf("identity = (%q,%q,%d), want (pkg/foo.go,main_task,3)", rec.FilePath, rec.TaskType, rec.RequestNo)
	}
	if rec.Model != "gpt-test" {
		t.Errorf("model = %q, want gpt-test", rec.Model)
	}
	if string(rec.Request) != reqBody {
		t.Errorf("request = %q, want raw body", rec.Request)
	}
	if string(rec.Response) != respBody {
		t.Errorf("response = %q, want raw body", rec.Response)
	}
	if rec.ResponseText != "" || rec.Error != "" {
		t.Errorf("unexpected response_text=%q error=%q", rec.ResponseText, rec.Error)
	}
	if rec.DurationMs < 0 {
		t.Errorf("duration_ms = %d, want >= 0", rec.DurationMs)
	}
}

// slowBody sleeps on its first Read so a test can prove duration_ms covers
// the full body capture, not just the next() round trip.
type slowBody struct {
	delay time.Duration
	once  bool
}

func (b *slowBody) Read(p []byte) (int, error) {
	if !b.once {
		b.once = true
		time.Sleep(b.delay)
		return copy(p, `{}`), nil
	}
	return 0, io.EOF
}

func (b *slowBody) Close() error { return nil }

func TestRawMiddleware_DurationCoversBodyRead(t *testing.T) {
	req := rawRequest(t, context.Background(), `{}`, nil)

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	const delay = 30 * time.Millisecond
	if _, err := mw(req, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: &slowBody{delay: delay}}, nil
	}); err != nil {
		t.Fatalf("middleware: %v", err)
	}

	rec := tw.one(t)
	if rec.DurationMs < delay.Milliseconds() {
		t.Errorf("duration_ms = %d, want >= %d (must cover the body read)", rec.DurationMs, delay.Milliseconds())
	}
}

func TestRawMiddleware_RedactsAuthHeaders(t *testing.T) {
	req := rawRequest(t, context.Background(), `{}`, map[string]string{
		"Authorization":        "Bearer super-secret",
		"X-Api-Key":            "sk-live-secret",
		"API-KEY":              "another-secret",
		"X-Amz-Security-Token": "aws-session-secret",
		"X-Session-Token":      "session-secret",
		"Proxy-Authorization":  "Basic secret",
		"User-Agent":           "open-code-review/test",
	})

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	if _, err := mw(req, func(*http.Request) (*http.Response, error) { return jsonResponse(`{}`), nil }); err != nil {
		t.Fatalf("middleware: %v", err)
	}

	rec := tw.one(t)
	for k, v := range rec.Headers {
		if sensitiveHeader(k) && v != "[REDACTED]" {
			t.Errorf("header %s = %q, want [REDACTED]", k, v)
		}
	}
	if got := rec.Headers["User-Agent"]; got != "open-code-review/test" {
		t.Errorf("User-Agent = %q, want passthrough value", got)
	}
	for _, v := range rec.Headers {
		if strings.Contains(v, "secret") {
			t.Errorf("secret leaked in headers: %v", rec.Headers)
		}
	}
}

func TestSensitiveHeader(t *testing.T) {
	redact := []string{
		"Authorization", "authorization",
		"X-Api-Key", "API-KEY",
		"X-Amz-Security-Token", "Proxy-Authorization",
		"x-auth-token", "X-Session-Token", "x-signing-key",
		"X-Client-Secret", "X-Credential-Provider",
		"Cookie", "x-session-cookie",
	}
	for _, h := range redact {
		if !sensitiveHeader(h) {
			t.Errorf("sensitiveHeader(%q) = false, want redacted", h)
		}
	}
	pass := []string{"User-Agent", "Content-Type", "Accept", "X-Request-ID", "X-Stainless-Lang", "Anthropic-Version"}
	for _, h := range pass {
		if sensitiveHeader(h) {
			t.Errorf("sensitiveHeader(%q) = true, want passthrough", h)
		}
	}
}

func TestRawMiddleware_TransportErrorRecordsErrorOnly(t *testing.T) {
	req := rawRequest(t, context.Background(), `{"model":"m"}`, nil)

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	wantErr := errors.New("connection reset")
	resp, err := mw(req, func(*http.Request) (*http.Response, error) { return nil, wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if resp != nil {
		t.Errorf("resp = %v, want nil", resp)
	}

	rec := tw.one(t)
	if rec.Error != "connection reset" {
		t.Errorf("error = %q, want transport error", rec.Error)
	}
	if rec.Response != nil || rec.ResponseText != "" {
		t.Errorf("error record must carry no response: %q / %q", rec.Response, rec.ResponseText)
	}
}

func TestRawMiddleware_NonJSONResponseGoesToResponseText(t *testing.T) {
	// SSE bodies (extra_body.stream=true) are not JSON; stuffing them into a
	// json.RawMessage field would emit a malformed JSONL line.
	sse := "data: {\"delta\":\"a\"}\n\ndata: {\"delta\":\"b\"}\n\ndata: [DONE]\n\n"
	req := rawRequest(t, context.Background(), `{}`, nil)

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	resp, err := mw(req, func(*http.Request) (*http.Response, error) { return jsonResponse(sse), nil })
	if err != nil {
		t.Fatalf("middleware: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != sse {
		t.Errorf("SSE body after capture = %q, want unchanged", got)
	}

	rec := tw.one(t)
	if rec.Response != nil {
		t.Errorf("response = %q, want empty for non-JSON body", rec.Response)
	}
	if rec.ResponseText != sse {
		t.Errorf("response_text = %q, want SSE body", rec.ResponseText)
	}
}

// brokenBody yields partial data once, then a read error, tracking Close.
type brokenBody struct {
	partial []byte
	err     error
	sent    bool
	closed  bool
}

func (b *brokenBody) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true
		return copy(p, b.partial), nil
	}
	return 0, b.err
}

func (b *brokenBody) Close() error {
	b.closed = true
	return nil
}

func TestRawMiddleware_BodyReadErrorPropagates(t *testing.T) {
	req := rawRequest(t, context.Background(), `{"model":"m"}`, nil)
	readErr := errors.New("connection reset by peer")
	body := &brokenBody{partial: []byte(`{"partial":`), err: readErr}

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	resp, err := mw(req, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	})
	if err != nil {
		t.Fatalf("middleware: %v", err)
	}
	if !body.closed {
		t.Error("original response body not closed")
	}

	// The SDK must see the same failure it would without capture: the buffered
	// partial bytes followed by the original read error.
	got, gotErr := io.ReadAll(resp.Body)
	if string(got) != `{"partial":` {
		t.Errorf("body after capture = %q, want partial bytes", got)
	}
	if !errors.Is(gotErr, readErr) {
		t.Errorf("body read error = %v, want %v", gotErr, readErr)
	}

	rec := tw.one(t)
	if rec.Error != readErr.Error() {
		t.Errorf("error = %q, want %q", rec.Error, readErr.Error())
	}
	if rec.ResponseText != `{"partial":` {
		t.Errorf("response_text = %q, want partial bytes", rec.ResponseText)
	}
	if rec.Response != nil {
		t.Errorf("response = %q, want empty on read error", rec.Response)
	}
}

func TestRawMiddleware_NoMetaOmitsIdentityAndFallsBackToBodyModel(t *testing.T) {
	req := rawRequest(t, context.Background(), `{"model":"body-model"}`, nil)

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	if _, err := mw(req, func(*http.Request) (*http.Response, error) { return jsonResponse(`{}`), nil }); err != nil {
		t.Fatalf("middleware: %v", err)
	}

	rec := tw.one(t)
	if rec.FilePath != "" || rec.TaskType != "" || rec.RequestNo != 0 {
		t.Errorf("identity must be omitted without meta: (%q,%q,%d)", rec.FilePath, rec.TaskType, rec.RequestNo)
	}
	if rec.Model != "body-model" {
		t.Errorf("model = %q, want fallback from request body", rec.Model)
	}
}

// TestRawMiddleware_TraceID covers the run-level join key: a valid OTel span
// context on the request ctx is captured as trace_id, and without one the
// field is omitted from the JSON entirely so the raw channel stays usable
// with telemetry off.
func TestRawMiddleware_TraceID(t *testing.T) {
	tid, err := oteltrace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("TraceIDFromHex: %v", err)
	}
	sid, err := oteltrace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("SpanIDFromHex: %v", err)
	}
	spanCtx := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{TraceID: tid, SpanID: sid})
	ctx := oteltrace.ContextWithRemoteSpanContext(context.Background(), spanCtx)

	run := func(ctx context.Context) RawRecord {
		t.Helper()
		req := rawRequest(t, ctx, `{"model":"m"}`, nil)
		tw := &recordingRawWriter{}
		holder := NewRawHolder()
		holder.Set(tw)
		mw := newRawMiddleware(holder)
		if _, err := mw(req, func(*http.Request) (*http.Response, error) { return jsonResponse(`{}`), nil }); err != nil {
			t.Fatalf("middleware: %v", err)
		}
		return tw.one(t)
	}

	rec := run(ctx)
	if rec.TraceID != tid.String() {
		t.Errorf("trace_id = %q, want %q", rec.TraceID, tid.String())
	}

	// Telemetry off: no span context, and the field must vanish from the JSON.
	rec = run(context.Background())
	if rec.TraceID != "" {
		t.Errorf("trace_id = %q, want omitted without span context", rec.TraceID)
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "trace_id") {
		t.Errorf("trace_id present in JSON without span context: %s", data)
	}
}

func TestRawMiddleware_NoWriterIsPassthrough(t *testing.T) {
	req := rawRequest(t, context.Background(), `{}`, nil)
	nextCalled := false
	mw := newRawMiddleware(NewRawHolder())

	resp, err := mw(req, func(*http.Request) (*http.Response, error) {
		nextCalled = true
		return jsonResponse(`{}`), nil
	})
	if err != nil || !nextCalled || resp == nil {
		t.Fatalf("passthrough broken: err=%v nextCalled=%v resp=%v", err, nextCalled, resp)
	}
}

// TestNewLLMClient_RawEndToEnd drives a real OpenAI client built through
// NewLLMClient against a fake endpoint and asserts one raw record lands in the
// bound writer, carrying the raw bodies verbatim.
func TestNewLLMClient_RawEndToEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"model":"gpt-test",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	defer server.Close()

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	client := NewLLMClient(ResolvedEndpoint{
		URL:      server.URL + "/v1",
		Token:    "test-key",
		Model:    "gpt-test",
		Protocol: ProtocolOpenAIChatCompletions,
		ExtraBody: map[string]any{
			"raw_marker": "raw-capture",
		},
	}, nil, holder)
	holder.Set(tw)

	meta := RequestMeta{
		Provider:  "openai",
		Model:     "gpt-test",
		FilePath:  "a/b.go",
		TaskType:  "main_task",
		RequestNo: 1,
	}
	ctx := WithRequestMeta(context.Background(), meta)
	if _, err := client.CompletionsWithCtx(ctx, ChatRequest{
		Messages:  []Message{{Role: "user", Content: "ping"}},
		MaxTokens: 64,
	}); err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}

	rec := tw.one(t)
	if rec.FilePath != "a/b.go" || rec.TaskType != "main_task" || rec.RequestNo != 1 {
		t.Errorf("identity = (%q,%q,%d), want meta values", rec.FilePath, rec.TaskType, rec.RequestNo)
	}
	// The captured request must be the raw body — extra_body merged in.
	var reqMap map[string]any
	if err := json.Unmarshal(rec.Request, &reqMap); err != nil {
		t.Fatalf("request is not valid JSON: %v", err)
	}
	if reqMap["raw_marker"] != "raw-capture" {
		t.Errorf("raw request lacks merged extra_body: %v", reqMap)
	}
	if reqMap["model"] != "gpt-test" {
		t.Errorf("raw request model = %v, want gpt-test", reqMap["model"])
	}
	// The captured response must be the raw completion JSON.
	var respMap map[string]any
	if err := json.Unmarshal(rec.Response, &respMap); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if respMap["id"] != "chatcmpl-test" {
		t.Errorf("raw response id = %v, want chatcmpl-test", respMap["id"])
	}
	if rec.Headers["Authorization"] != "[REDACTED]" {
		t.Errorf("Authorization = %q, want [REDACTED]", rec.Headers["Authorization"])
	}
}

// TestNewLLMClient_RawRecordsEveryRetryAttempt asserts the middleware sits
// inside the SDK retry loop: a retried request yields one record per real HTTP
// attempt, sharing identity but with distinct request_id values.
func TestNewLLMClient_RawRecordsEveryRetryAttempt(t *testing.T) {
	const successBody = `{
		"id":"chatcmpl-ok","object":"chat.completion","model":"gpt-test",
		"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
	}`
	const errorBody = `{"error":{"message":"boom","type":"server_error"}}`

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(errorBody))
			return
		}
		_, _ = w.Write([]byte(successBody))
	}))
	defer server.Close()

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	client := NewLLMClient(ResolvedEndpoint{
		URL:      server.URL + "/v1",
		Token:    "test-key",
		Model:    "gpt-test",
		Protocol: ProtocolOpenAIChatCompletions,
	}, nil, holder)
	holder.Set(tw)

	meta := RequestMeta{Provider: "openai", Model: "gpt-test", FilePath: "a/b.go", TaskType: "main_task", RequestNo: 1}
	ctx := WithRequestMeta(context.Background(), meta)
	if _, err := client.CompletionsWithCtx(ctx, ChatRequest{
		Messages:  []Message{{Role: "user", Content: "ping"}},
		MaxTokens: 64,
	}); err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}

	tw.mu.Lock()
	recs := append([]RawRecord(nil), tw.records...)
	tw.mu.Unlock()
	if len(recs) != 2 {
		t.Fatalf("recorded %d records, want 2 (one per HTTP attempt)", len(recs))
	}
	if recs[0].RequestID == "" || recs[0].RequestID == recs[1].RequestID {
		t.Errorf("request_id must differ per attempt: %q vs %q", recs[0].RequestID, recs[1].RequestID)
	}
	if !bytes.Equal(recs[0].Request, recs[1].Request) {
		t.Errorf("replayed request bodies differ:\n%s\n%s", recs[0].Request, recs[1].Request)
	}
	for i, rec := range recs {
		if rec.FilePath != "a/b.go" || rec.TaskType != "main_task" || rec.RequestNo != 1 {
			t.Errorf("attempt %d identity = (%q,%q,%d), want meta values", i+1, rec.FilePath, rec.TaskType, rec.RequestNo)
		}
	}
	if string(recs[0].Response) != errorBody {
		t.Errorf("attempt 1 response = %q, want the 500 body", recs[0].Response)
	}
	if string(recs[1].Response) != successBody {
		t.Errorf("attempt 2 response = %q, want the success body", recs[1].Response)
	}
}

// TestNewLLMClient_NoRawHolderMountsNothing asserts the default (switch off)
// path records nothing and behaves exactly as before.
func TestNewLLMClient_NoRawHolderMountsNothing(t *testing.T) {
	var reqCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test","object":"chat.completion","model":"gpt-test",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	defer server.Close()

	client := NewLLMClient(ResolvedEndpoint{
		URL:      server.URL + "/v1",
		Token:    "test-key",
		Model:    "gpt-test",
		Protocol: ProtocolOpenAIChatCompletions,
	}, nil, nil)

	if _, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages:  []Message{{Role: "user", Content: "ping"}},
		MaxTokens: 64,
	}); err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	if reqCount != 1 {
		t.Errorf("server saw %d requests, want 1", reqCount)
	}
}

// TestRawMiddleware_EmptyBodies guards the edges: nil request body and nil
// response body must not panic and must yield empty captures.
func TestRawMiddleware_EmptyBodies(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://llm.example.com", http.NoBody)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	resp, err := mw(req, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	if err != nil {
		t.Fatalf("middleware: %v", err)
	}
	if resp == nil {
		t.Fatal("resp is nil")
	}
	rec := tw.one(t)
	if len(rec.Request) != 0 || rec.Response != nil || rec.ResponseText != "" {
		t.Errorf("captures must be empty: req=%q resp=%q text=%q", rec.Request, rec.Response, rec.ResponseText)
	}
}

// TestRawHolder_ConcurrentSetAndWrite exercises the holder's locking under
// concurrent writers and captures.
func TestRawHolder_ConcurrentSetAndWrite(t *testing.T) {
	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	mw := newRawMiddleware(holder)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := rawRequest(t, context.Background(), `{"model":"m"}`, nil)
			_, _ = mw(req, func(*http.Request) (*http.Response, error) { return jsonResponse(`{}`), nil })
		}()
	}
	holder.Set(tw)
	wg.Wait()

	tw.mu.Lock()
	n := len(tw.records)
	tw.mu.Unlock()
	if n == 0 {
		t.Fatal("no records captured")
	}
	for _, rec := range tw.records {
		if !bytes.Equal(rec.Request, []byte(`{"model":"m"}`)) {
			t.Fatalf("corrupted record under concurrency: %q", rec.Request)
		}
	}
}

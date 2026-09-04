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
	if rec.StatusCode != http.StatusOK {
		t.Errorf("status_code = %d, want 200", rec.StatusCode)
	}
	if string(rec.RequestBody) != reqBody {
		t.Errorf("request_body = %q, want raw body", rec.RequestBody)
	}
	if string(rec.ResponseBody) != respBody {
		t.Errorf("response_body = %q, want raw body", rec.ResponseBody)
	}
	if rec.ResponseBodyText != "" || rec.Error != "" {
		t.Errorf("unexpected response_body_text=%q error=%q", rec.ResponseBodyText, rec.Error)
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

// Timestamp must be the attempt start, so that [timestamp, timestamp +
// duration_ms] reconstructs the attempt interval. The TTFB sleep inside
// next() is what pins this: taken after next() returns, the timestamp would
// land in a later RFC 3339 second than the start (the delay exceeds one
// second) and the assertion below would fail.
func TestRawMiddleware_TimestampIsAttemptStart(t *testing.T) {
	req := rawRequest(t, context.Background(), `{}`, nil)
	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	const ttfb = 1100 * time.Millisecond
	if _, err := mw(req, func(*http.Request) (*http.Response, error) {
		time.Sleep(ttfb)
		return jsonResponse(`{}`), nil
	}); err != nil {
		t.Fatalf("middleware: %v", err)
	}
	end := time.Now()

	rec := tw.one(t)
	ts, err := time.Parse(time.RFC3339, rec.Timestamp)
	if err != nil {
		t.Fatalf("timestamp %q not RFC 3339: %v", rec.Timestamp, err)
	}
	if !ts.Before(end.Truncate(time.Second)) {
		t.Errorf("timestamp %v is not before the attempt end %v; want the attempt start", ts, end)
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
	for k, v := range rec.RequestHeaders {
		if sensitiveHeader(k) && v != "[REDACTED]" {
			t.Errorf("header %s = %q, want [REDACTED]", k, v)
		}
	}
	if got := rec.RequestHeaders["User-Agent"]; got != "open-code-review/test" {
		t.Errorf("User-Agent = %q, want passthrough value", got)
	}
	for _, v := range rec.RequestHeaders {
		if strings.Contains(v, "secret") {
			t.Errorf("secret leaked in headers: %v", rec.RequestHeaders)
		}
	}
}

// Repeated headers are legal on the wire; capture must keep every value in
// the RFC 9110 comma-list form instead of silently truncating to the first.
func TestRawMiddleware_MultiValueHeadersAreJoined(t *testing.T) {
	req := rawRequest(t, context.Background(), `{}`, map[string]string{
		"Anthropic-Beta": "prompt-caching-2024-07-31",
	})
	req.Header.Add("Anthropic-Beta", "files-api-2025-04-14")

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	if _, err := mw(req, func(*http.Request) (*http.Response, error) { return jsonResponse(`{}`), nil }); err != nil {
		t.Fatalf("middleware: %v", err)
	}

	rec := tw.one(t)
	want := "prompt-caching-2024-07-31, files-api-2025-04-14"
	if got := rec.RequestHeaders["Anthropic-Beta"]; got != want {
		t.Errorf("Anthropic-Beta = %q, want %q", got, want)
	}
}

func TestRawMiddleware_CapturesResponseHeaders(t *testing.T) {
	req := rawRequest(t, context.Background(), `{}`, nil)
	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	if _, err := mw(req, func(*http.Request) (*http.Response, error) {
		resp := jsonResponse(`{}`)
		resp.Header = http.Header{
			"X-Request-Id": {"req-abc123"},
			"Set-Cookie":   {"session=super-secret"},
		}
		return resp, nil
	}); err != nil {
		t.Fatalf("middleware: %v", err)
	}

	rec := tw.one(t)
	if got := rec.ResponseHeaders["X-Request-Id"]; got != "req-abc123" {
		t.Errorf("X-Request-Id = %q, want req-abc123", got)
	}
	if got := rec.ResponseHeaders["Set-Cookie"]; got != "[REDACTED]" {
		t.Errorf("Set-Cookie = %q, want [REDACTED]", got)
	}
}

// A transport error has no response, so response_headers must be omitted
// from the JSON rather than written as an empty map.
func TestRawMiddleware_ResponseHeadersOmittedOnTransportError(t *testing.T) {
	req := rawRequest(t, context.Background(), `{}`, nil)
	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	if _, err := mw(req, func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: connection refused")
	}); err == nil {
		t.Fatal("middleware must propagate the transport error")
	}

	rec := tw.one(t)
	if rec.ResponseHeaders != nil {
		t.Errorf("response_headers = %v, want nil on a transport error", rec.ResponseHeaders)
	}
	line, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("record is not encodable: %v", err)
	}
	if strings.Contains(string(line), "response_headers") {
		t.Errorf("response_headers key present in JSON: %s", line)
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
	if rec.StatusCode != 0 {
		t.Errorf("status_code = %d, want 0 (no response)", rec.StatusCode)
	}
	if rec.ResponseBody != nil || rec.ResponseBodyText != "" {
		t.Errorf("error record must carry no response body: %q / %q", rec.ResponseBody, rec.ResponseBodyText)
	}
}

// A request-body read failure must be replayed to the SDK exactly as it would
// surface without capture: a clean truncated body would turn the client-side
// fault into a server-side 400. Mirrors the response-side errReader handling.
func TestRawMiddleware_RequestBodyReadErrorReplayed(t *testing.T) {
	readErr := errors.New("connection reset by peer")
	req := rawRequest(t, context.Background(), "", nil)
	req.Body = &brokenBody{partial: []byte(`{"model":"m",`), err: readErr}

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	var sdkReadErr error
	if _, err := mw(req, func(r *http.Request) (*http.Response, error) {
		_, sdkReadErr = io.ReadAll(r.Body)
		return jsonResponse(`{}`), nil
	}); err != nil {
		t.Fatalf("middleware: %v", err)
	}
	if !errors.Is(sdkReadErr, readErr) {
		t.Errorf("SDK body read error = %v, want %v", sdkReadErr, readErr)
	}

	rec := tw.one(t)
	if rec.Error != readErr.Error() {
		t.Errorf("error = %q, want %q", rec.Error, readErr.Error())
	}
	// The partial bytes of a failed read are not valid JSON and stay out of
	// RequestBody; the error field carries the failure.
	if rec.RequestBody != nil {
		t.Errorf("request_body = %q, want nil for a failed body read", rec.RequestBody)
	}
	if _, err := json.Marshal(rec); err != nil {
		t.Errorf("record is not encodable: %v", err)
	}
}

// When next() fails for an independent reason (e.g. the context is cancelled
// before the replayed body is read), the record must keep the body-read root
// cause alongside the downstream error instead of letting either hide the
// other.
func TestRawMiddleware_JoinsRequestBodyReadErrorWithNextError(t *testing.T) {
	readErr := errors.New("connection reset by peer")
	req := rawRequest(t, context.Background(), "", nil)
	req.Body = &brokenBody{partial: []byte(`{"model":"m",`), err: readErr}

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	nextErr := errors.New("context deadline exceeded")
	if _, err := mw(req, func(*http.Request) (*http.Response, error) {
		return nil, nextErr
	}); !errors.Is(err, nextErr) {
		t.Fatalf("middleware error = %v, want %v", err, nextErr)
	}

	rec := tw.one(t)
	want := readErr.Error() + "; next: " + nextErr.Error()
	if rec.Error != want {
		t.Errorf("error = %q, want %q", rec.Error, want)
	}
}

// HTTP-level failures are not transport errors — the SDK retries them and the
// body lands in the record as-is — so status_code must carry the signal.
func TestRawMiddleware_RecordsNon200StatusCode(t *testing.T) {
	req := rawRequest(t, context.Background(), `{"model":"m"}`, nil)

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	_, err := mw(req, func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader(`{"error":"slow down"}`)),
		}, nil
	})
	if err != nil {
		t.Fatalf("middleware: %v", err)
	}

	rec := tw.one(t)
	if rec.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status_code = %d, want 429", rec.StatusCode)
	}
	if rec.Error != "" {
		t.Errorf("error = %q, want empty for an HTTP-level failure", rec.Error)
	}
}

func TestRawMiddleware_NonJSONResponseGoesToResponseBodyText(t *testing.T) {
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
	if rec.ResponseBody != nil {
		t.Errorf("response_body = %q, want empty for non-JSON body", rec.ResponseBody)
	}
	if rec.ResponseBodyText != sse {
		t.Errorf("response_body_text = %q, want SSE body", rec.ResponseBodyText)
	}
}

// The SDK marshals every request body, so this path is unreachable today; if a
// future middleware mangles the bytes, the record must survive with them
// verbatim instead of being dropped whole.
func TestRawMiddleware_NonJSONRequestGoesToRequestBodyText(t *testing.T) {
	req := rawRequest(t, context.Background(), `{not-json`, nil)

	tw := &recordingRawWriter{}
	holder := NewRawHolder()
	holder.Set(tw)
	mw := newRawMiddleware(holder)

	if _, err := mw(req, func(*http.Request) (*http.Response, error) { return jsonResponse(`{}`), nil }); err != nil {
		t.Fatalf("middleware: %v", err)
	}

	rec := tw.one(t)
	if rec.RequestBody != nil {
		t.Errorf("request_body = %q, want empty for non-JSON body", rec.RequestBody)
	}
	if rec.RequestBodyText != `{not-json` {
		t.Errorf("request_body_text = %q, want malformed body verbatim", rec.RequestBodyText)
	}
	if _, err := json.Marshal(rec); err != nil {
		t.Errorf("record is not encodable: %v", err)
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
	if rec.ResponseBodyText != `{"partial":` {
		t.Errorf("response_body_text = %q, want partial bytes", rec.ResponseBodyText)
	}
	if rec.ResponseBody != nil {
		t.Errorf("response_body = %q, want empty on read error", rec.ResponseBody)
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
	if err := json.Unmarshal(rec.RequestBody, &reqMap); err != nil {
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
	if err := json.Unmarshal(rec.ResponseBody, &respMap); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if respMap["id"] != "chatcmpl-test" {
		t.Errorf("raw response id = %v, want chatcmpl-test", respMap["id"])
	}
	if rec.RequestHeaders["Authorization"] != "[REDACTED]" {
		t.Errorf("Authorization = %q, want [REDACTED]", rec.RequestHeaders["Authorization"])
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
	if !bytes.Equal(recs[0].RequestBody, recs[1].RequestBody) {
		t.Errorf("replayed request bodies differ:\n%s\n%s", recs[0].RequestBody, recs[1].RequestBody)
	}
	for i, rec := range recs {
		if rec.FilePath != "a/b.go" || rec.TaskType != "main_task" || rec.RequestNo != 1 {
			t.Errorf("attempt %d identity = (%q,%q,%d), want meta values", i+1, rec.FilePath, rec.TaskType, rec.RequestNo)
		}
	}
	if string(recs[0].ResponseBody) != errorBody {
		t.Errorf("attempt 1 response body = %q, want the 500 body", recs[0].ResponseBody)
	}
	if string(recs[1].ResponseBody) != successBody {
		t.Errorf("attempt 2 response body = %q, want the success body", recs[1].ResponseBody)
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
	if len(rec.RequestBody) != 0 || rec.ResponseBody != nil || rec.ResponseBodyText != "" {
		t.Errorf("captures must be empty: req=%q resp=%q text=%q", rec.RequestBody, rec.ResponseBody, rec.ResponseBodyText)
	}
	// The record must still encode: an empty non-nil RequestBody used to fail
	// json.Marshal and the writer silently dropped the whole record.
	if _, err := json.Marshal(rec); err != nil {
		t.Errorf("record with empty bodies is not encodable: %v", err)
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
		if !bytes.Equal(rec.RequestBody, []byte(`{"model":"m"}`)) {
			t.Fatalf("corrupted record under concurrency: %q", rec.RequestBody)
		}
	}
}

// slowRawWriter stands in for a costly capture sink so timing pollution is
// measurable regardless of disk speed.
type slowRawWriter struct {
	mu    sync.Mutex
	count int
	delay time.Duration
}

func (w *slowRawWriter) Write(rec RawRecord) {
	time.Sleep(w.delay)
	w.mu.Lock()
	w.count++
	w.mu.Unlock()
}

func (w *slowRawWriter) Close() error { return nil }

// TestRawCaptureDoesNotInflateObserverTiming guards the middleware order: raw
// must sit outside the retry observer, so its full-body read and writer call
// never land inside the observer's DurationToHeadersMS window. If raw is
// mounted inside, the sleeping writer pushes the measured duration past the
// delay and this assertion goes red.
func TestRawCaptureDoesNotInflateObserverTiming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openAIOKBody))
	}))
	defer server.Close()

	const writeDelay = 300 * time.Millisecond
	tw := &slowRawWriter{delay: writeDelay}
	c := NewRetryCollector()
	holder := NewRawHolder()
	client := NewLLMClient(ResolvedEndpoint{
		URL:      server.URL + "/v1",
		Token:    "test-key",
		Model:    "gpt-test",
		Protocol: ProtocolOpenAIChatCompletions,
	}, c, holder)
	holder.Set(tw)

	m := testMeta()
	if _, err := ping(metaCtx(m), client); err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}

	tw.mu.Lock()
	writes := tw.count
	tw.mu.Unlock()
	if writes != 1 {
		t.Fatalf("writer ran %d times, want 1 (assertion below only meaningful if capture is active)", writes)
	}

	got := attemptsFor(t, c, m)
	if len(got) != 1 {
		t.Fatalf("got %d attempts, want 1", len(got))
	}
	if got[0].DurationToHeadersMS >= writeDelay.Milliseconds()-50 {
		t.Errorf("duration_to_headers_ms = %d, inflated by raw capture (writer sleeps %v)",
			got[0].DurationToHeadersMS, writeDelay)
	}
}

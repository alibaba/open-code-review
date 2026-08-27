// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// rawLoggingEnv is the opt-in switch for raw LLM capture. Only the exact
// value "1" enables it (mirrors telemetry's OCR_ENABLE_TELEMETRY), so an unset
// or misspelled variable keeps the default: no middleware mounted, no files
// created, no behavior change.
const rawLoggingEnv = "OCR_RAW_LOGGING"

func RawLoggingEnabled() bool {
	return os.Getenv(rawLoggingEnv) == "1"
}

// RawRecord is one JSONL line: everything captured about a single HTTP
// attempt against an LLM endpoint. One logical request may expand into several
// attempts inside the SDK retry loop, and each attempt gets its own record.
type RawRecord struct {
	// SessionID identifies the review/scan session this attempt belongs to. It
	// is stamped by the writer, which is bound per session; the middleware
	// leaves it empty.
	SessionID string `json:"session_id"`

	// RequestID identifies this attempt, not the logical request above it.
	RequestID string `json:"request_id"`

	// Timestamp is when the record was written, RFC 3339 UTC.
	Timestamp string `json:"timestamp"`

	// FilePath, TaskType and RequestNo mirror session.TaskRecord identity and
	// come from the RequestMeta attached to the request context. Scan requests
	// carry no meta, so these stay empty / zero — an honest omission rather
	// than a fabricated identity; `ocr llm test` captures nothing at all.
	FilePath  string `json:"file_path,omitempty"`
	TaskType  string `json:"task_type,omitempty"`
	RequestNo int    `json:"request_no,omitempty"`

	// TraceID is the OTel trace ID of the span context the request carries
	// (the review.run tree). It joins raw records to the OTLP span tree when
	// telemetry is on; omitted otherwise, so the two channels stay independent.
	TraceID string `json:"trace_id,omitempty"`

	// Model is the meta's model, falling back to the request body's "model"
	// field when no meta is attached.
	Model string `json:"model,omitempty"`

	// DurationMs covers the whole attempt: next() plus the full response-body
	// capture, so for streaming it spans the entire stream, not just the
	// connection.
	DurationMs int64 `json:"duration_ms"`

	// Headers are the request headers, with credential-bearing ones redacted
	// by sensitiveHeader.
	Headers map[string]string `json:"headers"`

	// Request is the raw request body after extra_body merging and
	// session-key expansion — exactly what was sent. Always valid JSON (the
	// SDK marshals it), so unlike Response it needs no text fallback.
	Request json.RawMessage `json:"request"`

	// Response holds the raw response body when it is valid JSON
	// (every non-streaming completion response). Mutually exclusive with
	// ResponseText.
	Response json.RawMessage `json:"response,omitempty"`

	// ResponseText holds a non-JSON response body verbatim. This is the SSE
	// stream text (extra_body.stream=true) — stuffing it into Response would
	// emit a malformed JSONL line.
	ResponseText string `json:"response_text,omitempty"`

	// Error carries the transport-level error when next() failed; the record
	// then has no response. HTTP-level errors are not transport errors: the
	// SDK retries them internally, and the body lands in Response as-is.
	Error string `json:"error,omitempty"`
}

// RawWriter receives raw records. Implemented by session.RawFileWriter;
// kept as an interface so the llm package never learns where captures go.
type RawWriter interface {
	Write(rec RawRecord)
	Close() error
}

// RawHolder is a thread-safe late-binding slot for a RawWriter. The LLM
// client is constructed in loadLLMRuntime, before any session exists, while
// the writer needs the session's ID and repo dir — so the holder is created
// with the client and the writer is set once the session is known.
type RawHolder struct {
	mu sync.RWMutex
	w  RawWriter
}

func NewRawHolder() *RawHolder { return &RawHolder{} }

func (h *RawHolder) Set(w RawWriter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.w = w
}

func (h *RawHolder) get() RawWriter {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.w
}

// sensitiveHeaderKeywords names the substrings that mark a request header as
// credential-bearing: provider secrets (authorization, x-api-key,
// x-amz-security-token, x-session-token, …) and session cookies are named
// after what they carry. Missing one leaks a secret into raw logs, while a
// false positive only hides a header, so this errs toward redacting.
var sensitiveHeaderKeywords = []string{"auth", "token", "key", "secret", "credential", "cookie"}

func sensitiveHeader(name string) bool {
	lower := strings.ToLower(name)
	for _, kw := range sensitiveHeaderKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// newRawMiddleware builds the SDK middleware that captures raw
// request/response bodies into the writer held by holder. It sits inside the
// SDK retry loop, so it records one line per real HTTP attempt.
//
// The bodies are read fully and then restored for the SDK to consume. For a
// streaming response this means the middleware buffers the whole SSE stream
// before the client sees it — acceptable for an opt-in raw capture.
//
// Raw capture must never fail a review: every capture step tolerates errors
// and the record write happens last, fire-and-forget.
func newRawMiddleware(holder *RawHolder) retryObserver {
	return func(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		tw := holder.get()
		if tw == nil {
			return next(req)
		}

		meta, _ := RequestMetaFromContext(req.Context())

		headers := make(map[string]string, len(req.Header))
		for k, vs := range req.Header {
			if len(vs) == 0 {
				continue
			}
			if sensitiveHeader(k) {
				headers[k] = "[REDACTED]"
			} else {
				headers[k] = vs[0]
			}
		}

		var reqBody []byte
		if req.Body != nil {
			reqBody, _ = io.ReadAll(req.Body)
			req.Body = io.NopCloser(bytes.NewReader(reqBody))
		}

		model := meta.Model
		if model == "" && len(reqBody) > 0 {
			var partial struct {
				Model string `json:"model"`
			}
			if json.Unmarshal(reqBody, &partial) == nil {
				model = partial.Model
			}
		}

		startedAt := time.Now()
		resp, err := next(req)

		rec := RawRecord{
			RequestID: uuid.NewString(),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			FilePath:  meta.FilePath,
			TaskType:  meta.TaskType,
			RequestNo: meta.RequestNo,
			Model:     model,
			Headers:   headers,
			Request:   json.RawMessage(reqBody),
		}
		if sc := oteltrace.SpanContextFromContext(req.Context()); sc.HasTraceID() {
			rec.TraceID = sc.TraceID().String()
		}

		if err != nil {
			rec.DurationMs = time.Since(startedAt).Milliseconds()
			rec.Error = err.Error()
			tw.Write(rec)
			return resp, err
		}

		if resp.Body != nil {
			respBody, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				// Hand the SDK the same read failure it would see without capture.
				rec.DurationMs = time.Since(startedAt).Milliseconds()
				rec.Error = readErr.Error()
				if len(respBody) > 0 {
					rec.ResponseText = string(respBody)
				}
				tw.Write(rec)
				resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(respBody), errReader{readErr}))
				return resp, nil
			}
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			if json.Valid(respBody) {
				rec.Response = json.RawMessage(respBody)
			} else {
				rec.ResponseText = string(respBody)
			}
		}

		rec.DurationMs = time.Since(startedAt).Milliseconds()
		tw.Write(rec)
		return resp, nil
	}
}

// errReader replays a captured read error once the buffered bytes are drained.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

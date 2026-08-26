// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// logicalRequestIDVersion is the domain prefix mixed into every
// logical_request_id. Changing the field set of RequestMeta must bump this
// prefix so IDs from two different field sets can never collide.
const logicalRequestIDVersion = "ocr.llm-request/v1"

// RequestMeta identifies one logical LLM request. A logical request is a single
// business-level call — one main_task turn, one re-location, one memory
// compression — which may expand into several real HTTP attempts inside the SDK
// retry loop.
//
// The fields mirror the existing session.TaskRecord so the retry report joins
// against the session JSONL without introducing a second source of truth:
//
//   - FilePath is the raw session path (session.FileSession.FilePath), not the
//     manifest-normalized path. manifestPaths collapses the "/dev/null"
//     sentinel for item_id derivation; normalizing here would break the join.
//   - TaskType is the session.TaskType string.
//   - RequestNo is session.TaskRecord.RequestNo, a sequence starting at 1 that
//     is scoped per (FilePath, TaskType). That scoping is what makes the tuple
//     unique within a run.
//
// run_id is deliberately absent. It is only needed to compute
// logical_request_id, which happens in RetryCollector.Freeze, so the collector
// can be constructed before the session exists — the client is built in
// loadLLMRuntime, well before agent.New.
type RequestMeta struct {
	// Provider is the resolved provider label. It is legitimately empty for an
	// unnamed endpoint (only llm.ResolvedEndpoint built from the provider
	// config path carries a name), so empty is a valid value and never means
	// "missing meta". It must not be replaced by Protocol.
	Provider  string
	Model     string
	FilePath  string
	TaskType  string
	RequestNo int
}

// valid reports whether m can identify a logical request.
//
// Provider may be empty; Model, FilePath and TaskType may not. No field may
// contain NUL, because NUL is the canonical encoding separator: accepting it
// would let two different metas produce the same digest.
func (m RequestMeta) valid() bool {
	if m.RequestNo <= 0 {
		return false
	}
	if strings.ContainsRune(m.Provider, 0) {
		return false
	}
	for _, s := range []string{m.Model, m.FilePath, m.TaskType} {
		if s == "" || strings.ContainsRune(s, 0) {
			return false
		}
	}
	return true
}

// logicalRequestID computes the canonical logical_request_id for m under runID.
//
// Fields are NUL-terminated in a fixed order rather than concatenated, because
// FilePath, Provider and Model can hold arbitrary printable bytes and a plain
// concatenation would let distinct metas collide. RequestNo is encoded as
// decimal ASCII so no byte order is involved. Bytes participate as-is: no case
// or path normalization, since the ID only has to be stable within one run.
//
// Every field is terminated, including the last one, and RequestNo goes through
// the same loop as the strings. A trailing separator is redundant today, but a
// field appended after an unterminated RequestNo would collide immediately
// (request_no 12 + "abc" against request_no 1 + "2abc"), so the encoding is
// written so that adding a field cannot introduce that.
//
// The precondition is that m is valid: an invalid meta may contain NUL and
// therefore forge another meta's byte stream. It holds because the only metas
// reaching here are RetryCollector entry keys, and RecordAttempt validates
// before creating an entry. This is not re-checked here, because the reporting
// path must never panic or fail a review.
func (m RequestMeta) logicalRequestID(runID string) string {
	h := sha256.New()
	for _, field := range []string{
		logicalRequestIDVersion,
		runID,
		m.Provider,
		m.Model,
		m.FilePath,
		m.TaskType,
		strconv.Itoa(m.RequestNo),
	} {
		h.Write([]byte(field))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// describe renders m for internal construction errors raised by
// RetryCollector.Freeze.
//
// Those errors reach the user through runErr, and with --concurrency > 1 a run
// holds hundreds of logical requests: without identity there is no way to tell
// which one tripped the invariant. Only fields that RequestReport already emits
// are included, so this adds no disclosure surface.
func (m RequestMeta) describe() string {
	return fmt.Sprintf("file=%s task=%s request_no=%d", m.FilePath, m.TaskType, m.RequestNo)
}

// requestMetaKey is the private context key type for RequestMeta. It is a
// struct{} rather than a string so no other package can collide with it.
type requestMetaKey struct{}

// WithRequestMeta attaches m to ctx so the retry observer can associate every
// HTTP attempt made under that context with one logical request. It is the only
// way request identity reaches the observer: LLMClient is a single-method
// interface (CompletionsWithCtx), so no call site signature changes.
//
// An invalid meta is not attached and ctx is returned unchanged. The attempt
// then follows the same path as any request without identity — dropped by the
// collector, absent from the report — instead of introducing a new failure mode
// that could suppress the whole report or fail the review.
func WithRequestMeta(ctx context.Context, m RequestMeta) context.Context {
	if ctx == nil || !m.valid() {
		return ctx
	}
	return context.WithValue(ctx, requestMetaKey{}, m)
}

// RequestMetaFromContext returns the RequestMeta attached to ctx, if any.
//
// It is exported so the packages that stamp identity — internal/llmloop and
// internal/agent — can assert in their own tests that a request reached the
// client carrying the right meta, and that scan's requests carry none. A
// package-local export_test.go cannot serve that: it is only visible to tests
// of package llm, and none of the call sites live here. The accessor is
// read-only and stays inside internal/, so it adds no mutation path and no
// repository-external API.
func RequestMetaFromContext(ctx context.Context) (RequestMeta, bool) {
	if ctx == nil {
		return RequestMeta{}, false
	}
	m, ok := ctx.Value(requestMetaKey{}).(RequestMeta)
	return m, ok
}

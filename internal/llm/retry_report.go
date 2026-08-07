// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// RetryReportSchemaVersion is the contract version of the emitted report.
const RetryReportSchemaVersion = "ocr.llm-retry-report/v1"

// ErrorClass is the attempt-level error classification. It is derived only from
// the HTTP status and the Go error type — never from error message text — and it
// is not interchangeable with session.FailureClass (file level) or
// session.RunFailureClass (run level).
type ErrorClass string

const (
	ErrorClassRateLimited    ErrorClass = "rate_limited"
	ErrorClassOverloaded     ErrorClass = "overloaded"
	ErrorClassAuthentication ErrorClass = "authentication"
	ErrorClassTimeout        ErrorClass = "timeout"
	ErrorClassNetwork        ErrorClass = "network"
	ErrorClassProvider       ErrorClass = "provider"
	ErrorClassCancelled      ErrorClass = "cancelled"
	ErrorClassUnknown        ErrorClass = "unknown"
)

// valid reports whether c is one of the fixed attempt error classes.
func (c ErrorClass) valid() bool {
	switch c {
	case ErrorClassRateLimited, ErrorClassOverloaded, ErrorClassAuthentication,
		ErrorClassTimeout, ErrorClassNetwork, ErrorClassProvider,
		ErrorClassCancelled, ErrorClassUnknown:
		return true
	}
	return false
}

// FailurePhase records where in the request lifecycle the failure surfaced.
type FailurePhase string

const (
	FailurePhaseTransport      FailurePhase = "transport"
	FailurePhaseHTTP           FailurePhase = "http"
	FailurePhaseResponseDecode FailurePhase = "response_decode"
	FailurePhaseStream         FailurePhase = "stream"
	FailurePhaseResponseStatus FailurePhase = "response_status"
	FailurePhaseContext        FailurePhase = "context"
)

// valid reports whether p is one of the fixed failure phases.
func (p FailurePhase) valid() bool {
	switch p {
	case FailurePhaseTransport, FailurePhaseHTTP, FailurePhaseResponseDecode,
		FailurePhaseStream, FailurePhaseResponseStatus, FailurePhaseContext:
		return true
	}
	return false
}

// Outcome is the request-level result of a logical request. It is decided once,
// at Finalize, from the whole attempt sequence plus the logical request's own
// return value and the parent context state — never inferred from the last
// attempt.
type Outcome string

const (
	// OutcomeSucceeded means the logical request succeeded with no error
	// attempt. It still reaches the report when the server made the SDK retry a
	// successful response via x-should-retry, which is why recovered_requests
	// stays honest: nothing was recovered from.
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeRecovered Outcome = "recovered"
	OutcomeFailed    Outcome = "failed"
	OutcomeCancelled Outcome = "cancelled"
)

// AttemptOutcome is the per-attempt result.
type AttemptOutcome string

const (
	AttemptSuccess AttemptOutcome = "success"
	AttemptError   AttemptOutcome = "error"
)

// AttemptRecord is one observed real HTTP attempt.
//
// Number, Outcome, DurationToHeadersMS and ObservedBackoffMS are assigned by the
// collector; values passed in are ignored. The two durations are derived from the
// timestamps RecordAttempt receives, because ObservedBackoffMS spans two attempts
// and only the collector holds per-request state. Diagnostic fields carry
// observed values only — no request or response bodies, no prompts, no URLs, no
// raw SDK error strings.
type AttemptRecord struct {
	Number       int            `json:"attempt"`
	Outcome      AttemptOutcome `json:"outcome"`
	ErrorClass   ErrorClass     `json:"error_class,omitempty"`
	FailurePhase FailurePhase   `json:"failure_phase,omitempty"`

	StatusCode int    `json:"status_code,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	// RetryAfterMS is the server hint, normalized to milliseconds.
	RetryAfterMS int64 `json:"retry_after_ms,omitempty"`
	// ObservedBackoffMS is the measured gap between the end of the previous
	// attempt and the start of this one. It is not the SDK's planned wait: the
	// SDK's jitter is not readable from outside.
	ObservedBackoffMS int64 `json:"observed_backoff_ms,omitempty"`
	// DurationToHeadersMS covers request send to response headers only, not body
	// read, decode or streaming. Total logical request time stays on
	// session.TaskRecord.Duration.
	DurationToHeadersMS int64 `json:"duration_to_headers_ms,omitempty"`
	// SDKRetryDirective is the x-should-retry response header. A pointer so an
	// absent header is distinguishable from an explicit false.
	SDKRetryDirective *bool `json:"sdk_retry_directive,omitempty"`
}

// RequestReport is one logical request and its attempts.
type RequestReport struct {
	LogicalRequestID string `json:"logical_request_id"`
	// Provider is required but may be empty: an empty string stably denotes an
	// unnamed endpoint, so it is not omitempty. This is a deliberate difference
	// from the display-only jsonLLMIdentity.Provider, which is omitempty.
	Provider  string          `json:"provider"`
	Model     string          `json:"model"`
	FilePath  string          `json:"file_path"`
	TaskType  string          `json:"task_type"`
	RequestNo int             `json:"request_no"`
	Outcome   Outcome         `json:"outcome"`
	Attempts  []AttemptRecord `json:"attempts"`
}

// RetryReport is the frozen, immutable result both the terminal summary and the
// JSON output read from.
type RetryReport struct {
	SchemaVersion string `json:"schema_version"`
	// TotalRequests counts RequestMeta that produced at least one real HTTP
	// attempt. A logical request that failed before entering the observer has no
	// observed fact to report and is excluded.
	TotalRequests     int             `json:"total_requests"`
	RetriedRequests   int             `json:"retried_requests"`
	TotalRetries      int             `json:"total_retries"`
	RecoveredRequests int             `json:"recovered_requests"`
	FailedRequests    int             `json:"failed_requests"`
	Requests          []RequestReport `json:"requests"`
}

// attemptObservation is the provider-agnostic classifier input.
type attemptObservation struct {
	// StatusCode is 0 when no HTTP response was received.
	StatusCode int
	Err        error
}

// isErrorStatus reports whether an observed HTTP status proves the attempt
// failed on its own. Status 0 means no response was received, so it proves
// nothing.
//
// This is the single definition of that boundary, shared by classifyAttempt and
// the collector's consistency guard. Duplicating it would let the classifier and
// the guard disagree about, say, a 3xx, and the guard would then reject attempts
// the classifier considers fine.
func isErrorStatus(code int) bool {
	return code > 0 && (code < 200 || code >= 300)
}

// classifyAttempt maps an observation to an ErrorClass and FailurePhase.
//
// A non-2xx status is the strongest available fact, so it decides the class
// before the error is consulted. A 2xx carries no error information, so it falls
// through to the error-based branch; that is also the branch the client-boundary
// correction feeds when a 200 turns out to be truncated.
func classifyAttempt(obs attemptObservation) (ErrorClass, FailurePhase) {
	if isErrorStatus(obs.StatusCode) {
		switch obs.StatusCode {
		case 429:
			return ErrorClassRateLimited, FailurePhaseHTTP
		case 529:
			return ErrorClassOverloaded, FailurePhaseHTTP
		case 401, 403:
			return ErrorClassAuthentication, FailurePhaseHTTP
		case 408, 504:
			return ErrorClassTimeout, FailurePhaseHTTP
		default:
			// Coarse bucket on purpose: 402/404/409/413 and transient 5xx all
			// land here and are told apart by status_code, rather than growing
			// an enum that duplicates HTTP.
			return ErrorClassProvider, FailurePhaseHTTP
		}
	}

	switch {
	case errors.Is(obs.Err, context.Canceled):
		return ErrorClassCancelled, FailurePhaseContext
	case errors.Is(obs.Err, context.DeadlineExceeded):
		return ErrorClassTimeout, FailurePhaseContext
	case errors.Is(obs.Err, io.ErrUnexpectedEOF):
		return ErrorClassNetwork, FailurePhaseResponseDecode
	case obs.Err != nil:
		return ErrorClassNetwork, FailurePhaseTransport
	}

	// No status and no error is a caller bug: a successful attempt should never
	// be classified. Report it rather than guessing.
	if obs.StatusCode > 0 {
		return ErrorClassUnknown, FailurePhaseHTTP
	}
	return ErrorClassUnknown, FailurePhaseTransport
}

// requestEntry is the collector's mutable per-request state.
type requestEntry struct {
	attempts  []AttemptRecord
	outcome   Outcome
	finalized bool
	// violation records the first detected ordering bug (double Finalize, or
	// mutation after Finalize). It surfaces as a Freeze error so the invariant
	// "Finalize runs exactly once per logical request" is machine-checked
	// instead of only asserted in prose.
	violation string
	// lastAttemptEnd is when the previous attempt of this logical request
	// finished, and is the only state ObservedBackoffMS needs. It lives here
	// rather than in the observer because one observer instance serves every
	// concurrent request: keyed per-request state is exactly what the collector
	// already is.
	//
	// It stays zero until an attempt is actually appended, so a dropped attempt
	// never becomes the baseline for the next gap.
	lastAttemptEnd time.Time
}

func (e *requestEntry) hasErrorAttempt() bool {
	for _, a := range e.attempts {
		if a.Outcome == AttemptError {
			return true
		}
	}
	return false
}

// RetryCollector aggregates observed attempts for one review run.
//
// It is created per run and owned by llmRuntime; there is no package-level
// state, so two runs in one process can never share data. All methods are safe
// for concurrent use because --concurrency > 1 has several files writing
// attempts into the same instance.
//
// There is no Register step: the first RecordAttempt creates the entry. That
// makes "TotalRequests counts metas with at least one attempt" a property of the
// data structure rather than a rule to remember, and it makes a zero-attempt
// logical request a no-op at Finalize.
type RetryCollector struct {
	mu      sync.Mutex
	entries map[RequestMeta]*requestEntry
}

// NewRetryCollector returns an empty collector.
func NewRetryCollector() *RetryCollector {
	return &RetryCollector{entries: make(map[RequestMeta]*requestEntry)}
}

// RecordAttempt appends one observed HTTP attempt for m.
//
// startedAt and endedAt bracket the real HTTP call: the observer takes them
// immediately before and after the SDK's transport call returns response
// headers. They are passed in rather than read from a clock here so the
// collector stays a pure aggregator and the derived durations are fully
// deterministic in tests; no clock abstraction is needed.
//
// An invalid meta is dropped: without identity there is nothing to aggregate
// against, which is also how scan and llm test requests stay out of the report.
// Number, Outcome and both durations on a are derived here, so the observer can
// neither desynchronize the numbering from the real call order nor invent a
// backoff it cannot measure.
//
// An attempt arriving after Finalize is recorded as a violation and dropped
// rather than appended, the same way ReviseLastAttempt behaves: the outcome was
// already decided without knowledge of this attempt, so mutating the sequence
// afterwards could only produce a record that contradicts it.
func (c *RetryCollector) RecordAttempt(m RequestMeta, a AttemptRecord, startedAt, endedAt time.Time) {
	if c == nil || !m.valid() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	e := c.entries[m]
	if e == nil {
		e = &requestEntry{}
		c.entries[m] = e
	}
	if e.finalized {
		if e.violation == "" {
			e.violation = "attempt recorded after Finalize"
		}
		return
	}

	if a.ErrorClass != "" || a.FailurePhase != "" {
		a.Outcome = AttemptError
	} else {
		a.Outcome = AttemptSuccess
	}
	if a.Outcome == AttemptSuccess {
		a.ErrorClass = ""
		a.FailurePhase = ""
	}

	// Consistency guard. Outcome is derived from the classification alone, so an
	// observer that reports a non-2xx status without classifying it (a caller
	// that forgot classifyAttempt) would have the attempt derived as a success
	// against the strongest fact it had.
	//
	// This is caught here because nothing downstream can catch it: a lone
	// unclassified error attempt makes Finalize decide succeeded, which makes the
	// listing rule in Freeze skip the request entirely, so a failed request would
	// vanish from requests while still counted in total_requests. validateReport
	// only walks listed requests and would never see it.
	//
	// Recorded as a violation rather than repaired: the collector has the status
	// but not the error, so it cannot derive the class, and guessing one would put
	// a fabricated classification in the report. The attempt is still appended, so
	// the state stays inspectable; Freeze refuses to publish either way.
	if a.Outcome == AttemptSuccess && isErrorStatus(a.StatusCode) && e.violation == "" {
		e.violation = "non-2xx attempt recorded without a classification"
	}

	// Derived from the observed timestamps, never from what the caller put in a.
	//
	// The gap is skipped on a zero baseline rather than on "this is attempt 1".
	// The two differ on the OpenAI Chat Completions EOF recovery, which makes a
	// second SDK call under the same logical request: that call's first attempt
	// does have a predecessor, and its gap measures the client-side re-call
	// interval rather than an SDK backoff. That is still a real measured
	// interval, which is all the field claims to be.
	a.DurationToHeadersMS = nonNegativeMillis(endedAt.Sub(startedAt))
	a.ObservedBackoffMS = 0
	if !e.lastAttemptEnd.IsZero() {
		a.ObservedBackoffMS = nonNegativeMillis(startedAt.Sub(e.lastAttemptEnd))
	}

	a.Number = len(e.attempts) + 1
	e.attempts = append(e.attempts, a)
	e.lastAttemptEnd = endedAt
}

// nonNegativeMillis converts d to milliseconds, flooring at zero.
//
// Timestamps taken from time.Now carry a monotonic reading, so a real attempt
// can never measure negative. Hand-built time.Time values have no monotonic
// reading and can, so the floor exists to keep a nonsensical negative out of the
// report rather than to paper over a real inversion.
func nonNegativeMillis(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return d.Milliseconds()
}

// ReviseLastAttempt rewrites the last attempt of m as an error.
//
// It is the client-boundary correction for what the observer cannot see: an
// error surfacing only after HTTP 200 (truncated body, mid-stream failure, a
// non-success Responses object status).
//
// The precondition is enforced here rather than at the call site: the revision
// applies only while the last attempt is still recorded as a success. An attempt
// already classified from its status code (500, 402) is never rewritten to
// unknown/response_decode just because its error body failed to parse — the
// status code is the stronger fact, and a corrupt body only costs diagnostic
// richness.
func (c *RetryCollector) ReviseLastAttempt(m RequestMeta, class ErrorClass, phase FailurePhase) {
	if c == nil || !m.valid() || !class.valid() || !phase.valid() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	e := c.entries[m]
	if e == nil || len(e.attempts) == 0 {
		return
	}
	if e.finalized {
		if e.violation == "" {
			e.violation = "attempt revised after Finalize"
		}
		return
	}
	last := &e.attempts[len(e.attempts)-1]
	if last.Outcome != AttemptSuccess {
		return
	}
	last.Outcome = AttemptError
	last.ErrorClass = class
	last.FailurePhase = phase
}

// Finalize decides the request-level outcome for m exactly once.
//
// The three inputs are the recorded attempts, the error the logical request
// returned to the business layer, and whether the parent context was cancelled.
// parentCancelled is a bool rather than a context so the decision stays a pure
// function: reading the parent context is the client boundary's job, and only
// the boundary can tell the parent context from the SDK's per-attempt timeout
// context.
//
// Evaluation order matters and mirrors the design's ordered table:
//
//  1. no attempts       -> no record at all (nothing to report on)
//  2. parent cancelled  -> cancelled, even if the last attempt is an HTTP error
//  3. request returned an error -> failed
//  4. success with an error attempt -> recovered
//  5. success with no error attempt -> succeeded
//
// Rule 2 outranking rule 3 is the whole reason this is not inferred from the
// last attempt: cancelling during backoff produces no new attempt, so the
// sequence still ends in an error while the request outcome is cancelled.
//
// A logical request must be finalized once. The EOF recovery in OpenAI Chat
// Completions makes two SDK calls under one logical request and must finalize
// only at the outermost boundary; a second call is recorded as a violation and
// surfaces at Freeze.
func (c *RetryCollector) Finalize(m RequestMeta, reqErr error, parentCancelled bool) {
	if c == nil || !m.valid() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	e := c.entries[m]
	if e == nil {
		// Rule 1: the request never reached the observer.
		return
	}
	if e.finalized {
		if e.violation == "" {
			e.violation = "Finalize called more than once"
		}
		return
	}
	e.finalized = true

	switch {
	case parentCancelled:
		e.outcome = OutcomeCancelled
	case reqErr != nil:
		e.outcome = OutcomeFailed
	case e.hasErrorAttempt():
		e.outcome = OutcomeRecovered
	default:
		e.outcome = OutcomeSucceeded
	}
}

// Freeze builds the immutable report. It must be called once, after the run is
// done, and runID is the review session ID.
//
// The three return shapes are distinct and callers must handle all of them:
//
//	(nil, nil) — nothing worth reporting: no retry and no request error.
//	(nil, err) — an internal construction error. Do not publish a report.
//	(rep, nil) — publish rep.
//
// runID is only needed here, which is why the collector does not need it at
// construction time and therefore does not depend on the session existing when
// the client is built.
func (c *RetryCollector) Freeze(runID string) (*RetryReport, error) {
	if c == nil {
		return nil, nil
	}
	if runID == "" || strings.ContainsRune(runID, 0) {
		return nil, fmt.Errorf("retry report: invalid run_id")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	rep := &RetryReport{
		SchemaVersion: RetryReportSchemaVersion,
		TotalRequests: len(c.entries),
	}

	// Walk in logical_request_id order instead of map order. Both outputs depend
	// on it: the listed requests need the order anyway, and a construction error
	// has to name the same entry on every run, or a collector holding two broken
	// entries would report a different one each time and no test could pin it.
	type entryRef struct {
		id   string
		meta RequestMeta
		e    *requestEntry
	}
	refs := make([]entryRef, 0, len(c.entries))
	for meta, e := range c.entries {
		refs = append(refs, entryRef{id: meta.logicalRequestID(runID), meta: meta, e: e})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].id < refs[j].id })

	for _, ref := range refs {
		meta, e := ref.meta, ref.e
		if e.violation != "" {
			return nil, fmt.Errorf("retry report: %s (%s)", e.violation, meta.describe())
		}
		if !e.finalized {
			// Every logical request that produced an attempt must be finalized,
			// which forces the client boundary to finalize on every exit path
			// (including cancellation) rather than only on the happy path.
			return nil, fmt.Errorf("retry report: logical request not finalized (%s)", meta.describe())
		}
		if len(e.attempts) == 0 {
			return nil, fmt.Errorf("retry report: entry with no attempt (%s)", meta.describe())
		}

		rep.TotalRetries += len(e.attempts) - 1
		if len(e.attempts) > 1 {
			rep.RetriedRequests++
		}
		switch e.outcome {
		case OutcomeRecovered:
			rep.RecoveredRequests++
		case OutcomeFailed, OutcomeCancelled:
			rep.FailedRequests++
		}

		// Listing rule: anything that retried, anything that saw an error, and
		// anything whose outcome is not succeeded. The last clause is what keeps
		// the aggregates verifiable from the listed requests alone — a request
		// cancelled after a single clean attempt has no error attempt and no
		// retry, yet it is counted in FailedRequests, so it must be listed.
		if len(e.attempts) == 1 && !e.hasErrorAttempt() && e.outcome == OutcomeSucceeded {
			continue
		}
		rep.Requests = append(rep.Requests, RequestReport{
			LogicalRequestID: ref.id,
			Provider:         meta.Provider,
			Model:            meta.Model,
			FilePath:         meta.FilePath,
			TaskType:         meta.TaskType,
			RequestNo:        meta.RequestNo,
			Outcome:          e.outcome,
			Attempts:         append([]AttemptRecord(nil), e.attempts...),
		})
	}

	if len(rep.Requests) == 0 {
		return nil, nil
	}
	// No sort here: the walk above is already in logical_request_id order, which
	// is what makes the output stable under --concurrency > 1.
	if err := validateReport(rep); err != nil {
		return nil, err
	}
	return rep, nil
}

// validateReport enforces the report invariants. A violation returns an error
// and suppresses the report rather than publishing self-contradictory numbers.
//
// Every aggregate except TotalRequests is recomputed from the listed requests
// and compared with the value accumulated over all entries. That cross-check is
// the invariant: it holds only because the listing rule guarantees any request
// contributing to a count is listed. TotalRequests is the one aggregate that
// legitimately exceeds the listed set.
func validateReport(rep *RetryReport) error {
	if rep.SchemaVersion != RetryReportSchemaVersion {
		return fmt.Errorf("retry report: unexpected schema version %q", rep.SchemaVersion)
	}
	if rep.TotalRequests < len(rep.Requests) {
		return fmt.Errorf("retry report: total_requests %d below listed %d",
			rep.TotalRequests, len(rep.Requests))
	}

	seen := make(map[string]struct{}, len(rep.Requests))
	var retries, retried, recovered, failed int

	for _, r := range rep.Requests {
		if _, dup := seen[r.LogicalRequestID]; dup {
			return fmt.Errorf("retry report: duplicate logical_request_id")
		}
		seen[r.LogicalRequestID] = struct{}{}

		if len(r.Attempts) == 0 {
			return fmt.Errorf("retry report: request with no attempt")
		}
		hasError := false
		for i, a := range r.Attempts {
			if a.Number != i+1 {
				return fmt.Errorf("retry report: attempt numbering not contiguous from 1")
			}
			switch a.Outcome {
			case AttemptError:
				hasError = true
				if !a.ErrorClass.valid() || !a.FailurePhase.valid() {
					return fmt.Errorf("retry report: error attempt without valid classification")
				}
			case AttemptSuccess:
				if a.ErrorClass != "" || a.FailurePhase != "" {
					return fmt.Errorf("retry report: success attempt carries error fields")
				}
			default:
				return fmt.Errorf("retry report: unknown attempt outcome %q", a.Outcome)
			}
		}

		switch r.Outcome {
		case OutcomeRecovered:
			if !hasError {
				return fmt.Errorf("retry report: recovered request without error attempt")
			}
			recovered++
		case OutcomeSucceeded:
			if hasError {
				return fmt.Errorf("retry report: succeeded request with error attempt")
			}
			// A listed succeeded request must have retried. That is not an
			// independent rule: it holds only because Freeze's listing rule skips
			// a succeeded request whose single attempt is clean. The redundancy is
			// the point — this check fires if the listing rule ever drifts from
			// the invariant it implements, which no other check would notice.
			if len(r.Attempts) < 2 {
				return fmt.Errorf("retry report: succeeded request listed with a single attempt")
			}
		case OutcomeFailed, OutcomeCancelled:
			failed++
		default:
			return fmt.Errorf("retry report: unknown request outcome %q", r.Outcome)
		}

		retries += len(r.Attempts) - 1
		if len(r.Attempts) > 1 {
			retried++
		}
		if r.Model == "" || r.FilePath == "" || r.TaskType == "" || r.RequestNo <= 0 {
			return fmt.Errorf("retry report: incomplete request identity")
		}
	}

	if retries != rep.TotalRetries {
		return fmt.Errorf("retry report: total_retries %d != %d", rep.TotalRetries, retries)
	}
	if retried != rep.RetriedRequests {
		return fmt.Errorf("retry report: retried_requests %d != %d", rep.RetriedRequests, retried)
	}
	if recovered != rep.RecoveredRequests {
		return fmt.Errorf("retry report: recovered_requests %d != %d", rep.RecoveredRequests, recovered)
	}
	if failed != rep.FailedRequests {
		return fmt.Errorf("retry report: failed_requests %d != %d", rep.FailedRequests, failed)
	}
	return nil
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"net/http"
	"strconv"
	"time"
)

// retryObserver is the middleware signature both SDKs use. option.Middleware in
// anthropic-sdk-go and openai-go are type *aliases* for this exact function type,
// not defined types, so one observer value satisfies both without conversion —
// which is why the three clients share a single implementation.
type retryObserver = func(*http.Request, func(*http.Request) (*http.Response, error)) (*http.Response, error)

// newRetryObserver builds the middleware that feeds collector.
//
// It sits inside the SDK retry loop, so it runs once per real HTTP attempt and
// sees neither the backoff sleep (which happens around it) nor the response body
// (which the SDK reads after it returns). Both of those shape what the observer
// can honestly report: the measured gap between attempts covers the sleep, and
// anything that only surfaces while decoding the body is invisible here and is
// corrected at the client boundary instead.
//
// The observer never reads or closes the response body. The SDK closes it before
// retrying and reads it itself on the error path; consuming it here would break
// the SDK and put response content somewhere it must never go.
func newRetryObserver(collector *RetryCollector) retryObserver {
	return func(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		// Identity comes from the context the business layer attached before
		// calling the SDK. The per-attempt context the SDK builds is derived from
		// it, so the value is still visible here. Requests without identity —
		// scan and llm test — go straight through: the collector would drop them
		// anyway, and skipping the timestamps keeps those paths untouched.
		meta, ok := RequestMetaFromContext(req.Context())
		if collector == nil || !ok {
			return next(req)
		}

		startedAt := time.Now()
		res, err := next(req)
		endedAt := time.Now()

		collector.RecordAttempt(meta, observeAttempt(res, err, endedAt), startedAt, endedAt)
		return res, err
	}
}

// observeAttempt turns one transport result into an AttemptRecord.
//
// Classification runs whenever the status is non-2xx or the transport failed.
// The non-2xx half is not optional: RecordAttempt records an unclassified error
// status as a violation, because deriving it as a success would make the request
// vanish from the report while still being counted.
func observeAttempt(res *http.Response, err error, endedAt time.Time) AttemptRecord {
	a := AttemptRecord{}
	if res != nil {
		a.StatusCode = res.StatusCode
		a.RequestID = responseRequestID(res.Header)
		a.RetryAfterMS = parseRetryAfterMS(res.Header, endedAt)
		a.SDKRetryDirective = parseRetryDirective(res.Header)
	}
	if isErrorStatus(a.StatusCode) || err != nil {
		a.ErrorClass, a.FailurePhase = classifyAttempt(attemptObservation{StatusCode: a.StatusCode, Err: err})
	}
	return a
}

// responseRequestID reads the provider's request identifier. Anthropic sends
// request-id and OpenAI sends x-request-id; the observer is shared by all three
// clients, so it reads both rather than being parameterized by provider.
func responseRequestID(h http.Header) string {
	if v := h.Get("request-id"); v != "" {
		return v
	}
	return h.Get("x-request-id")
}

// parseRetryDirective reads x-should-retry, the header both SDKs consult ahead of
// the status code (and without excluding 2xx). Only the two values they act on
// are recorded; anything else is reported as absent rather than coerced, because
// the SDK ignores it too.
func parseRetryDirective(h http.Header) *bool {
	switch h.Get("x-should-retry") {
	case "true":
		v := true
		return &v
	case "false":
		v := false
		return &v
	}
	return nil
}

// parseRetryAfterMS reports the server's retry hint in milliseconds, mirroring
// the SDKs' parseRetryAfterHeader: Retry-After-Ms wins over Retry-After, each is
// first read as a number (milliseconds and seconds respectively), and only
// Retry-After falls back to an RFC1123 date.
//
// The date form is resolved against endedAt rather than a fresh time.Now so the
// value is a function of what this attempt observed. The two differ only by
// header-parsing time; the contract requires the same precedence as the SDK, not
// byte-identical numbers.
//
// A hint in the past yields zero, matching the SDK's max(0, delay): a negative
// wait is not a fact about the server, it just means the deadline has passed.
func parseRetryAfterMS(h http.Header, endedAt time.Time) int64 {
	for _, hdr := range []struct {
		name  string
		unit  time.Duration
		asRFC bool
	}{
		{name: "Retry-After-Ms", unit: time.Millisecond},
		{name: "Retry-After", unit: time.Second, asRFC: true},
	} {
		v := h.Get(hdr.name)
		if v == "" {
			continue
		}
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return nonNegativeMillis(time.Duration(n * float64(hdr.unit)))
		}
		if hdr.asRFC {
			if t, err := time.Parse(time.RFC1123, v); err == nil {
				return nonNegativeMillis(t.Sub(endedAt))
			}
		}
	}
	return 0
}

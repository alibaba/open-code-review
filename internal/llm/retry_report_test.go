// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

func errAttempt(class ErrorClass, phase FailurePhase, status int) AttemptRecord {
	return AttemptRecord{ErrorClass: class, FailurePhase: phase, StatusCode: status}
}

func okAttempt() AttemptRecord {
	return AttemptRecord{StatusCode: 200}
}

// The enum sets are fixed by the report contract: 8 error classes and 6 failure
// phases. Enumerating them pins both the membership and the size, so adding a
// class or a phase has to come with a deliberate edit here rather than silently
// widening what the report can emit.
func TestErrorClassAndFailurePhaseSets(t *testing.T) {
	classes := []ErrorClass{
		ErrorClassRateLimited, ErrorClassOverloaded, ErrorClassAuthentication,
		ErrorClassTimeout, ErrorClassNetwork, ErrorClassProvider,
		ErrorClassCancelled, ErrorClassUnknown,
	}
	if len(classes) != 8 {
		t.Fatalf("the contract fixes 8 error classes, enumerated %d", len(classes))
	}
	for _, c := range classes {
		if !c.valid() {
			t.Fatalf("error class %q rejected", c)
		}
	}
	for _, c := range []ErrorClass{"", "rate-limited", "Overloaded", "throttled"} {
		if c.valid() {
			t.Fatalf("error class %q accepted", c)
		}
	}

	phases := []FailurePhase{
		FailurePhaseTransport, FailurePhaseHTTP, FailurePhaseResponseDecode,
		FailurePhaseStream, FailurePhaseResponseStatus, FailurePhaseContext,
	}
	if len(phases) != 6 {
		t.Fatalf("the contract fixes 6 failure phases, enumerated %d", len(phases))
	}
	for _, p := range phases {
		if !p.valid() {
			t.Fatalf("failure phase %q rejected", p)
		}
	}
	for _, p := range []FailurePhase{"", "http_request", "HTTP", "decode"} {
		if p.valid() {
			t.Fatalf("failure phase %q accepted", p)
		}
	}
}

func TestClassifyAttempt(t *testing.T) {
	cases := []struct {
		name      string
		obs       attemptObservation
		wantClass ErrorClass
		wantPhase FailurePhase
	}{
		{"429", attemptObservation{StatusCode: 429}, ErrorClassRateLimited, FailurePhaseHTTP},
		{"529", attemptObservation{StatusCode: 529}, ErrorClassOverloaded, FailurePhaseHTTP},
		{"401", attemptObservation{StatusCode: 401}, ErrorClassAuthentication, FailurePhaseHTTP},
		{"403", attemptObservation{StatusCode: 403}, ErrorClassAuthentication, FailurePhaseHTTP},
		{"408", attemptObservation{StatusCode: 408}, ErrorClassTimeout, FailurePhaseHTTP},
		{"504", attemptObservation{StatusCode: 504}, ErrorClassTimeout, FailurePhaseHTTP},
		{"409", attemptObservation{StatusCode: 409}, ErrorClassProvider, FailurePhaseHTTP},
		{"402", attemptObservation{StatusCode: 402}, ErrorClassProvider, FailurePhaseHTTP},
		{"413", attemptObservation{StatusCode: 413}, ErrorClassProvider, FailurePhaseHTTP},
		{"500", attemptObservation{StatusCode: 500}, ErrorClassProvider, FailurePhaseHTTP},
		{"cancelled", attemptObservation{Err: context.Canceled}, ErrorClassCancelled, FailurePhaseContext},
		{"deadline", attemptObservation{Err: context.DeadlineExceeded}, ErrorClassTimeout, FailurePhaseContext},
		{"unexpected EOF", attemptObservation{Err: io.ErrUnexpectedEOF}, ErrorClassNetwork, FailurePhaseResponseDecode},
		{"wrapped EOF", attemptObservation{Err: fmt.Errorf("read body: %w", io.ErrUnexpectedEOF)}, ErrorClassNetwork, FailurePhaseResponseDecode},
		{"transport", attemptObservation{Err: errors.New("dial tcp: connection refused")}, ErrorClassNetwork, FailurePhaseTransport},
		{"no fact at all", attemptObservation{}, ErrorClassUnknown, FailurePhaseTransport},
		// Classifying a clean 2xx is a caller bug: there is no failure to
		// describe. It reports unknown rather than guessing, and keeps the phase
		// it does know — a response did arrive.
		{"clean 2xx has nothing to classify", attemptObservation{StatusCode: 200}, ErrorClassUnknown, FailurePhaseHTTP},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			class, phase := classifyAttempt(tc.obs)
			if class != tc.wantClass || phase != tc.wantPhase {
				t.Fatalf("got (%s, %s), want (%s, %s)", class, phase, tc.wantClass, tc.wantPhase)
			}
		})
	}
}

// A 2xx carries no error information, so the error decides. This is the path the
// client-boundary correction feeds when a 200 turns out to be truncated.
func TestClassifyAttemptTwoHundredFallsThroughToError(t *testing.T) {
	class, phase := classifyAttempt(attemptObservation{StatusCode: 200, Err: io.ErrUnexpectedEOF})
	if class != ErrorClassNetwork || phase != FailurePhaseResponseDecode {
		t.Fatalf("got (%s, %s), want (network, response_decode)", class, phase)
	}
}

func TestFinalizeDecisionOrder(t *testing.T) {
	cases := []struct {
		name            string
		attempts        []AttemptRecord
		reqErr          error
		parentCancelled bool
		want            Outcome
	}{
		{
			name:     "success after an error is recovered",
			attempts: []AttemptRecord{errAttempt(ErrorClassRateLimited, FailurePhaseHTTP, 429), okAttempt()},
			want:     OutcomeRecovered,
		},
		{
			name:     "success with no error attempt is succeeded",
			attempts: []AttemptRecord{okAttempt(), okAttempt()},
			want:     OutcomeSucceeded,
		},
		{
			name:     "returned error is failed",
			attempts: []AttemptRecord{errAttempt(ErrorClassOverloaded, FailurePhaseHTTP, 529)},
			reqErr:   errors.New("exhausted"),
			want:     OutcomeFailed,
		},
		{
			// Rule 2 outranks rule 3. Cancelling during backoff produces no new
			// attempt, so the sequence still ends in an error while the request
			// outcome is cancelled. Inferring from the last attempt would say
			// failed.
			name:            "cancellation outranks a returned error",
			attempts:        []AttemptRecord{errAttempt(ErrorClassRateLimited, FailurePhaseHTTP, 429)},
			reqErr:          context.Canceled,
			parentCancelled: true,
			want:            OutcomeCancelled,
		},
		{
			name:            "cancellation after a clean attempt",
			attempts:        []AttemptRecord{okAttempt()},
			parentCancelled: true,
			want:            OutcomeCancelled,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewRetryCollector()
			m := testMeta()
			for _, a := range tc.attempts {
				c.RecordAttempt(m, a)
			}
			c.Finalize(m, tc.reqErr, tc.parentCancelled)
			if got := c.entries[m].outcome; got != tc.want {
				t.Fatalf("outcome = %s, want %s", got, tc.want)
			}
		})
	}
}

// Rule 1: a logical request that never reached the observer produces no record
// at all, so it cannot inflate total_requests or appear in requests.
func TestFinalizeZeroAttemptProducesNoRecord(t *testing.T) {
	c := NewRetryCollector()
	m := testMeta()
	c.Finalize(m, errors.New("build request params"), false)

	if len(c.entries) != 0 {
		t.Fatalf("expected no entry, got %d", len(c.entries))
	}
	rep, err := c.Freeze("run-1")
	if err != nil {
		t.Fatalf("Freeze error: %v", err)
	}
	if rep != nil {
		t.Fatalf("expected no report, got %+v", rep)
	}
}

func TestRecordAttemptNumbersAndDerivesOutcome(t *testing.T) {
	c := NewRetryCollector()
	m := testMeta()

	// A caller-supplied number and outcome are ignored: numbering follows the
	// real call order, never the SDK's retry-count header.
	c.RecordAttempt(m, AttemptRecord{Number: 99, Outcome: AttemptError, StatusCode: 200})
	c.RecordAttempt(m, errAttempt(ErrorClassRateLimited, FailurePhaseHTTP, 429))

	got := c.entries[m].attempts
	if len(got) != 2 {
		t.Fatalf("got %d attempts, want 2", len(got))
	}
	if got[0].Number != 1 || got[0].Outcome != AttemptSuccess {
		t.Fatalf("attempt 1 = %+v, want number 1 and success", got[0])
	}
	if got[0].ErrorClass != "" || got[0].FailurePhase != "" {
		t.Fatalf("success attempt kept error fields: %+v", got[0])
	}
	if got[1].Number != 2 || got[1].Outcome != AttemptError {
		t.Fatalf("attempt 2 = %+v, want number 2 and error", got[1])
	}
}

// An observer that reports a non-2xx status without classifying it contradicts
// the strongest fact it had. The failure mode this closes is silence, not noise:
// derived as a success, the attempt makes Finalize decide succeeded, the listing
// rule skips the request, and a failed request disappears from requests while
// total_requests still counts it. validateReport only walks listed requests and
// cannot see it.
func TestRecordAttemptRejectsUnclassifiedErrorStatus(t *testing.T) {
	c := NewRetryCollector()
	m := testMeta()
	c.RecordAttempt(m, AttemptRecord{StatusCode: 500})
	c.Finalize(m, nil, false)

	// The attempt is kept so the state stays inspectable...
	if got := c.entries[m].attempts; len(got) != 1 {
		t.Fatalf("got %d attempts, want the attempt to be kept", len(got))
	}
	// ...but the report is refused instead of silently omitting the request.
	rep, err := c.Freeze("run-1")
	if err == nil {
		t.Fatalf("expected a construction error, got report %+v", rep)
	}
	if rep != nil {
		t.Fatalf("a failed Freeze must not return a report: %+v", rep)
	}
	if !strings.Contains(err.Error(), "without a classification") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// The guard keys off the same boundary as classifyAttempt, so a 2xx never trips
// it: an unclassified 200 is the normal success path, and an unclassified 200
// with an extra attempt is the x-should-retry case.
func TestRecordAttemptAcceptsUnclassifiedSuccessStatus(t *testing.T) {
	for _, status := range []int{200, 201, 299, 0} {
		c := NewRetryCollector()
		m := testMeta()
		c.RecordAttempt(m, AttemptRecord{StatusCode: status})
		if got := c.entries[m].violation; got != "" {
			t.Fatalf("status %d flagged a violation: %q", status, got)
		}
	}
}

func TestRecordAttemptDropsRequestsWithoutIdentity(t *testing.T) {
	c := NewRetryCollector()
	m := testMeta()
	m.TaskType = "" // e.g. a scan or llm test request: no RequestMeta in context
	c.RecordAttempt(m, errAttempt(ErrorClassRateLimited, FailurePhaseHTTP, 429))

	if len(c.entries) != 0 {
		t.Fatalf("expected the attempt to be dropped, got %d entries", len(c.entries))
	}
}

// Every method tolerates a nil collector. This is the design premise that the
// reporting path can never fail a review: a call site that has no collector — or
// a future one that forgets to wire it — degrades to no report instead of
// panicking mid-review.
func TestNilCollectorIsInert(t *testing.T) {
	var c *RetryCollector
	m := testMeta()

	c.RecordAttempt(m, errAttempt(ErrorClassRateLimited, FailurePhaseHTTP, 429))
	c.ReviseLastAttempt(m, ErrorClassNetwork, FailurePhaseResponseDecode)
	c.Finalize(m, errors.New("boom"), true)

	rep, err := c.Freeze("run-1")
	if rep != nil || err != nil {
		t.Fatalf("Freeze on a nil collector = (%+v, %v), want (nil, nil)", rep, err)
	}
}

// Invalid input is dropped rather than recorded. An unclassifiable correction is
// the dangerous case: writing it through would put an empty or bogus class on an
// attempt, and validateReport would then reject the whole report at Freeze —
// turning a caller mistake into a suppressed report.
func TestCollectorRejectsInvalidInput(t *testing.T) {
	m := testMeta()
	invalid := testMeta()
	invalid.TaskType = ""

	t.Run("revision with an unknown class or phase", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			class ErrorClass
			phase FailurePhase
		}{
			{"empty class", "", FailurePhaseResponseDecode},
			{"empty phase", ErrorClassNetwork, ""},
			{"bogus class", ErrorClass("flaky"), FailurePhaseResponseDecode},
			{"bogus phase", ErrorClassNetwork, FailurePhase("decoding")},
		} {
			t.Run(tc.name, func(t *testing.T) {
				c := NewRetryCollector()
				c.RecordAttempt(m, okAttempt())
				c.ReviseLastAttempt(m, tc.class, tc.phase)
				if got := c.entries[m].attempts[0]; got.Outcome != AttemptSuccess {
					t.Fatalf("attempt was revised with invalid input: %+v", got)
				}
			})
		}
	})

	t.Run("revision without identity", func(t *testing.T) {
		c := NewRetryCollector()
		c.RecordAttempt(m, okAttempt())
		c.ReviseLastAttempt(invalid, ErrorClassNetwork, FailurePhaseResponseDecode)
		if got := c.entries[m].attempts[0]; got.Outcome != AttemptSuccess {
			t.Fatalf("an unidentified revision reached another entry: %+v", got)
		}
	})

	// A correction can arrive before any attempt was observed (the client
	// boundary sees a decode error on a request that never entered the observer).
	// There is nothing to revise and nothing to invent.
	t.Run("revision with no attempt to revise", func(t *testing.T) {
		c := NewRetryCollector()
		c.ReviseLastAttempt(m, ErrorClassNetwork, FailurePhaseResponseDecode)
		if len(c.entries) != 0 {
			t.Fatalf("revision created an entry: %+v", c.entries)
		}
	})

	t.Run("finalize without identity", func(t *testing.T) {
		c := NewRetryCollector()
		c.RecordAttempt(m, okAttempt())
		c.Finalize(invalid, nil, false)
		if c.entries[m].finalized {
			t.Fatal("an unidentified Finalize finalized another entry")
		}
	})
}

func TestReviseLastAttempt(t *testing.T) {
	t.Run("rewrites a success attempt", func(t *testing.T) {
		c := NewRetryCollector()
		m := testMeta()
		c.RecordAttempt(m, okAttempt())
		c.ReviseLastAttempt(m, ErrorClassNetwork, FailurePhaseResponseDecode)

		last := c.entries[m].attempts[0]
		if last.Outcome != AttemptError || last.ErrorClass != ErrorClassNetwork ||
			last.FailurePhase != FailurePhaseResponseDecode {
			t.Fatalf("attempt not corrected: %+v", last)
		}
		if last.StatusCode != 200 {
			t.Fatalf("correction dropped the observed status code: %+v", last)
		}
	})

	// The precondition. A 500 already classified from its status code must not be
	// rewritten to unknown/response_decode just because its error body failed to
	// parse: the status code is the stronger fact.
	t.Run("never overwrites a status-code classification", func(t *testing.T) {
		c := NewRetryCollector()
		m := testMeta()
		c.RecordAttempt(m, errAttempt(ErrorClassProvider, FailurePhaseHTTP, 500))
		c.ReviseLastAttempt(m, ErrorClassUnknown, FailurePhaseResponseDecode)

		last := c.entries[m].attempts[0]
		if last.ErrorClass != ErrorClassProvider || last.FailurePhase != FailurePhaseHTTP {
			t.Fatalf("status-code classification was overwritten: %+v", last)
		}
	})

	t.Run("stream and response status phases", func(t *testing.T) {
		for _, phase := range []FailurePhase{FailurePhaseStream, FailurePhaseResponseStatus} {
			c := NewRetryCollector()
			m := testMeta()
			c.RecordAttempt(m, okAttempt())
			c.ReviseLastAttempt(m, ErrorClassProvider, phase)
			if got := c.entries[m].attempts[0].FailurePhase; got != phase {
				t.Fatalf("phase = %s, want %s", got, phase)
			}
		}
	})
}

func TestFreezeReturnsNothingWhenNoRetryHappened(t *testing.T) {
	c := NewRetryCollector()
	m := testMeta()
	c.RecordAttempt(m, okAttempt())
	c.Finalize(m, nil, false)

	rep, err := c.Freeze("run-1")
	if err != nil {
		t.Fatalf("Freeze error: %v", err)
	}
	if rep != nil {
		t.Fatalf("expected no report for a first-try success, got %+v", rep)
	}
}

func TestFreezeAggregatesAndSorts(t *testing.T) {
	c := NewRetryCollector()

	recovered := testMeta()
	recovered.RequestNo = 2
	c.RecordAttempt(recovered, errAttempt(ErrorClassRateLimited, FailurePhaseHTTP, 429))
	c.RecordAttempt(recovered, okAttempt())
	c.Finalize(recovered, nil, false)

	failed := testMeta()
	failed.FilePath = "config.go"
	c.RecordAttempt(failed, errAttempt(ErrorClassProvider, FailurePhaseHTTP, 402))
	c.Finalize(failed, errors.New("payment required"), false)

	// A first-try success stays out of requests but still counts in
	// total_requests.
	quiet := testMeta()
	quiet.FilePath = "quiet.go"
	c.RecordAttempt(quiet, okAttempt())
	c.Finalize(quiet, nil, false)

	rep, err := c.Freeze("run-1")
	if err != nil {
		t.Fatalf("Freeze error: %v", err)
	}
	if rep == nil {
		t.Fatal("expected a report")
	}
	if rep.SchemaVersion != RetryReportSchemaVersion {
		t.Fatalf("schema version = %q", rep.SchemaVersion)
	}
	if rep.TotalRequests != 3 || rep.RetriedRequests != 1 || rep.TotalRetries != 1 ||
		rep.RecoveredRequests != 1 || rep.FailedRequests != 1 {
		t.Fatalf("aggregates wrong: %+v", *rep)
	}
	if len(rep.Requests) != 2 {
		t.Fatalf("listed %d requests, want 2", len(rep.Requests))
	}
	ids := []string{rep.Requests[0].LogicalRequestID, rep.Requests[1].LogicalRequestID}
	if !sort.StringsAreSorted(ids) {
		t.Fatalf("requests are not sorted by logical_request_id: %v", ids)
	}
}

// A request cancelled after a single clean attempt has no error attempt and no
// retry, yet it counts in failed_requests. It must still be listed, or the
// aggregates would not be verifiable from the report alone.
func TestFreezeListsCancelledRequestWithoutErrorAttempt(t *testing.T) {
	c := NewRetryCollector()
	m := testMeta()
	c.RecordAttempt(m, okAttempt())
	c.Finalize(m, nil, true)

	rep, err := c.Freeze("run-1")
	if err != nil {
		t.Fatalf("Freeze error: %v", err)
	}
	if rep == nil || len(rep.Requests) != 1 {
		t.Fatalf("cancelled request was not listed: %+v", rep)
	}
	if rep.Requests[0].Outcome != OutcomeCancelled || rep.FailedRequests != 1 {
		t.Fatalf("unexpected report: %+v", *rep)
	}
}

// The x-should-retry case: a retry with no error at all. "1 retry, 0 recovered,
// 0 failed" is the correct answer, not a counting bug.
func TestFreezeSucceededRequestWithExtraAttempt(t *testing.T) {
	c := NewRetryCollector()
	m := testMeta()
	directive := true
	c.RecordAttempt(m, AttemptRecord{StatusCode: 200, SDKRetryDirective: &directive})
	c.RecordAttempt(m, okAttempt())
	c.Finalize(m, nil, false)

	rep, err := c.Freeze("run-1")
	if err != nil {
		t.Fatalf("Freeze error: %v", err)
	}
	if rep == nil || len(rep.Requests) != 1 {
		t.Fatalf("expected the extra attempt to be reported: %+v", rep)
	}
	r := rep.Requests[0]
	if r.Outcome != OutcomeSucceeded {
		t.Fatalf("outcome = %s, want succeeded", r.Outcome)
	}
	if rep.RetriedRequests != 1 || rep.TotalRetries != 1 {
		t.Fatalf("retry counts wrong: %+v", *rep)
	}
	if rep.RecoveredRequests != 0 || rep.FailedRequests != 0 {
		t.Fatalf("nothing was recovered or failed: %+v", *rep)
	}
}

func TestFreezeRejectsOrderingViolations(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*RetryCollector, RequestMeta)
		want string
	}{
		{
			name: "double finalize",
			mut: func(c *RetryCollector, m RequestMeta) {
				c.Finalize(m, nil, false)
				c.Finalize(m, nil, false)
			},
			want: "Finalize called more than once",
		},
		{
			name: "attempt after finalize",
			mut: func(c *RetryCollector, m RequestMeta) {
				c.Finalize(m, nil, false)
				c.RecordAttempt(m, okAttempt())
			},
			want: "attempt recorded after Finalize",
		},
		{
			name: "revision after finalize",
			mut: func(c *RetryCollector, m RequestMeta) {
				c.Finalize(m, nil, false)
				c.ReviseLastAttempt(m, ErrorClassNetwork, FailurePhaseResponseDecode)
			},
			want: "attempt revised after Finalize",
		},
		{
			name: "never finalized",
			mut:  func(*RetryCollector, RequestMeta) {},
			want: "not finalized",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewRetryCollector()
			m := testMeta()
			c.RecordAttempt(m, errAttempt(ErrorClassRateLimited, FailurePhaseHTTP, 429))
			c.RecordAttempt(m, okAttempt())
			tc.mut(c, m)

			rep, err := c.Freeze("run-1")
			if err == nil {
				t.Fatalf("expected a construction error, got report %+v", rep)
			}
			if rep != nil {
				t.Fatalf("a failed Freeze must not return a report: %+v", rep)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

// A construction error has to name the offending request: with --concurrency > 1
// a run holds hundreds of logical requests, and the error reaches the user as
// part of runErr with no other context.
func TestFreezeErrorIdentifiesTheRequest(t *testing.T) {
	c := NewRetryCollector()
	m := testMeta()
	m.FilePath = "internal/pay/charge.go"
	m.TaskType = "review_filter"
	m.RequestNo = 7
	c.RecordAttempt(m, okAttempt()) // recorded but never finalized

	_, err := c.Freeze("run-1")
	if err == nil {
		t.Fatal("expected a construction error")
	}
	for _, want := range []string{"not finalized", "file=internal/pay/charge.go", "task=review_filter", "request_no=7"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}

// Freeze walks entries in logical_request_id order rather than map order, so a
// collector holding several broken entries always blames the same one. Under map
// order this error would rotate between runs and could not be pinned by a test
// or matched against a bug report.
func TestFreezeErrorIsDeterministic(t *testing.T) {
	build := func() *RetryCollector {
		c := NewRetryCollector()
		for i := 1; i <= 8; i++ {
			m := testMeta()
			m.FilePath = fmt.Sprintf("file%02d.go", i)
			m.RequestNo = i
			c.RecordAttempt(m, okAttempt()) // none of them finalized
		}
		return c
	}

	first, err := build().Freeze("run-1")
	if err == nil {
		t.Fatalf("expected a construction error, got %+v", first)
	}
	for i := 0; i < 20; i++ {
		_, again := build().Freeze("run-1")
		if again == nil || again.Error() != err.Error() {
			t.Fatalf("error is not deterministic: %v vs %v", err, again)
		}
	}
}

func TestFreezeRejectsInvalidRunID(t *testing.T) {
	for _, runID := range []string{"", "run\x001"} {
		c := NewRetryCollector()
		m := testMeta()
		c.RecordAttempt(m, errAttempt(ErrorClassRateLimited, FailurePhaseHTTP, 429))
		c.RecordAttempt(m, okAttempt())
		c.Finalize(m, nil, false)

		rep, err := c.Freeze(runID)
		if err == nil || rep != nil {
			t.Fatalf("run_id %q: got (%+v, %v), want (nil, error)", runID, rep, err)
		}
	}
}

// Not reachable through the API: RecordAttempt is the only thing that creates an
// entry and it always appends. The entry is built directly because this guard is
// the last line of defense for "total_requests only counts metas with an
// attempt" — if some later layer registers a meta up front, the report must
// refuse rather than emit a request with an empty attempts array.
func TestFreezeRefusesEntryWithNoAttempt(t *testing.T) {
	c := NewRetryCollector()
	m := testMeta()
	c.entries[m] = &requestEntry{finalized: true, outcome: OutcomeSucceeded}

	rep, err := c.Freeze("run-1")
	if err == nil || rep != nil {
		t.Fatalf("got (%+v, %v), want (nil, error)", rep, err)
	}
	if !strings.Contains(err.Error(), "no attempt") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// The point is not the tampering — Freeze's own accounting cannot produce broken
// numbering. It is that validateReport is wired into Freeze rather than merely
// callable on its own, so a broken invariant suppresses the report instead of
// publishing self-contradictory numbers.
func TestFreezeSuppressesReportWhenValidationFails(t *testing.T) {
	c := NewRetryCollector()
	m := testMeta()
	c.RecordAttempt(m, errAttempt(ErrorClassRateLimited, FailurePhaseHTTP, 429))
	c.RecordAttempt(m, okAttempt())
	c.Finalize(m, nil, false)
	c.entries[m].attempts[1].Number = 7

	rep, err := c.Freeze("run-1")
	if err == nil || rep != nil {
		t.Fatalf("got (%+v, %v), want (nil, error)", rep, err)
	}
	if !strings.Contains(err.Error(), "numbering") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateReportCatchesInconsistency(t *testing.T) {
	base := func() *RetryReport {
		return &RetryReport{
			SchemaVersion:     RetryReportSchemaVersion,
			TotalRequests:     1,
			RetriedRequests:   1,
			TotalRetries:      1,
			RecoveredRequests: 1,
			Requests: []RequestReport{{
				LogicalRequestID: "a",
				Model:            "m",
				FilePath:         "f",
				TaskType:         "t",
				RequestNo:        1,
				Outcome:          OutcomeRecovered,
				Attempts: []AttemptRecord{
					{Number: 1, Outcome: AttemptError, ErrorClass: ErrorClassRateLimited, FailurePhase: FailurePhaseHTTP, StatusCode: 429},
					{Number: 2, Outcome: AttemptSuccess, StatusCode: 200},
				},
			}},
		}
	}
	if err := validateReport(base()); err != nil {
		t.Fatalf("baseline report rejected: %v", err)
	}

	cases := map[string]func(*RetryReport){
		"total requests below listed":   func(r *RetryReport) { r.TotalRequests = 0 },
		"retry count mismatch":          func(r *RetryReport) { r.TotalRetries = 5 },
		"retried count mismatch":        func(r *RetryReport) { r.RetriedRequests = 5 },
		"recovered count mismatch":      func(r *RetryReport) { r.RecoveredRequests = 0 },
		"failed count mismatch":         func(r *RetryReport) { r.FailedRequests = 1 },
		"non contiguous numbering":      func(r *RetryReport) { r.Requests[0].Attempts[1].Number = 3 },
		"recovered without error":       func(r *RetryReport) { r.Requests[0].Attempts[0] = AttemptRecord{Number: 1, Outcome: AttemptSuccess} },
		"success attempt carries class": func(r *RetryReport) { r.Requests[0].Attempts[1].ErrorClass = ErrorClassUnknown },
		"error attempt unclassified":    func(r *RetryReport) { r.Requests[0].Attempts[0].ErrorClass = "" },
		"error attempt unphased":        func(r *RetryReport) { r.Requests[0].Attempts[0].FailurePhase = "" },
		"unknown attempt outcome":       func(r *RetryReport) { r.Requests[0].Attempts[1].Outcome = AttemptOutcome("weird") },
		"unknown outcome":               func(r *RetryReport) { r.Requests[0].Outcome = Outcome("weird") },
		"request with no attempt":       func(r *RetryReport) { r.Requests[0].Attempts = nil },
		"unexpected schema version":     func(r *RetryReport) { r.SchemaVersion = "ocr.llm-retry-report/v0" },
		"incomplete identity":           func(r *RetryReport) { r.Requests[0].FilePath = "" },
		// The other half of the succeeded contract: the listing rule keeps a
		// clean single-attempt success out of the report, and an error attempt
		// makes the outcome recovered, so a succeeded request carrying one is a
		// contradiction from either direction.
		"succeeded with an error attempt": func(r *RetryReport) { r.Requests[0].Outcome = OutcomeSucceeded },
		"duplicate id": func(r *RetryReport) {
			r.Requests = append(r.Requests, r.Requests[0])
			r.TotalRequests = 2
			r.TotalRetries = 2
			r.RetriedRequests = 2
			r.RecoveredRequests = 2
		},
		"succeeded with a single attempt": func(r *RetryReport) {
			r.Requests[0].Outcome = OutcomeSucceeded
			r.Requests[0].Attempts = []AttemptRecord{{Number: 1, Outcome: AttemptSuccess}}
			r.TotalRetries = 0
			r.RetriedRequests = 0
			r.RecoveredRequests = 0
		},
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			r := base()
			mut(r)
			if err := validateReport(r); err == nil {
				t.Fatal("expected an invariant violation")
			}
		})
	}
}

// provider is required but may be empty: an empty string stably denotes an
// unnamed endpoint and must not be omitted.
func TestProviderIsEmittedEvenWhenEmpty(t *testing.T) {
	c := NewRetryCollector()
	m := testMeta()
	m.Provider = ""
	c.RecordAttempt(m, errAttempt(ErrorClassRateLimited, FailurePhaseHTTP, 429))
	c.RecordAttempt(m, okAttempt())
	c.Finalize(m, nil, false)

	rep, err := c.Freeze("run-1")
	if err != nil || rep == nil {
		t.Fatalf("Freeze = (%+v, %v)", rep, err)
	}
	blob, err := json.Marshal(rep.Requests[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(blob), `"provider":""`) {
		t.Fatalf("provider was omitted: %s", blob)
	}
}

// The security guarantee is structural rather than filter-based: the report has
// no free-text field, so there is nothing to redact. This test pins the exact
// set of plain-string fields, so adding one (an error message, a URL, a prompt)
// fails here and has to be argued for explicitly. Enum types are excluded
// automatically because their reflect.Type is not plain string.
func TestRetryReportHasNoUnexpectedTextFields(t *testing.T) {
	want := []string{
		"RetryReport.Requests[].Attempts[].RequestID",
		"RetryReport.Requests[].FilePath",
		"RetryReport.Requests[].LogicalRequestID",
		"RetryReport.Requests[].Model",
		"RetryReport.Requests[].Provider",
		"RetryReport.Requests[].TaskType",
		"RetryReport.SchemaVersion",
	}

	var got []string
	var walk func(t reflect.Type, path string)
	walk = func(rt reflect.Type, path string) {
		switch rt.Kind() {
		case reflect.Slice, reflect.Ptr:
			walk(rt.Elem(), path+"[]")
		case reflect.Struct:
			for i := 0; i < rt.NumField(); i++ {
				f := rt.Field(i)
				walk(f.Type, path+"."+f.Name)
			}
		case reflect.String:
			if rt == reflect.TypeOf("") {
				got = append(got, path)
			}
		}
	}
	walk(reflect.TypeOf(RetryReport{}), "RetryReport")
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plain string fields changed:\n got %v\nwant %v", got, want)
	}
}

func TestRetryCollectorConcurrentUse(t *testing.T) {
	c := NewRetryCollector()
	const requests = 32

	var wg sync.WaitGroup
	for i := 1; i <= requests; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m := testMeta()
			m.FilePath = fmt.Sprintf("file%02d.go", n)
			m.RequestNo = n
			c.RecordAttempt(m, errAttempt(ErrorClassRateLimited, FailurePhaseHTTP, 429))
			c.RecordAttempt(m, okAttempt())
			c.Finalize(m, nil, false)
		}(i)
	}
	wg.Wait()

	rep, err := c.Freeze("run-1")
	if err != nil {
		t.Fatalf("Freeze error: %v", err)
	}
	if rep == nil {
		t.Fatal("expected a report")
	}
	if rep.TotalRequests != requests || rep.RecoveredRequests != requests ||
		rep.TotalRetries != requests || len(rep.Requests) != requests {
		t.Fatalf("aggregates wrong under concurrency: %+v", *rep)
	}
	ids := make([]string, len(rep.Requests))
	for i, r := range rep.Requests {
		ids[i] = r.LogicalRequestID
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatal("report ordering is not stable")
	}
}

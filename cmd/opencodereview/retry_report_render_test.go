// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

// retryReportFixture is the report used by most rendering assertions: one
// request recovered after two errors, one request that never succeeded, and a
// total_requests larger than the listed set (the first-try successes are
// counted but not listed).
func retryReportFixture() *llm.RetryReport {
	return &llm.RetryReport{
		SchemaVersion:     llm.RetryReportSchemaVersion,
		TotalRequests:     12,
		RetriedRequests:   1,
		TotalRetries:      2,
		RecoveredRequests: 1,
		FailedRequests:    1,
		Requests: []llm.RequestReport{
			{
				LogicalRequestID: "aaa",
				Model:            "claude-test",
				FilePath:         "payment.go",
				TaskType:         "main_task",
				RequestNo:        2,
				Outcome:          llm.OutcomeRecovered,
				Attempts: []llm.AttemptRecord{
					{Number: 1, Outcome: llm.AttemptError, ErrorClass: llm.ErrorClassRateLimited, FailurePhase: llm.FailurePhaseHTTP, StatusCode: 429},
					{Number: 2, Outcome: llm.AttemptError, ErrorClass: llm.ErrorClassOverloaded, FailurePhase: llm.FailurePhaseHTTP, StatusCode: 529},
					{Number: 3, Outcome: llm.AttemptSuccess},
				},
			},
			{
				LogicalRequestID: "bbb",
				Model:            "claude-test",
				FilePath:         "config.go",
				TaskType:         "main_task",
				RequestNo:        1,
				Outcome:          llm.OutcomeFailed,
				Attempts: []llm.AttemptRecord{
					{Number: 1, Outcome: llm.AttemptError, ErrorClass: llm.ErrorClassProvider, FailurePhase: llm.FailurePhaseHTTP, StatusCode: 402},
				},
			},
		},
	}
}

// The expected rendering is fixed by docs/368/LLM请求重试实施细节.md §5, so the
// text contract is asserted whole rather than by substring.
const wantRetryReportText = `
LLM retry report: 1/12 requests retried, 2 retries, 1 recovered, 1 failed, 0 cancelled
- payment.go / main_task #2: rate_limited(429) -> overloaded(529) -> success
- config.go / main_task #1: provider(402) -> failed
`

func TestOutputRetryReportText_RecoveredAndFailed(t *testing.T) {
	var buf bytes.Buffer
	outputRetryReportText(&buf, retryReportFixture())
	if got := buf.String(); got != wantRetryReportText {
		t.Errorf("text report mismatch\n got: %q\nwant: %q", got, wantRetryReportText)
	}
}

func TestOutputRetryReportText_NilWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	outputRetryReportText(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("nil report must write nothing, got %q", buf.String())
	}
}

func TestOutputRetryReportText_SingularRetry(t *testing.T) {
	rep := retryReportFixture()
	rep.TotalRetries = 1
	var buf bytes.Buffer
	outputRetryReportText(&buf, rep)
	if !strings.Contains(buf.String(), "1 retry,") {
		t.Errorf("total_retries=1 must read %q, got %q", "1 retry,", buf.String())
	}
}

// A retry that ended in success without any error attempt is a real outcome:
// an HTTP 200 carrying x-should-retry: true makes the SDK attempt again. The
// summary then shows a retry with zero recovered and zero failed, which is
// expected rather than a bug (see the roadmap's risk table).
func TestOutputRetryReportText_SucceededAfterRetry(t *testing.T) {
	rep := &llm.RetryReport{
		SchemaVersion:   llm.RetryReportSchemaVersion,
		TotalRequests:   1,
		RetriedRequests: 1,
		TotalRetries:    1,
		Requests: []llm.RequestReport{{
			LogicalRequestID: "aaa",
			Model:            "claude-test",
			FilePath:         "payment.go",
			TaskType:         "main_task",
			RequestNo:        2,
			Outcome:          llm.OutcomeSucceeded,
			Attempts: []llm.AttemptRecord{
				{Number: 1, Outcome: llm.AttemptSuccess},
				{Number: 2, Outcome: llm.AttemptSuccess},
			},
		}},
	}
	want := "\nLLM retry report: 1/1 requests retried, 1 retry, 0 recovered, 0 failed, 0 cancelled\n" +
		"- payment.go / main_task #2: success -> success\n"
	var buf bytes.Buffer
	outputRetryReportText(&buf, rep)
	if got := buf.String(); got != want {
		t.Errorf("text report mismatch\n got: %q\nwant: %q", got, want)
	}
}

// A cancelled request keeps its clean attempt and is told apart from a provider
// failure only by the trailing outcome, so that suffix is part of the contract.
func TestOutputRetryReportText_CancelledSuffix(t *testing.T) {
	rep := &llm.RetryReport{
		SchemaVersion:     llm.RetryReportSchemaVersion,
		TotalRequests:     1,
		CancelledRequests: 1,
		Requests: []llm.RequestReport{{
			LogicalRequestID: "aaa",
			Model:            "claude-test",
			FilePath:         "payment.go",
			TaskType:         "memory_compression_task",
			RequestNo:        1,
			Outcome:          llm.OutcomeCancelled,
			Attempts:         []llm.AttemptRecord{{Number: 1, Outcome: llm.AttemptSuccess}},
		}},
	}
	want := "\nLLM retry report: 0/1 requests retried, 0 retries, 0 recovered, 0 failed, 1 cancelled\n" +
		"- payment.go / memory_compression_task #1: success -> cancelled\n"
	var buf bytes.Buffer
	outputRetryReportText(&buf, rep)
	if got := buf.String(); got != want {
		t.Errorf("text report mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestRetryAttemptChain_CancelledAttemptNotDuplicated(t *testing.T) {
	r := llm.RequestReport{
		Outcome: llm.OutcomeCancelled,
		Attempts: []llm.AttemptRecord{{
			Number: 1, Outcome: llm.AttemptError,
			ErrorClass: llm.ErrorClassCancelled, FailurePhase: llm.FailurePhaseContext,
		}},
	}
	if got, want := retryAttemptChain(r), "cancelled"; got != want {
		t.Errorf("chain = %q, want %q", got, want)
	}
}

// An attempt with no HTTP response has no status code to show, so the class is
// rendered bare rather than as "network(0)".
func TestRetryAttemptChain_NoStatusCode(t *testing.T) {
	r := llm.RequestReport{
		Outcome: llm.OutcomeFailed,
		Attempts: []llm.AttemptRecord{
			{Number: 1, Outcome: llm.AttemptError, ErrorClass: llm.ErrorClassNetwork, FailurePhase: llm.FailurePhaseTransport},
		},
	}
	if got, want := retryAttemptChain(r), "network -> failed"; got != want {
		t.Errorf("chain = %q, want %q", got, want)
	}
}

func TestOutputRetryReportText_SanitizesControlChars(t *testing.T) {
	rep := retryReportFixture()
	rep.Requests[0].FilePath = "pay\x1b[31mment.go"
	rep.Requests[0].TaskType = "main\x07_task"
	var buf bytes.Buffer
	outputRetryReportText(&buf, rep)
	if strings.ContainsAny(buf.String(), "\x1b\x07") {
		t.Errorf("control characters must be stripped, got %q", buf.String())
	}
}

// The report carries only aggregates, stable classes, status codes and request
// identity. This pins the emitted JSON key set so a future field cannot quietly
// add a prompt, URL, header or raw provider error string.
func TestRetryReportJSON_KeySetIsAllowlisted(t *testing.T) {
	raw, err := json.Marshal(retryReportFixture())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	allowedTop := map[string]bool{
		"schema_version": true, "total_requests": true, "retried_requests": true,
		"total_retries": true, "recovered_requests": true, "failed_requests": true,
		"cancelled_requests": true,
		"requests":           true,
	}
	for k := range top {
		if !allowedTop[k] {
			t.Errorf("unexpected top-level key %q in retry report", k)
		}
	}

	var reqs []map[string]json.RawMessage
	if err := json.Unmarshal(top["requests"], &reqs); err != nil {
		t.Fatalf("unmarshal requests: %v", err)
	}
	allowedReq := map[string]bool{
		"logical_request_id": true, "provider": true, "model": true,
		"file_path": true, "task_type": true, "request_no": true,
		"outcome": true, "attempts": true,
	}
	allowedAttempt := map[string]bool{
		"attempt": true, "outcome": true, "error_class": true, "failure_phase": true,
		"status_code": true, "request_id": true, "retry_after_ms": true,
		"observed_backoff_ms": true, "duration_to_headers_ms": true,
		"sdk_retry_directive": true,
	}
	for _, r := range reqs {
		for k := range r {
			if !allowedReq[k] {
				t.Errorf("unexpected request key %q in retry report", k)
			}
		}
		var attempts []map[string]json.RawMessage
		if err := json.Unmarshal(r["attempts"], &attempts); err != nil {
			t.Fatalf("unmarshal attempts: %v", err)
		}
		for _, a := range attempts {
			for k := range a {
				if !allowedAttempt[k] {
					t.Errorf("unexpected attempt key %q in retry report", k)
				}
			}
		}
	}
}

// provider is required and must survive as an empty string: an OCR_LLM_*
// endpoint has no provider name, and omitting the key there would make an
// unnamed endpoint indistinguishable from a missing field.
func TestRetryReportJSON_EmptyProviderKept(t *testing.T) {
	raw, err := json.Marshal(retryReportFixture().Requests[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"provider":""`)) {
		t.Errorf("empty provider must still be emitted, got %s", raw)
	}
}

// Both exits read the same frozen value, so a single collector Freeze must
// render identically through the terminal and the JSON output. Built through
// the real collector rather than a literal so the rendered numbers are ones
// Freeze itself validated.
func TestRetryReport_TerminalAndJSONReadSameFrozenResult(t *testing.T) {
	c := llm.NewRetryCollector()
	base := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

	recovered := llm.RequestMeta{Model: "claude-test", FilePath: "a.go", TaskType: "main_task", RequestNo: 1}
	c.RecordAttempt(recovered, llm.AttemptRecord{
		ErrorClass: llm.ErrorClassRateLimited, FailurePhase: llm.FailurePhaseHTTP, StatusCode: 429,
	}, base, base.Add(10*time.Millisecond))
	c.RecordAttempt(recovered, llm.AttemptRecord{}, base.Add(time.Second), base.Add(time.Second+10*time.Millisecond))
	c.Finalize(recovered, nil, false)

	failed := llm.RequestMeta{Model: "claude-test", FilePath: "b.go", TaskType: "main_task", RequestNo: 1}
	c.RecordAttempt(failed, llm.AttemptRecord{
		ErrorClass: llm.ErrorClassProvider, FailurePhase: llm.FailurePhaseHTTP, StatusCode: 402,
	}, base, base.Add(5*time.Millisecond))
	c.Finalize(failed, context.DeadlineExceeded, false)

	rep, err := c.Freeze("run-uuid")
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if rep == nil {
		t.Fatal("expected a report")
	}

	var text bytes.Buffer
	outputRetryReportText(&text, rep)

	ag := &mockResultProvider{filesReviewed: 2, manifest: mockManifest(session.StateComplete)}
	jsonGot := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "json", "developer", nil, nil, rep); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(jsonGot), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.RetryReport == nil {
		t.Fatal("retry_report missing")
	}

	wantHeader := "LLM retry report: 1/2 requests retried, 1 retry, 1 recovered, 1 failed, 0 cancelled"
	if !strings.Contains(text.String(), wantHeader) {
		t.Errorf("terminal header = %q, want it to contain %q", text.String(), wantHeader)
	}
	if out.RetryReport.RetriedRequests != 1 || out.RetryReport.TotalRequests != 2 ||
		out.RetryReport.TotalRetries != 1 || out.RetryReport.RecoveredRequests != 1 ||
		out.RetryReport.FailedRequests != 1 {
		t.Errorf("JSON aggregates disagree with the frozen report: %+v", out.RetryReport)
	}
	for _, r := range out.RetryReport.Requests {
		if !strings.Contains(text.String(), r.FilePath) {
			t.Errorf("%s listed in JSON but not in the terminal summary:\n%s", r.FilePath, text.String())
		}
	}
}

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

// The expected rendering is fixed, so the text contract is asserted whole
// rather than by substring. Failures list before recoveries, and the internal
// task_type/request_no identity the JSON report carries stays out of the
// terminal.
const wantRetryReportText = `
LLM retry report summary: 2 of 12 requests affected — 1 request failed, 1 request recovered after retry

Core review (2 requests):
- config.go — rejected by provider (HTTP 402) → failed
- payment.go — rate limited (HTTP 429) → provider overloaded (HTTP 529) → succeeded

Per-attempt detail: --format json (retry_report).
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

// Counted nouns are singular at one and plural above it.
func TestOutputRetryReportText_PluralAgreement(t *testing.T) {
	var buf bytes.Buffer
	outputRetryReportText(&buf, retryReportFixture())
	got := buf.String()
	for _, want := range []string{"2 of 12 requests affected", "(1 request)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	rep := retryReportFixture()
	rep.Requests = append(rep.Requests, llm.RequestReport{
		LogicalRequestID: "ccc",
		Model:            "claude-test",
		FilePath:         "billing.go",
		TaskType:         "main_task",
		RequestNo:        1,
		Outcome:          llm.OutcomeFailed,
		Attempts: []llm.AttemptRecord{
			{Number: 1, Outcome: llm.AttemptError, ErrorClass: llm.ErrorClassProvider, FailurePhase: llm.FailurePhaseHTTP, StatusCode: 402},
		},
	})
	buf.Reset()
	outputRetryReportText(&buf, rep)
	if got := buf.String(); !strings.Contains(got, "(2 requests)") {
		t.Errorf("two failures must read %q, got:\n%s", "(2 requests)", got)
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
	// "retried", not "recovered": no attempt failed, so nothing was recovered
	// from — the same distinction the JSON report's recovered_requests keeps.
	want := "\nLLM retry report summary: 1 of 1 request affected — 1 request retried at provider request\n" +
		"\nCore review (1 request):\n" +
		"- payment.go — succeeded (provider asked to retry) → succeeded\n" +
		"\nPer-attempt detail: --format json (retry_report).\n"
	var buf bytes.Buffer
	outputRetryReportText(&buf, rep)
	if got := buf.String(); got != want {
		t.Errorf("text report mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestOutputRetryReportText_CancelledIsShown(t *testing.T) {
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
	want := "\nLLM retry report summary: 1 of 1 request affected — 1 request cancelled\n" +
		"\nContext compaction (1 request):\n" +
		"- payment.go — succeeded → cancelled\n" +
		"\nPer-attempt detail: --format json (retry_report).\n"
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
	if got, want := retryAttemptChain(r), "network error → failed"; got != want {
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

	wantHeader := "LLM retry report summary: 2 of 2 requests affected — 1 request failed, 1 request recovered after retry"
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

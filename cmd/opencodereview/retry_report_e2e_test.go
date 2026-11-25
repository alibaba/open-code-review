// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

// End-to-end coverage of the #368 P5 wiring: a real review run against the fake
// Anthropic server, driven through runReview so review_cmd.go's own Freeze /
// publish decisions are the thing under test rather than a re-implementation of
// them.

// runReviewCapturingBoth runs a review and returns (stdout, stderr, error).
// Both streams are needed because the report has two possible exits and the
// contract is that exactly one of them carries it.
func runReviewCapturingBoth(t *testing.T, repoDir, format string, extraArgs ...string) (string, string, error) {
	t.Helper()
	var err error
	var out string
	errOut := captureStderr(t, func() {
		out = captureStdout(t, func() {
			args := []string{"--repo", repoDir, "--from", "HEAD~1", "--to", "HEAD", "--format", format}
			err = runReview(append(args, extraArgs...))
		})
	})
	return out, errOut, err
}

func TestReviewE2E_LocalBudgetGraceFailureKeepsSuccessfulExit(t *testing.T) {
	repo := retryTestRepo(t)
	server := newFakeLLM()
	var mainCalls atomic.Int32
	startFakeLLM(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if bytes.Contains(body, []byte(`"tools"`)) {
			switch mainCalls.Add(1) {
			case 2:
				writer.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(writer, `{"id":"over_budget","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"Not finished"}],"stop_reason":"end_turn","usage":{"input_tokens":100001,"output_tokens":5}}`)
				return
			case 3:
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusPaymentRequired)
				_, _ = io.WriteString(writer, `{"type":"error","error":{"type":"invalid_request_error","message":"payment required"}}`)
				return
			}
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		server.ServeHTTP(writer, request)
	}))
	out, stderr, err := runReviewCapturingBoth(t, repo, "json", "--effort", "low", "--no-filter", "--concurrency", "1", "--max-tokens-budget", "100000")
	if err != nil {
		t.Fatalf("budget-only partial review must exit successfully: %v\n%s", err, stderr)
	}
	var got jsonOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if mainCalls.Load() < 3 || !got.Summary.BudgetExceeded || got.Manifest == nil || got.Manifest.TerminalState != session.StatePartial || len(got.Manifest.Coverage.Completed) == 0 || len(got.Manifest.Coverage.Failed) == 0 || len(got.Manifest.Coverage.Completed)+len(got.Manifest.Coverage.Failed) != len(markers) {
		t.Fatalf("unexpected review result after %d requests: %s", mainCalls.Load(), out)
	}
	for _, item := range got.Manifest.Coverage.Failed {
		if item.Classification != session.FailureBudget {
			t.Errorf("%s: got %s, want budget", item.Path, item.Classification)
		}
	}
	if got.RetryReport == nil || got.RetryReport.FailedRequests != 1 {
		t.Errorf("final request failure missing from retry report: %+v", got.RetryReport)
	}
}

func TestReviewE2E_BudgetGraceCompletionControlsResume(t *testing.T) {
	const parentFinding = "parent finding"
	const commentCall = `{"type":"tool_use","id":"comment","name":"code_comment","input":{"comments":[{"path":"a.go","content":"parent finding","severity":"medium","category":"bug"}]}}`
	const doneCall = `{"type":"tool_use","id":"done","name":"task_done","input":{"state":"DONE"}}`
	const failedCall = `{"type":"tool_use","id":"failed","name":"task_done","input":{"state":"FAILED"}}`
	for _, tc := range []struct {
		name           string
		commentInGrace bool
		graceCalls     string
		complete       bool
	}{
		{name: "comments_only", commentInGrace: true, graceCalls: commentCall},
		{name: "empty"},
		{name: "explicit_failure", graceCalls: failedCall},
		{name: "done", graceCalls: doneCall, complete: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, err := resolveRepoDir(retryTestRepoWithFiles(t, map[string]string{"MARKER_ALPHA": "a.go"}))
			if err != nil {
				t.Fatal(err)
			}
			server := newFakeLLM()
			var mainCalls, requests atomic.Int32
			startFakeLLM(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				if bytes.Contains(body, []byte(`"tools"`)) {
					round := mainCalls.Add(1)
					calls, stopReason, tokens := "", "tool_use", 10
					switch round {
					case 1:
						calls, tokens = commentCall, 100001
						if tc.commentInGrace {
							calls, stopReason = `{"type":"text","text":"Still reviewing"}`, "end_turn"
						}
					case 2:
						calls = tc.graceCalls
						if calls == "" {
							stopReason = "end_turn"
						}
					}
					if round <= 2 {
						w.Header().Set("Content-Type", "application/json")
						_, _ = fmt.Fprintf(w, `{"id":"msg_budget","type":"message","role":"assistant","model":"claude-test",
							"content":[%s],"stop_reason":%q,"usage":{"input_tokens":%d,"output_tokens":5}}`, calls, stopReason, tokens)
						return
					}
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
				server.ServeHTTP(w, r)
			}))

			args := []string{"--effort", "low", "--no-filter", "--max-tokens-budget", "100000"}
			out, stderr, err := runReviewCapturingBoth(t, repo, "json", args...)
			if err != nil {
				t.Fatalf("parent review: %v\n%s\n%s", err, stderr, out)
			}
			var parent jsonOutput
			if err := json.Unmarshal([]byte(out), &parent); err != nil {
				t.Fatal(err)
			}
			wantState, wantReused, wantRerun := session.StatePartial, int64(0), int64(1)
			if tc.complete {
				wantState, wantReused, wantRerun = session.StateComplete, 1, 0
			}
			if mainCalls.Load() != 2 || parent.Status != string(wantState) || parent.Manifest == nil ||
				parent.Manifest.TerminalState != wantState || len(parent.Comments) != 1 || parent.Comments[0].Content != parentFinding {
				t.Fatalf("parent after %d main requests, want %s and one finding: %s", mainCalls.Load(), wantState, out)
			}
			saved, err := session.LoadSummary(repo, parent.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(saved.RunManifest, parent.Manifest) {
				t.Fatalf("persisted manifest = %+v, want %+v", saved.RunManifest, parent.Manifest)
			}
			if !tc.complete {
				coverage := saved.RunManifest.Coverage
				if len(coverage.Completed) != 0 || len(coverage.Failed) != 1 ||
					coverage.Failed[0].Path != "a.go" || coverage.Failed[0].Classification != session.FailureBudget {
					t.Fatalf("parent coverage = %+v, want a.go failed by budget", coverage)
				}
			}
			stored := captureStdout(t, func() {
				if err := runSessionCommentsCompat([]string{"--repo", repo, "--json", parent.SessionID}); err != nil {
					t.Fatal(err)
				}
			})
			var comments []model.LlmComment
			if err := json.Unmarshal([]byte(stored), &comments); err != nil {
				t.Fatal(err)
			}
			if len(comments) != 1 || comments[0].Path != "a.go" || comments[0].Content != parentFinding {
				t.Fatalf("session comments = %s, want parent finding on a.go", stored)
			}

			beforeRequests, beforeFile := requests.Load(), server.attemptCounts()["a.go"]
			out, stderr, err = runReviewCapturingBoth(t, repo, "json", append(args, "--resume", parent.SessionID)...)
			if err != nil {
				t.Fatalf("resumed review: %v\n%s\n%s", err, stderr, out)
			}
			var child jsonOutput
			if err := json.Unmarshal([]byte(out), &child); err != nil {
				t.Fatal(err)
			}
			if child.Resume == nil || child.Resume.ReusedFiles != wantReused || child.Resume.RerunFiles != wantRerun {
				t.Fatalf("resume = %+v, want reused=%d rerun=%d", child.Resume, wantReused, wantRerun)
			}
			if int64(mainCalls.Load()-2) != wantRerun {
				t.Fatalf("resumed main requests = %d, want %d", mainCalls.Load()-2, wantRerun)
			}
			if tc.complete && requests.Load() != beforeRequests {
				t.Fatalf("full reuse made %d model requests", requests.Load()-beforeRequests)
			}
			if !tc.complete && server.attemptCounts()["a.go"] <= beforeFile {
				t.Fatal("resumed review did not send a.go to the model")
			}
			if len(child.Comments) != int(wantReused) || (tc.complete && child.Comments[0].Content != parentFinding) {
				t.Fatalf("child comments = %+v, want %d parent findings", child.Comments, wantReused)
			}
		})
	}
}

func TestReviewE2E_CleanRunEmitsNoRetryReport(t *testing.T) {
	repoDir := retryTestRepo(t)
	startFakeLLM(t, newFakeLLM())

	out, errOut, err := runReviewCapturingBoth(t, repoDir, "json")
	if err != nil {
		t.Fatalf("review must succeed: %v\nstderr: %s", err, errOut)
	}
	if strings.Contains(out, "retry_report") {
		t.Errorf("a first-try-success run must not emit retry_report:\n%s", out)
	}
	if strings.Contains(errOut, "LLM retry report summary") {
		t.Errorf("nothing should reach the failure exit either:\n%s", errOut)
	}
}

func TestReviewE2E_RecoveredAndFailedReachesJSONExit(t *testing.T) {
	repoDir, err := resolveRepoDir(retryTestRepo(t))
	if err != nil {
		t.Fatal(err)
	}
	srv := newFakeLLM()
	srv.rateLimitOnce["a.go"] = true
	srv.hardFail["b.go"] = true
	startFakeLLM(t, srv)

	out, errOut, err := runReviewCapturingBoth(t, repoDir, "json")
	// One provider failure makes the review operationally incomplete even though
	// the sibling succeeded. The partial JSON result must still be published
	// before the command returns its non-zero status.
	if err == nil || !strings.Contains(err.Error(), "1 of 4 selected item(s) failed") {
		t.Fatalf("partial provider failure must exit non-zero with item counts: %v\nstderr: %s", err, errOut)
	}

	var got jsonOutput
	if e := json.Unmarshal([]byte(out), &got); e != nil {
		t.Fatalf("unmarshal stdout: %v\n%s", e, out)
	}
	rep := got.RetryReport
	if rep == nil {
		t.Fatalf("retry_report missing from the normal JSON exit:\n%s", out)
	}
	if rep.SchemaVersion != llm.RetryReportSchemaVersion {
		t.Errorf("schema_version = %q", rep.SchemaVersion)
	}
	if rep.TotalRequests < 2 {
		t.Errorf("total_requests = %d, want at least the two reviewed files", rep.TotalRequests)
	}
	if rep.RetriedRequests != 1 || rep.TotalRetries != 1 {
		t.Errorf("expected exactly one retried request with one retry, got %+v", rep)
	}
	if rep.RecoveredRequests != 1 {
		t.Errorf("recovered_requests = %d, want 1", rep.RecoveredRequests)
	}
	if rep.FailedRequests == 0 {
		t.Errorf("failed_requests = 0, want the hard-failing file counted: %+v", rep)
	}
	if n := srv.attemptCounts()["a.go"]; n < 2 {
		t.Errorf("the SDK, not OCR, must have retried a.go; saw %d HTTP attempts", n)
	}

	// The retried request must show the 429 and the recovery, and must carry the
	// observed backoff the Retry-After header asked for.
	var recovered *llm.RequestReport
	for i := range rep.Requests {
		if rep.Requests[i].Outcome == llm.OutcomeRecovered {
			recovered = &rep.Requests[i]
		}
	}
	if recovered == nil {
		t.Fatalf("no recovered request listed: %+v", rep.Requests)
	}
	if len(recovered.Attempts) != 2 {
		t.Fatalf("recovered request must list 2 attempts, got %+v", recovered.Attempts)
	}
	first, second := recovered.Attempts[0], recovered.Attempts[1]
	if first.ErrorClass != llm.ErrorClassRateLimited || first.StatusCode != 429 {
		t.Errorf("first attempt = %+v, want rate_limited/429", first)
	}
	if first.FailurePhase != llm.FailurePhaseHTTP {
		t.Errorf("failure_phase = %q, want http", first.FailurePhase)
	}
	if second.Outcome != llm.AttemptSuccess {
		t.Errorf("second attempt = %+v, want success", second)
	}
	if second.ObservedBackoffMS < 1000 {
		t.Errorf("observed_backoff_ms = %d, want at least the 1s Retry-After", second.ObservedBackoffMS)
	}

	// Whatever the JSON says must also be readable in the run's stderr-free
	// stdout stream as a single document.
	var stdout struct {
		RetryReport map[string]any `json:"retry_report"`
	}
	dec := json.NewDecoder(strings.NewReader(out))
	dec.UseNumber()
	if e := dec.Decode(&stdout); e != nil {
		t.Fatalf("decode: %v", e)
	}
	if dec.More() {
		t.Error("stdout must carry exactly one JSON document")
	}

	sessionPath, err := session.SessionFilePath(repoDir, got.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.TrimSuffix(data, []byte("\n"))
	lastLine := data[bytes.LastIndexByte(data, '\n')+1:]
	var lastRecord struct {
		Type        string         `json:"type"`
		RetryReport map[string]any `json:"llm_retry_report"`
	}
	recordDecoder := json.NewDecoder(bytes.NewReader(lastLine))
	recordDecoder.UseNumber()
	if err := recordDecoder.Decode(&lastRecord); err != nil {
		t.Fatalf("decode final session record: %v", err)
	}
	if lastRecord.Type != "session_end" {
		t.Fatalf("final session record type = %q, want session_end", lastRecord.Type)
	}
	if !reflect.DeepEqual(lastRecord.RetryReport, stdout.RetryReport) {
		t.Errorf("session_end.llm_retry_report = %v, want stdout.retry_report = %v", lastRecord.RetryReport, stdout.RetryReport)
	}
}

func TestReviewE2E_RetryReportReachesTextExit(t *testing.T) {
	repoDir := retryTestRepo(t)
	srv := newFakeLLM()
	srv.rateLimitOnce["a.go"] = true
	srv.hardFail["b.go"] = true
	startFakeLLM(t, srv)

	out, errOut, err := runReviewCapturingBoth(t, repoDir, "text")
	if err == nil || !strings.Contains(err.Error(), "1 of 4 selected item(s) failed") {
		t.Fatalf("partial provider failure must exit non-zero with item counts: %v\nstderr: %s", err, errOut)
	}
	// This test attributes a 429 and a 402 to named files, which only holds while
	// each file is its own request — i.e. while the fake still answers the
	// grouping call with one group per file.
	srv.assertGroupingRecognized(t)
	if !strings.Contains(out, "LLM retry report summary:") {
		t.Fatalf("terminal summary missing from stdout:\n%s", out)
	}
	if !strings.Contains(out, "rate limited (HTTP 429) -> succeeded") {
		t.Errorf("attempt chain missing from the terminal summary:\n%s", out)
	}
	if !strings.Contains(out, "rejected by provider (HTTP 402) -> failed") {
		t.Errorf("failed request missing from the terminal summary:\n%s", out)
	}
	// The summary is a run result, so it must not be duplicated onto stderr.
	if strings.Contains(errOut, "LLM retry report summary") {
		t.Errorf("report must not also appear on stderr:\n%s", errOut)
	}
	// It must carry no secret material: the token, the endpoint URL and raw
	// provider error text all stay out.
	for _, forbidden := range []string{"test-token", "x-api-key", "127.0.0.1", "payment required", "slow down"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("terminal output leaked %q:\n%s", forbidden, out)
		}
	}
}

// Every file failing still produces a manifest (terminal_state=failed), so the
// normal exit runs and the failure-usage record must not repeat the report.
// This is the dedup rule, and the only case where both exits execute.
func TestReviewE2E_AllFilesFailPublishesReportOnce(t *testing.T) {
	repoDir := retryTestRepo(t)
	srv := newFakeLLM()
	srv.failAll()
	startFakeLLM(t, srv)

	out, errOut, err := runReviewCapturingBoth(t, repoDir, "json")
	if err == nil {
		t.Fatalf("a fully failed review must exit non-zero\nstdout: %s", out)
	}

	var got jsonOutput
	if e := json.Unmarshal([]byte(out), &got); e != nil {
		t.Fatalf("unmarshal stdout: %v\n%s", e, out)
	}
	if got.RetryReport == nil {
		t.Fatalf("retry_report missing from the normal exit:\n%s", out)
	}
	if got.RetryReport.FailedRequests == 0 {
		t.Errorf("failed_requests = 0 on an all-failed run: %+v", got.RetryReport)
	}
	// The failure-usage record on stderr is emitted for the same run; it must not
	// carry a second copy.
	if strings.Contains(errOut, "retry_report") {
		t.Errorf("report duplicated onto the failure exit:\n%s", errOut)
	}
	if !strings.Contains(errOut, `"status": "failed"`) {
		t.Errorf("failure usage record missing from stderr:\n%s", errOut)
	}
}

func TestReviewE2E_AllFilesFailTextPublishesReportOnce(t *testing.T) {
	repoDir := retryTestRepo(t)
	srv := newFakeLLM()
	srv.failAll()
	startFakeLLM(t, srv)

	out, errOut, err := runReviewCapturingBoth(t, repoDir, "text")
	if err == nil {
		t.Fatal("a fully failed review must exit non-zero")
	}
	if !strings.Contains(out, "LLM retry report summary:") {
		t.Fatalf("report missing from stdout:\n%s", out)
	}
	if strings.Contains(errOut, "LLM retry report summary:") {
		t.Errorf("report duplicated onto stderr:\n%s", errOut)
	}
	if !strings.Contains(errOut, "[ocr] usage on failure:") {
		t.Errorf("failure usage line missing:\n%s", errOut)
	}
}

// A report-construction error is an observability warning. It must not
// change a successful review's exit status, print a --resume hint, emit a
// failure-usage record, or publish a partial report — while the review's own
// result is still published.
func TestReviewE2E_FreezeErrorIsAWarning(t *testing.T) {
	repoDir := retryTestRepo(t)
	startFakeLLM(t, newFakeLLM())

	orig := newRetryCollector
	newRetryCollector = poisonedRetryCollector
	t.Cleanup(func() { newRetryCollector = orig })

	out, errOut, err := runReviewCapturingBoth(t, repoDir, "json")
	if err != nil {
		t.Fatalf("retry-report observability must not fail a successful review: %v\nstderr: %s", err, errOut)
	}
	if !strings.Contains(errOut, "[ocr] warning: freeze retry report:") ||
		!strings.Contains(errOut, "(retry report suppressed)") {
		t.Errorf("freeze warning missing or incomplete:\n%s", errOut)
	}
	if strings.Contains(errOut, "--resume") {
		t.Errorf("no resume hint belongs on a successful review:\n%s", errOut)
	}
	if strings.Contains(errOut, `"status": "failed"`) {
		t.Errorf("no failure usage record belongs on a successful review:\n%s", errOut)
	}

	var got jsonOutput
	if e := json.Unmarshal([]byte(out), &got); e != nil {
		t.Fatalf("the review result must still be published: %v\n%s", e, out)
	}
	if got.RetryReport != nil {
		t.Errorf("a self-contradictory report must not be published: %+v", got.RetryReport)
	}
	if got.Manifest == nil {
		t.Errorf("the manifest must still be published:\n%s", out)
	}
}

// The run_id is the session's in-memory UUID rather than the
// persistence-gated ag.SessionID(), so a session whose JSONL file could not be
// created still yields a report with usable logical_request_ids — while
// session_id stays empty and no --resume hint is printed, because there is
// nothing to resume from.
func TestReviewE2E_ReportSurvivesSessionPersistenceFailure(t *testing.T) {
	repoDir := retryTestRepo(t)
	srv := newFakeLLM()
	srv.rateLimitOnce["a.go"] = true
	srv.hardFail["b.go"] = true
	startFakeLLM(t, srv)

	// Block session file creation by occupying the session subdirectory with a
	// regular file, so the writer's MkdirAll fails. The path is derived from
	// session.SessionsDir rather than hardcoded so it follows the subdirectory
	// name, and only the *session* level is blocked — $HOME/.opencodereview must
	// stay a real directory or global rule loading fails first and the run never
	// reaches an LLM request.
	sessionsDir, err := session.SessionsDir(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Dir(sessionsDir) // $HOME/.opencodereview/<subdir>
	if err := os.MkdirAll(filepath.Dir(blocked), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runReviewCapturingBoth(t, repoDir, "json")
	// The session-delivery failure is itself reported, but a manifest was still
	// constructed, so the normal exit publishes the complete result first.
	if err == nil {
		t.Fatal("a session delivery failure must surface")
	}
	var got jsonOutput
	if e := json.Unmarshal([]byte(out), &got); e != nil {
		t.Fatalf("the result must still be published: %v\n%s", e, out)
	}
	if got.RetryReport == nil {
		t.Fatalf("report must still be produced without persistence:\n%s", out)
	}
	if got.SessionID != "" {
		t.Errorf("session_id = %q, want empty when the session was not persisted", got.SessionID)
	}
	if strings.Contains(errOut, "--resume") {
		t.Errorf("no resume target exists, so no hint may be printed:\n%s", errOut)
	}
	if strings.Contains(errOut, "retry_report") {
		t.Errorf("report duplicated onto the failure exit:\n%s", errOut)
	}
	for _, r := range got.RetryReport.Requests {
		if r.LogicalRequestID == "" {
			t.Errorf("logical_request_id must still be derivable: %+v", r)
		}
	}
}

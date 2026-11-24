// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

// retryMeta is a minimal valid RequestMeta; RecordAttempt drops anything whose
// Model, FilePath or TaskType is empty, or whose RequestNo is not positive.
func retryMeta(requestNo int) llm.RequestMeta {
	return llm.RequestMeta{
		Provider:  "test-provider",
		Model:     "fake",
		FilePath:  "main.go",
		TaskType:  string(session.MainTask),
		RequestNo: requestNo,
	}
}

// runInputFailureAgent drives the load-diffs exit of Run, the cheapest terminal
// path that still goes through freezeRetryReport and session_end.
func runInputFailureAgent(t *testing.T, collector *llm.RetryCollector) (*Agent, string, error) {
	t.Helper()
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "missing-base",
		DiffTo:     "missing-head",
		Operation:  session.OperationReview,
	})
	a := New(Args{
		RepoDir:        repoDir,
		From:           "missing-base",
		To:             "missing-head",
		ReviewMode:     session.ReviewModeRange,
		GitRunner:      gitcmd.New(1),
		LLMClient:      manifestFlowClient{},
		RetryCollector: collector,
		Model:          "fake",
		Session:        sh,
	})
	_, err := a.Run(context.Background())
	if err == nil {
		t.Fatal("invalid review input must fail")
	}
	return a, repoDir, err
}

// TestRunPersistsRetryReport pins that the report is frozen inside Run and
// embedded into session_end. The command takes its own deterministic snapshot
// after Run, so the two serialized values must be equal.
func TestRunPersistsRetryReport(t *testing.T) {
	collector := llm.NewRetryCollector()
	meta := retryMeta(1)
	now := time.Now()
	collector.RecordAttempt(meta, llm.AttemptRecord{
		Number:       1,
		ErrorClass:   llm.ErrorClassOverloaded,
		FailurePhase: llm.FailurePhaseHTTP,
		StatusCode:   529,
	}, now, now.Add(time.Second))
	collector.RecordAttempt(meta, llm.AttemptRecord{Number: 2, StatusCode: 200}, now.Add(2*time.Second), now.Add(3*time.Second))
	collector.Finalize(meta, nil, false)

	a, repoDir, _ := runInputFailureAgent(t, collector)

	rep, err := collector.Freeze(a.Session().SessionID)
	if err != nil {
		t.Fatalf("Freeze() after Run = %v", err)
	}
	if rep == nil {
		t.Fatal("Freeze() after Run = nil, want the output snapshot")
	}
	if rep.RetriedRequests != 1 || rep.TotalRetries != 1 {
		t.Errorf("report = %+v, want RetriedRequests=1 TotalRetries=1", rep)
	}

	persisted := persistedRetryReport(t, repoDir, a.Session().SessionID)
	if persisted == nil {
		t.Fatal("session_end carries no llm_retry_report")
	}
	wantJSON, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("marshal output snapshot: %v", err)
	}
	var want map[string]any
	if err := json.Unmarshal(wantJSON, &want); err != nil {
		t.Fatalf("unmarshal output snapshot: %v", err)
	}
	if !reflect.DeepEqual(persisted, want) {
		t.Errorf("persisted report differs from output snapshot:\n persisted: %v\n output:    %v", persisted, want)
	}
}

// TestRunSuppressesInvalidRetryReport pins that a collector invariant failure
// does not enter Run's error or agent warnings and is persisted nowhere. The
// command's own Freeze call remains the single diagnostic outlet.
func TestRunSuppressesInvalidRetryReport(t *testing.T) {
	// An attempt that is never finalized is exactly what Freeze rejects — it means
	// the client boundary let a request escape without recording its outcome.
	collector := llm.NewRetryCollector()
	now := time.Now()
	collector.RecordAttempt(retryMeta(1), llm.AttemptRecord{Number: 1, StatusCode: 200}, now, now.Add(time.Second))

	a, repoDir, runErr := runInputFailureAgent(t, collector)

	_, freezeErr := collector.Freeze(a.Session().SessionID)
	if freezeErr == nil {
		t.Fatal("Freeze() after Run = nil, want the un-finalized-request error")
	}
	if !strings.Contains(runErr.Error(), "load diffs") {
		t.Errorf("Run error = %v, want the input failure", runErr)
	}
	if strings.Contains(runErr.Error(), freezeErr.Error()) {
		t.Errorf("Run error absorbed the freeze error: %v", runErr)
	}

	// A report that contradicts itself is published nowhere, not even partially.
	if persisted := persistedRetryReport(t, repoDir, a.Session().SessionID); persisted != nil {
		t.Errorf("session_end carries a report after a failed freeze: %v", persisted)
	}

	// Agent warnings feed text and JSON result output. The freeze diagnostic stays
	// solely in the command layer, so Run must not add a second output path.
	for _, w := range a.Warnings() {
		if w.Type == "retry_report_error" {
			t.Errorf("agent warnings unexpectedly include retry_report_error: %+v", a.Warnings())
		}
	}

	// The manifest terminal state stays the one the input failure produced.
	manifest := a.RunManifest()
	if manifest == nil || manifest.RunFailure == nil || manifest.RunFailure.Classification != session.RunFailureInput {
		t.Errorf("manifest = %+v, want an input run failure", manifest)
	}
}

// TestRunWithoutCollectorPersistsNoReport covers the nil-collector wiring, which
// is what scan and every test that does not opt in look like.
func TestRunWithoutCollectorPersistsNoReport(t *testing.T) {
	a, repoDir, _ := runInputFailureAgent(t, nil)

	if persisted := persistedRetryReport(t, repoDir, a.Session().SessionID); persisted != nil {
		t.Errorf("session_end carries a report without a collector: %v", persisted)
	}
}

func persistedRetryReport(t *testing.T, repoDir, sessionID string) map[string]any {
	t.Helper()
	dir, err := session.SessionsDir(repoDir)
	if err != nil {
		t.Fatalf("sessions dir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, sessionID+".jsonl"))
	if err != nil {
		t.Fatalf("read session jsonl: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unmarshal jsonl line: %v", err)
		}
		if rec["type"] != "session_end" {
			continue
		}
		rep, _ := rec["llm_retry_report"].(map[string]any)
		return rep
	}
	t.Fatal("no session_end record found")
	return nil
}

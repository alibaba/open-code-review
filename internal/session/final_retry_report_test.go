// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

// TestSessionEndEmbedsRetryReport pins that a frozen report reaches session_end
// under llm_retry_report, so a session can be diagnosed on its own without the
// command output that was printed at the time.
func TestSessionEndEmbedsRetryReport(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})

	sh.SetFinalRetryReport(&llm.RetryReport{
		SchemaVersion:   llm.RetryReportSchemaVersion,
		TotalRequests:   3,
		RetriedRequests: 1,
		TotalRetries:    2,
	})
	if err := sh.Finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	endRec := sessionEndRecord(t, repoDir, sh.SessionID)
	rep, ok := endRec["llm_retry_report"].(map[string]any)
	if !ok {
		t.Fatalf("llm_retry_report missing or wrong type: %v", endRec["llm_retry_report"])
	}
	if rep["schema_version"] != llm.RetryReportSchemaVersion {
		t.Errorf("schema_version = %v, want %v", rep["schema_version"], llm.RetryReportSchemaVersion)
	}
	if got, _ := rep["total_retries"].(float64); int(got) != 2 {
		t.Errorf("total_retries = %v, want 2", rep["total_retries"])
	}
}

// TestSessionEndOmitsAbsentRetryReport pins the other half: a run with nothing to
// report must leave the field out entirely rather than write an empty object,
// which a reader would otherwise have to distinguish from a real report.
func TestSessionEndOmitsAbsentRetryReport(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})

	if err := sh.Finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	endRec := sessionEndRecord(t, repoDir, sh.SessionID)
	if _, present := endRec["llm_retry_report"]; present {
		t.Errorf("llm_retry_report present without a frozen report: %v", endRec["llm_retry_report"])
	}
}

func sessionEndRecord(t *testing.T, repoDir, sessionID string) map[string]any {
	t.Helper()
	for _, r := range readJSONLRecords(t, sessionJSONLPath(t, repoDir, sessionID)) {
		if r["type"] == "session_end" {
			return r
		}
	}
	t.Fatal("no session_end record found")
	return nil
}

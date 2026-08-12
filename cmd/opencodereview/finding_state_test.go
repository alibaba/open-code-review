// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func completeResultJSON(sessionID, content string) []byte {
	return []byte(`{"status":"complete","session_id":"` + sessionID + `","comments":[{"path":"./pkg\\file.go","start_line":4,"end_line":4,"category":" BUG ","severity":" HIGH ","content":"` + content + `"}]}`)
}

func decodeFindingResult(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return result
}

func TestAnnotateAndRecordFindingsCountsConsecutiveReviews(t *testing.T) {
	dir := t.TempDir()
	first, err := annotateAndRecordFindings(dir, testBaseSHA, completeResultJSON("review-1", "same issue"))
	if err != nil {
		t.Fatalf("first review: %v", err)
	}
	second, err := annotateAndRecordFindings(dir, testBaseSHA, completeResultJSON("review-2", "same issue"))
	if err != nil {
		t.Fatalf("second review: %v", err)
	}
	third, err := annotateAndRecordFindings(dir, testBaseSHA, completeResultJSON("review-3", "same issue"))
	if err != nil {
		t.Fatalf("third review: %v", err)
	}

	for i, raw := range [][]byte{first, second, third} {
		result := decodeFindingResult(t, raw)
		comments := result["comments"].([]any)
		comment := comments[0].(map[string]any)
		wantCount := float64(i + 1)
		if comment["consecutive_review_count"] != wantCount {
			t.Fatalf("review %d count = %v, want %v", i+1, comment["consecutive_review_count"], wantCount)
		}
		if i < 2 && comment["automation_status"] != findingStatusActive {
			t.Fatalf("review %d status = %v, want active", i+1, comment["automation_status"])
		}
		if i == 2 && comment["automation_status"] != findingStatusDeferred {
			t.Fatalf("review %d status = %v, want deferred", i+1, comment["automation_status"])
		}
		if !strings.HasPrefix(comment["finding_id"].(string), findingIDVersion+":") {
			t.Fatalf("review %d finding_id = %v", i+1, comment["finding_id"])
		}
	}

	stateRaw, err := os.ReadFile(filepath.Join(dir, ".scratch", "finding-counts.json"))
	if err != nil {
		t.Fatalf("read counter: %v", err)
	}
	var state findingCounterFile
	if err := json.Unmarshal(stateRaw, &state); err != nil {
		t.Fatalf("decode counter: %v", err)
	}
	if len(state.ProcessedReviewIDs) != 3 || len(state.Findings) != 1 {
		t.Fatalf("state = %+v", state)
	}
}

func TestAnnotateAndRecordFindingsDuplicateSessionDoesNotIncrement(t *testing.T) {
	dir := t.TempDir()
	first, err := annotateAndRecordFindings(dir, testBaseSHA, completeResultJSON("review-1", "same issue"))
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := annotateAndRecordFindings(dir, testBaseSHA, completeResultJSON("review-1", "same issue"))
	if err != nil {
		t.Fatal(err)
	}
	for label, raw := range map[string][]byte{"first": first, "duplicate": duplicate} {
		result := decodeFindingResult(t, raw)
		comment := result["comments"].([]any)[0].(map[string]any)
		if comment["consecutive_review_count"] != float64(1) {
			t.Fatalf("%s count = %v, want 1", label, comment["consecutive_review_count"])
		}
	}
}

func TestAnnotateAndRecordFindingsDisappearedFindingIsRemoved(t *testing.T) {
	dir := t.TempDir()
	if _, err := annotateAndRecordFindings(dir, testBaseSHA, completeResultJSON("review-1", "same issue")); err != nil {
		t.Fatal(err)
	}
	withoutComments := []byte(`{"status":"complete","session_id":"review-2","comments":[]}`)
	if _, err := annotateAndRecordFindings(dir, testBaseSHA, withoutComments); err != nil {
		t.Fatal(err)
	}
	state, exists, err := loadFindingCounter(dir)
	if err != nil || !exists {
		t.Fatalf("load state: exists=%v err=%v", exists, err)
	}
	if len(state.Findings) != 0 {
		t.Fatalf("findings = %+v, want empty", state.Findings)
	}
}

func TestAnnotateAndRecordFindingsRequiresSessionID(t *testing.T) {
	_, err := annotateAndRecordFindings(t.TempDir(), testBaseSHA, []byte(`{"status":"complete","comments":[]}`))
	if err == nil || !strings.Contains(err.Error(), "session_id") {
		t.Fatalf("error = %v, want missing session_id", err)
	}
}

func TestFindingIDNormalizesPathAndContent(t *testing.T) {
	a := map[string]json.RawMessage{}
	a["path"] = json.RawMessage(`"./pkg\\file.go"`)
	a["start_line"] = json.RawMessage(`4`)
	a["end_line"] = json.RawMessage(`4`)
	a["category"] = json.RawMessage(`" BUG "`)
	a["severity"] = json.RawMessage(`" HIGH "`)
	a["content"] = json.RawMessage(`"same\r\n  issue"`)
	b := map[string]json.RawMessage{}
	b["path"] = json.RawMessage(`"pkg/file.go"`)
	b["start_line"] = json.RawMessage(`4`)
	b["end_line"] = json.RawMessage(`4`)
	b["category"] = json.RawMessage(`"bug"`)
	b["severity"] = json.RawMessage(`"high"`)
	b["content"] = json.RawMessage(`"same issue"`)
	if got, want := findingIDForRawComment(testBaseSHA, a), findingIDForRawComment(testBaseSHA, b); got != want {
		t.Fatalf("normalized IDs differ: %q != %q", got, want)
	}
}

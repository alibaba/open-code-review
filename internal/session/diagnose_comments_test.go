// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"strings"
	"testing"
)

const diagnosticCommentRecords = `{"type":"llm_request","uuid":"request-a","request_id":"request-a","filePath":"group-a","taskType":"main_task","request_no":1}
{"type":"llm_response","uuid":"response-a","request_id":"request-a","filePath":"group-a","taskType":"main_task","request_no":1,"tool_calls":[{"id":"call-a","name":"code_comment"}]}
{"type":"comment_evidence","uuid":"comment-a","request_id":"request-a","response_id":"response-a","tool_call_id":"call-a","tool_name":"code_comment","filePath":"group-a","taskType":"main_task","request_no":1,"comment_index":0,"original_comment":{"path":"a.go","content":"private finding","existing_code":"originalCall()"},"comment":{"evidence_id":"comment-a","path":"b.go","content":"private finding","existing_code":"actualCall()","start_line":3,"end_line":3},"code_evidence":{"status":"matched","path":"b.go","snapshot_sha256":"snapshot-id","spans":[{"side":"new","source":"new_file_content","start_line":3,"end_line":3,"excerpt":"actualCall()","match":"exact"}]}}
{"type":"review_item_done","filePath":"b.go","fingerprint":"fp","comments":[{"evidence_id":"comment-a","path":"b.go","content":"private finding","existing_code":"actualCall()","start_line":3,"end_line":3}]}
{"type":"session_end"}`

func TestDiagnoseCommentEvidenceDrillDown(t *testing.T) {
	repo := writeDiagnosticSession(t, diagnosticCommentRecords)
	report, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Comments) != 1 || len(report.Observations) != 0 || report.Comments[0].TaskKey != "group-a" || report.Comments[0].Path != "b.go" || len(report.Comments[0].Checkpoints) != 1 {
		t.Fatalf("group task and final comment path were conflated: %+v", report)
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), "private finding") || strings.Contains(string(raw), "actualCall()") {
		t.Fatal("overview copied private comment content or code evidence")
	}
	selected, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{CommentID: "comment-a", TaskKey: "group-a", RequestID: "request-a"})
	if err != nil {
		t.Fatal(err)
	}
	cm := selected.Comment
	if cm.Line != 3 || cm.ResponseID != "response-a" || cm.ToolCallID != "call-a" || cm.OriginalComment.ExistingCode != "originalCall()" || cm.Code.Spans[0].Excerpt != "actualCall()" || cm.Checkpoints[0].Line != 4 {
		t.Fatalf("comment provenance or original evidence lost: %+v", cm)
	}
	for _, opts := range []DiagnoseOptions{
		{CommentID: "missing"}, {CommentID: "comment-a", TaskKey: "b.go"},
		{CommentID: "comment-a", RequestID: "other"}, {CommentID: "comment-a", EventLine: 3},
	} {
		if _, err := Diagnose(repo, "diagnostic-run", opts); err == nil {
			t.Fatalf("invalid combined selection accepted: %+v", opts)
		}
	}
}

func TestDiagnoseCommentEvidenceCheckpointsAndReuse(t *testing.T) {
	repo := writeDiagnosticSession(t, diagnosticCommentRecords+"\n"+`{"type":"review_item_done","filePath":"b.go","comments":[{"evidence_id":"comment-a","path":"b.go","content":"merged finding","existing_code":"actualCall()","start_line":3,"end_line":3}]}
{"type":"review_item_reused","filePath":"cached.go","sourceSessionId":"parent-run","comments":[{"evidence_id":"external-comment","path":"cached.go","content":"reused"}]}
{"type":"review_item_failed","filePath":"b.go","fingerprint":"fp","error":"later checkpoint failed"}`)
	report, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{CommentID: "external-comment"})
	if err != nil || report.Comment.EvidenceStatus != "external" || report.Comment.Code != nil || report.Comment.Checkpoints[0].SourceSessionID != "parent-run" {
		t.Fatalf("reused comment origin was invented: %+v, %v", report, err)
	}
	changed, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{CommentID: "comment-a"})
	if err != nil || len(changed.Comment.Checkpoints) != 2 || changed.Comment.Comment.Content != "private finding" {
		t.Fatalf("historical evidence was overwritten by a later checkpoint: %+v, %v", changed, err)
	}
	found := false
	for _, obs := range changed.Observations {
		found = found || obs.Code == "comment_snapshot_changed"
	}
	if !found {
		t.Fatal("modified checkpoint did not mark the captured evidence as historical")
	}
}

func TestDiagnoseCommentEvidenceInconsistentRecords(t *testing.T) {
	for _, tc := range []struct {
		name, records, code, id string
		selectError             bool
	}{
		{"missing evidence", `{"type":"review_item_done","filePath":"a.go","comments":[{"evidence_id":"absent","content":"finding"}]}`, "comment_evidence_missing", "absent", false},
		{"duplicate evidence", diagnosticCommentRecords + "\n" + `{"type":"comment_evidence","uuid":"comment-a","comment":{"evidence_id":"comment-a"}}`, "comment_evidence_duplicate", "comment-a", true},
		{"wrong identity", `{"type":"comment_evidence","uuid":"event-a","comment":{"evidence_id":"other"}}`, "comment_identity_mismatch", "event-a", true},
		{"conflicting checkpoints", `{"type":"review_item_reused","filePath":"a.go","sourceSessionId":"parent","comments":[{"evidence_id":"external","content":"first"}]}` + "\n" + `{"type":"review_item_reused","filePath":"a.go","sourceSessionId":"parent","comments":[{"evidence_id":"external","content":"second"}]}`, "comment_checkpoint_ambiguous", "external", true},
		{"missing relocation request", strings.Replace(diagnosticCommentRecords, `"comment_index":0`, `"relocation_request_id":"absent","comment_index":0`, 1), "comment_relocation_link_mismatch", "comment-a", false},
		{"unknown evidence version", strings.Replace(diagnosticCommentRecords, `"comment_index":0`, `"evidence_version":"ocr.comment-evidence/v99","comment_index":0`, 1), "comment_evidence_version_unknown", "comment-a", true},
		{"unknown decision version", diagnosticCommentRecords + "\n" + `{"type":"comment_decision","decision_version":"ocr.comment-decision/v99","evidence_id":"comment-a","stage":"scan_dedup","outcome":"merged","output_comment":{"evidence_id":"other"}}`, "comment_decision_version_unknown", "comment-a", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := writeDiagnosticSession(t, tc.records)
			report, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, obs := range report.Observations {
				found = found || (obs.Code == tc.code && obs.CommentID == tc.id)
			}
			if !found {
				t.Fatalf("missing %s: %+v", tc.code, report.Observations)
			}
			selected, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{CommentID: tc.id})
			if (err != nil) != tc.selectError {
				t.Fatalf("selection error = %v, expected error %v", err, tc.selectError)
			}
			if tc.name == "missing evidence" && selected.Comment.Comment.Path != "a.go" {
				t.Fatal("checkpoint path fallback was lost")
			}
		})
	}
}

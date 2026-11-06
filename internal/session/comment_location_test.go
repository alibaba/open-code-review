// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

func TestCommentLocationBeforeCheckpoint(t *testing.T) {
	setTestHome(t, t.TempDir())
	sh := New(t.TempDir(), "main", "test", SessionOptions{})
	defer sh.Finalize()
	call := llm.ToolCall{ID: "call-a", Function: llm.FunctionCall{Name: "code_comment"}}
	rec := sh.GetOrCreateFileSession("group-a").AppendTaskRecord(MainTask, nil)
	rec.SetResponse(&llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{call}}}}}, 0)
	rec.AddToolCallResult(call, "(async)", true, 0)
	cm := model.LlmComment{Path: "a.go", Content: "private finding", ExistingCode: "privateSource()", StartLine: 210, EndLine: 211}
	rec.RecordCommentLocation(call, 0, cm)
	sh.persist.mu.Lock()
	err := sh.persist.writer.Flush()
	sh.persist.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	report, err := Diagnose(sh.RepoDir, sh.SessionID, DiagnoseOptions{RequestID: rec.requestID})
	if err != nil {
		t.Fatal(err)
	}
	phase := report.Tasks[0].Phases[0]
	if !report.Summary.Aborted || phase.CommentLocations != 1 || phase.ToolCalls != 1 || len(phase.Events) != 4 {
		t.Fatalf("stage and dispatch were conflated: %+v", report)
	}
	event := phase.Events[3]
	if event.Type != "comment_location" || event.ResponseID != rec.responseID || event.ToolCallID != call.ID || event.Location.Path != "a.go" || event.Location.Index != 0 || event.Location.StartLine != 210 || event.Location.EndLine != 211 {
		t.Fatalf("location scope lost: %+v", event)
	}
	raw, err := Diagnose(sh.RepoDir, sh.SessionID, DiagnoseOptions{EventLine: event.Line})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private finding", "privateSource()", "original_comment", "code_evidence", "sha256", "evidence_id"} {
		if strings.Contains(string(raw.Event.Record), forbidden) {
			t.Fatalf("stage record stored %q", forbidden)
		}
	}
	var stored map[string]any
	if err := json.Unmarshal(raw.Event.Record, &stored); err != nil || stored["type"] != "comment_location" {
		t.Fatal("raw stage event inaccessible")
	}
	if loaded, err := LoadComments(sh.RepoDir, sh.SessionID); err != nil || len(loaded) != 0 {
		t.Fatal("stage event became a completed checkpoint comment")
	}
}

func TestCommentLocationWithoutPersistence(t *testing.T) {
	for _, rec := range []*TaskRecord{nil, {}, {fileSession: &FileSession{}}, {fileSession: &FileSession{session: &SessionHistory{}}}} {
		rec.RecordCommentLocation(llm.ToolCall{}, 0, model.LlmComment{})
	}
}

func TestCommentLocationLinkValidation(t *testing.T) {
	const records = `{"type":"llm_request","uuid":"req","request_id":"req","filePath":"group-a","taskType":"main_task","request_no":1}
{"type":"llm_response","uuid":"resp","request_id":"req","filePath":"group-a","taskType":"main_task","request_no":1,"tool_calls":[{"id":"call-a","name":"code_comment"}]}
{"type":"comment_location","uuid":"location-a","request_id":"req","response_id":"resp","tool_call_id":"call-a","tool_name":"code_comment","filePath":"group-a","taskType":"main_task","request_no":1,"comment_index":0,"comment_path":"a.go","start_line":0,"end_line":0}
{"type":"session_end"}`
	for _, tc := range []struct{ name, input, code string }{
		{"consistent", records, ""},
		{"wrong tool", strings.Replace(records, `"tool_call_id":"call-a"`, `"tool_call_id":"missing"`, 1), "tool_call_link_mismatch"},
		{"wrong task", strings.Replace(records, `"filePath":"group-a","taskType":"main_task","request_no":1,"comment_index"`, `"filePath":"other-task","taskType":"main_task","request_no":1,"comment_index"`, 1), "request_scope_mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := writeDiagnosticSession(t, tc.input)
			report, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{RequestID: "req"})
			if err != nil {
				t.Fatal(err)
			}
			if tc.code == "" && len(report.Observations) != 0 {
				t.Fatalf("unlocated comment was treated as execution failure: %+v", report.Observations)
			}
			if tc.code != "" && (len(report.Observations) != 1 || report.Observations[0].Code != tc.code) {
				t.Fatalf("broken stage origin not detected: %+v", report.Observations)
			}
			if tc.code == "request_scope_mismatch" && report.Tasks[0].Phases[0].CommentLocations != 0 {
				t.Fatal("wrong-scope stage was counted in the selected request")
			}
		})
	}
}

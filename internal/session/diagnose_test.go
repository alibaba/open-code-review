// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeDiagnosticSession(t *testing.T, records string) string {
	t.Helper()
	setTestHome(t, t.TempDir())
	repo := t.TempDir()
	path, err := SessionFilePath(repo, "diagnostic-run")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(records), 0600); err != nil {
		t.Fatal(err)
	}
	return repo
}

const diagnosticInterleavedRecords = `{"type":"session_start","sessionId":"diagnostic-run"}
{"type":"llm_request","uuid":"a-request","filePath":"group-a","taskType":"main_task","request_no":1,"messages":[{"content":"original prompt"}]}
{"type":"llm_request","uuid":"b-request","filePath":"b.go","taskType":"main_task","request_no":1}
{"type":"tool_call","uuid":"a-tool-ok","parentUuid":"b-request","filePath":"group-a","taskType":"main_task","tool_name":"code_search","ok":true,"result":"Error: provider returned a recoverable result"}
{"type":"llm_error","filePath":"b.go","taskType":"main_task","request_no":1,"error":"provider timeout"}
{"type":"llm_response","filePath":"group-a","taskType":"main_task"}
{"type":"llm_request","filePath":"group-a","taskType":"memory_compression_task","request_no":1}
{"type":"llm_error","filePath":"group-a","taskType":"memory_compression_task","request_no":1,"error":"compression unavailable"}
{"type":"tool_call","uuid":"a-tool-failed","filePath":"group-a","taskType":"main_task","tool_name":"file_read","ok":false,"result":"missing file"}
{"type":"session_end","sessionId":"diagnostic-run","run_manifest":{"schema_version":"ocr.run-manifest/v1","terminal_state":"partial","coverage":{"selected":[{"item_id":"a","path":"a.go"},{"item_id":"b","path":"b.go"}],"completed":[{"item_id":"a","path":"a.go"}],"failed":[{"item_id":"b","path":"b.go","classification":"timeout","reason":"provider timeout"}]}}}`

func TestDiagnoseInterleavedTasks(t *testing.T) {
	repo := writeDiagnosticSession(t, diagnosticInterleavedRecords)
	report, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Aborted || report.Summary.RunManifest.TerminalState != StatePartial {
		t.Fatalf("recorded outcome changed: %+v", report.Summary)
	}
	if len(report.Tasks) != 2 || report.Tasks[0].Key != "group-a" || report.Tasks[1].Key != "b.go" {
		t.Fatalf("interleaved records were attributed incorrectly: %+v", report.Tasks)
	}
	main := report.Tasks[0].Phases[0]
	if main.Requests != 1 || main.Responses != 1 || main.ToolCalls != 2 || main.ToolFailures != 1 {
		t.Fatalf("recorded counts = %+v", main)
	}
	if len(main.Events) != 0 || len(report.Observations) != 3 {
		t.Fatalf("overview retained details or inferred extra failures: %+v", report)
	}
	if report.Observations[0].TaskKey != "b.go" || report.Observations[1].TaskType != "memory_compression_task" {
		t.Fatalf("failure ownership lost: %+v", report.Observations)
	}
	filtered, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{TaskKey: "group-a", EventLine: 9})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Tasks) != 1 || len(filtered.Observations) != 2 || len(filtered.Tasks[0].Phases[0].Events) != 4 {
		t.Fatalf("task drill-down = %+v", filtered)
	}
	if filtered.Event.ID != "a-tool-failed" || filtered.Event.RecordedOK == nil || *filtered.Event.RecordedOK {
		t.Fatalf("event status = %+v", filtered.Event)
	}
	var original map[string]any
	if err := json.Unmarshal(filtered.Event.Record, &original); err != nil || original["result"] != "missing file" {
		t.Fatalf("original evidence not available: %s, %v", filtered.Event.Record, err)
	}
	path, _ := SessionFilePath(repo, "diagnostic-run")
	contents, _ := os.ReadFile(path)
	if string(contents) != diagnosticInterleavedRecords {
		t.Fatal("diagnosis modified the persisted session")
	}
}

func TestDiagnoseIncompleteAndLegacyEvidence(t *testing.T) {
	repo := writeDiagnosticSession(t, `{"type":"session_start"}
{"type":"llm_request","filePath":"a.go","taskType":"main_task","request_no":1}
{"type":"tool_call","filePath":"a.go","taskType":"main_task","result":"`+strings.Repeat("x", 100000)+`"}
not-json
{"type":"llm_response","sessionId":"other-run","filePath":"a.go","taskType":"main_task"}
{"type":"future_event","content":"kept for future readers"}`)
	report, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, obs := range report.Observations {
		codes = append(codes, obs.Code)
	}
	want := []string{"tool_status_unrecorded", "invalid_record", "session_identity_mismatch", "session_end_missing", "request_outcomes_missing"}
	if !reflect.DeepEqual(codes, want) {
		t.Fatalf("observations = %v, want %v", codes, want)
	}
	if !report.Summary.Aborted || report.Tasks[0].Phases[0].ToolFailures != 0 {
		t.Fatal("missing evidence became a recorded failure or completion")
	}
	event, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{EventLine: 6})
	if err != nil || event.Event.Type != "future_event" {
		t.Fatalf("unknown event was not inspectable: %+v, %v", event, err)
	}
}

func TestDiagnoseSelectionErrors(t *testing.T) {
	repo := writeDiagnosticSession(t, diagnosticInterleavedRecords+"\ninvalid\n\n"+`{"type":"llm_response","sessionId":"other-run"}`)
	for _, tc := range []struct {
		name string
		id   string
		opts DiagnoseOptions
	}{
		{"negative event", "diagnostic-run", DiagnoseOptions{EventLine: -1}},
		{"invalid id", "../outside", DiagnoseOptions{}},
		{"empty id", "", DiagnoseOptions{}},
		{"missing file", "missing", DiagnoseOptions{}},
		{"missing task", "diagnostic-run", DiagnoseOptions{TaskKey: "missing"}},
		{"missing event", "diagnostic-run", DiagnoseOptions{EventLine: 100}},
		{"wrong task", "diagnostic-run", DiagnoseOptions{TaskKey: "group-a", EventLine: 3}},
		{"invalid record", "diagnostic-run", DiagnoseOptions{EventLine: 11}},
		{"blank line", "diagnostic-run", DiagnoseOptions{EventLine: 12}},
		{"foreign event", "diagnostic-run", DiagnoseOptions{EventLine: 13}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Diagnose(repo, tc.id, tc.opts); err == nil {
				t.Fatal("expected selection error")
			}
		})
	}
}

func TestDiagnoseReadsWriterOutput(t *testing.T) {
	setTestHome(t, t.TempDir())
	repo := t.TempDir()
	sh := New(repo, "main", "test-model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("a.go")
	rec := fs.AppendTaskRecord(MainTask, nil)
	rec.AddToolResult("code_search", `{}`, "No matches found")
	rec.AddToolFailure("file_read", `{}`, "missing file", 0)
	sh.RecordReviewItemFailed("a.go", "a.go", "a.go", "fp", "stopped")
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}
	report, err := Diagnose(repo, sh.SessionID, DiagnoseOptions{TaskKey: "a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Aborted || !report.Summary.Legacy || len(report.Observations) != 3 {
		t.Fatalf("writer output = %+v", report)
	}
	if report.Observations[1].Code != "review_item_failed" || report.Observations[1].Message != "stopped" {
		t.Fatalf("checkpoint failure not retained: %+v", report.Observations)
	}
}

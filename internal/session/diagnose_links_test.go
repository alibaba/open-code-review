// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestDiagnoseLinksWriterOutOfOrder(t *testing.T) {
	setTestHome(t, t.TempDir())
	repo := t.TempDir()
	sh := New(repo, "main", "test-model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("group-a")
	first := fs.AppendTaskRecord(MainTask, nil)
	second := fs.AppendTaskRecord(MainTask, nil)
	compression := fs.AppendTaskRecord(MemoryCompressionTask, nil)
	calls := []llm.ToolCall{
		{ID: "call-1", Function: llm.FunctionCall{Name: "code_search", Arguments: `{"q":"one"}`}},
		{ID: "call-2", Function: llm.FunctionCall{Name: "code_search", Arguments: `{"q":"two"}`}},
	}
	response := &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: calls}}},
		Usage:   &llm.UsageInfo{},
	}
	second.SetResponse(response, 0)
	second.AddToolCallResult(calls[1], "second result", true, 0)
	first.SetResponse(response, 0)
	first.AddToolCallResult(calls[0], "first request result", true, 0)
	compression.SetError(errors.New("compression unavailable"), 0)
	second.AddToolCallResult(calls[0], "Error: recoverable tool output", true, 0)
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}
	if first.requestID == "" || first.requestID == second.requestID || second.responseID == first.responseID {
		t.Fatal("request and response identities were not distinct")
	}
	report, err := Diagnose(repo, sh.SessionID, DiagnoseOptions{RequestID: second.requestID})
	if err != nil {
		t.Fatal(err)
	}
	phase := report.Tasks[0].Phases[0]
	if len(report.Tasks) != 1 || len(report.Observations) != 0 || phase.Requests != 1 || phase.Responses != 1 || phase.ToolCalls != 2 || phase.ToolFailures != 0 {
		t.Fatalf("request selection mixed execution scopes: %+v", report)
	}
	if len(phase.Events) != 4 || phase.Events[2].ToolCallID != "call-2" || phase.Events[3].ToolCallID != "call-1" {
		t.Fatalf("repeated tool names were matched by position: %+v", phase.Events)
	}
	for _, event := range phase.Events {
		if event.RequestID != second.requestID || event.RequestNo != 2 {
			t.Fatalf("request identity lost: %+v", event)
		}
		if event.Type == "tool_call" && event.ResponseID != second.responseID {
			t.Fatalf("tool result linked to the most recent global response: %+v", event)
		}
	}
	selected, err := Diagnose(repo, sh.SessionID, DiagnoseOptions{RequestID: second.requestID, EventLine: phase.Events[3].Line})
	if err != nil || selected.Event.ToolCallID != "call-1" || !strings.Contains(string(selected.Event.Record), "recoverable tool output") {
		t.Fatalf("linked raw event unavailable: %+v, %v", selected, err)
	}
	all, err := Diagnose(repo, sh.SessionID, DiagnoseOptions{})
	if err != nil || len(all.Observations) != 1 || all.Observations[0].RequestID != compression.requestID {
		t.Fatalf("compression failure lost its own request identity: %+v, %v", all, err)
	}
	failed, err := Diagnose(repo, sh.SessionID, DiagnoseOptions{RequestID: compression.requestID})
	if err != nil {
		t.Fatal(err)
	}
	failedPhase := failed.Tasks[0].Phases[0]
	if failedPhase.LLMErrors != 1 || len(failedPhase.Events) != 2 {
		t.Fatalf("failed request lost its recorded outcome: %+v", failedPhase)
	}
	for _, event := range failedPhase.Events {
		if event.RequestNo != compression.RequestNo || event.RequestID != compression.requestID || event.TaskType != string(MemoryCompressionTask) {
			t.Fatalf("error persistence lost the request scope: %+v", event)
		}
	}
}

const diagnosticLinkedRecords = `{"type":"session_start","sessionId":"diagnostic-run"}
{"type":"llm_request","uuid":"a-request","request_id":"a-request","filePath":"a.go","taskType":"main_task","request_no":1}
{"type":"llm_request","uuid":"b-request","request_id":"b-request","filePath":"b.go","taskType":"main_task","request_no":1}
{"type":"llm_response","uuid":"a-response","request_id":"a-request","filePath":"a.go","taskType":"main_task","request_no":1,"tool_calls":[{"id":"call-1","name":"code_search"}]}
{"type":"llm_response","uuid":"b-response","request_id":"b-request","filePath":"b.go","taskType":"main_task","request_no":1}
`

func TestDiagnoseLinksMissingOutcomeDespiteBalancedCounts(t *testing.T) {
	repo := writeDiagnosticSession(t, `{"type":"llm_request","uuid":"r1","request_id":"r1","filePath":"a.go","taskType":"main_task","request_no":1}
{"type":"llm_request","uuid":"r2","request_id":"r2","filePath":"a.go","taskType":"main_task","request_no":2}
{"type":"llm_response","uuid":"response1","request_id":"r1","filePath":"a.go","taskType":"main_task","request_no":1}
{"type":"llm_response","uuid":"response2","request_id":"r1","filePath":"a.go","taskType":"main_task","request_no":1}
{"type":"session_end"}`)
	report, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	phase := report.Tasks[0].Phases[0]
	if phase.Requests != phase.Responses || len(report.Observations) != 2 {
		t.Fatalf("fixture must have balanced counts but two distinct evidence gaps: %+v", report)
	}
	if report.Observations[0].Code != "request_outcomes_multiple" || report.Observations[0].RequestID != "r1" ||
		report.Observations[1].Code != "request_outcome_missing" || report.Observations[1].RequestID != "r2" || report.Observations[1].Line != 2 {
		t.Fatalf("missing request hidden by another request's duplicate outcome: %+v", report.Observations)
	}
}

func TestDiagnoseLinksInconsistentRecords(t *testing.T) {
	for _, tc := range []struct {
		name, record, code string
	}{
		{"missing request", `{"type":"llm_error","request_id":"absent","filePath":"a.go","taskType":"main_task"}`, "request_record_missing"},
		{"wrong task", `{"type":"llm_response","request_id":"a-request","filePath":"b.go","taskType":"main_task"}`, "request_scope_mismatch"},
		{"wrong phase", `{"type":"llm_response","request_id":"a-request","filePath":"a.go","taskType":"memory_compression_task"}`, "request_scope_mismatch"},
		{"wrong number", `{"type":"llm_response","request_id":"a-request","filePath":"a.go","taskType":"main_task","request_no":2}`, "request_scope_mismatch"},
		{"missing response", `{"type":"tool_call","request_id":"a-request","response_id":"absent","filePath":"a.go","taskType":"main_task","tool_name":"code_search","tool_call_id":"call-1","ok":true}`, "response_record_missing"},
		{"wrong response", `{"type":"tool_call","request_id":"a-request","response_id":"b-response","filePath":"a.go","taskType":"main_task","ok":true}`, "response_link_mismatch"},
		{"wrong tool", `{"type":"tool_call","request_id":"a-request","response_id":"a-response","filePath":"a.go","taskType":"main_task","tool_name":"file_read","tool_call_id":"call-1","ok":true}`, "tool_call_link_mismatch"},
		{"duplicate response", `{"type":"llm_response","uuid":"a-response","request_id":"a-request","filePath":"a.go","taskType":"main_task"}
{"type":"tool_call","request_id":"a-request","response_id":"a-response","filePath":"a.go","taskType":"main_task","ok":true}`, "response_id_duplicate"},
		{"wrong request identity", `{"type":"llm_request","uuid":"wrong","request_id":"inconsistent","filePath":"a.go","taskType":"main_task"}`, "request_identity_mismatch"},
		{"duplicate request", `{"type":"llm_request","uuid":"a-request","request_id":"a-request","filePath":"a.go","taskType":"main_task"}`, "request_id_duplicate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := writeDiagnosticSession(t, diagnosticLinkedRecords+tc.record+"\n"+`{"type":"session_end"}`)
			report, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for _, obs := range report.Observations {
				if obs.Code == tc.code && obs.RequestID != "" && obs.Line > 0 {
					return
				}
			}
			t.Fatalf("missing observation %s: %+v", tc.code, report.Observations)
		})
	}
}

func TestDiagnoseLinksSelections(t *testing.T) {
	repo := writeDiagnosticSession(t, diagnosticLinkedRecords+`{"type":"llm_response","request_id":"a-request","filePath":"b.go","taskType":"main_task"}
{"type":"session_end"}`)
	for _, opts := range []DiagnoseOptions{
		{RequestID: "absent"},
		{RequestID: "a-request", TaskKey: "b.go"},
		{RequestID: "a-request", EventLine: 3},
		{RequestID: "a-request", EventLine: 6},
	} {
		if _, err := Diagnose(repo, "diagnostic-run", opts); err == nil {
			t.Fatalf("invalid request selection accepted: %+v", opts)
		}
	}
	report, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{RequestID: "a-request", TaskKey: "a.go"})
	if err != nil || len(report.Tasks) != 1 || report.Tasks[0].Phases[0].Responses != 1 || len(report.Observations) != 1 {
		t.Fatalf("inconsistent scoped record was attributed to request: %+v, %v", report, err)
	}
	for _, extra := range []string{
		`{"type":"llm_request","uuid":"a-request","request_id":"a-request","filePath":"a.go","taskType":"main_task"}`,
		`{"type":"llm_request","uuid":"other","request_id":"inconsistent","filePath":"a.go","taskType":"main_task"}`,
	} {
		repo := writeDiagnosticSession(t, diagnosticLinkedRecords+extra)
		id := "a-request"
		if strings.Contains(extra, "inconsistent") {
			id = "inconsistent"
		}
		if _, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{RequestID: id}); err == nil {
			t.Fatal("ambiguous or inconsistent request selection accepted")
		}
	}
	legacy := writeDiagnosticSession(t, diagnosticInterleavedRecords)
	if _, err := Diagnose(legacy, "diagnostic-run", DiagnoseOptions{RequestID: "a-request"}); err == nil || !strings.Contains(err.Error(), "older records") {
		t.Fatalf("legacy links were guessed: %v", err)
	}
}

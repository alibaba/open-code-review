// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type DiagnoseOptions struct {
	TaskKey   string
	RequestID string
	EventLine int
}

type DiagnosticReport struct {
	SchemaVersion string                  `json:"schema_version"`
	Summary       *Summary                `json:"summary"`
	Observations  []DiagnosticObservation `json:"observations"`
	Tasks         []DiagnosticTask        `json:"tasks"`
	RequestID     string                  `json:"request_id,omitempty"`
	Event         *DiagnosticEvent        `json:"event,omitempty"`
}

type DiagnosticObservation struct {
	Code      string `json:"code"`
	TaskKey   string `json:"task_key,omitempty"`
	TaskType  string `json:"task_type,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Line      int    `json:"line,omitempty"`
	Message   string `json:"message"`
}

// Task keys are persisted labels, which may identify a group rather than a file.
type DiagnosticTask struct {
	Key    string            `json:"key"`
	Phases []DiagnosticPhase `json:"phases"`
}

type DiagnosticPhase struct {
	Type             string            `json:"type"`
	Requests         int               `json:"requests"`
	Responses        int               `json:"responses"`
	LLMErrors        int               `json:"llm_errors"`
	ToolCalls        int               `json:"tool_calls"`
	ToolFailures     int               `json:"tool_failures"`
	Events           []DiagnosticEvent `json:"events,omitempty"`
	unlinkedRequests int
	unlinkedOutcomes int
}

type DiagnosticEvent struct {
	Line       int             `json:"line"`
	ID         string          `json:"id,omitempty"`
	Type       string          `json:"type"`
	TaskKey    string          `json:"task_key,omitempty"`
	TaskType   string          `json:"task_type,omitempty"`
	RequestNo  int             `json:"request_no,omitempty"`
	RequestID  string          `json:"request_id,omitempty"`
	ResponseID string          `json:"response_id,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	RecordedOK *bool           `json:"recorded_ok,omitempty"`
	Error      string          `json:"error,omitempty"`
	Record     json.RawMessage `json:"record,omitempty"`
}

type diagnosticRecord struct {
	summaryRecord
	ID         string               `json:"uuid"`
	TaskType   string               `json:"taskType"`
	RequestNo  int                  `json:"request_no"`
	ToolName   string               `json:"tool_name"`
	OK         *bool                `json:"ok"`
	RequestID  string               `json:"request_id"`
	ResponseID string               `json:"response_id"`
	ToolCallID string               `json:"tool_call_id"`
	ToolCalls  []diagnosticToolCall `json:"tool_calls"`
}

// Diagnose reads recorded execution facts without assessing finding correctness.
// Physical line numbers remain usable for old sessions without event UUIDs.
func Diagnose(repoDir, sessionID string, opts DiagnoseOptions) (*DiagnosticReport, error) {
	if opts.EventLine < 0 {
		return nil, fmt.Errorf("event line must be positive")
	}
	if err := ValidateSessionID(sessionID); err != nil {
		return nil, err
	}
	path, err := SessionFilePath(repoDir, sessionID)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session %q: %w", sessionID, err)
	}
	defer f.Close()
	report := &DiagnosticReport{
		SchemaVersion: "ocr.session-diagnostics/v1",
		Summary:       &Summary{SessionID: sessionID, RepoDir: repoDir, FilePath: path, Aborted: true},
		Observations:  []DiagnosticObservation{},
		Tasks:         []DiagnosticTask{},
	}
	tasks := make(map[string]int)
	links := newDiagnosticLinks()
	reader := bufio.NewReader(f)
	for lineNo := 1; ; lineNo++ {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && readErr != io.EOF {
			return nil, fmt.Errorf("read session %q: %w", sessionID, readErr)
		}
		if len(bytes.TrimSpace(line)) > 0 {
			var rec diagnosticRecord
			if err := json.Unmarshal(line, &rec); err != nil || rec.Type == "" {
				report.Observations = append(report.Observations, DiagnosticObservation{
					Code: "invalid_record", Line: lineNo, Message: "Record could not be decoded; execution evidence is incomplete.",
				})
				if opts.EventLine == lineNo {
					return nil, fmt.Errorf("event at line %d is not a valid session record", lineNo)
				}
			} else if rec.SessionID != "" && rec.SessionID != sessionID {
				report.Observations = append(report.Observations, DiagnosticObservation{
					Code: "session_identity_mismatch", Line: lineNo, Message: "Record belongs to a different session and was excluded.",
				})
				if opts.EventLine == lineNo {
					return nil, fmt.Errorf("event at line %d belongs to a different session", lineNo)
				}
			} else {
				applyRecordToSummary(report.Summary, rec.summaryRecord)
				report.addDiagnosticRecord(rec, lineNo, opts, tasks)
				links.add(rec, lineNo)
				if opts.EventLine == lineNo {
					event := diagnosticEvent(rec, lineNo)
					event.Record = append(json.RawMessage(nil), bytes.TrimSpace(line)...)
					report.Event = &event
				}
			}
		}
		if readErr == io.EOF {
			break
		}
	}
	report.Observations = append(report.Observations, links.observations()...)
	if report.Summary.Aborted {
		report.Observations = append(report.Observations, DiagnosticObservation{
			Code: "session_end_missing", Message: "No session_end record is present; the session may still be running.",
		})
	}
	for _, task := range report.Tasks {
		for _, phase := range task.Phases {
			if missing := phase.unlinkedRequests - phase.unlinkedOutcomes; missing > 0 {
				report.Observations = append(report.Observations, DiagnosticObservation{
					Code: "request_outcomes_missing", TaskKey: task.Key, TaskType: phase.Type,
					Message: fmt.Sprintf("%d request(s) have no recorded outcome by count; individual requests cannot be identified from these records.", missing),
				})
			}
		}
	}
	if opts.RequestID != "" {
		root, err := links.selectRequest(opts.RequestID)
		if err != nil {
			return nil, err
		}
		if opts.TaskKey != "" && root.TaskKey != opts.TaskKey {
			return nil, fmt.Errorf("request %q does not belong to task %q", opts.RequestID, opts.TaskKey)
		}
		report.RequestID = opts.RequestID
		report.Tasks = []DiagnosticTask{}
		tasks = make(map[string]int)
		for _, linked := range links.records {
			if linked.event.RequestID == opts.RequestID && links.matchesRoot(linked.event) {
				report.addDiagnosticEvent(linked.event, opts, tasks)
			}
		}
	}
	if opts.TaskKey != "" {
		idx, found := tasks[opts.TaskKey]
		if !found {
			return nil, fmt.Errorf("task %q not found in session %s", opts.TaskKey, sessionID)
		}
		report.Tasks = []DiagnosticTask{report.Tasks[idx]}
	}
	filtered := make([]DiagnosticObservation, 0, len(report.Observations))
	for _, obs := range report.Observations {
		if opts.TaskKey != "" && obs.TaskKey != "" && obs.TaskKey != opts.TaskKey && (opts.RequestID == "" || obs.RequestID != opts.RequestID) {
			continue
		}
		if opts.RequestID != "" && obs.RequestID != opts.RequestID && (obs.TaskKey != "" || obs.RequestID != "") {
			continue
		}
		filtered = append(filtered, obs)
	}
	report.Observations = filtered
	if opts.EventLine > 0 {
		if report.Event == nil {
			return nil, fmt.Errorf("event at line %d not found in session %s", opts.EventLine, sessionID)
		}
		if opts.TaskKey != "" && report.Event.TaskKey != opts.TaskKey {
			return nil, fmt.Errorf("event at line %d does not belong to task %q", opts.EventLine, opts.TaskKey)
		}
		if opts.RequestID != "" && (report.Event.RequestID != opts.RequestID || !links.matchesRoot(*report.Event)) {
			return nil, fmt.Errorf("event at line %d is not consistently linked to request %q", opts.EventLine, opts.RequestID)
		}
	}
	return report, nil
}

func diagnosticEvent(rec diagnosticRecord, line int) DiagnosticEvent {
	return DiagnosticEvent{
		Line: line, ID: rec.ID, Type: rec.Type, TaskKey: rec.FilePath,
		TaskType: rec.TaskType, RequestNo: rec.RequestNo, ToolName: rec.ToolName,
		RequestID: rec.RequestID, ResponseID: rec.ResponseID, ToolCallID: rec.ToolCallID,
		RecordedOK: rec.OK, Error: rec.Error,
	}
}

func (r *DiagnosticReport) addDiagnosticRecord(rec diagnosticRecord, line int, opts DiagnoseOptions, tasks map[string]int) {
	code, message := "", rec.Error
	switch rec.Type {
	case "llm_error":
		code = "llm_request_failed"
	case "review_item_failed":
		code = "review_item_failed"
	case "tool_call":
		if rec.OK == nil {
			code, message = "tool_status_unrecorded", "This tool record has no explicit ok value."
		} else if !*rec.OK {
			code, message = "tool_call_failed", "The tool call was recorded with ok=false; inspect the event for its result."
		}
	}
	if code != "" {
		r.Observations = append(r.Observations, DiagnosticObservation{
			Code: code, TaskKey: rec.FilePath, TaskType: rec.TaskType, RequestID: rec.RequestID, Line: line, Message: message,
		})
	}
	r.addDiagnosticEvent(diagnosticEvent(rec, line), opts, tasks)
}

func (r *DiagnosticReport) addDiagnosticEvent(event DiagnosticEvent, opts DiagnoseOptions, tasks map[string]int) {
	switch event.Type {
	case "llm_request", "llm_response", "llm_error", "tool_call":
	default:
		return
	}
	idx, found := tasks[event.TaskKey]
	if !found {
		idx = len(r.Tasks)
		tasks[event.TaskKey] = idx
		r.Tasks = append(r.Tasks, DiagnosticTask{Key: event.TaskKey, Phases: []DiagnosticPhase{}})
	}
	task := &r.Tasks[idx]
	phaseIdx := -1
	for i := range task.Phases {
		if task.Phases[i].Type == event.TaskType {
			phaseIdx = i
			break
		}
	}
	if phaseIdx < 0 {
		phaseIdx = len(task.Phases)
		task.Phases = append(task.Phases, DiagnosticPhase{Type: event.TaskType})
	}
	phase := &task.Phases[phaseIdx]
	switch event.Type {
	case "llm_request":
		phase.Requests++
	case "llm_response":
		phase.Responses++
	case "llm_error":
		phase.LLMErrors++
	case "tool_call":
		phase.ToolCalls++
		if event.RecordedOK != nil && !*event.RecordedOK {
			phase.ToolFailures++
		}
	}
	if event.RequestID == "" {
		switch event.Type {
		case "llm_request":
			phase.unlinkedRequests++
		case "llm_response", "llm_error":
			phase.unlinkedOutcomes++
		}
	}
	if (opts.TaskKey != "" && opts.TaskKey == event.TaskKey) || (opts.RequestID != "" && opts.RequestID == event.RequestID) {
		phase.Events = append(phase.Events, event)
	}
}

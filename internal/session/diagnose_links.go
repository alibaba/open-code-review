// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import "fmt"

type diagnosticToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type diagnosticLinkedRecord struct {
	event DiagnosticEvent
	calls []diagnosticToolCall
}

type diagnosticLinks struct {
	records   []diagnosticLinkedRecord
	requests  map[string][]DiagnosticEvent
	responses map[string][]diagnosticLinkedRecord
}

func newDiagnosticLinks() *diagnosticLinks {
	return &diagnosticLinks{
		requests: make(map[string][]DiagnosticEvent), responses: make(map[string][]diagnosticLinkedRecord),
	}
}

func (l *diagnosticLinks) add(rec diagnosticRecord, line int) {
	if rec.Type == "llm_response" && rec.ID != "" {
		l.responses[rec.ID] = append(l.responses[rec.ID], diagnosticLinkedRecord{event: diagnosticEvent(rec, line), calls: rec.ToolCalls})
	}
	if rec.RequestID == "" {
		return
	}
	switch rec.Type {
	case "llm_request", "llm_response", "llm_error", "tool_call", "comment_location":
	default:
		return
	}
	linked := diagnosticLinkedRecord{event: diagnosticEvent(rec, line), calls: rec.ToolCalls}
	l.records = append(l.records, linked)
	if rec.Type == "llm_request" {
		l.requests[rec.RequestID] = append(l.requests[rec.RequestID], linked.event)
	}
}

func (l *diagnosticLinks) selectRequest(id string) (DiagnosticEvent, error) {
	roots := l.requests[id]
	if len(roots) == 0 {
		return DiagnosticEvent{}, fmt.Errorf("no request with explicit request_id %q; use --task or --event for older records", id)
	}
	if len(roots) != 1 || roots[0].ID != id {
		return DiagnosticEvent{}, fmt.Errorf("request_id %q has ambiguous or inconsistent request records; inspect individual events", id)
	}
	return roots[0], nil
}

func (l *diagnosticLinks) matchesRoot(event DiagnosticEvent) bool {
	roots := l.requests[event.RequestID]
	return len(roots) == 1 && roots[0].ID == event.RequestID &&
		roots[0].TaskKey == event.TaskKey && roots[0].TaskType == event.TaskType &&
		(event.RequestNo == 0 || roots[0].RequestNo == event.RequestNo)
}

func linkObservation(event DiagnosticEvent, code, message string) DiagnosticObservation {
	return DiagnosticObservation{
		Code: code, TaskKey: event.TaskKey, TaskType: event.TaskType,
		RequestID: event.RequestID, Line: event.Line, Message: message,
	}
}

func (l *diagnosticLinks) observations() []DiagnosticObservation {
	var out []DiagnosticObservation
	outcomes := make(map[string]int)
	for _, linked := range l.records {
		event := linked.event
		roots := l.requests[event.RequestID]
		if len(roots) == 0 {
			out = append(out, linkObservation(event, "request_record_missing", "No request record declares the explicitly referenced request_id."))
			continue
		}
		if len(roots) != 1 {
			if event.Type == "llm_request" {
				out = append(out, linkObservation(event, "request_id_duplicate", "Multiple request records declare this request_id; their links are ambiguous."))
			}
			continue
		}
		if roots[0].ID != event.RequestID {
			out = append(out, linkObservation(event, "request_identity_mismatch", "The request_id does not match the referenced request's UUID."))
			continue
		}
		if !l.matchesRoot(event) {
			out = append(out, linkObservation(event, "request_scope_mismatch", "The linked record's task, phase or request number disagrees with its request."))
			continue
		}
		switch event.Type {
		case "llm_response", "llm_error":
			outcomes[event.RequestID]++
		case "tool_call", "comment_location":
			if obs := l.checkToolResponse(linked); obs != nil {
				out = append(out, *obs)
			}
		}
	}
	for _, linked := range l.records {
		event := linked.event
		if event.Type != "llm_request" || !l.matchesRoot(event) {
			continue
		}
		switch count := outcomes[event.RequestID]; {
		case count == 0:
			out = append(out, linkObservation(event, "request_outcome_missing", "No response or LLM error is explicitly linked to this request; it may still be running."))
		case count > 1:
			out = append(out, linkObservation(event, "request_outcomes_multiple", fmt.Sprintf("%d outcomes reference this request; inspect the linked records before interpreting the result.", count)))
		}
	}
	return out
}

func (l *diagnosticLinks) checkToolResponse(linked diagnosticLinkedRecord) *DiagnosticObservation {
	event := linked.event
	if event.ResponseID == "" {
		return nil
	}
	responses := l.responses[event.ResponseID]
	code, message := "", ""
	switch {
	case len(responses) == 0:
		code, message = "response_record_missing", "The linked event's explicitly referenced response is absent."
	case len(responses) != 1:
		code, message = "response_id_duplicate", "The linked event references multiple response records with the same UUID."
	case responses[0].event.RequestID != event.RequestID || !l.matchesRoot(responses[0].event):
		code, message = "response_link_mismatch", "The linked event's response has no consistent recorded link to the same request and task."
	case event.ToolCallID != "":
		matches := 0
		for _, call := range responses[0].calls {
			if call.ID == event.ToolCallID && call.Name == event.ToolName {
				matches++
			}
		}
		if matches != 1 {
			code, message = "tool_call_link_mismatch", "The referenced response does not declare exactly one tool call with this ID and name."
		}
	}
	if code == "" {
		return nil
	}
	obs := linkObservation(event, code, message)
	return &obs
}

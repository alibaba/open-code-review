// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"time"

	"github.com/alibaba/open-code-review/internal/model"
)

type commentDecisionRecord struct {
	DecisionVersion string            `json:"decision_version,omitempty"`
	EvidenceID      string            `json:"evidence_id"`
	InputIndex      int               `json:"input_index"`
	Stage           string            `json:"stage"`
	Outcome         string            `json:"outcome"`
	Output          *model.LlmComment `json:"output_comment,omitempty"`
}

func (d commentDecisionRecord) validShape() bool {
	if d.Stage == "" || d.InputIndex < 0 {
		return false
	}
	switch d.Outcome {
	case "removed":
		return d.Output == nil
	case "retained", "rewritten":
		return d.Output != nil && d.Output.EvidenceID == d.EvidenceID
	case "merged":
		return d.Output != nil && d.Output.EvidenceID != d.EvidenceID
	}
	return false
}

// RecordCommentDecision records only decisions actually applied by a processor.
// Output may identify a different comment when dedup merges this input into it.
func (tr *TaskRecord) RecordCommentDecision(stage string, index int, before model.LlmComment, after *model.LlmComment) {
	if tr == nil || tr.fileSession == nil || tr.fileSession.session.persist == nil || tr.responseID == "" || before.EvidenceID == "" {
		return
	}
	outcome := "removed"
	if after != nil {
		switch {
		case before.EvidenceID != after.EvidenceID:
			outcome = "merged"
		case before != *after:
			outcome = "rewritten"
		default:
			outcome = "retained"
		}
	}
	fs := tr.fileSession
	jw := fs.session.persist
	link := tr.requestLink()
	link.ResponseID = tr.responseID
	id := generateUUID()
	jw.mu.Lock()
	defer jw.mu.Unlock()
	rec := map[string]any{
		"uuid": id, "parentUuid": jw.lastUUID, "type": "comment_decision", "sessionId": jw.sessionID,
		"decision_version": "ocr.comment-decision/v1",
		"timestamp":        time.Now().UTC().Format(time.RFC3339), "filePath": fs.FilePath, "taskType": string(tr.Type),
		"evidence_id": before.EvidenceID, "input_index": index, "stage": stage, "outcome": outcome,
	}
	if after != nil {
		rec["output_comment"] = *after
	}
	link.apply(rec)
	jw.writeRecordLocked(rec)
	jw.lastUUID = id
}

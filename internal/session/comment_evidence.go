// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

type commentEvidenceRecord struct {
	CommentIndex        int                       `json:"comment_index"`
	OriginalComment     model.LlmComment          `json:"original_comment"`
	Comment             model.LlmComment          `json:"comment"`
	Code                model.CommentCodeEvidence `json:"code_evidence"`
	RelocationRequestID string                    `json:"relocation_request_id,omitempty"`
}

func (tr *TaskRecord) RecordCommentEvidence(call llm.ToolCall, index int, original model.LlmComment, cm *model.LlmComment, code model.CommentCodeEvidence, relocation *TaskRecord) {
	if tr == nil || tr.fileSession == nil || tr.fileSession.session.persist == nil || cm == nil {
		return
	}
	fs := tr.fileSession
	cm.EvidenceID = generateUUID()
	record := commentEvidenceRecord{CommentIndex: index, OriginalComment: original, Comment: *cm, Code: code}
	if relocation != nil {
		record.RelocationRequestID = relocation.requestID
	}
	link := tr.requestLink()
	link.ResponseID, link.ToolCallID = tr.responseID, call.ID
	fs.session.persist.writeCommentEvidence(cm.EvidenceID, fs.FilePath, tr.Type, call.Function.Name, link, record)
}

func (jw *jsonlWriter) writeCommentEvidence(id, taskKey string, taskType TaskType, toolName string, link recordLink, record commentEvidenceRecord) {
	jw.mu.Lock()
	defer jw.mu.Unlock()
	rec := map[string]any{
		"uuid": id, "parentUuid": jw.lastUUID, "type": "comment_evidence", "sessionId": jw.sessionID,
		"timestamp": time.Now().UTC().Format(time.RFC3339), "filePath": taskKey, "taskType": string(taskType),
		"tool_name": toolName, "comment_index": record.CommentIndex, "original_comment": record.OriginalComment,
		"comment": record.Comment, "code_evidence": record.Code,
	}
	if record.RelocationRequestID != "" {
		rec["relocation_request_id"] = record.RelocationRequestID
	}
	link.apply(rec)
	jw.writeRecordLocked(rec)
	jw.lastUUID = id
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

type CommentLocation struct {
	Index     int    `json:"comment_index"`
	Path      string `json:"comment_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// RecordCommentLocation marks the point after location processing, before collection.
// A zero range means processing returned without locating the comment.
func (tr *TaskRecord) RecordCommentLocation(call llm.ToolCall, index int, cm model.LlmComment) {
	if tr == nil || tr.fileSession == nil || tr.fileSession.session == nil || tr.fileSession.session.persist == nil {
		return
	}
	fs := tr.fileSession
	jw := fs.session.persist
	link := tr.requestLink()
	link.ResponseID, link.ToolCallID = tr.responseID, call.ID
	id := generateUUID()
	jw.mu.Lock()
	defer jw.mu.Unlock()
	rec := map[string]any{
		"uuid": id, "parentUuid": jw.lastUUID, "type": "comment_location", "sessionId": jw.sessionID,
		"timestamp": time.Now().UTC().Format(time.RFC3339), "filePath": fs.FilePath, "taskType": string(tr.Type),
		"tool_name": call.Function.Name, "comment_index": index, "comment_path": cm.Path,
		"start_line": cm.StartLine, "end_line": cm.EndLine,
	}
	link.apply(rec)
	jw.writeRecordLocked(rec)
	jw.lastUUID = id
}

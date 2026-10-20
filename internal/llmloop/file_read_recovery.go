// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import "github.com/alibaba/open-code-review/internal/tool"

// FileReadRecovery records observed access to a verified alternative path,
// independently of task completion or the historical failure count.
type FileReadRecovery struct {
	Status         string           `json:"status"`
	TargetCommit   string           `json:"target_commit,omitempty"`
	CandidatePath  string           `json:"candidate_path,omitempty"`
	SuccessfulRead *FileReadSuccess `json:"successful_read,omitempty"`
}

type FileReadSuccess struct {
	ToolCallNumber int64  `json:"tool_call_number"`
	Arguments      string `json:"arguments"`
	tool.FileReadContent
}

func (r *Runner) recordFileReadSuccess(number int64, taskKey, arguments string, evidence *tool.FileReadEvidence) {
	if evidence == nil || evidence.TargetCommit == "" || evidence.Read == nil {
		return
	}
	r.toolCallsMu.Lock()
	defer r.toolCallsMu.Unlock()
	for i := range r.toolFailures {
		failure := &r.toolFailures[i]
		recovery := failure.Recovery
		if failure.ToolName != tool.FileRead.Name() || failure.FilePath != taskKey || failure.ToolCallNumber >= number ||
			recovery == nil || recovery.Status != "not_observed" || recovery.CandidatePath == "" ||
			recovery.TargetCommit != evidence.TargetCommit || recovery.CandidatePath != evidence.Read.FilePath {
			continue
		}
		recovery.Status = "candidate_read"
		recovery.SuccessfulRead = &FileReadSuccess{
			ToolCallNumber:  number,
			Arguments:       arguments,
			FileReadContent: *evidence.Read,
		}
	}
}

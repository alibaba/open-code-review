// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"fmt"

	"github.com/alibaba/open-code-review/internal/model"
)

type DiagnosticComment struct {
	ID                  string                      `json:"id"`
	Line                int                         `json:"line,omitempty"`
	TaskKey             string                      `json:"task_key,omitempty"`
	RequestID           string                      `json:"request_id,omitempty"`
	ResponseID          string                      `json:"response_id,omitempty"`
	ToolCallID          string                      `json:"tool_call_id,omitempty"`
	CommentIndex        int                         `json:"comment_index"`
	Path                string                      `json:"path"`
	StartLine           int                         `json:"start_line"`
	EndLine             int                         `json:"end_line"`
	EvidenceStatus      string                      `json:"evidence_status"`
	RelocationRequestID string                      `json:"relocation_request_id,omitempty"`
	Checkpoints         []DiagnosticCommentRef      `json:"checkpoints"`
	CheckpointState     string                      `json:"checkpoint_state"`
	Decisions           []DiagnosticCommentDecision `json:"decisions,omitempty"`
	MergedFrom          []string                    `json:"merged_from,omitempty"`
	OriginalComment     *model.LlmComment           `json:"original_comment,omitempty"`
	Comment             *model.LlmComment           `json:"comment,omitempty"`
	Code                *model.CommentCodeEvidence  `json:"code_evidence,omitempty"`
}

type DiagnosticCommentRef struct {
	Line            int               `json:"line"`
	Index           int               `json:"index"`
	Type            string            `json:"type"`
	SourceSessionID string            `json:"source_session_id,omitempty"`
	Active          bool              `json:"active"`
	Comment         *model.LlmComment `json:"comment,omitempty"`
}

type DiagnosticCommentDecision struct {
	Line             int               `json:"line"`
	Stage            string            `json:"stage"`
	Outcome          string            `json:"outcome"`
	InputIndex       int               `json:"input_index"`
	TaskKey          string            `json:"task_key"`
	RequestID        string            `json:"request_id,omitempty"`
	ResponseID       string            `json:"response_id,omitempty"`
	OutputEvidenceID string            `json:"output_evidence_id,omitempty"`
	Output           *model.LlmComment `json:"output_comment,omitempty"`
}

type diagnosticCommentNode struct {
	summary   DiagnosticComment
	record    *commentEvidenceRecord
	events    int
	comment   model.LlmComment
	claims    []model.LlmComment
	decisions []commentDecisionRecord
}

type diagnosticComments struct {
	order           []*diagnosticCommentNode
	byID            map[string]*diagnosticCommentNode
	checkpointLines map[string]int
	activeLines     map[int]bool
}

func newDiagnosticComments() *diagnosticComments {
	return &diagnosticComments{byID: make(map[string]*diagnosticCommentNode), checkpointLines: make(map[string]int), activeLines: make(map[int]bool)}
}

func (c *diagnosticComments) node(id string) *diagnosticCommentNode {
	if node := c.byID[id]; node != nil {
		return node
	}
	node := &diagnosticCommentNode{summary: DiagnosticComment{ID: id, EvidenceStatus: "unrecorded", Checkpoints: []DiagnosticCommentRef{}}}
	c.byID[id] = node
	c.order = append(c.order, node)
	return node
}

func (c *diagnosticComments) add(rec diagnosticRecord, line int) {
	if rec.Type == "comment_decision" && rec.EvidenceID != "" {
		node := c.node(rec.EvidenceID)
		decision := DiagnosticCommentDecision{
			Line: line, Stage: rec.Stage, Outcome: rec.Outcome, InputIndex: rec.InputIndex,
			TaskKey: rec.FilePath, RequestID: rec.RequestID, ResponseID: rec.ResponseID,
		}
		if rec.Output != nil {
			decision.OutputEvidenceID = rec.Output.EvidenceID
			if rec.Outcome == "merged" && rec.Output.EvidenceID != "" && rec.commentDecisionRecord.validShape() && (rec.DecisionVersion == "" || rec.DecisionVersion == "ocr.comment-decision/v1") {
				target := c.node(rec.Output.EvidenceID)
				target.summary.MergedFrom = append(target.summary.MergedFrom, rec.EvidenceID)
			}
		}
		node.summary.Decisions = append(node.summary.Decisions, decision)
		node.decisions = append(node.decisions, rec.commentDecisionRecord)
		return
	}
	if rec.Type == "comment_evidence" {
		node := c.node(rec.ID)
		node.events++
		if node.events == 1 {
			record := rec.commentEvidenceRecord
			node.record = &record
			node.comment = record.Comment
			node.summary.Line, node.summary.TaskKey = line, rec.FilePath
			node.summary.RequestID, node.summary.ResponseID, node.summary.ToolCallID = rec.RequestID, rec.ResponseID, rec.ToolCallID
			node.summary.CommentIndex, node.summary.RelocationRequestID = record.CommentIndex, record.RelocationRequestID
			if record.Code.Status != "" {
				node.summary.EvidenceStatus = record.Code.Status
			}
		}
		return
	}
	if rec.Type != "review_item_done" && rec.Type != "review_item_reused" && rec.Type != "review_item_failed" {
		return
	}
	if rec.Type == "review_item_failed" {
		delete(c.activeLines, c.checkpointLines[rec.Fingerprint])
		return
	}
	var comments []model.LlmComment
	if json.Unmarshal(rec.Comments, &comments) != nil {
		if len(rec.Comments) > 0 {
			return
		}
	}
	if rec.Fingerprint != "" {
		delete(c.activeLines, c.checkpointLines[rec.Fingerprint])
		c.checkpointLines[rec.Fingerprint] = line
	}
	c.activeLines[line] = true
	for i, cm := range comments {
		if cm.EvidenceID == "" {
			continue
		}
		if cm.Path == "" {
			cm.Path = rec.FilePath
			if cm.Path == "" {
				cm.Path = rec.NewPath
			}
		}
		node := c.node(cm.EvidenceID)
		if node.record == nil {
			node.comment = cm
		}
		node.claims = append(node.claims, cm)
		node.summary.Checkpoints = append(node.summary.Checkpoints, DiagnosticCommentRef{
			Line: line, Index: i, Type: rec.Type, SourceSessionID: rec.SourceSessionID,
		})
	}
}

func (c *diagnosticComments) finish(report *DiagnosticReport, opts DiagnoseOptions, links *diagnosticLinks) error {
	for _, node := range c.order {
		item := node.summary
		item.Checkpoints = append([]DiagnosticCommentRef(nil), node.summary.Checkpoints...)
		item.Decisions = append([]DiagnosticCommentDecision(nil), node.summary.Decisions...)
		item.CheckpointState = "unrecorded"
		if len(item.Checkpoints) > 0 {
			item.CheckpointState = "absent"
			for i := range item.Checkpoints {
				item.Checkpoints[i].Active = c.activeLines[item.Checkpoints[i].Line]
				if item.Checkpoints[i].Active {
					item.CheckpointState = "present"
				}
			}
		}
		item.Path, item.StartLine, item.EndLine = node.comment.Path, node.comment.StartLine, node.comment.EndLine
		observe := func(code, message string, line int) {
			report.Observations = append(report.Observations, DiagnosticObservation{
				Code: code, TaskKey: item.TaskKey, RequestID: item.RequestID, CommentID: item.ID, Line: line, Message: message,
			})
		}
		if node.events > 1 {
			observe("comment_evidence_duplicate", "Multiple evidence events have this ID; the comment origin is ambiguous.", item.Line)
		}
		if node.record != nil && (item.ID == "" || node.comment.EvidenceID != item.ID) {
			observe("comment_identity_mismatch", "The captured comment does not reference its evidence event's UUID.", item.Line)
		}
		if item.RelocationRequestID != "" {
			request, err := links.selectRequest(item.RelocationRequestID)
			if err != nil || request.TaskKey != item.Path || request.TaskType != string(ReLocationTask) {
				observe("comment_relocation_link_mismatch", "The re-location request does not resolve to a unique request for this comment's path and re-location phase.", item.Line)
			}
		}
		ambiguous := false
		if node.record != nil && node.record.EvidenceVersion != "" && node.record.EvidenceVersion != "ocr.comment-evidence/v1" {
			ambiguous = true
			observe("comment_evidence_version_unknown", "The evidence format version is unsupported; inspect the raw event instead.", item.Line)
		}
		if node.record == nil && len(node.claims) == 0 {
			observe("comment_evidence_missing", "A processing decision references a comment whose origin and checkpoint are absent from this session.", 0)
		}
		for i, decision := range node.decisions {
			if decision.Outcome == "merged" && decision.Output != nil && decision.Output.EvidenceID == "" {
				observe("comment_merge_target_untracked", "The comment was merged into a legacy output without an evidence ID; inspect the saved output and processing request without assuming an origin.", item.Decisions[i].Line)
			}
			if decision.DecisionVersion != "" && decision.DecisionVersion != "ocr.comment-decision/v1" {
				ambiguous = true
				observe("comment_decision_version_unknown", "The decision format version is unsupported; inspect the raw event instead.", item.Decisions[i].Line)
			}
			if !decision.validShape() {
				ambiguous = true
				observe("comment_decision_invalid", "The recorded decision has an inconsistent outcome or comment identity; inspect the source event.", item.Decisions[i].Line)
			}
		}
		for i, claim := range node.claims {
			ref := item.Checkpoints[i]
			if node.record == nil {
				if claim != node.comment {
					ambiguous = true
					observe("comment_checkpoint_ambiguous", "Checkpoints reference the same evidence ID with different comment bodies; inspect individual records.", ref.Line)
				}
				if ref.Type == "review_item_reused" && ref.SourceSessionID != "" {
					item.EvidenceStatus = "external"
				} else {
					observe("comment_evidence_missing", "The checkpoint references evidence that is not recorded in this session.", ref.Line)
				}
			} else if claim != node.comment {
				observe("comment_snapshot_changed", "The checkpoint comment differs from its captured evidence; inspect both records before reusing the code span.", ref.Line)
			}
		}
		matches := (opts.TaskKey == "" || opts.TaskKey == item.TaskKey) && (opts.RequestID == "" || opts.RequestID == item.RequestID)
		if matches {
			report.Comments = append(report.Comments, item)
		}
		if opts.CommentID == item.ID && opts.CommentID != "" {
			if !matches {
				return fmt.Errorf("comment %q does not have the selected task and request origin", item.ID)
			}
			if ambiguous || node.events > 1 || (node.record != nil && node.comment.EvidenceID != item.ID) {
				return fmt.Errorf("comment %q has ambiguous or inconsistent evidence; inspect individual events", item.ID)
			}
			item.Comment = &node.comment
			for i := range item.Checkpoints {
				item.Checkpoints[i].Comment = &node.claims[i]
			}
			for i := range item.Decisions {
				item.Decisions[i].Output = node.decisions[i].Output
			}
			if node.record != nil {
				item.OriginalComment, item.Code = &node.record.OriginalComment, &node.record.Code
			}
			report.Comment = &item
		}
	}
	if opts.CommentID != "" && report.Comment == nil {
		return fmt.Errorf("comment evidence %q not found; use --event for older comments without evidence IDs", opts.CommentID)
	}
	return nil
}

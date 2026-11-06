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

func TestCommentDecisionPersistence(t *testing.T) {
	setTestHome(t, t.TempDir())
	sh := New(t.TempDir(), "main", "test", SessionOptions{})
	call := llm.ToolCall{ID: "call-a", Function: llm.FunctionCall{Name: "code_comment"}}
	origin := sh.GetOrCreateFileSession("a.go").AppendTaskRecord(MainTask, nil)
	origin.SetResponse(&llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{call}}}}}, 0)
	var comments []model.LlmComment
	for i := range 4 {
		cm := model.LlmComment{Path: "a.go", Content: "private finding", ExistingCode: "privateCode()"}
		origin.RecordCommentEvidence(call, i, cm, &cm, model.CommentCodeEvidence{Status: "unavailable", Path: cm.Path}, nil)
		comments = append(comments, cm)
	}
	process := sh.GetOrCreateFileSession("group-a").AppendTaskRecord(ReviewFilterTask, nil)
	process.SetResponse(&llm.ChatResponse{Choices: []llm.Choice{{}}}, 0)
	merged := comments[0]
	merged.Content = "private merged finding"
	process.RecordCommentDecision("test_processing", 0, comments[0], &merged)
	process.RecordCommentDecision("test_processing", 1, comments[1], &merged)
	process.RecordCommentDecision("test_processing", 2, comments[2], nil)
	process.RecordCommentDecision("test_processing", 3, comments[3], &comments[3])
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp", []model.LlmComment{merged, comments[3]})
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}

	overview, err := Diagnose(sh.RepoDir, sh.SessionID, DiagnoseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(overview)
	if strings.Contains(string(raw), "private finding") || strings.Contains(string(raw), "private merged finding") || strings.Contains(string(raw), "privateCode()") {
		t.Fatal("overview exposed comment or decision bodies")
	}
	for i, outcome := range []string{"rewritten", "merged", "removed", "retained"} {
		report, err := Diagnose(sh.RepoDir, sh.SessionID, DiagnoseOptions{CommentID: comments[i].EvidenceID})
		if err != nil {
			t.Fatal(err)
		}
		cm := report.Comment
		if len(cm.Decisions) != 1 || cm.Decisions[0].Outcome != outcome || cm.Decisions[0].RequestID != process.requestID || cm.Decisions[0].ResponseID != process.responseID || cm.Decisions[0].InputIndex != i {
			t.Fatalf("decision %d = %+v", i, cm.Decisions)
		}
		if i == 0 && (len(cm.MergedFrom) != 1 || cm.MergedFrom[0] != comments[1].EvidenceID || cm.Checkpoints[0].Comment.Content != merged.Content) {
			t.Fatalf("canonical merge lost its input or checkpoint snapshot: %+v", cm)
		}
		if i == 1 && cm.Decisions[0].Output.EvidenceID != comments[0].EvidenceID {
			t.Fatal("merge target lost")
		}
		if i == 2 && (cm.Decisions[0].Output != nil || cm.CheckpointState != "unrecorded") {
			t.Fatal("removed comment was given a final checkpoint")
		}
	}
}

func TestCommentDecisionWithoutPersistence(t *testing.T) {
	before := model.LlmComment{EvidenceID: "before"}
	for _, rec := range []*TaskRecord{nil, {}, {fileSession: &FileSession{session: &SessionHistory{}}}} {
		rec.RecordCommentDecision("review_filter", 0, before, nil)
		rec.RecordCommentEvidence(llm.ToolCall{}, 0, before, &before, model.CommentCodeEvidence{}, nil)
	}
	if before.EvidenceID != "before" {
		t.Fatal("disabled persistence changed identity")
	}
}

func TestDiagnoseCommentDecisionInvalid(t *testing.T) {
	repo := writeDiagnosticSession(t, diagnosticCommentRecords+"\n"+`{"type":"comment_decision","evidence_id":"comment-a","stage":"scan_dedup","input_index":-1,"outcome":"merged","output_comment":{"evidence_id":"other"}}`)
	report, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, obs := range report.Observations {
		found = found || obs.Code == "comment_decision_invalid"
	}
	if !found {
		t.Fatal("inconsistent processing decision was accepted without an observation")
	}
	if len(report.Comments) != 1 {
		t.Fatal("an invalid decision created a canonical merge relationship")
	}
	if _, err := Diagnose(repo, "diagnostic-run", DiagnoseOptions{CommentID: "comment-a"}); err == nil {
		t.Fatal("invalid decision was accepted in aggregate comment selection")
	}
}

func TestDiagnoseCommentEvidenceReplayAndReuseChain(t *testing.T) {
	setTestHome(t, t.TempDir())
	repo := t.TempDir()
	first := New(repo, "main", "test", SessionOptions{})
	rec := first.GetOrCreateFileSession("a.go").AppendTaskRecord(MainTask, nil)
	cm := model.LlmComment{Path: "a.go", Content: "finding"}
	rec.RecordCommentEvidence(llm.ToolCall{}, 0, cm, &cm, model.CommentCodeEvidence{Status: "unavailable"}, nil)
	first.RecordReviewItemDone("a.go", "a.go", "a.go", "fp", []model.LlmComment{cm})
	first.RecordReviewItemFailed("a.go", "a.go", "a.go", "fp", "retry failed")
	first.RecordReviewItemDone("a.go", "a.go", "a.go", "fp", []model.LlmComment{cm})
	if err := first.Finalize(); err != nil {
		t.Fatal(err)
	}
	report, err := Diagnose(repo, first.SessionID, DiagnoseOptions{CommentID: cm.EvidenceID})
	if err != nil {
		t.Fatal(err)
	}
	if report.Comment.CheckpointState != "present" || report.Comment.Checkpoints[0].Active || !report.Comment.Checkpoints[1].Active {
		t.Fatal("superseded checkpoints were treated as active")
	}
	previous := first
	for range 2 {
		loaded, err := LoadComments(repo, previous.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		next := New(repo, "main", "test", SessionOptions{ResumedFrom: previous.SessionID})
		next.RecordReviewItemReused("a.go", "a.go", "a.go", "fp", previous.SessionID, loaded)
		if err := next.Finalize(); err != nil {
			t.Fatal(err)
		}
		report, err := Diagnose(repo, next.SessionID, DiagnoseOptions{CommentID: cm.EvidenceID})
		if err != nil {
			t.Fatal(err)
		}
		if report.Comment.Code != nil || report.Comment.RequestID != "" || report.Comment.EvidenceStatus != "external" || report.Comment.Checkpoints[0].SourceSessionID != previous.SessionID {
			t.Fatalf("resume invented a local comment origin: %+v", report.Comment)
		}
		previous = next
	}
}

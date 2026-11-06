// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestDedupCommentEvidenceLegacyCanonical(t *testing.T) {
	setTestHome(t, t.TempDir())
	sh := session.New(t.TempDir(), "main", "test", session.SessionOptions{})
	call := llm.ToolCall{ID: "comment-call", Function: llm.FunctionCall{Name: "code_comment"}}
	origin := sh.GetOrCreateFileSession("a.go").AppendTaskRecord(session.MainTask, nil)
	origin.SetResponse(&llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{call}}}}}, 0)
	cm := model.LlmComment{Path: "a.go", Content: "new finding"}
	origin.RecordCommentEvidence(call, 0, cm, &cm, model.CommentCodeEvidence{Status: "unavailable"}, nil)
	collector := tool.NewCommentCollector()
	collector.Add(model.LlmComment{Path: "a.go", Content: "legacy finding"})
	collector.Add(cm)
	content := `{"groups":[{"members":["c-0","c-1"],"merged_content":"combined finding"}]}`
	client := &fakeScanClient{responses: []*llm.ChatResponse{{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}}}}}
	tpl := makeTemplateWithFullScan()
	tpl.DedupTask = &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "{{batch_comments}}"}}}
	a := NewAgent(Args{Template: tpl, LLMClient: client, Model: "test", Session: sh, CommentCollector: collector, Tools: tool.NewRegistry()})
	a.maybeRunDedup(context.Background(), 0, 0)
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}
	comments := collector.Comments()
	if len(comments) != 1 || comments[0].EvidenceID != "" || comments[0].Content != "combined finding" {
		t.Fatal("legacy canonical identity was invented or the existing merge changed")
	}
	report, err := session.Diagnose(sh.RepoDir, sh.SessionID, session.DiagnoseOptions{CommentID: cm.EvidenceID})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Comment.Decisions) != 1 || report.Comment.Decisions[0].Outcome != "merged" || report.Comment.Decisions[0].OutputEvidenceID != "" || report.Comment.Decisions[0].Output.Content != "combined finding" {
		t.Fatalf("merge into an untracked legacy output was lost: %+v", report.Comment)
	}
	found := false
	for _, obs := range report.Observations {
		found = found || obs.Code == "comment_merge_target_untracked"
	}
	if !found {
		t.Fatal("legacy merge target was not identified as untracked")
	}
}

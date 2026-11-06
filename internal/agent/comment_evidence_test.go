// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestReviewFilterCommentEvidence(t *testing.T) {
	setTestHome(t, t.TempDir())
	sh := session.New(t.TempDir(), "main", "test", session.SessionOptions{})
	call := llm.ToolCall{ID: "comment-call", Function: llm.FunctionCall{Name: "code_comment"}}
	origin := sh.GetOrCreateFileSession("a.go").AppendTaskRecord(session.MainTask, nil)
	origin.SetResponse(&llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{call}}}}}, 0)
	collector := tool.NewCommentCollector()
	var inputs []model.LlmComment
	for i := range 2 {
		cm := model.LlmComment{Path: "a.go", Content: "finding"}
		origin.RecordCommentEvidence(call, i, cm, &cm, model.CommentCodeEvidence{Status: "unavailable"}, nil)
		inputs = append(inputs, cm)
		collector.Add(cm)
	}
	filter := llm.ToolCall{ID: "filter-call", Function: llm.FunctionCall{Name: "report_incorrect_comments", Arguments: `{"comment_ids":["c-1"]}`}}
	client := &fakeAgentClient{responses: []*llm.ChatResponse{{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{filter}}}}}}}
	a := New(Args{
		LLMClient: client, Model: "test", Session: sh, CommentCollector: collector,
		Template: template.Template{MaxTokens: 10000, MainTask: template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "review"}}}, ReviewFilterTask: &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "{{comments}}"}}}},
	})
	a.executeGroupReviewFilter(context.Background(), FileGroup{Diffs: []model.Diff{{NewPath: "a.go"}}}, nil)
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp", collector.Comments())
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}
	for i, expected := range []string{"retained", "removed"} {
		report, err := session.Diagnose(sh.RepoDir, sh.SessionID, session.DiagnoseOptions{CommentID: inputs[i].EvidenceID})
		if err != nil {
			t.Fatal(err)
		}
		cm := report.Comment
		if len(report.Observations) != 0 || len(cm.Decisions) != 1 || cm.Decisions[0].Stage != "review_filter" || cm.Decisions[0].Outcome != expected || cm.Decisions[0].RequestID == cm.RequestID {
			t.Fatalf("filter origin or applied decision missing: %+v, %+v", cm, report.Observations)
		}
		request, err := session.Diagnose(sh.RepoDir, sh.SessionID, session.DiagnoseOptions{RequestID: cm.Decisions[0].RequestID})
		if err != nil || request.Tasks[0].Phases[0].Type != string(session.ReviewFilterTask) {
			t.Fatalf("filter request cannot be inspected: %+v, %v", request, err)
		}
		if i == 1 && len(cm.Checkpoints) != 0 {
			t.Fatal("filter removal was treated as checkpoint acceptance")
		}
	}
}

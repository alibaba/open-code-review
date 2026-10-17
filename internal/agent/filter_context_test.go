// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type filterContextClient func(llm.ChatRequest) (*llm.ChatResponse, error)

func (f filterContextClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return f(req)
}

func TestReviewFilterContextAndDecisions(t *testing.T) {
	tpl, err := template.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, evidence, decision string
		remove, incomplete       bool
	}{
		{"real defect", "The supported caller requires validation, but untrusted input bypasses it.", "approve_all_comments", false, false},
		{"documented limit", "The schema declares that renames preserve the existing stable identity.", "report_incorrect_comments", true, false},
		{"optional diagnostic", "The API contract deliberately exposes one error category and rejects malformed input.", "report_incorrect_comments", true, false},
		{"missing context", "The required caller contract could not be read.", "report_incomplete_review", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sess := session.New(t.TempDir(), "main", "fake", session.SessionOptions{ReviewMode: "diff"})
			fs := sess.GetOrCreateFileSession("check.go")
			fs.AppendTaskRecord(session.PlanTask, nil).AddToolResult("file_read", `{"path":"scope.md"}`, "Supported mode is attended validation.")
			rec := fs.AppendTaskRecord(session.MainTask, nil)
			if tc.incomplete {
				rec.AddToolFailure("file_read", `{"path":"contract.md"}`, tc.evidence, time.Millisecond)
			} else {
				rec.AddToolResult("file_read", `{"path":"contract.md"}`, tc.evidence)
			}
			sess.GetOrCreateFileSession("other.go").AppendTaskRecord(session.MainTask, nil).AddToolResult("file_read", "{}", "unrelated group evidence")
			collector := tool.NewCommentCollector()
			collector.Add(model.LlmComment{Path: "check.go", Content: "Candidate under review"})
			calls := 0
			client := filterContextClient(func(req llm.ChatRequest) (*llm.ChatResponse, error) {
				calls++
				var prompt string
				for _, m := range req.Messages {
					prompt += m.ExtractText()
				}
				for _, want := range []string{"Respect the supported caller contract.", "Verify untrusted input validation.", "Supported mode is attended validation.", "contract.md", tc.evidence, "+validate(input)", "+// Literal: {{review_context}}", "Actual defect", "Never remove by category or wording alone."} {
					if !strings.Contains(prompt, want) {
						t.Errorf("filter omitted %q", want)
					}
				}
				for _, absent := range []string{"unrelated group evidence", "{{requirement_background}}", "{{system_rule}}", "value veto"} {
					if strings.Contains(prompt, absent) {
						t.Errorf("filter contains %q", absent)
					}
				}
				if tc.incomplete && !strings.Contains(prompt, `"OK":false`) {
					t.Error("failed retrieval was not identified")
				}
				// Fixed responses test delivery and disposition, not a model's judgment.
				return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{{
					Type: "function", Function: llm.FunctionCall{Name: tc.decision, Arguments: `{"analysis":["fixture decision"],"comment_ids":["c-0"],"reason":"required contract unavailable"}`},
				}}}}}}, nil
			})
			a := New(Args{Session: sess, LLMClient: client, Model: "fake", Template: *tpl, CommentCollector: collector, Background: "Respect the supported caller contract.", SystemRule: &rules.SystemRule{DefaultRule: "Verify untrusted input validation."}})
			err := a.executeGroupReviewFilter(context.Background(), FileGroup{Diffs: []model.Diff{{NewPath: "check.go", Diff: "+validate(input)\n+// Literal: {{review_context}}"}}}, nil)
			if errors.Is(err, errReviewFilterIncomplete) != tc.incomplete {
				t.Fatalf("filter error = %v, incomplete = %v", err, tc.incomplete)
			}
			if !tc.incomplete && err != nil {
				t.Fatal(err)
			}
			if calls != 1 || (len(collector.CommentsForPath("check.go")) == 0) != tc.remove {
				t.Fatalf("calls = %d, comments = %v", calls, collector.CommentsForPath("check.go"))
			}
		})
	}
}

func TestReviewFilterContextBudgetRetainsFindings(t *testing.T) {
	for _, aggregate := range []bool{false, true} {
		t.Run(map[bool]string{false: "prompt", true: "aggregate"}[aggregate], func(t *testing.T) {
			tpl, err := template.LoadDefault()
			if err != nil {
				t.Fatal(err)
			}
			collector := tool.NewCommentCollector()
			collector.Add(model.LlmComment{Path: "a.go", Content: "retain this finding"})
			client := &filterRequestCaptureClient{}
			args := Args{Session: session.New(t.TempDir(), "main", "fake", session.SessionOptions{}), Template: *tpl, LLMClient: client, CommentCollector: collector}
			if aggregate {
				args.MaxTokensBudget = 1
			} else {
				args.Template.MaxTokens = 1
			}
			a := New(args)
			err = a.executeGroupReviewFilter(context.Background(), FileGroup{Diffs: []model.Diff{{NewPath: "a.go", Diff: "+x"}}}, nil)
			if !errors.Is(err, errReviewFilterIncomplete) || client.calls != 0 || len(collector.CommentsForPath("a.go")) != 1 {
				t.Fatalf("error = %v, calls = %d, comments = %v", err, client.calls, collector.CommentsForPath("a.go"))
			}
		})
	}
}

func TestReviewFilterMissingContextFailsManifest(t *testing.T) {
	var a *Agent
	calls := 0
	client := filterContextClient(func(_ llm.ChatRequest) (*llm.ChatResponse, error) {
		calls++
		if calls == 1 {
			a.args.CommentCollector.Add(model.LlmComment{Path: "a.go", Content: "Requires unavailable caller context"})
			return agentTaskDoneResponse(), nil
		}
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{
			{Type: "function", Function: llm.FunctionCall{Name: "report_incorrect_comments", Arguments: `{"comment_ids":["c-0"]}`}},
			{Type: "function", Function: llm.FunctionCall{Name: "report_incomplete_review", Arguments: `{"reason":"caller contract unavailable"}`}},
		}}}}}, nil
	})
	a = newManifestFlowAgentWithClient(t, []model.Diff{{NewPath: "a.go", Diff: "+x", Insertions: 1}}, nil, client)
	a.args.Template.ReviewFilterTask = &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "{{comments}} {{review_context}}"}}}
	_, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m := finishManifestFlow(t, a)
	if calls != 2 || m.TerminalState != session.StateFailed || len(m.Coverage.Failed) != 1 || len(m.Coverage.Completed) != 0 || len(a.args.CommentCollector.CommentsForPath("a.go")) != 1 {
		t.Fatalf("calls = %d, manifest = %+v, comments = %v", calls, m, a.args.CommentCollector.CommentsForPath("a.go"))
	}
}

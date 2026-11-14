// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/tool"
)

type filterRetryClient struct {
	responses []llm.ResponseMessage
	requests  []llm.ChatRequest
	metas     []llm.RequestMeta
	afterCall func()
	err       error
}

func (c *filterRetryClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	meta, _ := llm.RequestMetaFromContext(ctx)
	c.metas = append(c.metas, meta)
	c.requests = append(c.requests, req)
	if c.afterCall != nil {
		c.afterCall()
	}
	if c.err != nil {
		return nil, c.err
	}
	msg := c.responses[len(c.requests)-1]
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: msg}},
		Usage:   &llm.UsageInfo{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10},
	}, nil
}

func TestReviewFilterMalformedOutputRecovery(t *testing.T) {
	text := func(s string) llm.ResponseMessage { return llm.ResponseMessage{Content: &s} }
	call := func(name, args string) llm.ToolCall {
		return llm.ToolCall{Type: "function", Function: llm.FunctionCall{Name: name, Arguments: args}}
	}
	tools := func(calls ...llm.ToolCall) llm.ResponseMessage {
		return llm.ResponseMessage{ToolCalls: calls}
	}
	remove := tools(call("report_incorrect_comments", `{"comment_ids":["c-0"]}`))
	for _, tt := range []struct {
		name      string
		responses []llm.ResponseMessage
		removed   bool
		failed    bool
		budget    int64
		cancel    bool
		err       error
	}{
		{name: "empty then valid tool", responses: []llm.ResponseMessage{text(""), remove}, removed: true},
		{name: "truncated then fenced JSON", responses: []llm.ResponseMessage{text(`["c-`), text("```json\n[\"c-0\"]\n```")}, removed: true},
		{name: "malformed tool then approval", responses: []llm.ResponseMessage{tools(call("report_incorrect_comments", "{")), tools(call("approve_all_comments", "{}"))}},
		{name: "missing ids then approval", responses: []llm.ResponseMessage{tools(call("report_incorrect_comments", "{}")), text("[]")}},
		{name: "null is not approval", responses: []llm.ResponseMessage{text("null"), text("[]")}},
		{name: "reasoning is not a decision", responses: []llm.ResponseMessage{{ReasoningContent: `["c-0"]`}, text("[]")}},
		{name: "mixed valid and malformed calls", responses: []llm.ResponseMessage{tools(call("report_incorrect_comments", `{"comment_ids":["c-0"]}`), call("report_incorrect_comments", "{")), text("[]")}},
		{name: "malformed approval", responses: []llm.ResponseMessage{tools(call("approve_all_comments", "{")), text("[]")}},
		{name: "permanent malformed", responses: []llm.ResponseMessage{text(""), text("not JSON")}, failed: true},
		{name: "valid empty array", responses: []llm.ResponseMessage{text("[]")}},
		{name: "valid tool approval", responses: []llm.ResponseMessage{tools(call("approve_all_comments", "{}"))}},
		{name: "approval without arguments", responses: []llm.ResponseMessage{tools(call("approve_all_comments", ""))}},
		{name: "approval with null arguments", responses: []llm.ResponseMessage{tools(call("approve_all_comments", "null"))}},
		{name: "budget exhausted", responses: []llm.ResponseMessage{text("")}, budget: 10, failed: true},
		{name: "canceled before retry", responses: []llm.ResponseMessage{text("")}, cancel: true, failed: true},
		{name: "transport error is not retried", responses: []llm.ResponseMessage{text("")}, err: errors.New("unavailable"), failed: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var log bytes.Buffer
			defer stdout.Swap(&log)()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := &filterRetryClient{responses: tt.responses, err: tt.err}
			if tt.cancel {
				client.afterCall = cancel
			}
			sess := session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: "diff"})
			collector := tool.NewCommentCollector()
			collector.Add(model.LlmComment{Path: "a.go", Content: "previous round"})
			collector.Add(model.LlmComment{Path: "a.go", Content: "new candidate"})
			collector.Add(model.LlmComment{Path: "b.go", Content: "keep second file"})
			a := New(Args{
				LLMClient: client, Model: "test", Provider: "test-provider", Session: sess,
				CommentCollector: collector, MaxTokensBudget: tt.budget,
				Template: template.Template{
					ReviewFilterTask: &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "Filter {{comments}} for {{path}}: {{diff}}"}}},
					MaxTokens:        10000, MaxToolRequestTimes: 5,
					MainTask: template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "review"}}},
				},
			})
			group := FileGroup{Diffs: []model.Diff{{NewPath: "a.go", Diff: "+a"}, {NewPath: "b.go", Diff: "+b"}}}
			a.executeGroupReviewFilter(ctx, group, map[string]int{"a.go": 1})
			if len(client.requests) != len(tt.responses) {
				t.Fatalf("requests = %d, want %d", len(client.requests), len(tt.responses))
			}
			want := []string{"previous round", "new candidate"}
			if tt.removed {
				want = want[:1]
			}
			var got []string
			for _, cm := range collector.CommentsForPath("a.go") {
				got = append(got, cm.Content)
			}
			if !reflect.DeepEqual(got, want) || len(collector.CommentsForPath("b.go")) != 1 {
				t.Fatalf("comments after filtering = %v; want %v, with second file preserved", got, want)
			}
			failed := strings.Contains(log.String(), "Review filter failed") || strings.Contains(log.String(), "Review filter: failed")
			if failed != tt.failed {
				t.Errorf("failure marker = %v, want %v; log: %s", failed, tt.failed, log.String())
			}
			key := fileGroupKey(group.Diffs)
			records := sess.GetOrCreateFileSession(key).TaskRecords[session.ReviewFilterTask]
			if len(records) != len(client.requests) {
				t.Fatalf("records = %d, requests = %d", len(records), len(client.requests))
			}
			for i, req := range client.requests {
				meta := client.metas[i]
				if meta.RequestNo != records[i].RequestNo || meta.RequestNo != i+1 || meta.FilePath != key || meta.TaskType != string(session.ReviewFilterTask) {
					t.Errorf("request %d has wrong identity: %+v", i, meta)
				}
				if req.ToolChoice != "" || len(req.Tools) != len(filterTools) {
					t.Errorf("request %d changed tool availability or forced tool choice", i)
				}
				if i > 0 && (len(req.Messages) != len(client.requests[0].Messages)+1 || req.Messages[len(req.Messages)-1].Role != "user") {
					t.Error("retry must append a correction to the original prompt")
				}
			}
			wantTokens := int64(10 * len(client.requests))
			if tt.err != nil {
				wantTokens = 0
			}
			if a.runner.TotalTokensUsed() != wantTokens {
				t.Errorf("usage = %d, want %d", a.runner.TotalTokensUsed(), wantTokens)
			}
			if tt.budget > 0 {
				if !a.BudgetExceeded() {
					t.Error("budget stop was not recorded")
				}
				var warnings int
				for _, warning := range a.Warnings() {
					if warning.Type == "token_budget_reached" {
						warnings++
					}
				}
				if warnings != 1 {
					t.Errorf("budget warnings = %d, want 1", warnings)
				}
			}
			if !tt.failed && sess.LLMFailures() != 0 {
				t.Error("successful recovery recorded a permanent LLM failure")
			}
		})
	}
}

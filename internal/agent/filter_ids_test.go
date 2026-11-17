// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestReviewFilterIgnoresUnissuedCommentIDs(t *testing.T) {
	for _, id := range []string{
		"c-1-extra", "c-1.5", "c-1 c-0", "c-1 ", "c-1\n", " c-1",
		"c-01", "c-+1", "c--0", "c- 1", "c-", "c--1", "x-1", "c-12",
		"c-999999999999999999999999999999999999999",
	} {
		t.Run(id, func(t *testing.T) {
			ids := []string{"c-2", id, "c-10", "c-2"}
			text, err := json.Marshal(ids)
			if err != nil {
				t.Fatal(err)
			}
			args, err := json.Marshal(map[string][]string{"comment_ids": ids})
			if err != nil {
				t.Fatal(err)
			}
			want := map[int]struct{}{2: {}, 10: {}}
			results := map[string]map[int]struct{}{
				"text": parseFilterResponse(string(text), 12),
				"tool": parseFilterToolCalls([]llm.ToolCall{{Function: llm.FunctionCall{
					Name: "report_incorrect_comments", Arguments: string(args),
				}}}, 12),
			}
			for name, got := range results {
				if !reflect.DeepEqual(got, want) {
					t.Errorf("%s decision = %v, want %v; %q was never issued", name, got, want, id)
				}
			}
		})
	}
}

func TestReviewFilterMalformedIDDoesNotRemoveAnotherFinding(t *testing.T) {
	for _, transport := range []string{"text", "tool"} {
		t.Run(transport, func(t *testing.T) {
			content := `["c-0","c-1-extra"]`
			message := llm.ResponseMessage{Content: &content}
			if transport == "tool" {
				message = llm.ResponseMessage{ToolCalls: []llm.ToolCall{{Function: llm.FunctionCall{
					Name: "report_incorrect_comments", Arguments: `{"comment_ids":["c-0","c-1-extra"]}`,
				}}}}
			}
			client := &filterRequestCaptureClient{response: &llm.ChatResponse{
				Choices: []llm.Choice{{Message: message}},
			}}
			collector := tool.NewCommentCollector()
			previous := model.LlmComment{Path: "a.go", Content: "Already reviewed in an earlier round"}
			keep := model.LlmComment{Path: "b.go", Content: "This finding's exact ID is c-1"}
			collector.Add(previous)
			collector.Add(model.LlmComment{Path: "a.go", Content: "Remove this exact c-0 finding"})
			collector.Add(keep)
			a := New(Args{
				LLMClient: client, Model: "test", CommentCollector: collector,
				Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: "diff"}),
				Template: template.Template{
					ReviewFilterTask: &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "Filter: {{comments}}"}}},
					MaxTokens:        10000,
				},
			})
			a.executeGroupReviewFilter(context.Background(), FileGroup{
				Label: "both files", Diffs: []model.Diff{{NewPath: "a.go"}, {NewPath: "b.go"}},
			}, map[string]int{"a.go": 1})
			if client.calls != 1 {
				t.Fatalf("filter requests = %d, want 1", client.calls)
			}
			if got, want := collector.Comments(), []model.LlmComment{previous, keep}; !reflect.DeepEqual(got, want) {
				t.Fatalf("remaining findings = %+v, want %+v", got, want)
			}
		})
	}
}

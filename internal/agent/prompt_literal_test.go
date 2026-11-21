// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestPromptRendering_LiteralMainValues(t *testing.T) {
	const rule = "rule: {{diffs}}"
	const files = "file: {{requirement_background}}"
	const patch = "+const value = \"{{plan_guidance}} {{confirmed_comments}}\";"
	const background = "check {{plan_guidance}}"
	const plan = "plan mentions {{confirmed_comments}}"
	const confirmed = "finding with {{system_rule}}"
	a := &Agent{currentDate: "2026-11-17", args: Args{
		Background: background,
		Template: template.Template{MainTask: template.LlmConversation{Messages: []template.ChatMessage{
			{Role: "user", Content: "{{current_system_date_time}}|{{system_rule}}|{{change_files}}|{{diffs}}|{{requirement_background}}|{{plan_guidance}}|{{confirmed_comments}}|{{unknown}}"},
		}}},
	}}
	got := a.buildMainTaskMessages(rule, files, patch, plan, confirmed)
	want := "2026-11-17|" + rule + "|" + files + "|" + patch + "|" + background + "|" + plan + "|" + confirmed + "|{{unknown}}"
	if len(got) != 1 || got[0].Role != "user" || got[0].ExtractText() != want {
		t.Fatalf("messages = %+v, want literal values %q", got, want)
	}
}

func TestPromptRendering_OnlyStripEmptyTemplateSections(t *testing.T) {
	const sections = "### Review Plan\n{{plan_guidance}}\n\n### Previously Confirmed Findings\n{{confirmed_comments}}\n\n"
	a := &Agent{args: Args{Template: template.Template{MainTask: template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "rules:\n{{system_rule}}\n" + sections + "diff:\n{{diffs}}"}},
	}}}}
	const patch = "+const template = \"{{plan_guidance}} {{confirmed_comments}}\";"
	got := a.buildMainTaskMessages(sections, "", patch, "", "")
	want := "rules:\n" + sections + "\ndiff:\n" + patch
	if len(got) != 1 || got[0].ExtractText() != want {
		t.Fatalf("messages = %+v, want only template sections removed: %q", got, want)
	}
}

func TestPromptRendering_LiteralPlanValues(t *testing.T) {
	client := &filterRequestCaptureClient{}
	a := New(Args{
		LLMClient:  client,
		Session:    session.New(t.TempDir(), "main", "test", session.SessionOptions{}),
		Background: "background {{plan_tools}}",
		Template: template.Template{PlanTask: &template.LlmConversation{Messages: []template.ChatMessage{
			{Role: "user", Content: "{{system_rule}}|{{diffs}}|{{requirement_background}}|{{plan_tools}}"},
		}}},
	})
	const rule = "rule {{diffs}}"
	const patch = "+const template = \"{{requirement_background}} {{plan_tools}}\";"
	group := FileGroup{Diffs: []model.Diff{{NewPath: "main.go", Diff: patch}}}
	if _, err := a.executeGroupPlanPhase(context.Background(), group, patch, "", rule); err != nil {
		t.Fatal(err)
	}
	want := rule + "|" + patch + "|" + a.args.Background + "|" + formatToolDefs(a.args.PlanToolDefs)
	if client.calls != 1 || len(client.request.Messages) != 1 || client.request.Messages[0].ExtractText() != want {
		t.Fatalf("plan request = %+v, want literal values %q", client.request.Messages, want)
	}
}

func TestPromptRendering_LiteralFilterDiff(t *testing.T) {
	client := &filterRequestCaptureClient{}
	collector := tool.NewCommentCollector()
	comment := model.LlmComment{Path: "main.go", Content: "keep {{diff}} literal"}
	collector.Add(comment)
	a := New(Args{
		LLMClient: client, CommentCollector: collector,
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{}),
		Template: template.Template{ReviewFilterTask: &template.LlmConversation{Messages: []template.ChatMessage{
			{Role: "user", Content: "{{path}}|{{diff}}|{{comments}}"},
		}}},
	})
	group := FileGroup{Diffs: []model.Diff{{NewPath: "main.go", Diff: "+const template = \"{{comments}}\";"}}}
	a.executeGroupReviewFilter(context.Background(), group, nil)
	want := "main.go|" + buildConcatenatedDiffs(group.Diffs) + "|" + buildGroupFilterCommentsJSON([]model.LlmComment{comment})
	if client.calls != 1 || len(client.request.Messages) != 1 || client.request.Messages[0].ExtractText() != want {
		t.Fatalf("filter request = %+v, want literal diff %q", client.request.Messages, want)
	}
}

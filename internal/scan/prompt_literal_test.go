// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestPromptRendering_LiteralMainValues(t *testing.T) {
	const rule = "rule {{file_content}}"
	const plan = "plan {{system_rule}}"
	item := model.ScanItem{Path: "{{system_rule}}.go", Content: "const value = \"{{requirement_background}}\";"}
	a := &Agent{currentDate: "2026-11-17", args: Args{
		Background: "background {{file_content}}",
		Template: template.ScanTemplate{MainTask: template.LlmConversation{Messages: []template.ChatMessage{
			{Role: "user", Content: "{{plan_guidance}}|{{current_system_date_time}}|{{current_file_path}}|{{system_rule}}|{{change_files}}|{{file_content}}|{{requirement_background}}|{{unknown}}"},
		}}},
	}}
	got := a.renderMessages(item, rule, plan)
	want := plan + "|2026-11-17|" + item.Path + "|" + rule + "|" + changeFilesScanLiteral + "|" + item.Content + "|" + a.args.Background + "|{{unknown}}"
	if len(got) != 1 || got[0].Role != "user" || got[0].ExtractText() != want {
		t.Fatalf("messages = %+v, want literal values %q", got, want)
	}
}

func TestPromptRendering_LiteralPlanValues(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	tpl.PlanTask = &template.LlmConversation{Messages: []template.ChatMessage{
		{Role: "user", Content: "{{current_system_date_time}}|{{current_file_path}}|{{system_rule}}|{{file_content}}"},
	}}
	a := newAgentForTest(t, tpl)
	a.args.LLMClient = &fakeScanClient{}
	a.currentDate = "2026-11-17"
	item := model.ScanItem{Path: "{{system_rule}}.go", Content: "const value = \"{{current_file_path}}\";"}
	const rule = "rule {{file_content}}"
	a.maybeRunPlan(t.Context(), item, rule)
	records := a.Session().GetOrCreateFileSession(item.Path).TaskRecords[session.PlanTask]
	if len(records) != 1 || len(records[0].RequestMessages) != 1 {
		t.Fatalf("expected one plan request, got %+v", records)
	}
	want := "2026-11-17|" + item.Path + "|" + rule + "|" + item.Content
	if got := records[0].RequestMessages[0].ExtractText(); got != want {
		t.Fatalf("plan request = %q, want literal values %q", got, want)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

// The plan is a single shot: there is no next round that could recover a reply
// cut at the output cap, and the failure surfaces as whatever the truncated
// plan breaks downstream. Recording it is the only way the reader learns the
// cap was the cause. The silent half is pinned too, since a warning on every
// plan would be noise.
func TestExecuteGroupPlanPhase_ReportsATruncatedPlan(t *testing.T) {
	cases := []struct {
		name         string
		finishReason string
		wantWarnings int
	}{
		{"cut at the cap", "length", 1},
		{"finished normally", "stop", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess := session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: "diff"})
			planText := "review plan output"
			client := &fakeAgentClient{responses: []*llm.ChatResponse{{
				Choices: []llm.Choice{{
					Message:      llm.ResponseMessage{Content: &planText},
					FinishReason: tc.finishReason,
				}},
				Usage: &llm.UsageInfo{PromptTokens: 20, CompletionTokens: 512},
			}}}
			a := New(Args{
				LLMClient: client,
				Model:     "test",
				Session:   sess,
				Template: template.Template{
					PlanTask: &template.LlmConversation{
						Messages: []template.ChatMessage{{Role: "user", Content: "Plan: {{diffs}}"}},
					},
					MaxTokens:           10000,
					MaxCompletionTokens: 512,
					MaxToolRequestTimes: 5,
					MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
				},
			})
			a.currentDate = "2025-06-26 10:00"

			g := FileGroup{Label: "core", Diffs: []model.Diff{{NewPath: "main.go", Diff: "+new code"}}}
			if _, err := a.executeGroupPlanPhase(context.Background(), g,
				buildConcatenatedDiffs(g.Diffs), "main.go", "rule"); err != nil {
				t.Fatalf("executeGroupPlanPhase: %v", err)
			}

			warnings := a.runner.Warnings()
			if len(warnings) != tc.wantWarnings {
				t.Fatalf("warnings = %d, want %d", len(warnings), tc.wantWarnings)
			}
			if tc.wantWarnings == 0 {
				return
			}
			w := warnings[0]
			if w.Type != "response_truncated" {
				t.Errorf("warning type = %q, want response_truncated", w.Type)
			}
			// Filed under the group key, the same string the plan record and the
			// retry report use, so the warning joins its group without a lookup.
			if w.File != "main.go" {
				t.Errorf("warning file = %q, want the group key main.go", w.File)
			}
			if !strings.Contains(w.Message, "512 tokens") {
				t.Errorf("warning message %q must name the 512-token cap", w.Message)
			}
		})
	}
}

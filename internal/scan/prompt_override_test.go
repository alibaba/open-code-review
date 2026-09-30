// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestMaybeRunPlan_CustomFallbackOnEverySkipOrFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	const fallback = "No plan. Review only the selected function using supplied evidence."
	for _, name := range []string{"no plan task", "no-plan flag", "request failure", "empty response", "empty checklist"} {
		t.Run(name, func(t *testing.T) {
			tpl := makeTemplateWithFullScan()
			tpl.NoPlanGuidance = fallback
			tpl.PlanTask = &template.LlmConversation{Messages: []template.ChatMessage{
				{Role: "user", Content: "{{file_content}}"},
			}}
			client := &fakeScanClient{}
			a := newAgentForTest(t, tpl)
			t.Cleanup(func() {
				if err := a.Session().Finalize(); err != nil {
					t.Error(err)
				}
			})
			a.args.LLMClient = client
			switch name {
			case "no plan task":
				a.args.Template.PlanTask = nil
			case "no-plan flag":
				a.args.SkipPlan = true
			case "request failure":
				a.args.LLMClient = &errorScanClient{err: fmt.Errorf("test failure")}
			case "empty checklist":
				text := "{\"summary\":\"\",\"checkpoints\":[]}"
				client.responses = []*llm.ChatResponse{{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &text}}}}}
			}
			item := model.ScanItem{Path: "handler.go", Content: "package handler\n"}
			got := a.maybeRunPlan(t.Context(), item, "rules")
			if got != fallback {
				t.Fatalf("fallback = %q, want %q", got, fallback)
			}
			messages := a.renderMessages(item, "rules", got)
			if !strings.Contains(messages[1].ExtractText(), fallback) {
				t.Fatal("custom fallback did not reach MAIN_TASK")
			}
			if (name == "no plan task" || name == "no-plan flag") && client.calls != 0 {
				t.Fatal("disabled planning called the LLM")
			}
		})
	}
}

func TestScanPromptOverride_ResumeUsesMatchingContract(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	repo := initTestRepo(t)
	writeFile(t, repo, "handler.go", []byte("package handler\nfunc Handle() {}\nfunc Parse() {}\n"))
	gitCommit(t, repo, "init")
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "scan", "bounded-template.json"))
	if err != nil {
		t.Fatal(err)
	}
	load := func(data []byte) template.ScanTemplate {
		t.Helper()
		path := filepath.Join(t.TempDir(), "prompts.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		tpl, err := template.LoadScan(path)
		if err != nil {
			t.Fatal(err)
		}
		return *tpl
	}
	const background = "Review only Handle"
	const rule = "Review only the selected function using supplied evidence."
	newScan := func(tpl template.ScanTemplate, client *fakeScanClient, resume *session.ResumeState, language string) *Agent {
		tpl.ApplyLanguage(language)
		return NewAgent(Args{
			RepoDir: repo, Template: tpl, LLMClient: client, Resume: resume,
			Background: background, SystemRule: &rules.SystemRule{DefaultRule: rule},
			MaxConcurrency: 1, SkipSummary: true, SkipDedup: true,
			MainToolDefs: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "task_done"}}},
		})
	}
	first := newScan(load(data), promptDoneClient(), nil, "English")
	if _, err := first.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	resume, err := session.LoadResumeState(repo, first.SessionID())
	if err != nil || resume.CompletedCount() != 1 {
		t.Fatalf("load checkpoint: %v", err)
	}
	defaults, err := template.LoadScanDefault()
	if err != nil {
		t.Fatal(err)
	}
	var fields any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	pretty, err := json.MarshalIndent(fields, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name                       string
		tpl                        template.ScanTemplate
		background, rule, language string
		wantCalls                  int
	}{
		{name: "same prompts at another path", tpl: load(data)},
		{name: "same prompts with reformatted JSON", tpl: load(pretty)},
		{name: "changed main", tpl: load([]byte(strings.Replace(string(data), "Review only the function", "Audit only the function", 1))), wantCalls: 1},
		{name: "changed fallback", tpl: load([]byte(strings.Replace(string(data), "No pre-scan plan.", "Planning disabled.", 1))), wantCalls: 1},
		{name: "changed background", tpl: load(data), background: "Review only Parse", wantCalls: 1},
		{name: "changed resolved rule", tpl: load(data), rule: "Check error handling in the selected function.", wantCalls: 1},
		{name: "changed configured language", tpl: load(data), language: "Chinese", wantCalls: 1},
		{name: "default contract", tpl: *defaults, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := promptDoneClient()
			language := tt.language
			if language == "" {
				language = "English"
			}
			a := newScan(tt.tpl, client, resume, language)
			if tt.background != "" {
				a.args.Background = tt.background
			}
			if tt.rule != "" {
				a.args.SystemRule = &rules.SystemRule{DefaultRule: tt.rule}
			}
			a.args.SkipPlan = true
			if _, err := a.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			if client.calls != tt.wantCalls {
				t.Fatalf("LLM calls = %d, want %d", client.calls, tt.wantCalls)
			}
			info := a.ResumeInfo()
			if info.RerunFiles != int64(tt.wantCalls) || info.ReusedFiles != int64(1-tt.wantCalls) {
				t.Fatalf("wrong checkpoint reuse: %+v", info)
			}
		})
	}
}

func promptDoneClient() *fakeScanClient {
	return &fakeScanClient{responses: []*llm.ChatResponse{{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{{
			ID: "done", Type: "function", Function: llm.FunctionCall{Name: "task_done", Arguments: "{}"},
		}}}}},
	}}}
}

func TestScanPromptOverride_FingerprintStableInputs(t *testing.T) {
	tpl, err := template.LoadScan(filepath.Join("..", "..", "examples", "scan", "bounded-template.json"))
	if err != nil {
		t.Fatal(err)
	}
	tpl.MainTask.Messages[1].Content += "\nDate: {{current_system_date_time}}"
	resolver := &rules.SystemRule{DefaultRule: "STABLE_RULE", PathRules: []rules.PathRule{
		{Pattern: "changed.go", Rule: "ORIGINAL_RULE"},
	}}
	a := &Agent{args: Args{Template: *tpl, Background: "Review only Handle", SystemRule: resolver}}
	changed := model.ScanItem{Path: "changed.go", Content: "package handler\n"}
	stable := model.ScanItem{Path: "stable.go", Content: "package handler\n"}
	changedFingerprint := a.scanItemFingerprint(changed)
	stableFingerprint := a.scanItemFingerprint(stable)
	a.currentDate = "2026-01-01 00:00"
	before := a.renderMessages(changed, resolver.Resolve(changed.Path), tpl.PlanFallbackGuidance())
	a.currentDate = "2026-12-31 23:59"
	after := a.renderMessages(changed, resolver.Resolve(changed.Path), tpl.PlanFallbackGuidance())
	if before[1].ExtractText() == after[1].ExtractText() || a.scanItemFingerprint(changed) != changedFingerprint {
		t.Fatal("date must reach the request without invalidating a checkpoint")
	}
	resolver.PathRules[0].Rule = "CHANGED_RULE"
	if a.scanItemFingerprint(changed) == changedFingerprint || a.scanItemFingerprint(stable) != stableFingerprint {
		t.Fatal("a changed per-file rule must invalidate only its matching file")
	}
	defaults, err := template.LoadScanDefault()
	if err != nil {
		t.Fatal(err)
	}
	a.args.Template.PlanTask = defaults.PlanTask
	planningFingerprint := a.scanItemFingerprint(changed)
	a.args.SkipPlan = true
	if a.scanItemFingerprint(changed) == planningFingerprint {
		t.Fatal("changing planning enablement must invalidate the custom contract")
	}
}

type promptCaptureClient struct {
	*fakeScanClient
	request llm.ChatRequest
}

func (c *promptCaptureClient) CompletionsWithCtx(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	c.request = request
	return c.fakeScanClient.CompletionsWithCtx(ctx, request)
}

func TestScanPromptOverride_RequestUsesFrozenRule(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	for _, original := range []string{"ORIGINAL_RULE", ""} {
		t.Run(fmt.Sprintf("rule=%q", original), func(t *testing.T) {
			tpl, err := template.LoadScan(filepath.Join("..", "..", "examples", "scan", "bounded-template.json"))
			if err != nil {
				t.Fatal(err)
			}
			resolver := &rules.SystemRule{DefaultRule: original}
			client := &promptCaptureClient{fakeScanClient: promptDoneClient()}
			a := NewAgent(Args{
				RepoDir: t.TempDir(), Template: *tpl, SystemRule: resolver, LLMClient: client,
				MainToolDefs: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "task_done"}}},
			})
			t.Cleanup(func() {
				if err := a.Session().Finalize(); err != nil {
					t.Error(err)
				}
			})
			item := model.ScanItem{Path: "handler.go", Content: "package handler\n"}
			a.initScanFingerprints([]model.ScanItem{item})
			resolver.DefaultRule = "LATER_RULE"
			if completed, reason, err := a.executeSubtask(t.Context(), item); err != nil || !completed {
				t.Fatalf("executeSubtask: completed=%v, reason=%q, err=%v", completed, reason, err)
			}
			content := client.request.Messages[1].ExtractText()
			if !strings.Contains(content, "Review rules:\n"+original+"\n") || strings.Contains(content, "LATER_RULE") {
				t.Fatal("the actual request must use the rule frozen with its checkpoint fingerprint")
			}
		})
	}
}

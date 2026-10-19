// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/plugin"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type pluginReviewClient struct {
	calls    int
	retry    bool
	failAt   int
	requests []llm.ChatRequest
}

type pluginStageFilterClient struct {
	pluginReviewClient
	filters []llm.ChatRequest
}

func (c *pluginStageFilterClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	for _, def := range req.Tools {
		if def.Function.Name == "approve_all_comments" {
			c.filters = append(c.filters, req)
			res := agentTaskDoneResponse()
			call := &res.Choices[0].Message.ToolCalls[0].Function
			call.Name, call.Arguments = "approve_all_comments", `{}`
			if len(c.filters) == 2 {
				call.Name, call.Arguments = "report_incorrect_comments", `{"comment_ids":["c-0"]}`
			}
			return res, nil
		}
	}
	return c.pluginReviewClient.CompletionsWithCtx(ctx, req)
}

func newPluginStageAgent(t *testing.T, client llm.LLMClient, mode string, budget int64) *Agent {
	t.Helper()
	a := newManifestFlowAgentWithClient(t, []model.Diff{{NewPath: "good.go", Diff: "+value", Insertions: 1}}, nil, client)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	a.args.ReviewPlugin = &plugin.Installed{Manifest: plugin.Manifest{SchemaVersion: 1, ID: "custom", Name: "Custom", Version: "1", Review: &plugin.ReviewContribution{Protocol: "stdio-v1", Command: executable, Args: []string{"-test.run=^TestAgentReviewProcess$", "--", "agent-review-helper", mode}, Stages: []string{"inspect", "verify"}}}}
	a.args.MaxTokensBudget = budget
	a.args.Tools.Register(&tool.CodeCommentProvider{Collector: a.args.CommentCollector})
	a.runner = llmloop.NewRunner(llmloop.Deps{LLMClient: client, Model: a.args.Model, Template: a.args.Template, Tools: a.args.Tools, MainToolDefs: a.args.MainToolDefs, CommentCollector: a.args.CommentCollector, Session: a.session, DiffLookup: a.findDiff, AllDiffs: a.allDiffs, NewRequestMeta: a.newRequestMeta, MaxTokensBudget: budget})
	return a
}

func TestPluginReviewFiltersOnlyCurrentStage(t *testing.T) {
	client := &pluginStageFilterClient{}
	a := newPluginStageAgent(t, client, "stage-findings", 0)
	a.args.Template.ReviewFilterTask = &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "Filter {{comments}}"}}}
	a.args.CommentCollector.Add(model.LlmComment{Path: "good.go", Content: "Previously accepted finding"})
	completed, stop, err := a.executePluginReview(context.Background(), FileGroup{Diffs: a.diffs})
	if err != nil || !completed || stop != nil || len(client.filters) != 2 {
		t.Fatalf("completed=%v stop=%+v error=%v filters=%d", completed, stop, err, len(client.filters))
	}
	for i, stage := range []string{"inspect", "verify"} {
		var candidates []struct{ Content string }
		prompt := client.filters[i].Messages[0].Content.(string)
		if err := json.Unmarshal([]byte(strings.TrimPrefix(prompt, "Filter ")), &candidates); err != nil {
			t.Fatal(err)
		}
		if len(candidates) != 1 || candidates[0].Content != "Finding in "+stage {
			t.Fatalf("stage %s re-filtered earlier findings: %+v", stage, candidates)
		}
	}
	comments := a.args.CommentCollector.Comments()
	if len(comments) != 2 || comments[0].Content != "Previously accepted finding" || comments[1].Content != "Finding in inspect" {
		t.Fatalf("later filter changed earlier accepted findings: %+v", comments)
	}
}

type pluginBudgetClient struct {
	pluginReviewClient
	tokens     int64
	incomplete bool
}

func (c *pluginBudgetClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	res, err := c.pluginReviewClient.CompletionsWithCtx(ctx, req)
	if err != nil {
		return res, err
	}
	res.Usage = &llm.UsageInfo{PromptTokens: c.tokens, TotalTokens: c.tokens}
	if c.incomplete {
		res.Choices[0].Message.ToolCalls = nil
	}
	return res, nil
}

func TestPluginReviewRecordsAggregateBudgetWarning(t *testing.T) {
	for _, mode := range []string{"before-stage", "between-stages", "completed-over-budget", "during-stage", "unlimited"} {
		t.Run(mode, func(t *testing.T) {
			budget := int64(1000)
			client := &pluginBudgetClient{tokens: budget}
			if mode == "during-stage" || mode == "completed-over-budget" {
				client.tokens++
				client.incomplete = mode == "during-stage"
			}
			if mode == "unlimited" {
				budget = 0
			}
			a := newPluginStageAgent(t, client, "complete", budget)
			if mode == "before-stage" {
				a.runner.RecordUsage(&llm.UsageInfo{PromptTokens: budget})
			}
			completed, stop, err := a.executePluginReview(context.Background(), FileGroup{Diffs: a.diffs})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "unlimited" {
				if !completed || stop != nil || a.BudgetExceeded() {
					t.Fatalf("unlimited review stopped: completed=%v stop=%+v", completed, stop)
				}
			} else if completed || stop == nil || !stop.required || stop.class != session.FailureBudget || !a.BudgetExceeded() {
				t.Fatalf("budget stop lost: completed=%v stop=%+v budgetExceeded=%v", completed, stop, a.BudgetExceeded())
			}
			var warnings []AgentWarning
			for _, warning := range a.Warnings() {
				if warning.Type == "token_budget_reached" {
					warnings = append(warnings, warning)
				}
			}
			want := 1
			if mode == "unlimited" {
				want = 0
			}
			if len(warnings) != want || (want > 0 && (warnings[0].File != "good.go" || !strings.Contains(warnings[0].Message, "1000"))) {
				t.Fatalf("aggregate budget warnings = %+v", warnings)
			}
		})
	}
}

func TestPluginReviewBudgetWarningSharedWithDispatch(t *testing.T) {
	for _, lookAheadFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("look-ahead-first=%v", lookAheadFirst), func(t *testing.T) {
			diffs := makeBudgetDiffs(3)
			budget := estimateDiffFileTokens(diffs[0]) * 4
			if lookAheadFirst {
				budget = 1
			}
			client := &pluginBudgetClient{tokens: budget + 1}
			a := newPluginStageAgent(t, client, "complete", budget)
			a.diffs = diffs
			a.args.MaxConcurrency = 1
			a.args.ReviewPlugin.Manifest.Review.Stages = []string{"inspect"}
			if _, err := a.dispatchSubtasks(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(a.fileGroups) != 3 {
				t.Fatalf("expected three groups, got %+v", a.fileGroups)
			}
			for _, group := range a.fileGroups {
				if len(group.Diffs) != 1 {
					t.Fatalf("expected single-file group: %+v", group)
				}
			}
			for _, report := range a.PluginReviewReports() {
				if report.Group == "f2.go" {
					t.Fatal("dispatch gate did not stop the final group")
				}
			}
			wantCalls, wantCompleted := 1, 1
			if lookAheadFirst {
				wantCalls, wantCompleted = 0, 0
			}
			if client.calls != wantCalls || !a.BudgetExceeded() {
				t.Fatalf("calls=%d want=%d budgetExceeded=%v", client.calls, wantCalls, a.BudgetExceeded())
			}
			manifest := finishManifestFlow(t, a)
			if len(manifest.Coverage.Completed) != wantCompleted || len(manifest.Coverage.Failed) != 3-wantCompleted || manifest.RunFailure != nil {
				t.Fatalf("budget dispatch changed coverage: %+v", manifest)
			}
			for _, item := range manifest.Coverage.Failed {
				if item.Classification != session.FailureBudget {
					t.Fatalf("undispatched item lost its budget cause: %+v", item)
				}
			}
			var warnings []AgentWarning
			for _, warning := range a.Warnings() {
				if warning.Type == "token_budget_reached" {
					warnings = append(warnings, warning)
				}
			}
			if len(warnings) != 1 || warnings[0].File != "f0.go" || (lookAheadFirst && !strings.HasPrefix(warnings[0].Message, "stopped dispatch:")) {
				t.Fatalf("aggregate budget warning must be recorded once: %+v", warnings)
			}
		})
	}
}

func TestValidatePluginCommentsIdentifiesInvalidFinding(t *testing.T) {
	valid := model.LlmComment{Path: "good.go", Content: "A value is lost.", Severity: "medium", Category: "bug"}
	paths := []string{"good.go"}
	for _, tc := range []struct {
		field string
		edit  func(*model.LlmComment)
	}{
		{"path", func(c *model.LlmComment) { c.Path = "outside.go" }},
		{"content", func(c *model.LlmComment) { c.Content = " \n\t" }},
		{"severity", func(c *model.LlmComment) { c.Severity = "urgent" }},
		{"category", func(c *model.LlmComment) { c.Category = "unknown" }},
	} {
		t.Run(tc.field, func(t *testing.T) {
			invalid := valid
			tc.edit(&invalid)
			err := validatePluginComments([]model.LlmComment{valid, invalid}, paths)
			if err == nil || !strings.Contains(err.Error(), "plugin finding 2:") || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("diagnostic must identify the finding and field: %v", err)
			}
		})
	}
	if err := validatePluginComments([]model.LlmComment{valid}, paths); err != nil {
		t.Fatal(err)
	}
	if err := validatePluginComments(nil, paths); err != nil {
		t.Fatal(err)
	}
}

func TestPluginStartupCancellationClassification(t *testing.T) {
	diffs := []model.Diff{{NewPath: "good.go", Diff: "+value", Insertions: 1}}
	a := newManifestFlowAgentWithClient(t, diffs, nil, &pluginReviewClient{})
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	a.args.ReviewPlugin = &plugin.Installed{Manifest: plugin.Manifest{SchemaVersion: 1, ID: "cancelled", Name: "Cancelled", Version: "1", Review: &plugin.ReviewContribution{Protocol: "stdio-v1", Command: executable, Stages: []string{"inspect"}}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = a.executePluginReview(ctx, FileGroup{Diffs: diffs})
	class, _ := classifyItemError(err)
	if class != session.FailureCancelled {
		t.Fatalf("cancelled plugin classified as %s", class)
	}
}

func (c *pluginReviewClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.calls++
	c.requests = append(c.requests, req)
	if c.calls == c.failAt {
		return nil, errors.New("provider unavailable")
	}
	res := agentTaskDoneResponse()
	res.Choices[0].Message.ToolCalls[0].Function.Arguments = `{"checked":true}`
	if c.retry && c.calls == 1 {
		res.Choices[0].Message.ToolCalls[0].Function.Arguments = `{}`
	}
	return res, nil
}

func TestPluginReviewRecoveryAndFailures(t *testing.T) {
	for _, mode := range []string{"complete", "retry", "provider-after-rejection", "incomplete", "finding-incomplete", "missing-complete", "crash", "invalid-comment", "omit-report", "report-error"} {
		t.Run(mode, func(t *testing.T) {
			client := &pluginReviewClient{retry: mode == "retry" || mode == "provider-after-rejection"}
			if mode == "provider-after-rejection" {
				client.failAt = 2
			}
			a := newManifestFlowAgentWithClient(t, []model.Diff{{NewPath: "good.go", Diff: "+value", Insertions: 1}}, nil, client)
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			a.args.ReviewPlugin = &plugin.Installed{Manifest: plugin.Manifest{SchemaVersion: 1, ID: "custom", Name: "Custom", Version: "1", Review: &plugin.ReviewContribution{Protocol: "stdio-v1", Command: executable, Args: []string{"-test.run=^TestAgentReviewProcess$", "--", "agent-review-helper", mode}, Stages: []string{"inspect", "verify"}}}}
			a.args.Template.MaxToolRequestTimes = 4
			a.args.Tools.Register(&tool.CodeCommentProvider{Collector: a.args.CommentCollector})
			a.runner = llmloop.NewRunner(llmloop.Deps{LLMClient: client, Model: a.args.Model, Template: a.args.Template, Tools: a.args.Tools, MainToolDefs: a.args.MainToolDefs, CommentCollector: a.args.CommentCollector, Session: a.session, DiffLookup: a.findDiff, AllDiffs: a.allDiffs, NewRequestMeta: a.newRequestMeta})
			comments, _ := a.dispatchSubtasks(context.Background())
			manifest := finishManifestFlow(t, a)
			complete := mode == "complete" || mode == "retry" || mode == "omit-report"
			if (manifest.TerminalState == session.StateComplete) != complete {
				t.Fatalf("wrong completion: %+v", manifest)
			}
			reports := a.PluginReviewReports()
			if len(reports) == 0 || len(reports[0].Report) == 0 {
				t.Fatal("plugin state lost")
			}
			saved, err := session.LoadPluginReviewReports(a.args.RepoDir, a.SessionID())
			if err != nil || !reflect.DeepEqual(saved, reports) {
				t.Fatalf("persisted reports = %+v, want %+v, error = %v", saved, reports, err)
			}
			if mode == "report-error" && !strings.Contains(string(reports[0].Report), "updated-before-error") {
				t.Fatalf("failed response report lost: %s", reports[0].Report)
			}
			if complete && (len(reports) != 2 || !reports[1].Complete) {
				t.Fatalf("stage plan not completed: %+v", reports)
			}
			wantCalls := 1
			if mode == "complete" || mode == "provider-after-rejection" || mode == "omit-report" {
				wantCalls = 2
			}
			if mode == "retry" {
				wantCalls = 3
			}
			if client.calls != wantCalls {
				t.Fatalf("calls=%d want %d", client.calls, wantCalls)
			}
			wantComments := 0
			if mode == "finding-incomplete" {
				wantComments = 1
			}
			if len(comments) != wantComments {
				t.Fatalf("comments=%+v", comments)
			}
			if mode == "provider-after-rejection" && (!strings.Contains(reports[0].Error, "checked is required") || manifest.Coverage.Failed[0].Classification != session.FailureProvider) {
				t.Fatalf("last accepted state/feedback lost: %+v %+v", reports, manifest)
			}
			if mode == "crash" || mode == "missing-complete" || mode == "invalid-comment" || mode == "report-error" {
				if manifest.Coverage.Failed[0].Classification != session.FailureUnknown {
					t.Fatalf("plugin fault classified as provider fault: %+v", manifest)
				}
			}
			for _, request := range client.requests {
				for _, def := range request.Tools {
					if def.Function.Name == "code_comment" {
						t.Fatal("model can bypass plugin validator")
					}
				}
			}
		})
	}
}

func TestAgentReviewProcess(t *testing.T) {
	i := slices.Index(os.Args, "agent-review-helper")
	if i < 0 {
		return
	}
	mode := os.Args[i+1]
	lines := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for lines.Scan() {
		var req plugin.ReviewRequest
		if json.Unmarshal(lines.Bytes(), &req) != nil {
			os.Exit(2)
		}
		res := plugin.ReviewResponse{Version: 1, Report: json.RawMessage(`{"saved":"plugin-owned-state"}`)}
		if req.Method == "begin" {
			res.Task = &plugin.ReviewTask{Instructions: "Inspect this change.", Input: "Custom stage: " + req.Stage, Completion: plugin.ReviewCompletion{Description: "Submit checked.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"checked": map[string]any{"type": "boolean"}}}}}
		} else {
			if mode == "crash" {
				os.Exit(3)
			}
			accepted := req.Arguments["checked"] == true
			complete := mode != "incomplete" && mode != "finding-incomplete"
			res.Accepted, res.Complete = &accepted, &complete
			if !accepted {
				res.Complete = nil
				res.Feedback = "checked is required"
			}
			if mode == "missing-complete" {
				res.Complete = nil
			}
			if mode == "omit-report" {
				res.Report = nil
			}
			if mode == "report-error" {
				res.Report = json.RawMessage(`{"saved":"updated-before-error"}`)
				res.Error = "validator unavailable"
			}
			if mode == "finding-incomplete" || mode == "invalid-comment" || mode == "stage-findings" {
				res.Comments = []model.LlmComment{{Path: "good.go", Content: "The value is lost.", ExistingCode: "value", Severity: "medium", Category: "bug"}}
				if mode == "stage-findings" {
					res.Comments[0].Content = "Finding in " + req.Stage
				}
				if mode == "invalid-comment" {
					res.Comments[0].Path = "outside.go"
				}
			}
		}
		if enc.Encode(res) != nil {
			os.Exit(4)
		}
	}
	os.Exit(0)
}

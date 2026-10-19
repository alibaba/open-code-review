// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestRun_PromptOverflowRecordsFailure(t *testing.T) {
	for _, tc := range []struct {
		name          string
		systemWords   int
		content       string
		withSmallFile bool
		reuseSmall    bool
		withFinding   bool
		batchSize     int
	}{
		{name: "all files overflow", systemWords: 100, content: "package main\n"},
		{name: "one file completes", systemWords: 20, content: "package main\n// " + strings.Repeat("test", 70) + "\n", withSmallFile: true},
		{name: "one file reused with finding", systemWords: 100, content: "package main\n", withSmallFile: true, reuseSmall: true, withFinding: true},
		{name: "one file reused without findings", systemWords: 100, content: "package main\n", withSmallFile: true, reuseSmall: true},
		{name: "reused finding in later batch", systemWords: 100, content: "package main\n", withSmallFile: true, reuseSmall: true, withFinding: true, batchSize: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := initTestRepo(t)
			writeFile(t, repo, "overflow.go", []byte(tc.content))
			if tc.withSmallFile {
				writeFile(t, repo, "small.go", []byte("package main\n"))
			}
			gitCommit(t, repo, "init")
			tpl := budgetTestTemplate()
			tpl.MaxTokens = 100
			if tc.batchSize > 0 {
				tpl.BatchStrategy = string(BatchByLanguage)
				tpl.BatchSize = tc.batchSize
			}
			tpl.MainTask.Messages = []template.ChatMessage{
				{Role: "system", Content: strings.Repeat("review ", tc.systemWords)},
				{Role: "user", Content: "review {{file_content}}"},
			}
			client := &fakeBudgetClient{}
			var resume *session.ResumeState
			if tc.reuseSmall {
				item := model.ScanItem{Path: "small.go", Content: "package main\n", LineCount: 1}
				fingerprint := scanItemFingerprint(item)
				var comments []model.LlmComment
				if tc.withFinding {
					comments = []model.LlmComment{{Path: item.Path, Content: "saved finding", StartLine: 1, EndLine: 1}}
				}
				resume = &session.ResumeState{
					SessionID: "previous-scan",
					Items: map[string]session.ResumeItem{fingerprint: {
						FilePath: item.Path, OldPath: item.Path, NewPath: item.Path,
						Fingerprint: fingerprint, Comments: comments,
					}},
				}
			}
			a := NewAgent(Args{
				RepoDir:        repo,
				Template:       tpl,
				LLMClient:      client,
				MaxConcurrency: 1,
				SkipPlan:       true,
				SkipDedup:      true,
				SkipSummary:    true,
				Resume:         resume,
				Session:        session.New(repo, "main", "test", session.SessionOptions{ReviewMode: session.ReviewModeFullScan}),
			})
			comments, err := a.Run(context.Background())
			wantCalls := int64(0)
			wantItems := 0
			if tc.withSmallFile {
				wantItems = 1
				if !tc.reuseSmall {
					wantCalls = 1
				}
				if err != nil {
					t.Fatalf("mixed scan: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "all 1 file scan(s) failed") {
				t.Fatalf("expected all-files-failed error, got %v", err)
			}
			if calls := atomic.LoadInt64(&client.calls); calls != wantCalls {
				t.Fatalf("MAIN_TASK calls = %d, want %d", calls, wantCalls)
			}
			if tc.withFinding && (len(comments) != 1 || comments[0].Content != "saved finding" || comments[0].Path != "small.go") {
				t.Fatalf("reused finding lost: %+v", comments)
			}
			if !tc.withFinding && len(comments) != 0 {
				t.Fatalf("unexpected findings: %+v", comments)
			}
			var foundWarning bool
			for _, warning := range a.Warnings() {
				if warning.Type == "scan_subtask_error" && warning.File == "overflow.go" && strings.Contains(warning.Message, "prompt tokens") {
					foundWarning = true
				}
			}
			if !foundWarning {
				t.Fatalf("missing prompt-overflow failure warning: %+v", a.Warnings())
			}
			state, err := session.LoadResumeState(repo, a.SessionID())
			if err != nil {
				t.Fatal(err)
			}
			if len(state.Items) != wantItems {
				t.Fatalf("reusable items = %d, want %d", len(state.Items), wantItems)
			}
			for _, item := range state.Items {
				if item.FilePath != "small.go" {
					t.Errorf("incomplete file was marked reusable: %s", item.FilePath)
				}
				if tc.withFinding && (len(item.Comments) != 1 || item.Comments[0].Content != "saved finding") {
					t.Fatalf("reused checkpoint lost finding: %+v", item)
				}
			}
			path, err := session.SessionFilePath(repo, a.SessionID())
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var foundFailure bool
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				var record struct {
					Type     string `json:"type"`
					FilePath string `json:"filePath"`
					Error    string `json:"error"`
				}
				if err := json.Unmarshal([]byte(line), &record); err != nil {
					t.Fatal(err)
				}
				if record.Type == "review_item_failed" && record.FilePath == "overflow.go" && strings.Contains(record.Error, "prompt tokens") {
					foundFailure = true
				}
			}
			if !foundFailure {
				t.Fatal("session omitted the prompt-overflow failure record")
			}
		})
	}
}

// TestRun_TokenBudgetStopSurfacesAsBudgetExceeded pins the gap the dispatch
// look-ahead cannot close: a single scan item admitted while usage is still
// low can outgrow the budget on its own, and only an in-conversation check
// (llmloop.RunMainTask's per-round check) can stop it. The resulting
// StopTokenBudget must surface on the agent's BudgetExceeded() flag and
// emit a token_budget_reached warning so --format json and the stderr
// usage line agree with the run-level state.
//
// Without this propagation, --format json reports budget_exceeded: false
// after a token-budget stop fired and the stderr "budget_exceeded=..."
// line lies too, masking the partial stop from CI consumers.
//
// The fake client returns no tool calls, so the main loop retries rather
// than completing. Its first response consumes 26K tokens, exceeding the
// 25K budget; the next round's pre-flight check trips StopTokenBudget.
// The dispatch look-ahead admits the file because its estimated total
// cost is about 19K tokens, below the 25K budget.
func TestRun_TokenBudgetStopSurfacesAsBudgetExceeded(t *testing.T) {
	perCall := int64(26_000)
	budget := int64(25_000) // > per-file estimate (~19K) so the look-ahead admits the file
	client := &emptyRetryClient{perCallTokens: perCall}

	repo := initTestRepo(t)
	writeFile(t, repo, "single.go", []byte("package main\n"))
	gitCommit(t, repo, "init")

	tpl := budgetTestTemplate()
	tpl.MaxTokens = 100_000
	tpl.MaxToolRequestTimes = 3
	tpl.MainTask = template.LlmConversation{
		Messages: []template.ChatMessage{
			{Role: "user", Content: "review {{file_content}}"},
		},
	}

	a := NewAgent(Args{
		RepoDir:         repo,
		Template:        tpl,
		LLMClient:       client,
		MaxConcurrency:  1,
		SkipPlan:        true,
		SkipDedup:       true,
		SkipSummary:     true,
		MaxTokensBudget: budget,
		Session: session.New(repo, "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})
	_, err := a.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "all 1 file scan(s) failed") {
		t.Fatalf("expected all-files-failed error, got %v", err)
	}
	if !a.BudgetExceeded() {
		t.Error("BudgetExceeded() = false, want true after an in-flight token-budget stop")
	}
	var budgetWarnings int
	for _, w := range a.Warnings() {
		if w.Type == "token_budget_reached" {
			budgetWarnings++
		}
	}
	if budgetWarnings == 0 {
		t.Errorf("expected a token_budget_reached warning; got %+v", a.Warnings())
	}
	// The per-item scan_subtask_error warning also names the same stop
	// trigger via stop.Reason(); llmloop's per-round check tripped
	// StopTokenBudget, so that string is the diagnostic that survives
	// --format json's discarded [ocr] progress lines.
	var sawStopReason bool
	for _, w := range a.Warnings() {
		if w.Type == "scan_subtask_error" && strings.Contains(w.Message, llmloop.StopTokenBudget.Reason()) {
			sawStopReason = true
		}
	}
	if !sawStopReason {
		t.Errorf("scan_subtask_error warning does not name %q: %+v", llmloop.StopTokenBudget.Reason(), a.Warnings())
	}
}

// emptyRetryClient answers every request with an empty assistant content and
// no tool calls, so llmloop.RunMainTask takes the no-tool-calls retry branch
// instead of completing on task_done. Used to drive the per-round
// token-budget stop on a scan path that otherwise completes in one round.
type emptyRetryClient struct {
	perCallTokens int64
	calls         int64 // atomic
}

func (f *emptyRetryClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	atomic.AddInt64(&f.calls, 1)
	empty := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{Role: "assistant", Content: &empty},
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{
			PromptTokens:     f.perCallTokens,
			CompletionTokens: 0,
			TotalTokens:      f.perCallTokens,
		},
	}, nil
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestAgent_Getters(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test-model", session.SessionOptions{ReviewMode: "diff"})
	collector := tool.NewCommentCollector()

	a := New(Args{
		LLMClient:        &fakeAgentClient{},
		Model:            "test-model",
		CommentCollector: collector,
		Session:          sess,
		Template: template.Template{
			MaxTokens:           10000,
			MaxToolRequestTimes: 10,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "test"}},
			},
		},
	})

	a.diffs = []model.Diff{
		{NewPath: "a.go", Diff: "+code"},
		{NewPath: "b.go", Diff: "+more"},
	}

	if a.Session() != sess {
		t.Error("Session() does not return expected session")
	}
	if a.FilesReviewed() != 2 {
		t.Errorf("FilesReviewed() = %d, want 2", a.FilesReviewed())
	}
	if len(a.Diffs()) != 2 {
		t.Errorf("Diffs() len = %d, want 2", len(a.Diffs()))
	}
	if a.ProjectSummary() != "" {
		t.Errorf("ProjectSummary() = %q, want empty", a.ProjectSummary())
	}
	if a.TotalTokensUsed() != 0 {
		t.Errorf("TotalTokensUsed() = %d, want 0", a.TotalTokensUsed())
	}
	if a.TotalCacheReadTokens() != 0 {
		t.Errorf("TotalCacheReadTokens() = %d, want 0", a.TotalCacheReadTokens())
	}
	if a.TotalCacheWriteTokens() != 0 {
		t.Errorf("TotalCacheWriteTokens() = %d, want 0", a.TotalCacheWriteTokens())
	}
	if len(a.Warnings()) != 0 {
		t.Errorf("Warnings() should be empty initially, got %d", len(a.Warnings()))
	}
	if len(a.ToolCalls()) != 0 {
		t.Errorf("ToolCalls() should be empty initially, got %d", len(a.ToolCalls()))
	}
}

func TestAgentFilesReviewedCountsDispatchableDiffs(t *testing.T) {
	a := New(Args{})
	a.diffs = []model.Diff{
		{NewPath: "kept.go", OldPath: "kept.go", Diff: "+kept"},
		{NewPath: "removed.go", OldPath: "removed.go", Diff: "-removed", IsDeleted: true},
		{NewPath: "also-kept.go", OldPath: "also-kept.go", Diff: "+more"},
	}

	if got := a.FilesReviewed(); got != 2 {
		t.Errorf("FilesReviewed() = %d, want 2", got)
	}
	if got := len(a.Diffs()); got != 3 {
		t.Errorf("Diffs() len = %d, want 3", got)
	}
}

func TestAgent_RecordWarning(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test-model", session.SessionOptions{ReviewMode: "diff"})
	a := New(Args{
		LLMClient: &fakeAgentClient{},
		Model:     "test-model",
		Session:   sess,
		Template:  template.Template{MaxTokens: 10000, MaxToolRequestTimes: 5, MainTask: template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}}},
	})

	a.recordWarning("error", "main.go", "something")
	warnings := a.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].Type != "error" || warnings[0].File != "main.go" {
		t.Errorf("unexpected warning: %+v", warnings[0])
	}
}

func TestNewCommentWorkerPool(t *testing.T) {
	pool := NewCommentWorkerPool(2)
	if pool == nil {
		t.Fatal("NewCommentWorkerPool returned nil")
	}
}

func TestInjectDiffMap(t *testing.T) {
	reg := tool.NewRegistry()
	emptyDM := tool.NewDiffMap(nil)
	frd := tool.NewFileReadDiff(emptyDM)
	reg.Register(frd)

	a := New(Args{
		LLMClient: &fakeAgentClient{},
		Tools:     reg,
		Template:  template.Template{MaxTokens: 10000, MaxToolRequestTimes: 5, MainTask: template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}}},
	})
	a.diffs = []model.Diff{
		{NewPath: "main.go", OldPath: "main.go", Diff: "+new code"},
		{NewPath: "/dev/null", OldPath: "deleted.go", Diff: "-deleted"},
	}

	a.injectDiffMap()

	result, err := frd.Execute(context.Background(), map[string]any{
		"path_array": []any{"main.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "+new code") {
		t.Errorf("DiffMap did not contain main.go diff, got: %q", result)
	}

	result2, _ := frd.Execute(context.Background(), map[string]any{
		"path_array": []any{"deleted.go"},
	})
	if !strings.Contains(result2, "not found") {
		t.Errorf("/dev/null path should not be in DiffMap, got: %q", result2)
	}
}

func TestFilterDiffs(t *testing.T) {
	a := New(Args{
		FileFilter: &rules.FileFilter{
			Exclude: []string{"vendor/**"},
		},
	})
	a.diffs = []model.Diff{
		{NewPath: "main.go"},
		{NewPath: "vendor/dep.go"},
		{NewPath: "image.png", IsBinary: true},
		{NewPath: "handler.go"},
	}

	kept := a.filterDiffs(a.diffs)

	names := make(map[string]bool)
	for _, d := range kept {
		names[d.NewPath] = true
	}
	if names["vendor/dep.go"] {
		t.Error("vendor file should be filtered")
	}
	if names["image.png"] {
		t.Error("binary file should be filtered")
	}
	if !names["main.go"] || !names["handler.go"] {
		t.Error("valid files should be kept")
	}
}

func TestResolveSystemRule(t *testing.T) {
	t.Run("nil SystemRule returns empty", func(t *testing.T) {
		a := New(Args{SystemRule: nil})
		if got := a.resolveSystemRule("main.go"); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("with resolver", func(t *testing.T) {
		rule, err := rules.LoadDefault()
		if err != nil {
			t.Skipf("cannot load default rules: %v", err)
		}
		a := New(Args{SystemRule: rule})
		got := a.resolveSystemRule("main.go")
		if got == "" {
			t.Error("expected non-empty rule for .go file")
		}
	})
}

func TestFindDiff(t *testing.T) {
	a := New(Args{})
	a.diffs = []model.Diff{
		{NewPath: "a.go", OldPath: "a.go", Diff: "+a"},
		{NewPath: "b.go", OldPath: "old_b.go", Diff: "+b"},
	}

	if d := a.findDiff("a.go"); d == nil || d.NewPath != "a.go" {
		t.Error("findDiff should find by NewPath")
	}
	if d := a.findDiff("old_b.go"); d == nil || d.NewPath != "b.go" {
		t.Error("findDiff should find by OldPath")
	}
	if d := a.findDiff("nonexist.go"); d != nil {
		t.Error("findDiff should return nil for missing path")
	}
}

func TestExecuteReviewFilter_NoFilterTask(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})
	client := &fakeAgentClient{}
	a := New(Args{
		LLMClient: client,
		Model:     "test",
		Session:   sess,
		Template: template.Template{
			ReviewFilterTask:    nil,
			MaxTokens:           10000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
		},
	})

	a.executeReviewFilter(context.Background(), model.Diff{NewPath: "a.go"}, "a.go")
	if client.calls != 0 {
		t.Errorf("no LLM calls expected when ReviewFilterTask is nil, got %d", client.calls)
	}
}

func TestExecuteReviewFilter_NoComments(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})
	client := &fakeAgentClient{}
	a := New(Args{
		LLMClient: client,
		Model:     "test",
		Session:   sess,
		Template: template.Template{
			ReviewFilterTask: &template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Filter {{comments}} for {{path}} in {{diff}}"}},
			},
			MaxTokens:           10000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
		},
	})

	a.executeReviewFilter(context.Background(), model.Diff{NewPath: "a.go", Diff: "+x"}, "a.go")
	if client.calls != 0 {
		t.Errorf("no LLM calls expected when no comments exist, got %d", client.calls)
	}
}

func TestExecuteReviewFilter_RemovesComments(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})

	client := &fakeAgentClient{
		responses: []*llm.ChatResponse{{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{
					ToolCalls: []llm.ToolCall{{
						ID:   "call_1",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "report_incorrect_comments",
							Arguments: `{"comment_ids":["c-1"]}`,
						},
					}},
				},
			}},
			Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5},
		}},
	}

	collector := tool.NewCommentCollector()
	collector.Add(model.LlmComment{Path: "a.go", Content: "keep this"})
	collector.Add(model.LlmComment{Path: "a.go", Content: "remove this"})
	collector.Add(model.LlmComment{Path: "a.go", Content: "also keep"})

	a := New(Args{
		LLMClient:        client,
		Model:            "test",
		Session:          sess,
		CommentCollector: collector,
		Template: template.Template{
			ReviewFilterTask: &template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Filter: {{comments}} path={{path}} diff={{diff}}"}},
			},
			MaxTokens:           10000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
		},
	})

	a.executeReviewFilter(context.Background(), model.Diff{NewPath: "a.go", Diff: "+code"}, "a.go")

	comments := collector.CommentsForPath("a.go")
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments after filter, got %d", len(comments))
	}
	for _, c := range comments {
		if c.Content == "remove this" {
			t.Error("filtered comment should have been removed")
		}
	}
}

func TestExecuteReviewFilter_LLMError(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})

	client := &fakeAgentClient{
		responses: nil,
	}

	collector := tool.NewCommentCollector()
	collector.Add(model.LlmComment{Path: "a.go", Content: "comment"})

	a := New(Args{
		LLMClient:        client,
		Model:            "test",
		Session:          sess,
		CommentCollector: collector,
		Template: template.Template{
			ReviewFilterTask: &template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "{{comments}} {{path}} {{diff}}"}},
			},
			MaxTokens:           10000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
		},
	})

	a.executeReviewFilter(context.Background(), model.Diff{NewPath: "a.go", Diff: "+x"}, "a.go")

	comments := collector.CommentsForPath("a.go")
	if len(comments) != 1 {
		t.Errorf("comments should be unchanged on LLM error, got %d", len(comments))
	}
}

func TestExecuteReviewFilter_SkipFilter(t *testing.T) {
	t.Run("AC-1: SkipFilter disables the filter", func(t *testing.T) {
		tmpDir := t.TempDir()
		sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})
		client := &fakeAgentClient{}
		collector := tool.NewCommentCollector()
		collector.Add(model.LlmComment{Path: "a.go", Content: "comment"})

		a := New(Args{
			LLMClient:        client,
			Model:            "test",
			Session:          sess,
			SkipFilter:       true,
			CommentCollector: collector,
			Template: template.Template{
				ReviewFilterTask: &template.LlmConversation{
					Messages: []template.ChatMessage{{Role: "user", Content: "Filter: {{comments}}"}},
				},
				MaxTokens:           10000,
				MaxToolRequestTimes: 5,
				MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
			},
		})

		a.executeReviewFilter(context.Background(), model.Diff{NewPath: "a.go", Diff: "+code"}, "a.go")

		if client.calls != 0 {
			t.Errorf("no LLM calls expected when SkipFilter is true, got %d", client.calls)
		}
		comments := collector.CommentsForPath("a.go")
		if len(comments) != 1 {
			t.Errorf("comments should be unchanged when filter is skipped, got %d", len(comments))
		}
	})

	t.Run("AC-2: All comments preserved when skipped", func(t *testing.T) {
		tmpDir := t.TempDir()
		sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})
		client := &fakeAgentClient{}
		collector := tool.NewCommentCollector()
		collector.Add(model.LlmComment{Path: "a.go", Content: "comment 1"})
		collector.Add(model.LlmComment{Path: "a.go", Content: "comment 2"})
		collector.Add(model.LlmComment{Path: "a.go", Content: "comment 3"})

		a := New(Args{
			LLMClient:        client,
			Model:            "test",
			Session:          sess,
			SkipFilter:       true,
			CommentCollector: collector,
			Template: template.Template{
				ReviewFilterTask: &template.LlmConversation{
					Messages: []template.ChatMessage{{Role: "user", Content: "Filter: {{comments}}"}},
				},
				MaxTokens:           10000,
				MaxToolRequestTimes: 5,
				MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
			},
		})

		a.executeReviewFilter(context.Background(), model.Diff{NewPath: "a.go", Diff: "+code"}, "a.go")

		comments := collector.CommentsForPath("a.go")
		if len(comments) != 3 {
			t.Fatalf("expected 3 comments when filter is skipped, got %d", len(comments))
		}
	})

	t.Run("AC-3: Default (no SkipFilter) still runs filter", func(t *testing.T) {
		tmpDir := t.TempDir()
		sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})

		client := &fakeAgentClient{
			responses: []*llm.ChatResponse{{
				Choices: []llm.Choice{{
					Message: llm.ResponseMessage{
						ToolCalls: []llm.ToolCall{{
							ID:   "call_1",
							Type: "function",
							Function: llm.FunctionCall{
								Name:      "report_incorrect_comments",
								Arguments: `{"comment_ids":["c-1"]}`,
							},
						}},
					},
				}},
				Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5},
			}},
		}

		collector := tool.NewCommentCollector()
		collector.Add(model.LlmComment{Path: "a.go", Content: "keep this"})
		collector.Add(model.LlmComment{Path: "a.go", Content: "remove this"})

		a := New(Args{
			LLMClient:        client,
			Model:            "test",
			Session:          sess,
			CommentCollector: collector,
			Template: template.Template{
				ReviewFilterTask: &template.LlmConversation{
					Messages: []template.ChatMessage{{Role: "user", Content: "Filter: {{comments}} path={{path}} diff={{diff}}"}},
				},
				MaxTokens:           10000,
				MaxToolRequestTimes: 5,
				MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
			},
		})

		a.executeReviewFilter(context.Background(), model.Diff{NewPath: "a.go", Diff: "+code"}, "a.go")

		if client.calls == 0 {
			t.Error("LLM client should have been called when SkipFilter is false (default)")
		}
		comments := collector.CommentsForPath("a.go")
		if len(comments) != 1 {
			t.Errorf("expected 1 comment after filter, got %d", len(comments))
		}
	})

	t.Run("AC-4: SkipFilter is reached when ReviewFilterTask is non-nil", func(t *testing.T) {
		// After the nil-template guard, SkipFilter is the next early-return.
		// With a non-nil ReviewFilterTask + zero comments, the function would
		// normally fall through to the LLM call; SkipFilter must short-circuit it.
		tmpDir := t.TempDir()
		sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})
		client := &fakeAgentClient{}
		collector := tool.NewCommentCollector()
		collector.Add(model.LlmComment{Path: "a.go", Content: "comment"})

		a := New(Args{
			LLMClient:        client,
			Model:            "test",
			Session:          sess,
			SkipFilter:       true,
			CommentCollector: collector,
			Template: template.Template{
				ReviewFilterTask: &template.LlmConversation{
					Messages: []template.ChatMessage{{Role: "user", Content: "Filter: {{comments}}"}},
				},
				MaxTokens:           10000,
				MaxToolRequestTimes: 5,
				MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
			},
		})

		a.executeReviewFilter(context.Background(), model.Diff{NewPath: "a.go", Diff: "+x"}, "a.go")

		if client.calls != 0 {
			t.Errorf("no LLM calls expected when SkipFilter is true, got %d", client.calls)
		}
		if len(collector.CommentsForPath("a.go")) != 1 {
			t.Errorf("comments should be unchanged when filter is skipped")
		}
	})

	t.Run("AC-5: Skip takes priority over no comments", func(t *testing.T) {
		tmpDir := t.TempDir()
		sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})
		client := &fakeAgentClient{}

		a := New(Args{
			LLMClient:  client,
			Model:      "test",
			Session:    sess,
			SkipFilter: true,
			Template: template.Template{
				ReviewFilterTask: &template.LlmConversation{
					Messages: []template.ChatMessage{{Role: "user", Content: "Filter: {{comments}}"}},
				},
				MaxTokens:           10000,
				MaxToolRequestTimes: 5,
				MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
			},
		})

		a.executeReviewFilter(context.Background(), model.Diff{NewPath: "a.go", Diff: "+x"}, "a.go")

		if client.calls != 0 {
			t.Errorf("no LLM calls expected when SkipFilter is true, got %d", client.calls)
		}
	})
}

func TestExecutePlanPhase(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})

	planText := "review plan output"
	client := &fakeAgentClient{
		responses: []*llm.ChatResponse{{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{Content: &planText},
			}},
			Usage: &llm.UsageInfo{PromptTokens: 20, CompletionTokens: 10},
		}},
	}

	a := New(Args{
		LLMClient:  client,
		Model:      "test",
		Session:    sess,
		Background: "test background",
		Template: template.Template{
			PlanTask: &template.LlmConversation{
				Messages: []template.ChatMessage{
					{Role: "system", Content: "You are a planner. Date: {{current_system_date_time}}"},
					{Role: "user", Content: "Plan review for {{current_file_path}}. Rule: {{system_rule}}. Changes: {{change_files}}. Diff: {{diff}}. Background: {{requirement_background}}. Tools: {{plan_tools}}"},
				},
			},
			MaxTokens:           10000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
		},
	})
	a.currentDate = "2025-06-26 10:00"

	result, err := a.executePlanPhase(context.Background(), "main.go", "+new code", "helper.go", "check for bugs")
	if err != nil {
		t.Fatalf("executePlanPhase: %v", err)
	}
	if result != "review plan output" {
		t.Errorf("result = %q", result)
	}
	if a.TotalInputTokens() != 20 {
		t.Errorf("TotalInputTokens = %d, want 20", a.TotalInputTokens())
	}
}

func TestExecutePlanPhase_LLMError(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})

	client := &fakeAgentClient{responses: nil}

	a := New(Args{
		LLMClient: client,
		Model:     "test",
		Session:   sess,
		Template: template.Template{
			PlanTask: &template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "{{diff}}"}},
			},
			MaxTokens:           10000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
		},
	})

	_, err := a.executePlanPhase(context.Background(), "a.go", "+x", "", "")
	if err != nil {
		t.Logf("expected no-error from empty response, got: %v", err)
	}
}

func TestExecuteSubtask_EmptyMainTask(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})

	a := New(Args{
		LLMClient: &fakeAgentClient{},
		Model:     "test",
		Session:   sess,
		Template: template.Template{
			MaxTokens:           10000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: nil},
		},
	})
	a.currentDate = "2025-06-26 10:00"

	completed, stop, err := a.executeSubtask(context.Background(), model.Diff{NewPath: "a.go", Diff: "+x", Insertions: 1})
	if err == nil {
		t.Fatal("expected error for empty main_task messages")
	}
	if completed {
		t.Fatal("empty main_task should not complete review")
	}
	if stop != nil {
		t.Fatalf("stop = %+v, want nil on error", stop)
	}
	if !errors.Is(err, errMainTaskEmpty) {
		t.Errorf("expected errMainTaskEmpty sentinel via errors.Is, got: %v", err)
	}
}

func TestExecuteSubtask_TokenThresholdExceeded(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})

	a := New(Args{
		LLMClient: &fakeAgentClient{},
		Model:     "test",
		Session:   sess,
		Template: template.Template{
			MaxTokens:           10,
			MaxToolRequestTimes: 5,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{
					{Role: "user", Content: "Review: {{diff}}"},
				},
			},
		},
	})
	a.currentDate = "2025-06-26 10:00"
	a.diffs = []model.Diff{{NewPath: "a.go", Diff: strings.Repeat("code ", 200), Insertions: 100}}

	completed, stop, err := a.executeSubtask(context.Background(), a.diffs[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if completed {
		t.Fatal("token-threshold skip should not complete review")
	}
	if stop == nil {
		t.Fatal("expected structured stop for token-threshold skip")
	}
	if stop.class != session.FailureBudget {
		t.Errorf("stop.class = %q, want %q", stop.class, session.FailureBudget)
	}
	if stop.checkpoint == "" {
		t.Error("expected checkpoint text for token-threshold skip")
	}

	warnings := a.Warnings()
	found := false
	for _, w := range warnings {
		if w.Type == "token_threshold_exceeded" {
			found = true
		}
	}
	if !found {
		t.Error("expected token_threshold_exceeded warning")
	}
}

func TestExecuteSubtask_WithPlanPhase(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})

	planText := "my plan"
	doneContent := ""
	client := &fakeAgentClient{
		responses: []*llm.ChatResponse{
			{
				Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &planText}}},
				Usage:   &llm.UsageInfo{PromptTokens: 5, CompletionTokens: 3},
			},
			{
				Choices: []llm.Choice{{
					Message: llm.ResponseMessage{
						Content: &doneContent,
						ToolCalls: []llm.ToolCall{{
							ID: "c1", Type: "function",
							Function: llm.FunctionCall{Name: "task_done", Arguments: "{}"},
						}},
					},
				}},
				Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5},
			},
		},
	}

	reg := tool.NewRegistry()
	a := New(Args{
		LLMClient: client,
		Model:     "test",
		Session:   sess,
		Tools:     reg,
		Template: template.Template{
			MaxTokens:             100000,
			MaxToolRequestTimes:   10,
			PlanModeLineThreshold: 0,
			PlanTask: &template.LlmConversation{
				Messages: []template.ChatMessage{
					{Role: "user", Content: "Plan for {{current_file_path}}: {{diff}}"},
				},
			},
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{
					{Role: "user", Content: "Review {{current_file_path}} with plan {{plan_guidance}}: {{diff}}"},
				},
			},
		},
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}},
		},
	})
	a.currentDate = "2025-06-26 10:00"
	a.diffs = []model.Diff{{NewPath: "main.go", OldPath: "main.go", Diff: "+new code", Insertions: 5}}

	completed, stop, err := a.executeSubtask(context.Background(), a.diffs[0])
	if err != nil {
		t.Fatalf("executeSubtask: %v", err)
	}
	if !completed {
		t.Fatal("expected completed review")
	}
	if stop != nil {
		t.Fatalf("stop = %+v, want nil on completed review", stop)
	}
}

func TestExecuteSubtask_ContextCancelled(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})

	a := New(Args{
		LLMClient: &fakeAgentClient{},
		Model:     "test",
		Session:   sess,
		Template: template.Template{
			MaxTokens:           10000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "{{diff}}"}}},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	completed, stop, err := a.executeSubtask(ctx, model.Diff{NewPath: "a.go", Diff: "+x", Insertions: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if completed {
		t.Fatal("cancelled context should not complete review")
	}
	if stop != nil {
		t.Fatalf("stop = %+v, want nil on error", stop)
	}
}

func TestExecuteReviewFilter_WithTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})

	client := &fakeAgentClient{
		responses: []*llm.ChatResponse{{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{
					ToolCalls: []llm.ToolCall{{
						ID:   "call_1",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "approve_all_comments",
							Arguments: `{}`,
						},
					}},
				},
			}},
			Usage: &llm.UsageInfo{PromptTokens: 5, CompletionTokens: 2},
		}},
	}

	collector := tool.NewCommentCollector()
	collector.Add(model.LlmComment{Path: "a.go", Content: "comment"})

	a := New(Args{
		LLMClient:        client,
		Model:            "test",
		Session:          sess,
		CommentCollector: collector,
		Template: template.Template{
			ReviewFilterTask: &template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "{{comments}} {{path}} {{diff}}"}},
			},
			MaxTokens:           10000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
		},
	})

	a.executeReviewFilter(context.Background(), model.Diff{NewPath: "a.go", Diff: "+x"}, "a.go")

	comments := collector.CommentsForPath("a.go")
	if len(comments) != 1 {
		t.Errorf("expected 1 comment unchanged, got %d", len(comments))
	}
}

func TestDispatchSubtasks_AllFilteredBySize(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})

	a := New(Args{
		LLMClient: &fakeAgentClient{},
		Model:     "test",
		Session:   sess,
		Template: template.Template{
			MaxTokens:           10,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "{{diff}}"}}},
		},
	})
	a.diffs = []model.Diff{
		{NewPath: "big.go", Diff: strings.Repeat("word ", 500), Insertions: 100},
	}

	// All diffs oversized ⇒ nothing selected ⇒ skipped run, not a hard error.
	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Errorf("all-filtered-by-size should be a skipped run (nil error), got: %v", err)
	}
	if len(comments) != 0 {
		t.Errorf("expected no comments for a skipped run, got %d", len(comments))
	}
}

func TestDispatchSubtasks_AllFailed(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})

	a := New(Args{
		LLMClient: &fakeAgentClient{},
		Model:     "test",
		Session:   sess,
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: nil},
		},
	})
	a.diffs = []model.Diff{
		{NewPath: "a.go", Diff: "+x", Insertions: 1},
	}
	a.currentDate = "2025-06-26"

	_, err := a.dispatchSubtasks(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Errorf("expected failure error, got: %v", err)
	}
}

// classifyItemError maps a per-file review error to a coverage FailureClass and a
// static, leak-free reason. The mapping must recognize the sentinels through
// wrapping (errors.Is), and the reason must never echo the raw error text.
func TestClassifyItemError(t *testing.T) {
	const secret = "token=sk-LEAKED-SECRET absolute /home/alice/x"
	for _, tc := range []struct {
		name      string
		err       error
		wantClass session.FailureClass
	}{
		{"deadline", context.DeadlineExceeded, session.FailureTimeout},
		{"deadline_wrapped", fmt.Errorf("review %s: %w", secret, context.DeadlineExceeded), session.FailureTimeout},
		{"cancelled", context.Canceled, session.FailureCancelled},
		{"cancelled_wrapped", fmt.Errorf("aborted %s: %w", secret, context.Canceled), session.FailureCancelled},
		{"main_task_empty", errMainTaskEmpty, session.FailureConfiguration},
		{"main_task_empty_wrapped", fmt.Errorf("subtask %s: %w", secret, errMainTaskEmpty), session.FailureConfiguration},
		{"default_provider", errors.New(secret), session.FailureProvider},
	} {
		t.Run(tc.name, func(t *testing.T) {
			class, reason := classifyItemError(tc.err)
			if class != tc.wantClass {
				t.Errorf("class = %q, want %q", class, tc.wantClass)
			}
			if reason == "" {
				t.Error("reason is empty; a static safe reason is required")
			}
			if strings.Contains(reason, "sk-LEAKED-SECRET") || strings.Contains(reason, "/home/alice") {
				t.Errorf("reason leaked raw error text: %q", reason)
			}
		})
	}
}

// classifyMainLoopStop keeps the unknown class for the empty-round and
// compression exits, but each reason must name its trigger: in --format json
// runs the manifest reason is the only stop diagnostic that leaves the runner,
// so the three non-budget stops must not collapse into one string.
func TestClassifyMainLoopStop(t *testing.T) {
	reasons := make(map[string]llmloop.MainLoopStop)
	for _, tc := range []struct {
		name       string
		stop       llmloop.MainLoopStop
		wantClass  session.FailureClass
		wantReason string
	}{
		{"max_rounds", llmloop.StopMaxRounds, session.FailureBudget, "reached the maximum tool-request rounds without finishing"},
		{"empty_rounds", llmloop.StopEmptyRounds, session.FailureUnknown, "stopped after repeated rounds without a usable tool result"},
		{"compression", llmloop.StopCompression, session.FailureUnknown, "stopped because context compression exceeded its threshold"},
		{"none", llmloop.StopNone, session.FailureUnknown, "main task stopped before completing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			class, reason := classifyMainLoopStop(tc.stop)
			if class != tc.wantClass {
				t.Errorf("class = %q, want %q", class, tc.wantClass)
			}
			if reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", reason, tc.wantReason)
			}
			if prev, dup := reasons[reason]; dup {
				t.Errorf("reason %q is shared by %v and %v; stops must stay distinguishable", reason, prev, tc.stop)
			}
			reasons[reason] = tc.stop
		})
	}
}

func TestMaybeRunProjectSummary_Success(t *testing.T) {
	summaryText := "The codebase has consistent error handling but lacks input validation."
	client := &fakeAgentClient{
		responses: []*llm.ChatResponse{{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summaryText}}},
			Usage:   &llm.UsageInfo{PromptTokens: 200, CompletionTokens: 80},
		}},
	}

	sess := session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: "diff"})
	a := New(Args{
		LLMClient:        client,
		Model:            "test",
		SummaryEnabled:   true,
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		Session:          sess,
		Template: template.Template{
			MaxTokens:           10000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
			ProjectSummaryTask: &template.LlmConversation{
				Messages: []template.ChatMessage{
					{Role: "user", Content: "Summarize {{comment_count}} comments across {{file_count}} files:\n{{all_comments}}"},
				},
			},
		},
	})

	comments := []model.LlmComment{
		{Path: "a.go", Content: "missing error check"},
		{Path: "b.go", Content: "no input validation"},
	}

	a.maybeRunProjectSummary(context.Background(), comments)

	if a.ProjectSummary() != summaryText {
		t.Errorf("ProjectSummary() = %q, want %q", a.ProjectSummary(), summaryText)
	}
	if client.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", client.calls)
	}
}

func TestMaybeRunProjectSummary_SkipWhenDisabled(t *testing.T) {
	client := &fakeAgentClient{}
	sess := session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: "diff"})
	a := New(Args{
		LLMClient:        client,
		Model:            "test",
		SummaryEnabled:   false,
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		Session:          sess,
		Template: template.Template{
			MaxTokens:           10000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
			ProjectSummaryTask: &template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "{{all_comments}}"}},
			},
		},
	})

	a.maybeRunProjectSummary(context.Background(), []model.LlmComment{
		{Path: "a.go", Content: "something"},
	})

	if a.ProjectSummary() != "" {
		t.Errorf("ProjectSummary() = %q, want empty when SummaryEnabled is false", a.ProjectSummary())
	}
	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls when disabled, got %d", client.calls)
	}
}

func TestMaybeRunProjectSummary_SkipWhenNoComments(t *testing.T) {
	client := &fakeAgentClient{}
	sess := session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: "diff"})
	a := New(Args{
		LLMClient:        client,
		Model:            "test",
		SummaryEnabled:   true,
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		Session:          sess,
		Template: template.Template{
			MaxTokens:           10000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
			ProjectSummaryTask: &template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "{{all_comments}}"}},
			},
		},
	})

	a.maybeRunProjectSummary(context.Background(), nil)

	if a.ProjectSummary() != "" {
		t.Errorf("ProjectSummary() = %q, want empty when no comments", a.ProjectSummary())
	}
	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls with no comments, got %d", client.calls)
	}
}

// summaryTestAgent builds an agent whose only configured optional task is the
// project summary, so a test can drive maybeRunProjectSummary directly.
func summaryTestAgent(t *testing.T, client llm.LLMClient, enabled bool) *Agent {
	t.Helper()
	return New(Args{
		LLMClient:        client,
		Model:            "test",
		SummaryEnabled:   enabled,
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		Session:          session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: "diff"}),
		Template: template.Template{
			MaxTokens:           10000,
			MaxToolRequestTimes: 5,
			MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "t"}}},
			ProjectSummaryTask: &template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "{{all_comments}}"}},
			},
		},
	})
}

// summaryWarnings returns the project-summary warnings recorded on a.
func summaryWarnings(a *Agent) []AgentWarning {
	var out []AgentWarning
	for _, w := range a.Warnings() {
		if w.Type == "project_summary_error" {
			out = append(out, w)
		}
	}
	return out
}

// failingSummaryClient fails every request, standing in for a provider outage
// on the summary call.
type failingSummaryClient struct {
	calls int
	err   error
}

func (c *failingSummaryClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	c.calls++
	return nil, c.err
}

// TestMaybeRunProjectSummary_FailureRecordsWarning pins the failure to a
// warning, not just a stdout line: --format json and --audience agent replace
// stdout with io.Discard, so printing alone loses the fact that a summary the
// user asked for never arrived.
func TestMaybeRunProjectSummary_FailureRecordsWarning(t *testing.T) {
	client := &failingSummaryClient{err: errors.New("provider unavailable")}
	a := summaryTestAgent(t, client, true)

	a.maybeRunProjectSummary(context.Background(), []model.LlmComment{{Path: "a.go", Content: "boom"}})

	if client.calls != 1 {
		t.Fatalf("expected 1 LLM call, got %d", client.calls)
	}
	if a.ProjectSummary() != "" {
		t.Errorf("ProjectSummary() = %q, want empty after a failed call", a.ProjectSummary())
	}
	warnings := summaryWarnings(a)
	if len(warnings) != 1 {
		t.Fatalf("got %d project_summary_error warnings, want 1 (all: %+v)", len(warnings), a.Warnings())
	}
	if !strings.Contains(warnings[0].Message, "provider unavailable") {
		t.Errorf("warning message = %q, want it to carry the provider error", warnings[0].Message)
	}
}

// TestMaybeRunProjectSummary_EmptyBodyRecordsWarning covers the other silent
// hole: the call succeeds but the model returns nothing usable.
func TestMaybeRunProjectSummary_EmptyBodyRecordsWarning(t *testing.T) {
	blank := "   \n"
	client := &fakeAgentClient{responses: []*llm.ChatResponse{{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &blank}}},
		Usage:   &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 1},
	}}}
	a := summaryTestAgent(t, client, true)

	a.maybeRunProjectSummary(context.Background(), []model.LlmComment{{Path: "a.go", Content: "boom"}})

	if a.ProjectSummary() != "" {
		t.Errorf("ProjectSummary() = %q, want empty for a blank response", a.ProjectSummary())
	}
	if got := len(summaryWarnings(a)); got != 1 {
		t.Fatalf("got %d project_summary_error warnings, want 1 (all: %+v)", got, a.Warnings())
	}
}

// TestMaybeRunProjectSummary_SkipWhenBudgetExceeded and its cancellation
// counterpart guard the two gates that exist so a run stopped by a budget cap
// or a Ctrl-C does not spend one more request on a summary of partial results.
func TestMaybeRunProjectSummary_SkipWhenBudgetExceeded(t *testing.T) {
	client := &fakeAgentClient{}
	a := summaryTestAgent(t, client, true)
	a.budgetExceeded = true

	a.maybeRunProjectSummary(context.Background(), []model.LlmComment{{Path: "a.go", Content: "boom"}})

	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls once the budget is exceeded, got %d", client.calls)
	}
	if a.ProjectSummary() != "" {
		t.Errorf("ProjectSummary() = %q, want empty", a.ProjectSummary())
	}
}

func TestMaybeRunProjectSummary_SkipWhenContextCancelled(t *testing.T) {
	client := &fakeAgentClient{}
	a := summaryTestAgent(t, client, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a.maybeRunProjectSummary(ctx, []model.LlmComment{{Path: "a.go", Content: "boom"}})

	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls on a cancelled context, got %d", client.calls)
	}
	if a.ProjectSummary() != "" {
		t.Errorf("ProjectSummary() = %q, want empty", a.ProjectSummary())
	}
}

// TestMaybeRunProjectSummary_SkipWhenTemplateMissing mirrors the scan-side case:
// a template without the task must be a no-op, never a nil dereference.
func TestMaybeRunProjectSummary_SkipWhenTemplateMissing(t *testing.T) {
	client := &fakeAgentClient{}
	a := summaryTestAgent(t, client, true)
	a.args.Template.ProjectSummaryTask = nil

	a.maybeRunProjectSummary(context.Background(), []model.LlmComment{{Path: "a.go", Content: "boom"}})

	if client.calls != 0 {
		t.Errorf("expected 0 LLM calls without a summary template, got %d", client.calls)
	}
	if a.ProjectSummary() != "" {
		t.Errorf("ProjectSummary() = %q, want empty", a.ProjectSummary())
	}
}

func TestBuildSummaryCommentsList(t *testing.T) {
	t.Run("basic format", func(t *testing.T) {
		comments := []model.LlmComment{
			{Path: "main.go", Content: "missing nil check"},
			{Path: "handler.go", Content: "unused import"},
		}
		got := buildSummaryCommentsList(comments)
		if !strings.Contains(got, "- `main.go`: missing nil check") {
			t.Errorf("expected path-anchored entry for main.go, got:\n%s", got)
		}
		if !strings.Contains(got, "- `handler.go`: unused import") {
			t.Errorf("expected path-anchored entry for handler.go, got:\n%s", got)
		}
	})

	t.Run("newlines flattened", func(t *testing.T) {
		comments := []model.LlmComment{
			{Path: "a.go", Content: "line1\nline2\nline3"},
		}
		got := buildSummaryCommentsList(comments)
		if strings.Contains(got, "\n- ") && strings.Count(got, "\n") > 1 {
			t.Errorf("multi-line comment should be flattened to one line, got:\n%s", got)
		}
		if !strings.Contains(got, "line1 line2 line3") {
			t.Errorf("newlines should become spaces, got:\n%s", got)
		}
	})

	t.Run("truncation at 280 runes", func(t *testing.T) {
		long := strings.Repeat("あ", 300) // allow-non-english: fixture exercises multibyte truncation
		comments := []model.LlmComment{
			{Path: "x.go", Content: long},
		}
		got := buildSummaryCommentsList(comments)
		if !strings.Contains(got, "...") {
			t.Error("long content should be truncated with ellipsis")
		}
		line := strings.TrimPrefix(got, "- `x.go`: ")
		line = strings.TrimSuffix(line, "\n")
		runes := []rune(line)
		if len(runes) > 284 { // 280 + "..."
			t.Errorf("truncated line too long: %d runes", len(runes))
		}
	})
}

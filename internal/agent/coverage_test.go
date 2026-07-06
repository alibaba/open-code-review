package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/config/template"
	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/session"
	"github.com/open-code-review/open-code-review/internal/tool"
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

	filterResp := `["c-1"]`
	client := &fakeAgentClient{
		responses: []*llm.ChatResponse{{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{Content: &filterResp},
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

	completed, skipReason, err := a.executeSubtask(context.Background(), model.Diff{NewPath: "a.go", Diff: "+x", Insertions: 1})
	if err == nil {
		t.Fatal("expected error for empty main_task messages")
	}
	if completed {
		t.Fatal("empty main_task should not complete review")
	}
	if skipReason != "" {
		t.Fatalf("skipReason = %q, want empty on error", skipReason)
	}
	if !strings.Contains(err.Error(), "main_task.messages is empty") {
		t.Errorf("unexpected error: %v", err)
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

	completed, skipReason, err := a.executeSubtask(context.Background(), a.diffs[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if completed {
		t.Fatal("token-threshold skip should not complete review")
	}
	if skipReason == "" {
		t.Fatal("expected skip reason for token-threshold skip")
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

	completed, skipReason, err := a.executeSubtask(context.Background(), a.diffs[0])
	if err != nil {
		t.Fatalf("executeSubtask: %v", err)
	}
	if !completed {
		t.Fatal("expected completed review")
	}
	if skipReason != "" {
		t.Fatalf("skipReason = %q, want empty on completed review", skipReason)
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

	completed, skipReason, err := a.executeSubtask(ctx, model.Diff{NewPath: "a.go", Diff: "+x", Insertions: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if completed {
		t.Fatal("cancelled context should not complete review")
	}
	if skipReason != "" {
		t.Fatalf("skipReason = %q, want empty on error", skipReason)
	}
}

func TestExecuteReviewFilter_WithTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	sess := session.New(tmpDir, "main", "test", session.SessionOptions{ReviewMode: "diff"})

	filterResp := `[]`
	client := &fakeAgentClient{
		responses: []*llm.ChatResponse{{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &filterResp}}},
			Usage:   &llm.UsageInfo{PromptTokens: 5, CompletionTokens: 2},
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
				Timeout:  30,
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

	_, err := a.dispatchSubtasks(context.Background())
	if err == nil || !strings.Contains(err.Error(), "all diffs filtered out") {
		t.Errorf("expected 'all diffs filtered out' error, got: %v", err)
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

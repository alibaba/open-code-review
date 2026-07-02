package scan

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

// fakeScanClient is a minimal LLM client for scan tests that returns
// pre-configured responses in sequence.
type fakeScanClient struct {
	responses []*llm.ChatResponse
	idx       int
}

func (f *fakeScanClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	if f.idx >= len(f.responses) {
		empty := ""
		return &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &empty}}},
			Usage:   &llm.UsageInfo{},
		}, nil
	}
	resp := f.responses[f.idx]
	f.idx++
	return resp, nil
}

// errorScanClient always returns an error.
type errorScanClient struct {
	err error
}

func (e *errorScanClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, e.err
}

func TestAgent_Getters(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	a := newAgentForTest(t, tpl)
	a.items = []model.ScanItem{
		{Path: "a.go", Content: "package a", LineCount: 1},
		{Path: "b.go", Content: "package b", LineCount: 1},
	}

	if a.ProjectSummary() != "" {
		t.Errorf("ProjectSummary() should be empty, got %q", a.ProjectSummary())
	}
	if a.Session() == nil {
		t.Error("Session() should not be nil")
	}
	if a.FilesReviewed() != 2 {
		t.Errorf("FilesReviewed() = %d, want 2", a.FilesReviewed())
	}
	diffs := a.Diffs()
	if len(diffs) != 2 {
		t.Fatalf("Diffs() len = %d, want 2", len(diffs))
	}
	if diffs[0].NewPath != "a.go" || diffs[1].NewPath != "b.go" {
		t.Errorf("Diffs paths wrong: %q, %q", diffs[0].NewPath, diffs[1].NewPath)
	}
	if a.TotalTokensUsed() != 0 {
		t.Errorf("TotalTokensUsed() = %d, want 0", a.TotalTokensUsed())
	}
	if len(a.ToolCalls()) != 0 {
		t.Errorf("ToolCalls() should be empty")
	}
}

func TestLookupDiff(t *testing.T) {
	a := newAgentForTest(t, makeTemplateWithFullScan())
	a.items = []model.ScanItem{
		{Path: "main.go", Content: "package main\n", LineCount: 1},
		{Path: "lib.go", Content: "package lib\n", LineCount: 1},
	}

	d := a.lookupDiff("main.go")
	if d == nil {
		t.Fatal("expected non-nil for existing path")
	}
	if d.NewPath != "main.go" {
		t.Errorf("NewPath = %q, want main.go", d.NewPath)
	}
	if d.NewFileContent != "package main\n" {
		t.Errorf("NewFileContent = %q", d.NewFileContent)
	}

	if d2 := a.lookupDiff("nonexist.go"); d2 != nil {
		t.Errorf("expected nil for missing path, got %+v", d2)
	}
}

func TestFilterScanItems(t *testing.T) {
	a := NewAgent(Args{
		Template: makeTemplateWithFullScan(),
		FileFilter: &rules.FileFilter{
			Exclude: []string{"vendor/**"},
		},
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})

	items := []model.ScanItem{
		{Path: "main.go", Content: "package main\n", LineCount: 1},
		{Path: "image.png", Content: "", IsBinary: true},
		{Path: "vendor/dep.go", Content: "package dep\n", LineCount: 1},
		{Path: "handler.go", Content: "package h\n", LineCount: 1},
	}

	kept := a.filterScanItems(items)
	if len(kept) != 2 {
		t.Fatalf("expected 2 kept, got %d", len(kept))
	}
	for _, it := range kept {
		if it.Path == "image.png" || it.Path == "vendor/dep.go" {
			t.Errorf("should not keep %s", it.Path)
		}
	}
}

func TestWhyExcluded_AllBranches(t *testing.T) {
	tests := []struct {
		name   string
		item   model.ScanItem
		filter *rules.FileFilter
		want   model.ExcludeReason
	}{
		{
			name: "binary",
			item: model.ScanItem{Path: "img.png", IsBinary: true},
			want: model.ExcludeBinary,
		},
		{
			name:   "user exclude",
			item:   model.ScanItem{Path: "vendor/dep.go", Content: "x"},
			filter: &rules.FileFilter{Exclude: []string{"vendor/**"}},
			want:   model.ExcludeUserRule,
		},
		{
			name: "unsupported extension",
			item: model.ScanItem{Path: "data.xyz123"},
			want: model.ExcludeExtension,
		},
		{
			name:   "user include match passes",
			item:   model.ScanItem{Path: "src/main.go", Content: "x"},
			filter: &rules.FileFilter{Include: []string{"src/**"}},
			want:   model.ExcludeNone,
		},
		{
			name: "default excluded path",
			item: model.ScanItem{Path: "pkg/handler_test.go", Content: "x"},
			want: model.ExcludeDefaultPath,
		},
		{
			name: "allowed file passes",
			item: model.ScanItem{Path: "main.go", Content: "x"},
			want: model.ExcludeNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAgent(Args{
				Template:   makeTemplateWithFullScan(),
				FileFilter: tt.filter,
				Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
					ReviewMode: session.ReviewModeFullScan,
				}),
			})
			got := a.whyExcluded(tt.item)
			if got != tt.want {
				t.Errorf("whyExcluded(%q) = %q, want %q", tt.item.Path, got, tt.want)
			}
		})
	}
}

func TestExtFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", ".go"},
		{"src/lib/utils.ts", ".ts"},
		{"Makefile", ""},
		{".gitignore", ""},
		{"path/to/FILE.Go", ".go"},
		{"a/b/c.Test.JS", ".js"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extFromPath(tt.path)
			if got != tt.want {
				t.Errorf("extFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestMaybeRunPlan_Success(t *testing.T) {
	planJSON := `{"summary":"check error handling","checkpoints":[{"focus":"nil check","lines":"10-20","why":"potential NPE"}]}`
	client := &fakeScanClient{
		responses: []*llm.ChatResponse{{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &planJSON}}},
			Usage:   &llm.UsageInfo{PromptTokens: 100, CompletionTokens: 50},
		}},
	}

	tpl := makeTemplateWithFullScan()
	tpl.PlanTask = &template.LlmConversation{
		Messages: []template.ChatMessage{
			{Role: "user", Content: "Plan for {{current_file_path}}: {{file_content}}"},
		},
	}

	a := NewAgent(Args{
		Template:         tpl,
		LLMClient:        client,
		Model:            "test",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})
	a.currentDate = "2026-06-26 10:00"

	it := model.ScanItem{Path: "handler.go", Content: "package h\nfunc Handle() {}\n"}
	guidance := a.maybeRunPlan(context.Background(), it, "rule-text")

	if !strings.Contains(guidance, "nil check") {
		t.Errorf("guidance missing checkpoint, got: %q", guidance)
	}
	if !strings.Contains(guidance, "check error handling") {
		t.Errorf("guidance missing summary, got: %q", guidance)
	}
	if a.TotalTokensUsed() != 150 {
		t.Errorf("TotalTokensUsed() = %d, want 150", a.TotalTokensUsed())
	}
}

func TestMaybeRunProjectSummary_Success(t *testing.T) {
	summaryText := "Overall the code has good error handling but lacks input validation."
	client := &fakeScanClient{
		responses: []*llm.ChatResponse{{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summaryText}}},
			Usage:   &llm.UsageInfo{PromptTokens: 200, CompletionTokens: 80},
		}},
	}

	tpl := makeTemplateWithFullScan()
	tpl.ProjectSummaryTask = &template.LlmConversation{
		Messages: []template.ChatMessage{
			{Role: "user", Content: "Summarize {{comment_count}} comments across {{file_count}} files:\n{{all_comments}}"},
		},
	}

	a := NewAgent(Args{
		Template:         tpl,
		LLMClient:        client,
		Model:            "test",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})

	comments := []model.LlmComment{
		{Path: "a.go", Content: "missing error check"},
		{Path: "b.go", Content: "no input validation"},
	}

	a.maybeRunProjectSummary(context.Background(), comments)

	if a.ProjectSummary() != summaryText {
		t.Errorf("ProjectSummary() = %q, want %q", a.ProjectSummary(), summaryText)
	}
}

func TestMaybeRunProjectSummary_SkipWhenDisabled(t *testing.T) {
	a := newAgentForTest(t, makeTemplateWithFullScan())
	a.maybeRunProjectSummary(context.Background(), []model.LlmComment{{Path: "a.go", Content: "x"}})
	if a.ProjectSummary() != "" {
		t.Error("summary should be empty when template has no ProjectSummaryTask")
	}
}

func TestMaybeRunProjectSummary_SkipWhenNoComments(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	tpl.ProjectSummaryTask = &template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "{{all_comments}}"}},
	}
	a := NewAgent(Args{
		Template:         tpl,
		LLMClient:        &fakeScanClient{},
		Model:            "test",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})

	a.maybeRunProjectSummary(context.Background(), nil)
	if a.ProjectSummary() != "" {
		t.Error("summary should be empty when no comments")
	}
}

func TestMaybeRunDedup_Success(t *testing.T) {
	dedupResp := `{"groups":[{"members":["c-0","c-1"],"merged_content":"combined finding"},{"members":["c-2"]}]}`
	client := &fakeScanClient{
		responses: []*llm.ChatResponse{{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &dedupResp}}},
			Usage:   &llm.UsageInfo{PromptTokens: 80, CompletionTokens: 30},
		}},
	}

	tpl := makeTemplateWithFullScan()
	tpl.DedupTask = &template.LlmConversation{
		Messages: []template.ChatMessage{
			{Role: "user", Content: "Dedup: {{batch_comments}}"},
		},
	}

	collector := tool.NewCommentCollector()
	collector.Add(model.LlmComment{Path: "a.go", Content: "duplicate finding 1"})
	collector.Add(model.LlmComment{Path: "a.go", Content: "duplicate finding 2"})
	collector.Add(model.LlmComment{Path: "b.go", Content: "unique finding"})

	a := NewAgent(Args{
		Template:         tpl,
		LLMClient:        client,
		Model:            "test",
		CommentCollector: collector,
		Tools:            tool.NewRegistry(),
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})

	batchStart := 0
	a.maybeRunDedup(context.Background(), 0, batchStart)

	comments := collector.Comments()
	if len(comments) != 2 {
		t.Fatalf("expected 2 deduped comments, got %d", len(comments))
	}
	if comments[0].Content != "combined finding" {
		t.Errorf("merged comment content = %q, want 'combined finding'", comments[0].Content)
	}
	if comments[1].Content != "unique finding" {
		t.Errorf("second comment = %q, want 'unique finding'", comments[1].Content)
	}
}

func TestMaybeRunDedup_SkipWhenDisabled(t *testing.T) {
	collector := tool.NewCommentCollector()
	collector.Add(model.LlmComment{Path: "a.go", Content: "c1"})
	collector.Add(model.LlmComment{Path: "a.go", Content: "c2"})
	collector.Add(model.LlmComment{Path: "a.go", Content: "c3"})

	a := newAgentForTest(t, makeTemplateWithFullScan())
	a.args.CommentCollector = collector
	a.maybeRunDedup(context.Background(), 0, 0)

	if len(collector.Comments()) != 3 {
		t.Errorf("comments should be unchanged when dedup is disabled")
	}
}

func TestMaybeRunDedup_SkipWhenTooFewComments(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	tpl.DedupTask = &template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "{{batch_comments}}"}},
	}
	tpl.DedupMinComments = 5

	collector := tool.NewCommentCollector()
	collector.Add(model.LlmComment{Path: "a.go", Content: "only one"})

	a := NewAgent(Args{
		Template:         tpl,
		LLMClient:        &fakeScanClient{},
		Model:            "test",
		CommentCollector: collector,
		Tools:            tool.NewRegistry(),
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})

	a.maybeRunDedup(context.Background(), 0, 0)
	if len(collector.Comments()) != 1 {
		t.Error("comments should be unchanged when below min threshold")
	}
}

func TestExecuteSubtask_Success(t *testing.T) {
	doneContent := ""
	client := &fakeScanClient{
		responses: []*llm.ChatResponse{{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{
					Content: &doneContent,
					ToolCalls: []llm.ToolCall{{
						ID: "c1", Type: "function",
						Function: llm.FunctionCall{Name: "task_done", Arguments: "{}"},
					}},
				},
			}},
			Usage: &llm.UsageInfo{PromptTokens: 50, CompletionTokens: 20},
		}},
	}

	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 100000

	a := NewAgent(Args{
		Template:         tpl,
		LLMClient:        client,
		Model:            "test",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		SkipPlan:         true,
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})
	a.currentDate = "2026-06-26 10:00"

	it := model.ScanItem{Path: "main.go", Content: "package main\n", LineCount: 1}
	err := a.executeSubtask(context.Background(), it)
	if err != nil {
		t.Fatalf("executeSubtask: %v", err)
	}
	if a.TotalTokensUsed() != 70 {
		t.Errorf("TotalTokensUsed() = %d, want 70", a.TotalTokensUsed())
	}
}

func TestExecuteSubtask_WithPlan(t *testing.T) {
	planJSON := `{"summary":"focus on error paths","checkpoints":[]}`
	doneContent := ""
	client := &fakeScanClient{
		responses: []*llm.ChatResponse{
			{
				Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &planJSON}}},
				Usage:   &llm.UsageInfo{PromptTokens: 30, CompletionTokens: 20},
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
				Usage: &llm.UsageInfo{PromptTokens: 60, CompletionTokens: 30},
			},
		},
	}

	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 100000
	tpl.PlanTask = &template.LlmConversation{
		Messages: []template.ChatMessage{
			{Role: "user", Content: "Plan {{current_file_path}}: {{file_content}}"},
		},
	}

	a := NewAgent(Args{
		Template:         tpl,
		LLMClient:        client,
		Model:            "test",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})
	a.currentDate = "2026-06-26 10:00"

	it := model.ScanItem{Path: "handler.go", Content: "package h\nfunc Handle() error { return nil }\n", LineCount: 2}
	err := a.executeSubtask(context.Background(), it)
	if err != nil {
		t.Fatalf("executeSubtask: %v", err)
	}
}

func TestExecuteSubtask_ContextCancelled(t *testing.T) {
	a := newAgentForTest(t, makeTemplateWithFullScan())
	a.currentDate = "2026-06-26"

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := a.executeSubtask(ctx, model.ScanItem{Path: "a.go", Content: "x", LineCount: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestRun_EmptyTemplate(t *testing.T) {
	a := NewAgent(Args{
		Template: template.ScanTemplate{},
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})
	_, err := a.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "MAIN_TASK is missing") {
		t.Errorf("expected MAIN_TASK error, got: %v", err)
	}
}

func TestRun_NoReviewableFiles(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "img.png", []byte{0x89, 0x50, 0x4e, 0x47})
	gitCommit(t, repo, "binary")

	a := NewAgent(Args{
		RepoDir:          repo,
		Template:         makeTemplateWithFullScan(),
		LLMClient:        &fakeScanClient{},
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		SkipPlan:         true,
		SkipDedup:        true,
		SkipSummary:      true,
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})

	comments, err := a.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(comments) != 0 {
		t.Errorf("expected 0 comments for binary-only repo, got %d", len(comments))
	}
}

func TestRun_FullPipeline(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "main.go", []byte("package main\nfunc main() {}\n"))
	gitCommit(t, repo, "init")

	doneContent := ""
	client := &fakeScanClient{
		responses: []*llm.ChatResponse{{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{
					Content: &doneContent,
					ToolCalls: []llm.ToolCall{{
						ID: "c1", Type: "function",
						Function: llm.FunctionCall{Name: "task_done", Arguments: "{}"},
					}},
				},
			}},
			Usage: &llm.UsageInfo{PromptTokens: 100, CompletionTokens: 50},
		}},
	}

	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 100000

	a := NewAgent(Args{
		RepoDir:          repo,
		Template:         tpl,
		LLMClient:        client,
		Model:            "test",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MaxConcurrency:   1,
		SkipPlan:         true,
		SkipDedup:        true,
		SkipSummary:      true,
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})

	comments, err := a.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	_ = comments

	if a.FilesReviewed() != 1 {
		t.Errorf("FilesReviewed = %d, want 1", a.FilesReviewed())
	}
	if a.TotalTokensUsed() == 0 {
		t.Error("expected non-zero tokens after run")
	}
}

func TestDispatchSubtasks_AllFailed(t *testing.T) {
	client := &errorScanClient{err: context.DeadlineExceeded}

	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 100000

	a := NewAgent(Args{
		Template:         tpl,
		LLMClient:        client,
		Model:            "test",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MaxConcurrency:   1,
		SkipPlan:         true,
		SkipDedup:        true,
		SkipSummary:      true,
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})
	a.items = []model.ScanItem{{Path: "a.go", Content: "x", LineCount: 1}}
	a.currentDate = "2026-06-26"
	a.args.Tools.Freeze()

	_, err := a.dispatchSubtasks(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Errorf("expected all-failed error, got: %v", err)
	}
}

func TestPhaseEnabled(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	a := newAgentForTest(t, tpl)

	if a.planEnabled() {
		t.Error("planEnabled should be false without PlanTask")
	}
	if a.dedupEnabled() {
		t.Error("dedupEnabled should be false without DedupTask")
	}
	if a.summaryEnabled() {
		t.Error("summaryEnabled should be false without ProjectSummaryTask")
	}

	tpl.PlanTask = &template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "plan"}},
	}
	tpl.DedupTask = &template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "dedup"}},
	}
	tpl.ProjectSummaryTask = &template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "summary"}},
	}

	a2 := NewAgent(Args{
		Template:         tpl,
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})
	if !a2.planEnabled() {
		t.Error("planEnabled should be true with PlanTask")
	}
	if !a2.dedupEnabled() {
		t.Error("dedupEnabled should be true with DedupTask")
	}
	if !a2.summaryEnabled() {
		t.Error("summaryEnabled should be true with ProjectSummaryTask")
	}

	a3 := NewAgent(Args{
		Template:         tpl,
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		SkipPlan:         true,
		SkipDedup:        true,
		SkipSummary:      true,
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})
	if a3.planEnabled() {
		t.Error("planEnabled should be false with SkipPlan")
	}
	if a3.dedupEnabled() {
		t.Error("dedupEnabled should be false with SkipDedup")
	}
	if a3.summaryEnabled() {
		t.Error("summaryEnabled should be false with SkipSummary")
	}
}

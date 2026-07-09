package llmloop

import (
	"context"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/config/template"
	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/session"
	"github.com/open-code-review/open-code-review/internal/tool"
)

type fakeLLMClient struct {
	response *llm.ChatResponse
	err      error
}

func (f *fakeLLMClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	return f.response, f.err
}

func newTestRunner(client llm.LLMClient, tpl template.Template) *Runner {
	sess := session.New(t_tempDir, "main", "test-model", session.SessionOptions{ReviewMode: "diff"})
	collector := tool.NewCommentCollector()
	return NewRunner(Deps{
		LLMClient:        client,
		Model:            "test-model",
		Template:         tpl,
		CommentCollector: collector,
		Session:          sess,
	})
}

var t_tempDir string

func TestRecordWarning(t *testing.T) {
	t_tempDir = t.TempDir()
	r := newTestRunner(&fakeLLMClient{}, template.Template{})
	r.RecordWarning("error", "main.go", "something went wrong")
	r.RecordWarning("warn", "lib.go", "not great")

	warnings := r.Warnings()
	if len(warnings) != 2 {
		t.Fatalf("len = %d, want 2", len(warnings))
	}
	if warnings[0].Type != "error" || warnings[0].File != "main.go" {
		t.Errorf("warning[0] = %+v", warnings[0])
	}
	if warnings[1].Message != "not great" {
		t.Errorf("warning[1].Message = %q", warnings[1].Message)
	}
}

func TestRecordToolCall(t *testing.T) {
	t_tempDir = t.TempDir()
	r := newTestRunner(&fakeLLMClient{}, template.Template{})
	r.recordToolCall("file_read")
	r.recordToolCall("file_read")
	r.recordToolCall("code_comment")

	calls := r.ToolCalls()
	if calls["file_read"] != 2 {
		t.Errorf("file_read = %d, want 2", calls["file_read"])
	}
	if calls["code_comment"] != 1 {
		t.Errorf("code_comment = %d, want 1", calls["code_comment"])
	}
}

func TestRecordUsage(t *testing.T) {
	t_tempDir = t.TempDir()
	r := newTestRunner(&fakeLLMClient{}, template.Template{})

	r.RecordUsage(nil)
	if r.TotalInputTokens() != 0 {
		t.Error("nil usage should not change counters")
	}

	r.RecordUsage(&llm.UsageInfo{
		PromptTokens:     100,
		CompletionTokens: 50,
		CacheReadTokens:  10,
		CacheWriteTokens: 5,
	})
	if r.TotalInputTokens() != 100 {
		t.Errorf("TotalInputTokens = %d, want 100", r.TotalInputTokens())
	}
	if r.TotalOutputTokens() != 50 {
		t.Errorf("TotalOutputTokens = %d, want 50", r.TotalOutputTokens())
	}
	if r.TotalCacheReadTokens() != 10 {
		t.Errorf("TotalCacheReadTokens = %d, want 10", r.TotalCacheReadTokens())
	}
	if r.TotalCacheWriteTokens() != 5 {
		t.Errorf("TotalCacheWriteTokens = %d, want 5", r.TotalCacheWriteTokens())
	}
	if r.TotalTokensUsed() != 150 {
		t.Errorf("TotalTokensUsed = %d, want 150", r.TotalTokensUsed())
	}
}

func TestCollectPendingComments_NilPool(t *testing.T) {
	t_tempDir = t.TempDir()
	collector := tool.NewCommentCollector()
	collector.Add(model.LlmComment{Path: "a.go", Content: "fix"})

	r := NewRunner(Deps{
		CommentCollector:  collector,
		CommentWorkerPool: nil,
	})
	comments := r.CollectPendingComments()
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Path != "a.go" {
		t.Errorf("comment.Path = %q", comments[0].Path)
	}
}

func TestCancelPendingCompression_NilJob(t *testing.T) {
	t_tempDir = t.TempDir()
	r := newTestRunner(&fakeLLMClient{}, template.Template{})
	r.cancelPendingCompression()
}

func TestCancelPendingCompression_WithJob(t *testing.T) {
	t_tempDir = t.TempDir()
	r := newTestRunner(&fakeLLMClient{}, template.Template{})

	cancelled := false
	job := &compressionJob{
		done:   make(chan struct{}),
		cancel: func() { cancelled = true },
	}
	r.pendingJob = job
	r.cancelPendingCompression()

	if !cancelled {
		t.Error("cancel was not called")
	}
	if r.pendingJob != nil {
		t.Error("pendingJob should be nil after cancel")
	}
}

func TestTryApplyPendingCompression_NilJob(t *testing.T) {
	t_tempDir = t.TempDir()
	r := newTestRunner(&fakeLLMClient{}, template.Template{})
	msgs := []llm.Message{msg("user", "hi")}
	if r.tryApplyPendingCompression(&msgs) {
		t.Error("expected false for nil job")
	}
}

func TestTryApplyPendingCompression_NotDone(t *testing.T) {
	t_tempDir = t.TempDir()
	r := newTestRunner(&fakeLLMClient{}, template.Template{})

	job := &compressionJob{
		done:   make(chan struct{}),
		cancel: func() {},
	}
	r.pendingJob = job

	msgs := []llm.Message{msg("user", "hi")}
	if r.tryApplyPendingCompression(&msgs) {
		t.Error("expected false for non-completed job")
	}
}

func TestTryApplyPendingCompression_Applied(t *testing.T) {
	t_tempDir = t.TempDir()
	r := newTestRunner(&fakeLLMClient{}, template.Template{})

	rebuilt := []llm.Message{msg("system", "sys"), msg("user", "compressed")}
	job := &compressionJob{
		done:        make(chan struct{}),
		cancel:      func() {},
		rebuilt:     rebuilt,
		snapshotLen: 3,
	}
	close(job.done)
	r.pendingJob = job

	msgs := []llm.Message{
		msg("system", "sys"),
		msg("user", "orig"),
		msg("assistant", "resp"),
		msg("tool", "appended after snapshot"),
	}
	applied := r.tryApplyPendingCompression(&msgs)
	if !applied {
		t.Fatal("expected applied=true")
	}
	if len(msgs) != 3 {
		t.Fatalf("len(msgs) = %d, want 3", len(msgs))
	}
	if msgs[1].ExtractText() != "compressed" {
		t.Errorf("msgs[1] = %q, want compressed", msgs[1].ExtractText())
	}
	if msgs[2].ExtractText() != "appended after snapshot" {
		t.Errorf("msgs[2] = %q, want appended after snapshot", msgs[2].ExtractText())
	}
	if r.pendingJob != nil {
		t.Error("pendingJob should be nil after apply")
	}
}

func TestTryApplyPendingCompression_NilRebuilt(t *testing.T) {
	t_tempDir = t.TempDir()
	r := newTestRunner(&fakeLLMClient{}, template.Template{})

	job := &compressionJob{
		done:        make(chan struct{}),
		cancel:      func() {},
		rebuilt:     nil,
		snapshotLen: 3,
	}
	close(job.done)
	r.pendingJob = job

	msgs := []llm.Message{msg("user", "hi")}
	applied := r.tryApplyPendingCompression(&msgs)
	if applied {
		t.Error("expected false when rebuilt is nil (compression failed)")
	}
	if r.pendingJob != nil {
		t.Error("pendingJob should be nil even on non-apply")
	}
}

func TestPartitionMessages_CompressionNeeded(t *testing.T) {
	messages := []llm.Message{
		msg("system", "sys"),
		msg("user", "prompt"),
	}
	for i := 0; i < 20; i++ {
		messages = append(messages, msg("assistant", strings.Repeat("word ", 200)))
		messages = append(messages, msg("tool", strings.Repeat("data ", 100)))
	}

	result := partitionMessages(messages, 500, 0)

	if result.frozenEnd != 2 {
		t.Errorf("frozenEnd = %d, want 2", result.frozenEnd)
	}
	if result.activeCount == 0 {
		t.Error("activeCount should be > 0 for compression-needed case")
	}
	if result.compressEnd >= len(messages) {
		t.Errorf("compressEnd = %d, should be < %d", result.compressEnd, len(messages))
	}
	if result.compressEnd <= result.frozenEnd {
		t.Errorf("compressEnd (%d) should be > frozenEnd (%d)", result.compressEnd, result.frozenEnd)
	}
}

func TestRunCompression_EmptyTemplate(t *testing.T) {
	t_tempDir = t.TempDir()
	r := newTestRunner(&fakeLLMClient{}, template.Template{
		MaxTokens: 1000,
	})

	msgs := []llm.Message{
		msg("system", "sys"),
		msg("user", "prompt"),
		msg("assistant", "resp"),
	}
	got, err := r.runCompression(context.Background(), msgs, "test.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 (frozen only), got %d", len(got))
	}
}

func TestRunCompression_ShortMessages(t *testing.T) {
	t_tempDir = t.TempDir()
	tpl := template.Template{
		MemoryCompressionTask: template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "{{context}}"}},
		},
		MaxTokens: 1000,
	}
	r := newTestRunner(&fakeLLMClient{}, tpl)

	msgs := []llm.Message{msg("system", "sys"), msg("user", "prompt")}
	got, err := r.runCompression(context.Background(), msgs, "test.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2, got %d", len(got))
	}
}

func TestRunCompression_Success(t *testing.T) {
	t_tempDir = t.TempDir()
	summaryText := "compressed summary"
	client := &fakeLLMClient{
		response: &llm.ChatResponse{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{Content: &summaryText},
			}},
			Usage: &llm.UsageInfo{PromptTokens: 100, CompletionTokens: 20},
		},
	}
	tpl := template.Template{
		MemoryCompressionTask: template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "Summarize: {{context}}"}},
		},
		MaxTokens: 50,
	}
	r := newTestRunner(client, tpl)

	msgs := []llm.Message{
		msg("system", "sys"),
		msg("user", "prompt"),
	}
	for i := 0; i < 10; i++ {
		msgs = append(msgs, msg("assistant", strings.Repeat("word ", 100)))
		msgs = append(msgs, msg("tool", strings.Repeat("data ", 50)))
	}

	got, err := r.runCompression(context.Background(), msgs, "test.go")
	if err != nil {
		t.Fatalf("runCompression: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(got))
	}
	if !strings.Contains(got[1].ExtractText(), "previous_review_summary") {
		t.Errorf("expected summary in rebuilt messages, got: %s", got[1].ExtractText())
	}
	if r.TotalInputTokens() != 100 {
		t.Errorf("TotalInputTokens = %d, want 100", r.TotalInputTokens())
	}
}

func TestRunCompression_LLMError(t *testing.T) {
	t_tempDir = t.TempDir()
	client := &fakeLLMClient{
		err: context.DeadlineExceeded,
	}
	tpl := template.Template{
		MemoryCompressionTask: template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "{{context}}"}},
		},
		MaxTokens: 50,
	}
	r := newTestRunner(client, tpl)

	msgs := []llm.Message{
		msg("system", "sys"),
		msg("user", "prompt"),
	}
	for i := 0; i < 10; i++ {
		msgs = append(msgs, msg("assistant", strings.Repeat("word ", 100)))
		msgs = append(msgs, msg("tool", strings.Repeat("data ", 50)))
	}

	got, err := r.runCompression(context.Background(), msgs, "test.go")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(got) != len(msgs) {
		t.Errorf("expected messages unchanged on error, got %d vs %d", len(got), len(msgs))
	}
}

func TestRunCompression_EmptySummary(t *testing.T) {
	t_tempDir = t.TempDir()
	emptyStr := ""
	client := &fakeLLMClient{
		response: &llm.ChatResponse{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{Content: &emptyStr},
			}},
		},
	}
	tpl := template.Template{
		MemoryCompressionTask: template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "{{context}}"}},
		},
		MaxTokens: 50,
	}
	r := newTestRunner(client, tpl)

	msgs := []llm.Message{
		msg("system", "sys"),
		msg("user", "prompt"),
	}
	for i := 0; i < 10; i++ {
		msgs = append(msgs, msg("assistant", strings.Repeat("word ", 100)))
		msgs = append(msgs, msg("tool", strings.Repeat("data ", 50)))
	}

	got, err := r.runCompression(context.Background(), msgs, "test.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(msgs) {
		t.Errorf("expected messages unchanged on empty summary, got %d vs %d", len(got), len(msgs))
	}
}

func TestTriggerAsyncCompression(t *testing.T) {
	t_tempDir = t.TempDir()
	summaryText := "async summary"
	client := &fakeLLMClient{
		response: &llm.ChatResponse{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{Content: &summaryText},
			}},
			Usage: &llm.UsageInfo{PromptTokens: 50, CompletionTokens: 10},
		},
	}
	tpl := template.Template{
		MemoryCompressionTask: template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "{{context}}"}},
		},
		MaxTokens: 50,
	}
	r := newTestRunner(client, tpl)

	msgs := []llm.Message{
		msg("system", "sys"),
		msg("user", "prompt"),
	}
	for i := 0; i < 10; i++ {
		msgs = append(msgs, msg("assistant", strings.Repeat("word ", 100)))
		msgs = append(msgs, msg("tool", strings.Repeat("data ", 50)))
	}

	r.triggerAsyncCompression(context.Background(), msgs, "test.go")

	r.compressionMu.Lock()
	job := r.pendingJob
	r.compressionMu.Unlock()

	if job == nil {
		t.Fatal("expected pendingJob to be set")
	}
	<-job.done

	if job.rebuilt == nil {
		t.Fatal("expected rebuilt to be set after completion")
	}
}

func TestStripMarkdownFences_AdditionalCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"markdown fence", "```markdown\ncontent\n```", "content"},
		{"xml fence", "```xml\n<tag/>\n```", "<tag/>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripMarkdownFences(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

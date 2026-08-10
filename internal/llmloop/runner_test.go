// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type fakeLLMClient struct {
	response *llm.ChatResponse
	err      error
}

func (f *fakeLLMClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	return f.response, f.err
}

// gatedLLMClient signals when a request starts, then blocks until release
// is closed. Requests whose context is canceled while blocked are counted,
// letting tests assert that no in-flight compression was aborted.
type gatedLLMClient struct {
	started  chan struct{}
	release  chan struct{}
	canceled atomic.Int64
	response *llm.ChatResponse
}

func (g *gatedLLMClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	g.started <- struct{}{}
	select {
	case <-g.release:
		return g.response, nil
	case <-ctx.Done():
		g.canceled.Add(1)
		return nil, ctx.Err()
	}
}

// concurrentFakeClient is goroutine-safe and distinguishes compression
// requests (no tools attached) from main-loop requests (tools attached),
// mirroring how runCompression and RunPerFile build their ChatRequests.
type concurrentFakeClient struct {
	compressionCalls atomic.Int64
}

func (c *concurrentFakeClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if len(req.Tools) == 0 {
		c.compressionCalls.Add(1)
		summary := "compressed summary"
		return &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}},
		}, nil
	}
	content := strings.Repeat("word ", 650)
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &content,
				ToolCalls: []llm.ToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "file_read",
						Arguments: `{"path":"f.go"}`,
					},
				}},
			},
		}},
	}, nil
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
	r.cancelPendingCompression(&compressionState{})
}

func TestCancelPendingCompression_WithJob(t *testing.T) {
	t_tempDir = t.TempDir()
	r := newTestRunner(&fakeLLMClient{}, template.Template{})

	cancelled := false
	job := &compressionJob{
		done:   make(chan struct{}),
		cancel: func() { cancelled = true },
	}
	st := &compressionState{pendingJob: job}
	r.cancelPendingCompression(st)

	if !cancelled {
		t.Error("cancel was not called")
	}
	if st.pendingJob != nil {
		t.Error("pendingJob should be nil after cancel")
	}
}

func TestTryApplyPendingCompression_NilJob(t *testing.T) {
	t_tempDir = t.TempDir()
	r := newTestRunner(&fakeLLMClient{}, template.Template{})
	msgs := []llm.Message{msg("user", "hi")}
	if r.tryApplyPendingCompression(&compressionState{}, &msgs) {
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
	st := &compressionState{pendingJob: job}

	msgs := []llm.Message{msg("user", "hi")}
	if r.tryApplyPendingCompression(st, &msgs) {
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
	st := &compressionState{pendingJob: job}

	msgs := []llm.Message{
		msg("system", "sys"),
		msg("user", "orig"),
		msg("assistant", "resp"),
		msg("tool", "appended after snapshot"),
	}
	applied := r.tryApplyPendingCompression(st, &msgs)
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
	if st.pendingJob != nil {
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
	st := &compressionState{pendingJob: job}

	msgs := []llm.Message{msg("user", "hi")}
	applied := r.tryApplyPendingCompression(st, &msgs)
	if applied {
		t.Error("expected false when rebuilt is nil (compression failed)")
	}
	if st.pendingJob != nil {
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

func TestRunCompression_EmptyTemplatePreservesConversation(t *testing.T) {
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
	// Without a compression template there is nothing to summarize into;
	// the conversation must be kept rather than truncated to the frozen zone.
	if len(got) != len(msgs) {
		t.Fatalf("compression without a template dropped history: got %d messages, want %d", len(got), len(msgs))
	}
	for i := range msgs {
		if got[i].Role != msgs[i].Role || got[i].ExtractText() != msgs[i].ExtractText() {
			t.Errorf("message %d changed: got %+v, want %+v", i, got[i], msgs[i])
		}
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

	st := &compressionState{}
	r.triggerAsyncCompression(context.Background(), st, msgs, "test.go")

	st.mu.Lock()
	job := st.pendingJob
	st.mu.Unlock()

	if job == nil {
		t.Fatal("expected pendingJob to be set")
	}
	<-job.done

	if job.rebuilt == nil {
		t.Fatal("expected rebuilt to be set after completion")
	}
}

func TestCompression_CrossFileIsolation(t *testing.T) {
	t_tempDir = t.TempDir()
	summary := "summary A"
	gated := &gatedLLMClient{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
		response: &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}},
		},
	}
	tpl := template.Template{
		MemoryCompressionTask: template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "{{context}}"}},
		},
		MaxTokens: 50,
	}
	r := newTestRunner(gated, tpl)

	msgsA := []llm.Message{msg("system", "sys"), msg("user", "file A prompt")}
	for i := 0; i < 10; i++ {
		msgsA = append(msgsA, msg("assistant", strings.Repeat("word ", 100)))
		msgsA = append(msgsA, msg("tool", strings.Repeat("data ", 50)))
	}

	// File A starts an async compression; its LLM request is now in flight.
	stA := &compressionState{}
	r.triggerAsyncCompression(context.Background(), stA, msgsA, "a.go")
	<-gated.started

	// File B, sharing the Runner, cancels and tries to consume a pending
	// job. Neither operation may touch file A's in-flight compression.
	stB := &compressionState{}
	msgsB := []llm.Message{msg("system", "sys"), msg("user", "file B prompt")}
	wantB := msgsB[1].ExtractText()

	r.cancelPendingCompression(stB)
	if r.tryApplyPendingCompression(stB, &msgsB) {
		t.Error("file B must not consume a compression job it never started")
	}
	if len(msgsB) != 2 || msgsB[1].ExtractText() != wantB {
		t.Error("file B's messages must be unchanged")
	}

	stA.mu.Lock()
	pending := stA.pendingJob
	stA.mu.Unlock()
	if pending == nil {
		t.Fatal("file A's job must still be pending after file B's cancel")
	}
	if got := gated.canceled.Load(); got != 0 {
		t.Errorf("in-flight compression requests canceled = %d, want 0", got)
	}

	// A second trigger while a job is pending must be a no-op, not a
	// replacement — the in-flight job stays the owner of the slot.
	r.triggerAsyncCompression(context.Background(), stA, msgsA, "a.go")
	stA.mu.Lock()
	replaced := stA.pendingJob != pending
	stA.mu.Unlock()
	if replaced {
		t.Error("second trigger must not replace the in-flight job")
	}

	// Let A's compression finish.
	close(gated.release)
	<-pending.done

	t.Run("SummaryAppliesOnlyToOwningConversation", func(t *testing.T) {
		if r.tryApplyPendingCompression(stB, &msgsB) {
			t.Error("file B must not receive file A's summary")
		}
		if len(msgsB) != 2 || msgsB[1].ExtractText() != wantB {
			t.Error("file B's messages must be byte-identical")
		}

		// A message appended after the snapshot must survive the apply.
		msgsA = append(msgsA, msg("tool", "post-snapshot"))
		if !r.tryApplyPendingCompression(stA, &msgsA) {
			t.Fatal("file A should apply its own completed compression")
		}
		if !strings.Contains(msgsA[1].ExtractText(), "<previous_review_summary>") {
			t.Errorf("file A's rebuilt prompt should embed the summary, got: %s", msgsA[1].ExtractText())
		}
		if last := msgsA[len(msgsA)-1].ExtractText(); last != "post-snapshot" {
			t.Errorf("post-snapshot suffix lost, last message = %q", last)
		}
	})
}

func TestAddNextMessage_NoStartThenCancelSameCall(t *testing.T) {
	t_tempDir = t.TempDir()
	summary := "compressed summary"
	// release is pre-closed: requests complete instantly, and canceled
	// only increments if a compression context was aborted mid-flight.
	gated := &gatedLLMClient{
		started: make(chan struct{}, 8),
		release: make(chan struct{}),
		response: &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}},
		},
	}
	close(gated.release)
	tpl := template.Template{
		MemoryCompressionTask: template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "{{context}}"}},
		},
		MaxTokens: 1000, // soft 600, warn 800
	}
	r := newTestRunner(gated, tpl)

	// Pre-append the conversation sits between the soft and warning
	// thresholds; the appended round pushes it over the warning threshold —
	// the same-file case from #384 where a job was started and then
	// cancelled within one update.
	msgs := []llm.Message{
		msg("system", "sys"),
		msg("user", "prompt"),
		msg("assistant", strings.Repeat("word ", 400)),
		msg("tool", strings.Repeat("data ", 290)),
	}
	calls := []llm.ToolCall{{
		ID:       "c1",
		Type:     "function",
		Function: llm.FunctionCall{Name: "file_read", Arguments: `{"path":"f.go"}`},
	}}
	results := []tool.ToolCallResult{{
		ToolCallID: "c1",
		Name:       "file_read",
		Result:     strings.Repeat("data ", 100),
	}}

	st := &compressionState{}
	additions := []llm.Message{llm.NewToolCallMessage(strings.Repeat("word ", 200), calls)}
	for _, rs := range results {
		additions = append(additions, llm.NewToolResultMessage(rs.ToolCallID, rs.Result))
	}
	ok := r.appendMessagesWithCompression(context.Background(), additions, &msgs, "f.go", st)

	if !ok {
		t.Error("expected true: sync compression should bring the count under the warning threshold")
	}
	if got := gated.canceled.Load(); got != 0 {
		t.Errorf("compression requests canceled mid-flight = %d, want 0", got)
	}
	st.mu.Lock()
	pending := st.pendingJob
	st.mu.Unlock()
	if pending != nil {
		t.Error("no async job should be pending after a sync compression in the same call")
	}
	if len(gated.started) != 1 {
		t.Errorf("compression LLM calls = %d, want exactly 1 (the sync one)", len(gated.started))
	}
	if !strings.Contains(msgs[1].ExtractText(), "<previous_review_summary>") {
		t.Errorf("sync compression should have rebuilt the prompt, got: %s", msgs[1].ExtractText())
	}
}

func TestRunPerFile_ConcurrentFilesCompression_Race(t *testing.T) {
	t_tempDir = t.TempDir()
	tpl := template.Template{
		MemoryCompressionTask: template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "{{context}}"}},
		},
		MaxTokens:           1000,
		MaxToolRequestTimes: 5,
	}
	reg := tool.NewRegistry()
	reg.Register(&fakeFileReadProvider{result: "package main\n"})
	client := &concurrentFakeClient{}
	r := NewRunner(Deps{
		LLMClient:        client,
		Model:            "test-model",
		Template:         tpl,
		Tools:            reg,
		CommentCollector: tool.NewCommentCollector(),
		// MainToolDefs must be non-empty: the fake client classifies a
		// request with no tools as a compression request, mirroring how
		// RunPerFile and runCompression build their ChatRequests.
		MainToolDefs: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "file_read"}}},
		Session:      session.New(t_tempDir, "main", "test-model", session.SessionOptions{ReviewMode: "diff"}),
	})

	// Four files reviewed concurrently over one shared Runner, like the
	// agent/scan fan-out. Each conversation crosses the soft and warning
	// thresholds repeatedly; the real assertion is -race cleanliness.
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			msgs := []llm.Message{msg("system", "sys"), msg("user", "review this file")}
			_, _, err := r.RunPerFile(context.Background(), msgs, fmt.Sprintf("f%d.go", i))
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("file %d: RunPerFile: %v", i, err)
		}
	}
	// Guard against the test going vacuous: if no compression request was
	// ever issued, the conversations never crossed the thresholds and the
	// -race assertion proved nothing.
	if client.compressionCalls.Load() == 0 {
		t.Error("no compression requests were made; the compression path was not exercised")
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

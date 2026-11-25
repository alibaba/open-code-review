// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package reviewtask

import (
	"context"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type relocationLifecycleClient struct {
	started chan struct{}
	release chan struct{}
	meta    llm.RequestMeta
	key     string
	err     error
}

func (client *relocationLifecycleClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	client.meta, _ = llm.RequestMetaFromContext(ctx)
	client.key = llm.SessionKeyFromContext(ctx)
	close(client.started)
	<-client.release
	client.err = ctx.Err()
	content := "Cannot locate this code."
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
		Usage:   &llm.UsageInfo{PromptTokens: 5, CompletionTokens: 7, CacheReadTokens: 3, CacheWriteTokens: 4},
	}, nil
}

func TestCommentLifecycleKeepsKeyWaitsAndRelocationAccounting(t *testing.T) {
	client := &relocationLifecycleClient{started: make(chan struct{}), release: make(chan struct{})}
	pool := NewCommentWorkerPool(2)
	t.Cleanup(func() {
		close(client.release)
		pool.Await()
	})
	deps := newTestDeps(client)
	deps.Session = session.New(t.TempDir(), "main", "fake", session.SessionOptions{})
	deps.CommentWorkerPool = pool
	deps.NewRequestMeta = metaFactory("openai", "fake")
	deps.Tools.Register(&tool.CodeCommentProvider{Collector: deps.CommentCollector})
	deps.Template.ReLocationTask = &template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "Locate {existing_code} in {diff}."}},
	}
	deps.DiffLookup = func(path string) *model.Diff {
		return &model.Diff{NewPath: path, NewFileContent: "package main\n"}
	}
	runner := NewRunner(deps)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checkpoint := runner.executeToolCall(ctx, "group-a", llm.ToolCall{Function: llm.FunctionCall{
		Name: "code_comment", Arguments: `{"comments":[{"path":"a.go","content":"issue a","existing_code":"missing code"}]}`,
	}}, nil, "")
	if checkpoint.Data != tool.CommentSucceed {
		t.Fatalf("comment submission = %+v", checkpoint)
	}
	select {
	case <-client.started:
	case <-time.After(3 * time.Second):
		t.Fatal("relocation did not start")
	}
	cancel()
	runner.executeToolCall(context.Background(), "group-b", llm.ToolCall{Function: llm.FunctionCall{
		Name: "code_comment", Arguments: `{"comments":[{"path":"b.go","content":"issue b","existing_code":"package main"}]}`,
	}}, nil, "")
	drained := make(chan struct{})
	go func() {
		pool.AwaitKey("group-b")
		runner.WaitBackground()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(3 * time.Second):
		t.Fatal("a different key's comment worker blocked the local wait")
	}
	if comments := deps.CommentCollector.Comments(); len(comments) != 1 || comments[0].Path != "b.go" {
		t.Fatalf("local wait returned before its comment was collected: %+v", comments)
	}
	client.release <- struct{}{}
	pool.AwaitKey("group-a")
	if client.err != nil {
		t.Fatalf("async relocation inherited caller cancellation: %v", client.err)
	}
	if client.meta.FilePath != "a.go" || client.meta.TaskType != string(session.ReLocationTask) || client.meta.RequestNo != 1 {
		t.Fatalf("relocation identity = %+v", client.meta)
	}
	if client.key != llm.SessionTaskKey(deps.Session.SessionID, string(session.ReLocationTask), "a.go") {
		t.Fatalf("relocation affinity = %q", client.key)
	}
	if runner.TotalInputTokens() != 5 || runner.TotalOutputTokens() != 7 || runner.TotalCacheReadTokens() != 3 || runner.TotalCacheWriteTokens() != 4 {
		t.Fatal("relocation usage did not reach the shared runtime counters")
	}
	if comments := runner.CollectPendingComments(); len(comments) != 2 {
		t.Fatalf("comments lost at final drain: %+v", comments)
	}
	records := deps.Session.GetOrCreateFileSession("a.go").TaskRecords[session.ReLocationTask]
	if len(records) != 1 || records[0].Response == nil || records[0].Error != "" {
		t.Fatalf("relocation session record not finalized: %+v", records)
	}
}

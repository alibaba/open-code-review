// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// TestScanRun_ReceiptTracksBatchGroups drives a two-batch scan end to end and
// pins the #2124 receipt contract for scan: each batch appears as a group, its
// files flip to done as checkpoints land, and the final receipt carries the
// terminal status.
func TestScanRun_ReceiptTracksBatchGroups(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "a.go", []byte("package a\n"))
	writeFile(t, repo, "b.go", []byte("package b\n"))
	gitCommit(t, repo, "init")

	client := &fakeScanClient{responses: []*llm.ChatResponse{scanTaskDoneResponse(), scanTaskDoneResponse()}}
	sess := session.New(repo, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeFullScan,
	})
	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 1000
	tpl.BatchSize = 1 // force two batches

	a := NewAgent(Args{
		RepoDir:          repo,
		Template:         tpl,
		LLMClient:        client,
		Model:            "test-model",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
			{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
		},
		SkipPlan:       true,
		SkipDedup:      true,
		SkipSummary:    true,
		MaxConcurrency: 1,
		Session:        sess,
	})

	if _, err := a.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	dir, err := session.SessionsDir(repo)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, sess.SessionID+".receipt.json"))
	if err != nil {
		t.Fatalf("read receipt: %v", err)
	}
	var receipt session.Receipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatalf("receipt not valid JSON: %v", err)
	}

	if len(receipt.Groups) != 2 {
		t.Fatalf("groups = %d, want 2 (one per batch)", len(receipt.Groups))
	}
	for i, g := range receipt.Groups {
		if g.GroupID != fmt.Sprintf("batch-%d", i) {
			t.Errorf("group %d id = %q, want batch-%d", i, g.GroupID, i)
		}
		if g.State != "done" || g.CompletedFiles != 1 || len(g.MissingFiles) != 0 {
			t.Errorf("group %s rollup = %+v, want done with 1 completed", g.GroupID, g)
		}
	}
	if receipt.Status != session.ReceiptStatusComplete {
		t.Errorf("status = %q, want complete", receipt.Status)
	}
}

// blockingGateClient blocks the first main-task call until released, so the
// test can cancel the context mid-batch.
type blockingGateClient struct {
	started chan struct{}
	release chan struct{}
}

func (c *blockingGateClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if len(req.Tools) > 0 {
		close(c.started)
		select {
		case <-c.release:
		case <-ctx.Done():
		}
		return nil, context.Canceled
	}
	empty := ""
	return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &empty}}}}, nil
}

// TestScanRun_ReceiptCancelledOnMidBatchCancellation pins the #2124 contract
// for interrupted scans: all batches are installed up front, and a
// mid-batch cancellation finalizes the receipt as cancelled with the
// undispatched batch visibly pending.
func TestScanRun_ReceiptCancelledOnMidBatchCancellation(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "a.go", []byte("package a\n"))
	writeFile(t, repo, "b.go", []byte("package b\n"))
	gitCommit(t, repo, "init")

	client := &blockingGateClient{started: make(chan struct{}), release: make(chan struct{})}
	sess := session.New(repo, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeFullScan,
	})
	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 1000
	tpl.BatchSize = 1

	a := NewAgent(Args{
		RepoDir:          repo,
		Template:         tpl,
		LLMClient:        client,
		Model:            "test-model",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
			{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
		},
		SkipPlan:       true,
		SkipDedup:      true,
		SkipSummary:    true,
		MaxConcurrency: 1,
		Session:        sess,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := a.Run(ctx)
		done <- err
	}()

	<-client.started
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	dir, err := session.SessionsDir(repo)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, sess.SessionID+".receipt.json"))
	if err != nil {
		t.Fatalf("read receipt: %v", err)
	}
	var receipt session.Receipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatalf("receipt not valid JSON: %v", err)
	}
	if receipt.Status != session.ReceiptStatusCancelled {
		t.Errorf("status = %q, want cancelled", receipt.Status)
	}
	if len(receipt.Groups) != 2 {
		t.Fatalf("groups = %d, want 2 (both batches installed up front)", len(receipt.Groups))
	}
	if receipt.Groups[1].State != "pending" {
		t.Errorf("undispatched batch state = %q, want pending", receipt.Groups[1].State)
	}
}

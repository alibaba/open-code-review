// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// cancelOnFirstRequestClient cancels the run from inside its first LLM request.
// That is how a Ctrl-C during dispatch actually arrives: the request in flight
// fails with the context error and the run unwinds through its normal finalize,
// so the label has to come from Run's own boundary rather than from a subtask.
type cancelOnFirstRequestClient struct {
	cancel context.CancelFunc
}

func (c *cancelOnFirstRequestClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	c.cancel()
	return nil, ctx.Err()
}

// TestRunRecordsCancellationInSession keeps full-scan in step with the review
// pipeline: a cancelled scan still finalizes, and its session_end has to say
// why instead of leaving `ocr session list` to guess.
func TestRunRecordsCancellationInSession(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "main.go", []byte("package main\nfunc main() {}\n"))
	gitCommit(t, repo, "init")

	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 100000

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := NewAgent(Args{
		RepoDir:          repo,
		Template:         tpl,
		LLMClient:        &cancelOnFirstRequestClient{cancel: cancel},
		Model:            "test",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MaxConcurrency:   1,
		SkipPlan:         true,
		SkipDedup:        true,
		SkipSummary:      true,
		Session: session.New(repo, "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})

	if _, err := a.Run(ctx); err == nil {
		t.Fatal("a cancelled scan must report an error")
	}

	summaries, err := session.ListSessions(repo)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("got %d sessions, want 1", len(summaries))
	}
	got := summaries[0]
	if got.InterruptedBy != session.InterruptReasonCancelled {
		t.Fatalf("InterruptedBy = %q, want %q", got.InterruptedBy, session.InterruptReasonCancelled)
	}
	if got.Aborted {
		t.Error("a cancelled scan that finalized must not report Aborted")
	}
}

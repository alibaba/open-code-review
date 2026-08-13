// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// identityProbeClient records, for every request it receives, whether the
// context carried a retry-report RequestMeta.
type identityProbeClient struct {
	mu        sync.Mutex
	withMeta  []llm.RequestMeta
	callCount int
	reply     string
}

func (c *identityProbeClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	c.mu.Lock()
	c.callCount++
	if meta, ok := llm.RequestMetaFromContext(ctx); ok {
		c.withMeta = append(c.withMeta, meta)
	}
	c.mu.Unlock()

	reply := c.reply
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}},
		Usage:   &llm.UsageInfo{PromptTokens: 1, CompletionTokens: 1},
	}, nil
}

func (c *identityProbeClient) assertClean(t *testing.T, wantCalls int) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.callCount != wantCalls {
		t.Fatalf("got %d requests, want %d", c.callCount, wantCalls)
	}
	if len(c.withMeta) != 0 {
		t.Errorf("scan requests carried identity %+v, want none", c.withMeta)
	}
}

// newProbeAgent builds a scan Agent whose only non-default piece is the probe
// client, so the assertions describe NewAgent's real wiring.
func newProbeAgent(t *testing.T, tpl template.ScanTemplate, client *identityProbeClient, collector *tool.CommentCollector) *Agent {
	t.Helper()
	if collector == nil {
		collector = tool.NewCommentCollector()
	}
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
	a.currentDate = "2026-08-07 10:00"
	return a
}

// TestScanRequestsCarryNoIdentity is the gate that keeps scan out of the retry
// report. It covers scan's three own request types; the three it reaches through
// the shared llmloop Runner are covered there by the nil-factory cases, which
// this file's sibling in internal/llmloop asserts.
func TestScanRequestsCarryNoIdentity(t *testing.T) {
	t.Run("plan", func(t *testing.T) {
		tpl := makeTemplateWithFullScan()
		tpl.PlanTask = &template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "plan {{current_file_path}} {{file_content}}"}},
		}
		client := &identityProbeClient{reply: `{"summary":"s","checkpoints":[]}`}
		a := newProbeAgent(t, tpl, client, nil)

		a.maybeRunPlan(context.Background(), model.ScanItem{Path: "h.go", Content: "package h\n"}, "rule")
		client.assertClean(t, 1)
	})

	t.Run("project summary", func(t *testing.T) {
		tpl := makeTemplateWithFullScan()
		tpl.ProjectSummaryTask = &template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "summarize {{all_comments}}"}},
		}
		client := &identityProbeClient{reply: "overall summary"}
		a := newProbeAgent(t, tpl, client, nil)

		a.maybeRunProjectSummary(context.Background(), []model.LlmComment{
			{Path: "a.go", Content: "missing error check"},
			{Path: "b.go", Content: "no input validation"},
		})
		client.assertClean(t, 1)
	})

	t.Run("dedup", func(t *testing.T) {
		tpl := makeTemplateWithFullScan()
		tpl.DedupTask = &template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "dedup {{batch_comments}}"}},
		}
		collector := tool.NewCommentCollector()
		collector.Add(model.LlmComment{Path: "a.go", Content: "dup 1"})
		collector.Add(model.LlmComment{Path: "a.go", Content: "dup 2"})
		collector.Add(model.LlmComment{Path: "b.go", Content: "unique"})

		client := &identityProbeClient{reply: `{"groups":[{"members":["c-0","c-1"],"merged_content":"combined"},{"members":["c-2"]}]}`}
		a := newProbeAgent(t, tpl, client, collector)

		a.maybeRunDedup(context.Background(), 0, 0)
		client.assertClean(t, 1)
	})

	t.Run("main task via shared runner", func(t *testing.T) {
		client := &identityProbeClient{reply: "no findings"}
		a := newProbeAgent(t, makeTemplateWithFullScan(), client, nil)

		// executeSubtask drives llmloop.RunPerFile, the code path scan shares
		// with review — so this is the assertion that NewAgent leaves
		// Deps.NewRequestMeta nil.
		if _, _, err := a.executeSubtask(context.Background(), model.ScanItem{
			Path:    "h.go",
			Content: "package h\n",
		}); err != nil {
			t.Fatalf("executeSubtask: %v", err)
		}
		if client.callCount == 0 {
			t.Fatal("expected at least one main_task request")
		}
		client.mu.Lock()
		defer client.mu.Unlock()
		if len(client.withMeta) != 0 {
			t.Errorf("scan main_task carried identity %+v, want none", client.withMeta)
		}
	})
}

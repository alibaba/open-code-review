// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/tool"
)

type progressClient struct {
	started   chan struct{}
	release   <-chan struct{}
	failPath  string
	calls     atomic.Int64
	active    atomic.Int64
	maxActive atomic.Int64
}

func (c *progressClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.calls.Add(1)
	active := c.active.Add(1)
	defer c.active.Add(-1)
	for {
		maxActive := c.maxActive.Load()
		if active <= maxActive || c.maxActive.CompareAndSwap(maxActive, active) {
			break
		}
	}
	if c.started != nil {
		c.started <- struct{}{}
	}
	if c.release != nil {
		select {
		case <-c.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if c.failPath != "" && requestContains(req, c.failPath) {
		return nil, errors.New("test scan failure")
	}
	return scanTaskDoneResponse(), nil
}

func requestContains(req llm.ChatRequest, value string) bool {
	for i := range req.Messages {
		if strings.Contains(req.Messages[i].ExtractText(), value) {
			return true
		}
	}
	return false
}

func newProgressAgent(t *testing.T, client llm.LLMClient, items []model.ScanItem) *Agent {
	t.Helper()
	a := NewAgent(Args{
		Template:         makeTemplateWithFullScan(),
		LLMClient:        client,
		Model:            "test",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MaxConcurrency:   2,
		SkipPlan:         true,
		SkipDedup:        true,
		SkipSummary:      true,
		Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})
	a.items = items
	a.currentDate = "2026-10-12"
	a.args.Tools.Freeze()
	return a
}

type progressOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *progressOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *progressOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func captureScanProgress(t *testing.T, run func() error) (string, error) {
	t.Helper()
	var output progressOutput
	restore := stdout.Swap(&output)
	defer restore()
	err := run()
	return output.String(), err
}

func progressLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "[ocr] scan progress:") {
			lines = append(lines, line)
		}
	}
	return lines
}

func assertProgress(t *testing.T, output string, total, processed int) {
	t.Helper()
	lines := progressLines(output)
	if len(lines) != processed {
		t.Fatalf("progress lines = %d, want %d\noutput:\n%s", len(lines), processed, output)
	}
	for i, line := range lines {
		want := fmt.Sprintf("[ocr] scan progress: %d/%d files processed", i+1, total)
		if line != want {
			t.Fatalf("progress line %d = %q, want %q", i, line, want)
		}
	}
}

func TestScanProgress_ConcurrentMultiBatch(t *testing.T) {
	release := make(chan struct{})
	client := &progressClient{started: make(chan struct{}, 4), release: release}
	items := makeScanItems(4)
	a := newProgressAgent(t, client, items)
	a.args.Template.BatchSize = 2
	a.args.Template.BatchStrategy = string(BatchByLanguage)

	type result struct {
		output string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, err := captureScanProgress(t, func() error {
			_, err := a.dispatchSubtasks(context.Background())
			return err
		})
		done <- result{output: output, err: err}
	}()
	for range 2 {
		waitForProgressSignal(t, client.started)
	}
	close(release)
	var got result
	select {
	case got = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("scan did not finish")
	}
	if got.err != nil {
		t.Fatalf("dispatchSubtasks: %v", got.err)
	}
	if client.maxActive.Load() < 2 {
		t.Fatalf("max concurrent calls = %d, want at least 2", client.maxActive.Load())
	}
	assertProgress(t, got.output, 4, 4)
}

func TestScanProgress_ResumeAndFailure(t *testing.T) {
	items := []model.ScanItem{
		{Path: "cached.go", Content: "package cached\n", LineCount: 1},
		{Path: "ok.go", Content: "package ok\n", LineCount: 1},
		{Path: "fail.go", Content: "package fail\n", LineCount: 1},
	}
	fingerprint := scanItemFingerprint(items[0])
	client := &progressClient{failPath: "fail.go"}
	a := newProgressAgent(t, client, items)
	a.args.Resume = &session.ResumeState{
		SessionID:  "prior-session",
		ReviewMode: session.ReviewModeFullScan,
		Items: map[string]session.ResumeItem{
			fingerprint: {FilePath: items[0].Path, Fingerprint: fingerprint},
		},
	}

	output, err := captureScanProgress(t, func() error {
		_, err := a.dispatchSubtasks(context.Background())
		return err
	})
	if err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	if got := client.calls.Load(); got != 2 {
		t.Fatalf("LLM calls = %d, want 2", got)
	}
	assertProgress(t, output, 3, 3)
}

func TestScanProgress_BudgetSkipsUndispatchedFiles(t *testing.T) {
	client := &fakeBudgetClient{perCallTokens: 50_000}
	a := newProgressAgent(t, client, makeScanItems(10))
	a.args.Template = budgetTestTemplate()
	a.args.MaxConcurrency = 1
	a.args.MaxTokensBudget = 120_000

	output, err := captureScanProgress(t, func() error {
		_, err := a.dispatchSubtasks(context.Background())
		return err
	})
	if err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	processed := int(atomic.LoadInt64(&client.calls))
	if processed == 0 || processed >= len(a.items) {
		t.Fatalf("processed files = %d, want a non-zero partial scan", processed)
	}
	assertProgress(t, output, len(a.items), processed)
}

func TestScanProgress_CancelSkipsUnscheduledFiles(t *testing.T) {
	started := make(chan struct{}, 1)
	client := &progressClient{started: started, release: make(chan struct{})}
	a := newProgressAgent(t, client, makeScanItems(3))
	a.args.MaxConcurrency = 1
	ctx, cancel := context.WithCancel(context.Background())

	type result struct {
		output string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, err := captureScanProgress(t, func() error {
			_, err := a.dispatchSubtasks(ctx)
			return err
		})
		done <- result{output: output, err: err}
	}()
	waitForProgressSignal(t, started)
	cancel()
	var got result
	select {
	case got = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("scan did not finish")
	}
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("dispatchSubtasks error = %v, want context.Canceled", got.err)
	}
	assertProgress(t, got.output, 3, 1)
}

func TestScanProgress_EmptyScanEmitsNothing(t *testing.T) {
	a := newProgressAgent(t, &progressClient{}, nil)
	output, err := captureScanProgress(t, func() error {
		_, err := a.dispatchSubtasks(context.Background())
		return err
	})
	if err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	assertProgress(t, output, 0, 0)
}

func TestScanProgress_QuietWriter(t *testing.T) {
	a := newProgressAgent(t, &progressClient{}, makeScanItems(1))
	restore := stdout.Quiet()
	defer restore()

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
}

func waitForProgressSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(10 * time.Second):
		t.Fatal("scan did not start")
	}
}

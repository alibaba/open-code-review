// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"errors"
	"sync"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/tool"
)

// failFirstScanClient fails the first failCount calls, then answers task_done.
type failFirstScanClient struct {
	mu        sync.Mutex
	failCount int
	calls     int
}

func (c *failFirstScanClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.calls <= c.failCount {
		return nil, errors.New("simulated LLM failure")
	}
	empty := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{
			Content: &empty,
			ToolCalls: []llm.ToolCall{{
				ID: "done", Type: "function",
				Function: llm.FunctionCall{Name: "task_done", Arguments: "{}"},
			}},
		}}},
		Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5},
	}, nil
}

// reviewDurationSamples returns how many samples the run-duration histogram holds.
func reviewDurationSamples(t *testing.T, reader *sdkmetric.ManualReader) uint64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	var n uint64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "ocr.review.duration_seconds" {
				continue
			}
			if h, ok := m.Data.(metricdata.Histogram[int64]); ok {
				for _, dp := range h.DataPoints {
					n += dp.Count
				}
			}
		}
	}
	return n
}

func TestRun_RecordsOneDurationSamplePerRun(t *testing.T) {
	tests := []struct {
		name      string
		failCount int
		wantErr   bool
	}{
		{name: "success", failCount: 0},
		{name: "partial failure", failCount: 1},
		{name: "all files failed", failCount: 1 << 20, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			restore := telemetry.EnableMetricsForTest(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
			defer restore()

			repo := initTestRepo(t)
			writeFile(t, repo, "a.go", []byte("package a\n"))
			writeFile(t, repo, "b.go", []byte("package b\n"))
			gitCommit(t, repo, "init")

			tpl := makeTemplateWithFullScan()
			tpl.MaxTokens = 100000
			a := NewAgent(Args{
				RepoDir:          repo,
				Template:         tpl,
				LLMClient:        &failFirstScanClient{failCount: tt.failCount},
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

			_, err := a.Run(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("Run error = %v, wantErr %v", err, tt.wantErr)
			}
			if got := reviewDurationSamples(t, reader); got != 1 {
				t.Errorf("recorded %d duration samples, want exactly 1", got)
			}
		})
	}
}

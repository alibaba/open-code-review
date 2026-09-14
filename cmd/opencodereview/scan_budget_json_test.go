// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/scan"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// fakeScanBudgetClient finishes every file in one round and reports a fixed
// 50K token usage, so the aggregate budget gate trips deterministically.
type fakeScanBudgetClient struct{}

func (fakeScanBudgetClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{{
					ID:       "1",
					Type:     "function",
					Function: llm.FunctionCall{Name: "task_done", Arguments: "{}"},
				}},
			},
			FinishReason: "tool_calls",
		}},
		Usage: &llm.UsageInfo{PromptTokens: 50_000, TotalTokens: 50_000},
	}, nil
}

// TestScanBudgetJSON is the end-to-end contract for #771: a real *scan.Agent,
// real enumeration, real budget gate, through the shared emitRunResult
// pipeline. BudgetExceeded() was hard-coded false, so summary.budget_exceeded
// never appeared however early the gate cut the run short.
//
// The unlimited case asserts on the RAW capture, not the decoded struct:
// budget_exceeded is omitempty, so false and absent are indistinguishable
// after Unmarshal.
func TestScanBudgetJSON(t *testing.T) {
	cases := []struct {
		name       string
		budget     int64
		want       bool
		wantStatus string
	}{
		// The budget stop must NOT invent a typed status: it stays the
		// ordinary warning-derived one (output.go leaves out.Status alone).
		{name: "budget stop sets budget_exceeded", budget: 120_000, want: true, wantStatus: "completed_with_warnings"},
		{name: "unlimited budget omits the key", budget: 0, want: false, wantStatus: "success"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			repoDir := t.TempDir()
			// Zero-padded so lexical walk order is stable (f10 vs f2).
			for _, name := range []string{"f01.go", "f02.go", "f03.go", "f04.go", "f05.go", "f06.go", "f07.go", "f08.go"} {
				if err := os.WriteFile(filepath.Join(repoDir, name), []byte("package x\n"), 0o600); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}

			ag := scan.NewAgent(scan.Args{
				RepoDir: repoDir,
				Template: template.ScanTemplate{
					MaxTokens:           100000,
					MaxToolRequestTimes: 5,
					MainTask: template.LlmConversation{
						Messages: []template.ChatMessage{
							{Role: "system", Content: "scan"},
							{Role: "user", Content: "review {{file_content}}"},
						},
					},
				},
				LLMClient:        fakeScanBudgetClient{},
				Tools:            tool.NewRegistry(),
				CommentCollector: tool.NewCommentCollector(),
				MaxConcurrency:   1, // serialize so the gate is deterministic
				MaxTokensBudget:  tc.budget,
				Session:          session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: session.ReviewModeFullScan}),
				SkipPlan:         true,
				SkipDedup:        true,
				SkipSummary:      true,
			})

			comments, err := ag.Run(context.Background())
			if err != nil {
				t.Fatalf("scan Run: %v", err)
			}

			raw := captureStdout(t, func() {
				if err := emitRunResult(context.Background(), ag, comments, time.Now(), "json", "developer", nil, nil, os.Stdout, nil); err != nil {
					t.Fatalf("emitRunResult: %v", err)
				}
			})

			var out jsonOutput
			if err := json.Unmarshal([]byte(raw), &out); err != nil {
				t.Fatalf("unmarshal %q: %v", raw, err)
			}
			if out.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", out.Status, tc.wantStatus)
			}
			if out.Summary == nil {
				t.Fatal("summary must be present")
			}
			if got := out.Summary.BudgetExceeded; got != tc.want {
				t.Errorf("summary.budget_exceeded = %v, want %v", got, tc.want)
			}
			if !tc.want && strings.Contains(raw, "budget_exceeded") {
				t.Errorf("unlimited budget must omit budget_exceeded entirely, got %s", raw)
			}

			var warned bool
			for _, w := range out.Warnings {
				if w.Type == "token_budget_reached" {
					warned = true
					break
				}
			}
			if warned != tc.want {
				t.Errorf("token_budget_reached warning present = %v, want %v", warned, tc.want)
			}
		})
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
)

var errUnverifiedPlan = errors.New("NEAR AI response verification failed")

type failedPlanClient struct {
	fakeBudgetClient
	planErr             error
	verificationFailure bool
	failed              atomic.Bool
}

func (c *failedPlanClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if strings.HasPrefix(req.Messages[0].ExtractText(), "PLAN") {
		c.failed.Store(true)
		return nil, c.planErr
	}
	return c.fakeBudgetClient.CompletionsWithCtx(ctx, req)
}
func (c *failedPlanClient) VerificationError() error {
	if c.failed.Load() && c.verificationFailure {
		return c.planErr
	}
	return nil
}

func TestNearAIOptionalPlanFailure(t *testing.T) {
	for _, verificationFailure := range []bool{false, true} {
		status := "request_failed"
		planErr := errors.New("NEAR AI inference failed (http.503)")
		if verificationFailure {
			status, planErr = "failed", errUnverifiedPlan
		}
		t.Run(status, func(t *testing.T) {
			repo := initTestRepo(t)
			writeFile(t, repo, "main.go", []byte("package main\n"))
			gitCommit(t, repo, "fixture")
			tpl := budgetTestTemplate()
			tpl.PlanTask = &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "PLAN {{file_content}}"}}}
			client := &failedPlanClient{planErr: planErr, verificationFailure: verificationFailure}
			a := NewAgent(Args{RepoDir: repo, Template: tpl, LLMClient: client, Model: "test", MaxConcurrency: 1, SkipDedup: true, SkipSummary: true,
				MainToolDefs: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "task_done"}}}})
			_, err := a.Run(context.Background())
			if !client.failed.Load() {
				t.Fatal("optional plan did not run")
			}
			if verificationFailure {
				if !errors.Is(err, errUnverifiedPlan) {
					t.Fatalf("scan hid optional verification failure: %v", err)
				}
			} else if err != nil || atomic.LoadInt64(&client.calls) == 0 {
				t.Fatalf("plan HTTP error prevented main scan: err=%v calls=%d", err, atomic.LoadInt64(&client.calls))
			}
		})
	}
}

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

type unverifiedPlanClient struct {
	fakeBudgetClient
	failed atomic.Bool
}

func (c *unverifiedPlanClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if strings.HasPrefix(req.Messages[0].ExtractText(), "PLAN") {
		c.failed.Store(true)
		return nil, errUnverifiedPlan
	}
	return c.fakeBudgetClient.CompletionsWithCtx(ctx, req)
}
func (c *unverifiedPlanClient) VerificationError() error {
	if c.failed.Load() {
		return errUnverifiedPlan
	}
	return nil
}
func (c *unverifiedPlanClient) VerificationReport() *llm.VerificationReport {
	return &llm.VerificationReport{Status: "failed", Failed: 1}
}

func TestNearAIVerificationFailureInOptionalPlanFailsScan(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "main.go", []byte("package main\n"))
	gitCommit(t, repo, "fixture")
	tpl := budgetTestTemplate()
	tpl.PlanTask = &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "PLAN {{file_content}}"}}}
	client := &unverifiedPlanClient{}
	a := NewAgent(Args{RepoDir: repo, Template: tpl, LLMClient: client, Model: "test", MaxConcurrency: 1, SkipDedup: true, SkipSummary: true,
		MainToolDefs: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "task_done"}}}})
	_, err := a.Run(context.Background())
	if !client.failed.Load() || !errors.Is(err, errUnverifiedPlan) {
		t.Fatalf("scan hid optional verification failure: %v", err)
	}
}

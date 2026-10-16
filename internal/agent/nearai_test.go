// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

var errUnverifiedPlan = errors.New("NEAR AI response verification failed (signature.invalid)")

type unverifiedPlanClient struct{ failed atomic.Bool }

func (c *unverifiedPlanClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if strings.HasPrefix(req.Messages[0].ExtractText(), "PLAN") {
		c.failed.Store(true)
		return nil, errUnverifiedPlan
	}
	return agentTaskDoneResponse(), nil
}
func (c *unverifiedPlanClient) VerificationError() error {
	if c.failed.Load() {
		return errUnverifiedPlan
	}
	return nil
}
func (c *unverifiedPlanClient) VerificationReport() *llm.VerificationReport {
	return &llm.VerificationReport{Status: "failed", Failed: 1, NearAISettings: llm.NearAISettings{E2EE: true, VerificationLevel: "model-tee"}}
}

func TestNearAIVerificationFailureInOptionalPlanFailsRun(t *testing.T) {
	repo := initPreviewRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	client := &unverifiedPlanClient{}
	a := New(Args{RepoDir: repo, Provider: "nearai", LLMClient: client, Model: "test", MaxConcurrency: 1,
		Template: template.Template{MaxTokens: 100000, MaxToolRequestTimes: 5,
			PlanTask: &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "PLAN {{diffs}}"}}},
			MainTask: template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}}},
		},
		MainToolDefs: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "task_done"}}},
	})
	_, err := a.Run(context.Background())
	if !client.failed.Load() || !errors.Is(err, errUnverifiedPlan) {
		t.Fatalf("optional plan failure was hidden: %v", err)
	}
	manifest := a.RunManifest()
	if manifest == nil || manifest.TerminalState != session.StateFailed || manifest.RunFailure == nil || manifest.RunFailure.Classification != session.RunFailureVerification {
		t.Fatalf("manifest: %+v", manifest)
	}
	path, err := session.SessionFilePath(repo, a.session.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"verification":{"status":"failed"`) {
		t.Fatalf("session lost verification result: %s", data)
	}
}

func TestNearAIPolicyChangesRuntimeIdentity(t *testing.T) {
	base := &Agent{args: Args{Model: "test", RuntimeConfig: RuntimeConfig{Verification: "nearai/v1:e2ee=true:verification=model-tee"}}}
	changed := &Agent{args: base.args}
	changed.args.RuntimeConfig.Verification = "nearai/v1:e2ee=false:verification=gateway"
	if base.runtimeConfigSHA256() == changed.runtimeConfigSHA256() {
		t.Fatal("verification policy missing from run identity")
	}
}

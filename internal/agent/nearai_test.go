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

type failedPlanClient struct {
	planErr             error
	verificationFailure bool
	failed              atomic.Bool
	mainCalls           atomic.Int32
}

func (c *failedPlanClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if strings.HasPrefix(req.Messages[0].ExtractText(), "PLAN") {
		c.failed.Store(true)
		return nil, c.planErr
	}
	c.mainCalls.Add(1)
	return agentTaskDoneResponse(), nil
}
func (c *failedPlanClient) VerificationError() error {
	if c.failed.Load() && c.verificationFailure {
		return c.planErr
	}
	return nil
}
func (c *failedPlanClient) VerificationReport() *llm.VerificationReport {
	r := &llm.VerificationReport{Status: "verified", Verified: int(c.mainCalls.Load()), NearAISettings: llm.NearAISettings{E2EE: true, VerificationLevel: "model-tee"}}
	if c.failed.Load() {
		if c.verificationFailure {
			r.Status, r.Failed = "failed", 1
		} else {
			r.Status, r.RequestFailed = "request_failed", 1
		}
	}
	return r
}

func TestNearAIOptionalPlanFailure(t *testing.T) {
	for _, verificationFailure := range []bool{false, true} {
		status := "request_failed"
		planErr := errors.New("NEAR AI inference failed (http.503)")
		if verificationFailure {
			status, planErr = "failed", errUnverifiedPlan
		}
		t.Run(status, func(t *testing.T) {
			repo := initPreviewRepo(t)
			if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0600); err != nil {
				t.Fatal(err)
			}
			client := &failedPlanClient{planErr: planErr, verificationFailure: verificationFailure}
			a := New(Args{RepoDir: repo, Provider: "nearai", LLMClient: client, Model: "test", MaxConcurrency: 1,
				Template: template.Template{MaxTokens: 100000, MaxToolRequestTimes: 5,
					PlanTask: &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "PLAN {{diffs}}"}}},
					MainTask: template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}}},
				},
				MainToolDefs: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "task_done"}}},
			})
			_, err := a.Run(context.Background())
			if !client.failed.Load() {
				t.Fatal("optional plan did not run")
			}
			manifest := a.RunManifest()
			if verificationFailure {
				if !errors.Is(err, errUnverifiedPlan) || manifest == nil || manifest.TerminalState != session.StateFailed || manifest.RunFailure == nil || manifest.RunFailure.Classification != session.RunFailureVerification {
					t.Fatalf("verification failure was hidden: err=%v manifest=%+v", err, manifest)
				}
			} else if err != nil || client.mainCalls.Load() == 0 || manifest == nil || manifest.TerminalState != session.StateComplete || manifest.RunFailure != nil {
				t.Fatalf("plan HTTP error prevented main review: err=%v calls=%d manifest=%+v", err, client.mainCalls.Load(), manifest)
			}
			path, err := session.SessionFilePath(repo, a.session.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), `"verification":{"status":"`+status+`"`) {
				t.Fatalf("session lost verification result: %s", data)
			}
		})
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

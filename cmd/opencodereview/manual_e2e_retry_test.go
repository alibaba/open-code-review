// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build manual_e2e

// Manual end-to-end verification harness for #368 P5. It is excluded from the
// default build: run it explicitly with
//
//	go test -tags manual_e2e -run TestManualE2ERetryReport -v ./cmd/opencodereview/
//
// The harness mirrors executeReview's wiring (loadCommonContext →
// loadLLMRuntime → agent.New → ag.Run → RunManifest → Freeze) against a fake
// Anthropic server so the chain "real SDK retry → observer → boundary
// Finalize → Collector → Freeze → both output exits" can be inspected BEFORE
// any P5 production code exists.
package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// The fake Anthropic server, the fixture repo and the OCR_LLM_* wiring live in
// retry_fake_llm_test.go, shared with the automated end-to-end tests so the
// manual harness cannot drift from what those consider a retryable server.

type manualResult struct {
	report    *llm.RetryReport
	freezeErr error
	manifest  *session.RunManifest
	runErr    error
	sessionID string
	attempts  map[string]int
}

// runManualReview mirrors executeReview's wiring up to the point P5 will hook
// into, and returns everything the P5 output layer will read.
func runManualReview(t *testing.T, srv *fakeLLM) manualResult {
	t.Helper()

	repoDir := retryTestRepo(t)
	startFakeLLM(t, srv)

	cc, err := loadCommonContext(repoDir, "", "", 0, 4, true)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	rt, err := loadLLMRuntime(cc.Template, "", llm.ResolveOptions{})
	if err != nil {
		t.Fatalf("loadLLMRuntime: %v", err)
	}
	if rt.RetryCollector == nil {
		t.Fatal("llmRuntime.RetryCollector is nil; P2 wiring regressed")
	}

	mode := tool.ParseReviewMode("HEAD~1", "HEAD", "")
	fileReader := &tool.FileReader{RepoDir: cc.RepoDir, Mode: mode, Ref: "HEAD", Runner: cc.GitRunner}
	tools := buildToolRegistry(rt.Collector, fileReader)

	ag := agent.New(agent.Args{
		RepoDir:               cc.RepoDir,
		From:                  "HEAD~1",
		To:                    "HEAD",
		ReviewMode:            session.ReviewModeRange,
		Template:              *cc.Template,
		SystemRule:            cc.Resolver,
		FileFilter:            cc.FileFilter,
		LLMClient:             rt.Client,
		Tools:                 tools,
		PlanToolDefs:          rt.PlanToolDefs,
		MainToolDefs:          rt.MainToolDefs,
		CommentCollector:      rt.Collector,
		CommentWorkerPool:     agent.NewCommentWorkerPool(2),
		MaxConcurrency:        2,
		ConcurrentTaskTimeout: 120,
		Model:                 rt.Model,
		Provider:              rt.Provider,
		GitRunner:             cc.GitRunner,
		RuntimeConfig:         rt.RuntimeConfig,
	})

	comments, runErr := ag.Run(context.Background())
	manifest := ag.RunManifest()

	// The frozen run ID is the session's in-memory UUID, not the
	// persistence-gated ag.SessionID().
	runID := ag.Session().SessionID
	report, freezeErr := rt.RetryCollector.Freeze(runID)

	t.Logf("comments=%d runErr=%v", len(comments), runErr)
	t.Logf("ag.Session().SessionID=%q ag.SessionID()=%q", runID, ag.SessionID())
	if manifest != nil {
		t.Logf("manifest.terminal_state=%s", manifest.TerminalState)
	} else {
		t.Log("manifest=nil")
	}

	attempts := srv.attemptCounts()
	t.Logf("real HTTP attempts per file: %v", attempts)

	if freezeErr != nil {
		t.Logf("Freeze error: %v", freezeErr)
	} else if report == nil {
		t.Log("Freeze returned (nil, nil): nothing worth reporting")
	} else {
		raw, _ := json.MarshalIndent(map[string]any{"retry_report": report}, "", "  ")
		t.Logf("frozen retry_report:\n%s", raw)
	}

	return manualResult{
		report:    report,
		freezeErr: freezeErr,
		manifest:  manifest,
		runErr:    runErr,
		sessionID: runID,
		attempts:  attempts,
	}
}

func TestManualE2ERetryReport(t *testing.T) {
	t.Run("clean_first_try_success", func(t *testing.T) {
		res := runManualReview(t, newFakeLLM())
		if res.freezeErr != nil {
			t.Fatalf("Freeze must succeed on a clean run: %v", res.freezeErr)
		}
		if res.report != nil {
			t.Fatalf("clean run must produce no report, got %+v", res.report)
		}
		if res.runErr != nil {
			t.Fatalf("clean run must not fail: %v", res.runErr)
		}
		if res.sessionID == "" {
			t.Fatal("session UUID must be non-empty")
		}
	})

	t.Run("recovered_and_failed", func(t *testing.T) {
		srv := newFakeLLM()
		srv.rateLimitOnce["a.go"] = true
		srv.hardFail["b.go"] = true
		res := runManualReview(t, srv)
		if res.freezeErr != nil {
			t.Fatalf("Freeze: %v", res.freezeErr)
		}
		if res.report == nil {
			t.Fatal("expected a report when one request was rate limited and another failed")
		}
		if res.report.RecoveredRequests == 0 {
			t.Errorf("expected at least one recovered request: %+v", res.report)
		}
		if res.report.FailedRequests == 0 {
			t.Errorf("expected at least one failed request: %+v", res.report)
		}
		if got := res.attempts["a.go"]; got < 2 {
			t.Errorf("SDK should have retried a.go, saw %d HTTP attempts", got)
		}
	})

	t.Run("all_files_fail", func(t *testing.T) {
		srv := newFakeLLM()
		srv.hardFail["a.go"] = true
		srv.hardFail["b.go"] = true
		res := runManualReview(t, srv)
		if res.freezeErr != nil {
			t.Fatalf("Freeze: %v", res.freezeErr)
		}
		if res.report == nil {
			t.Fatal("expected a report when every file failed")
		}
		// This is the exit where emitRunResult and emitFailureUsage
		// can both run, so record which one the report must travel through.
		t.Logf("manifest==nil? %v ; runErr!=nil? %v -> emitRunResult runs: %v",
			res.manifest == nil, res.runErr != nil, res.manifest != nil || res.runErr == nil)
	})
}

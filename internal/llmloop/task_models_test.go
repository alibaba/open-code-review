// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

// TestRun_TaskModelOverrideReachesWire pins the #322 contract at the wire: a
// task_models entry for main_task must change the Model on every request Run
// sends, not just a metadata label.
func TestRun_TaskModelOverrideReachesWire(t *testing.T) {
	var requests []llm.ChatRequest
	client := taskClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
		requests = append(requests, request)
		return testTaskResponse("submit_test_result", `{"value":42}`), nil
	})
	deps := Deps{
		LLMClient:  client,
		Model:      "fake",
		TaskModels: llm.TaskModels{"main_task": "cheap-main"},
		Session:    session.New(t.TempDir(), "main", "fake", session.SessionOptions{}),
	}
	runner := NewRunner(deps, testTask())

	result, err := runner.Run(context.Background(), taskMessages(), "data")
	if err != nil || !result.Completed {
		t.Fatalf("Run = %+v, %v", result, err)
	}
	if len(requests) == 0 {
		t.Fatal("no requests captured")
	}
	for i, req := range requests {
		if req.Model != "cheap-main" {
			t.Fatalf("request %d Model = %q, want override %q", i, req.Model, "cheap-main")
		}
	}
}

// TestRun_UnconfiguredTaskFallsBackToRunModel pins the fallback: a task_models
// map that does not name main_task leaves MAIN on the run model — a partially
// filled map must never change unconfigured tasks.
func TestRun_UnconfiguredTaskFallsBackToRunModel(t *testing.T) {
	var requests []llm.ChatRequest
	client := taskClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
		requests = append(requests, request)
		return testTaskResponse("submit_test_result", `{"value":42}`), nil
	})
	deps := Deps{
		LLMClient:  client,
		Model:      "fake",
		TaskModels: llm.TaskModels{"plan_task": "cheap-plan"},
		Session:    session.New(t.TempDir(), "main", "fake", session.SessionOptions{}),
	}
	runner := NewRunner(deps, testTask())

	if _, err := runner.Run(context.Background(), taskMessages(), "data"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(requests) == 0 {
		t.Fatal("no requests captured")
	}
	for i, req := range requests {
		if req.Model != "fake" {
			t.Fatalf("request %d Model = %q, want run model %q", i, req.Model, "fake")
		}
	}
}

// TestRunCompression_TaskModelOverrideReachesWire covers the aux-task path:
// memory compression rides the memory_compression_task key.
func TestRunCompression_TaskModelOverrideReachesWire(t *testing.T) {
	var requests []llm.ChatRequest
	client := taskClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
		requests = append(requests, request)
		return &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: strPtr("summary")}}},
			Usage:   &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}, nil
	})
	deps := Deps{
		LLMClient:  client,
		Model:      "fake",
		TaskModels: llm.TaskModels{"memory_compression_task": "cheap-compress"},
		Session:    session.New(t.TempDir(), "main", "fake", session.SessionOptions{ReviewMode: "diff"}),
	}
	task := Task{
		MaxContextTokens: 200,
		Compression: CompressionPolicy{
			Messages: []MessageTemplate{{Role: "user", Content: "Summarize: {{context}}"}},
		},
	}
	r := NewRunner(deps, task)

	msgs := []llm.Message{llm.NewTextMessage("system", "sys"), llm.NewTextMessage("user", "prompt")}
	for i := 0; i < 10; i++ {
		msgs = append(msgs, llm.NewTextMessage("assistant", strings.Repeat("word ", 100)))
		msgs = append(msgs, llm.NewTextMessage("tool", strings.Repeat("data ", 50)))
	}
	if _, err := r.runCompression(context.Background(), msgs, "test.go"); err != nil {
		t.Fatalf("runCompression: %v", err)
	}
	if len(requests) == 0 {
		t.Fatal("no compression request captured")
	}
	for i, req := range requests {
		if req.Model != "cheap-compress" {
			t.Fatalf("compression request %d Model = %q, want override", i, req.Model)
		}
	}
}

// TestDepsModelForTask covers the resolver helpers directly: nil map and
// missing-key lookups fall back, a hit wins.
func TestDepsModelForTask(t *testing.T) {
	d := Deps{Model: "run-model"}
	if got := d.ModelForTask(session.MainTask); got != "run-model" {
		t.Fatalf("nil map: got %q, want run model", got)
	}
	d.TaskModels = llm.TaskModels{"main_task": "cheap-main", "plan_task": ""}
	if got := d.ModelForTask(session.MainTask); got != "cheap-main" {
		t.Fatalf("hit: got %q, want override", got)
	}
	if got := d.ModelForTask(session.PlanTask); got != "run-model" {
		t.Fatalf("empty value: got %q, want run model", got)
	}
}

func strPtr(s string) *string { return &s }

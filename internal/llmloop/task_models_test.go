// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// TestRunMainTask_TaskModelOverrideReachesWire pins the #322 contract at the
// wire: a task_models entry for main_task must change the Model on every
// request RunMainTask sends, not just a metadata label.
func TestRunMainTask_TaskModelOverrideReachesWire(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{taskDoneResponse()}}
	deps := newTestDeps(client)
	deps.TaskModels = llm.TaskModels{"main_task": "cheap-main"}
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review this file")}
	completed, _, err := runner.RunMainTask(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done to complete RunMainTask")
	}
	if len(client.requests) == 0 {
		t.Fatal("no requests captured")
	}
	for i, req := range client.requests {
		if req.Model != "cheap-main" {
			t.Fatalf("request %d Model = %q, want override %q", i, req.Model, "cheap-main")
		}
	}
}

// TestRunMainTask_UnconfiguredTaskFallsBackToRunModel pins the fallback: a
// task_models map that does not name main_task leaves MAIN on the run model —
// a partially filled map must never change unconfigured tasks.
func TestRunMainTask_UnconfiguredTaskFallsBackToRunModel(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{taskDoneResponse()}}
	deps := newTestDeps(client)
	deps.TaskModels = llm.TaskModels{"plan_task": "cheap-plan"}
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review this file")}
	if _, _, err := runner.RunMainTask(context.Background(), msgs, "main.go"); err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	for i, req := range client.requests {
		if req.Model != deps.Model {
			t.Fatalf("request %d Model = %q, want run model %q", i, req.Model, deps.Model)
		}
	}
}

// TestRunCompression_TaskModelOverrideReachesWire covers the aux-task path:
// memory compression rides the memory_compression_task key.
func TestRunCompression_TaskModelOverrideReachesWire(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: ptr("summary")}}}, Model: "fake"},
	}}
	sess := session.New(t.TempDir(), "main", "fake", session.SessionOptions{ReviewMode: "diff"})
	deps := Deps{
		LLMClient: client,
		Model:     "fake",
		TaskModels: llm.TaskModels{
			"memory_compression_task": "cheap-compress",
		},
		Template: template.Template{
			MemoryCompressionTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Summarize: {{context}}"}},
			},
			MaxTokens: 50,
		},
		CommentCollector: tool.NewCommentCollector(),
		Session:          sess,
	}
	r := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("system", "sys"), llm.NewTextMessage("user", "prompt")}
	for i := 0; i < 10; i++ {
		msgs = append(msgs, llm.NewTextMessage("assistant", strings.Repeat("word ", 100)))
		msgs = append(msgs, llm.NewTextMessage("tool", strings.Repeat("data ", 50)))
	}
	if _, err := r.runCompression(context.Background(), msgs, "test.go"); err != nil {
		t.Fatalf("runCompression: %v", err)
	}
	if len(client.requests) == 0 {
		t.Fatal("no compression request captured")
	}
	for i, req := range client.requests {
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

func ptr(s string) *string { return &s }

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/telemetry"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// initTraceParentRepo builds a two-commit git repository so the range diff
// provider has real work for diff.parse.
func initTraceParentRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "trace@example.com")
	run("config", "user.name", "Trace Test")
	if err := os.WriteFile(filepath.Join(dir, "alpha.go"), []byte("package main\n\nfunc one() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "base")
	run("checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "alpha.go"), []byte("package main\n\nfunc one() int { return 2 }\n\nfunc two() int { return 3 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "change")
	return dir
}

// setupRecordingTelemetry enables telemetry through the public Init entrypoint,
// then redirects the global tracer provider to an in-process span recorder so
// exported spans can be asserted without any exporter I/O.
func setupRecordingTelemetry(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	t.Setenv("OCR_ENABLE_TELEMETRY", "1")
	telemetry.Init(context.Background())
	recorder := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return recorder
}

// TestRunPipelineSpansDoNotParentToDiffParse guards the trace hierarchy fix for
// #1710: after diff.parse ends, the review pipeline (group subtasks, main loop)
// must not parent to the already-ended parse span.
func TestRunPipelineSpansDoNotParentToDiffParse(t *testing.T) {
	recorder := setupRecordingTelemetry(t)
	repoDir := initTraceParentRepo(t)
	setTestHome(t, t.TempDir())

	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Operation:  session.OperationReview,
	})
	t.Cleanup(func() { _ = sh.Finalize() })

	a := New(Args{
		RepoDir:    repoDir,
		From:       "main",
		To:         "feature",
		ReviewMode: session.ReviewModeRange,
		LLMClient:  manifestFlowClient{},
		Model:      "fake",
		Session:    sh,
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 5,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}},
			},
		},
		MainToolDefs: []llm.ToolDef{{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "task_done",
				Description: "finish the review",
			},
		}},
	})
	a.currentDate = "2026-07-26 12:00"

	if _, err := a.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var diffParseID trace.SpanID
	haveParse := false
	var pipeline []sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		name := span.Name()
		switch {
		case name == "diff.parse":
			diffParseID = span.SpanContext().SpanID()
			haveParse = true
		case strings.HasPrefix(name, "subtask.execute.group."),
			name == "main.loop",
			name == "review_filter.execute",
			name == "plan.execute":
			pipeline = append(pipeline, span)
		}
	}
	if !haveParse {
		t.Fatal("expected a diff.parse span to be exported")
	}
	if len(pipeline) == 0 {
		t.Fatal("expected review pipeline spans (subtask.execute.group.*, main.loop, ...) to be exported")
	}
	for _, span := range pipeline {
		if span.Parent().SpanID() == diffParseID {
			t.Errorf("%s parents to the ended diff.parse span (parent=%s); the review pipeline must not inherit the parse span's context",
				span.Name(), span.Parent().SpanID())
		}
	}
}

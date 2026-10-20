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
	prevTP := otel.GetTracerProvider()
	prevMP := otel.GetMeterProvider()
	prevProp := otel.GetTextMapPropagator()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(tp)
	// Full global-state restore (#1713 review): shutting down the providers
	// Init registered flushes and releases exporter resources, and every
	// global Init replaced goes back to its pre-test value. Shutdown re-arms
	// Init, so a later test in the same process initializes fresh providers
	// instead of inheriting a no-op.
	t.Cleanup(func() {
		_ = telemetry.Shutdown(context.Background())
		otel.SetTracerProvider(prevTP)
		otel.SetMeterProvider(prevMP)
		otel.SetTextMapPropagator(prevProp)
	})
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

	// The review command wraps the agent in a review.run root span; mirror
	// that here so the expected hierarchy can be asserted (#1713 review).
	runCtx, runSpan := telemetry.StartSpan(context.Background(), "review.run")
	if _, err := a.Run(runCtx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	runSpan.End()

	var diffParseID trace.SpanID
	haveParse := false
	var runSpanID trace.SpanID
	haveRun := false
	var pipeline []sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		name := span.Name()
		switch {
		case name == "diff.parse":
			diffParseID = span.SpanContext().SpanID()
			haveParse = true
		case name == "review.run":
			runSpanID = span.SpanContext().SpanID()
			haveRun = true
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
	if !haveRun {
		t.Fatal("expected a review.run span to be exported")
	}
	if len(pipeline) == 0 {
		t.Fatal("expected review pipeline spans (subtask.execute.group.*, main.loop, ...) to be exported")
	}
	// The group subtasks parent directly to the review.run root; main.loop and
	// review_filter.execute run inside their group's subtask span. Both shapes
	// must stay rooted in review.run (transitively), never in the ended
	// diff.parse span.
	rooted := map[string]bool{runSpanID.String(): true}
	for _, span := range recorder.Ended() {
		if strings.HasPrefix(span.Name(), "subtask.execute.group.") &&
			span.Parent().SpanID() == runSpanID {
			rooted[span.SpanContext().SpanID().String()] = true
		}
	}
	for _, span := range pipeline {
		if span.Parent().SpanID() == diffParseID {
			t.Errorf("%s parents to the ended diff.parse span (parent=%s); the review pipeline must not inherit the parse span's context",
				span.Name(), span.Parent().SpanID())
		}
		if !rooted[span.Parent().SpanID().String()] {
			t.Errorf("%s has parent %s; the review pipeline must stay rooted in the review.run span (rooted=%v)",
				span.Name(), span.Parent().SpanID(), rooted)
		}
	}
}

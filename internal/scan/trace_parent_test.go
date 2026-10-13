// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/tool"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type scanTaskDoneClient struct{}

func (scanTaskDoneClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	doneContent := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &doneContent,
				ToolCalls: []llm.ToolCall{{
					ID:   "call_done",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "task_done",
						Arguments: `{"state":"DONE"}`,
					},
				}},
			},
		}},
		Usage: &llm.UsageInfo{PromptTokens: 20, CompletionTokens: 10},
	}, nil
}

// setupRecordingTelemetry enables telemetry through the public Init entrypoint,
// then redirects the global tracer provider to an in-process span recorder.
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
	// global Init replaced goes back to its pre-test value. `initialized`
	// stays true, which only makes later Init calls no-ops — these tests swap
	// the tracer provider themselves, and nothing else in the package relies
	// on Init re-registering.
	t.Cleanup(func() {
		_ = telemetry.Shutdown(context.Background())
		otel.SetTracerProvider(prevTP)
		otel.SetMeterProvider(prevMP)
		otel.SetTextMapPropagator(prevProp)
	})
	return recorder
}

// TestScanSubtaskSpansDoNotParentToEnumerate guards the trace hierarchy fix for
// #1710 on the scan side: after scan.enumerate ends, scan.subtask.* spans and
// post-enumeration events must not parent to the already-ended enumeration span.
func TestScanSubtaskSpansDoNotParentToEnumerate(t *testing.T) {
	recorder := setupRecordingTelemetry(t)
	repo := initTestRepo(t)
	writeFile(t, repo, "main.go", []byte("package main\n\nfunc main() {}\n"))
	gitCommit(t, repo, "init")

	sess := session.New(repo, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeFullScan,
	})
	a := NewAgent(Args{
		RepoDir:          repo,
		Template:         makeTemplateWithFullScan(),
		LLMClient:        scanTaskDoneClient{},
		Model:            "test-model",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
			{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
		},
		SkipPlan:       true,
		SkipDedup:      true,
		SkipSummary:    true,
		MaxConcurrency: 1,
		Session:        sess,
	})

	if _, err := a.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var enumerateSpanID trace.SpanID
	haveEnumerate := false
	var subtasks []sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		name := span.Name()
		switch {
		case name == "scan.enumerate":
			enumerateSpanID = span.SpanContext().SpanID()
			haveEnumerate = true
		case strings.HasPrefix(name, "scan.subtask."):
			subtasks = append(subtasks, span)
		}
	}
	if !haveEnumerate {
		t.Fatal("expected a scan.enumerate span to be exported")
	}
	if len(subtasks) == 0 {
		t.Fatal("expected scan.subtask.* spans to be exported")
	}
	for _, span := range subtasks {
		if span.Parent().SpanID() == enumerateSpanID {
			t.Errorf("%s parents to the ended scan.enumerate span (parent=%s); scan subtasks must not inherit the enumeration span's context",
				span.Name(), span.Parent().SpanID())
		}
	}
}

// TestScanSubtaskSpansParentToRunRoot extends the #1710 guard with the
// affirmative half: under a scan.run root (the span the scan command creates),
// scan.subtask.* spans must parent to that root — not merely avoid the ended
// scan.enumerate span (#1713 review).
func TestScanSubtaskSpansParentToRunRoot(t *testing.T) {
	recorder := setupRecordingTelemetry(t)
	repo := initTestRepo(t)
	writeFile(t, repo, "main.go", []byte("package main\n\nfunc main() {}\n"))
	gitCommit(t, repo, "init")

	sess := session.New(repo, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeFullScan,
	})
	a := NewAgent(Args{
		RepoDir:          repo,
		Template:         makeTemplateWithFullScan(),
		LLMClient:        scanTaskDoneClient{},
		Model:            "test-model",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
			{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
		},
		SkipPlan:       true,
		SkipDedup:      true,
		SkipSummary:    true,
		MaxConcurrency: 1,
		Session:        sess,
	})

	// The scan command wraps the agent in a scan.run root span; mirror that
	// here so the expected hierarchy can be asserted.
	runCtx, runSpan := telemetry.StartSpan(context.Background(), "scan.run")
	if _, err := a.Run(runCtx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	runSpan.End()

	var runSpanID trace.SpanID
	haveRun := false
	var subtasks []sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		name := span.Name()
		switch {
		case name == "scan.run":
			runSpanID = span.SpanContext().SpanID()
			haveRun = true
		case strings.HasPrefix(name, "scan.subtask."):
			subtasks = append(subtasks, span)
		}
	}
	if !haveRun {
		t.Fatal("expected a scan.run span to be exported")
	}
	if len(subtasks) == 0 {
		t.Fatal("expected scan.subtask.* spans to be exported")
	}
	for _, span := range subtasks {
		if span.Parent().SpanID() != runSpanID {
			t.Errorf("%s has parent %s; scan subtasks must parent to the scan.run root (expected %s)",
				span.Name(), span.Parent().SpanID(), runSpanID)
		}
	}
}

// TestScanNoFilesEventParentsToIncomingContext covers the empty-scan path
// (#1713 review): with nothing reviewable, the event.scan.no.files event must
// still fire, and it must parent to the incoming context — not to the already
// ended scan.enumerate span.
func TestScanNoFilesEventParentsToIncomingContext(t *testing.T) {
	recorder := setupRecordingTelemetry(t)
	repo := initTestRepo(t)

	sess := session.New(repo, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeFullScan,
	})
	a := NewAgent(Args{
		RepoDir:          repo,
		Template:         makeTemplateWithFullScan(),
		LLMClient:        scanTaskDoneClient{},
		Model:            "test-model",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
			{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
		},
		SkipPlan:       true,
		SkipDedup:      true,
		SkipSummary:    true,
		MaxConcurrency: 1,
		Session:        sess,
	})

	runCtx, runSpan := telemetry.StartSpan(context.Background(), "scan.run")
	if _, err := a.Run(runCtx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	runSpan.End()

	var runSpanID, enumerateSpanID trace.SpanID
	haveRun, haveEnumerate := false, false
	var noFiles []sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		switch span.Name() {
		case "scan.run":
			runSpanID = span.SpanContext().SpanID()
			haveRun = true
		case "scan.enumerate":
			enumerateSpanID = span.SpanContext().SpanID()
			haveEnumerate = true
		case "event.scan.no.files":
			noFiles = append(noFiles, span)
		}
	}
	if !haveRun || !haveEnumerate {
		t.Fatal("expected scan.run and scan.enumerate spans to be exported")
	}
	if len(noFiles) == 0 {
		t.Fatal("expected an event.scan.no.files span on the empty-scan path")
	}
	for _, span := range noFiles {
		if span.Parent().SpanID() == enumerateSpanID {
			t.Errorf("event.scan.no.files parents to the ended scan.enumerate span (parent=%s); it must parent to the incoming context",
				span.Parent().SpanID())
		}
		if span.Parent().SpanID() != runSpanID {
			t.Errorf("event.scan.no.files has parent %s; it must parent to the incoming scan.run root (expected %s)",
				span.Parent().SpanID(), runSpanID)
		}
	}
}

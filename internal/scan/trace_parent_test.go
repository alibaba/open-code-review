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
	prev := otel.GetTracerProvider()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
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

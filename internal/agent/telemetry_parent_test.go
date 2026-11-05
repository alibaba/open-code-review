// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRunKeepsReviewWorkUnderRunSpan(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousMeterProvider := otel.GetMeterProvider()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OCR_ENABLE_TELEMETRY", "1")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	if !telemetry.Init(context.Background()) {
		t.Fatal("enable telemetry")
	}

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	otel.SetMeterProvider(noop.NewMeterProvider())
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		_ = telemetry.Shutdown(context.Background())
		otel.SetTracerProvider(previousProvider)
		otel.SetMeterProvider(previousMeterProvider)
	})

	repoDir := initPreviewRepo(t)
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write reviewable diff: %v", err)
	}

	a := New(Args{
		RepoDir:   repoDir,
		LLMClient: manifestFlowClient{},
		Model:     "fake",
		Template: template.Template{
			MaxTokens:           1000,
			MaxToolRequestTimes: 5,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}},
			},
		},
		MainToolDefs: []llm.ToolDef{{
			Type:     "function",
			Function: llm.FunctionDef{Name: "task_done", Description: "finish the review"},
		}},
	})

	ctx, runSpan := otel.Tracer("test").Start(context.Background(), "review.run")
	if _, err := a.Run(ctx); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	runSpan.End()

	spans := recorder.Ended()
	run := findEndedSpan(t, spans, "review.run")
	diff := findEndedSpan(t, spans, "diff.parse")
	subtask := findEndedSpanPrefix(t, spans, "subtask.execute.group.")
	started := findEndedSpan(t, spans, "event.review.started")

	if got, want := diff.Parent().SpanID(), run.SpanContext().SpanID(); got != want {
		t.Fatalf("diff.parse parent = %s, want review.run %s", got, want)
	}
	for _, span := range []sdktrace.ReadOnlySpan{subtask, started} {
		if got, want := span.Parent().SpanID(), run.SpanContext().SpanID(); got != want {
			t.Errorf("%s parent = %s, want review.run %s (diff.parse is %s)",
				span.Name(), got, want, diff.SpanContext().SpanID())
		}
	}
}

func findEndedSpan(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	t.Fatalf("span %q not found", name)
	return nil
}

func findEndedSpanPrefix(t *testing.T, spans []sdktrace.ReadOnlySpan, prefix string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range spans {
		if strings.HasPrefix(span.Name(), prefix) {
			return span
		}
	}
	t.Fatalf("span with prefix %q not found", prefix)
	return nil
}

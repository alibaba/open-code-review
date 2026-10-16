// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package telemetry

import (
	"context"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/alibaba/open-code-review/internal/llm"
)

func recordingSpan(t *testing.T) (trace.Span, *tracetest.SpanRecorder) {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	_, span := tp.Tracer("test").Start(context.Background(), "test.span")
	return span, sr
}

func attrs(sr *tracetest.SpanRecorder) map[string]string {
	m := map[string]string{}
	for _, s := range sr.Ended() {
		for _, a := range s.Attributes() {
			m[string(a.Key)] = a.Value.Emit()
		}
	}
	return m
}

func TestSpanContentDisabled(t *testing.T) {
	// helpers must no-op without OCR_TRACE_CONTENT — no panic on nil/noop spans
	span := trace.SpanFromContext(context.Background())
	RecordLLMInput(span, "s", "m", []string{"x"})
	RecordLLMOutput(span, "o", nil, "r", &llm.UsageInfo{TotalTokens: 1})
	RecordToolInput(span, map[string]any{"a": 1})
	RecordToolOutput(span, "ok")
}

func TestSpanContentEnabled(t *testing.T) {
	t.Setenv("OCR_TRACE_CONTENT", "1")
	t.Setenv("OCR_TRACE_NAME", "tr")
	t.Setenv("OCR_TRACE_ENV", "ci")
	t.Setenv("OCR_TRACE_SESSION", "sess-cfg")
	t.Setenv("OCR_TRACE_RELEASE", "rel1")
	t.Setenv("OCR_TRACE_TAGS", "a,b")
	setupEnabledTelemetry(t)

	span, sr := recordingSpan(t)
	RecordLLMInput(span, "sess-rt", "m1", []any{map[string]any{"role": "user", "content": "hi"}})
	RecordLLMOutput(span, "done", []any{map[string]any{"name": "code_comment"}},
		"thinking", &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, CacheReadTokens: 3})
	RecordToolInput(span, map[string]any{"path": "x.go"})
	RecordToolOutput(span, "result")
	applyTraceAttrs(context.Background(), span)
	span.End()

	a := attrs(sr)
	for _, k := range []string{
		"langfuse.observation.type", "langfuse.observation.input", "langfuse.observation.output",
		"langfuse.observation.metadata.reasoning_content", "langfuse.observation.model.name",
		"gen_ai.request.model", "gen_ai.usage.input_tokens", "langfuse.observation.usage_details",
		"langfuse.trace.name", "langfuse.environment", "langfuse.session.id",
		"langfuse.release", "langfuse.tags",
	} {
		if _, ok := a[k]; !ok {
			t.Errorf("missing attribute %q", k)
		}
	}
	if got := a["langfuse.session.id"]; got != "sess-cfg" {
		t.Errorf("expected configured session to win, got %q", got)
	}
	if got := a["langfuse.tags"]; got != `["a","b"]` {
		t.Errorf("expected tags as string array, got %q", got)
	}
	if got := a["langfuse.observation.type"]; got != `tool` {
		t.Errorf("expected tool type, got %q", got)
	}
}

func TestSpanContentSessionFallback(t *testing.T) {
	t.Setenv("OCR_TRACE_CONTENT", "1")
	setupEnabledTelemetry(t)

	span, sr := recordingSpan(t)
	RecordLLMInput(span, "sess-rt", "m1", nil)
	span.End()
	if got := attrs(sr)["langfuse.session.id"]; got != "sess-rt" {
		t.Errorf("expected runtime session fallback, got %q", got)
	}
}

func TestRuntimeSessionPropagatesToSpans(t *testing.T) {
	t.Setenv("OCR_TRACE_CONTENT", "1")
	setupEnabledTelemetry(t)

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx := WithSessionID(context.Background(), "sess-rt")
	_, llmSpan := StartLLMSpan(ctx, "m1")
	RecordLLMInput(llmSpan, "sess-rt", "m1", nil)
	llmSpan.End()

	_, toolSpan := StartToolSpan(ctx, "file_read")
	toolSpan.End()

	found := false
	for _, s := range sr.Ended() {
		if s.Name() != "tool.execute.file_read" {
			continue
		}
		found = true
		for _, a := range s.Attributes() {
			if a.Key == "langfuse.session.id" && a.Value.AsString() == "sess-rt" {
				return
			}
		}
		t.Error("tool span missing langfuse.session.id")
	}
	if !found {
		t.Fatal("tool span not recorded")
	}
}

// Two conversations running in parallel must each keep their own session ID:
// a regression test for the shared-global race where one subtask's session
// overwrote another's and attached tool spans to the wrong session.
func TestConcurrentSessionsDoNotBleed(t *testing.T) {
	t.Setenv("OCR_TRACE_CONTENT", "1")
	setupEnabledTelemetry(t)

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	var wg sync.WaitGroup
	for _, sess := range []string{"sess-a", "sess-b"} {
		wg.Add(1)
		go func(sess string) {
			defer wg.Done()
			ctx := WithSessionID(context.Background(), sess)
			_, llmSpan := StartLLMSpan(ctx, "m1")
			RecordLLMInput(llmSpan, sess, "m1", nil)
			llmSpan.End()
			_, toolSpan := StartToolSpan(ctx, "file_read")
			toolSpan.End()
		}(sess)
	}
	wg.Wait()

	perSession := map[string]int{}
	for _, s := range sr.Ended() {
		var sid string
		for _, a := range s.Attributes() {
			if a.Key == "langfuse.session.id" {
				sid = a.Value.AsString()
			}
		}
		if sid != "sess-a" && sid != "sess-b" {
			t.Errorf("span %q carries unexpected session %q", s.Name(), sid)
			continue
		}
		perSession[sid]++
	}
	for _, sess := range []string{"sess-a", "sess-b"} {
		if perSession[sess] != 2 {
			t.Errorf("expected 2 spans under %q, got %d", sess, perSession[sess])
		}
	}
}

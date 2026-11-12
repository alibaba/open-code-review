// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/telemetry"
)

// setupSessionTelemetry enables telemetry through the real Init path, then
// swaps the global tracer provider for a SpanRecorder so the test can
// inspect the attributes spans were stamped with.
func setupSessionTelemetry(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	t.Setenv("OCR_ENABLE_TELEMETRY", "1")
	t.Setenv("OCR_TRACE_CONTENT", "1")
	if !telemetry.Init(context.Background()) {
		t.Fatal("telemetry.Init returned disabled")
	}
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)
	// Leave telemetry initialized: Init is a one-shot latch and Shutdown does
	// not reset it, so tearing down here would permanently disable telemetry
	// for every later test in this package.
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return sr
}

func spanAttr(s sdktrace.ReadOnlySpan, key string) string {
	for _, a := range s.Attributes() {
		if string(a.Key) == key {
			return a.Value.AsString()
		}
	}
	return ""
}

// TestRunMainTask_InheritsRunSessionID is a regression test for the nested
// overwrite a package-global-free propagation still allowed: Agent.Run
// places the OCR run's session ID on the context, then RunMainTask used to
// stamp a fresh conversation UUID over it, splitting one trace's spans
// across two session IDs (loop spans under the run ID, llm.request and
// tool.execute spans under the UUID). Langfuse treats session.id as
// trace-level identity, so filtering by the run ID silently dropped the
// main generations. Two conversations under one run context must report
// the run's session ID on every span while still tagging each
// conversation's own UUID separately.
func TestRunMainTask_InheritsRunSessionID(t *testing.T) {
	sr := setupSessionTelemetry(t)

	// What Agent.Run places on the context before dispatching subtasks.
	ctx := telemetry.WithSessionID(context.Background(), "run-session-1")

	for _, taskKey := range []string{"a.go", "b.go"} {
		client := &scriptedLLMClient{responses: []*llm.ChatResponse{
			codeCommentResponse("reasoning", "commenting"),
			taskDoneResponse(),
		}}
		r, _ := newThinkingTestRunner(t, client)
		ok, _, err := r.RunMainTask(ctx, []llm.Message{msg("user", "review")}, taskKey)
		if err != nil || !ok {
			t.Fatalf("RunMainTask(%s) = (%v, err %v), want completed", taskKey, ok, err)
		}
	}

	spans := sr.Ended()
	if len(spans) == 0 {
		t.Fatal("no spans recorded")
	}
	conversations := map[string]bool{}
	for _, s := range spans {
		if sid := spanAttr(s, "langfuse.session.id"); sid != "run-session-1" {
			t.Errorf("span %q session.id = %q, want inherited run-session-1", s.Name(), sid)
		}
		if s.Name() == "llm.request" {
			if cid := spanAttr(s, "gen_ai.conversation.id"); cid != "" {
				conversations[cid] = true
			}
		}
	}
	if len(conversations) != 2 {
		t.Errorf("llm.request spans carry %d distinct conversation IDs, want 2 (one per RunMainTask)", len(conversations))
	}
}

// TestRunMainTask_SessionIDFallback verifies a caller that brings no run
// session still gets one stable ID for the whole conversation rather than
// an empty session.
func TestRunMainTask_SessionIDFallback(t *testing.T) {
	sr := setupSessionTelemetry(t)

	client := &scriptedLLMClient{responses: []*llm.ChatResponse{
		codeCommentResponse("reasoning", "commenting"),
		taskDoneResponse(),
	}}
	r, _ := newThinkingTestRunner(t, client)
	ok, _, err := r.RunMainTask(context.Background(), []llm.Message{msg("user", "review")}, "file.go")
	if err != nil || !ok {
		t.Fatalf("RunMainTask = (%v, err %v), want completed", ok, err)
	}

	var want string
	for _, s := range sr.Ended() {
		sid := spanAttr(s, "langfuse.session.id")
		if sid == "" {
			t.Errorf("span %q has no session.id; fallback should stamp the conversation ID", s.Name())
			continue
		}
		if want == "" {
			want = sid
		} else if sid != want {
			t.Errorf("span %q session.id = %q, want %q (one ID per conversation)", s.Name(), sid, want)
		}
	}
}

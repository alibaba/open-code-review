package telemetry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestAnyToAttr(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  interface{}
		want attribute.KeyValue
	}{
		{"string", "k", "hello", attribute.String("k", "hello")},
		{"int", "k", 42, attribute.Int64("k", 42)},
		{"int64", "k", int64(100), attribute.Int64("k", 100)},
		{"bool", "k", true, attribute.Bool("k", true)},
		{"float64", "k", 3.14, attribute.Float64("k", 3.14)},
		{"default fallback", "k", []int{1, 2}, attribute.String("k", fmt.Sprintf("%v", []int{1, 2}))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AnyToAttr(tc.key, tc.val)
			if got != tc.want {
				t.Errorf("AnyToAttr(%q, %v) = %v, want %v", tc.key, tc.val, got, tc.want)
			}
		})
	}
}

func TestStartSpan_Enabled(t *testing.T) {
	setupEnabledTelemetry(t)
	ctx := context.Background()
	newCtx, span := StartSpan(ctx, "test.span")
	defer span.End()
	if newCtx == nil {
		t.Error("expected non-nil context")
	}
	if !span.SpanContext().IsValid() {
		t.Error("expected valid span context when telemetry is enabled")
	}
}

func TestStartSpan_Disabled(t *testing.T) {
	initialized = false
	shutdownFuncs = nil
	defer func() { initialized = false }()

	ctx := context.Background()
	newCtx, span := StartSpan(ctx, "test.span")
	if newCtx != ctx {
		t.Error("expected same context when disabled")
	}
	_ = span
}

func TestStartSpan_WithOptions(t *testing.T) {
	setupEnabledTelemetry(t)
	ctx := context.Background()
	_, span := StartSpan(ctx, "test.span", trace.WithAttributes(attribute.String("key", "val")))
	defer span.End()
	if !span.SpanContext().IsValid() {
		t.Error("expected valid span")
	}
}

func TestEndSpan_NoError(t *testing.T) {
	setupEnabledTelemetry(t)
	_, span := StartSpan(context.Background(), "test.span")
	EndSpan(span, nil)
}

func TestEndSpan_WithError(t *testing.T) {
	setupEnabledTelemetry(t)
	_, span := StartSpan(context.Background(), "test.span")
	EndSpan(span, errors.New("something went wrong"))
}

func TestStartToolSpan_Enabled(t *testing.T) {
	setupEnabledTelemetry(t)
	ctx := context.Background()
	newCtx, span := StartToolSpan(ctx, "file_read")
	defer span.End()
	if newCtx == nil {
		t.Error("expected non-nil context")
	}
	if !span.SpanContext().IsValid() {
		t.Error("expected valid span for tool")
	}
}

func TestGetTracer(t *testing.T) {
	setupEnabledTelemetry(t)
	tracer := getTracer()
	if tracer == nil {
		t.Error("expected non-nil tracer")
	}
}

func TestSetAttr_AllTypes(t *testing.T) {
	setupEnabledTelemetry(t)
	_, span := StartSpan(context.Background(), "test.setattr")
	defer span.End()

	SetAttr(span, "str", "hello")
	SetAttr(span, "int", 42)
	SetAttr(span, "int64", int64(100))
	SetAttr(span, "bool", true)
	SetAttr(span, "float64", 3.14)
	SetAttr(span, "default", []int{1, 2})
}

func TestSetAttr_NilSpan(t *testing.T) {
	SetAttr(nil, "key", "value")
}

func TestRecordToolResult_Success(t *testing.T) {
	setupEnabledTelemetry(t)
	_, span := StartSpan(context.Background(), "test.tool")
	RecordToolResult(span, "file_read", 123, nil)
	span.End()
}

func TestRecordToolResult_Error(t *testing.T) {
	setupEnabledTelemetry(t)
	_, span := StartSpan(context.Background(), "test.tool")
	RecordToolResult(span, "file_read", 456, errors.New("read failed"))
	span.End()
}

func TestRecordToolResult_NilSpan(t *testing.T) {
	RecordToolResult(nil, "tool", 100, nil)
	RecordToolResult(nil, "tool", 100, fmt.Errorf("err"))
}

func TestStartLLMSpan(t *testing.T) {
	setupEnabledTelemetry(t)
	ctx, span := StartLLMSpan(context.Background(), "qwen-max")
	if !span.SpanContext().IsValid() {
		t.Error("expected valid span context")
	}
	if ctx == nil {
		t.Error("expected non-nil context")
	}
	span.End()
}

func TestRecordLLMResult_Success(t *testing.T) {
	setupEnabledTelemetry(t)
	_, span := StartSpan(context.Background(), "test.llm")
	RecordLLMResult(span, 500*time.Millisecond, 1200, nil)
	span.End()
}

func TestRecordLLMResult_Error(t *testing.T) {
	setupEnabledTelemetry(t)
	_, span := StartSpan(context.Background(), "test.llm")
	RecordLLMResult(span, 100*time.Millisecond, 0, errors.New("timeout"))
	span.End()
}

func TestRecordLLMResult_NilSpan(t *testing.T) {
	RecordLLMResult(nil, 100*time.Millisecond, 0, nil)
	RecordLLMResult(nil, 100*time.Millisecond, 0, fmt.Errorf("err"))
}

// A well-formed W3C traceparent: version-traceID-spanID-flags.
const validTraceParent = "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
const wantExtractedTraceID = "0af7651916cd43dd8448eb211c80319c"

func TestContextWithTraceParentFromEnv_Extracts(t *testing.T) {
	setupEnabledTelemetry(t)
	// Init registers TraceContext+Baggage at the global propagator; mirror
	// that here so Extract works without going through the full Init path.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	t.Setenv("TRACEPARENT", validTraceParent)
	ctx := ContextWithTraceParentFromEnv(context.Background())

	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		t.Fatal("expected a valid SpanContext after extracting TRACEPARENT")
	}
	if got := sc.TraceID().String(); got != wantExtractedTraceID {
		t.Errorf("extracted TraceID = %q, want %q", got, wantExtractedTraceID)
	}

	// A span started on this ctx must inherit the upstream trace (child, not root).
	_, span := StartSpan(ctx, "test.child")
	defer span.End()
	if got := span.SpanContext().TraceID().String(); got != wantExtractedTraceID {
		t.Errorf("child span TraceID = %q, want %q (must inherit upstream)", got, wantExtractedTraceID)
	}
}

func TestContextWithTraceParentFromEnv_AbsentOrDisabled(t *testing.T) {
	// Telemetry disabled: must short-circuit and leave ctx with no span context.
	initialized = false
	shutdownFuncs = nil
	t.Setenv("TRACEPARENT", validTraceParent)
	ctx := ContextWithTraceParentFromEnv(context.Background())
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		t.Error("expected no valid SpanContext when telemetry is disabled")
	}

	// Telemetry enabled but TRACEPARENT unset: ctx unchanged (no upstream parent).
	setupEnabledTelemetry(t)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	t.Setenv("TRACEPARENT", "")
	ctx = ContextWithTraceParentFromEnv(context.Background())
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		t.Error("expected no valid SpanContext when TRACEPARENT is unset")
	}
}

func TestContextWithTraceParentFromEnv_Malformed(t *testing.T) {
	setupEnabledTelemetry(t)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	for _, bad := range []string{
		"not-a-traceparent",
		"00-invalidtraceid-invalidspanid-01",
	} {
		t.Run(bad, func(t *testing.T) {
			t.Setenv("TRACEPARENT", bad)
			ctx := ContextWithTraceParentFromEnv(context.Background())
			if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
				t.Errorf("expected no valid SpanContext for malformed TRACEPARENT %q", bad)
			}
		})
	}
}

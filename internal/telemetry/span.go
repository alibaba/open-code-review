package telemetry

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func getTracer() trace.Tracer {
	return otel.GetTracerProvider().Tracer(serviceName)
}

// StartSpan creates a new span from the given context. When telemetry is not enabled,
// it returns a no-op span so callers can safely defer .End().
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if !IsEnabled() {
		return ctx, trace.SpanFromContext(ctx)
	}
	return getTracer().Start(ctx, name, opts...)
}

// ContextWithTraceParentFromEnv extracts W3C traceparent from the TRACEPARENT
// environment variable and returns a context carrying the upstream span context.
// Returns ctx unchanged when telemetry is disabled or the variable is unset.
func ContextWithTraceParentFromEnv(ctx context.Context) context.Context {
	if !IsEnabled() {
		return ctx
	}
	tp := os.Getenv("TRACEPARENT")
	if tp == "" {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier{"traceparent": tp})
}

// TraceIDFromContext returns the hex-encoded trace ID of the span carried by
// ctx, or "" if ctx carries no valid span (e.g. telemetry is disabled).
func TraceIDFromContext(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.TraceID().IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

// EndSpan ends the span and records error status if present.
func EndSpan(span trace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	}
	span.End()
}

// SetAttr sets a single attribute on a span.
func SetAttr(span trace.Span, key string, value interface{}) {
	if span == nil {
		return
	}
	switch v := value.(type) {
	case string:
		span.SetAttributes(attribute.String(key, v))
	case int:
		span.SetAttributes(attribute.Int64(key, int64(v)))
	case int64:
		span.SetAttributes(attribute.Int64(key, v))
	case bool:
		span.SetAttributes(attribute.Bool(key, v))
	case float64:
		span.SetAttributes(attribute.Float64(key, v))
	default:
		span.SetAttributes(attribute.String(key, ""))
	}
}

// StartToolSpan creates a span for a tool execution with standard attributes.
func StartToolSpan(ctx context.Context, toolName string) (context.Context, trace.Span) {
	return StartSpan(ctx, "tool.execute."+toolName,
		trace.WithAttributes(attribute.String("tool.name", toolName)))
}

// RecordToolResult sets the outcome of a tool execution on the span.
func RecordToolResult(span trace.Span, toolName string, durationMs int64, err error) {
	if span == nil {
		return
	}
	SetAttr(span, "tool.duration_ms", durationMs)
	if err != nil {
		SetAttr(span, "tool.status", "error")
		SetAttr(span, "tool.error", err.Error())
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		SetAttr(span, "tool.status", "ok")
	}
}

// StartLLMSpan creates a span for an LLM request with standard attributes.
func StartLLMSpan(ctx context.Context, model string) (context.Context, trace.Span) {
	return StartSpan(ctx, "llm.request",
		trace.WithAttributes(attribute.String("llm.model", model)))
}

// RecordLLMResult sets the outcome of an LLM request on the span.
func RecordLLMResult(span trace.Span, duration time.Duration, totalTokens int64, err error) {
	if span == nil {
		return
	}
	SetAttr(span, "llm.duration_ms", duration.Milliseconds())
	SetAttr(span, "llm.total_tokens", totalTokens)
	if err != nil {
		SetAttr(span, "llm.status", "error")
		SetAttr(span, "llm.error", err.Error())
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		SetAttr(span, "llm.status", "ok")
	}
}

// AnyToAttr converts an arbitrary value to an OTel attribute.KeyValue.
func AnyToAttr(k string, v interface{}) attribute.KeyValue {
	switch val := v.(type) {
	case string:
		return attribute.String(k, val)
	case int:
		return attribute.Int64(k, int64(val))
	case int64:
		return attribute.Int64(k, val)
	case bool:
		return attribute.Bool(k, val)
	case float64:
		return attribute.Float64(k, val)
	default:
		return attribute.String(k, fmt.Sprintf("%v", v))
	}
}

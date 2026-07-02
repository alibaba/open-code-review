package telemetry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func setupEnabledTelemetry(t *testing.T) {
	t.Helper()
	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(resource.Default()))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(resource.Default()))
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	tracerProvider = tp
	meterProvider = mp
	initialized = true
	shutdownFuncs = []func(context.Context) error{
		func(ctx context.Context) error { return tp.Shutdown(ctx) },
		func(ctx context.Context) error { return mp.Shutdown(ctx) },
	}
	initMetricsOnce = false
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
		tracerProvider = nil
		meterProvider = nil
		initialized = false
		shutdownFuncs = nil
		initMetricsOnce = false
	})
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	fn()
	w.Close()

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestEvent_Enabled(t *testing.T) {
	setupEnabledTelemetry(t)
	ctx := context.Background()
	Event(ctx, "test.event", attribute.String("key", "value"))
}

func TestEvent_Disabled(t *testing.T) {
	initialized = false
	shutdownFuncs = nil
	defer func() { initialized = false }()
	Event(context.Background(), "test.event")
}

func TestEvent_NilCtx(t *testing.T) {
	setupEnabledTelemetry(t)
	Event(nil, "test.event") //nolint:staticcheck
}

func TestEventf_Enabled(t *testing.T) {
	setupEnabledTelemetry(t)
	ctx := context.Background()
	Eventf(ctx, "test.eventf", "hello world", attribute.Int("count", 5))
}

func TestErrorEvent_Enabled(t *testing.T) {
	setupEnabledTelemetry(t)
	ctx := context.Background()
	ErrorEvent(ctx, "test.error", errors.New("something failed"), attribute.String("detail", "extra"))
}

func TestErrorEvent_NilErr(t *testing.T) {
	setupEnabledTelemetry(t)
	ctx := context.Background()
	ErrorEvent(ctx, "test.error", nil)
}

func TestErrorEvent_NilCtx(t *testing.T) {
	setupEnabledTelemetry(t)
	ErrorEvent(nil, "test.error", errors.New("fail")) //nolint:staticcheck
}

func TestErrorEvent_Disabled(t *testing.T) {
	initialized = false
	shutdownFuncs = nil
	defer func() { initialized = false }()
	ErrorEvent(context.Background(), "test.error", errors.New("fail"))
}

func TestPhaseEvent_Success(t *testing.T) {
	setupEnabledTelemetry(t)
	ctx := context.Background()
	PhaseEvent(ctx, "scan", "main.go", 500*time.Millisecond, nil)
}

func TestPhaseEvent_WithError(t *testing.T) {
	setupEnabledTelemetry(t)
	ctx := context.Background()
	PhaseEvent(ctx, "scan", "main.go", 500*time.Millisecond, errors.New("parse error"))
}

func TestPrintTraceSummary_WithTokenDetails(t *testing.T) {
	PrintTraceSummary(5, 10, 1000, 200, 1200, 0, 0, 3*time.Second)
}

func TestPrintTraceSummary_WithCacheTokens(t *testing.T) {
	PrintTraceSummary(3, 2, 500, 100, 600, 200, 50, 2*time.Second)
}

func TestPrintTraceSummary_NoTokenDetails(t *testing.T) {
	PrintTraceSummary(2, 1, 0, 0, 500, 0, 0, 1*time.Second)
}

func TestPrintToolCallStarted_WithArgs(t *testing.T) {
	PrintToolCallStarted("file_read", map[string]any{"path": "main.go"})
}

func TestPrintToolCallStarted_NoArgs(t *testing.T) {
	PrintToolCallStarted("list_files", nil)
}

func TestPrintToolCallFinished(t *testing.T) {
	PrintToolCallFinished("file_read", 123*time.Millisecond)
}

func TestPrintToolCallError(t *testing.T) {
	out := captureStderr(t, func() {
		PrintToolCallError("file_read", fmt.Errorf("permission denied"))
	})
	if !strings.Contains(out, "✘ file_read") {
		t.Errorf("expected tool name with X mark, got %q", out)
	}
	if !strings.Contains(out, "permission denied") {
		t.Errorf("expected error message, got %q", out)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		dur  time.Duration
		want string
	}{
		{0, "0s"},
		{1500 * time.Millisecond, "1.5s"},
		{60 * time.Second, "1m0s"},
		{123 * time.Millisecond, "123ms"},
		{2*time.Minute + 30*time.Second, "2m30s"},
	}
	for _, tc := range tests {
		got := FormatDuration(tc.dur)
		if got != tc.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tc.dur, got, tc.want)
		}
	}
}

func TestSummarizeArgs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"nil map", nil, ""},
		{"empty map", map[string]any{}, ""},
		{"path key returns quoted", map[string]any{"path": "foo/bar.go"}, `"foo/bar.go"`},
		{"search key returns quoted", map[string]any{"search": "hello"}, `"hello"`},
		{"query key returns quoted", map[string]any{"query": "world"}, `"world"`},
		{"pattern key returns quoted", map[string]any{"pattern": "*.go"}, `"*.go"`},
		{"generic short value", map[string]any{"foo": "bar"}, "foo=bar"},
		{"long value skipped", map[string]any{"data": string(make([]byte, 60))}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := summarizeArgs(tc.args)
			if got != tc.want {
				t.Errorf("summarizeArgs(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

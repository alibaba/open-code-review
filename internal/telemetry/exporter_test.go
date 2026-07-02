package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/resource"
)

func TestNewStdoutTraceExporter(t *testing.T) {
	exp, err := newStdoutTraceExporter()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exp == nil {
		t.Error("expected non-nil exporter")
	}
}

func TestNewStdoutMetricExporter(t *testing.T) {
	exp, err := newStdoutMetricExporter()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exp == nil {
		t.Error("expected non-nil exporter")
	}
}

func TestInitConsoleProviders(t *testing.T) {
	tracerProvider = nil
	meterProvider = nil
	shutdownFuncs = nil
	defer func() {
		for _, fn := range shutdownFuncs {
			_ = fn(context.Background())
		}
		tracerProvider = nil
		meterProvider = nil
		shutdownFuncs = nil
	}()

	initConsoleProviders(resource.Default())
	if tracerProvider == nil {
		t.Error("expected tracerProvider to be set after initConsoleProviders")
	}
	if meterProvider == nil {
		t.Error("expected meterProvider to be set after initConsoleProviders")
	}
	if len(shutdownFuncs) != 2 {
		t.Errorf("expected 2 shutdown funcs, got %d", len(shutdownFuncs))
	}
}

func TestInitOTLPProviders_InvalidEndpoint(t *testing.T) {
	tracerProvider = nil
	meterProvider = nil
	shutdownFuncs = nil
	defer func() {
		for _, fn := range shutdownFuncs {
			_ = fn(context.Background())
		}
		tracerProvider = nil
		meterProvider = nil
		shutdownFuncs = nil
	}()

	cfg := Config{
		Exporter:     "otlp",
		OTLPEndpoint: "localhost:0",
	}
	initOTLPProviders(context.Background(), resource.Default(), cfg)
	if tracerProvider == nil {
		t.Error("expected tracerProvider to be set (OTLP exporter creation is lazy)")
	}
}

package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/resource"
)

func TestParseOTLPEndpoint(t *testing.T) {
	cases := []struct {
		name         string
		endpoint     string
		wantAddr     string
		wantInsecure bool
	}{
		{"http scheme strips and is insecure", "http://192.0.2.1:4317", "192.0.2.1:4317", true},
		{"https scheme strips and keeps TLS", "https://otel.example.com:4317", "otel.example.com:4317", false},
		{"bare host:port unchanged and keeps TLS", "localhost:4317", "localhost:4317", false},
		{"uppercase HTTP scheme strips and is insecure", "HTTP://192.0.2.1:4317", "192.0.2.1:4317", true},
		{"mixed-case Https scheme strips and keeps TLS", "Https://otel.example.com:4317", "otel.example.com:4317", false},
		{"endpoint shorter than scheme prefix is unchanged", "ht", "ht", false},
		{"http scheme with trailing slash is trimmed", "http://192.0.2.1:4317/", "192.0.2.1:4317", true},
		{"https scheme with trailing slash is trimmed", "https://otel.example.com:4317/", "otel.example.com:4317", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr, insecure := parseOTLPEndpoint(tc.endpoint)
			if addr != tc.wantAddr {
				t.Errorf("addr = %q, want %q", addr, tc.wantAddr)
			}
			if insecure != tc.wantInsecure {
				t.Errorf("insecure = %v, want %v", insecure, tc.wantInsecure)
			}
		})
	}
}

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

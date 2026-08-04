package telemetry

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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

func TestInitOTLPHTTPProviders_InvalidEndpoint(t *testing.T) {
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
		OTLPProtocol: "http/protobuf",
	}
	initOTLPHTTPProviders(context.Background(), resource.Default(), cfg)
	if tracerProvider == nil {
		t.Error("expected tracerProvider to be set (OTLP HTTP exporter creation is lazy)")
	}
}

func TestInitOTLPProviders_ProtocolRouting(t *testing.T) {
	cases := []struct {
		name        string
		protocol    string
		wantWarning bool // default branch emits a gRPC fallback warning
	}{
		{"grpc default", "grpc", false},
		{"empty defaults to grpc", "", false},
		{"http/protobuf routes to http", "http/protobuf", false},
		{"http/json routes to http", "http/json", false},
		{"unknown falls back to grpc", "foo", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tracerProvider = nil
			meterProvider = nil
			shutdownFuncs = nil

			// Capture stderr to assert on the fallback warning.
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			cfg := Config{
				Exporter:     "otlp",
				OTLPEndpoint: "localhost:0",
				OTLPProtocol: tc.protocol,
			}
			initOTLPProviders(context.Background(), resource.Default(), cfg)
			if err := w.Close(); err != nil {
				t.Fatalf("close stderr pipe: %v", err)
			}
			os.Stderr = oldStderr

			defer func() {
				for _, fn := range shutdownFuncs {
					_ = fn(context.Background())
				}
				tracerProvider = nil
				meterProvider = nil
				shutdownFuncs = nil
			}()

			if tracerProvider == nil {
				t.Error("expected tracerProvider to be set")
			}
			if meterProvider == nil {
				t.Error("expected meterProvider to be set")
			}
			if len(shutdownFuncs) != 2 {
				t.Errorf("expected 2 shutdown funcs, got %d", len(shutdownFuncs))
			}

			var buf bytes.Buffer
			if _, err := io.Copy(&buf, r); err != nil {
				t.Fatalf("read stderr pipe: %v", err)
			}
			stderrOut := buf.String()
			if tc.wantWarning {
				if !strings.Contains(stderrOut, "falling back to gRPC") {
					t.Errorf("expected gRPC fallback warning in stderr, got %q", stderrOut)
				}
			} else {
				if strings.Contains(stderrOut, "falling back to gRPC") {
					t.Errorf("expected no fallback warning, got %q", stderrOut)
				}
			}
		})
	}
}

func TestOTLPSignalURL(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		signal   string
		want     string
	}{
		{"http scheme keeps its scheme", "http://192.0.2.1:4318", "/v1/traces", "http://192.0.2.1:4318/v1/traces"},
		{"https scheme keeps its scheme", "https://otel.example.com:4318", "/v1/traces", "https://otel.example.com:4318/v1/traces"},
		{"bare host:port gains https to keep TLS", "localhost:4318", "/v1/traces", "https://localhost:4318/v1/traces"},
		{"a base path is preserved", "http://langfuse:3000/api/public/otel", "/v1/traces", "http://langfuse:3000/api/public/otel/v1/traces"},
		{"a trailing slash does not double up", "http://192.0.2.1:4318/", "/v1/traces", "http://192.0.2.1:4318/v1/traces"},
		{"an endpoint already naming the signal is left alone", "http://192.0.2.1:4318/v1/traces", "/v1/traces", "http://192.0.2.1:4318/v1/traces"},
		{"metrics get their own path", "http://192.0.2.1:4318", "/v1/metrics", "http://192.0.2.1:4318/v1/metrics"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := otlpSignalURL(tc.endpoint, tc.signal); got != tc.want {
				t.Errorf("otlpSignalURL(%q, %q) = %q, want %q", tc.endpoint, tc.signal, got, tc.want)
			}
		})
	}
}

// TestInitOTLPHTTPProviders_HonoursBasePath is the regression test for the bug this
// exporter had: WithEndpoint takes a bare host:port, so a base path in
// OTEL_EXPORTER_OTLP_ENDPOINT was discarded and every span went to /v1/traces at the
// root of the host. Backends addressed through a path prefix therefore answered 404
// and no telemetry arrived, with nothing in the exporter to say why.
//
// The assertion is on the request path the collector actually receives, because that is
// the thing that was wrong — a unit test of the URL helper alone would not have caught
// it, since the helper did not exist and the discarding happened in the exporter option.
//
// It also covers the TLS decision that moved into WithEndpointURL. httptest.NewServer
// speaks plaintext and its URL carries an http:// scheme, so the request only arrives if
// Insecure was resolved to true from that scheme — which is what the explicit
// WithInsecure call used to do. A regression there shows up here as a timeout rather
// than as a wrong path.
func TestInitOTLPHTTPProviders_HonoursBasePath(t *testing.T) {
	const basePath = "/api/public/otel"

	gotPath := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case gotPath <- r.URL.Path:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

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
		OTLPEndpoint: srv.URL + basePath,
		OTLPProtocol: "http/protobuf",
	}
	initOTLPHTTPProviders(context.Background(), resource.Default(), cfg)
	if tracerProvider == nil {
		t.Fatal("tracerProvider was not set")
	}

	_, span := tracerProvider.Tracer("test").Start(context.Background(), "probe")
	span.End()
	if err := tracerProvider.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush() = %v", err)
	}

	select {
	case path := <-gotPath:
		if want := basePath + "/v1/traces"; path != want {
			t.Errorf("collector received %q, want %q", path, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the collector was never called; the endpoint's base path was discarded")
	}
}

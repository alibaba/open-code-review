package telemetry

import (
	"context"
	"fmt"
	"os"
	"strings"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	otlpmetricgrpc "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otlpmetrichttp "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	otlptracegrpc "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otlptracehttp "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	stdoutmetric "go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	stdouttrace "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
)

// newStdoutTraceExporter creates a console trace exporter with pretty-print output.
func newStdoutTraceExporter() (*stdouttrace.Exporter, error) {
	return stdouttrace.New(stdouttrace.WithPrettyPrint())
}

// newStdoutMetricExporter creates a console metric exporter with pretty-print output.
func newStdoutMetricExporter() (sdkmetric.Exporter, error) {
	return stdoutmetric.New(stdoutmetric.WithPrettyPrint())
}

// parseOTLPEndpoint strips a http:// or https:// scheme from the endpoint and
// reports whether the connection should be plaintext (insecure).
// Scheme matching is case-insensitive per RFC 3986. A bare host:port (no
// scheme) is left unchanged and defaults to TLS. Any trailing slash left
// over from a URL-style endpoint (e.g. "http://localhost:4317/") is
// trimmed, since both gRPC and HTTP exporters' WithEndpoint expects a bare
// host:port with no path.
func parseOTLPEndpoint(endpoint string) (addr string, insecure bool) {
	switch {
	case len(endpoint) >= 7 && strings.EqualFold(endpoint[:7], "http://"):
		return strings.TrimRight(endpoint[7:], "/"), true
	case len(endpoint) >= 8 && strings.EqualFold(endpoint[:8], "https://"):
		return strings.TrimRight(endpoint[8:], "/"), false
	default:
		return endpoint, false
	}
}

// initOTLPProviders dispatches to the gRPC or HTTP exporter based on cfg.OTLPProtocol.
func initOTLPProviders(ctx context.Context, res *resource.Resource, cfg Config) {
	switch cfg.OTLPProtocol {
	case "http/protobuf", "http/json":
		initOTLPHTTPProviders(ctx, res, cfg)
	case "", "grpc":
		initOTLPGRPCProviders(ctx, res, cfg)
	default:
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: unsupported OTLP protocol %q, falling back to gRPC\n", cfg.OTLPProtocol)
		initOTLPGRPCProviders(ctx, res, cfg)
	}
}

// initOTLPGRPCProviders sets up OTLP gRPC exporters for traces and metrics.
func initOTLPGRPCProviders(ctx context.Context, res *resource.Resource, cfg Config) {
	addr, insecure := parseOTLPEndpoint(cfg.OTLPEndpoint)

	traceOpts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(addr)}
	if insecure {
		traceOpts = append(traceOpts, otlptracegrpc.WithInsecure())
	}
	traceExp, err := otlptracegrpc.New(ctx, traceOpts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: failed to create OTLP trace exporter: %v\n", err)
		return
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	tracerProvider = tp
	shutdownFuncs = append(shutdownFuncs, func(ctx context.Context) error { return tp.Shutdown(ctx) })

	metricOpts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(addr)}
	if insecure {
		metricOpts = append(metricOpts, otlpmetricgrpc.WithInsecure())
	}
	metricExp, err := otlpmetricgrpc.New(ctx, metricOpts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: failed to create OTLP metric exporter: %v\n", err)
		return
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	meterProvider = mp
	shutdownFuncs = append(shutdownFuncs, func(ctx context.Context) error { return mp.Shutdown(ctx) })
}

// initOTLPHTTPProviders sets up OTLP HTTP exporters for traces and metrics.
func initOTLPHTTPProviders(ctx context.Context, res *resource.Resource, cfg Config) {
	addr, insecure := parseOTLPEndpoint(cfg.OTLPEndpoint)

	traceOpts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(addr)}
	if insecure {
		traceOpts = append(traceOpts, otlptracehttp.WithInsecure())
	}
	traceExp, err := otlptracehttp.New(ctx, traceOpts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: failed to create OTLP HTTP trace exporter: %v\n", err)
		return
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	tracerProvider = tp
	shutdownFuncs = append(shutdownFuncs, func(ctx context.Context) error { return tp.Shutdown(ctx) })

	metricOpts := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpoint(addr)}
	if insecure {
		metricOpts = append(metricOpts, otlpmetrichttp.WithInsecure())
	}
	metricExp, err := otlpmetrichttp.New(ctx, metricOpts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: failed to create OTLP HTTP metric exporter: %v\n", err)
		return
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	meterProvider = mp
	shutdownFuncs = append(shutdownFuncs, func(ctx context.Context) error { return mp.Shutdown(ctx) })
}

// initConsoleProviders sets up stdout exporters for traces and metrics.
func initConsoleProviders(res *resource.Resource) {
	traceExp, err := newStdoutTraceExporter()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: failed to create console trace exporter: %v\n", err)
		return
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	tracerProvider = tp
	shutdownFuncs = append(shutdownFuncs, func(ctx context.Context) error { return tp.Shutdown(ctx) })

	metricExp, err := newStdoutMetricExporter()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: failed to create console metric exporter: %v\n", err)
		return
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	meterProvider = mp
	shutdownFuncs = append(shutdownFuncs, func(ctx context.Context) error { return mp.Shutdown(ctx) })
}

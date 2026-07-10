package telemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Global OTel providers. Set by Init, used via otel.GetTracerProvider() / otel.GetMeterProvider().
var (
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	shutdownFuncs  []func(context.Context) error
	initialized    bool
)

// serviceName holds the name set during Init.
var serviceName = "open-code-review"

// Init initializes global TracerProvider and MeterProvider based on
// environment variables and optional config file. Returns true when enabled.
// Safe to call multiple times.
func Init(ctx context.Context) bool {
	if initialized {
		return len(shutdownFuncs) > 0
	}
	initialized = true

	cfg := ResolveConfig(HomeConfigPath())
	serviceName = cfg.ServiceName
	if !cfg.Enabled {
		return false
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithHost(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: failed to create OTel resource: %v\n", err)
		res = resource.Default()
	}

	switch cfg.Exporter {
	case "otlp":
		initOTLPProviders(ctx, res, cfg)
	default:
		initConsoleProviders(res)
	}

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return len(shutdownFuncs) > 0
}

// IsEnabled returns true when telemetry has been initialized with exporters.
func IsEnabled() bool {
	return initialized && len(shutdownFuncs) > 0
}

// ContentLogging returns true when content logging is enabled via OCR_CONTENT_LOGGING.
func ContentLogging() bool {
	if !IsEnabled() {
		return false
	}
	cfg := ResolveConfig("")
	return cfg.ContentLog
}

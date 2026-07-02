package telemetry

import (
	"context"
	"os"
	"testing"
)

func TestIsEnabled_NotInitialized(t *testing.T) {
	// Reset global state
	initialized = false
	shutdownFuncs = nil

	if IsEnabled() {
		t.Error("expected IsEnabled()=false when not initialized")
	}
}

func TestIsEnabled_InitializedButNoShutdowns(t *testing.T) {
	initialized = true
	shutdownFuncs = nil
	defer func() {
		initialized = false
	}()

	if IsEnabled() {
		t.Error("expected IsEnabled()=false when no shutdown funcs registered")
	}
}

func TestIsEnabled_WithShutdowns(t *testing.T) {
	initialized = true
	shutdownFuncs = []func(context.Context) error{
		func(ctx context.Context) error { return nil },
	}
	defer func() {
		initialized = false
		shutdownFuncs = nil
	}()

	if !IsEnabled() {
		t.Error("expected IsEnabled()=true when shutdown funcs registered")
	}
}

func TestContentLogging_Disabled(t *testing.T) {
	initialized = false
	shutdownFuncs = nil

	if ContentLogging() {
		t.Error("expected ContentLogging()=false when telemetry disabled")
	}
}

func TestContentLogging_EnabledWithEnv(t *testing.T) {
	setupEnabledTelemetry(t)
	t.Setenv("OCR_CONTENT_LOGGING", "1")
	if !ContentLogging() {
		t.Error("expected ContentLogging()=true when enabled and env var set")
	}
}

func TestContentLogging_EnabledWithoutEnv(t *testing.T) {
	setupEnabledTelemetry(t)
	t.Setenv("OCR_CONTENT_LOGGING", "")
	os.Unsetenv("OCR_CONTENT_LOGGING")
	if ContentLogging() {
		t.Error("expected ContentLogging()=false when enabled but env var not set")
	}
}

func TestInit_AlreadyInitialized(t *testing.T) {
	initialized = true
	shutdownFuncs = nil
	defer func() {
		initialized = false
		shutdownFuncs = nil
	}()

	result := Init(context.Background())
	if result {
		t.Error("expected false when already initialized with no shutdown funcs")
	}
}

func TestInit_AlreadyInitializedWithShutdowns(t *testing.T) {
	initialized = true
	shutdownFuncs = []func(context.Context) error{
		func(ctx context.Context) error { return nil },
	}
	defer func() {
		initialized = false
		shutdownFuncs = nil
	}()

	result := Init(context.Background())
	if !result {
		t.Error("expected true when already initialized with shutdown funcs")
	}
}

func TestInit_DisabledByDefault(t *testing.T) {
	initialized = false
	shutdownFuncs = nil
	tracerProvider = nil
	meterProvider = nil

	envKeys := []string{
		"OCR_ENABLE_TELEMETRY", "OTEL_SERVICE_NAME",
		"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_PROTOCOL",
		"OCR_CONTENT_LOGGING",
	}
	for _, k := range envKeys {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}

	defer func() {
		initialized = false
		shutdownFuncs = nil
		tracerProvider = nil
		meterProvider = nil
	}()

	result := Init(context.Background())
	if result {
		t.Error("expected false when telemetry is not enabled via env")
	}
}

func TestInit_EnabledConsole(t *testing.T) {
	initialized = false
	shutdownFuncs = nil
	tracerProvider = nil
	meterProvider = nil

	t.Setenv("OCR_ENABLE_TELEMETRY", "1")
	envKeys := []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_PROTOCOL",
	}
	for _, k := range envKeys {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}

	defer func() {
		for _, fn := range shutdownFuncs {
			_ = fn(context.Background())
		}
		initialized = false
		shutdownFuncs = nil
		tracerProvider = nil
		meterProvider = nil
	}()

	result := Init(context.Background())
	if !result {
		t.Error("expected true when telemetry is enabled")
	}
	if tracerProvider == nil {
		t.Error("expected tracerProvider to be set")
	}
	if meterProvider == nil {
		t.Error("expected meterProvider to be set")
	}
}

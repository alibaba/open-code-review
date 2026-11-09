// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
)

func TestShutdown_ReinitializeMetrics(t *testing.T) {
	ctx := context.Background()
	previousTracer := otel.GetTracerProvider()
	previousMeter := otel.GetMeterProvider()
	previousPropagator := otel.GetTextMapPropagator()
	previousServiceName := serviceName
	initialized = false
	shutdownFuncs = nil
	initMetricsOnce = false

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("OCR_ENABLE_TELEMETRY", "1")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "")
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "3600000")
	t.Cleanup(func() {
		if err := Shutdown(ctx); err != nil {
			t.Errorf("cleanup telemetry: %v", err)
		}
		otel.SetTracerProvider(previousTracer)
		otel.SetMeterProvider(previousMeter)
		otel.SetTextMapPropagator(previousPropagator)
		tracerProvider = nil
		meterProvider = nil
		serviceName = previousServiceName
		initialized = false
		shutdownFuncs = nil
		initMetricsOnce = false
	})

	for _, expected := range []int64{3, 5} {
		output, err := os.CreateTemp(t.TempDir(), "metrics-*.json")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = output.Close() })
		func() {
			previousStdout := os.Stdout
			os.Stdout = output
			defer func() { os.Stdout = previousStdout }()
			if !Init(ctx) {
				t.Fatal("expected telemetry to initialize")
			}
		}()

		RecordFilesReviewed(ctx, expected)
		if err := Shutdown(ctx); err != nil {
			t.Fatalf("shutdown telemetry: %v", err)
		}
		data, err := os.ReadFile(output.Name())
		if err != nil {
			t.Fatal(err)
		}
		var exported struct {
			ScopeMetrics []struct {
				Metrics []struct {
					Name string
					Data struct {
						DataPoints []struct {
							Value int64
						}
					}
				}
			}
		}
		if err := json.Unmarshal(data, &exported); err != nil {
			t.Fatalf("decode exported metrics: %v", err)
		}
		found := false
		for _, scope := range exported.ScopeMetrics {
			for _, metric := range scope.Metrics {
				if metric.Name != "ocr.files_reviewed_total" {
					continue
				}
				found = true
				if len(metric.Data.DataPoints) != 1 {
					t.Fatalf("expected one files-reviewed data point, got %d", len(metric.Data.DataPoints))
				}
				if actual := metric.Data.DataPoints[0].Value; actual != expected {
					t.Errorf("exported files reviewed = %d, want %d", actual, expected)
				}
			}
		}
		if !found {
			t.Fatalf("expected files-reviewed metric with value %d after shutdown", expected)
		}
	}
}

func TestShutdown_NoFuncs(t *testing.T) {
	shutdownFuncs = nil
	err := Shutdown(context.Background())
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestShutdown_Success(t *testing.T) {
	called := false
	shutdownFuncs = []func(context.Context) error{
		func(ctx context.Context) error {
			called = true
			return nil
		},
	}
	defer func() { shutdownFuncs = nil }()

	err := Shutdown(context.Background())
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if !called {
		t.Error("expected shutdown function to be called")
	}
	if shutdownFuncs != nil {
		t.Error("expected shutdownFuncs to be cleared after shutdown")
	}
}

func TestShutdown_WithErrors(t *testing.T) {
	initialized = true
	initMetricsOnce = true
	shutdownFuncs = []func(context.Context) error{
		func(ctx context.Context) error { return nil },
		func(ctx context.Context) error { return errors.New("fail1") },
		func(ctx context.Context) error { return errors.New("fail2") },
	}
	defer func() {
		shutdownFuncs = nil
		initialized = false
		initMetricsOnce = false
	}()

	err := Shutdown(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if shutdownFuncs != nil {
		t.Error("expected shutdownFuncs to be cleared even on error")
	}
	if initialized || initMetricsOnce {
		t.Error("expected provider and metric initialization to reset even on error")
	}
}

func TestShutdownWithTimeout(t *testing.T) {
	called := false
	shutdownFuncs = []func(context.Context) error{
		func(ctx context.Context) error {
			called = true
			return nil
		},
	}
	defer func() { shutdownFuncs = nil }()

	ShutdownWithTimeout(context.Background(), 5*time.Second)
	if !called {
		t.Error("expected shutdown function to be called via ShutdownWithTimeout")
	}
}

func TestShutdownWithTimeout_Error(t *testing.T) {
	shutdownFuncs = []func(context.Context) error{
		func(ctx context.Context) error { return errors.New("shutdown fail") },
	}
	defer func() { shutdownFuncs = nil }()

	ShutdownWithTimeout(context.Background(), 5*time.Second)
}

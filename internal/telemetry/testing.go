// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// EnableMetricsForTest routes metric recording to mp until the returned
// restore function is called. It lets tests in other packages assert which
// metrics a code path records; production code enables telemetry via Init.
func EnableMetricsForTest(mp metric.MeterProvider) (restore func()) {
	prevInitialized, prevShutdown := initialized, shutdownFuncs
	initialized = true
	shutdownFuncs = []func(context.Context) error{func(context.Context) error { return nil }}
	otel.SetMeterProvider(mp)
	initMetricsOnce = false
	return func() {
		initialized, shutdownFuncs = prevInitialized, prevShutdown
		otel.SetMeterProvider(noop.NewMeterProvider())
		initMetricsOnce = false
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package telemetry

import (
	"context"
	"testing"
)

// TestTraceIDFromContext_Empty covers the invalid-span branch: a bare context
// carries no span, so an empty string is returned.
func TestTraceIDFromContext_Empty(t *testing.T) {
	if got := TraceIDFromContext(context.Background()); got != "" {
		t.Errorf("TraceIDFromContext(bare ctx) = %q, want empty", got)
	}
}

// TestTraceIDFromContext_Valid covers the valid-span branch: a context carrying
// an active span reports its hex-encoded trace ID.
func TestTraceIDFromContext_Valid(t *testing.T) {
	setupEnabledTelemetry(t)
	ctx, span := StartSpan(context.Background(), "test.traceid")
	defer span.End()

	got := TraceIDFromContext(ctx)
	if got == "" {
		t.Fatal("TraceIDFromContext with active span returned empty")
	}
	if want := span.SpanContext().TraceID().String(); got != want {
		t.Errorf("TraceIDFromContext = %q, want %q", got, want)
	}
}

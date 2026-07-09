package telemetry

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestRecordFunctions_DisabledTelemetry(t *testing.T) {
	// Reset state so telemetry is disabled
	initialized = false
	shutdownFuncs = nil

	ctx := context.Background()

	// These should all be no-ops when telemetry is disabled
	RecordReviewDuration(ctx, 5*time.Second)
	RecordFilesReviewed(ctx, 10)
	RecordCommentsGenerated(ctx, 3)
	RecordLLMRequest(ctx, "gpt-4", 2*time.Second, 1000, "ok")
	RecordToolCall(ctx, "file_read", 100*time.Millisecond, true)
	RecordToolCall(ctx, "file_read", 100*time.Millisecond, false)
}

func TestCheckMetricErr(t *testing.T) {
	checkMetricErr(nil)
	checkMetricErr(fmt.Errorf("some error"))
}

func TestRecordFunctions_EnabledTelemetry(t *testing.T) {
	setupEnabledTelemetry(t)
	ctx := context.Background()

	RecordReviewDuration(ctx, 5*time.Second)
	RecordFilesReviewed(ctx, 10)
	RecordCommentsGenerated(ctx, 3)
	RecordLLMRequest(ctx, "gpt-4", 2*time.Second, 1000, "ok")
	RecordLLMRequest(ctx, "gpt-4", 1*time.Second, 0, "error")
	RecordToolCall(ctx, "file_read", 100*time.Millisecond, true)
	RecordToolCall(ctx, "file_read", 200*time.Millisecond, false)
}

func TestEnsureMetrics_Idempotent(t *testing.T) {
	setupEnabledTelemetry(t)
	ensureMetrics()
	ensureMetrics()
	if mReviewDuration == nil {
		t.Error("expected mReviewDuration to be initialized")
	}
	if mFilesReviewed == nil {
		t.Error("expected mFilesReviewed to be initialized")
	}
}

func TestGetMeter(t *testing.T) {
	setupEnabledTelemetry(t)
	m := getMeter()
	if m == nil {
		t.Error("expected non-nil meter")
	}
}

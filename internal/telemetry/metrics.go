package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metric handles — initialized once on first use.
var (
	initMetricsOnce    bool
	mReviewDuration    metric.Int64Histogram
	mFilesReviewed     metric.Int64Counter
	mCommentsGenerated metric.Int64Counter
	mLLMRequests       metric.Int64Counter
	mLLMTokens         metric.Int64Counter
	mLLMDuration       metric.Float64Histogram
	mToolCalls         metric.Int64Counter
	mToolExecutionTime metric.Float64Histogram
)

func getMeter() metric.Meter {
	return otel.GetMeterProvider().Meter(serviceName)
}

func ensureMetrics() {
	if initMetricsOnce {
		return
	}
	initMetricsOnce = true
	m := getMeter()

	var err error
	mReviewDuration, err = m.Int64Histogram("ocr.review.duration_seconds",
		metric.WithUnit("s"), metric.WithDescription("Total duration of a code review run"))
	checkMetricErr(err)

	mFilesReviewed, err = m.Int64Counter("ocr.files_reviewed_total",
		metric.WithDescription("Number of files reviewed in this session"))
	checkMetricErr(err)

	mCommentsGenerated, err = m.Int64Counter("ocr.comments_generated_total",
		metric.WithDescription("Number of review comments generated"))
	checkMetricErr(err)

	mLLMRequests, err = m.Int64Counter("ocr.llm.requests_total",
		metric.WithDescription("Total LLM API requests made"))
	checkMetricErr(err)

	mLLMTokens, err = m.Int64Counter("ocr.llm.tokens_used",
		metric.WithDescription("Tokens consumed by LLM requests"))
	checkMetricErr(err)

	mLLMDuration, err = m.Float64Histogram("ocr.llm.request_duration_seconds",
		metric.WithUnit("s"), metric.WithDescription("Duration of individual LLM API requests"))
	checkMetricErr(err)

	mToolCalls, err = m.Int64Counter("ocr.tool.calls_total",
		metric.WithDescription("Total tool calls made"))
	checkMetricErr(err)

	mToolExecutionTime, err = m.Float64Histogram("ocr.tool.execution_duration_seconds",
		metric.WithUnit("s"), metric.WithDescription("Duration of tool executions"))
	checkMetricErr(err)
}

// checkMetricErr intentionally ignores metric registration errors:
// telemetry is best-effort and must not interrupt the main flow.
func checkMetricErr(err error) {}

func RecordReviewDuration(ctx context.Context, dur time.Duration) {
	if !IsEnabled() {
		return
	}
	ensureMetrics()
	if mReviewDuration != nil {
		mReviewDuration.Record(ctx, int64(dur.Seconds()))
	}
}

func RecordFilesReviewed(ctx context.Context, n int64) {
	if !IsEnabled() {
		return
	}
	ensureMetrics()
	if mFilesReviewed != nil {
		mFilesReviewed.Add(ctx, n)
	}
}

func RecordCommentsGenerated(ctx context.Context, n int64) {
	if !IsEnabled() {
		return
	}
	ensureMetrics()
	if mCommentsGenerated != nil {
		mCommentsGenerated.Add(ctx, n)
	}
}

func RecordLLMRequest(ctx context.Context, model string, dur time.Duration, totalTokens int64, status string) {
	if !IsEnabled() {
		return
	}
	ensureMetrics()

	attrs := []attribute.KeyValue{attribute.String("model", model), attribute.String("status", status)}

	if mLLMRequests != nil {
		mLLMRequests.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
	if mLLMDuration != nil {
		mLLMDuration.Record(ctx, dur.Seconds(), metric.WithAttributes(attribute.String("model", model)))
	}

	if mLLMTokens != nil && totalTokens > 0 {
		mLLMTokens.Add(ctx, totalTokens, metric.WithAttributes(attribute.String("type", "total"), attribute.String("model", model)))
	}
}

func RecordToolCall(ctx context.Context, name string, dur time.Duration, ok bool) {
	if !IsEnabled() {
		return
	}
	ensureMetrics()

	status := "ok"
	if !ok {
		status = "error"
	}
	attrs := []attribute.KeyValue{attribute.String("tool.name", name), attribute.String("status", status)}

	if mToolCalls != nil {
		mToolCalls.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
	if mToolExecutionTime != nil {
		mToolExecutionTime.Record(ctx, dur.Seconds(), metric.WithAttributes(attribute.String("tool.name", name)))
	}
}

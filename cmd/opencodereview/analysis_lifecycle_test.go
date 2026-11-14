// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/workflow"
)

func TestAnalysisPreparationSharesDeadlineWithoutBlockingReview(test *testing.T) {
	execution := newAnalysisExecution(nil, reviewOptions{}, &llmRuntime{Model: "fake"}, nil, "review", nil)
	execution.timeout = 100 * time.Millisecond
	preparationStarted := make(chan struct{})
	reviewStarted := make(chan struct{})
	execution.initialize = func(ctx context.Context) error {
		close(preparationStarted)
		if _, exists := ctx.Deadline(); !exists {
			return errors.New("analysis preparation has no deadline")
		}
		select {
		case <-reviewStarted:
		case <-ctx.Done():
			return ctx.Err()
		}
		<-ctx.Done()
		return ctx.Err()
	}
	result := workflow.Run(test.Context(), func(context.Context) ([]model.LlmComment, error) {
		<-preparationStarted
		close(reviewStarted)
		return []model.LlmComment{{Path: "main.go", Content: "Independent review completed."}}, nil
	}, execution.run)
	if result.ReviewError != nil || len(result.Comments) != 1 || !errors.Is(result.AnalysisError, context.DeadlineExceeded) {
		test.Fatalf("result: %+v", result)
	}
	if report := execution.report(result.Analysis, result.AnalysisError); report.Status != "failed" || len(report.Errors) == 0 {
		test.Fatalf("preparation timeout was not reported: %+v", report)
	}
}

func TestAnalysisPreparationFailureClosesInitializedResources(test *testing.T) {
	execution := newAnalysisExecution(nil, reviewOptions{}, &llmRuntime{Model: "fake"}, nil, "review", nil)
	closed := false
	failure := errors.New("preparation failed")
	execution.initialize = func(context.Context) error {
		execution.closeRaw = func() { closed = true }
		return failure
	}
	_, err := execution.run(test.Context())
	if !errors.Is(err, failure) || !closed {
		test.Fatalf("error=%v closed=%v", err, closed)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package workflow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/behaviortask"
	"github.com/alibaba/open-code-review/internal/model"
)

func TestPublicReportOmitsRawErrors(test *testing.T) {
	const raw = "untrusted provider response"
	report := NewReport(behaviortask.Result{Errors: []string{raw}}, errors.New(raw), "")
	if len(report.Errors) != 1 || strings.Contains(report.Errors[0], raw) {
		test.Fatal(report.Errors)
	}
}

func TestReportPreservesBothTimeoutAndArtifactFailure(test *testing.T) {
	report := NewReport(behaviortask.Result{}, errors.Join(context.DeadlineExceeded, behaviortask.ErrResultWrite), "")
	if len(report.Errors) != 2 || report.Errors[0] != "analysis timed out" || report.Errors[1] != "could not write analysis result" {
		test.Fatal(report.Errors)
	}
}

func TestRunIsolationAndJoin(test *testing.T) {
	for _, failReview := range []bool{false, true} {
		started := make(chan struct{})
		failed := errors.New("role failed")
		result := Run(test.Context(), func(ctx context.Context) ([]model.LlmComment, error) {
			select {
			case <-started:
			case <-time.After(time.Second):
				test.Error("roles did not run concurrently")
			}
			if ctx.Err() != nil {
				test.Error("sibling failure canceled review")
			}
			if failReview {
				return nil, failed
			}
			return []model.LlmComment{{}}, nil
		}, func(ctx context.Context) (behaviortask.Result, error) {
			close(started)
			if failReview {
				return behaviortask.Result{Completed: true}, nil
			}
			return behaviortask.Result{}, failed
		})
		if failReview {
			if !errors.Is(result.ReviewError, failed) || !result.Analysis.Completed || result.AnalysisError != nil {
				test.Fatalf("outcome: %+v", result)
			}
		} else if !errors.Is(result.AnalysisError, failed) || len(result.Comments) != 1 || result.ReviewError != nil {
			test.Fatalf("outcome: %+v", result)
		}
	}
}

func TestRunCancellation(test *testing.T) {
	ctx, cancel := context.WithCancel(test.Context())
	var ready sync.WaitGroup
	ready.Add(2)
	go func() { ready.Wait(); cancel() }()
	result := Run(ctx, func(ctx context.Context) ([]model.LlmComment, error) {
		ready.Done()
		<-ctx.Done()
		return nil, ctx.Err()
	}, func(ctx context.Context) (behaviortask.Result, error) {
		ready.Done()
		<-ctx.Done()
		return behaviortask.Result{}, ctx.Err()
	})
	if !errors.Is(result.ReviewError, context.Canceled) || !errors.Is(result.AnalysisError, context.Canceled) {
		test.Fatalf("outcome: %+v", result)
	}
}

func TestReportStatus(test *testing.T) {
	for _, fixture := range []struct {
		name   string
		result behaviortask.Result
		err    error
		status string
	}{
		{"no submission", behaviortask.Result{}, errors.New("provider failed"), "failed"},
		{"partial", behaviortask.Result{Analysis: &behaviortask.Analysis{DeclaredCompletion: "partial"}, Completed: true, StopReason: "none"}, nil, "completed"},
		{"complete", behaviortask.Result{Analysis: &behaviortask.Analysis{DeclaredCompletion: "complete"}, Completed: true, StopReason: "none"}, nil, "completed"},
		{"omitted completeness", behaviortask.Result{Analysis: &behaviortask.Analysis{Markdown: "Report"}, Completed: true, StopReason: "none"}, nil, "completed"},
		{"budget", behaviortask.Result{Analysis: &behaviortask.Analysis{DeclaredCompletion: "complete"}, Completed: true, StopReason: "token_budget"}, nil, "partial"},
		{"delivery", behaviortask.Result{Analysis: &behaviortask.Analysis{DeclaredCompletion: "complete"}, Completed: true, StopReason: "none", Errors: []string{"disk full"}}, errors.New("disk full"), "partial"},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			report := NewReport(fixture.result, fixture.err, "artifacts")
			if report.Status != fixture.status || report.ArtifactDir != "artifacts" || report.CitedObservationIDs == nil {
				test.Fatalf("report: %+v", report)
			}
			if fixture.err != nil && len(report.Errors) != 1 {
				test.Fatalf("errors: %v", report.Errors)
			}
			if fixture.result.Analysis != nil {
				declared := fixture.result.Analysis.DeclaredCompletion
				if declared == "" {
					declared = "unknown"
				}
				if report.Markdown != fixture.result.Analysis.Markdown || report.DeclaredCompletion != declared {
					test.Fatalf("model content changed: %+v", report)
				}
			}
		})
	}
}

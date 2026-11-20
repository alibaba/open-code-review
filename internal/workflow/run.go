// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package workflow

import (
	"context"
	"sync"
	"time"

	"github.com/alibaba/open-code-review/internal/behaviortask"
	"github.com/alibaba/open-code-review/internal/model"
)

type Outcome struct {
	Comments      []model.LlmComment
	ReviewError   error
	ReviewElapsed time.Duration
	Analysis      behaviortask.Result
	AnalysisError error
}

func Run(ctx context.Context, review func(context.Context) ([]model.LlmComment, error), analysis func(context.Context) (behaviortask.Result, error)) Outcome {
	var result Outcome
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		started := time.Now()
		result.Comments, result.ReviewError = review(ctx)
		result.ReviewElapsed = time.Since(started)
	}()
	go func() {
		defer workers.Done()
		result.Analysis, result.AnalysisError = analysis(ctx)
	}()
	workers.Wait()
	return result
}

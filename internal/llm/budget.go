// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"errors"
)

var ErrTokenBudget = errors.New("aggregate LLM token budget exhausted")

type FinalSubmissionClient interface {
	FinalSubmission(ctx context.Context, request ChatRequest) (response *ChatResponse, budgetReached bool, err error)
}

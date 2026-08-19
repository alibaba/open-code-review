// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"testing"

	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// TestBudgetExceededFlag pins BudgetExceeded() to the actual state of the
// token budget gate. Before this it was hard-coded false, so a budget stop
// never reached summary.budget_exceeded in `ocr scan --format json` (#771).
//
// Both cases run the same 50K-tokens-per-file fake through dispatchSubtasks;
// only MaxTokensBudget differs. MaxConcurrency=1 serializes dispatch so the
// gate trips deterministically.
//
// The zero-value case is not repeated here: TestScanGettersOnEmptyAgent in
// getters_test.go already asserts BudgetExceeded()==false on &Agent{}, and
// that assertion stops being vacuous now that the getter reads a field.
func TestBudgetExceededFlag(t *testing.T) {
	cases := []struct {
		name   string
		budget int64
		items  int
		want   bool
	}{
		{name: "gate trips", budget: 120_000, items: 10, want: true},
		{name: "unlimited budget", budget: 0, items: 5, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAgent(Args{
				Template:         budgetTestTemplate(),
				LLMClient:        &fakeBudgetClient{perCallTokens: 50_000},
				CommentCollector: tool.NewCommentCollector(),
				Tools:            tool.NewRegistry(),
				MaxConcurrency:   1,
				MaxTokensBudget:  tc.budget,
				Session:          session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: session.ReviewModeFullScan}),
				SkipPlan:         true,
				SkipDedup:        true,
				SkipSummary:      true,
			})
			a.items = makeScanItems(tc.items)
			a.args.Tools.Freeze()

			if _, err := a.dispatchSubtasks(context.Background()); err != nil {
				t.Fatalf("dispatchSubtasks: %v", err)
			}
			if got := a.BudgetExceeded(); got != tc.want {
				t.Errorf("BudgetExceeded() = %v, want %v", got, tc.want)
			}
		})
	}
}

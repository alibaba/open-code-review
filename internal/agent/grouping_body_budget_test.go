// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

// bodyWithTokens returns text whose CountTokens is at least target. It measures
// the repeat unit once and multiplies, avoiding an O(n^2) grow-and-recount loop.
func bodyWithTokens(target int) string {
	const unit = "alert rule threshold reduce noDataState execErrState summary description\n"
	per := llm.CountTokens(unit)
	if per < 1 {
		per = 1
	}
	return strings.Repeat(unit, target/per+1)
}

// smallDiffLargeBody is the pathological shape: a tiny patch against a large
// file. The old diff-only budget saw only the diff and never split.
func smallDiffLargeBody(path string, bodyTokens int) model.Diff {
	return model.Diff{
		NewPath:        path,
		Diff:           "@@ -1,1 +1,2 @@\n context\n+one changed line\n", // tiny
		NewFileContent: bodyWithTokens(bodyTokens),
	}
}

func TestEnforceGroupTokenBudget_SplitsLargeBodySmallDiffGroup(t *testing.T) {
	const tokenLimit = 160000 // == PromptTokenLimit(200000)

	// The real observability bundle that overflowed a 160k per-item ceiling:
	// six small-diff files whose measured body tokens total ~97k. Combined diff
	// is trivial, so the OLD diff-only budget kept them as one item.
	bundle := []struct {
		path       string
		bodyTokens int
	}{
		{"grafana-alerts-dev.yaml", 35308},
		{"grafana-alerts-production.yaml", 35992},
		{"grafana-dashboard-journey-health.json", 11091},
		{"grafana-dashboard-router-pay.json", 6214},
		{"grafana-alert-contract.test.ts", 6056},
		{"grafana-router-pay-assets.test.ts", 2533},
	}
	var diffs []model.Diff
	for _, f := range bundle {
		diffs = append(diffs, smallDiffLargeBody(f.path, f.bodyTokens))
	}
	group := FileGroup{Label: "Grafana observability bundle", Diffs: diffs}

	// Guard: the old diff-only measure would NOT have split this group.
	var diffOnly int64
	for _, d := range group.Diffs {
		diffOnly += int64(llm.CountTokens(d.Diff))
	}
	if diffOnly > int64(tokenLimit) {
		t.Fatalf("test setup: diffs alone (%d) must be under the limit to prove the body drives the split", diffOnly)
	}

	got := enforceGroupTokenBudget([]FileGroup{group}, tokenLimit)
	if len(got) != len(diffs) {
		t.Fatalf("expected the over-budget bundle to split to %d per-file groups, got %d groups", len(diffs), len(got))
	}
	for _, g := range got {
		if len(g.Diffs) != 1 {
			t.Fatalf("expected each split group to hold exactly one file, got %d", len(g.Diffs))
		}
	}
}

func TestEnforceGroupTokenBudget_KeepsWithinBudgetGroup(t *testing.T) {
	const tokenLimit = 160000
	// The real 4-file subset that reviewed cleanly as one group on its own PR:
	// the two big alert YAMLs plus their two tests, ~80k body tokens total.
	group := FileGroup{Label: "keep", Diffs: []model.Diff{
		smallDiffLargeBody("grafana-alerts-dev.yaml", 35308),
		smallDiffLargeBody("grafana-alerts-production.yaml", 35992),
		smallDiffLargeBody("grafana-alert-contract.test.ts", 6056),
		smallDiffLargeBody("grafana-router-pay-assets.test.ts", 2533),
	}}
	got := enforceGroupTokenBudget([]FileGroup{group}, tokenLimit)
	if len(got) != 1 || len(got[0].Diffs) != 4 {
		t.Fatalf("within-budget group must stay intact, got %d groups", len(got))
	}
}

func TestEnforceGroupTokenBudget_NeverSplitsSingleFile(t *testing.T) {
	const tokenLimit = 160000
	// A lone oversized file cannot be split further; leave it for the
	// read-chunking + compression path, not the grouping split.
	group := FileGroup{Label: "solo", Diffs: []model.Diff{smallDiffLargeBody("huge.json", 200000)}}
	got := enforceGroupTokenBudget([]FileGroup{group}, tokenLimit)
	if len(got) != 1 || len(got[0].Diffs) != 1 {
		t.Fatalf("single-file group must not be split, got %d groups", len(got))
	}
}

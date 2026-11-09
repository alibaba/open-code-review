// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/estimate"
	"github.com/alibaba/open-code-review/internal/model"
)

// TestHumanTokens mirrors scan's TestHumanTokens so the diff- and scan-path
// formatters stay byte-identical. If this table and scan's ever disagree, one
// copy has drifted.
func TestHumanTokens(t *testing.T) {
	cases := map[int64]string{
		0:         "0",
		420:       "420",
		999:       "999",
		1000:      "1K",
		1500:      "2K", // rounds
		850_000:   "850K",
		1_000_000: "1.0M",
		2_400_000: "2.4M",
	}
	for in, want := range cases {
		if got := humanTokens(in); got != want {
			t.Errorf("humanTokens(%d) = %q, want %q", in, got, want)
		}
	}
}

// TestEstimateDiffFileTokens_ZeroForSkipped verifies deleted and empty diffs
// project to zero tokens so they never trip the budget gate (they are skipped
// before dispatch). Also asserts a normal diff projects a sane positive value
// bounded below by the fixed per-file overhead.
func TestEstimateDiffFileTokens_ZeroForSkipped(t *testing.T) {
	if got := estimateDiffFileTokens(model.Diff{IsDeleted: true, Diff: "+x"}, estimate.Parameters{}); got != 0 {
		t.Errorf("deleted diff projected %d tokens, want 0", got)
	}
	if got := estimateDiffFileTokens(model.Diff{NewPath: "a.go", Diff: ""}, estimate.Parameters{}); got != 0 {
		t.Errorf("empty diff projected %d tokens, want 0", got)
	}
	got := estimateDiffFileTokens(model.Diff{NewPath: "a.go", Diff: "+package main\nfunc f() {}\n"}, estimate.Parameters{})
	if got <= 0 {
		t.Errorf("normal diff projected %d tokens, want > 0", got)
	}
	// The look-ahead must be at least the fixed per-file overhead
	// (promptOverhead*(1+rounds) + plan output + main output) so even a tiny
	// diff carries meaningful cost in the projection.
	const minExpected = int64(2000*8 + 400 + 700*7)
	if got < minExpected {
		t.Errorf("projected %d, want >= %d (fixed overhead floor)", got, minExpected)
	}
}

// TestEstimateDiffCost verifies the aggregate estimate sums per-file costs,
// skips deleted/empty diffs, and satisfies TotalTokens == Input + Output. Used
// by the pre-review scale warning.
func TestEstimateDiffCost(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "a.go", Diff: "+a\n"},
		{NewPath: "b.go", Diff: "+b\n"},
		{NewPath: "c.go", IsDeleted: true, Diff: "+c\n"}, // skipped
		{NewPath: "d.go", Diff: ""},                      // skipped
	}
	est := estimateDiffCost(diffs, estimate.Parameters{})
	if est.Files != 2 {
		t.Errorf("Files = %d, want 2 (deleted + empty skipped)", est.Files)
	}
	perFile := estimateDiffFileTokens(diffs[0], estimate.Parameters{})
	if est.TotalTokens != perFile*2 {
		t.Errorf("TotalTokens = %d, want 2*%d = %d", est.TotalTokens, perFile, perFile*2)
	}
	if est.InputTokens <= 0 || est.OutputTokens <= 0 {
		t.Errorf("expected non-zero input/output splits, got in=%d out=%d", est.InputTokens, est.OutputTokens)
	}
	// Split invariant the String() rendering relies on.
	if est.TotalTokens != est.InputTokens+est.OutputTokens {
		t.Errorf("TotalTokens = %d, want Input(%d)+Output(%d) = %d",
			est.TotalTokens, est.InputTokens, est.OutputTokens, est.InputTokens+est.OutputTokens)
	}
	if s := est.String(); s == "" || !strings.Contains(s, "token") {
		t.Errorf("expected non-empty estimate string mentioning tokens, got %q", s)
	}
}

// TestEstimateDiffCost_ScalesWithContent verifies that a larger diff projects
// strictly more tokens than a small one (the estimate isn't a flat constant).
func TestEstimateDiffCost_ScalesWithContent(t *testing.T) {
	small := estimateDiffFileTokens(model.Diff{NewPath: "a.go", Diff: "+x\n"}, estimate.Parameters{})
	large := estimateDiffFileTokens(model.Diff{NewPath: "a.go", Diff: strings.Repeat("line of code\n", 200)}, estimate.Parameters{})
	if large <= small {
		t.Errorf("expected large diff to project more tokens than small: large=%d small=%d", large, small)
	}
}

func TestEstimateDiffCost_CustomParameters(t *testing.T) {
	diffs := []model.Diff{
		{NewPath: "a.go", Diff: "+a\n"},
		{NewPath: "b.go", Diff: "+longer diff\n"},
		{NewPath: "deleted.go", IsDeleted: true, Diff: "-deleted\n"},
		{NewPath: "empty.go"},
	}
	params := estimate.Parameters{PromptOverheadTokens: 8000, OutputTokensPerRound: 3000}
	custom := estimateDiffCost(diffs, params)
	baseline := estimateDiffCost(diffs, estimate.Parameters{})
	if custom.InputTokens-baseline.InputTokens != 2*8*(8000-2000) ||
		custom.OutputTokens-baseline.OutputTokens != 2*7*(3000-700) {
		t.Fatalf("custom estimate = %+v, baseline = %+v", custom, baseline)
	}
	var total int64
	for _, d := range diffs {
		total += estimateDiffFileTokens(d, params)
	}
	if custom.Files != 2 || custom.TotalTokens != total || total != custom.InputTokens+custom.OutputTokens {
		t.Fatalf("aggregate %+v disagrees with per-file total %d", custom, total)
	}
}

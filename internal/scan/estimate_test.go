// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/estimate"
	"github.com/alibaba/open-code-review/internal/model"
)

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

func TestEstimateCost_ScalesWithContentAndPhases(t *testing.T) {
	items := []model.ScanItem{
		{Path: "a.go", Content: strings.Repeat("token ", 500)}, // ~non-trivial
		{Path: "b.go", Content: strings.Repeat("x ", 300)},
		{Path: "bin.dat", IsBinary: true}, // skipped
		{Path: "empty.go", Content: ""},   // skipped
	}

	// Plan off, dedup off, summary off → only MAIN_TASK cost.
	base := estimateCost(items, false, false, false, estimate.Parameters{})
	if base.Files != 2 {
		t.Fatalf("expected 2 reviewable files, got %d", base.Files)
	}
	if base.TotalTokens <= 0 {
		t.Fatal("expected positive total")
	}

	// Turning plan on must increase the estimate.
	withPlan := estimateCost(items, true, false, false, estimate.Parameters{})
	if withPlan.TotalTokens <= base.TotalTokens {
		t.Errorf("plan should raise estimate: base=%d withPlan=%d", base.TotalTokens, withPlan.TotalTokens)
	}

	// Summary + dedup on top must increase further.
	full := estimateCost(items, true, true, true, estimate.Parameters{})
	if full.TotalTokens <= withPlan.TotalTokens {
		t.Errorf("dedup+summary should raise estimate: withPlan=%d full=%d", withPlan.TotalTokens, full.TotalTokens)
	}

	// TotalTokens must equal input + output.
	if full.TotalTokens != full.InputTokens+full.OutputTokens {
		t.Errorf("total %d != input %d + output %d", full.TotalTokens, full.InputTokens, full.OutputTokens)
	}
}

func TestEstimateFileTokens(t *testing.T) {
	// Binary / empty → 0 (skipped before dispatch).
	if got := estimateFileTokens(model.ScanItem{Path: "x", IsBinary: true}, true, estimate.Parameters{}); got != 0 {
		t.Errorf("binary file should estimate 0, got %d", got)
	}
	if got := estimateFileTokens(model.ScanItem{Path: "x", Content: ""}, true, estimate.Parameters{}); got != 0 {
		t.Errorf("empty file should estimate 0, got %d", got)
	}

	it := model.ScanItem{Path: "a.go", Content: strings.Repeat("token ", 400)}
	withPlan := estimateFileTokens(it, true, estimate.Parameters{})
	noPlan := estimateFileTokens(it, false, estimate.Parameters{})
	if withPlan <= 0 || noPlan <= 0 {
		t.Fatalf("expected positive estimates, got plan=%d noplan=%d", withPlan, noPlan)
	}
	if withPlan <= noPlan {
		t.Errorf("plan-enabled estimate (%d) should exceed plan-disabled (%d)", withPlan, noPlan)
	}

	// Per-file estimate must equal the aggregate single-file MAIN+PLAN cost
	// (sanity that the aggregate and look-ahead share the same model).
	agg := estimateCost([]model.ScanItem{it}, true, false, false, estimate.Parameters{})
	if agg.TotalTokens != withPlan {
		t.Errorf("aggregate single-file total (%d) != per-file estimate (%d)", agg.TotalTokens, withPlan)
	}
}

func TestEstimateCost_EmptyItems(t *testing.T) {
	est := estimateCost(nil, true, true, true, estimate.Parameters{})
	if est.Files != 0 || est.TotalTokens != 0 {
		t.Errorf("empty input should yield zero estimate, got %+v", est)
	}
}

func TestEstimateCost_CustomParameters(t *testing.T) {
	items := []model.ScanItem{
		{Path: "a.go", Content: "package a\n"},
		{Path: "b.go", Content: "package b\nvar x = 1\n"},
		{Path: "bin.dat", IsBinary: true, Content: "binary"},
		{Path: "empty.go"},
	}
	params := estimate.Parameters{PromptOverheadTokens: 8000, OutputTokensPerRound: 3000}
	for _, plan := range []bool{false, true} {
		custom := estimateCost(items, plan, false, false, params)
		baseline := estimateCost(items, plan, false, false, estimate.Parameters{})
		calls := int64(2 * 7)
		if plan {
			calls += 2
		}
		if custom.InputTokens-baseline.InputTokens != calls*(8000-2000) ||
			custom.OutputTokens-baseline.OutputTokens != 2*7*(3000-700) {
			t.Fatalf("plan=%v: custom = %+v, baseline = %+v", plan, custom, baseline)
		}
		var total int64
		for _, it := range items {
			total += estimateFileTokens(it, plan, params)
		}
		if custom.Files != 2 || custom.TotalTokens != total || total != custom.InputTokens+custom.OutputTokens {
			t.Fatalf("aggregate %+v disagrees with per-file total %d", custom, total)
		}
		for _, phases := range [][2]bool{{true, false}, {false, true}, {true, true}} {
			full := estimateCost(items, plan, phases[0], phases[1], params)
			base := estimateCost(items, plan, phases[0], phases[1], estimate.Parameters{})
			phaseCalls := calls
			for _, enabled := range phases {
				if enabled {
					phaseCalls++
				}
			}
			if full.InputTokens-base.InputTokens != phaseCalls*(8000-2000) ||
				full.OutputTokens-base.OutputTokens != 2*7*(3000-700) {
				t.Fatalf("optional phases use inconsistent estimates: custom=%+v baseline=%+v", full, base)
			}
		}
	}
	if got := estimateCost(nil, true, true, true, params); got.TotalTokens != 0 {
		t.Fatalf("empty input with overrides estimated %+v", got)
	}
}

func TestEstimate_StringMentionsTokens(t *testing.T) {
	est := Estimate{Files: 3, InputTokens: 1_200_000, OutputTokens: 90_000, TotalTokens: 1_290_000}
	s := est.String()
	for _, want := range []string{"3 file", "1.2M", "90K", "1.3M"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() missing %q: %s", want, s)
		}
	}
}

func TestPhaseEnabled_GatedByTemplateAndFlag(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	tpl.PlanTask = &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "plan {{file_content}}"}}}
	tpl.DedupTask = &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "dedup {{batch_comments}}"}}}
	tpl.ProjectSummaryTask = &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "summary {{all_comments}}"}}}

	a := newAgentForTest(t, tpl)
	if !a.planEnabled() {
		t.Error("planEnabled should be true when template has PlanTask and SkipPlan is false")
	}
	if !a.dedupEnabled() {
		t.Error("dedupEnabled should be true")
	}
	if !a.summaryEnabled() {
		t.Error("summaryEnabled should be true")
	}

	// Flags disable each phase independently.
	a.args.SkipPlan = true
	a.args.SkipDedup = true
	a.args.SkipSummary = true
	if a.planEnabled() || a.dedupEnabled() || a.summaryEnabled() {
		t.Error("--no-* flags must disable the corresponding phase")
	}

	// Nil template field disables regardless of flag.
	a2 := newAgentForTest(t, makeTemplateWithFullScan())
	a2.args.Template.DedupTask = nil
	if a2.dedupEnabled() {
		t.Error("nil DedupTask must disable dedup")
	}
}

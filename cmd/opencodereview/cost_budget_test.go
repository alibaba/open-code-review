// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

type mockCostResultProvider struct {
	mockResultProvider
	cost               float64
	priced             bool
	costBudgetExceeded bool
}

func (m *mockCostResultProvider) TotalCost() (float64, bool) { return m.cost, m.priced }
func (m *mockCostResultProvider) CostBudgetExceeded() bool   { return m.costBudgetExceeded }

func TestEmitRunResult_JSONReportsCost(t *testing.T) {
	ag := &mockCostResultProvider{
		mockResultProvider: mockResultProvider{filesReviewed: 1, totalTokens: 10, budgetExceeded: true},
		cost:               1.23456789, priced: true, costBudgetExceeded: true,
	}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, []model.LlmComment{}, time.Now(), "json", "developer", nil, nil, os.Stdout, nil); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	s := out.Summary
	if s == nil || s.CostUSD == nil || *s.CostUSD != 1.234568 || !s.CostBudgetExceeded || !s.BudgetExceeded {
		t.Fatalf("summary = %+v, want cost_usd 1.234568 and both budget flags", s)
	}

	failure := captureStderr(t, func() { emitFailureUsage(ag, time.Second, "json", nil, nil) })
	if !strings.Contains(failure, `"cost_usd": 1.234568`) || !strings.Contains(failure, `"cost_budget_exceeded": true`) {
		t.Fatalf("failure record = %s, want the cost fields", failure)
	}
}

func TestEmitRunResult_JSONOmitsUnknownCost(t *testing.T) {
	ag := &mockCostResultProvider{mockResultProvider: mockResultProvider{filesReviewed: 1, totalTokens: 10}}
	got := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, []model.LlmComment{}, time.Now(), "json", "developer", nil, nil, os.Stdout, nil); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	if strings.Contains(got, "cost_usd") || strings.Contains(got, "cost_budget_exceeded") {
		t.Fatalf("JSON without a price must omit the cost fields, got %s", got)
	}
}

func TestResolveCostBudget(t *testing.T) {
	configured := &llm.Price{Input: 3, CacheRead: 0.3, CacheWrite: 3.75, Output: 15}
	rt := &llmRuntime{Model: "m", Price: configured}

	budget, price, err := resolveCostBudget(&Config{MaxCostBudget: 2}, 0, "", rt)
	if err != nil || budget != 2 || price != configured {
		t.Fatalf("config budget = %v, %+v, %v", budget, price, err)
	}
	budget, price, err = resolveCostBudget(&Config{MaxCostBudget: 2}, 5, "input=1,output=2", rt)
	if err != nil || budget != 5 || *price != (llm.Price{Input: 1, CacheRead: 1, CacheWrite: 1, Output: 2}) {
		t.Fatalf("flag override = %v, %+v, %v", budget, price, err)
	}
	if _, _, err := resolveCostBudget(nil, 1, "", &llmRuntime{Model: "m"}); err == nil || !strings.Contains(err.Error(), "needs the price") {
		t.Fatalf("budget without a price: err = %v", err)
	}
	if _, _, err := resolveCostBudget(nil, 0, "input=1", rt); err == nil || !strings.Contains(err.Error(), "--price-per-m") {
		t.Fatalf("bad --price-per-m: err = %v", err)
	}
	if _, _, err := resolveCostBudget(&Config{MaxCostBudget: -1}, 0, "", rt); err == nil {
		t.Fatal("negative max_cost_budget in config must fail")
	}
	// A price alone only reports the cost.
	if budget, price, err := resolveCostBudget(nil, 0, "", rt); err != nil || budget != 0 || price != configured {
		t.Fatalf("price only = %v, %+v, %v", budget, price, err)
	}
}

func TestSetConfigValueMaxCostBudget(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "max_cost_budget", "2.5"); err != nil || cfg.MaxCostBudget != 2.5 {
		t.Fatalf("set = %v, %v", cfg.MaxCostBudget, err)
	}
	if err := setConfigValue(cfg, "max_cost_budget", "0"); err != nil || cfg.MaxCostBudget != 0 {
		t.Fatalf("clear = %v, %v", cfg.MaxCostBudget, err)
	}
	for _, bad := range []string{"-1", "abc", "NaN"} {
		if err := setConfigValue(cfg, "max_cost_budget", bad); err == nil {
			t.Errorf("max_cost_budget %q accepted", bad)
		}
	}
}

func TestValidateReviewOptionsRejectsNegativeCostBudget(t *testing.T) {
	opts := &reviewOptions{audience: "human", outputFormat: "text", maxCostBudget: -1}
	if err := validateReviewOptions(opts); err == nil || !strings.Contains(err.Error(), "--max-cost-budget") {
		t.Fatalf("err = %v", err)
	}
	scan := &scanOptions{audience: "human", outputFormat: "text", maxCostBudget: -1}
	if err := validateScanOptions(scan); err == nil || !strings.Contains(err.Error(), "--max-cost-budget") {
		t.Fatalf("scan err = %v", err)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"math"
	"strings"
	"testing"
)

func TestPriceCost(t *testing.T) {
	p := Price{Input: 3, CacheRead: 0.3, CacheWrite: 3.75, Output: 15}
	// 1M prompt tokens of which 600K cache reads and 300K cache writes, plus 100K output.
	got := p.Cost(1_000_000, 100_000, 600_000, 300_000)
	want := 0.1*3 + 0.6*0.3 + 0.3*3.75 + 0.1*15
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("Cost = %v, want %v", got, want)
	}
	// Cache counts above the prompt total (a provider that reports them
	// separately) never bill a negative uncached remainder.
	if got := p.Cost(0, 0, 1_000_000, 0); math.Abs(got-0.3) > 1e-9 {
		t.Fatalf("Cost = %v, want 0.3", got)
	}
}

func TestParsePrice(t *testing.T) {
	p, err := ParsePrice(" input=3, cache_read=0.3 ,cache_write=3.75,output=15")
	if err != nil {
		t.Fatalf("ParsePrice: %v", err)
	}
	if p != (Price{Input: 3, CacheRead: 0.3, CacheWrite: 3.75, Output: 15}) {
		t.Fatalf("ParsePrice = %+v", p)
	}
	if again, err := ParsePrice(p.String()); err != nil || again != p {
		t.Fatalf("String round trip = %+v, %v", again, err)
	}

	p, err = ParsePrice("input=2,output=10")
	if err != nil {
		t.Fatalf("ParsePrice: %v", err)
	}
	if p.CacheRead != 2 || p.CacheWrite != 2 {
		t.Fatalf("missing cache prices = %+v, want the input price", p)
	}

	for in, want := range map[string]string{
		"input=3":                  "both input and output",
		"output=3":                 "both input and output",
		"input=3,output=x":         "not a number",
		"input=3,output=-1":        "non-negative number, got -1",
		"input=3,output=NaN":       "non-negative",
		"input=3,output=1,foo=1":   "unknown price key",
		"input=3;output=1":         "not a number",
		"input=3,output=1,cache=1": "unknown price key",
		"":                         "key=value",
	} {
		if _, err := ParsePrice(in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParsePrice(%q) error = %v, want it to mention %q", in, err, want)
		}
	}
}

func TestResolveEndpointPriceFromProviderTable(t *testing.T) {
	clearAllEnv(t)
	prices := map[string]map[string]float64{"model-a": {"input": 3, "output": 15, "cache_read": 0.3}}
	path, _ := writeResolverConfig(t, configFile{
		Provider: "my-proxy",
		CustomProviders: map[string]providerEntryConfig{"my-proxy": {
			APIKey: "k", URL: "https://proxy.example", Protocol: "openai", Model: "model-a", Prices: prices,
		}},
	})
	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.Price == nil || *ep.Price != (Price{Input: 3, CacheRead: 0.3, CacheWrite: 3, Output: 15}) {
		t.Fatalf("Price = %+v", ep.Price)
	}

	// A model without an entry has no price.
	ep, err = ResolveEndpointWithModelOverride(path, "model-b")
	if err != nil || ep.Price != nil {
		t.Fatalf("model-b: Price = %+v, err = %v; want nil, nil", ep.Price, err)
	}

	prices["model-a"] = map[string]float64{"input": 3}
	path, _ = writeResolverConfig(t, configFile{
		Provider: "my-proxy",
		CustomProviders: map[string]providerEntryConfig{"my-proxy": {
			APIKey: "k", URL: "https://proxy.example", Protocol: "openai", Model: "model-a", Prices: prices,
		}},
	})
	if _, err := ResolveEndpoint(path); err == nil || !strings.Contains(err.Error(), `prices for model "model-a"`) {
		t.Fatalf("invalid price error = %v", err)
	}
}

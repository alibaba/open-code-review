// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Price is a model's list price in USD per million tokens, for each token kind
// providers bill at its own rate.
type Price struct {
	Input      float64
	CacheRead  float64
	CacheWrite float64
	Output     float64
}

// Cost returns the USD cost of the given usage. promptTokens includes the
// cache read and write tokens, as UsageInfo.PromptTokens does, so only the
// remainder is billed at the uncached input price.
func (p Price) Cost(promptTokens, completionTokens, cacheReadTokens, cacheWriteTokens int64) float64 {
	uncached := max(promptTokens-cacheReadTokens-cacheWriteTokens, 0)
	return (float64(uncached)*p.Input + float64(cacheReadTokens)*p.CacheRead +
		float64(cacheWriteTokens)*p.CacheWrite + float64(completionTokens)*p.Output) / 1e6
}

// String renders the price in the form ParsePrice reads.
func (p Price) String() string {
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	return "input=" + f(p.Input) + ",cache_read=" + f(p.CacheRead) +
		",cache_write=" + f(p.CacheWrite) + ",output=" + f(p.Output)
}

// ParsePrice reads a price written as "input=3,cache_read=0.3,cache_write=3.75,output=15"
// (USD per million tokens).
func ParsePrice(s string) (Price, error) {
	m := map[string]float64{}
	for _, part := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return Price{}, fmt.Errorf("price %q: want key=value pairs separated by commas", s)
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return Price{}, fmt.Errorf("price %q: %s is not a number", s, strings.TrimSpace(k))
		}
		m[strings.TrimSpace(k)] = f
	}
	return PriceFromMap(m)
}

// PriceFromMap builds a Price from its keys (input, cache_read, cache_write,
// output). input and output are required. A missing cache price defaults to
// the input price, which is exact for providers that bill no cache premium;
// set cache_write where a cache write costs more than input.
func PriceFromMap(m map[string]float64) (Price, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch k {
		case "input", "cache_read", "cache_write", "output":
		default:
			return Price{}, fmt.Errorf("unknown price key %q (want input, cache_read, cache_write, output)", k)
		}
		if v := m[k]; v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return Price{}, fmt.Errorf("price %s must be a non-negative number", k)
		}
	}
	input, hasInput := m["input"]
	output, hasOutput := m["output"]
	if !hasInput || !hasOutput {
		return Price{}, fmt.Errorf("price needs both input and output")
	}
	p := Price{Input: input, CacheRead: input, CacheWrite: input, Output: output}
	if v, ok := m["cache_read"]; ok {
		p.CacheRead = v
	}
	if v, ok := m["cache_write"]; ok {
		p.CacheWrite = v
	}
	return p, nil
}

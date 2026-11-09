// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package estimate shares rough token-cost heuristics between review and scan.
package estimate

// Parameters controls the per-call estimates used by both pre-run projections
// and dispatch budget look-ahead. It does not change LLM request limits or
// API-reported usage. Zero values select the historical defaults.
type Parameters struct {
	PromptOverheadTokens int64
	OutputTokensPerRound int64
}

// WithDefaults returns a copy with unspecified estimates filled in. Negative
// values also fall back to defaults for callers outside the validated CLI.
func (p Parameters) WithDefaults() Parameters {
	if p.PromptOverheadTokens <= 0 {
		p.PromptOverheadTokens = 2000
	}
	if p.OutputTokensPerRound <= 0 {
		p.OutputTokensPerRound = 700
	}
	return p
}

// FileTokens projects the input/output cost of seven MAIN_TASK rounds plus an
// optional PLAN call. Tool-use history can inflate actual costs substantially;
// these values are an order-of-magnitude projection, not a billing guarantee.
func (p Parameters) FileTokens(contentTokens int64, planEnabled bool) (input, output int64) {
	p = p.WithDefaults()
	const mainRounds = 7
	input = (contentTokens + p.PromptOverheadTokens) * mainRounds
	output = p.OutputTokensPerRound * mainRounds
	if planEnabled {
		input += contentTokens + p.PromptOverheadTokens
		output += 400 // PLAN normally returns a small JSON result.
	}
	return input, output
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/model"
)

// emptySHA256 is the canonical digest of an empty input, which hashFields must
// return for an empty field sequence.
func emptySHA256() string {
	sum := sha256.Sum256(nil)
	return hex.EncodeToString(sum[:])
}

func TestHashFields_EmptyIsCanonical(t *testing.T) {
	if got := hashFields(); got != emptySHA256() {
		t.Errorf("hashFields() = %q, want empty-input digest %q", got, emptySHA256())
	}
}

func TestHashFields_LengthPrefixPreventsCollision(t *testing.T) {
	// Without length-prefix framing, ["ab",""] and ["a","b"] both concatenate to
	// "ab" and would collide. The 8-byte prefix must keep them distinct.
	if hashFields("ab", "") == hashFields("a", "b") {
		t.Error("length-prefix framing failed: distinct field sequences collided")
	}
}

func TestRuleConfigSHA256(t *testing.T) {
	base := &rules.SystemRule{
		DefaultRule: "default",
		PathRules:   []rules.PathRule{{Pattern: "*.go", Rule: "go rule"}},
	}
	a := New(Args{SystemRule: base})

	if a.ruleConfigSHA256() != a.ruleConfigSHA256() {
		t.Fatal("ruleConfigSHA256 is not deterministic")
	}

	// A changed default rule changes the digest.
	changed := New(Args{SystemRule: &rules.SystemRule{
		DefaultRule: "different",
		PathRules:   base.PathRules,
	}})
	if a.ruleConfigSHA256() == changed.ruleConfigSHA256() {
		t.Error("changing the default rule did not change rule_config_sha256")
	}

	// Rule order is significant (first match wins) — reversing must change it.
	ordered := New(Args{SystemRule: &rules.SystemRule{PathRules: []rules.PathRule{
		{Pattern: "*.go", Rule: "go rule"},
		{Pattern: "*.py", Rule: "py rule"},
	}}})
	swapped := New(Args{SystemRule: &rules.SystemRule{PathRules: []rules.PathRule{
		{Pattern: "*.py", Rule: "py rule"},
		{Pattern: "*.go", Rule: "go rule"},
	}}})
	if ordered.ruleConfigSHA256() == swapped.ruleConfigSHA256() {
		t.Error("reordering path rules did not change rule_config_sha256")
	}

	// The applied file filter participates in the digest.
	withFilter := New(Args{
		SystemRule: base,
		FileFilter: &rules.FileFilter{Include: []string{"*.go"}, Exclude: []string{"vendor/**"}},
	})
	if a.ruleConfigSHA256() == withFilter.ruleConfigSHA256() {
		t.Error("adding a file filter did not change rule_config_sha256")
	}
}

func TestRuleConfigSHA256_NilResolverAndFilter(t *testing.T) {
	a := New(Args{SystemRule: nil, FileFilter: nil})
	if got := a.ruleConfigSHA256(); got != emptySHA256() {
		t.Errorf("empty rule config = %q, want empty-input digest %q", got, emptySHA256())
	}
}

func TestSourceArtifactSHA256_DedupsByItemID(t *testing.T) {
	// Two diffs that normalize to the same (mode, old, new) share one item_id, so
	// RegisterSelected seals a single selected item. The artifact digest must use
	// the same denominator: the pair is deduplicated (first wins) so the run with
	// the duplicate hashes identically to the run carrying only the first diff.
	d1 := model.Diff{OldPath: "a.go", NewPath: "a.go", Diff: "@@ first content @@"}
	d2 := model.Diff{OldPath: "a.go", NewPath: "a.go", Diff: "@@ second content @@"}

	dup := New(Args{})
	dup.diffs = []model.Diff{d1, d2}

	single := New(Args{})
	single.diffs = []model.Diff{d1}

	got := dup.sourceArtifactSHA256()
	if got != dup.sourceArtifactSHA256() {
		t.Fatal("sourceArtifactSHA256 is not deterministic")
	}
	if got != single.sourceArtifactSHA256() {
		t.Errorf("duplicate item_id was not deduplicated: digest %q != single-item digest %q",
			got, single.sourceArtifactSHA256())
	}
	// Sanity: it is a real digest, not the empty-input sentinel.
	if got == emptySHA256() {
		t.Error("non-empty selected set produced the empty-input digest")
	}
}

func TestRuntimeConfigSHA256(t *testing.T) {
	baseArgs := Args{
		Model:          "m",
		MaxConcurrency: 4,
		RuntimeConfig: RuntimeConfig{
			Protocol:     "anthropic",
			EndpointHost: "api.example.com",
			Language:     "en",
			Timeout:      30 * time.Second,
		},
	}
	a := New(baseArgs)
	if a.runtimeConfigSHA256() != a.runtimeConfigSHA256() {
		t.Fatal("runtimeConfigSHA256 is not deterministic")
	}

	// Every allowlisted field must participate in the digest.
	cases := []struct {
		name   string
		mutate func(*Args)
	}{
		{"protocol", func(x *Args) { x.RuntimeConfig.Protocol = "openai" }},
		{"model", func(x *Args) { x.Model = "other" }},
		{"host", func(x *Args) { x.RuntimeConfig.EndpointHost = "api.other.com" }},
		{"language", func(x *Args) { x.RuntimeConfig.Language = "zh" }},
		{"timeout", func(x *Args) { x.RuntimeConfig.Timeout = 60 * time.Second }},
		{"concurrency", func(x *Args) { x.MaxConcurrency = 8 }},
		// The aggregate budget changes what coverage a run can even attempt, so two
		// otherwise-identical runs with different caps must not share an identity.
		{"max_tokens_budget", func(x *Args) { x.MaxTokensBudget = 100_000 }},
		{"max_tokens_budget_explicit", func(x *Args) { x.MaxTokensBudgetExplicit = true }},
		{"max_tokens_budget_multiplier", func(x *Args) { x.Template.MaxTokensBudgetMultiplier = 2.5 }},
		{"max_tool_request_times", func(x *Args) { x.Template.MaxToolRequestTimes = 75 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mutated := baseArgs // RuntimeConfig has no pointers/slices: a value copy is a full copy.
			tc.mutate(&mutated)
			if New(mutated).runtimeConfigSHA256() == a.runtimeConfigSHA256() {
				t.Errorf("changing %s did not change runtime_config_sha256", tc.name)
			}
		})
	}
}

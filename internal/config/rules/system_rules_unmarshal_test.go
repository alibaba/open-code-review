// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSystemRuleUnmarshalJSON exercises the custom order-preserving decoder in
// SystemRule.UnmarshalJSON: the happy path, the absent/null map short-circuits,
// and the malformed-input error branches.
func TestSystemRuleUnmarshalJSON(t *testing.T) {
	t.Run("preserves path_rule_map key order", func(t *testing.T) {
		var r SystemRule
		in := `{"default_rule":"d.md","path_rule_map":{"*.go":"go.md","*.php":"php.md"}}`
		if err := json.Unmarshal([]byte(in), &r); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if r.DefaultRule != "d.md" {
			t.Errorf("DefaultRule = %q, want d.md", r.DefaultRule)
		}
		if len(r.PathRules) != 2 {
			t.Fatalf("PathRules len = %d, want 2", len(r.PathRules))
		}
		if r.PathRules[0].Pattern != "*.go" || r.PathRules[1].Pattern != "*.php" {
			t.Errorf("order not preserved: %+v", r.PathRules)
		}
	})

	t.Run("absent path_rule_map yields no rules", func(t *testing.T) {
		var r SystemRule
		if err := json.Unmarshal([]byte(`{"default_rule":"d.md"}`), &r); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(r.PathRules) != 0 {
			t.Errorf("PathRules = %+v, want empty", r.PathRules)
		}
	})

	t.Run("null path_rule_map yields no rules", func(t *testing.T) {
		var r SystemRule
		if err := json.Unmarshal([]byte(`{"default_rule":"d.md","path_rule_map":null}`), &r); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(r.PathRules) != 0 {
			t.Errorf("PathRules = %+v, want empty", r.PathRules)
		}
	})

	errorCases := []struct {
		name string
		in   string
		want string
	}{
		{"invalid top-level json", `{`, ""},
		{"path_rule_map is not an object", `{"default_rule":"d.md","path_rule_map":[1,2]}`, "expected '{'"},
		{"path_rule_map value is not a string", `{"default_rule":"d.md","path_rule_map":{"*.go":123}}`, "read path_rule_map value"},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			var r SystemRule
			err := json.Unmarshal([]byte(tc.in), &r)
			if err == nil {
				t.Fatalf("in %q: expected error, got nil", tc.in)
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

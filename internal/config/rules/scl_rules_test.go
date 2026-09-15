// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"strings"
	"testing"
)

func TestResolve_SCLRule(t *testing.T) {
	rule, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}

	for _, path := range []string{"main.scl", "plc/Blocks/Motor.SCL", "src/control.scl"} {
		t.Run(path, func(t *testing.T) {
			got := rule.Resolve(path)
			if !strings.Contains(got, "Siemens SCL Review Principles") {
				t.Fatalf("Resolve(%q): expected Siemens SCL rule, got %q", path, truncate(got, 120))
			}
		})
	}
}

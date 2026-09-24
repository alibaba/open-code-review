// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package estimate

import "testing"

func TestFileTokens(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params Parameters
		plan   bool
		input  int64
		output int64
	}{
		{"defaults without plan", Parameters{}, false, 14700, 4900},
		{"defaults with plan", Parameters{}, true, 16800, 5300},
		{"custom overhead", Parameters{PromptOverheadTokens: 8000}, true, 64800, 5300},
		{"custom output", Parameters{OutputTokensPerRound: 3000}, true, 16800, 21400},
		{"both overrides", Parameters{8000, 3000}, false, 56700, 21000},
		{"negative fallback", Parameters{-1, -1}, true, 16800, 5300},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, output := tc.params.FileTokens(100, tc.plan)
			if input != tc.input || output != tc.output {
				t.Fatalf("FileTokens = (%d, %d), want (%d, %d)", input, output, tc.input, tc.output)
			}
		})
	}
}

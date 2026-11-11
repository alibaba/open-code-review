// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"testing"
)

func TestResumeCodeAccessProfile(test *testing.T) {
	for _, profile := range []string{"", CodeAccessCurrentOnly, CodeAccessComparisonSides, "future/v2"} {
		test.Run(profile, func(test *testing.T) {
			state := parentState(func(manifest *RunManifest) {
				manifest.Execution.CodeAccessProfile = profile
			})
			got, err := state.CodeAccessProfile()
			if profile == "future/v2" {
				if err == nil || state.ValidateResume(request(nil)) == nil {
					test.Fatal("unknown profile accepted")
				}
				return
			}
			want := profile
			if want == "" {
				want = CodeAccessCurrentOnly
			}
			if err != nil || got != want || state.ValidateResume(request(nil)) != nil {
				test.Fatalf("profile: %q %v", got, err)
			}
			raw, err := json.Marshal(state.Manifest)
			if err != nil {
				test.Fatal(err)
			}
			var decoded RunManifest
			if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Execution.CodeAccessProfile != profile {
				test.Fatalf("profile lost in JSON round trip: %v", err)
			}
		})
	}
	for _, state := range []*ResumeState{nil, {}} {
		if got, err := state.CodeAccessProfile(); err != nil || got != CodeAccessCurrentOnly {
			test.Fatalf("legacy profile: %q %v", got, err)
		}
	}
	state := parentState(func(manifest *RunManifest) {
		manifest.Execution.CodeAccessProfile = CodeAccessComparisonSides
		manifest.Input.Mode = InputModeWorkspace
	})
	if _, err := state.CodeAccessProfile(); err == nil {
		test.Fatal("workspace comparison profile accepted")
	}
}

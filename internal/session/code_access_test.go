// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResumeComparisonEndpoints(test *testing.T) {
	for _, profile := range []string{"", CodeAccessCurrentOnly, CodeAccessComparisonSides} {
		test.Run(profile, func(test *testing.T) {
			for _, scenario := range []struct {
				name       string
				mode       string
				parentBase string
				parentHead string
				base       string
				head       string
				reject     bool
			}{
				{name: "same range", mode: InputModeRange, parentBase: "base", parentHead: "head", base: "base", head: "head"},
				{name: "changed base", mode: InputModeRange, parentBase: "base", parentHead: "head", base: "other", head: "head", reject: true},
				{name: "changed head", mode: InputModeRange, parentBase: "base", parentHead: "head", base: "base", head: "other", reject: true},
				{name: "same commit", mode: InputModeCommit, parentBase: "base", parentHead: "head", base: "base", head: "head"},
				{name: "same root", mode: InputModeCommit, parentHead: "root", head: "root"},
				{name: "changed root", mode: InputModeCommit, parentHead: "root", head: "other", reject: true},
				{name: "empty to nonempty base", mode: InputModeCommit, parentHead: "head", base: "base", head: "head", reject: true},
				{name: "nonempty to empty base", mode: InputModeCommit, parentBase: "base", parentHead: "head", head: "head", reject: true},
				{name: "missing parent head", mode: InputModeRange, parentBase: "base", base: "base", head: "head", reject: true},
				{name: "missing current head", mode: InputModeRange, parentBase: "base", parentHead: "head", base: "base", reject: true},
				{name: "both heads missing", mode: InputModeRange, parentBase: "base", base: "base", reject: true},
				{name: "missing parent range base", mode: InputModeRange, parentHead: "head", base: "base", head: "head", reject: true},
				{name: "missing current range base", mode: InputModeRange, parentBase: "base", parentHead: "head", head: "head", reject: true},
				{name: "both range bases missing", mode: InputModeRange, parentHead: "head", head: "head", reject: true},
			} {
				test.Run(scenario.name, func(test *testing.T) {
					state := parentState(func(manifest *RunManifest) {
						manifest.Execution.CodeAccessProfile = profile
						manifest.Input.Mode = scenario.mode
						manifest.Input.ResolvedBase = scenario.parentBase
						manifest.Input.ResolvedHead = scenario.parentHead
					})
					req := request(func(req *ResumeRequest) {
						req.Identity.Mode = scenario.mode
						req.Identity.ResolvedBase = scenario.base
						req.Identity.ResolvedHead = scenario.head
					})
					err := state.ValidateResume(req)
					if profile == CodeAccessComparisonSides && scenario.reject {
						if err == nil || !strings.Contains(err.Error(), "comparison endpoints") || !strings.Contains(err.Error(), resumeHint) {
							test.Fatalf("expected actionable endpoint rejection, got %v", err)
						}
					} else if err != nil {
						test.Fatalf("compatible resume rejected: %v", err)
					}
				})
			}
		})
	}
}

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

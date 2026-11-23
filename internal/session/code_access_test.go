// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResumeComparisonEndpoints(test *testing.T) {
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
		{name: "workspace without endpoints", mode: InputModeWorkspace},
		{name: "workspace with changed head", mode: InputModeWorkspace, parentHead: "head", head: "other"},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			state := parentState(func(manifest *RunManifest) {
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
			if scenario.reject {
				if err == nil || !strings.Contains(err.Error(), "comparison endpoints") || !strings.Contains(err.Error(), resumeHint) {
					test.Fatalf("expected actionable endpoint rejection, got %v", err)
				}
			} else if err != nil {
				test.Fatalf("compatible resume rejected: %v", err)
			}
		})
	}
}

func TestResumeIgnoresLegacyCodeAccessProfile(test *testing.T) {
	for _, profile := range []string{"", "current-only/v1", "comparison-sides/v1"} {
		test.Run(profile, func(test *testing.T) {
			state := parentState(nil)
			stored := struct {
				*RunManifest
				Execution struct {
					ManifestExecution
					Profile string `json:"code_access_profile,omitempty"`
				} `json:"execution"`
			}{RunManifest: state.Manifest}
			stored.Execution.ManifestExecution = state.Manifest.Execution
			stored.Execution.Profile = profile
			raw, err := json.Marshal(stored)
			if err != nil {
				test.Fatal(err)
			}
			var decoded RunManifest
			if err := json.Unmarshal(raw, &decoded); err != nil {
				test.Fatal(err)
			}
			state.Manifest = &decoded
			if err := state.ValidateResume(request(nil)); err != nil {
				test.Fatalf("legacy metadata prevented resume: %v", err)
			}
			changed := request(func(req *ResumeRequest) { req.Identity.ResolvedHead = "other" })
			if err := state.ValidateResume(changed); err == nil || !strings.Contains(err.Error(), "comparison endpoints changed") {
				test.Fatalf("legacy metadata bypassed endpoint validation: %v", err)
			}
			raw, err = json.Marshal(decoded)
			if err != nil || strings.Contains(string(raw), "code_access_profile") {
				test.Fatalf("retired profile was persisted: %s %v", raw, err)
			}
		})
	}
}

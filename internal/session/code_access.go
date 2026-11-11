// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import "fmt"

const (
	CodeAccessCurrentOnly     = "current-only/v1"
	CodeAccessComparisonSides = "comparison-sides/v1"
)

func (state *ResumeState) CodeAccessProfile() (string, error) {
	if state == nil || state.Manifest == nil {
		return CodeAccessCurrentOnly, nil
	}
	profile := state.Manifest.Execution.CodeAccessProfile
	switch profile {
	case "", CodeAccessCurrentOnly:
		return CodeAccessCurrentOnly, nil
	case CodeAccessComparisonSides:
		mode := state.Manifest.Input.Mode
		if mode != ReviewModeRange && mode != ReviewModeCommit {
			return "", fmt.Errorf("comparison code access is not supported for review mode %q", mode)
		}
		return profile, nil
	default:
		return "", fmt.Errorf("unsupported code access profile %q; %s", profile, resumeHint)
	}
}

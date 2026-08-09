// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import "testing"

// TestAgentGettersNilSafe covers the nil-safe guard paths of the small
// ResultProvider getters, which the happy-path budget/manifest tests do not
// exercise.
func TestAgentGettersNilSafe(t *testing.T) {
	a := &Agent{}

	if got := a.SessionID(); got != "" {
		t.Errorf("SessionID() on session-less agent = %q, want empty", got)
	}
	if got := a.RunManifest(); got != nil {
		t.Errorf("RunManifest() on session-less agent = %v, want nil", got)
	}

	var nilAgent *Agent
	if got := nilAgent.SessionID(); got != "" {
		t.Errorf("nil-receiver SessionID() = %q, want empty", got)
	}
	if got := nilAgent.RunManifest(); got != nil {
		t.Errorf("nil-receiver RunManifest() = %v, want nil", got)
	}
}

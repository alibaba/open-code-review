// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import "testing"

// TestScanGettersOnEmptyAgent exercises the small ResultProvider getters that
// scan implements as constants or nil-safe guards, so review and scan can share
// the output pipeline.
func TestScanGettersOnEmptyAgent(t *testing.T) {
	a := &Agent{}

	if got := a.SessionID(); got != "" {
		t.Errorf("SessionID() on session-less agent = %q, want empty", got)
	}
	if got := a.RunManifest(); got != nil {
		t.Errorf("RunManifest() = %v, want nil", got)
	}
	if a.BudgetExceeded() {
		t.Errorf("BudgetExceeded() = true, want false")
	}

	// Nil receiver must not panic for SessionID (guarded).
	var nilAgent *Agent
	if got := nilAgent.SessionID(); got != "" {
		t.Errorf("nil-receiver SessionID() = %q, want empty", got)
	}
}

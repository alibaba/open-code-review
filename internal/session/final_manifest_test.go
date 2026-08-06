// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import "testing"

func TestFinalManifest(t *testing.T) {
	// Nil receiver must not panic.
	var nilSH *SessionHistory
	if got := nilSH.FinalManifest(); got != nil {
		t.Errorf("nil-receiver FinalManifest() = %v, want nil", got)
	}

	// A session with no frozen manifest (legacy/scan) returns nil.
	sh := &SessionHistory{}
	if got := sh.FinalManifest(); got != nil {
		t.Errorf("FinalManifest() with no manifest = %v, want nil", got)
	}

	// SetFinalManifest is a no-op on a nil receiver.
	nilSH.SetFinalManifest(&RunManifest{RunID: "ignored"})

	// After storing, FinalManifest returns a cloned copy carrying the data.
	sh.SetFinalManifest(&RunManifest{RunID: "run-123", Operation: "review"})
	got := sh.FinalManifest()
	if got == nil {
		t.Fatal("FinalManifest() after set = nil, want value")
	}
	if got.RunID != "run-123" || got.Operation != "review" {
		t.Errorf("FinalManifest() = %+v, want RunID=run-123 Operation=review", got)
	}
}

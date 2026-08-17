// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"strings"
	"testing"
)

// TestValidateScanOptions covers every branch of ResumeState.ValidateScanOptions:
// the nil receiver, the missing/mismatched review mode errors, the scan-path
// scope mismatch, and the two success paths (with and without a recorded scope).
func TestValidateScanOptions(t *testing.T) {
	t.Run("nil state is a no-op", func(t *testing.T) {
		var s *ResumeState
		if err := s.ValidateScanOptions([]string{"a"}); err != nil {
			t.Errorf("nil state = %v, want nil", err)
		}
	})

	t.Run("missing review mode metadata errors", func(t *testing.T) {
		s := &ResumeState{SessionID: "sess-1", ReviewMode: ""}
		err := s.ValidateScanOptions(nil)
		if err == nil || !strings.Contains(err.Error(), "missing review mode metadata") {
			t.Errorf("err = %v, want missing review mode metadata", err)
		}
	})

	t.Run("non-scan mode errors", func(t *testing.T) {
		s := &ResumeState{SessionID: "sess-1", ReviewMode: ReviewModeRange}
		err := s.ValidateScanOptions(nil)
		if err == nil || !strings.Contains(err.Error(), "does not match current mode") {
			t.Errorf("err = %v, want mode mismatch", err)
		}
	})

	t.Run("scope mismatch errors", func(t *testing.T) {
		s := &ResumeState{
			ReviewMode:       ReviewModeFullScan,
			HasScanPathScope: true,
			ScanPaths:        []string{"src"},
		}
		err := s.ValidateScanOptions([]string{"docs"})
		if err == nil || !strings.Contains(err.Error(), "scan path scope") {
			t.Errorf("err = %v, want scan path scope mismatch", err)
		}
	})

	t.Run("matching scope succeeds after normalization", func(t *testing.T) {
		s := &ResumeState{
			ReviewMode:       ReviewModeFullScan,
			HasScanPathScope: true,
			ScanPaths:        []string{"src"},
		}
		// "./src/" normalizes to "src", so the scopes match.
		if err := s.ValidateScanOptions([]string{"./src/"}); err != nil {
			t.Errorf("matching scope = %v, want nil", err)
		}
	})

	t.Run("no recorded scope skips the scope check", func(t *testing.T) {
		s := &ResumeState{
			ReviewMode:       ReviewModeFullScan,
			HasScanPathScope: false,
			ScanPaths:        nil,
		}
		if err := s.ValidateScanOptions([]string{"anything"}); err != nil {
			t.Errorf("no scope = %v, want nil", err)
		}
	})
}

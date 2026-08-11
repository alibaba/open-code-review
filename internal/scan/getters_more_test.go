// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

// TestScanAgent_SessionID_Persistent covers the non-empty return of SessionID:
// a session with a JSONL writer reports its persisted ID.
func TestScanAgent_SessionID_Persistent(t *testing.T) {
	setTestHome(t, t.TempDir())
	sess := session.New(t.TempDir(), "", "model-x", session.SessionOptions{
		ReviewMode: session.ReviewModeFullScan,
	})
	if !sess.HasPersistence() {
		t.Skip("session persistence unavailable in this environment")
	}
	a := &Agent{session: sess}
	if got := a.SessionID(); got != sess.SessionID {
		t.Errorf("SessionID() = %q, want %q", got, sess.SessionID)
	}
}

// TestScanAgent_ResumeInfo covers both the nil and non-nil branches, and that
// the returned pointer is a defensive copy.
func TestScanAgent_ResumeInfo(t *testing.T) {
	a := &Agent{}
	if got := a.ResumeInfo(); got != nil {
		t.Errorf("ResumeInfo() on fresh agent = %v, want nil", got)
	}
	a.resumeInfo = &session.ResumeInfo{ResumedFrom: "sess-1", ReusedFiles: 3}
	got := a.ResumeInfo()
	if got == nil || got.ResumedFrom != "sess-1" || got.ReusedFiles != 3 {
		t.Fatalf("ResumeInfo() = %+v, want copy of resumeInfo", got)
	}
	if got == a.resumeInfo {
		t.Error("ResumeInfo() must return a copy, not the internal pointer")
	}
}

// TestScanAgent_Fingerprints covers initScanFingerprints (map build) and
// scanItemFingerprint's cached-hit branch versus the fallback compute.
func TestScanAgent_Fingerprints(t *testing.T) {
	a := &Agent{}
	items := []model.ScanItem{
		{Path: "a.go", Content: "package a\n"},
		{Path: "b.go", Content: "package b\n"},
	}
	a.initScanFingerprints(items)
	if len(a.scanFingerprints) != 2 {
		t.Fatalf("scanFingerprints size = %d, want 2", len(a.scanFingerprints))
	}
	// Cached hit: matches the map value for a known path.
	if got, want := a.scanItemFingerprint(items[0]), a.scanFingerprints["a.go"]; got != want {
		t.Errorf("cached fingerprint = %q, want %q", got, want)
	}
	// Fallback compute: an item not in the map still gets a stable fingerprint.
	other := model.ScanItem{Path: "c.go", Content: "package c\n"}
	if got := a.scanItemFingerprint(other); got == "" || got != scanItemFingerprint(other) {
		t.Errorf("fallback fingerprint = %q, want computed value", got)
	}

	// initScanFingerprints with no items is a no-op (leaves map nil).
	empty := &Agent{}
	empty.initScanFingerprints(nil)
	if empty.scanFingerprints != nil {
		t.Errorf("scanFingerprints = %v, want nil for empty items", empty.scanFingerprints)
	}
}

// TestResumedFromSession covers the nil and non-nil resume-state branches.
func TestResumedFromSession(t *testing.T) {
	if got := resumedFromSession(nil); got != "" {
		t.Errorf("resumedFromSession(nil) = %q, want empty", got)
	}
	got := resumedFromSession(&session.ResumeState{SessionID: "prev-123"})
	if got != "prev-123" {
		t.Errorf("resumedFromSession = %q, want prev-123", got)
	}
}

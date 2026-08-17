// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"runtime"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
)

// TestParseRecordTime covers the empty, valid-RFC3339, and garbage branches.
func TestParseRecordTime(t *testing.T) {
	if got := parseRecordTime(""); !got.IsZero() {
		t.Errorf("parseRecordTime(\"\") = %v, want zero", got)
	}
	want := time.Date(2026, 8, 5, 10, 30, 0, 0, time.UTC)
	if got := parseRecordTime("2026-08-05T10:30:00Z"); !got.Equal(want) {
		t.Errorf("parseRecordTime(RFC3339) = %v, want %v", got, want)
	}
	if got := parseRecordTime("not-a-timestamp"); !got.IsZero() {
		t.Errorf("parseRecordTime(garbage) = %v, want zero", got)
	}
}

// TestSessionsDir_HomeUnset covers the os.UserHomeDir error branch by clearing
// the HOME environment variable.
func TestSessionsDir_HomeUnset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME is not the home-dir source on Windows")
	}
	t.Setenv("HOME", "")
	if _, err := SessionsDir(t.TempDir()); err == nil {
		t.Fatal("SessionsDir with HOME unset should error")
	}
}

// TestLoadSummary_HomeUnset covers the SessionFilePath error branch in
// LoadSummary (and, by extension, LoadDetail) when the home dir cannot resolve.
func TestLoadSummary_HomeUnset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME is not the home-dir source on Windows")
	}
	t.Setenv("HOME", "")
	if _, err := LoadSummary(t.TempDir(), "sess-1"); err == nil {
		t.Fatal("LoadSummary with HOME unset should error")
	}
	if _, _, err := LoadDetail(t.TempDir(), "sess-1"); err == nil {
		t.Fatal("LoadDetail with HOME unset should error")
	}
}

// TestManifest_NilReceiver covers the sh == nil branch of Manifest.
func TestManifest_NilReceiver(t *testing.T) {
	var sh *SessionHistory
	if got := sh.Manifest(); got != nil {
		t.Errorf("(nil).Manifest() = %v, want nil", got)
	}
}

// TestRecordReviewItem_NilReceiver covers the sh == nil guard on all three
// checkpoint recorders (no panic, no-op).
func TestRecordReviewItem_NilReceiver(t *testing.T) {
	var sh *SessionHistory
	sh.RecordReviewItemDone("a.go", "", "", "fp", nil)
	sh.RecordReviewItemReused("a.go", "", "", "fp", "src", nil)
	sh.RecordReviewItemFailed("a.go", "", "", "fp", "boom")
}

// TestRecordReviewItem_EmptyFilePathUsesNewPath covers the filePath == "" →
// filePath = newPath branch of each checkpoint recorder. Without persistence the
// recorders still create the in-memory FileSession keyed by newPath.
func TestRecordReviewItem_EmptyFilePathUsesNewPath(t *testing.T) {
	sh := New(t.TempDir(), "main", "test-model", SessionOptions{})

	sh.RecordReviewItemDone("", "old.go", "done.go", "fp1", nil)
	if _, ok := sh.FileSessions["done.go"]; !ok {
		t.Error("RecordReviewItemDone with empty filePath should key FileSession by newPath")
	}

	sh.RecordReviewItemReused("", "old.go", "reused.go", "fp2", "src", []model.LlmComment{{Content: "x"}})
	if _, ok := sh.FileSessions["reused.go"]; !ok {
		t.Error("RecordReviewItemReused with empty filePath should key FileSession by newPath")
	}

	sh.RecordReviewItemFailed("", "old.go", "failed.go", "fp3", "boom")
	if _, ok := sh.FileSessions["failed.go"]; !ok {
		t.Error("RecordReviewItemFailed with empty filePath should key FileSession by newPath")
	}
}

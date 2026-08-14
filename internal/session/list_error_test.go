// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"os"
	"path/filepath"
	"testing"
)

// TestListSessions_DirIsFile covers the ReadDir error branch (a non-NotExist
// error): when the computed sessions dir path is occupied by a regular file,
// os.ReadDir fails with ENOTDIR and ListSessions must surface it.
func TestListSessions_DirIsFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()

	dir, err := SessionsDir(repoDir)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	// Occupy the sessions-dir path with a file so ReadDir cannot treat it as a dir.
	if err := os.WriteFile(dir, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file at dir path: %v", err)
	}

	if _, err := ListSessions(repoDir); err == nil {
		t.Fatal("ListSessions should error when the sessions path is a file")
	}
}

// TestRecordToItem covers the non-item type (returns false) and the
// empty-FilePath fallback to NewPath for a recognized item record.
func TestRecordToItem(t *testing.T) {
	if _, ok := recordToItem(summaryRecord{Type: "session_start"}); ok {
		t.Error("session_start should not convert to an item")
	}

	item, ok := recordToItem(summaryRecord{
		Type:    "review_item_done",
		NewPath: "renamed.go",
	})
	if !ok {
		t.Fatal("review_item_done should convert to an item")
	}
	if item.FilePath != "renamed.go" {
		t.Errorf("FilePath = %q, want NewPath fallback %q", item.FilePath, "renamed.go")
	}
	if item.Type != "done" {
		t.Errorf("Type = %q, want %q", item.Type, "done")
	}
}

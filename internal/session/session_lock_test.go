// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionLockReportsActiveOwner(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "session.jsonl")
	lock, err := acquireSessionLock(sessionPath)
	if err != nil {
		t.Fatalf("acquireSessionLock: %v", err)
	}
	if !lock.supported {
		t.Skip("platform does not support session activity locks")
	}
	t.Cleanup(func() { _ = lock.close() })

	active, err := IsSessionActive(sessionPath)
	if err != nil {
		t.Fatalf("IsSessionActive while held: %v", err)
	}
	if !active {
		t.Fatal("session should be active while its lock is held")
	}

	if _, err := acquireSessionLock(sessionPath); !errors.Is(err, errSessionAlreadyActive) {
		t.Fatalf("second acquire error = %v, want %v", err, errSessionAlreadyActive)
	}

	if err := lock.close(); err != nil {
		t.Fatalf("close session lock: %v", err)
	}
	active, err = IsSessionActive(sessionPath)
	if err != nil {
		t.Fatalf("IsSessionActive after close: %v", err)
	}
	if active {
		t.Fatal("session should be inactive after its lock is released")
	}
}

func TestSessionLockMissingOrStaleFileIsInactive(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "session.jsonl")

	active, err := IsSessionActive(sessionPath)
	if err != nil {
		t.Fatalf("IsSessionActive without lock file: %v", err)
	}
	if active {
		t.Fatal("missing lock file should be inactive")
	}

	if err := os.WriteFile(sessionLockPath(sessionPath), []byte("stale"), 0600); err != nil {
		t.Fatalf("write stale lock file: %v", err)
	}
	active, err = IsSessionActive(sessionPath)
	if err != nil {
		t.Fatalf("IsSessionActive with stale lock file: %v", err)
	}
	if active {
		t.Fatal("unheld stale lock file should be inactive")
	}
}

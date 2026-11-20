// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionIsActive_MissingSidecarIsInactive(t *testing.T) {
	// A session written by an older build has no sidecar at all. That is the
	// normal case for every session predating liveness tracking, and it must
	// read as finished rather than as an error the viewer refuses to render.
	path := filepath.Join(t.TempDir(), "old.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if SessionIsActive(path) {
		t.Error("session with no sidecar reported active")
	}
}

func TestSessionIsActive_HeldLockIsActive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.jsonl")
	lock, err := holdActiveLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()

	if !SessionIsActive(path) {
		t.Error("session with a held lock reported inactive")
	}
}

func TestSessionIsActive_ReleasedLockIsInactive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.jsonl")
	lock, err := holdActiveLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if SessionIsActive(path) {
		t.Error("session reported active after its run released the lock")
	}
}

// The sidecar has to outlive the run, or "finished" and "never ran here" become
// indistinguishable and every archived session would need the write permission
// a read-only viewer does not have.
func TestActiveLock_SidecarSurvivesClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.jsonl")
	lock, err := holdActiveLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(activeLockPath(path)); err != nil {
		t.Errorf("sidecar removed on close: %v", err)
	}
}

func TestActiveLock_CloseIsIdempotent(t *testing.T) {
	// Every exit path calls closeFile, and a nil handle must not panic there.
	var nilLock *activeLock
	if err := nilLock.Close(); err != nil {
		t.Errorf("closing a nil lock returned %v", err)
	}

	path := filepath.Join(t.TempDir(), "run.jsonl")
	lock, err := holdActiveLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Errorf("second close returned %v", err)
	}
}

// A session file and its sidecar live in the same directory and are told apart
// only by suffix, so a collision would make the viewer probe the wrong file.
func TestActiveLockPath_DoesNotCollideWithSessionFile(t *testing.T) {
	got := activeLockPath("/x/y/run.jsonl")
	if want := "/x/y/run.lock"; got != want {
		t.Errorf("activeLockPath = %q, want %q", got, want)
	}
}

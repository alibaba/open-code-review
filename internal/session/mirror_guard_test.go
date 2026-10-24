// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"context"
	"os"
	"sync"
	"testing"
)

// recordingBackend remembers the keys it was asked to store, so a test can tell
// whether a session was mirrored.
type recordingBackend struct {
	mu   sync.Mutex
	puts []string
}

func (r *recordingBackend) Name() string { return "recording" }

func (r *recordingBackend) Put(_ context.Context, key string, _ []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.puts = append(r.puts, key)
	return nil
}

func (r *recordingBackend) Get(context.Context, string) ([]byte, error) {
	return nil, os.ErrNotExist
}

func (r *recordingBackend) List(context.Context, string) ([]string, error) { return nil, nil }

func (r *recordingBackend) Delete(context.Context, string) error { return nil }

func (r *recordingBackend) stored() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.puts...)
}

// TestFinalizeDoesNotUploadAnUnfinalisedSession pins that a session whose
// session_end record could not be written stays in the local cache instead of
// being mirrored. The store is the copy the other machines read from, so
// uploading a half-written file would spread the damage rather than contain it.
// The failure is forced by closing the file out from under the buffered writer,
// which makes the Flush inside WriteSessionEnd error.
func TestFinalizeDoesNotUploadAnUnfinalisedSession(t *testing.T) {
	setTestHome(t, t.TempDir())

	backend := &recordingBackend{}
	ConfigureStorage(backend)
	t.Cleanup(func() { ConfigureStorage(nil) })

	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	if sh.persist == nil || sh.persist.file == nil {
		t.Fatal("expected an open persist writer")
	}
	if err := sh.persist.file.Close(); err != nil {
		t.Fatalf("pre-close session file: %v", err)
	}

	if err := sh.Finalize(); err == nil {
		t.Fatal("Finalize returned nil despite a write failure")
	}
	if puts := backend.stored(); len(puts) != 0 {
		t.Errorf("an unfinalised session was uploaded: %v", puts)
	}
}

// TestFinalizeUploadsAFinalisedSession keeps the guard honest from the other
// side: when the session_end record is written, the mirror still runs.
func TestFinalizeUploadsAFinalisedSession(t *testing.T) {
	setTestHome(t, t.TempDir())

	backend := &recordingBackend{}
	ConfigureStorage(backend)
	t.Cleanup(func() { ConfigureStorage(nil) })

	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	if err := sh.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if puts := backend.stored(); len(puts) != 1 {
		t.Errorf("a finalised session was not uploaded exactly once: %v", puts)
	}
}

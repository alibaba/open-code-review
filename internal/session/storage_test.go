// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/storage"
)

// configureFileStore installs a filesystem-backed mirror at root and returns the
// root plus the backend. Every test that installs a backend must clear it again,
// or the next test in the package inherits a mirror it did not ask for.
func configureFileStore(t *testing.T) (string, storage.Backend) {
	t.Helper()
	root := t.TempDir()
	backend, err := storage.Open(storage.Config{Type: storage.TypeFilesystem, Path: root})
	if err != nil {
		t.Fatalf("open filesystem backend: %v", err)
	}
	if backend == nil {
		t.Fatal("filesystem configuration produced no backend")
	}
	ConfigureStorage(backend)
	t.Cleanup(func() { ConfigureStorage(nil) })
	return root, backend
}

// objectKeyFor returns the key one session's object is stored under.
func objectKeyFor(t *testing.T, repoDir, sessionID string) string {
	t.Helper()
	key, err := sessionObjectKey(repoDir, sessionID)
	if err != nil {
		t.Fatalf("sessionObjectKey: %v", err)
	}
	return key
}

// runSession writes a small session for repoDir and finalizes it, returning its
// id and local path.
func runSession(t *testing.T, repoDir string) (string, string) {
	t.Helper()
	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeWorkspace,
		LLMSource:  "test",
	})
	sh.RecordReviewItemDone("internal/a.go", "", "", "fingerprint-a", []model.LlmComment{
		{Path: "internal/a.go", Content: "a finding"},
	})
	if err := sh.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	path, err := SessionFilePath(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("SessionFilePath: %v", err)
	}
	return sh.SessionID, path
}

// captureStderr returns what one call writes to stderr, which is where mirror
// warnings go.
func captureStderr(t *testing.T, run func()) string {
	t.Helper()
	original := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = writer
	defer func() { os.Stderr = original }()

	run()
	if err := writer.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return string(data)
}

// failingBackend fails every operation, standing in for an unreachable or
// rejecting store.
type failingBackend struct{ err error }

func (f failingBackend) Name() string { return "failing" }

func (f failingBackend) Put(context.Context, string, []byte) error { return f.err }

func (f failingBackend) Get(context.Context, string) ([]byte, error) { return nil, f.err }

func (f failingBackend) List(context.Context, string) ([]string, error) { return nil, f.err }

func (f failingBackend) Delete(context.Context, string) error { return f.err }

func TestFinalizeUploadsSessionToStore(t *testing.T) {
	repoDir := t.TempDir()
	root, backend := configureFileStore(t)

	sessionID, path := runSession(t, repoDir)

	local, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read local session: %v", err)
	}
	object, err := backend.Get(context.Background(), objectKeyFor(t, repoDir, sessionID))
	if err != nil {
		t.Fatalf("session was not uploaded: %v", err)
	}
	if string(object) != string(local) {
		t.Errorf("the uploaded object does not match the local file:\nobject: %d bytes\nlocal:  %d bytes",
			len(object), len(local))
	}
	if !strings.Contains(string(object), "session_end") {
		t.Error("the uploaded object does not contain the final record")
	}
	// The object lives at a path derived from the repository, so a bucket keeps
	// one directory per repository.
	expected := filepath.Join(root, "sessions", encodeRepoPath(repoDir), sessionID+".jsonl")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("object not stored at %s: %v", expected, err)
	}
}

func TestFinalizeWithoutStoreWritesNothing(t *testing.T) {
	repoDir := t.TempDir()
	if StorageBackend() != nil {
		t.Fatal("a backend is configured outside a mirror test")
	}
	sessionID, path := runSession(t, repoDir)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("local session missing: %v", err)
	}
	_ = sessionID
}

func TestFinalizeReportsUploadFailureWithoutFailingTheRun(t *testing.T) {
	repoDir := t.TempDir()
	ConfigureStorage(failingBackend{err: errors.New("connection refused")})
	t.Cleanup(func() { ConfigureStorage(nil) })

	var sessionID string
	warning := captureStderr(t, func() {
		sessionID, _ = runSession(t, repoDir)
	})
	if !strings.Contains(warning, "upload session") || !strings.Contains(warning, "connection refused") {
		t.Errorf("stderr does not report the failed upload: %q", warning)
	}
	// The session is still complete locally: a mirror failure must not take the
	// run's own record with it.
	path, err := SessionFilePath(repoDir, sessionID)
	if err != nil {
		t.Fatalf("SessionFilePath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("local session missing after a failed upload: %v", err)
	}
	if !strings.Contains(string(data), "session_end") {
		t.Error("the local session is incomplete after a failed upload")
	}
}

func TestHydrateSessionsDownloadsMissingObjects(t *testing.T) {
	repoDir := t.TempDir()
	_, backend := configureFileStore(t)

	sessionID, path := runSession(t, repoDir)
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove the local copy: %v", err)
	}

	// A second machine asks for this repository's sessions; the object it has
	// never seen locally arrives from the store.
	downloaded, err := HydrateSessions(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("HydrateSessions: %v", err)
	}
	if downloaded != 1 {
		t.Errorf("downloaded %d sessions, want 1", downloaded)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("session was not restored locally: %v", err)
	}
	object, err := backend.Get(context.Background(), objectKeyFor(t, repoDir, sessionID))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read restored session: %v", err)
	}
	if string(restored) != string(object) {
		t.Error("the downloaded file does not match the stored object")
	}
}

func TestHydrateSessionsKeepsExistingFiles(t *testing.T) {
	repoDir := t.TempDir()
	_, backend := configureFileStore(t)

	sessionID, path := runSession(t, repoDir)
	// A newer local copy, e.g. a run that was still writing when the object was
	// uploaded, must not be replaced by the older object.
	if err := os.WriteFile(path, []byte("{\"type\":\"session_start\",\"local\":true}\n"), 0o600); err != nil {
		t.Fatalf("rewrite local session: %v", err)
	}
	if _, err := backend.Get(context.Background(), objectKeyFor(t, repoDir, sessionID)); err != nil {
		t.Fatalf("Get: %v", err)
	}

	downloaded, err := HydrateSessions(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("HydrateSessions: %v", err)
	}
	if downloaded != 0 {
		t.Errorf("downloaded %d sessions, want 0 when every object is already local", downloaded)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read local session: %v", err)
	}
	if !strings.Contains(string(data), "\"local\":true") {
		t.Error("hydration overwrote a local session file")
	}
}

func TestHydrateAllSessionsCoversEveryRepository(t *testing.T) {
	repoA, repoB := t.TempDir(), t.TempDir()
	_, backend := configureFileStore(t)

	sessionA, pathA := runSession(t, repoA)
	sessionB, pathB := runSession(t, repoB)
	for _, path := range []string{pathA, pathB} {
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove %s: %v", path, err)
		}
	}

	downloaded, err := HydrateAllSessions(context.Background())
	if err != nil {
		t.Fatalf("HydrateAllSessions: %v", err)
	}
	if downloaded != 2 {
		t.Errorf("downloaded %d sessions, want 2", downloaded)
	}
	for _, item := range []struct{ repo, id string }{{repoA, sessionA}, {repoB, sessionB}} {
		key := objectKeyFor(t, item.repo, item.id)
		if _, err := backend.Get(context.Background(), key); err != nil {
			t.Fatalf("Get %s: %v", key, err)
		}
	}
	if _, err := ListSessions(repoA); err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if _, err := ListSessions(repoB); err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
}

func TestHydrateAllSessionsIgnoresNonSessionObjects(t *testing.T) {
	_, backend := configureFileStore(t)
	ctx := context.Background()

	// A bucket shared with other data holds objects that are not sessions: a
	// top-level file, a non-JSONL file inside the sessions tree, a JSONL file
	// with no repository directory between the prefix and the file name, and one
	// nested deeper than the layout allows. All are skipped rather than written
	// to the cache.
	for _, key := range []string{
		"README.md",
		"sessions/repo/notes.txt",
		"sessions/orphan.jsonl",
		"sessions/a/b/id.jsonl",
	} {
		if err := backend.Put(ctx, key, []byte("x")); err != nil {
			t.Fatalf("Put %s: %v", key, err)
		}
	}
	downloaded, err := HydrateAllSessions(ctx)
	if err != nil {
		t.Fatalf("HydrateAllSessions: %v", err)
	}
	if downloaded != 0 {
		t.Errorf("downloaded %d non-session objects, want 0", downloaded)
	}
}

func TestListSessionsHydratesBeforeListing(t *testing.T) {
	repoDir := t.TempDir()
	_, backend := configureFileStore(t)

	sessionID, path := runSession(t, repoDir)
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove local copy: %v", err)
	}
	if _, err := backend.Get(context.Background(), objectKeyFor(t, repoDir, sessionID)); err != nil {
		t.Fatalf("Get: %v", err)
	}

	summaries, err := ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	found := false
	for _, summary := range summaries {
		if summary.SessionID == sessionID {
			found = true
		}
	}
	if !found {
		t.Errorf("ListSessions did not list the stored session %s: %+v", sessionID, summaries)
	}
}

func TestListSessionsKeepsLocalResultsWhenHydrationFails(t *testing.T) {
	repoDir := t.TempDir()
	sessionID, _ := runSession(t, repoDir)

	ConfigureStorage(failingBackend{err: errors.New("tls handshake timeout")})
	t.Cleanup(func() { ConfigureStorage(nil) })

	var summaries []Summary
	var err error
	warning := captureStderr(t, func() {
		summaries, err = ListSessions(repoDir)
	})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(summaries) != 1 || summaries[0].SessionID != sessionID {
		t.Errorf("listing lost the local session: %+v", summaries)
	}
	if !strings.Contains(warning, "tls handshake timeout") {
		t.Errorf("stderr does not report the failed hydration: %q", warning)
	}
}

func TestLoadReviewResumeStateHydratesRemoteSession(t *testing.T) {
	repoDir := t.TempDir()
	_, backend := configureFileStore(t)
	ctx := context.Background()

	sessionID, path := runSession(t, repoDir)
	object, err := backend.Get(ctx, objectKeyFor(t, repoDir, sessionID))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove local copy: %v", err)
	}

	// Resuming a run whose session was recorded elsewhere works because the
	// resume path hydrates before it opens the file by path.
	state, err := LoadReviewResumeState(repoDir, sessionID)
	if err != nil {
		t.Fatalf("LoadReviewResumeState: %v", err)
	}
	if state.SessionID != sessionID {
		t.Errorf("state.SessionID = %q, want %q", state.SessionID, sessionID)
	}
	if len(state.Items) != 1 {
		t.Errorf("replayed %d items, want the one checkpoint: %+v", len(state.Items), state.Items)
	}
	if string(object) == "" {
		t.Error("the stored object is empty")
	}
}

func TestDeleteSessionRemovesStoredObject(t *testing.T) {
	repoDir := t.TempDir()
	_, backend := configureFileStore(t)
	ctx := context.Background()

	sessionID, _ := runSession(t, repoDir)
	key := objectKeyFor(t, repoDir, sessionID)
	if _, err := backend.Get(ctx, key); err != nil {
		t.Fatalf("session was not uploaded: %v", err)
	}

	locations, err := FindSessionsByID(sessionID)
	if err != nil {
		t.Fatalf("FindSessionsByID: %v", err)
	}
	if len(locations) != 1 {
		t.Fatalf("found %d locations, want 1", len(locations))
	}
	if err := DeleteSessionAt(locations[0]); err != nil {
		t.Fatalf("DeleteSessionAt: %v", err)
	}
	if _, err := backend.Get(ctx, key); !errors.Is(err, storage.ErrNotExist) {
		t.Errorf("object still stored after delete: %v", err)
	}

	// The deleted session must not come back on the next hydration.
	downloaded, err := HydrateAllSessions(ctx)
	if err != nil {
		t.Fatalf("HydrateAllSessions: %v", err)
	}
	if downloaded != 0 {
		t.Errorf("a deleted session was downloaded again (%d objects)", downloaded)
	}
	if locations, err := FindSessionsByID(sessionID); err != nil || len(locations) != 0 {
		t.Errorf("deleted session is visible again: %v %v", locations, err)
	}
}

func TestDeleteSessionReportsStoreFailure(t *testing.T) {
	repoDir := t.TempDir()
	sessionID, _ := runSession(t, repoDir)

	ConfigureStorage(failingBackend{err: errors.New("store offline")})
	t.Cleanup(func() { ConfigureStorage(nil) })

	// FindSessionsByID hydrates first; with a failing store it still finds the
	// local file, so the delete proceeds and then reports the store failure
	// rather than claiming the session is gone everywhere.
	locations, err := FindSessionsByID(sessionID)
	if err != nil {
		t.Fatalf("FindSessionsByID: %v", err)
	}
	if len(locations) != 1 {
		t.Fatalf("found %d locations, want 1", len(locations))
	}
	err = DeleteSessionAt(locations[0])
	if err == nil {
		t.Fatal("DeleteSessionAt reported success while the store delete failed")
	}
	if !strings.Contains(err.Error(), "store offline") {
		t.Errorf("error does not name the store failure: %v", err)
	}
}

func TestSessionObjectKey(t *testing.T) {
	key, err := sessionObjectKey("/home/dev/repo", "abc-123")
	if err != nil {
		t.Fatalf("sessionObjectKey: %v", err)
	}
	if key != "sessions/home-dev-repo/abc-123.jsonl" {
		t.Errorf("key = %q", key)
	}
	if _, err := sessionObjectKey("/home/dev/repo", "../escape"); err == nil {
		t.Error("sessionObjectKey accepted a session id with a path")
	}
	if _, err := sessionObjectKey("", "abc-123"); err == nil {
		t.Error("sessionObjectKey accepted an empty repository")
	}
}

func TestHydrateSessionsIsNoOpWithoutStore(t *testing.T) {
	repoDir := t.TempDir()
	if StorageBackend() != nil {
		t.Fatal("a backend is configured outside a mirror test")
	}
	downloaded, err := HydrateSessions(context.Background(), repoDir)
	if err != nil || downloaded != 0 {
		t.Errorf("HydrateSessions = %d, %v; want a no-op", downloaded, err)
	}
	if err := RefreshFromStore(); err != nil {
		t.Errorf("RefreshFromStore without a store: %v", err)
	}
	downloaded, err = HydrateAllSessions(context.Background())
	if err != nil || downloaded != 0 {
		t.Errorf("HydrateAllSessions = %d, %v; want a no-op", downloaded, err)
	}
}

func TestHydrateSessionsSkipsEmptyRepository(t *testing.T) {
	configureFileStore(t)
	downloaded, err := HydrateSessions(context.Background(), "   ")
	if err != nil || downloaded != 0 {
		t.Errorf("HydrateSessions with an empty repo = %d, %v; want a no-op", downloaded, err)
	}
}

func TestMirrorTimeoutIsBounded(t *testing.T) {
	// A mirror write may not block a finishing run indefinitely.
	if mirrorTimeout <= 0 || mirrorTimeout > 5*time.Minute {
		t.Errorf("mirrorTimeout = %v, want a bounded value", mirrorTimeout)
	}
}

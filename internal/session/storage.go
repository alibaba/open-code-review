// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/open-code-review/internal/storage"
)

// sessionStorePrefix is the object-key prefix every session file is stored
// under, so a bucket shared with other data stays navigable.
const sessionStorePrefix = "sessions"

// mirrorTimeout bounds one upload or one hydration pass. A mirror that cannot
// finish inside it is reported and skipped: the local cache still holds the
// session, and the run must not wait on a stuck object store.
const mirrorTimeout = 60 * time.Second

var (
	backendMu sync.RWMutex
	// backend is the configured replica for session objects. It is nil for the
	// default local-only configuration, and every helper below treats nil as
	// "nothing to mirror".
	backend storage.Backend
)

// ConfigureStorage installs the backend that session records are mirrored to and
// hydrated from. A nil backend keeps every read and write local. The CLI calls
// it once at startup from the resolved configuration; tests call it directly.
func ConfigureStorage(b storage.Backend) {
	backendMu.Lock()
	defer backendMu.Unlock()
	backend = b
}

// StorageBackend returns the configured replica backend, or nil when sessions
// are local to this machine.
func StorageBackend() storage.Backend {
	backendMu.RLock()
	defer backendMu.RUnlock()
	return backend
}

// sessionObjectKey returns the object key that holds one session file.
func sessionObjectKey(repoDir, sessionID string) (string, error) {
	if err := ValidateSessionID(sessionID); err != nil {
		return "", err
	}
	if strings.TrimSpace(repoDir) == "" {
		return "", fmt.Errorf("session repo dir is required to derive an object key")
	}
	return storage.Key(sessionStorePrefix, encodeRepoPath(repoDir), sessionID+".jsonl"), nil
}

// uploadSessionFile mirrors one finished (or aborted) session file. Uploading
// reads the local file back rather than buffering what the writer produced,
// because the file on disk is the record of truth and the mirror has to match it
// byte for byte, including records written after the last flush.
func uploadSessionFile(ctx context.Context, repoDir, sessionID, path string) error {
	b := StorageBackend()
	if b == nil {
		return nil
	}
	key, err := sessionObjectKey(repoDir, sessionID)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read session file %q: %w", path, err)
	}
	if err := b.Put(ctx, key, data); err != nil {
		return fmt.Errorf("upload %s: %w", key, err)
	}
	return nil
}

// HydrateSessions downloads the session objects of one repository that are not
// in the local cache yet, and returns how many files it wrote. With no backend
// configured it is a no-op, so local-only installs never touch the network.
func HydrateSessions(ctx context.Context, repoDir string) (int, error) {
	b := StorageBackend()
	if b == nil || strings.TrimSpace(repoDir) == "" {
		return 0, nil
	}
	encoded := encodeRepoPath(repoDir)
	prefix := storage.Key(sessionStorePrefix, encoded) + "/"
	keys, err := b.List(ctx, prefix)
	if err != nil {
		return 0, err
	}
	dir, err := SessionsDir(repoDir)
	if err != nil {
		return 0, err
	}
	localPath := func(key string) (string, bool) {
		sessionID, ok := sessionIDFromKey(key, prefix)
		if !ok {
			return "", false
		}
		return filepath.Join(dir, sessionID+".jsonl"), true
	}
	return downloadObjects(ctx, b, keys, localPath)
}

// HydrateAllSessions downloads every session object missing from the local
// cache, across all repositories, and returns how many files it wrote. The
// viewer uses it to show sessions produced by other machines.
func HydrateAllSessions(ctx context.Context) (int, error) {
	b := StorageBackend()
	if b == nil {
		return 0, nil
	}
	prefix := sessionStorePrefix + "/"
	keys, err := b.List(ctx, prefix)
	if err != nil {
		return 0, err
	}
	root, err := sessionsRoot()
	if err != nil {
		return 0, err
	}
	// The local cache repeats the store layout below the prefix, so an object at
	// sessions/<encoded-repo>/<id>.jsonl lands at
	// <sessions root>/<encoded-repo>/<id>.jsonl. Anything that is not exactly
	// that shape is another tool's object and is left alone.
	localPath := func(key string) (string, bool) {
		relative, ok := strings.CutPrefix(key, prefix)
		if !ok {
			return "", false
		}
		parts := strings.Split(relative, "/")
		if len(parts) != 2 || parts[0] == "" {
			return "", false
		}
		if !strings.HasSuffix(parts[1], ".jsonl") {
			return "", false
		}
		if err := ValidateSessionID(strings.TrimSuffix(parts[1], ".jsonl")); err != nil {
			return "", false
		}
		cleaned := filepath.Clean(filepath.FromSlash(relative))
		if cleaned == "." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
			return "", false
		}
		return filepath.Join(root, cleaned), true
	}
	return downloadObjects(ctx, b, keys, localPath)
}

// downloadObjects fetches every listed key whose local file is missing. localPath
// maps a key to the file to write it to; a key it rejects is not a session
// object and is skipped.
func downloadObjects(ctx context.Context, b storage.Backend, keys []string, localPath func(string) (string, bool)) (int, error) {
	downloaded := 0
	for _, key := range keys {
		path, ok := localPath(key)
		if !ok {
			continue
		}
		if existing, err := os.Stat(path); err == nil && !existing.IsDir() {
			continue
		}
		data, err := b.Get(ctx, key)
		if err != nil {
			if errors.Is(err, storage.ErrNotExist) {
				// Removed between the listing and the fetch: nothing to hydrate.
				continue
			}
			return downloaded, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return downloaded, fmt.Errorf("create session dir: %w", err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return downloaded, fmt.Errorf("write session file %q: %w", path, err)
		}
		downloaded++
	}
	return downloaded, nil
}

// deleteSessionObject removes the mirrored copy of one session, so a deleted
// session does not come back on the next hydration.
func deleteSessionObject(ctx context.Context, encodedDir, sessionID string) error {
	b := StorageBackend()
	if b == nil {
		return nil
	}
	if err := ValidateSessionID(sessionID); err != nil {
		return err
	}
	if strings.TrimSpace(encodedDir) == "" {
		return fmt.Errorf("session encoded dir is required to derive an object key")
	}
	key := storage.Key(sessionStorePrefix, encodedDir, sessionID+".jsonl")
	if err := b.Delete(ctx, key); err != nil {
		return fmt.Errorf("delete %s: %w", key, err)
	}
	return nil
}

// sessionIDFromKey extracts the session id from an object key under prefix.
func sessionIDFromKey(key, prefix string) (string, bool) {
	remainder, ok := strings.CutPrefix(key, prefix)
	if !ok {
		return "", false
	}
	name, ok := strings.CutSuffix(remainder, ".jsonl")
	if !ok || strings.Contains(name, "/") {
		return "", false
	}
	if err := ValidateSessionID(name); err != nil {
		return "", false
	}
	return name, true
}

// hydrateSessionsForRead mirrors the sessions of one repository before a read
// path looks for them locally. The error is returned rather than reported, so a
// caller that can still serve the local cache decides what to tell the user.
func hydrateSessionsForRead(repoDir string) error {
	if StorageBackend() == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), mirrorTimeout)
	defer cancel()
	_, err := HydrateSessions(ctx, repoDir)
	return err
}

// RefreshFromStore downloads every session object that is missing from the local
// cache, and reports nothing to do when no store is configured. Callers that
// render sessions (the viewer, session listings) run it first so the cache
// reflects the whole store.
func RefreshFromStore() error { return hydrateAllForRead() }

// hydrateAllForRead mirrors every session before a read path that is not scoped
// to one repository (the viewer, and session commands that take an id alone).
func hydrateAllForRead() error {
	if StorageBackend() == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), mirrorTimeout)
	defer cancel()
	_, err := HydrateAllSessions(ctx)
	return err
}

// warnStorage reports a failed mirror operation without failing the command: the
// session record still exists locally, so the run stays usable, but the user has
// to know the durable copy is missing.
func warnStorage(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[ocr session] warning: "+format+"\n", args...)
}

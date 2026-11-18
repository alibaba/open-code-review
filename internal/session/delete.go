// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrSessionNotFound reports that no persisted session matched the id.
var ErrSessionNotFound = errors.New("session not found")

// ErrSessionOtherRepo reports that a session records a different repository.
var ErrSessionOtherRepo = errors.New("session belongs to a different repository")

// ErrSessionOutsideRoot reports that a Location falls outside the sessions directory.
var ErrSessionOutsideRoot = errors.New("session path is outside the sessions directory")

// ErrSessionUnverifiable reports that a session records no repository to check --repo against.
var ErrSessionUnverifiable = errors.New("session records no repository")

// ValidateSessionID rejects ids that are not a single path element.
func ValidateSessionID(sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("session id is required")
	}
	if sessionID != filepath.Base(sessionID) ||
		strings.ContainsAny(sessionID, `/\`) ||
		sessionID == "." || sessionID == ".." {
		return fmt.Errorf("invalid session id %q: must not contain a path", sessionID)
	}
	return nil
}

// CheckSessionRepo reports whether a session may be operated on from repoDir.
func CheckSessionRepo(repoDir, sessionID string) error {
	if err := ValidateSessionID(sessionID); err != nil {
		return err
	}
	path, err := sessionPath(repoDir, sessionID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
		}
		return fmt.Errorf("stat session %q: %w", sessionID, err)
	}
	recorded, ok := recordedRepoDir(path)
	if !ok {
		return fmt.Errorf(
			"%w: %s records no repository, so it cannot be verified as this one; delete it by id alone, without --repo",
			ErrSessionUnverifiable, sessionID,
		)
	}
	if !sameRepoPath(recorded, repoDir) {
		return fmt.Errorf("%w: %s was recorded for %s", ErrSessionOtherRepo, sessionID, recorded)
	}
	return nil
}

// sessionPath resolves a validated session id to its file inside the sessions directory.
func sessionPath(repoDir, sessionID string) (string, error) {
	dir, err := SessionsDir(repoDir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	if filepath.Dir(path) != filepath.Clean(dir) {
		return "", fmt.Errorf("invalid session id %q: resolves outside the sessions directory", sessionID)
	}
	return path, nil
}

// DeleteSession removes one persisted session for a repository.
func DeleteSession(repoDir, sessionID string) error {
	if err := CheckSessionRepo(repoDir, sessionID); err != nil {
		return err
	}
	path, err := sessionPath(repoDir, sessionID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete session %q: %w", sessionID, err)
	}
	return nil
}

// recordedRepoDir returns the working directory a session recorded, and whether it had one.
func recordedRepoDir(path string) (string, bool) {
	var cwd string
	if err := walkSessionFile(path, func(rec summaryRecord) {
		if rec.Cwd != "" {
			cwd = rec.Cwd
		}
	}); err != nil {
		return "", false
	}
	if cwd == "" {
		return "", false
	}
	return cwd, true
}

// sameRepoPath reports whether two repository paths name the same directory.
func sameRepoPath(a, b string) bool {
	ca, cb := filepath.Clean(a), filepath.Clean(b)
	if ca == cb {
		return true
	}
	fa, errA := os.Stat(ca)
	fb, errB := os.Stat(cb)
	if errA != nil || errB != nil {
		return false
	}
	return os.SameFile(fa, fb)
}

// Location is one on-disk session file, with the repository it recorded.
type Location struct {
	SessionID  string
	Path       string
	RepoDir    string
	EncodedDir string
}

// FindSessionsByID returns every session file matching an id, in any repository.
func FindSessionsByID(sessionID string) ([]Location, error) {
	if err := ValidateSessionID(sessionID); err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	root := filepath.Join(home, ".opencodereview", sessionSubDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions dir %q: %w", root, err)
	}

	var found []Location
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), sessionID+".jsonl")
		info, statErr := os.Stat(path)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return nil, fmt.Errorf("stat session %q: %w", path, statErr)
		}
		if info.IsDir() {
			continue
		}
		repoDir, _ := recordedRepoDir(path)
		found = append(found, Location{
			SessionID:  sessionID,
			Path:       path,
			RepoDir:    repoDir,
			EncodedDir: entry.Name(),
		})
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].RepoDir != found[j].RepoDir {
			return found[i].RepoDir < found[j].RepoDir
		}
		return found[i].EncodedDir < found[j].EncodedDir
	})
	return found, nil
}

// sessionsRoot returns the directory that holds every repository's sessions.
func sessionsRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".opencodereview", sessionSubDir), nil
}

// DeleteSessionAt removes one session file found by FindSessionsByID.
func DeleteSessionAt(loc Location) error {
	path, err := resolveLocationPath(loc)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrSessionNotFound, loc.SessionID)
		}
		return fmt.Errorf("delete session %q: %w", loc.SessionID, err)
	}
	return nil
}

// resolveLocationPath rebuilds a Location's path from its parts and checks the two agree.
func resolveLocationPath(loc Location) (string, error) {
	if err := ValidateSessionID(loc.SessionID); err != nil {
		return "", err
	}
	if loc.EncodedDir == "" ||
		loc.EncodedDir != filepath.Base(loc.EncodedDir) ||
		strings.ContainsAny(loc.EncodedDir, `/\`) ||
		loc.EncodedDir == "." || loc.EncodedDir == ".." {
		return "", fmt.Errorf("%w: %q is not a single sessions directory", ErrSessionOutsideRoot, loc.EncodedDir)
	}
	root, err := sessionsRoot()
	if err != nil {
		return "", err
	}
	want := filepath.Join(root, loc.EncodedDir, loc.SessionID+".jsonl")
	if filepath.Clean(loc.Path) != want {
		return "", fmt.Errorf("%w: %s is not %s", ErrSessionOutsideRoot, loc.Path, want)
	}
	return want, nil
}

// ListAllSessionIDs returns every saved session id.
func ListAllSessionIDs() ([]Location, error) {
	root, err := sessionsRoot()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions dir %q: %w", root, err)
	}

	var found []Location
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		files, readErr := os.ReadDir(filepath.Join(root, entry.Name()))
		if readErr != nil {
			continue
		}
		for _, f := range files {
			name := f.Name()
			if f.IsDir() || !strings.HasSuffix(name, ".jsonl") {
				continue
			}
			id := strings.TrimSuffix(name, ".jsonl")
			if ValidateSessionID(id) != nil {
				continue
			}
			path := filepath.Join(root, entry.Name(), name)
			repoDir, _ := recordedRepoDir(path)
			found = append(found, Location{
				SessionID:  id,
				Path:       path,
				RepoDir:    repoDir,
				EncodedDir: entry.Name(),
			})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].SessionID < found[j].SessionID })
	return found, nil
}

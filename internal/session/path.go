// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const sessionPathVersion = "v2"

// RepoSessionKey returns the collision-resistant directory name used for a
// repository's persisted sessions. The readable prefix is only for humans;
// the digest is derived from the canonical path and is the identity boundary.
func RepoSessionKey(repoDir string) string {
	canonical := canonicalRepoPath(repoDir)
	readable := encodeRepoPath(canonical)
	if len(readable) > 48 {
		readable = strings.TrimRight(readable[:48], "-")
	}
	if readable == "" {
		readable = "empty"
	}
	digest := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("%s-%s-%x", sessionPathVersion, readable, digest[:12])
}

// IsRepoSessionKey reports whether key has the shape produced by
// RepoSessionKey. Checking the complete shape avoids mistaking legacy encoded
// paths that merely begin with "v2-" for current-format directories.
func IsRepoSessionKey(key string) bool {
	if !strings.HasPrefix(key, sessionPathVersion+"-") {
		return false
	}
	separator := strings.LastIndexByte(key, '-')
	if separator <= len(sessionPathVersion) || len(key)-separator-1 != 24 {
		return false
	}
	_, err := hex.DecodeString(key[separator+1:])
	return err == nil
}

// canonicalRepoPath normalizes aliases that refer to the same repository
// before they are hashed. EvalSymlinks is best-effort because callers may ask
// for a path that has not been created yet (notably in tests).
func canonicalRepoPath(repoDir string) string {
	if repoDir == "" {
		return "empty"
	}

	p, err := filepath.Abs(repoDir)
	if err != nil {
		p = filepath.Clean(repoDir)
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

// encodeRepoPath preserves the pre-v2 directory encoding for compatibility
// with sessions written by older OCR releases. It must not be used for new
// session directories.
func encodeRepoPath(p string) string {
	if p == "" {
		return "empty"
	}

	vol := filepath.VolumeName(p)
	p = p[len(vol):]
	p = strings.TrimLeft(p, "/\\")
	p = strings.ReplaceAll(p, "/", "-")
	p = strings.ReplaceAll(p, "\\", "-")
	vol = strings.ReplaceAll(vol, ":", "_")
	result := vol + p
	if result == "" {
		return "empty"
	}
	return result
}

func sessionDirectories(repoDir string) (current, legacy string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("resolve home dir: %w", err)
	}
	root := filepath.Join(home, ".opencodereview", sessionSubDir)
	return filepath.Join(root, RepoSessionKey(repoDir)), filepath.Join(root, encodeRepoPath(repoDir)), nil
}

func sessionFileCandidates(repoDir, sessionID string) (current, legacy string, err error) {
	if sessionID == "" {
		return "", "", fmt.Errorf("session id is required")
	}
	currentDir, legacyDir, err := sessionDirectories(repoDir)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(currentDir, sessionID+".jsonl"), filepath.Join(legacyDir, sessionID+".jsonl"), nil
}

// findSessionFile returns the current-format file when present. If only a
// legacy file exists, it is accepted only when its session_start cwd belongs
// to repoDir; this prevents old colliding directories from leaking sessions.
func findSessionFile(repoDir, sessionID string) (string, error) {
	current, legacy, err := sessionFileCandidates(repoDir, sessionID)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(current); err == nil {
		return current, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if _, err := os.Stat(legacy); err == nil {
		matches, matchErr := sessionFileBelongsToRepo(legacy, repoDir)
		if matchErr != nil {
			return "", matchErr
		}
		if matches {
			return legacy, nil
		}
		return "", os.ErrNotExist
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return current, nil
}

func sessionFileBelongsToRepo(path, repoDir string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	for {
		line, readErr := reader.ReadBytes('\n')
		var rec struct {
			Type string `json:"type"`
			Cwd  string `json:"cwd"`
		}
		if json.Unmarshal(line, &rec) == nil && rec.Type == "session_start" {
			return rec.Cwd != "" && sameRepoPath(rec.Cwd, repoDir), nil
		}
		if readErr == io.EOF {
			return false, nil
		}
		if readErr != nil {
			return false, readErr
		}
	}
}

func sameRepoPath(a, b string) bool {
	return canonicalRepoPath(a) == canonicalRepoPath(b)
}

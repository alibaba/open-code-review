// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package codexauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const authFileName = "codex.json"

// CodexAuth contains the credentials and account metadata returned by OpenAI's
// OAuth service.
type CodexAuth struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	IDToken      string    `json:"id_token,omitempty"`
	AccountID    string    `json:"account_id,omitempty"`
	PlanType     string    `json:"plan_type,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// CodexStore abstracts credential persistence so another secure backend can be
// added without changing authentication or resolver code.
type CodexStore interface {
	Load() (*CodexAuth, error)
	Save(*CodexAuth) error
	Clear() error
}

// FileStore persists Codex credentials under the current user's home directory.
type FileStore struct{}

var renameFile = os.Rename

// Load reads credentials from the default file store.
func Load() (*CodexAuth, error) {
	return FileStore{}.Load()
}

// Save writes credentials to the default file store.
func Save(auth *CodexAuth) error {
	return FileStore{}.Save(auth)
}

// Clear removes credentials from the default file store.
func Clear() error {
	return FileStore{}.Clear()
}

// Path returns the location of the Codex credential file.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".opencodereview", "auth", authFileName), nil
}

// Load reads credentials from disk.
func (FileStore) Load() (*CodexAuth, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("no Codex credentials found")
		}
		return nil, fmt.Errorf("read Codex credentials: %w", err)
	}
	var auth CodexAuth
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("decode Codex credentials: %w", err)
	}
	if auth.AccessToken == "" {
		return nil, errors.New("Codex credentials do not contain an access token")
	}
	return &auth, nil
}

// Save writes credentials atomically with owner-only permissions.
func (FileStore) Save(auth *CodexAuth) error {
	if auth == nil {
		return errors.New("cannot save nil Codex credentials")
	}
	if auth.AccessToken == "" {
		return errors.New("cannot save Codex credentials without an access token")
	}

	path, err := Path()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create Codex auth directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure Codex auth directory: %w", err)
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Codex credentials: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".codex-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary credential file: %w", err)
	}
	tmpPath := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secure temporary credential file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary credential file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary credential file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary credential file: %w", err)
	}
	if err := renameFile(tmpPath, path); err != nil {
		return fmt.Errorf("replace Codex credential file: %w", err)
	}
	removeTemp = false
	return nil
}

// Clear removes locally stored credentials. It succeeds when no file exists.
func (FileStore) Clear() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove Codex credentials: %w", err)
	}
	return nil
}

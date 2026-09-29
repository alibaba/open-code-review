// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package codexauth

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func setTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func testAuth() *CodexAuth {
	return &CodexAuth{
		AccessToken:  "access-secret",
		RefreshToken: "refresh-secret",
		IDToken:      "id-secret",
		AccountID:    "acct_1234567890",
		PlanType:     "plus",
		ExpiresAt:    time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC),
	}
}

func TestFileStoreSaveLoadAndClear(t *testing.T) {
	home := setTestHome(t)
	store := FileStore{}
	want := testAuth()

	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if *got != *want {
		t.Errorf("Load() = %#v, want %#v", got, want)
	}

	path := filepath.Join(home, ".opencodereview", "auth", authFileName)
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat auth file: %v", err)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat auth directory: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := fileInfo.Mode().Perm(); perm != 0o600 {
			t.Errorf("file permission = %o, want 600", perm)
		}
		if perm := dirInfo.Mode().Perm(); perm != 0o700 {
			t.Errorf("directory permission = %o, want 700", perm)
		}
	}

	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("second Clear: %v", err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "no Codex credentials") {
		t.Fatalf("Load after Clear error = %v, want missing-credentials error", err)
	}
}

func TestFileStoreSaveSecuresTempBeforeRename(t *testing.T) {
	setTestHome(t)
	originalRename := renameFile
	t.Cleanup(func() { renameFile = originalRename })

	checked := false
	renameFile = func(oldPath, newPath string) error {
		checked = true
		info, err := os.Stat(oldPath)
		if err != nil {
			return err
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Errorf("temporary file permission at rename = %o, want 600", info.Mode().Perm())
		}
		return os.Rename(oldPath, newPath)
	}

	if err := (FileStore{}).Save(testAuth()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !checked {
		t.Fatal("rename hook was not called")
	}
}

func TestFileStoreSaveIsAtomicWhenRenameFails(t *testing.T) {
	setTestHome(t)
	store := FileStore{}
	original := testAuth()
	if err := store.Save(original); err != nil {
		t.Fatalf("initial Save: %v", err)
	}

	originalRename := renameFile
	renameFile = func(string, string) error { return errors.New("rename failed") }
	t.Cleanup(func() { renameFile = originalRename })

	replacement := testAuth()
	replacement.AccessToken = "replacement-access"
	if err := store.Save(replacement); err == nil || !strings.Contains(err.Error(), "replace Codex credential file") {
		t.Fatalf("Save error = %v, want rename error", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load original: %v", err)
	}
	if got.AccessToken != original.AccessToken {
		t.Errorf("access token after failed replacement = %q, want original", got.AccessToken)
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".codex-*.tmp"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("temporary files remain after failure: %v", matches)
	}
}

func TestFileStoreRejectsInvalidCredentials(t *testing.T) {
	setTestHome(t)
	store := FileStore{}
	if err := store.Save(nil); err == nil {
		t.Fatal("Save(nil) succeeded")
	}
	if err := store.Save(&CodexAuth{}); err == nil {
		t.Fatal("Save without access token succeeded")
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "access token") {
		t.Fatalf("Load empty auth error = %v, want access-token error", err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile malformed: %v", err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "decode Codex credentials") {
		t.Fatalf("Load malformed auth error = %v, want decode error", err)
	}
}

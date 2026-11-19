// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package tool

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRegularWorkspaceRejectsNamedPipe(t *testing.T) {
	repo := t.TempDir()
	if err := unix.Mkfifo(filepath.Join(repo, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	if file, err := OpenRegularWorkspaceFile(repo, "pipe"); err == nil {
		file.Close()
		t.Fatal("opened a non-regular file")
	}
}

func TestRegularWorkspaceSearchableDirectory(t *testing.T) {
	if directoryAccess == unix.O_RDONLY {
		t.Skip("no search-only directory descriptor")
	}
	repo := t.TempDir()
	dir := filepath.Join(repo, "known")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("readable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0111); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	file, err := OpenRegularWorkspaceFile(repo, "known/file")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil || string(data) != "readable" {
		t.Fatalf("%q: %v", data, err)
	}
}

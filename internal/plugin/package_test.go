// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func packageDirectoryLink(t *testing.T, target, link string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		return
	}
	// Junctions exercise Windows traversal without requiring symlink privileges.
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		"$ErrorActionPreference='Stop'; New-Item -ItemType Junction -Path $env:OCR_TEST_LINK -Target $env:OCR_TEST_TARGET | Out-Null")
	cmd.Env = append(os.Environ(), "OCR_TEST_LINK="+link, "OCR_TEST_TARGET="+target)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create directory junction: %v: %s", err, output)
	}
}

type packageMutationContext struct {
	context.Context
	calls  int
	at     int
	mutate func()
}

func (c *packageMutationContext) Err() error {
	c.calls++
	if c.calls == c.at {
		c.mutate()
	}
	return c.Context.Err()
}

func renamePackageDirectory(t *testing.T, source, destination string) bool {
	t.Helper()
	if err := os.Rename(source, destination); err != nil {
		// An open root handle prevents directory replacement on Windows.
		if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(32)) {
			return false
		}
		t.Fatal(err)
	}
	return true
}

func TestCopyPackageRejectsDestinationDirectoryLink(t *testing.T) {
	source, destination, outside := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "created.txt"), []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	packageDirectoryLink(t, outside, filepath.Join(destination, "nested"))
	if err := copyPackage(context.Background(), source, destination); err == nil {
		t.Fatal("copy followed a directory link outside its destination")
	}
	if _, err := os.Stat(filepath.Join(outside, "created.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside destination was written: %v", err)
	}
}

func TestCopyPackageConfinesChangingSource(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(fmt.Sprintf("nested=%t", nested), func(t *testing.T) {
			parent, destination, outside := t.TempDir(), t.TempDir(), t.TempDir()
			source := filepath.Join(parent, "source")
			replace, relative, at := source, "data.txt", 1
			if nested {
				replace, relative, at = filepath.Join(source, "nested"), "nested/data.txt", 3
			}
			if err := os.MkdirAll(replace, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(source, relative), []byte("public"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(outside, "data.txt"), []byte("private sentinel"), 0600); err != nil {
				t.Fatal(err)
			}
			blocked := false
			ctx := &packageMutationContext{Context: context.Background(), at: at, mutate: func() {
				if !renamePackageDirectory(t, replace, filepath.Join(parent, "original")) {
					blocked = true
					return
				}
				packageDirectoryLink(t, outside, replace)
			}}
			copyErr := copyPackage(ctx, source, destination)
			if ctx.calls < ctx.at {
				t.Fatal("source replacement was not attempted")
			}
			if blocked && copyErr != nil {
				t.Fatalf("copy failed after the OS blocked source replacement: %v", copyErr)
			}
			data, readErr := os.ReadFile(filepath.Join(destination, relative))
			if string(data) == "private sentinel" {
				t.Fatal("copy read outside the selected source")
			}
			if copyErr == nil && (readErr != nil || string(data) != "public") {
				t.Fatalf("successful copy did not retain selected contents: %q, %v", data, readErr)
			}
		})
	}
}

type failureReader struct{}

func (failureReader) Read([]byte) (int, error) { return 0, errors.New("read failure") }

func TestBoundedReads(t *testing.T) {
	data, err := readBounded(strings.NewReader("12345"), 5)
	if err != nil || string(data) != "12345" {
		t.Fatalf("exact-limit read = %s, %v", data, err)
	}
	_, err = readBounded(strings.NewReader("123456"), 5)
	requireError(t, err, "byte limit")
	_, err = readBounded(failureReader{}, 10)
	requireError(t, err, "read failure")
	_, err = readFileBounded(filepath.Join(t.TempDir(), "missing"), 10)
	if err == nil {
		t.Fatal("missing file read succeeded")
	}
}

func TestPackageValidationAndLimits(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	missing := t.TempDir()
	_, err := store.Install(ctx, missing, false)
	requireError(t, err, ManifestFilename)
	source := testPackage(t, "example", "1")
	if err := os.Remove(filepath.Join(source, "rules.json")); err != nil {
		t.Fatal(err)
	}
	_, err = store.Install(ctx, source, false)
	requireError(t, err, "existing regular file")
	if err := os.Mkdir(filepath.Join(source, "rules.json"), 0700); err != nil {
		t.Fatal(err)
	}
	_, err = store.Install(ctx, source, false)
	requireError(t, err, "existing regular file")
	source = testPackage(t, "example", "1")
	if err := os.WriteFile(filepath.Join(source, "bad."), []byte("bad"), 0600); err == nil {
		// Windows normalizes this filename; only systems that retain it exercise
		// portable-name rejection for directory snapshots.
		if _, err := os.Lstat(filepath.Join(source, "bad.")); err == nil {
			entries, _ := os.ReadDir(source)
			for _, entry := range entries {
				if entry.Name() == "bad." {
					_, err := store.Install(ctx, source, false)
					requireError(t, err, "nonportable filename")
				}
			}
		}
	}
	oversized := testPackage(t, "oversized", "1")
	file, err := os.Create(filepath.Join(oversized, "large.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxPackageBytes + 1); err != nil {
		t.Fatal(err)
	}
	file.Close()
	_, err = store.Install(ctx, oversized, false)
	requireError(t, err, "byte limit")
	largeManifest := testPackage(t, "manifest", "1")
	if err := os.WriteFile(filepath.Join(largeManifest, ManifestFilename), []byte(strings.Repeat(" ", maxManifestBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = store.Install(ctx, largeManifest, false)
	requireError(t, err, "byte limit")
	invalid := testPackage(t, "invalid", "1")
	if err := os.WriteFile(filepath.Join(invalid, ManifestFilename), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = store.Install(ctx, invalid, false)
	requireError(t, err, "unsupported plugin schema_version")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.Install(cancelled, source, false)
	requireError(t, err, "context canceled")
	_, err = inspectPackage(cancelled, source)
	requireError(t, err, "context canceled")
	requireError(t, copyPackage(cancelled, source, t.TempDir()), "context canceled")
	_, err = inspectPackage(ctx, filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("missing package inspection succeeded")
	}
	filePath := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(filePath, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = inspectPackage(ctx, filePath)
	requireError(t, err, "must be a directory")
	inside := testPackage(t, "inside", "1")
	_, err = NewStore(filepath.Join(inside, "store")).Install(ctx, inside, false)
	requireError(t, err, "cannot contain the plugin store")
}

func TestPackageSymlinksRejected(t *testing.T) {
	ctx := context.Background()
	source := testPackage(t, "example", "1")
	link := filepath.Join(source, "linked.txt")
	if err := os.Symlink(filepath.Join(source, "rules.json"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	store := NewStore(t.TempDir())
	_, err := store.Install(ctx, source, false)
	requireError(t, err, "symlink or special file")
	requireError(t, copyPackage(ctx, source, t.TempDir()), "symlink or special file")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	item, err := store.Install(ctx, source, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(source, "rules.json"), filepath.Join(item.Dir, "added.txt")); err != nil {
		t.Fatal(err)
	}
	_, err = store.Enabled()
	requireError(t, err, "symlink or special file")
	if err := store.SetEnabled("example", false); err != nil {
		t.Fatal(err)
	}
	requireError(t, store.SetEnabled("example", true), "symlink or special file")
	_, err = store.Install(ctx, link, false)
	if err == nil {
		t.Fatal("missing symlink source accepted")
	}
}

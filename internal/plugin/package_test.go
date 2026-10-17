// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func TestDirectArchiveInstallAndFailureCleanup(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	archive := filepath.Join(t.TempDir(), "plugin.zip")
	if err := os.WriteFile(archive, testZIP(t, "example", "1"), 0600); err != nil {
		t.Fatal(err)
	}
	installed, err := store.Install(ctx, archive, true)
	if err != nil || installed.Source != archive {
		t.Fatalf("local ZIP install = %+v, %v", installed, err)
	}
	if err := os.WriteFile(archive, testZIP(t, "example", "2"), 0600); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Update(ctx, "example")
	if err != nil || updated.Manifest.Version != "2" || !updated.Enabled {
		t.Fatalf("local ZIP update = %+v, %v", updated, err)
	}
	if err := os.WriteFile(archive, []byte("bad ZIP"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = store.Update(ctx, "example")
	requireError(t, err, "read plugin ZIP")
	items, err := store.List()
	if err != nil || len(items) != 1 || items[0].Dir != updated.Dir {
		t.Fatalf("failed archive update changed registry: %v, %v", items, err)
	}
	entries, err := os.ReadDir(filepath.Join(store.root, "packages"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("failed ZIP update left staging files: %v, %v", entries, err)
	}
}

func TestUnsafeZIPEntries(t *testing.T) {
	ctx := context.Background()
	for _, name := range []string{"../escape", "/absolute", "C:/drive", `dir\file`, "dir/../../escape", "con/rules.json", "dir./file", "dir /file"} {
		t.Run(name, func(t *testing.T) {
			stage := t.TempDir()
			data := zipEntries(t, map[string]string{name: "malicious"})
			if err := extractZIP(ctx, data, stage); err == nil {
				t.Fatalf("unsafe ZIP entry accepted: %q", name)
			}
		})
	}
	duplicate := zipEntries(t, map[string]string{"Rules.json": "a", "rules.json": "b"})
	requireError(t, extractZIP(ctx, duplicate, t.TempDir()), "duplicate filename")
	for _, mode := range []os.FileMode{os.ModeSymlink | 0777, os.ModeNamedPipe | 0600, os.ModeDevice | 0600} {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		header := &zip.FileHeader{Name: "special"}
		header.SetMode(mode)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write([]byte("target"))
		writer.Close()
		requireError(t, extractZIP(ctx, buffer.Bytes(), t.TempDir()), "symlink or special file")
	}
	data := zipEntries(t, map[string]string{"docs/": "", "docs/readme.md": "Instructions"})
	stage := t.TempDir()
	if err := extractZIP(ctx, data, stage); err != nil {
		t.Fatalf("valid nested ZIP rejected: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(stage, "docs", "readme.md"))
	if err != nil || string(content) != "Instructions" {
		t.Fatalf("nested ZIP content = %s, %v", content, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	requireError(t, extractZIP(cancelled, data, t.TempDir()), "context canceled")
}

func TestZIPExtractionRejectsDestinationSymlink(t *testing.T) {
	stage, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(stage, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	data := zipEntries(t, map[string]string{"linked/escaped.txt": "malicious"})
	if err := extractZIP(context.Background(), data, stage); err == nil {
		t.Fatal("ZIP extraction followed a symlink outside its destination")
	}
	if _, err := os.Stat(filepath.Join(outside, "escaped.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside file exists or cannot be inspected: %v", err)
	}
}

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

func TestPrepareInstallKeepsInspectedSourceRoot(t *testing.T) {
	source, outside := testPackage(t, "example", "1"), testPackage(t, "outside", "1")
	original := filepath.Join(t.TempDir(), "original")
	store := NewStore(t.TempDir())
	// The first copy callback follows the initial check and three inspection
	// callbacks (root, manifest, rules), before copying any file.
	ctx := &packageMutationContext{Context: context.Background(), at: 5, mutate: func() {
		if !renamePackageDirectory(t, source, original) {
			return
		}
		packageDirectoryLink(t, outside, source)
	}}
	prepared, err := store.PrepareInstall(ctx, source, "")
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if ctx.calls < ctx.at || prepared.Manifest.ID != "example" {
		t.Fatalf("preparation switched away from its inspected root: %s, %d callbacks", prepared.Manifest.ID, ctx.calls)
	}
}

func TestZIPBoundsAndCorruption(t *testing.T) {
	ctx := context.Background()
	requireError(t, extractZIP(ctx, make([]byte, maxArchiveBytes+1), t.TempDir()), "byte limit")
	var many bytes.Buffer
	writer := zip.NewWriter(&many)
	for i := 0; i <= maxPackageFiles; i++ {
		if _, err := writer.Create(strings.Repeat("x", i%20) + string(rune(0x100+i))); err != nil {
			t.Fatal(err)
		}
	}
	writer.Close()
	requireError(t, extractZIP(ctx, many.Bytes(), t.TempDir()), "entries")
	// A ZIP bomb is bounded by uncompressed bytes even when compression makes
	// the downloaded archive tiny.
	var bomb bytes.Buffer
	writer = zip.NewWriter(&bomb)
	entry, err := writer.Create("oversized")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(entry, io.LimitReader(zeroReader{}, maxPackageBytes+1)); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	requireError(t, extractZIP(ctx, bomb.Bytes(), t.TempDir()), "uncompressed byte limit")
	valid := zipEntries(t, map[string]string{"file": "payload"})
	// Corrupt the compressed data while retaining readable ZIP headers.
	reader, err := zip.NewReader(bytes.NewReader(valid), int64(len(valid)))
	if err != nil {
		t.Fatal(err)
	}
	offset, err := reader.File[0].DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	valid[int(offset)] ^= 0xff
	if err := extractZIP(ctx, valid, t.TempDir()); err == nil {
		t.Fatal("corrupt compressed data accepted")
	}
}

type zeroReader struct{}

func (zeroReader) Read(data []byte) (int, error) {
	clear(data)
	return len(data), nil
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

func TestLocalCatalogDigestVerification(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	stage := t.TempDir()
	source := testPackage(t, "example", "1")
	requireError(t, store.prepare(ctx, source, strings.Repeat("a", 64), stage), "not directory")
	archive := filepath.Join(t.TempDir(), "archive.zip")
	data := testZIP(t, "example", "1")
	if err := os.WriteFile(archive, data, 0600); err != nil {
		t.Fatal(err)
	}
	requireError(t, store.prepare(ctx, archive, strings.Repeat("a", 64), stage), "sha256 does not match")
	requireError(t, verifyDigest(data, "invalid"), "64 hexadecimal")
	if err := store.prepare(ctx, archive, strings.ToUpper(zipDigest(data)), stage); err != nil {
		t.Fatal(err)
	}
}

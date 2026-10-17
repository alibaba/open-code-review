// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
)

type tarFixture struct {
	header tar.Header
	body   string
}

func tarFixtures(t *testing.T, entries ...tarFixture) []byte {
	t.Helper()
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(compressed)
	for _, entry := range entries {
		header := entry.header
		header.Size = int64(len(entry.body))
		if err := writer.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(writer, entry.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func tarEntries(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	fixtures := make([]tarFixture, 0, len(entries))
	for _, name := range names {
		fixtures = append(fixtures, tarFixture{tar.Header{Name: name, Mode: 0644, Typeflag: tar.TypeReg}, entries[name]})
	}
	return tarFixtures(t, fixtures...)
}

func testTARGZ(t *testing.T, id, version string) []byte {
	t.Helper()
	manifest, err := json.Marshal(testManifest(id, version))
	if err != nil {
		t.Fatal(err)
	}
	return tarEntries(t, map[string]string{ManifestFilename: string(manifest), "rules.json": "{}", "docs/review.md": "Review instructions"})
}

func TestTARGZRejectsUnsafeEntries(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "C:/drive", "dir\\file", "dir/../../escape", "con/rules.json", "dir./file", "./../escape"} {
		t.Run(name, func(t *testing.T) {
			if err := extractTARGZ(context.Background(), tarEntries(t, map[string]string{name: "bad"}), t.TempDir()); err == nil {
				t.Fatalf("unsafe path accepted: %q", name)
			}
		})
	}
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo} {
		data := tarFixtures(t, tarFixture{header: tar.Header{Name: "special", Typeflag: kind, Linkname: "../outside"}})
		requireError(t, extractTARGZ(context.Background(), data, t.TempDir()), "link or special file")
	}
	for _, name := range []string{"/", "//"} {
		data := tarFixtures(t, tarFixture{header: tar.Header{Name: name, Typeflag: tar.TypeDir}})
		if err := extractTARGZ(context.Background(), data, t.TempDir()); err == nil {
			t.Fatalf("absolute root accepted: %q", name)
		}
	}
	data := tarEntries(t, map[string]string{"Rules.json": "a", "rules.json": "b"})
	requireError(t, extractTARGZ(context.Background(), data, t.TempDir()), "duplicate filename")
	data = tarEntries(t, map[string]string{"./rules.json": "a", "rules.json": "b"})
	requireError(t, extractTARGZ(context.Background(), data, t.TempDir()), "duplicate filename")
	stage, outside := t.TempDir(), t.TempDir()
	packageDirectoryLink(t, outside, filepath.Join(stage, "linked"))
	data = tarEntries(t, map[string]string{"linked/escaped.txt": "bad"})
	if err := extractTARGZ(context.Background(), data, stage); err == nil {
		t.Fatal("extraction followed a directory link")
	}
	if _, err := os.Stat(filepath.Join(outside, "escaped.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside destination changed: %v", err)
	}
}

func TestTARGZMetadataAndRootDirectory(t *testing.T) {
	data := tarFixtures(t,
		tarFixture{header: tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader, PAXRecords: map[string]string{"comment": "commit"}}},
		tarFixture{header: tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0755}},
		tarFixture{header: tar.Header{Name: "./docs/", Typeflag: tar.TypeDir, Mode: 0755}},
		tarFixture{header: tar.Header{Name: "./docs/readme.md", Typeflag: tar.TypeReg, Mode: 0644, Format: tar.FormatPAX}, body: "Instructions"},
	)
	stage := t.TempDir()
	if err := extractTARGZ(context.Background(), data, stage); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(stage, "docs", "readme.md"))
	if err != nil || string(content) != "Instructions" {
		t.Fatalf("extracted contents = %q, %v", content, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	requireError(t, extractTARGZ(cancelled, data, t.TempDir()), "context canceled")
	requireError(t, extractTARGZ(context.Background(), data, filepath.Join(t.TempDir(), "absent")), "absent")
}

func gzipTestBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestTARGZBoundsAndCorruption(t *testing.T) {
	ctx := context.Background()
	requireError(t, extractTARGZ(ctx, make([]byte, maxArchiveBytes+1), t.TempDir()), "byte limit")
	requireError(t, extractTARGZ(ctx, []byte("not gzip"), t.TempDir()), "read plugin tar.gz")
	data := testTARGZ(t, "example", "1")
	corrupt := append([]byte(nil), data...)
	corrupt[len(corrupt)-8] ^= 1
	for _, broken := range [][]byte{corrupt, data[:len(data)-5], gzipTestBytes(t, []byte("not tar")), append(data, data...)} {
		if err := extractTARGZ(ctx, broken, t.TempDir()); err == nil {
			t.Fatal("corrupt or concatenated archive accepted")
		}
	}
	var oversized bytes.Buffer
	writer := tar.NewWriter(&oversized)
	if err := writer.WriteHeader(&tar.Header{Name: "huge", Size: maxPackageBytes + 1, Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	// Deliberately omit the body: the declared size must fail before copying.
	requireError(t, extractTARGZ(ctx, gzipTestBytes(t, oversized.Bytes()), t.TempDir()), "uncompressed byte limit")
	fixtures := make([]tarFixture, maxPackageFiles+1)
	for i := range fixtures {
		fixtures[i].header = tar.Header{Name: fmt.Sprintf("file-%d", i), Mode: 0600, Typeflag: tar.TypeReg}
	}
	requireError(t, extractTARGZ(ctx, tarFixtures(t, fixtures...), t.TempDir()), "entries")
	requireError(t, extractTARGZ(ctx, gzipTestBytes(t, make([]byte, maxTARStreamBytes+1)), t.TempDir()), "excessive padding")
}

func TestTARGZInstallAndUpdate(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"local", "direct"} {
		t.Run(mode, func(t *testing.T) {
			archives := map[int32][]byte{1: testTARGZ(t, "example", "1"), 2: testTARGZ(t, "example", "2")}
			var version atomic.Int32
			version.Store(1)
			var corrupt atomic.Bool
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				current := version.Load()
				switch r.URL.Path {
				case "/download":
					if corrupt.Load() {
						w.Write([]byte{0x1f, 0x8b, 0})
					} else {
						w.Write(archives[current])
					}
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			store := NewStore(t.TempDir())
			store.client = server.Client()
			source := server.URL + "/download"
			switch mode {
			case "local":
				source = filepath.Join(t.TempDir(), "plugin.tar.gz")
				if err := os.WriteFile(source, archives[1], 0600); err != nil {
					t.Fatal(err)
				}
			}
			prepared, err := store.PrepareInstall(ctx, source, "")
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			installed, err := store.InstallPrepared(ctx, prepared, true)
			if err != nil || installed.Manifest.Version != "1" || installed.SHA256 != archiveDigest(archives[1]) {
				t.Fatalf("install = %+v, %v", installed, err)
			}
			version.Store(2)
			if mode == "local" {
				if err := os.WriteFile(source, archives[2], 0600); err != nil {
					t.Fatal(err)
				}
			}
			updated, err := store.Update(ctx, "example")
			if err != nil || updated.Manifest.Version != "2" || !updated.Enabled || updated.Source != source {
				t.Fatalf("update = %+v, %v", updated, err)
			}
			corrupt.Store(true)
			if mode == "local" {
				if err := os.WriteFile(source, []byte{0x1f, 0x8b, 0}, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.Update(ctx, "example"); err == nil {
				t.Fatal("corrupt archive update succeeded")
			}
			items, err := store.List()
			if err != nil || len(items) != 1 || items[0].Dir != updated.Dir {
				t.Fatalf("failed update changed registry: %v, %v", items, err)
			}
		})
	}
}

func TestTARGZInstalledExecutablePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable mode bits")
	}
	manifest, err := json.Marshal(testManifest("example", "1"))
	if err != nil {
		t.Fatal(err)
	}
	data := tarFixtures(t,
		tarFixture{header: tar.Header{Name: ManifestFilename, Mode: 0644}, body: string(manifest)},
		tarFixture{header: tar.Header{Name: "rules.json", Mode: 0644}, body: "{}"},
		tarFixture{header: tar.Header{Name: "bin/review", Mode: 06755}, body: "#!/bin/sh\nprintf 'plugin-ran'\n"},
	)
	source := filepath.Join(t.TempDir(), "plugin.tar.gz")
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	installed, err := NewStore(t.TempDir()).Install(context.Background(), source, false)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(installed.Dir, "bin", "review")
	info, err := os.Stat(executable)
	if err != nil || info.Mode().Perm()&0111 != 0111 || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		t.Fatalf("installed executable mode = %v, %v", info, err)
	}
	rule, err := os.Stat(filepath.Join(installed.Dir, "rules.json"))
	if err != nil || rule.Mode().Perm()&0111 != 0 {
		t.Fatalf("data file became executable: %v, %v", rule, err)
	}
	output, err := exec.Command(executable).Output()
	if err != nil || strings.TrimSpace(string(output)) != "plugin-ran" {
		t.Fatalf("installed executable failed: %q, %v", output, err)
	}
}

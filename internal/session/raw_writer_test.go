// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func rawPath(home, repoDir, sessionID string) string {
	return filepath.Join(home, ".opencodereview", rawSubDir, encodeRepoPath(repoDir), sessionID+".jsonl")
}

func TestNewRawFileWriter_CreatesFileUnderRawDir(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	repoDir := "/srv/repos/myapp"

	w, err := NewRawFileWriter(repoDir, "sess-123")
	if err != nil {
		t.Fatalf("NewRawFileWriter: %v", err)
	}
	defer w.Close()

	path := rawPath(home, repoDir, "sess-123")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("raw file not created at %s: %v", path, err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("raw file perm = %o, want 0600", perm)
		}
		dirInfo, err := os.Stat(filepath.Dir(path))
		if err != nil {
			t.Fatalf("stat raw dir: %v", err)
		}
		if perm := dirInfo.Mode().Perm(); perm != 0700 {
			t.Errorf("raw dir perm = %o, want 0700", perm)
		}
	}
}

func TestRawFileWriter_StampsSessionIDAndTimestamp(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	repoDir := "/srv/repos/myapp"

	w, err := NewRawFileWriter(repoDir, "sess-456")
	if err != nil {
		t.Fatalf("NewRawFileWriter: %v", err)
	}
	w.Write(llm.RawRecord{
		RequestID: "req-1",
		Model:     "m",
		Request:   json.RawMessage(`{"model":"m"}`),
	})
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(rawPath(home, repoDir, "sess-456"))
	if err != nil {
		t.Fatalf("read raw file: %v", err)
	}
	var rec llm.RawRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("raw line is not valid JSON: %v\n%s", err, data)
	}
	if rec.SessionID != "sess-456" {
		t.Errorf("session_id = %q, want stamped value", rec.SessionID)
	}
	if rec.Timestamp == "" {
		t.Error("timestamp not defaulted")
	}
	if rec.RequestID != "req-1" {
		t.Errorf("request_id = %q, want req-1", rec.RequestID)
	}
}

func TestRawFileWriter_LandsPerRecordOnDisk(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	repoDir := "/srv/repos/myapp"
	path := rawPath(home, repoDir, "sess-flush")

	w, err := NewRawFileWriter(repoDir, "sess-flush")
	if err != nil {
		t.Fatalf("NewRawFileWriter: %v", err)
	}
	w.Write(llm.RawRecord{RequestID: "req-1"})

	// Without Close: the record must already be on disk so a crashed run
	// keeps everything written up to the crash.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read before close: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("record not on disk before Close")
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestRawFileWriter_AppendsAcrossReopen pins the O_APPEND contract: reopening
// the same session ID must keep the previous records instead of truncating
// the file. Today every run gets a fresh session ID, so this guards a
// defensive path, not the live resume flow.
func TestRawFileWriter_AppendsAcrossReopen(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	repoDir := "/srv/repos/myapp"

	w, err := NewRawFileWriter(repoDir, "sess-resume")
	if err != nil {
		t.Fatalf("NewRawFileWriter: %v", err)
	}
	w.Write(llm.RawRecord{RequestID: "run1-req"})
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	w2, err := NewRawFileWriter(repoDir, "sess-resume")
	if err != nil {
		t.Fatalf("reopen NewRawFileWriter: %v", err)
	}
	w2.Write(llm.RawRecord{RequestID: "run2-req"})
	if err := w2.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(rawPath(home, repoDir, "sess-resume"))
	if err != nil {
		t.Fatalf("read raw file: %v", err)
	}
	var ids []string
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var rec llm.RawRecord
		if err := dec.Decode(&rec); err != nil {
			t.Fatalf("corrupted JSONL after reopen: %v", err)
		}
		ids = append(ids, rec.RequestID)
	}
	if len(ids) != 2 || ids[0] != "run1-req" || ids[1] != "run2-req" {
		t.Errorf("records after reopen = %v, want [run1-req run2-req]", ids)
	}
}

func TestRawFileWriter_ConcurrentWrites(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	repoDir := "/srv/repos/myapp"

	w, err := NewRawFileWriter(repoDir, "sess-conc")
	if err != nil {
		t.Fatalf("NewRawFileWriter: %v", err)
	}
	const goroutines = 32
	const perG = 10
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				w.Write(llm.RawRecord{RequestID: "r"})
			}
		}()
	}
	wg.Wait()
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(rawPath(home, repoDir, "sess-conc"))
	if err != nil {
		t.Fatalf("read raw file: %v", err)
	}
	lines := 0
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var rec llm.RawRecord
		if err := dec.Decode(&rec); err != nil {
			t.Fatalf("corrupted JSONL under concurrency: %v", err)
		}
		lines++
	}
	if lines != goroutines*perG {
		t.Errorf("lines = %d, want %d", lines, goroutines*perG)
	}
}

func TestRawFileWriter_OpenFailure(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	repoDir := "/repo"
	// Squat the repo's raw directory path with a regular file: MkdirAll
	// cannot create a directory over it, on any platform.
	blocker := filepath.Join(home, ".opencodereview", rawSubDir, encodeRepoPath(repoDir))
	if err := os.MkdirAll(filepath.Dir(blocker), 0700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if _, err := NewRawFileWriter(repoDir, "sess-x"); err == nil {
		t.Fatal("expected open failure under unusable raw dir")
	}
}

// TestRawSubDirIsSeparateFromSessions guards the isolation contract: raw
// captures must never land in the session checkpoint directory. UseTestSessions
// (run from this package's init) redirects both directories; pinning the
// literals catches a collision or silent rename, whereas comparing the two
// variables only ever saw two already-redirected strings.
func TestRawSubDirIsSeparateFromSessions(t *testing.T) {
	if rawSubDir != "test-raw" {
		t.Errorf("rawSubDir = %q, want literal \"test-raw\"", rawSubDir)
	}
	if sessionSubDir != "test-sessions" {
		t.Errorf("sessionSubDir = %q, want literal \"test-sessions\"", sessionSubDir)
	}
}

// failingRawFile swallows its first n writes with err, then behaves like a
// normal file — a transient disk failure the writer must survive.
type failingRawFile struct {
	data     []byte
	failures int
	err      error
}

func (f *failingRawFile) Write(p []byte) (int, error) {
	if f.failures > 0 {
		f.failures--
		return 0, f.err
	}
	f.data = append(f.data, p...)
	return len(p), nil
}

func (f *failingRawFile) Close() error { return nil }

// captureStderrForTest captures everything written to os.Stderr during fn.
func captureStderrForTest(t *testing.T, fn func()) string {
	t.Helper()
	r, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = wPipe
	defer func() { os.Stderr = old }()
	fn()
	wPipe.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	return string(out)
}

// TestRawFileWriter_RecoversFromTransientWriteFailure pins the recovery
// contract: one disk failure must drop only the one record in flight and warn
// once, not silently kill every later capture.
func TestRawFileWriter_RecoversFromTransientWriteFailure(t *testing.T) {
	f := &failingRawFile{failures: 2, err: errors.New("no space left on device")}
	w := &RawFileWriter{sessionID: "sess-1", file: f}
	w.encoder = json.NewEncoder(&w.buf)
	w.encoder.SetEscapeHTML(false)

	stderr := captureStderrForTest(t, func() {
		w.Write(llm.RawRecord{RequestID: "r1"})
		w.Write(llm.RawRecord{RequestID: "r2"})
		w.Write(llm.RawRecord{RequestID: "r3"})
	})
	if n := strings.Count(stderr, "raw logging failed"); n != 1 {
		t.Errorf("got %d write-failure warnings, want 1: %q", n, stderr)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// r1 and r2 hit the failure and are dropped; r3 lands after recovery.
	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(string(f.data)), "\n") {
		if line == "" {
			continue
		}
		var rec struct {
			RequestID string `json:"request_id"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unparseable line %q: %v", line, err)
		}
		ids = append(ids, rec.RequestID)
	}
	if len(ids) != 1 || ids[0] != "r3" {
		t.Errorf("surviving records = %v, want only r3", ids)
	}
}

// TestRawFileWriter_EncodeFailureWarnsAndRecovers pins the no-silent-drop
// contract: an unencodable record (e.g. an invalid RawMessage from a future
// caller) warns once and must not wedge later records.
func TestRawFileWriter_EncodeFailureWarnsAndRecovers(t *testing.T) {
	f := &failingRawFile{}
	w := &RawFileWriter{sessionID: "sess-1", file: f}
	w.encoder = json.NewEncoder(&w.buf)
	w.encoder.SetEscapeHTML(false)

	stderr := captureStderrForTest(t, func() {
		w.Write(llm.RawRecord{RequestID: "bad", Request: json.RawMessage(`{not-json`)})
		w.Write(llm.RawRecord{RequestID: "good"})
	})
	if n := strings.Count(stderr, "raw logging failed"); n != 1 {
		t.Errorf("got %d encode-failure warnings, want 1: %q", n, stderr)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data := string(f.data)
	if strings.Contains(data, "bad") {
		t.Errorf("unencodable record must be dropped, got: %q", data)
	}
	if !strings.Contains(data, `"request_id":"good"`) {
		t.Errorf("record after encode failure not written: %q", data)
	}
}

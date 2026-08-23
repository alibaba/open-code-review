// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- stripAnsiWriter ---

func TestStripAnsiWriter_Passthrough(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	in := "hello world\nplain text without escapes\n"
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != in {
		t.Fatalf("got %q, want %q", buf.String(), in)
	}
}

func TestStripAnsiWriter_StripsCSIColors(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	in := "\033[2m─── main.go:1-2 ───\033[0m\n\033[91m[high]\033[0m content\n"
	want := "─── main.go:1-2 ───\n[high] content\n"
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

func TestStripAnsiWriter_StripsTrueColorBackground(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	in := "\033[48;2;0;60;0m+\033[0m line\n"
	want := "+ line\n"
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

func TestStripAnsiWriter_StripsOSC(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	in := "before\033]0;window title\007after"
	want := "beforeafter"
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

func TestStripAnsiWriter_StripsOSCWithST(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	in := "before\033]52;c;data\033\\after"
	want := "beforeafter"
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

func TestStripAnsiWriter_SplitAcrossWrites(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	parts := []string{"\033[2m", "─── ", "main", ".go:1-2 ", "───", "\033[0m\n", "plain text\n"}
	for _, p := range parts {
		if _, err := w.Write([]byte(p)); err != nil {
			t.Fatalf("write %q: %v", p, err)
		}
	}
	want := "─── main.go:1-2 ───\nplain text\n"
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

func TestStripAnsiWriter_SequenceSplitMidParameter(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	// A color sequence split mid-way through its parameters across writes.
	if _, err := w.Write([]byte("a\033[48;2;0;6")); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	if _, err := w.Write([]byte("0;0m")); err != nil {
		t.Fatalf("write 2: %v", err)
	}
	if _, err := w.Write([]byte("b")); err != nil {
		t.Fatalf("write 3: %v", err)
	}
	if buf.String() != "ab" {
		t.Fatalf("got %q, want %q", buf.String(), "ab")
	}
}

func TestStripAnsiWriter_StripsMultiByteEscape(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	in := "a\033(Bb" // ESC ( B: two-byte escape with intermediate byte 0x20-0x2f
	want := "ab"
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

func TestStripAnsiWriter_MultiByteEscapeSplitAcrossWrites(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	// ESC ( is split mid-sequence; the intermediate byte '(' must keep the
	// escape open so the trailing 'B' is discarded with it.
	for _, p := range []string{"a\x1b(", "B", "b"} {
		if _, err := w.Write([]byte(p)); err != nil {
			t.Fatalf("write %q: %v", p, err)
		}
	}
	if buf.String() != "ab" {
		t.Fatalf("got %q, want %q", buf.String(), "ab")
	}
}

func TestStripAnsiWriter_StripsDCS(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	in := "a\033P1;2|data\033\\b" // DCS string terminated by ST (ESC \)
	want := "ab"
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

func TestStripAnsiWriter_ESCAtEndOfWrite(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	// ESC is the last byte of a write; the next write completes the sequence.
	if _, err := w.Write([]byte("a\x1b")); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	if _, err := w.Write([]byte("[31m")); err != nil {
		t.Fatalf("write 2: %v", err)
	}
	if _, err := w.Write([]byte("b")); err != nil {
		t.Fatalf("write 3: %v", err)
	}
	if buf.String() != "ab" {
		t.Fatalf("got %q, want %q", buf.String(), "ab")
	}
}

// --- resolveOutputWriter ---

func TestResolveOutputWriter_Stdout(t *testing.T) {
	for _, path := range []string{"", "-"} {
		w, closeFn, err := resolveOutputWriter(path, "json")
		if err != nil {
			t.Fatalf("resolve(%q): %v", path, err)
		}
		if w != os.Stdout {
			t.Fatalf("resolve(%q): got writer %T, want os.Stdout", path, w)
		}
		if err := closeFn(); err != nil {
			t.Fatalf("close(%q): %v", path, err)
		}
	}
}

func TestResolveOutputWriter_DirectoryTarget(t *testing.T) {
	dir := t.TempDir()
	_, _, err := resolveOutputWriter(dir, "json")
	if err == nil {
		t.Fatal("expected error for a directory --output target")
	}
}

func TestResolveOutputWriter_MissingParent(t *testing.T) {
	_, _, err := resolveOutputWriter(filepath.Join(t.TempDir(), "no", "such", "dir", "out.json"), "json")
	if err == nil {
		t.Fatal("expected error for a missing --output parent directory")
	}
}

// --- lazyFileWriter ---

// TestLazyFileWriter_NoWriteLeavesExistingFileUntouched pins the core
// data-safety contract: a writer that is resolved but never written to (a
// failed run, a preview error) must not create or truncate the target file.
func TestLazyFileWriter_NoWriteLeavesExistingFileUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(path, []byte("previous results\n"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}
	w, closeFn, err := resolveOutputWriter(path, "json")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "previous results\n" {
		t.Fatalf("existing file was modified, got %q", data)
	}
	_ = w
}

// TestLazyFileWriter_NoWriteDoesNotCreateFile verifies a never-written lazy
// writer leaves no empty file behind (no phantom file on failure paths).
func TestLazyFileWriter_NoWriteDoesNotCreateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	_, closeFn, err := resolveOutputWriter(path, "json")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no file to be created, stat err = %v", err)
	}
}

func TestLazyFileWriter_WriteCreatesFileAndPrintsHint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	stderr := captureStderr(t, func() {
		w, closeFn, err := resolveOutputWriter(path, "json")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if _, err := w.Write([]byte(`{"status":"success"}`)); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := closeFn(); err != nil {
			t.Fatalf("close: %v", err)
		}
	})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != `{"status":"success"}` {
		t.Fatalf("file content = %q, want the written bytes", data)
	}
	if !strings.Contains(stderr, "[ocr] Results written to "+path) {
		t.Fatalf("expected 'Results written' hint on stderr, got %q", stderr)
	}
}

func TestLazyFileWriter_TextStripsAnsi(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	w, closeFn, err := resolveOutputWriter(path, "text")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := w.Write([]byte("\033[2m dim line \033[0m\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(data), "\033") {
		t.Fatalf("text file contains ANSI escapes: %q", data)
	}
	if string(data) != " dim line \n" {
		t.Fatalf("file content = %q, want the ANSI-stripped text", data)
	}
}

func TestLazyFileWriter_JSONKeepsBytesUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	w, closeFn, err := resolveOutputWriter(path, "json")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	payload := `{"status":"success"}`
	if _, err := w.Write([]byte(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != payload {
		t.Fatalf("json file content = %q, want unchanged bytes", data)
	}
}

// --- deferred write-error propagation ---

// TestLazyFileWriter_ErrOnCreateFailure pins that a failed lazy create is
// reported through Err() so text-mode callers can surface it as a command
// error (JSON mode already propagates it via Encoder.Encode).
func TestLazyFileWriter_ErrOnCreateFailure(t *testing.T) {
	w := &lazyFileWriter{path: filepath.Join(t.TempDir(), "no", "such", "out.json")}
	if _, err := w.Write([]byte("x")); err == nil {
		t.Fatal("expected the first write to fail")
	}
	if w.Err() == nil {
		t.Fatal("Err() must report the create failure")
	}
}

func TestWriteOutError_StdoutNil(t *testing.T) {
	if err := writeOutError(os.Stdout); err != nil {
		t.Fatalf("writeOutError(os.Stdout) = %v, want nil", err)
	}
}

func TestWriteOutError_NilAfterSuccessfulWrite(t *testing.T) {
	w, closeFn, err := resolveOutputWriter(filepath.Join(t.TempDir(), "out.json"), "json")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := w.Write([]byte(`{}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := writeOutError(w); err != nil {
		t.Fatalf("writeOutError = %v, want nil", err)
	}
}

// TestEmitRunResult_TextWriteFailurePropagates pins the text/json parity: a
// --output write failure in text mode must make emitRunResult return an error
// (exit non-zero), exactly like JSON mode does.
func TestEmitRunResult_TextWriteFailurePropagates(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 2}
	w := &lazyFileWriter{path: filepath.Join(t.TempDir(), "no", "such", "out.txt")}
	if err := emitRunResult(context.Background(), ag, nil, time.Now(), "text", "developer", nil, nil, w, nil); err == nil {
		t.Fatal("emitRunResult must propagate the output write failure")
	}
}

// TestStripAnsiWriter_OSCBareEscKeepsTrailingText pins the regression where an
// OSC string terminated by a bare ESC (no ST '\') swallowed the first byte of
// the following text instead of re-parsing it.
func TestStripAnsiWriter_OSCBareEscKeepsTrailingText(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	in := "before\033]0;window title\033hello"
	want := "beforehello"
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

// TestStripAnsiWriter_OSCBareEscThenNewEscape pins that a bare-ESC-terminated
// OSC followed by a new escape sequence starts a fresh sequence instead of
// leaking or dropping bytes.
func TestStripAnsiWriter_OSCBareEscThenNewEscape(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	in := "before\033]0;window title\033\033[31mhello"
	want := "beforehello"
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

// failingWriter always fails; used to pin the Write return-value contract.
type failingWriter struct{ err error }

func (f *failingWriter) Write([]byte) (int, error) { return 0, f.err }

// TestStripAnsiWriter_WriteFailureReturnsConsumedLen pins that Write reports
// the underlying error but still returns len(p): the state machine has already
// consumed the input, so returning 0 would make a caller retry the same bytes
// and corrupt the stream.
func TestStripAnsiWriter_WriteFailureReturnsConsumedLen(t *testing.T) {
	boom := &failingWriter{err: errors.New("boom")}
	w := &stripAnsiWriter{dst: boom}
	n, err := w.Write([]byte("plain text"))
	if n != len("plain text") {
		t.Fatalf("n = %d, want %d (input was consumed even though dst failed)", n, len("plain text"))
	}
	if err == nil {
		t.Fatal("expected underlying write error to propagate")
	}
}

// TestStripAnsiWriter_CSIInvalidByteFallback pins that an invalid CSI byte
// (non-ASCII >= 0x80 or control char < 0x20) safely flushes pending bytes as
// normal text without corrupting or swallowing user content.
func TestStripAnsiWriter_CSIInvalidByteFallback(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	in := "hello\033[\xe4\xbd\xa0world"
	want := "hello\033[\xe4\xbd\xa0world"
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

// TestStripAnsiWriter_CSILengthExceededFallback pins that an unclosed or
// oversized CSI sequence exceeding maxCSISequenceLength is flushed to avoid
// unbounded buffering and content truncation.
func TestStripAnsiWriter_CSILengthExceededFallback(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	longParam := strings.Repeat("1", maxCSISequenceLength+10)
	in := "\033[" + longParam
	want := "\033[" + longParam
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

// TestStripAnsiWriter_OSCLengthExceededFallback pins that an unclosed or
// oversized OSC sequence exceeding maxOSCSequenceLength is flushed to avoid
// swallowing subsequent user content.
func TestStripAnsiWriter_OSCLengthExceededFallback(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	longBody := strings.Repeat("x", maxOSCSequenceLength+10)
	in := "\033]" + longBody
	want := "\033]" + longBody
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

// TestResolveOutputWriter_SafeByDefaultStripsNonMachineReadable pins that any
// non-machine-readable format (not json/sarif) wraps the writer in stripAnsiWriter.
func TestResolveOutputWriter_SafeByDefaultStripsNonMachineReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.custom")
	w, closeFn, err := resolveOutputWriter(path, "custom_txt")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	defer closeFn()
	lazy, ok := w.(*lazyFileWriter)
	if !ok {
		t.Fatalf("expected *lazyFileWriter, got %T", w)
	}
	if !lazy.strip {
		t.Fatal("expected strip to be true for non-machine-readable format")
	}
}

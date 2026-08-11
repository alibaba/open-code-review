// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestStripAnsiWriter_StripsSingleByteEscape(t *testing.T) {
	var buf bytes.Buffer
	w := &stripAnsiWriter{dst: &buf}
	in := "a\033(Bb" // ESC ( B: select charset; the 2-byte ESC ( sequence is discarded
	want := "aBb"
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

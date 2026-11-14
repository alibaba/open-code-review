// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestManCommandWritesManPages pins that `ocr man <dir>` emits a roff tree with
// a root page, a page per visible subcommand, and no page for the hidden man
// command itself.
func TestManCommandWritesManPages(t *testing.T) {
	dir := t.TempDir()
	if err := manCmd.RunE(manCmd, []string{dir}); err != nil {
		t.Fatalf("man RunE: %v", err)
	}

	root := filepath.Join(dir, "ocr.1")
	body, err := os.ReadFile(root)
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}
	for _, want := range []string{`.TH "OCR" "1"`, "SYNOPSIS", "ocr - OpenCodeReview"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("%s: missing %q", root, want)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "ocr-review.1")); err != nil {
		t.Errorf("expected a page for ocr review: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "ocr-man.1")); err == nil {
		t.Error("ocr-man.1 should not exist: man is hidden and must not document itself")
	}
}

// TestManCommandRequiresDirectory pins the arity of `ocr man`: exactly one
// argument, the output directory.
func TestManCommandRequiresDirectory(t *testing.T) {
	if err := manCmd.Args(manCmd, nil); err == nil {
		t.Error("expected an error for zero arguments")
	}
	if err := manCmd.Args(manCmd, []string{"a", "b"}); err == nil {
		t.Error("expected an error for two arguments")
	}
	if err := manCmd.Args(manCmd, []string{"dir"}); err != nil {
		t.Errorf("unexpected error for one argument: %v", err)
	}
}

// TestManCommandIsHidden pins that the generator stays out of `ocr -h` and out
// of its own generated tree, so the user-facing invocation is `man ocr` rather
// than `ocr man`.
func TestManCommandIsHidden(t *testing.T) {
	if !manCmd.Hidden {
		t.Error("manCmd should be hidden")
	}
	if manCmd.IsAvailableCommand() {
		t.Error("manCmd should not be an available (visible) command")
	}
}

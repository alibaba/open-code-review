//go:build linux

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWorkspaceFIFORead(t *testing.T) {
	if kind := os.Getenv("OCR_TEST_FIFO_READ_KIND"); kind != "" {
		reader := &FileReader{RepoDir: os.Getenv("OCR_TEST_FIFO_REPO"), Mode: ModeWorkspace}
		path := os.Getenv("OCR_TEST_FIFO_PATH")
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		var err error
		switch kind {
		case "full":
			_, err = reader.Read(ctx, path)
		case "lines":
			_, _, err = reader.ReadLines(ctx, path, 1, 10)
		case "tool":
			_, err = NewFileRead(reader).Execute(ctx, map[string]any{"file_path": path})
		default:
			t.Fatalf("unknown child read kind: %s", kind)
		}
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("expected a non-regular-file error, got %v", err)
		}
		return
	}

	repo := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(repo, "events.go"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("events.go", filepath.Join(repo, "alias.go")); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"full", "lines", "tool"} {
		for _, path := range []string{"events.go", "alias.go"} {
			t.Run(kind+"/"+path, func(t *testing.T) {
				// A blocked FIFO open cannot observe the read context. Isolate the
				// call so a regression is terminated without leaking a goroutine.
				ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkspaceFIFORead$")
				cmd.Env = append(os.Environ(), "OCR_TEST_FIFO_READ_KIND="+kind, "OCR_TEST_FIFO_REPO="+repo, "OCR_TEST_FIFO_PATH="+path)
				output, err := cmd.CombinedOutput()
				if ctx.Err() != nil {
					t.Fatal("workspace read blocked on a FIFO despite its 50ms context deadline")
				}
				if err != nil {
					t.Fatalf("child read failed: %v\n%s", err, output)
				}
			})
		}
	}
}

func TestWorkspaceRegularSymlinkRead(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "target.go"), []byte("first\nsecond"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.go", filepath.Join(repo, "alias.go")); err != nil {
		t.Fatal(err)
	}
	reader := &FileReader{RepoDir: repo, Mode: ModeWorkspace}
	for _, path := range []string{"target.go", "alias.go"} {
		content, err := reader.Read(t.Context(), path)
		if err != nil || content != "first\nsecond" {
			t.Fatalf("Read(%q) = %q, %v", path, content, err)
		}
		output, err := NewFileRead(reader).Execute(t.Context(), map[string]any{"file_path": path})
		if err != nil || !strings.Contains(output, "1|first\n2|second\n") {
			t.Fatalf("file_read(%q) = %q, %v", path, output, err)
		}
	}
}

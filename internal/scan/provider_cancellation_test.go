// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestProvider_Enumerate_NilRunnerCancellation(t *testing.T) {
	dir := t.TempDir()
	markerPath := filepath.Join(t.TempDir(), "ready")
	t.Setenv("OCR_TEST_MARKER", markerPath)

	shimDir := t.TempDir()
	if runtime.GOOS == "windows" {
		cmdBody := "@echo off\r\nif \"%1\"==\"rev-parse\" (\r\n  echo .git\r\n  exit /b 0\r\n)\r\necho ready>\"%OCR_TEST_MARKER%\"\r\n:loop\r\ngoto loop\r\n"
		if err := os.WriteFile(filepath.Join(shimDir, "git.cmd"), []byte(cmdBody), 0o644); err != nil {
			t.Fatal(err)
		}
		if pathExt := os.Getenv("PATHEXT"); !strings.Contains(strings.ToUpper(pathExt), ".CMD") {
			t.Setenv("PATHEXT", pathExt+string(os.PathListSeparator)+".CMD")
		}
	} else {
		shBody := "#!/bin/sh\nif [ \"$1\" = \"rev-parse\" ]; then printf '.git\\n'; exit 0; fi\nprintf 'ready\\n' > \"$OCR_TEST_MARKER\"\nwhile :; do :; done\n"
		if err := os.WriteFile(filepath.Join(shimDir, "git"), []byte(shBody), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	baseCtx, baseCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer baseCancel()
	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			if _, err := os.Stat(markerPath); err == nil {
				cancel()
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	t.Cleanup(func() { <-done })

	_, err := NewProvider(dir, nil, nil, 0).Enumerate(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want errors.Is(err, context.Canceled)", err)
	}
}

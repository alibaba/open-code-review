// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileReader_NilRunner_GitCancellation(t *testing.T) {
	setupHang := func(t *testing.T) (context.Context, *FileReader) {
		t.Helper()
		const secret = "SECRET_OUTPUT_LEAK"
		markerPath := filepath.Join(t.TempDir(), "ready")
		t.Setenv("OCR_TEST_MARKER", markerPath)
		shimGit(t,
			"printf 'line1\\n'\nprintf '%s\\n' '"+secret+"' >&2\nprintf 'ready\\n' > \"$OCR_TEST_MARKER\"\nwhile :; do :; done\n",
			"echo line1\r\necho "+secret+">&2\r\necho ready>\"%OCR_TEST_MARKER%\"\r\n:loop\r\ngoto loop\r\n",
		)

		baseCtx, baseCancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(baseCancel)
		ctx, cancel := context.WithCancel(baseCtx)
		t.Cleanup(cancel)
		waitForMarkerAndCancel(t, markerPath, cancel)
		return ctx, &FileReader{RepoDir: t.TempDir(), Mode: ModeCommit, Ref: "HEAD"}
	}

	t.Run("Read", func(t *testing.T) {
		ctx, fr := setupHang(t)
		_, err := fr.Read(ctx, "main.go")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want errors.Is(err, context.Canceled)", err)
		}
	})

	t.Run("ReadLines_canceled", func(t *testing.T) {
		ctx, fr := setupHang(t)
		_, _, err := fr.ReadLines(ctx, "main.go", 1, 5)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want errors.Is(err, context.Canceled)", err)
		}
		if err != nil && strings.Contains(err.Error(), "SECRET_OUTPUT_LEAK") {
			t.Errorf("error leaked stderr: %v", err)
		}
	})

	t.Run("ReadLines_normal_failure_preserves_stderr", func(t *testing.T) {
		shimGit(t,
			"printf 'fatal: simulated bad object\\n' >&2\nexit 128\n",
			"echo fatal: simulated bad object>&2\r\nexit /b 128\r\n",
		)
		fr := &FileReader{RepoDir: t.TempDir(), Mode: ModeCommit, Ref: "HEAD"}
		_, _, err := fr.ReadLines(context.Background(), "main.go", 1, 5)
		if err == nil || !strings.Contains(err.Error(), "fatal: simulated bad object") {
			t.Fatalf("err = %v, want git stderr preserved", err)
		}
	})
}

func TestFileFindProvider_NilRunner_GitCancellation(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "ready")
	t.Setenv("OCR_TEST_MARKER", markerPath)
	shimGit(t,
		"printf 'ready\\n' > \"$OCR_TEST_MARKER\"\nwhile :; do :; done\n",
		"echo ready>\"%OCR_TEST_MARKER%\"\r\n:loop\r\ngoto loop\r\n",
	)

	baseCtx, baseCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer baseCancel()
	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()
	waitForMarkerAndCancel(t, markerPath, cancel)

	p := NewFileFind(&FileReader{RepoDir: t.TempDir(), Mode: ModeCommit, Ref: "HEAD"})
	_, err := p.Execute(ctx, map[string]any{"query_name": "main.go"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want errors.Is(err, context.Canceled)", err)
	}
}

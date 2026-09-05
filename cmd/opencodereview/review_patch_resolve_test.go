// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/tool"
)

// writePatchDir persists a single-file patch applying a change to x.go.
func writePatchDir(t *testing.T, patch string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "change.patch"), []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const xgoPatch = "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1,3 @@\n package x\n+\n+func X() {}\n"

func TestResolvePatchInput(t *testing.T) {
	ctx := context.Background()

	t.Run("non-patch run returns nil", func(t *testing.T) {
		dir := initTestGitRepo(t)
		cc, err := loadCommonContext(dir, "", "", 0, 0, true)
		if err != nil {
			t.Fatalf("loadCommonContext: %v", err)
		}
		got, err := resolvePatchInput(ctx, cc, reviewOptions{})
		if err != nil {
			t.Fatalf("resolvePatchInput: %v", err)
		}
		if got != nil {
			t.Errorf("got %v, want nil for a run without --patch", got)
		}
	})

	t.Run("missing directory is rejected upfront", func(t *testing.T) {
		dir := initTestGitRepo(t)
		cc, err := loadCommonContext(dir, "", "", 0, 0, true)
		if err != nil {
			t.Fatalf("loadCommonContext: %v", err)
		}
		missing := filepath.Join(t.TempDir(), "missing")
		_, err = resolvePatchInput(ctx, cc, reviewOptions{diffDir: missing})
		if err == nil || !strings.Contains(err.Error(), "validate --patch") {
			t.Errorf("error = %v, want validate --patch failure", err)
		}
	})

	t.Run("resolves the branch ref to a head", func(t *testing.T) {
		dir := initTestGitRepo(t)
		cc, err := loadCommonContext(dir, "", "", 0, 0, true)
		if err != nil {
			t.Fatalf("loadCommonContext: %v", err)
		}
		patchDir := writePatchDir(t, xgoPatch)

		got, err := resolvePatchInput(ctx, cc, reviewOptions{diffDir: patchDir})
		if err != nil {
			t.Fatalf("resolvePatchInput: %v", err)
		}
		if got == nil || got.ResolvedHead == "" {
			t.Fatalf("got %v, want a resolved head (branch defaults to HEAD)", got)
		}
		if want := revParse(t, dir, "HEAD"); got.ResolvedHead != want {
			t.Errorf("ResolvedHead = %q, want HEAD %q", got.ResolvedHead, want)
		}
	})

	t.Run("preview never materializes", func(t *testing.T) {
		dir := initTestGitRepo(t)
		gitCommitFile(t, dir, "x.go", "package x\n", "post-image")
		cc, err := loadCommonContext(dir, "", "", 0, 0, true)
		if err != nil {
			t.Fatalf("loadCommonContext: %v", err)
		}
		// The patch does not apply to the base commit (wrong content), so a
		// materialization attempt would fail. Preview must not even try.
		patchDir := writePatchDir(t, "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-package wrong\n+package changed\n")

		before := revParse(t, dir, "HEAD")
		got, err := resolvePatchInput(ctx, cc, reviewOptions{diffDir: patchDir, diffApply: true, preview: true})
		if err != nil {
			t.Fatalf("preview must not materialize: %v", err)
		}
		if got == nil || got.ResolvedHead != before {
			t.Errorf("got %v, want the unresolved head %q", got, before)
		}
	})

	t.Run("apply-patch pins the head to the materialized commit", func(t *testing.T) {
		dir := initTestGitRepo(t)
		gitCommitFile(t, dir, "x.go", "package x\n", "post-image")
		cc, err := loadCommonContext(dir, "", "", 0, 0, true)
		if err != nil {
			t.Fatalf("loadCommonContext: %v", err)
		}
		patchDir := writePatchDir(t, xgoPatch)

		before := revParse(t, dir, "HEAD")
		got, err := resolvePatchInput(ctx, cc, reviewOptions{diffDir: patchDir, diffApply: true})
		if err != nil {
			t.Fatalf("resolvePatchInput: %v", err)
		}
		if got == nil {
			t.Fatal("got nil, want a resolution")
		}
		if got.ResolvedHead == before {
			t.Errorf("ResolvedHead = %q, want a new materialized commit", got.ResolvedHead)
		}
	})
}

func TestResolveSealedReadState(t *testing.T) {
	patch := &diff.InputResolution{ResolvedHead: "patch-head"}

	t.Run("sealed resolution wins over patch input", func(t *testing.T) {
		sealed := &agent.SealedInput{Resolution: diff.InputResolution{ResolvedHead: "sealed-head"}}
		got, _ := resolveSealedReadState(sealed, patch, reviewOptions{})
		if got == nil || got.ResolvedHead != "sealed-head" {
			t.Errorf("got %v, want the sealed resolution", got)
		}
	})

	t.Run("patch input is the fallback without a seal", func(t *testing.T) {
		got, _ := resolveSealedReadState(nil, patch, reviewOptions{})
		if got == nil || got.ResolvedHead != "patch-head" {
			t.Errorf("got %v, want the patch input", got)
		}
	})

	t.Run("both absent yields nil", func(t *testing.T) {
		if got, _ := resolveSealedReadState(nil, nil, reviewOptions{}); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("patch with a branch forces commit mode", func(t *testing.T) {
		_, mode := resolveSealedReadState(nil, patch, reviewOptions{diffDir: "d", branch: "main"})
		if mode != tool.ModeCommit {
			t.Errorf("mode = %v, want %v", mode, tool.ModeCommit)
		}
	})

	t.Run("patch with apply forces commit mode", func(t *testing.T) {
		_, mode := resolveSealedReadState(nil, patch, reviewOptions{diffDir: "d", diffApply: true})
		if mode != tool.ModeCommit {
			t.Errorf("mode = %v, want %v", mode, tool.ModeCommit)
		}
	})

	t.Run("patch alone keeps the parsed mode", func(t *testing.T) {
		_, mode := resolveSealedReadState(nil, patch, reviewOptions{commit: "HEAD"})
		if mode != tool.ModeCommit {
			t.Errorf("mode = %v, want %v from the parsed options", mode, tool.ModeCommit)
		}
	})
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/tool"
)

func runPreview(cc *commonContext, opts reviewOptions, out io.Writer) error {
	return runPreviewContext(context.Background(), cc, opts, out, nil)
}

func TestRunPreview(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "x.go", "package x\n", "add x")
	cc, err := loadCommonContext(dir, "", "", 0, 0, true)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	silenceStdout(t, func() {
		if err := runPreview(cc, reviewOptions{commit: "HEAD"}, os.Stdout); err != nil {
			t.Fatalf("runPreview error: %v", err)
		}
	})
}

func TestRunPreviewJSONFormat(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "main.go", "package main\n", "add main")
	gitCommitFile(t, dir, "notes.md", "# notes\n", "add notes")
	cc, err := loadCommonContext(dir, "", "", 0, 0, true)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runPreview(cc, reviewOptions{commit: "HEAD", outputFormat: "json"}, os.Stdout); err != nil {
			t.Errorf("runPreview error: %v", err)
		}
	})

	got := decodeSinglePreviewJSON(t, out)
	if got.TotalFiles != 1 {
		t.Fatalf("total_files = %d, want 1 (HEAD adds notes.md only)", got.TotalFiles)
	}
	if got.Entries[0].Path != "notes.md" {
		t.Errorf("path = %q, want notes.md", got.Entries[0].Path)
	}
	if got.Entries[0].WillReview {
		t.Error("notes.md should be excluded by the extension allowlist")
	}
	if got.Entries[0].ExcludeReason != model.ExcludeExtension {
		t.Errorf("exclude_reason = %q, want %q", got.Entries[0].ExcludeReason, model.ExcludeExtension)
	}
}

func TestRunPreviewUsesPatchDirectory(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "x.go", "package x\n\nfunc X() {}\n", "post-image")
	patchDir := t.TempDir()
	patch := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1,3 @@\n package x\n+\n+func X() {}\n"
	if err := os.WriteFile(filepath.Join(patchDir, "change.patch"), []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}
	cc, err := loadCommonContext(dir, "", "", 0, 0, true)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runPreviewContext(context.Background(), cc, reviewOptions{diffDir: patchDir, outputFormat: "json"}, os.Stdout, nil); err != nil {
			t.Errorf("runPreviewContext error: %v", err)
		}
	})
	got := decodeSinglePreviewJSON(t, out)
	if got.TotalFiles != 1 || got.Entries[0].Path != "x.go" {
		t.Fatalf("preview = %+v, want patch entry x.go", got)
	}
}

func TestPreviewDiffApplyDoesNotRequireApplicablePatch(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "x.go", "package x\n", "base")
	patchDir := t.TempDir()
	badPatch := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-package wrong\n+package changed\n"
	if err := os.WriteFile(filepath.Join(patchDir, "bad.patch"), []byte(badPatch), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := executeReviewContext(context.Background(), reviewOptions{
		repoDir:      dir,
		diffDir:      patchDir,
		diffApply:    true,
		preview:      true,
		outputFormat: "json",
	}); err != nil {
		t.Fatalf("preview should not apply patches: %v", err)
	}
}

func TestExecuteReviewRejectsInvalidPatchDirectoryUpfront(t *testing.T) {
	dir := initTestGitRepo(t)
	missing := filepath.Join(t.TempDir(), "missing")
	err := executeReviewContext(context.Background(), reviewOptions{
		repoDir:      dir,
		diffDir:      missing,
		preview:      true,
		outputFormat: "json",
	})
	if err == nil || !strings.Contains(err.Error(), "validate --patch") {
		t.Fatalf("error = %v, want upfront --patch validation error", err)
	}
}

// TestRunPreviewCreatesNoSession pins that previewing never opens session
// persistence. Building the agent with agent.New auto-created a session, which
// left an unfinalized (and usually empty) JSONL file under the OCR home even
// though preview never runs or finalizes a review.
func TestRunPreviewCreatesNoSession(t *testing.T) {
	home := freshOCRHome(t)

	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "x.go", "package x\n", "add x")
	cc, err := loadCommonContext(dir, "", "", 0, 0, true)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	silenceStdout(t, func() {
		if err := runPreview(cc, reviewOptions{commit: "HEAD"}, os.Stdout); err != nil {
			t.Fatalf("runPreview error: %v", err)
		}
	})

	assertNoSessionStore(t, home)
}

func TestLoadReviewResumeState(t *testing.T) {
	dir := initTestGitRepo(t)

	t.Run("empty resume returns nil", func(t *testing.T) {
		state, err := loadReviewResumeState(dir, reviewOptions{})
		if err != nil || state != nil {
			t.Errorf("got state=%v err=%v, want nil,nil", state, err)
		}
	})

	t.Run("workspace resume rejected", func(t *testing.T) {
		_, err := loadReviewResumeState(dir, reviewOptions{resume: "sess-1"})
		if err == nil {
			t.Fatal("expected error for workspace-mode resume")
		}
	})

	t.Run("missing session load fails", func(t *testing.T) {
		_, err := loadReviewResumeState(dir, reviewOptions{resume: "does-not-exist", commit: "HEAD"})
		if err == nil {
			t.Fatal("expected error loading nonexistent resume session")
		}
	})
}

func TestInitMCPClients(t *testing.T) {
	ctx := context.Background()
	reg := tool.NewRegistry()

	t.Run("nil config", func(t *testing.T) {
		if got := initMCPClients(ctx, nil, reg, "/tmp", "v"); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("empty servers", func(t *testing.T) {
		if got := initMCPClients(ctx, &Config{}, reg, "/tmp", "v"); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("remote without url skipped", func(t *testing.T) {
		cfg := &Config{MCPServers: map[string]MCPServerConfig{
			"r": {Type: "remote"},
		}}
		if got := initMCPClients(ctx, cfg, reg, "/tmp", "v"); len(got) != 0 {
			t.Errorf("got %d clients, want 0", len(got))
		}
	})

	t.Run("stdio without command skipped", func(t *testing.T) {
		cfg := &Config{MCPServers: map[string]MCPServerConfig{
			"s": {Type: "stdio"},
		}}
		if got := initMCPClients(ctx, cfg, reg, "/tmp", "v"); len(got) != 0 {
			t.Errorf("got %d clients, want 0", len(got))
		}
	})
}

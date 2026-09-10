// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/rules"
)

func initPreviewRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# r\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func runPreviewGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func writePreviewFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestPreview exercises Agent.Preview against a real workspace diff so the
// full preview-building path (loadDiffs + whyExcluded + entry assembly) runs.
func TestPreview(t *testing.T) {
	dir := initPreviewRepo(t)

	// A reviewable Go file and an excluded binary-ish/extension file.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.bin"), []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatalf("write data.bin: %v", err)
	}

	a := New(Args{RepoDir: dir})
	preview, err := a.preview(context.Background())
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	if preview.TotalFiles == 0 {
		t.Fatal("Preview reported zero files despite workspace changes")
	}
	if preview.ReviewableCount == 0 {
		t.Error("expected at least one reviewable entry (main.go)")
	}
	if len(preview.Entries) != preview.TotalFiles {
		t.Errorf("entries=%d totalFiles=%d, want equal", len(preview.Entries), preview.TotalFiles)
	}
}

// TestPreviewEmptyEntriesNotNil pins that a clean workspace still yields a
// non-nil Entries slice, so JSON output marshals `"files":[]` instead of
// `"files":null`. Scan preview already guarantees this; review must agree.
func TestPreviewEmptyEntriesNotNil(t *testing.T) {
	dir := initPreviewRepo(t)

	a := New(Args{RepoDir: dir})
	preview, err := a.preview(context.Background())
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	if preview.TotalFiles != 0 {
		t.Fatalf("expected a clean workspace, got %d file(s)", preview.TotalFiles)
	}
	if preview.Entries == nil {
		t.Error("Entries is nil; JSON output would emit \"files\":null")
	}
}

func TestPreviewTrackedDefaultExcludedDirIsAccountedAndCanBeIncluded(t *testing.T) {
	dir := initPreviewRepo(t)

	writePreviewFile(t, dir, "src/app.rs", "pub fn app() -> i32 { 1 }\n")
	writePreviewFile(t, dir, "vendor/lib.rs", "pub fn dep() -> i32 { 1 }\n")
	runPreviewGit(t, dir, "add", "src/app.rs", "vendor/lib.rs")
	runPreviewGit(t, dir, "commit", "-m", "add rust files")

	writePreviewFile(t, dir, "src/app.rs", "pub fn app() -> i32 { 2 }\n")
	writePreviewFile(t, dir, "vendor/lib.rs", "pub fn dep() -> i32 { 2 }\n")
	runPreviewGit(t, dir, "add", "src/app.rs", "vendor/lib.rs")
	runPreviewGit(t, dir, "commit", "-m", "update rust files")

	preview, err := Preview(context.Background(), Args{
		RepoDir: dir,
		From:    "HEAD~1",
		To:      "HEAD",
	})
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	if preview.TotalFiles != 2 || preview.ReviewableCount != 1 || preview.ExcludedCount != 1 {
		t.Fatalf("preview counts = total:%d reviewable:%d excluded:%d, want 2/1/1",
			preview.TotalFiles, preview.ReviewableCount, preview.ExcludedCount)
	}
	vendor := previewEntry(t, preview, "vendor/lib.rs")
	if vendor.WillReview || vendor.ExcludeReason != ExcludeDefaultPath {
		t.Fatalf("vendor/lib.rs entry = %+v, want excluded default_path", vendor)
	}

	fullScopePreview, err := Preview(context.Background(), Args{
		RepoDir: dir,
		From:    "HEAD~1",
		To:      "HEAD",
		FileFilter: &rules.FileFilter{
			Include: []string{"**/*"},
		},
	})
	if err != nil {
		t.Fatalf("Preview with include error: %v", err)
	}
	if fullScopePreview.TotalFiles != 2 || fullScopePreview.ReviewableCount != 2 || fullScopePreview.ExcludedCount != 0 {
		t.Fatalf("full-scope counts = total:%d reviewable:%d excluded:%d, want 2/2/0",
			fullScopePreview.TotalFiles, fullScopePreview.ReviewableCount, fullScopePreview.ExcludedCount)
	}
	vendor = previewEntry(t, fullScopePreview, "vendor/lib.rs")
	if !vendor.WillReview || vendor.ExcludeReason != ExcludeNone {
		t.Fatalf("vendor/lib.rs full-scope entry = %+v, want reviewable", vendor)
	}
}

func previewEntry(t *testing.T, preview *DiffPreview, path string) DiffPreviewEntry {
	t.Helper()
	for _, entry := range preview.Entries {
		if entry.Path == path {
			return entry
		}
	}
	t.Fatalf("%s missing from preview entries: %+v", path, preview.Entries)
	return DiffPreviewEntry{}
}

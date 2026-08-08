// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
	preview, err := a.Preview(context.Background())
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
	preview, err := a.Preview(context.Background())
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

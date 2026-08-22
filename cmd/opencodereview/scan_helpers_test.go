// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/model"
)

func TestLoadScanResumeState(t *testing.T) {
	dir := initTestGitRepo(t)

	t.Run("empty resume returns nil", func(t *testing.T) {
		state, err := loadScanResumeState(dir, scanOptions{}, nil)
		if err != nil || state != nil {
			t.Errorf("got state=%v err=%v, want nil,nil", state, err)
		}
	})

	t.Run("missing session load fails", func(t *testing.T) {
		_, err := loadScanResumeState(dir, scanOptions{resume: "nope"}, nil)
		if err == nil {
			t.Fatal("expected error loading nonexistent resume session")
		}
	})
}

func TestRunScanPreview(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "y.go", "package y\n", "add y")
	cc, err := loadCommonContext(dir, "", 0, 0, false)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	scanTpl, err := template.LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	silenceStdout(t, func() {
		if err := runScanPreview(cc, scanTpl, nil, "text"); err != nil {
			t.Fatalf("runScanPreview error: %v", err)
		}
	})
}

func TestRunScanPreviewJSONFormat(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "y.go", "package y\n", "add y")
	cc, err := loadCommonContext(dir, "", 0, 0, false)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	scanTpl, err := template.LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runScanPreview(cc, scanTpl, nil, "json"); err != nil {
			t.Errorf("runScanPreview error: %v", err)
		}
	})

	got := decodeSinglePreviewJSON(t, out)
	var found bool
	for _, e := range got.Entries {
		if e.Path == "y.go" {
			found = true
			if e.Status != "scan" || !e.WillReview {
				t.Errorf("y.go = %+v, want a selected scan entry", e)
			}
		}
	}
	if !found {
		t.Errorf("y.go missing from scan preview: %+v", got.Entries)
	}
}

func TestRunScanPreviewAppliesMaxTokens(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	configDir := filepath.Join(home, ".opencodereview")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"max_tokens":100}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "large.go"), []byte(strings.Repeat("token ", 200)), 0o644); err != nil {
		t.Fatalf("write large.go: %v", err)
	}

	out := captureStdout(t, func() {
		if err := executeScan(scanOptions{
			repoDir:      dir,
			paths:        "large.go",
			outputFormat: "json",
			preview:      true,
		}); err != nil {
			t.Errorf("executeScan error: %v", err)
		}
	})

	got := decodeSinglePreviewJSON(t, out)
	if len(got.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(got.Entries))
	}
	entry := got.Entries[0]
	if entry.Path != "large.go" {
		t.Fatalf("entry path = %q, want large.go", entry.Path)
	}
	if entry.WillReview {
		t.Fatal("large.go exceeds max_tokens and must not be listed as reviewable")
	}
	if entry.ExcludeReason != model.ExcludeTooLarge {
		t.Fatalf("exclude_reason = %q, want %q", entry.ExcludeReason, model.ExcludeTooLarge)
	}
}

// TestRunScanPreviewCreatesNoSession mirrors TestRunPreviewCreatesNoSession:
// scan.NewAgent auto-creates a session too, so scan preview leaked the same
// unfinalized JSONL artifact.
func TestRunScanPreviewCreatesNoSession(t *testing.T) {
	home := freshOCRHome(t)

	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "y.go", "package y\n", "add y")
	cc, err := loadCommonContext(dir, "", 0, 0, false)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	scanTpl, err := template.LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	silenceStdout(t, func() {
		if err := runScanPreview(cc, scanTpl, nil, "text"); err != nil {
			t.Fatalf("runScanPreview error: %v", err)
		}
	})

	assertNoSessionStore(t, home)
}

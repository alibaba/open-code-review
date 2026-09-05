// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"os"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
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
	cc, err := loadCommonContext(dir, "", "", 0, 0, false)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	scanTpl, err := template.LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	silenceStdout(t, func() {
		if err := runScanPreviewContext(context.Background(), cc, scanTpl, nil, "text", os.Stdout); err != nil {
			t.Fatalf("runScanPreview error: %v", err)
		}
	})
}

func TestRunScanPreviewJSONFormat(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "y.go", "package y\n", "add y")
	cc, err := loadCommonContext(dir, "", "", 0, 0, false)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	scanTpl, err := template.LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runScanPreviewContext(context.Background(), cc, scanTpl, nil, "json", os.Stdout); err != nil {
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

// TestRunScanPreviewCreatesNoSession mirrors TestRunPreviewCreatesNoSession:
// scan.NewAgent auto-creates a session too, so scan preview leaked the same
// unfinalized JSONL artifact.
func TestRunScanPreviewCreatesNoSession(t *testing.T) {
	home := freshOCRHome(t)

	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "y.go", "package y\n", "add y")
	cc, err := loadCommonContext(dir, "", "", 0, 0, false)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	scanTpl, err := template.LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	silenceStdout(t, func() {
		if err := runScanPreviewContext(context.Background(), cc, scanTpl, nil, "text", os.Stdout); err != nil {
			t.Fatalf("runScanPreview error: %v", err)
		}
	})

	assertNoSessionStore(t, home)
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
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
	cc, err := loadCommonContext(dir, "", 0, 0, false)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	scanTpl, err := template.LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	silenceStdout(t, func() {
		if err := runScanPreview(cc, scanTpl, nil); err != nil {
			t.Fatalf("runScanPreview error: %v", err)
		}
	})
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteDelegateScanJSON(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "app.go", "package app\n\nfunc App() {}\n", "add app")

	out := captureDelegateStdout(t, func() {
		if err := executeDelegateScan(delegateScanOptions{repoDir: dir, format: "json"}); err != nil {
			t.Fatalf("executeDelegateScan(json) error: %v", err)
		}
	})

	var got delegateScanJSON
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode scan JSON: %v\n%s", err, out)
	}
	if got.SchemaVersion != delegateSchemaVersion || got.Mode != "scan" {
		t.Fatalf("unexpected envelope: %#v", got)
	}
	if got.BatchStrategy == "" {
		t.Error("batch_strategy must be reported so the host knows how files were grouped")
	}
	if got.Batches == nil || got.ExcludedFiles == nil || got.RuleGroups == nil {
		t.Fatal("JSON arrays must not be null")
	}
	if len(got.Batches) != 1 || len(got.Batches[0].Files) != 1 {
		t.Fatalf("batches = %#v, want one batch holding one file", got.Batches)
	}
	if got.Batches[0].Files[0].Path != "app.go" {
		t.Errorf("scanned path = %q, want app.go", got.Batches[0].Files[0].Path)
	}
	if got.Batches[0].BatchID != 1 {
		t.Errorf("batch_id = %d, want 1-based numbering", got.Batches[0].BatchID)
	}
	if got.ScannableCount != 1 {
		t.Errorf("scannable_count = %d, want 1", got.ScannableCount)
	}
	// Rules ship with the plan by default — that is what makes this a single
	// command a host agent can act on without a follow-up call.
	if len(got.RuleGroups) == 0 {
		t.Error("rule_groups must be populated by default")
	}
}

func TestExecuteDelegateScan_NoRulesOmitsRuleGroups(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "app.go", "package app\n", "add app")

	out := captureDelegateStdout(t, func() {
		if err := executeDelegateScan(delegateScanOptions{repoDir: dir, format: "json", noRules: true}); err != nil {
			t.Fatalf("executeDelegateScan(--no-rules) error: %v", err)
		}
	})

	var got delegateScanJSON
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode scan JSON: %v\n%s", err, out)
	}
	if len(got.RuleGroups) != 0 {
		t.Errorf("rule_groups = %#v, want empty under --no-rules", got.RuleGroups)
	}
	if got.RuleGroups == nil {
		t.Error("rule_groups must stay a non-null array even when empty")
	}
}

func TestExecuteDelegateScanText(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "app.go", "package app\n", "add app")

	out := captureDelegateStdout(t, func() {
		if err := executeDelegateScan(delegateScanOptions{repoDir: dir, format: "text"}); err != nil {
			t.Fatalf("executeDelegateScan(text) error: %v", err)
		}
	})

	text := string(out)
	for _, want := range []string{"# Scan Plan", "- mode: scan", "- batch_strategy:", "## Batch 1", "app.go"} {
		if !strings.Contains(text, want) {
			t.Errorf("text output missing %q:\n%s", want, text)
		}
	}
}

func TestExecuteDelegateScan_PathScopeAndBatchOverride(t *testing.T) {
	dir := initTestGitRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "svc"), 0o755); err != nil {
		t.Fatalf("mkdir svc: %v", err)
	}
	gitCommitFile(t, dir, "app.go", "package app\n", "add app")
	if err := os.WriteFile(filepath.Join(dir, "svc", "one.go"), []byte("package svc\n"), 0o644); err != nil {
		t.Fatalf("write svc/one.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "svc", "two.go"), []byte("package svc\n"), 0o644); err != nil {
		t.Fatalf("write svc/two.go: %v", err)
	}

	out := captureDelegateStdout(t, func() {
		err := executeDelegateScan(delegateScanOptions{
			repoDir: dir, format: "json", paths: "svc", batch: "by-directory", noRules: true,
		})
		if err != nil {
			t.Fatalf("executeDelegateScan(--path svc) error: %v", err)
		}
	})

	var got delegateScanJSON
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode scan JSON: %v\n%s", err, out)
	}
	if got.BatchStrategy != "by-directory" {
		t.Errorf("batch_strategy = %q, want the --batch override to win", got.BatchStrategy)
	}
	if len(got.Batches) != 1 {
		t.Fatalf("batches = %#v, want one by-directory batch", got.Batches)
	}
	if got.Batches[0].Key != "svc" {
		t.Errorf("batch key = %q, want svc", got.Batches[0].Key)
	}
	for _, f := range got.Batches[0].Files {
		if !strings.HasPrefix(f.Path, "svc/") {
			t.Errorf("--path svc leaked %q into the plan", f.Path)
		}
	}
	if len(got.Batches[0].Files) != 2 {
		t.Errorf("files = %d, want both svc files", len(got.Batches[0].Files))
	}
}

func TestExecuteDelegateScan_MergesInlineAndFileBackground(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "app.go", "package app\n", "add app")
	if err := os.WriteFile(filepath.Join(dir, "ctx.md"), []byte("payment retry epic\n"), 0o644); err != nil {
		t.Fatalf("write ctx.md: %v", err)
	}

	out := captureDelegateStdout(t, func() {
		err := executeDelegateScan(delegateScanOptions{
			repoDir:        dir,
			format:         "json",
			noRules:        true,
			background:     "inline note",
			backgroundFile: "ctx.md",
		})
		if err != nil {
			t.Fatalf("executeDelegateScan(--background-file) error: %v", err)
		}
	})

	var got delegateScanJSON
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode scan JSON: %v\n%s", err, out)
	}
	if !strings.Contains(got.Background, "inline note") {
		t.Errorf("background %q lost the inline value", got.Background)
	}
	if !strings.Contains(got.Background, "payment retry epic") {
		t.Errorf("background %q lost the --background-file content", got.Background)
	}
}

func TestExecuteDelegateScan_MissingBackgroundFileErrors(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "app.go", "package app\n", "add app")

	silenceStdout(t, func() {
		err := executeDelegateScan(delegateScanOptions{
			repoDir: dir, format: "json", backgroundFile: "nope.md",
		})
		if err == nil {
			t.Fatal("expected an error for a missing --background-file, got nil")
		}
	})
}

// Excluded files carry the reason the filter dropped them; a host agent reads
// this to justify why a file it can see was not scanned.
func TestExecuteDelegateScanText_RendersExclusions(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "app.go", "package app\n", "add app")
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), []byte{0x89, 'P', 'N', 'G', 0x00}, 0o644); err != nil {
		t.Fatalf("write logo.png: %v", err)
	}

	out := captureDelegateStdout(t, func() {
		if err := executeDelegateScan(delegateScanOptions{repoDir: dir, format: "text", noRules: true}); err != nil {
			t.Fatalf("executeDelegateScan(text) error: %v", err)
		}
	})

	text := string(out)
	if !strings.Contains(text, "## Excluded") {
		t.Errorf("text output missing the Excluded section:\n%s", text)
	}
	if !strings.Contains(text, "logo.png") || !strings.Contains(text, "binary") {
		t.Errorf("text output missing the excluded file and its reason:\n%s", text)
	}
}

// The whole point of delegation is that no LLM runs on the OCR side, so the
// command must not open session persistence either.
func TestExecuteDelegateScanCreatesNoSession(t *testing.T) {
	home := freshOCRHome(t)

	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "app.go", "package app\n", "add app")
	silenceStdout(t, func() {
		if err := executeDelegateScan(delegateScanOptions{repoDir: dir, format: "text"}); err != nil {
			t.Fatalf("executeDelegateScan error: %v", err)
		}
	})

	assertNoSessionStore(t, home)
}

// A full-file scan needs no refs, so unlike `delegate preview` it must accept
// a plain directory. The docs promise this; the guarantee is asserted here.
func TestExecuteDelegateScan_WorksOutsideGitRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "svc"), 0o755); err != nil {
		t.Fatalf("mkdir svc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "svc", "a.go"), []byte("package svc\n"), 0o644); err != nil {
		t.Fatalf("write svc/a.go: %v", err)
	}

	out := captureDelegateStdout(t, func() {
		if err := executeDelegateScan(delegateScanOptions{repoDir: dir, format: "json", noRules: true}); err != nil {
			t.Fatalf("executeDelegateScan on a non-git directory: %v", err)
		}
	})

	var got delegateScanJSON
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode scan JSON: %v\n%s", err, out)
	}
	if got.ScannableCount != 1 || len(got.Batches) != 1 {
		t.Fatalf("plan = %#v, want the one non-git file planned", got)
	}
	if got.Batches[0].Files[0].Path != "svc/a.go" {
		t.Errorf("planned path = %q, want svc/a.go", got.Batches[0].Files[0].Path)
	}

	// The diff-based sibling still requires git — the two commands differ here
	// on purpose, and a regression that made scan require git would go unnoticed
	// if only scan were asserted.
	silenceStdout(t, func() {
		if err := executeDelegatePreview(delegateOptions{repoDir: dir, format: "json"}); err == nil {
			t.Error("delegate preview must still reject a non-git directory")
		}
	})
}

func TestValidateDelegateScanOptions(t *testing.T) {
	cases := []struct {
		name    string
		opts    delegateScanOptions
		wantErr bool
	}{
		{"text ok", delegateScanOptions{format: "text"}, false},
		{"json ok", delegateScanOptions{format: "json"}, false},
		{"sarif rejected", delegateScanOptions{format: "sarif"}, true},
		{"empty format rejected", delegateScanOptions{format: ""}, true},
		{"negative git procs rejected", delegateScanOptions{format: "text", maxGitProcs: -1}, true},
		{"negative batch size rejected", delegateScanOptions{format: "text", batchSize: -1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDelegateScanOptions(&tc.opts)
			if tc.wantErr && err == nil {
				t.Error("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

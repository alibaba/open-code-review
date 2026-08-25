// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/delegate"
)

func writeRepoFile(t *testing.T, dir, name, content string) error {
	t.Helper()
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

func TestExecuteDelegateTaskJSON(t *testing.T) {
	dir := initTestGitRepo(t)
	if err := writeRepoFile(t, dir, "app.go", "package app\n"); err != nil {
		t.Fatalf("write app.go: %v", err)
	}
	// skip.go is deliberately excluded via --excludes to exercise ExcludedFiles.
	if err := writeRepoFile(t, dir, "skip.go", "package skip\n"); err != nil {
		t.Fatalf("write skip.go: %v", err)
	}
	out := captureDelegateStdout(t, func() {
		if err := executeDelegateTask(delegateOptions{repoDir: dir, format: "json", excludes: "**/skip.go"}); err != nil {
			t.Fatalf("executeDelegateTask(json) error: %v", err)
		}
	})
	var got delegate.TaskSpec
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode task JSON: %v\n%s", err, out)
	}

	if got.SchemaVersion != delegate.TaskSchemaVersion {
		t.Errorf("schema_version = %q, want %q", got.SchemaVersion, delegate.TaskSchemaVersion)
	}
	if got.SchemaVersion != "2" {
		t.Errorf("schema_version = %q, want \"2\"", got.SchemaVersion)
	}
	if got.Repository == "" {
		t.Error("repository must not be empty")
	}
	if got.Scope.Mode != "workspace" {
		t.Errorf("scope.mode = %q, want workspace", got.Scope.Mode)
	}
	if got.Scope.MergeBase != "" {
		t.Errorf("workspace merge_base must be empty, got %q", got.Scope.MergeBase)
	}
	if len(got.Scope.ReviewableFiles) != 1 || got.Scope.ReviewableFiles[0].Path != "app.go" {
		t.Fatalf("reviewable_files = %#v", got.Scope.ReviewableFiles)
	}
	if got.Scope.ReviewableFiles == nil || got.Scope.ExcludedFiles == nil {
		t.Fatal("JSON arrays must not be null")
	}
	var excluded bool
	for _, f := range got.Scope.ExcludedFiles {
		if f.Path == "skip.go" {
			excluded = true
		}
	}
	if !excluded {
		t.Fatalf("excluded_files missing skip.go: %#v", got.Scope.ExcludedFiles)
	}
	if len(got.Diffs) != 1 || got.Diffs[0].Path != "app.go" {
		t.Fatalf("diffs = %#v", got.Diffs)
	}
	if !strings.Contains(got.Diffs[0].Hunk, "package app") {
		t.Errorf("diff hunk missing change content: %q", got.Diffs[0].Hunk)
	}
	if got.Rules == nil {
		t.Error("rules must not be null")
	}
	if len(got.AcceptanceCriteria) != len(delegate.DefaultAcceptanceCriteria()) {
		t.Errorf("acceptance_criteria = %#v", got.AcceptanceCriteria)
	}
}

func TestExecuteDelegateTaskMarkdown(t *testing.T) {
	dir := initTestGitRepo(t)
	if err := writeRepoFile(t, dir, "app.go", "package app\n"); err != nil {
		t.Fatalf("write app.go: %v", err)
	}
	if err := writeRepoFile(t, dir, "skip.go", "package skip\n"); err != nil {
		t.Fatalf("write skip.go: %v", err)
	}
	out := captureDelegateStdout(t, func() {
		if err := executeDelegateTask(delegateOptions{repoDir: dir, format: "text", excludes: "**/skip.go"}); err != nil {
			t.Fatalf("executeDelegateTask(markdown) error: %v", err)
		}
	})
	md := string(out)

	wants := []string{
		"# OCR Delegate Task",
		"- repository:",
		"- mode: workspace",
		"## Scope",
		"### Reviewable",
		"app.go",
		"### Excluded",
		"skip.go", // excluded via --excludes
		"## Diffs",
		"package app",
		"## Acceptance Criteria",
	}
	for _, w := range wants {
		if !strings.Contains(md, w) {
			t.Errorf("markdown output missing %q\n---\n%s", w, md)
		}
	}
}

func TestExecuteDelegateTaskRange(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "b.go", "package b\n", "add b")

	out := captureDelegateStdout(t, func() {
		if err := executeDelegateTask(delegateOptions{repoDir: dir, from: "HEAD~1", to: "HEAD", format: "json"}); err != nil {
			t.Fatalf("executeDelegateTask(range) error: %v", err)
		}
	})
	var got delegate.TaskSpec
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode task JSON: %v\n%s", err, out)
	}
	if got.Scope.Mode != "range" {
		t.Errorf("scope.mode = %q, want range", got.Scope.Mode)
	}
	if got.Scope.MergeBase == "" {
		t.Error("range merge_base must not be empty")
	}
	if len(got.Diffs) != 1 || got.Diffs[0].Path != "b.go" {
		t.Fatalf("diffs = %#v", got.Diffs)
	}
}

// TestExecuteDelegateTaskCreatesNoSession mirrors the preview invariant: the task
// command must not open the review session store, i.e. it calls no LLM and never
// finalizes a review.
func TestExecuteDelegateTaskCreatesNoSession(t *testing.T) {
	home := freshOCRHome(t)
	dir := initTestGitRepo(t)
	if err := writeRepoFile(t, dir, "app.go", "package app\n"); err != nil {
		t.Fatalf("write app.go: %v", err)
	}
	silenceStdout(t, func() {
		if err := executeDelegateTask(delegateOptions{repoDir: dir, format: "json"}); err != nil {
			t.Fatalf("executeDelegateTask error: %v", err)
		}
	})
	assertNoSessionStore(t, home)
}

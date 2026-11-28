// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/capacity"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestCheckChangesetCapacity_NoLimitsReturnsNil(t *testing.T) {
	a := New(Args{})
	a.diffs = []model.Diff{
		{NewPath: "a.go", Diff: "huge"},
		{NewPath: "b.go", Diff: "diffs"},
	}
	if err := a.checkChangesetCapacity(); err != nil {
		t.Fatalf("unconfigured limits must never reject, got %v", err)
	}
	if a.CapacityError() != nil {
		t.Fatalf("CapacityError must stay nil without a rejection")
	}
}

func TestCheckChangesetCapacity_WithinLimitsReturnsNil(t *testing.T) {
	a := New(Args{MaxChangesetFiles: 10, MaxChangesetBytes: 1 << 20})
	a.diffs = []model.Diff{
		{NewPath: "a.go", Diff: "package a"},
	}
	if err := a.checkChangesetCapacity(); err != nil {
		t.Fatalf("within limits must pass, got %v", err)
	}
}

func TestCheckChangesetCapacity_LimitIsInclusiveUpperBound(t *testing.T) {
	a := New(Args{MaxChangesetFiles: 2, MaxChangesetBytes: 16})
	a.diffs = []model.Diff{
		{NewPath: "a.go", Diff: "12345678"},
		{NewPath: "b.go", Diff: "87654321"},
	}
	if err := a.checkChangesetCapacity(); err != nil {
		t.Fatalf("exactly-at-limit must pass, got %v", err)
	}
}

func TestCheckChangesetCapacity_RejectsOverFileLimit(t *testing.T) {
	a := New(Args{MaxChangesetFiles: 1})
	a.diffs = []model.Diff{
		{NewPath: "a.go", Diff: "package a"},
		{NewPath: "b.go", Diff: "package b"},
	}
	err := a.checkChangesetCapacity()
	if err == nil {
		t.Fatal("expected rejection over the file limit")
	}
	var capErr *capacity.Error
	if !errors.As(err, &capErr) {
		t.Fatalf("expected *capacity.Error, got %T", err)
	}
	if capErr.Stage != capacity.StageReviewChangeset {
		t.Errorf("stage = %q, want %q", capErr.Stage, capacity.StageReviewChangeset)
	}
	if capErr.RequestedFiles != 2 || capErr.LimitFiles != 1 {
		t.Errorf("unexpected counts: %+v", capErr)
	}
	if a.CapacityError() != capErr {
		t.Errorf("CapacityError must expose the rejection for the CLI layer")
	}
}

func TestCheckChangesetCapacity_DeletionsNeverCount(t *testing.T) {
	a := New(Args{MaxChangesetFiles: 1})
	a.diffs = []model.Diff{
		{NewPath: "gone.go", IsDeleted: true, Diff: "deleted content"},
		{NewPath: "kept.go", Diff: "package kept"},
	}
	if err := a.checkChangesetCapacity(); err != nil {
		t.Fatalf("deletions are kept for context but never dispatched; got %v", err)
	}
}

func TestCheckChangesetCapacity_RejectsOverByteLimit(t *testing.T) {
	a := New(Args{MaxChangesetBytes: 8})
	a.diffs = []model.Diff{
		{NewPath: "a.go", Diff: "0123456789"},
	}
	err := a.checkChangesetCapacity()
	var capErr *capacity.Error
	if !errors.As(err, &capErr) {
		t.Fatalf("expected *capacity.Error, got %v", err)
	}
	if capErr.RequestedBytes != 10 || capErr.LimitBytes != 8 {
		t.Errorf("unexpected byte fields: %+v", capErr)
	}
}

// TestAgentRun_CapacityRejectionIssuesNoModelRequests drives Run end to end
// against a fake client (workspace review over a real temp repo) and proves
// the pre-flight gate fires before the first model request — including the
// grouping call (#2125's core guarantee on the review path).
func TestAgentRun_CapacityRejectionIssuesNoModelRequests(t *testing.T) {
	repoDir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	writeRepoFile(t, repoDir, "a.go", "package a\n")
	writeRepoFile(t, repoDir, "b.go", "package b\n")
	runGit("add", ".")
	runGit("commit", "-qm", "init")
	// Leave uncommitted modifications so the workspace review has a change set.
	writeRepoFile(t, repoDir, "a.go", "package a modified\n")
	writeRepoFile(t, repoDir, "b.go", "package b modified\n")

	client := &fakeAgentClient{}
	a := New(Args{
		RepoDir:   repoDir,
		LLMClient: client,
		Model:     "fake",
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 5,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{
					{Role: "user", Content: "Review {{diffs}}"},
				},
			},
		},
		MaxChangesetFiles: 1,
	})

	_, err := a.Run(context.Background())
	var capErr *capacity.Error
	if !errors.As(err, &capErr) {
		t.Fatalf("expected *capacity.Error from Run, got %v", err)
	}
	if capErr.Stage != capacity.StageReviewChangeset || capErr.RequestedFiles != 2 || capErr.LimitFiles != 1 {
		t.Errorf("unexpected rejection fields: %+v", capErr)
	}
	if client.calls != 0 {
		t.Errorf("capacity rejection must precede the first model request, got %d", client.calls)
	}
	if m := a.RunManifest(); m == nil || m.TerminalState != session.StateFailed {
		t.Errorf("rejection must finalize a failed manifest, got %+v", m)
	}
}

func writeRepoFile(t *testing.T, repoDir, rel, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(repoDir, rel)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, rel), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"errors"
	"testing"

	"github.com/alibaba/open-code-review/internal/capacity"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestScanCheckChangesetCapacity_NoLimitsReturnsNil(t *testing.T) {
	a := NewAgent(Args{Tools: tool.NewRegistry()})
	a.items = []model.ScanItem{
		{Path: "a.go", Content: "package a"},
	}
	if err := a.checkChangesetCapacity(); err != nil {
		t.Fatalf("unconfigured limits must never reject, got %v", err)
	}
}

func TestScanCheckChangesetCapacity_RejectsOverFileLimit(t *testing.T) {
	a := NewAgent(Args{Tools: tool.NewRegistry(), MaxChangesetFiles: 1})
	a.items = []model.ScanItem{
		{Path: "a.go", Content: "package a"},
		{Path: "b.go", Content: "package b"},
	}
	err := a.checkChangesetCapacity()
	var capErr *capacity.Error
	if !errors.As(err, &capErr) {
		t.Fatalf("expected *capacity.Error, got %v", err)
	}
	if capErr.Stage != capacity.StageScanChangeset {
		t.Errorf("stage = %q, want %q", capErr.Stage, capacity.StageScanChangeset)
	}
	if capErr.RequestedFiles != 2 || capErr.LimitFiles != 1 {
		t.Errorf("unexpected fields: %+v", capErr)
	}
	if a.CapacityError() != capErr {
		t.Errorf("CapacityError must expose the rejection for the CLI layer")
	}
}

func TestScanCheckChangesetCapacity_BinarySkippedEmptyCounts(t *testing.T) {
	a := NewAgent(Args{Tools: tool.NewRegistry(), MaxChangesetFiles: 1})
	a.items = []model.ScanItem{
		{Path: "img.png", IsBinary: true, Content: "\x00\x01binaryblob"},
		{Path: "empty.go", Content: ""},
		{Path: "real.go", Content: "package real"},
	}
	// Binary items never dispatch and are excluded; empty-content items DO
	// dispatch (only the plan phase skips them), so they must count.
	var capErr *capacity.Error
	if !errors.As(a.checkChangesetCapacity(), &capErr) {
		t.Fatal("expected rejection: empty-content items dispatch and must count")
	}
	if capErr.RequestedFiles != 2 {
		t.Errorf("requested_files = %d, want 2 (empty + real; binary excluded)", capErr.RequestedFiles)
	}
}

func TestScanCheckChangesetCapacity_RejectsOverByteLimit(t *testing.T) {
	a := NewAgent(Args{Tools: tool.NewRegistry(), MaxChangesetBytes: 8})
	a.items = []model.ScanItem{
		{Path: "a.go", Content: "0123456789"},
	}
	var capErr *capacity.Error
	if !errors.As(a.checkChangesetCapacity(), &capErr) {
		t.Fatal("expected rejection over the byte limit")
	}
	if capErr.RequestedBytes != 10 || capErr.LimitBytes != 8 {
		t.Errorf("unexpected byte fields: %+v", capErr)
	}
}

// TestScanRun_CapacityRejectionIssuesNoModelRequests drives Run end to end
// against a fake client and proves the pre-flight gate fires before the first
// model request (#2125's core guarantee).
func TestScanRun_CapacityRejectionIssuesNoModelRequests(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "a.go", []byte("package a\n"))
	writeFile(t, repo, "b.go", []byte("package b\n"))
	gitCommit(t, repo, "init")

	client := &fakeScanClient{}
	sess := session.New(repo, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeFullScan,
	})
	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 1000

	a := NewAgent(Args{
		RepoDir:          repo,
		Template:         tpl,
		LLMClient:        client,
		Model:            "test-model",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
			{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
		},
		SkipPlan:          true,
		SkipDedup:         true,
		SkipSummary:       true,
		MaxConcurrency:    1,
		Session:           sess,
		MaxChangesetFiles: 1,
	})

	_, err := a.Run(context.Background())
	var capErr *capacity.Error
	if !errors.As(err, &capErr) {
		t.Fatalf("expected *capacity.Error from Run, got %v", err)
	}
	if capErr.Stage != capacity.StageScanChangeset || capErr.RequestedFiles != 2 || capErr.LimitFiles != 1 {
		t.Errorf("unexpected rejection fields: %+v", capErr)
	}
	if client.calls != 0 {
		t.Errorf("capacity rejection must precede the first model request, got %d", client.calls)
	}
}

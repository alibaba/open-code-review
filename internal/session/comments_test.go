// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestLoadComments_ReturnsCommentsInOrder(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{
		{Path: "a.go", Content: "first", Severity: "high", Category: "bug"},
		{Content: "second, no path", Severity: "low"},
	})
	sh.RecordReviewItemReused("b.go", "b.go", "b.go", "fp-b", "prior-session", []model.LlmComment{
		{Path: "b.go", Content: "cached", Severity: "medium"},
	})
	sh.RecordReviewItemFailed("c.go", "c.go", "c.go", "fp-c", "boom")
	sh.Finalize()

	got, err := LoadComments(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("LoadComments: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 comments, got %d: %+v", len(got), got)
	}
	if got[0].Content != "first" || got[1].Content != "second, no path" || got[2].Content != "cached" {
		t.Errorf("unexpected order: %+v", got)
	}
	if got[1].Path != "a.go" {
		t.Errorf("comment without path should inherit record file path, got %q", got[1].Path)
	}
}

func TestLoadComments_LaterCheckpointSupersedes(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{
		{Path: "a.go", Content: "stale"},
	})
	sh.RecordReviewItemFailed("a.go", "a.go", "a.go", "fp-a", "stopped", model.LlmComment{Path: "a.go", Content: "fresh"})
	sh.RecordReviewItemDone("b.go", "b.go", "b.go", "fp-b", []model.LlmComment{
		{Path: "b.go", Content: "kept"},
	})
	sh.RecordReviewItemFailed("b.go", "b.go", "b.go", "fp-b", "boom")
	sh.Finalize()

	got, err := LoadComments(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("LoadComments: %v", err)
	}
	if len(got) != 1 || got[0].Content != "fresh" {
		t.Fatalf("expected only the superseding comment, got %+v", got)
	}
}

func TestLoadComments_FailedItemFindingsAreVisibleWithoutReuse(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{
		Operation:  OperationReview,
		ReviewMode: ReviewModeCommit,
		DiffCommit: "abc123",
	})
	b := sh.Manifest()
	b.SetInput(ManifestInput{Mode: InputModeCommit})
	item := sel("a")
	if err := b.RegisterSelected(item); err != nil {
		t.Fatal(err)
	}
	finding := model.LlmComment{Content: "confirmed before the stop"}
	if err := b.MarkFailed(item.ItemID, FailureBudget, "round budget exhausted", finding); err != nil {
		t.Fatal(err)
	}
	sh.RecordReviewItemFailed("", item.Path, item.Path, item.Fingerprint, "unfinished", finding)
	manifest := mustFinalize(t, b)
	sh.SetFinalManifest(&manifest)
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}

	comments, err := LoadComments(repoDir, sh.SessionID)
	if err != nil || len(comments) != 1 || comments[0].Path != item.Path || comments[0].Content != finding.Content {
		t.Fatalf("comments = %+v, error = %v, want confirmed finding on %s", comments, err, item.Path)
	}
	summary, details, err := LoadDetail(repoDir, sh.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalComments != 1 || summary.FailedFiles != 1 || summary.CompletedFiles != 0 ||
		summary.RunManifest == nil || summary.RunManifest.TerminalState != StatePartial {
		t.Fatalf("summary = %+v, want one failed file with one comment", summary)
	}
	if len(details) != 1 || details[0].Type != "failed" || details[0].Comments != 1 || details[0].Error != "unfinished" {
		t.Fatalf("details = %+v, want one failed record with its finding and reason", details)
	}
	resume, err := LoadReviewResumeState(repoDir, sh.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resume.ReusableItem(item.Fingerprint); ok || len(resume.Items) != 0 {
		t.Fatalf("failed item was added to resume index: %+v", resume.Items)
	}
}

func TestLoadComments_MissingSession(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	if _, err := LoadComments(t.TempDir(), "nonexistent"); err == nil {
		t.Fatal("expected error for missing session")
	}
}

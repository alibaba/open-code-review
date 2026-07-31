package session

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestLoadComments_ReturnsCommentsInOrder(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
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
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{
		{Path: "a.go", Content: "stale"},
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{
		{Path: "a.go", Content: "fresh"},
	})
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

func TestLoadComments_MissingSession(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if _, err := LoadComments(t.TempDir(), "nonexistent"); err == nil {
		t.Fatal("expected error for missing session")
	}
}

package session

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/open-code-review/open-code-review/internal/model"
)

func TestListSessions_EmptyRepoReturnsNil(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	got, err := ListSessions(t.TempDir())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d entries", len(got))
	}
}

func TestListSessions_SortsAndAggregates(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	older := writeTestSession(t, repoDir, "feature-a", "commit-x", []model.LlmComment{
		{Path: "a.go", Content: "one"},
	}, 1, 0, true)
	time.Sleep(1100 * time.Millisecond)
	newer := writeTestSession(t, repoDir, "feature-a", "commit-y", []model.LlmComment{
		{Path: "b.go", Content: "one"},
		{Path: "b.go", Content: "two"},
	}, 2, 1, false)

	got, err := ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(got))
	}
	if got[0].SessionID != newer {
		t.Errorf("expected newest first, got %q vs %q", got[0].SessionID, newer)
	}
	if got[1].SessionID != older {
		t.Errorf("expected older second, got %q", got[1].SessionID)
	}

	if !got[0].Aborted {
		t.Errorf("newest session was interrupted; expected Aborted=true")
	}
	if got[1].Aborted {
		t.Errorf("older session was finalized; expected Aborted=false")
	}
	if got[0].TotalComments != 2 {
		t.Errorf("newest TotalComments = %d, want 2", got[0].TotalComments)
	}
	if got[0].FailedFiles != 1 {
		t.Errorf("newest FailedFiles = %d, want 1", got[0].FailedFiles)
	}
	if got[0].CompletedFiles != 2 {
		t.Errorf("newest CompletedFiles = %d, want 2", got[0].CompletedFiles)
	}
}

func TestLoadDetail_ReturnsItems(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{{Path: "a.go", Content: "note"}})
	sh.RecordReviewItemReused("b.go", "b.go", "b.go", "fp-b", "prior-session", []model.LlmComment{{Path: "b.go", Content: "cached"}})
	sh.RecordReviewItemFailed("c.go", "c.go", "c.go", "fp-c", "boom")
	sh.Finalize()

	summary, items, err := LoadDetail(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("LoadDetail: %v", err)
	}
	if summary.CompletedFiles != 1 || summary.ReusedFiles != 1 || summary.FailedFiles != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.TotalComments != 2 {
		t.Errorf("TotalComments = %d, want 2", summary.TotalComments)
	}
	if summary.Aborted {
		t.Errorf("summary should not be aborted after Finalize")
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	byType := map[string]ItemDetail{}
	for _, it := range items {
		byType[it.Type] = it
	}
	if reused := byType["reused"]; reused.SourceSessionID != "prior-session" {
		t.Errorf("reused source = %q, want prior-session", reused.SourceSessionID)
	}
	if failed := byType["failed"]; failed.Error != "boom" {
		t.Errorf("failed error = %q, want boom", failed.Error)
	}
	if done := byType["done"]; done.Comments != 1 {
		t.Errorf("done comments = %d, want 1", done.Comments)
	}
}

func TestLoadSummary_FallsBackToSessionEndFilesReviewed(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeWorkspace,
	})
	sh.GetOrCreateFileSession("legacy-a.go")
	sh.GetOrCreateFileSession("legacy-b.go")
	sh.Finalize()

	summary, items, err := LoadDetail(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("LoadDetail: %v", err)
	}
	if summary.CompletedFiles != 2 {
		t.Fatalf("CompletedFiles = %d, want 2", summary.CompletedFiles)
	}
	if summary.ReusedFiles != 0 || summary.FailedFiles != 0 {
		t.Fatalf("unexpected checkpoint counts: %+v", summary)
	}
	if len(items) != 0 {
		t.Fatalf("legacy session should not synthesize item details, got %d", len(items))
	}
}

func TestLoadSummary_MissingFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if _, err := LoadSummary(t.TempDir(), "nonexistent"); err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestLoadComments_ReturnsContent(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{
		{Path: "a.go", Content: "note", Category: "bug", Severity: "high"},
	})
	sh.RecordReviewItemReused("b.go", "b.go", "b.go", "fp-b", "prior-session", []model.LlmComment{
		{Path: "b.go", Content: "cached", Category: "style", Severity: "low"},
	})
	// Failed records carry no comments and must contribute nothing.
	sh.RecordReviewItemFailed("c.go", "c.go", "c.go", "fp-c", "boom")
	sh.Finalize()

	summary, entries, err := LoadComments(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("LoadComments: %v", err)
	}

	// done + reused only; failed naturally excluded.
	if len(entries) != 2 {
		t.Fatalf("expected 2 comment entries (done+reused), got %d: %+v", len(entries), entries)
	}
	if entries[0].Type != "done" || entries[0].FilePath != "a.go" {
		t.Errorf("entry[0] = %+v", entries[0])
	}
	if len(entries[0].Comments) != 1 || entries[0].Comments[0].Content != "note" {
		t.Errorf("done comments = %+v", entries[0].Comments)
	}
	if entries[1].Type != "reused" || entries[1].FilePath != "b.go" {
		t.Errorf("entry[1] = %+v", entries[1])
	}
	if len(entries[1].Comments) != 1 || entries[1].Comments[0].Content != "cached" {
		t.Errorf("reused comments = %+v", entries[1].Comments)
	}

	// Parity: LoadComments' summary TotalComments must equal LoadDetail's
	// because both walk the same records via applyRecordToSummary.
	detailSummary, _, err := LoadDetail(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("LoadDetail: %v", err)
	}
	if summary.TotalComments != detailSummary.TotalComments {
		t.Errorf("TotalComments parity broken: LoadComments=%d, LoadDetail=%d",
			summary.TotalComments, detailSummary.TotalComments)
	}
}

func TestLoadComments_EmptySession(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeCommit,
		DiffCommit: "abc123",
	})
	// Only a failed record (no comments) and no done/reused records.
	sh.RecordReviewItemFailed("c.go", "c.go", "c.go", "fp-c", "boom")
	sh.Finalize()

	summary, entries, err := LoadComments(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("LoadComments: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for comment-less session, got %d", len(entries))
	}
	if summary.TotalComments != 0 {
		t.Errorf("TotalComments = %d, want 0", summary.TotalComments)
	}
}

func TestLoadComments_PreservesRecordOrder(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeCommit,
		DiffCommit: "abc123",
	})
	// Two done records + one reused in deliberate order; on-disk order must be preserved.
	sh.RecordReviewItemDone("first.go", "first.go", "first.go", "fp-1", []model.LlmComment{
		{Path: "first.go", Content: "one"},
	})
	sh.RecordReviewItemReused("second.go", "second.go", "second.go", "fp-2", "src", []model.LlmComment{
		{Path: "second.go", Content: "two"},
	})
	sh.RecordReviewItemDone("third.go", "third.go", "third.go", "fp-3", []model.LlmComment{
		{Path: "third.go", Content: "three"},
	})
	sh.Finalize()

	_, entries, err := LoadComments(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("LoadComments: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	wantPaths := []string{"first.go", "second.go", "third.go"}
	for i, want := range wantPaths {
		if entries[i].FilePath != want {
			t.Errorf("entry[%d] FilePath = %q, want %q (on-disk order not preserved)", i, entries[i].FilePath, want)
		}
	}
}

func TestLoadComments_MissingFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if _, _, err := LoadComments(t.TempDir(), "nonexistent"); err == nil {
		t.Fatal("expected error for missing session")
	}
}

// writeTestSession creates a real JSONL session using the persistence layer
// so tests exercise the same on-disk format that ListSessions consumes.
// It returns the session id.
func writeTestSession(t *testing.T, repoDir, from, to string, comments []model.LlmComment, doneCount, failedCount int, finalize bool) string {
	t.Helper()
	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeRange,
		DiffFrom:   from,
		DiffTo:     to,
	})
	for i := 0; i < doneCount; i++ {
		filePath := filepath.Base(t.TempDir()) + ".go"
		var perFile []model.LlmComment
		if i < len(comments) {
			perFile = []model.LlmComment{comments[i]}
		}
		sh.RecordReviewItemDone(filePath, filePath, filePath, "fp-"+filePath, perFile)
	}
	for i := 0; i < failedCount; i++ {
		filePath := "failed-" + filepath.Base(t.TempDir()) + ".go"
		sh.RecordReviewItemFailed(filePath, filePath, filePath, "fp-fail-"+filePath, "test error")
	}
	if finalize {
		sh.Finalize()
	} else {
		// Simulate an aborted run: flush the writer without emitting session_end.
		if sh.persist != nil {
			sh.persist.mu.Lock()
			if sh.persist.writer != nil {
				sh.persist.writer.Flush()
			}
			if sh.persist.file != nil {
				_ = sh.persist.file.Close()
			}
			sh.persist.writer = nil
			sh.persist.file = nil
			sh.persist.mu.Unlock()
		}
	}
	return sh.SessionID
}

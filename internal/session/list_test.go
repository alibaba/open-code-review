// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
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

func TestListSessions_FiltersCollidingLegacySessionsByRepository(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	base := t.TempDir()
	repoA := filepath.Join(base, "team", "service-api")
	repoB := filepath.Join(base, "team-service", "api")
	if err := os.MkdirAll(repoA, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repoB, 0755); err != nil {
		t.Fatal(err)
	}

	legacyDir := filepath.Join(tmpHome, ".opencodereview", "test-sessions", encodeRepoPath(repoA))
	if err := os.MkdirAll(legacyDir, 0700); err != nil {
		t.Fatal(err)
	}
	writeJSONL := func(sessionID, cwd string) {
		t.Helper()
		path := filepath.Join(legacyDir, sessionID+".jsonl")
		data, err := json.Marshal(map[string]string{
			"type":      "session_start",
			"sessionId": sessionID,
			"timestamp": "2026-08-01T00:00:00Z",
			"cwd":       cwd,
		})
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSONL("repo-a-session", repoA)
	writeJSONL("repo-b-session", repoB)
	commentRecord, err := json.Marshal(map[string]any{
		"type":        "review_item_done",
		"filePath":    "legacy.go",
		"fingerprint": "fp-legacy",
		"comments":    []model.LlmComment{{Content: "legacy comment"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	commentFile, err := os.OpenFile(filepath.Join(legacyDir, "repo-a-session.jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commentFile.Write(append(commentRecord, '\n')); err != nil {
		commentFile.Close()
		t.Fatal(err)
	}
	if err := commentFile.Close(); err != nil {
		t.Fatal(err)
	}

	gotA, err := ListSessions(repoA)
	if err != nil {
		t.Fatalf("ListSessions(repoA): %v", err)
	}
	if len(gotA) != 1 || gotA[0].SessionID != "repo-a-session" {
		t.Fatalf("repo A sessions = %+v, want only repo-a-session", gotA)
	}

	gotB, err := ListSessions(repoB)
	if err != nil {
		t.Fatalf("ListSessions(repoB): %v", err)
	}
	if len(gotB) != 1 || gotB[0].SessionID != "repo-b-session" {
		t.Fatalf("repo B sessions = %+v, want only repo-b-session", gotB)
	}

	if _, err := LoadSummary(repoA, "repo-b-session"); !os.IsNotExist(err) {
		t.Fatalf("LoadSummary(repoA, repo-b-session) error = %v, want not found", err)
	}

	detail, items, err := LoadDetail(repoA, "repo-a-session")
	if err != nil {
		t.Fatalf("LoadDetail(repoA, repo-a-session): %v", err)
	}
	if detail.RepoDir != repoA || len(items) != 1 || items[0].FilePath != "legacy.go" {
		t.Fatalf("legacy detail = (%+v, %+v), want repo A legacy item", detail, items)
	}
	if _, _, err := LoadDetail(repoB, "repo-a-session"); !os.IsNotExist(err) {
		t.Fatalf("LoadDetail(repoB, repo-a-session) error = %v, want not found", err)
	}

	state, err := LoadResumeState(repoA, "repo-a-session")
	if err != nil {
		t.Fatalf("LoadResumeState(repoA, repo-a-session): %v", err)
	}
	if state.RepoDir != repoA {
		t.Fatalf("legacy resume RepoDir = %q, want %q", state.RepoDir, repoA)
	}
	if _, err := LoadResumeState(repoB, "repo-a-session"); !os.IsNotExist(err) {
		t.Fatalf("LoadResumeState(repoB, repo-a-session) error = %v, want not found", err)
	}

	comments, err := LoadComments(repoA, "repo-a-session")
	if err != nil {
		t.Fatalf("LoadComments(repoA, repo-a-session): %v", err)
	}
	if len(comments) != 1 || comments[0].Content != "legacy comment" || comments[0].Path != "legacy.go" {
		t.Fatalf("legacy comments = %+v, want one inherited-path comment", comments)
	}
	if _, err := LoadComments(repoB, "repo-a-session"); !os.IsNotExist(err) {
		t.Fatalf("LoadComments(repoB, repo-a-session) error = %v, want not found", err)
	}
}

func TestListSessions_DeduplicatesCurrentAndLegacySessionID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	currentDir, legacyDir, err := sessionDirectories(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{currentDir, legacyDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}

	const sessionID = "duplicate-session"
	writeStart := func(dir, model string) {
		t.Helper()
		data, marshalErr := json.Marshal(map[string]string{
			"type":      "session_start",
			"sessionId": sessionID,
			"timestamp": "2026-08-01T00:00:00Z",
			"cwd":       repoDir,
			"model":     model,
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), append(data, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeStart(currentDir, "current-model")
	writeStart(legacyDir, "legacy-model")

	summaries, err := ListSessions(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("sessions = %+v, want one deduplicated session", summaries)
	}
	if summaries[0].SessionID != sessionID || summaries[0].Model != "current-model" {
		t.Fatalf("session = %+v, want current-format version", summaries[0])
	}
}

func TestNewSessionsUseDistinctDirectoriesForCollidingPaths(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	base := t.TempDir()
	repoA := filepath.Join(base, "team", "service-api")
	repoB := filepath.Join(base, "team-service", "api")
	if err := os.MkdirAll(repoA, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repoB, 0755); err != nil {
		t.Fatal(err)
	}

	first := New(repoA, "main", "mock", SessionOptions{ReviewMode: ReviewModeWorkspace})
	if err := first.Finalize(); err != nil {
		t.Fatal(err)
	}
	second := New(repoB, "main", "mock", SessionOptions{ReviewMode: ReviewModeWorkspace})
	if err := second.Finalize(); err != nil {
		t.Fatal(err)
	}

	dirA, err := SessionsDir(repoA)
	if err != nil {
		t.Fatal(err)
	}
	dirB, err := SessionsDir(repoB)
	if err != nil {
		t.Fatal(err)
	}
	if dirA == dirB {
		t.Fatalf("session directories collided: %q", dirA)
	}
	if _, err := os.Stat(filepath.Join(dirA, first.SessionID+".jsonl")); err != nil {
		t.Fatalf("repo A session missing from its directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dirB, second.SessionID+".jsonl")); err != nil {
		t.Fatalf("repo B session missing from its directory: %v", err)
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
	if !summary.Legacy || summary.RunManifest != nil {
		t.Fatalf("legacy summary flags = legacy:%v manifest:%v", summary.Legacy, summary.RunManifest)
	}
}

func TestLoadSummaryPrefersV1RunManifest(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeWorkspace,
		Operation:  OperationReview,
	})
	b := sh.Manifest()
	b.SetInput(ManifestInput{Mode: InputModeWorkspace})
	items := []CoverageItem{
		{ItemID: "a", Path: "a.go"},
		{ItemID: "b", Path: "b.go"},
		{ItemID: "c", Path: "c.go"},
		{ItemID: "d", Path: "d.go"},
	}
	for _, item := range items {
		if err := b.RegisterSelected(item); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.SealSelected(); err != nil {
		t.Fatal(err)
	}
	if err := b.MarkCompleted("a"); err != nil {
		t.Fatal(err)
	}
	if err := b.MarkReused("b"); err != nil {
		t.Fatal(err)
	}
	if err := b.MarkFailed("c", FailureProvider, "provider request failed"); err != nil {
		t.Fatal(err)
	}
	if err := b.MarkWaived("d", "accepted by user"); err != nil {
		t.Fatal(err)
	}
	manifest, err := b.Finalize(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	sh.SetFinalManifest(&manifest)
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}

	summary, err := LoadSummary(repoDir, sh.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Legacy || summary.Aborted || summary.RunManifest == nil {
		t.Fatalf("summary flags = legacy:%v aborted:%v manifest:%v", summary.Legacy, summary.Aborted, summary.RunManifest)
	}
	if summary.RunManifest.TerminalState != StatePartial {
		t.Fatalf("terminal_state = %q", summary.RunManifest.TerminalState)
	}
	if summary.SelectedFiles != 4 || summary.CompletedFiles != 1 || summary.ReusedFiles != 1 || summary.FailedFiles != 1 || summary.WaivedFiles != 1 {
		t.Fatalf("coverage counts = %+v", summary)
	}
}

func TestLoadSummaryIgnoresUnknownManifestVersion(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	sh.SetFinalManifest(&RunManifest{SchemaVersion: "ocr.run-manifest/v999", TerminalState: StateComplete})
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}
	summary, err := LoadSummary(repoDir, sh.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !summary.Legacy || summary.RunManifest != nil {
		t.Fatalf("unknown schema must be treated as legacy: %+v", summary)
	}
}

func TestLoadSummary_MissingFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if _, err := LoadSummary(t.TempDir(), "nonexistent"); err == nil {
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

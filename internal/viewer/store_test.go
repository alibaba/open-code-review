// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/session"
)

func writeJSONL(t *testing.T, path string, lines ...string) {
	t.Helper()
	var content string
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func sessionStartLine(t *testing.T, timestamp, cwd string) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{
		"type":      "session_start",
		"timestamp": timestamp,
		"cwd":       cwd,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDiscoverRepos_Empty(t *testing.T) {
	root := t.TempDir()
	repos, err := DiscoverRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func TestDiscoverRepos_NonExistentDir(t *testing.T) {
	repos, err := DiscoverRepos("/nonexistent/path/abc123")
	if err != nil {
		t.Fatal(err)
	}
	if repos != nil {
		t.Errorf("expected nil for non-existent dir, got %v", repos)
	}
}

func TestDiscoverRepos_SkipsFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "stray.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	repos, err := DiscoverRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func TestDiscoverRepos_FindsRepos(t *testing.T) {
	root := t.TempDir()

	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	if err := os.MkdirAll(repoA, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repoB, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoA, "session1.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T10:00:00Z"}`)
	writeJSONL(t, filepath.Join(repoA, "session2.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-02T10:00:00Z"}`)
	writeJSONL(t, filepath.Join(repoB, "session3.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-03T10:00:00Z"}`)

	// Ensure repo-b's file has a strictly later mtime so sort-by-ModTime is deterministic.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(repoB, "session3.jsonl"), future, future); err != nil {
		t.Fatal(err)
	}

	repos, err := DiscoverRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	if repos[0].EncodedPath != "repo-b" {
		t.Errorf("expected most recent repo first, got %q", repos[0].EncodedPath)
	}
	if repos[1].SessionCount != 2 {
		t.Errorf("repo-a session count = %d, want 2", repos[1].SessionCount)
	}
}

func TestDiscoverRepos_SplitsCollidingLegacyRepositories(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "team-service-api")
	if err := os.MkdirAll(legacy, 0755); err != nil {
		t.Fatal(err)
	}
	repoA := filepath.Join(t.TempDir(), "team", "service-api")
	repoB := filepath.Join(t.TempDir(), "team-service", "api")
	writeJSONL(t, filepath.Join(legacy, "a.jsonl"),
		sessionStartLine(t, "2026-08-01T00:00:00Z", repoA))
	writeJSONL(t, filepath.Join(legacy, "b.jsonl"),
		sessionStartLine(t, "2026-08-02T00:00:00Z", repoB))

	repos, err := DiscoverRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected two logical repositories, got %+v", repos)
	}
	keys := map[string]int{}
	for _, repo := range repos {
		keys[repo.EncodedPath] = repo.SessionCount
	}
	if keys[session.RepoSessionKey(repoA)] != 1 || keys[session.RepoSessionKey(repoB)] != 1 {
		t.Fatalf("legacy repositories were not split by cwd: %+v", keys)
	}

	sessions, err := ListSessions(root, session.RepoSessionKey(repoA))
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "a" {
		t.Fatalf("repo A sessions = %+v, want only a", sessions)
	}
	if _, err := LoadSession(root, session.RepoSessionKey(repoB), "a"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadSession(repo B, a) error = %v, want not found", err)
	}
}

func TestViewerMergesCurrentAndLegacySessionsForRepository(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(t.TempDir(), "repo")
	currentKey := session.RepoSessionKey(repoDir)
	currentDir := filepath.Join(root, currentKey)
	legacyDir := filepath.Join(root, "legacy-repo")
	if err := os.MkdirAll(currentDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(currentDir, "current.jsonl"),
		sessionStartLine(t, "2026-08-02T00:00:00Z", repoDir))
	writeJSONL(t, filepath.Join(legacyDir, "legacy.jsonl"),
		sessionStartLine(t, "2026-08-01T00:00:00Z", repoDir))

	repos, err := DiscoverRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].EncodedPath != currentKey || repos[0].SessionCount != 2 {
		t.Fatalf("repositories = %+v, want one merged repository with two sessions", repos)
	}

	sessions, err := ListSessions(root, currentKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[0].SessionID != "current" || sessions[1].SessionID != "legacy" {
		t.Fatalf("sessions = %+v, want current and legacy in timestamp order", sessions)
	}

	loaded, err := LoadSession(root, currentKey, "legacy")
	if err != nil {
		t.Fatalf("LoadSession legacy fallback: %v", err)
	}
	if loaded.Summary.SessionID != "legacy" || loaded.Summary.CWD != repoDir {
		t.Fatalf("loaded legacy session = %+v", loaded.Summary)
	}
}

func TestDiscoverRepos_SkipsDirsWithNoJSONL(t *testing.T) {
	root := t.TempDir()
	emptyRepo := filepath.Join(root, "empty-repo")
	if err := os.MkdirAll(emptyRepo, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emptyRepo, "readme.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	repos, err := DiscoverRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos for dir with no .jsonl, got %d", len(repos))
	}
}

func TestListSessions(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "myrepo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "aaa.jsonl"),
		`{"type":"session_start","timestamp":"2025-03-01T09:00:00Z","cwd":"/home/user/proj","gitBranch":"main","model":"gpt-4","reviewMode":"workspace"}`,
		`{"type":"session_end","duration_seconds":120.5,"files_reviewed":["a.go","b.go"],"llm_failures":1}`)

	writeJSONL(t, filepath.Join(repoDir, "bbb.jsonl"),
		`{"type":"session_start","timestamp":"2025-03-02T10:00:00Z","cwd":"/home/user/proj","gitBranch":"feat","model":"claude","reviewMode":"commit","diffCommit":"abc123"}`,
		`{"type":"session_end","duration_seconds":60.0,"files_reviewed":["c.go"],"llm_failures":0}`)

	// Non-jsonl file should be skipped
	if err := os.WriteFile(filepath.Join(repoDir, "notes.txt"), []byte("ignored"), 0644); err != nil {
		t.Fatal(err)
	}

	sessions, err := ListSessions(root, "myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Should be sorted newest first
	if sessions[0].SessionID != "bbb" {
		t.Errorf("expected newest session first, got %q", sessions[0].SessionID)
	}
	if sessions[0].Model != "claude" {
		t.Errorf("Model = %q", sessions[0].Model)
	}
	if sessions[0].ReviewMode != "commit" {
		t.Errorf("ReviewMode = %q", sessions[0].ReviewMode)
	}
	if sessions[0].DiffCommit != "abc123" {
		t.Errorf("DiffCommit = %q", sessions[0].DiffCommit)
	}
	if sessions[0].DurationSec != 60.0 {
		t.Errorf("DurationSec = %f", sessions[0].DurationSec)
	}
	if sessions[0].FileCount != 1 {
		t.Errorf("FileCount = %d", sessions[0].FileCount)
	}

	if sessions[1].SessionID != "aaa" {
		t.Errorf("second session = %q", sessions[1].SessionID)
	}
	if sessions[1].LLMFailures != 1 {
		t.Errorf("LLMFailures = %d", sessions[1].LLMFailures)
	}
}

func TestPeekSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	writeJSONL(t, path,
		`{"type":"session_start","timestamp":"2025-06-15T14:30:00Z","cwd":"/repo","gitBranch":"dev","model":"gpt-4o","reviewMode":"range","diffFrom":"a1b2","diffTo":"c3d4"}`,
		`{"type":"llm_request","filePath":"main.go","taskType":"main_task","request_no":1}`,
		`{"type":"llm_response","filePath":"main.go","taskType":"main_task","content":"looks good"}`,
		`{"type":"session_end","duration_seconds":45.2,"files_reviewed":["main.go","util.go"],"llm_failures":2}`)

	s, err := peekSession(path)
	if err != nil {
		t.Fatal(err)
	}

	expected := time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)
	if !s.Timestamp.Equal(expected) {
		t.Errorf("Timestamp = %v, want %v", s.Timestamp, expected)
	}
	if s.CWD != "/repo" {
		t.Errorf("CWD = %q", s.CWD)
	}
	if s.GitBranch != "dev" {
		t.Errorf("GitBranch = %q", s.GitBranch)
	}
	if s.Model != "gpt-4o" {
		t.Errorf("Model = %q", s.Model)
	}
	if s.ReviewMode != "range" {
		t.Errorf("ReviewMode = %q", s.ReviewMode)
	}
	if s.DiffFrom != "a1b2" {
		t.Errorf("DiffFrom = %q", s.DiffFrom)
	}
	if s.DiffTo != "c3d4" {
		t.Errorf("DiffTo = %q", s.DiffTo)
	}
	if s.DurationSec != 45.2 {
		t.Errorf("DurationSec = %f", s.DurationSec)
	}
	if len(s.FilesReviewed) != 2 {
		t.Errorf("FilesReviewed = %v", s.FilesReviewed)
	}
	if s.LLMFailures != 2 {
		t.Errorf("LLMFailures = %d", s.LLMFailures)
	}
	if s.FileCount != 2 {
		t.Errorf("FileCount = %d", s.FileCount)
	}
}

func TestPeekSession_MissingFile(t *testing.T) {
	_, err := peekSession("/nonexistent/path/session.jsonl")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestPeekSession_NoSessionEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.jsonl")
	writeJSONL(t, path,
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`)

	s, err := peekSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.CWD != "/x" {
		t.Errorf("CWD = %q", s.CWD)
	}
	if s.DurationSec != 0 {
		t.Errorf("DurationSec should be 0 without session_end, got %f", s.DurationSec)
	}
	if s.FileCount != 0 {
		t.Errorf("FileCount should be 0 without session_end, got %d", s.FileCount)
	}
	if !s.Aborted || s.Legacy {
		t.Fatalf("unfinished session flags = aborted:%v legacy:%v", s.Aborted, s.Legacy)
	}
}

func TestPeekSessionUsesV1ManifestCoverage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.jsonl")
	writeJSONL(t, path,
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"session_end","duration_seconds":2,"files_reviewed":["legacy.go"],"run_manifest":{"schema_version":"ocr.run-manifest/v1","run_id":"run-1","operation":"review","terminal_state":"partial","repository":{},"input":{"mode":"workspace"},"execution":{},"coverage":{"selected":[{"item_id":"a","path":"a.go"},{"item_id":"b","path":"b.go"}],"completed":[{"item_id":"a","path":"a.go"}],"reused":[],"failed":[{"item_id":"b","path":"b.go","classification":"provider"}],"waived":[]},"elapsed_ms":2000}}`)

	s, err := peekSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Aborted || s.Legacy || s.RunManifest == nil {
		t.Fatalf("manifest flags = aborted:%v legacy:%v manifest:%v", s.Aborted, s.Legacy, s.RunManifest)
	}
	if s.TerminalState != "partial" || s.FileCount != 2 || s.SelectedCount != 2 || s.CompletedCount != 1 || s.FailedCount != 1 {
		t.Fatalf("manifest summary = %+v", s)
	}
}

func TestPeekSessionUnknownManifestIsLegacy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "future.jsonl")
	writeJSONL(t, path,
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z"}`,
		`{"type":"session_end","files_reviewed":["a.go"],"run_manifest":{"schema_version":"ocr.run-manifest/v2","terminal_state":"complete"}}`)
	s, err := peekSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Legacy || s.RunManifest != nil || s.FileCount != 1 {
		t.Fatalf("future schema summary = %+v", s)
	}
}

package viewer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
}

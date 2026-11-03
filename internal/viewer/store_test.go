// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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

func TestBuildGroupingIndex(t *testing.T) {
	// The list mirrors buildFileList([%d] ) + formatDiffEntry (STATUS   path (+N/-M))
	// across every status, plus a path with a space to prove the "(+N/-M)" anchor.
	content := "Group the following changed files:\n\n" +
		"[0] ADDED   internal/auth/handler.go (+10/-0)\n" +
		"[1] MODIFIED   internal/auth/handler_test.go (+5/-2)\n" +
		"[2] RENAMED   cmd/app/main.go (+2/-1)\n" +
		"[3] DELETED   docs/old notes.md (+0/-7)\n\n" +
		"Respond with a JSON array"
	msgs := []any{
		map[string]any{"role": "system", "content": "You are a file grouping assistant."},
		map[string]any{"role": "user", "content": content},
	}
	index := buildGroupingIndex(msgs)
	want := map[int]string{
		0: "internal/auth/handler.go",
		1: "internal/auth/handler_test.go",
		2: "cmd/app/main.go",
		3: "docs/old notes.md",
	}
	if len(index) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(index), len(want), index)
	}
	for k, v := range want {
		if index[k] != v {
			t.Errorf("index[%d] = %q, want %q", k, index[k], v)
		}
	}

	if buildGroupingIndex(nil) != nil {
		t.Error("nil messages should yield nil index")
	}
	if buildGroupingIndex([]any{map[string]any{"role": "user", "content": "no file list here"}}) != nil {
		t.Error("content without a file list should yield nil index")
	}
}

func TestBuildGroupingIndex_IgnoresSystemMessage(t *testing.T) {
	// Only the user message's list may seed the map. A worked example living in
	// the system prompt (which users can reword onto its own line) must not
	// pollute the index — otherwise an out-of-range index would render as a bogus
	// path instead of "#idx".
	msgs := []any{
		map[string]any{"role": "system", "content": "e.g.\n[0] MODIFIED   bogus/from-prompt.go (+1/-1)\n"},
		map[string]any{"role": "user", "content": "[0] MODIFIED   real/file.go (+2/-1)\n"},
	}
	index := buildGroupingIndex(msgs)
	if index[0] != "real/file.go" {
		t.Errorf("index[0] = %q, want the user message's path (system example must be ignored)", index[0])
	}
}

func TestParseGroupingGroups(t *testing.T) {
	t.Run("plain index JSON", func(t *testing.T) {
		groups, ok := parseGroupingGroups(`[{"label":"auth","files":[0,1]},{"label":"docs","files":[2]}]`)
		if !ok {
			t.Fatal("expected ok")
		}
		if len(groups) != 2 || groups[0].Label != "auth" || len(groups[0].Files) != 2 || groups[0].Files[1] != 1 {
			t.Errorf("unexpected parse: %+v", groups)
		}
	})
	t.Run("markdown fenced", func(t *testing.T) {
		groups, ok := parseGroupingGroups("```json\n" + `[{"label":"all","files":[0,1]}]` + "\n```")
		if !ok || len(groups) != 1 || len(groups[0].Files) != 2 {
			t.Errorf("fenced parse failed: ok=%v groups=%+v", ok, groups)
		}
	})
	t.Run("legacy path-string response is not index-shaped", func(t *testing.T) {
		// Sessions recorded before the index switch had "files" as path strings.
		if _, ok := parseGroupingGroups(`[{"label":"auth","files":["a.go","b.go"]}]`); ok {
			t.Error("path-string files should report ok=false so the viewer falls back to raw text")
		}
	})
	t.Run("garbage", func(t *testing.T) {
		if _, ok := parseGroupingGroups("not json at all"); ok {
			t.Error("non-JSON should report ok=false")
		}
	})
	t.Run("truncated response is not index-shaped", func(t *testing.T) {
		// One-shot Unmarshal of a cut-off array fails, so the viewer falls back to
		// the raw text — the same call the backend recorded.
		if _, ok := parseGroupingGroups(`[{"label":"g1","files":[0,1]},{"label":"g2","fil`); ok {
			t.Error("a truncated response should report ok=false")
		}
	})
}

func TestGroupingView(t *testing.T) {
	req := []any{map[string]any{"role": "user", "content": "" +
		"[0] MODIFIED   a.go (+1/-1)\n" +
		"[1] MODIFIED   b.go (+2/-0)\n"}}

	t.Run("resolves indices to paths, flags out-of-range", func(t *testing.T) {
		card := &TaskCard{
			RequestMessages: req,
			ResponseContent: `[{"label":"g","files":[0,1,9]}]`,
		}
		views := groupingView(card)
		if len(views) != 1 || len(views[0].Files) != 3 {
			t.Fatalf("unexpected views: %+v", views)
		}
		if !views[0].Files[0].Resolved || views[0].Files[0].Path != "a.go" {
			t.Errorf("file 0 = %+v, want resolved a.go", views[0].Files[0])
		}
		if views[0].Files[2].Resolved {
			t.Errorf("index 9 should be unresolved, got %+v", views[0].Files[2])
		}
	})
	t.Run("nil when request list missing", func(t *testing.T) {
		card := &TaskCard{ResponseContent: `[{"label":"g","files":[0]}]`}
		if groupingView(card) != nil {
			t.Error("missing request list should yield nil (fall back to raw)")
		}
	})
	t.Run("nil when response not index-shaped", func(t *testing.T) {
		card := &TaskCard{RequestMessages: req, ResponseContent: `[{"label":"g","files":["a.go"]}]`}
		if groupingView(card) != nil {
			t.Error("legacy path response should yield nil (fall back to raw)")
		}
	})
	t.Run("nil card", func(t *testing.T) {
		if groupingView(nil) != nil {
			t.Error("nil card should yield nil")
		}
	})
	t.Run("nil when no referenced index resolves (format drift)", func(t *testing.T) {
		// Request list parses (index != nil) but every index the response cites is
		// out of range: the list shape drifted from what the response indexes into,
		// so a wall of "#idx" would mislead — fall back to raw instead.
		card := &TaskCard{RequestMessages: req, ResponseContent: `[{"label":"g","files":[8,9]}]`}
		if groupingView(card) != nil {
			t.Error("all-unresolved indices should yield nil (fall back to raw)")
		}
	})
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

package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

// --- SessionFilePath ---

func TestSessionFilePath_EmptyID(t *testing.T) {
	_, err := SessionFilePath("/some/repo", "")
	if err == nil {
		t.Fatal("expected error for empty session ID")
	}
}

func TestSessionFilePath_ValidID(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	path, err := SessionFilePath("/some/repo", "abc-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedSuffix := filepath.Join("test-sessions", encodeRepoPath("/some/repo"), "abc-123.jsonl")
	if !strings.Contains(path, expectedSuffix) {
		t.Errorf("path %q does not contain expected suffix %q", path, expectedSuffix)
	}
}

// --- ResumeState.CompletedCount ---

func TestCompletedCount_NilState(t *testing.T) {
	var s *ResumeState
	if got := s.CompletedCount(); got != 0 {
		t.Errorf("CompletedCount on nil = %d, want 0", got)
	}
}

func TestCompletedCount_EmptyItems(t *testing.T) {
	s := &ResumeState{Items: make(map[string]ResumeItem)}
	if got := s.CompletedCount(); got != 0 {
		t.Errorf("CompletedCount on empty = %d, want 0", got)
	}
}

func TestCompletedCount_WithItems(t *testing.T) {
	s := &ResumeState{Items: map[string]ResumeItem{
		"fp1": {FilePath: "a.go"},
		"fp2": {FilePath: "b.go"},
	}}
	if got := s.CompletedCount(); got != 2 {
		t.Errorf("CompletedCount = %d, want 2", got)
	}
}

// --- ResumeState.Item ---

func TestItem_NilState(t *testing.T) {
	var s *ResumeState
	_, ok := s.Item("fp1")
	if ok {
		t.Error("Item on nil state should return false")
	}
}

func TestItem_Missing(t *testing.T) {
	s := &ResumeState{Items: make(map[string]ResumeItem)}
	_, ok := s.Item("nonexistent")
	if ok {
		t.Error("Item should return false for missing key")
	}
}

func TestItem_Found(t *testing.T) {
	comments := []model.LlmComment{{Path: "x.go", Content: "issue"}}
	s := &ResumeState{Items: map[string]ResumeItem{
		"fp1": {FilePath: "x.go", Fingerprint: "fp1", Comments: comments},
	}}
	item, ok := s.Item("fp1")
	if !ok {
		t.Fatal("expected Item to be found")
	}
	if item.FilePath != "x.go" {
		t.Errorf("FilePath = %q, want x.go", item.FilePath)
	}
	if len(item.Comments) != 1 || item.Comments[0].Content != "issue" {
		t.Errorf("Comments mismatch: %+v", item.Comments)
	}
}

func TestItem_ReturnsCopy(t *testing.T) {
	original := []model.LlmComment{{Content: "original"}}
	s := &ResumeState{Items: map[string]ResumeItem{
		"fp1": {Comments: original},
	}}
	item, _ := s.Item("fp1")
	item.Comments[0].Content = "mutated"

	// The original should not be affected.
	stored := s.Items["fp1"]
	if stored.Comments[0].Content != "original" {
		t.Error("Item should return a defensive copy of comments")
	}
}

// --- ValidateOptions ---

func TestValidateOptions_NilState(t *testing.T) {
	var s *ResumeState
	err := s.ValidateOptions(SessionOptions{ReviewMode: ReviewModeRange})
	if err != nil {
		t.Errorf("nil state should return nil error, got: %v", err)
	}
}

func TestValidateOptions_RejectsWorkspaceMode(t *testing.T) {
	s := &ResumeState{ReviewMode: ReviewModeRange}
	err := s.ValidateOptions(SessionOptions{ReviewMode: ReviewModeWorkspace})
	if err == nil {
		t.Fatal("expected error for workspace mode")
	}
}

func TestValidateOptions_RejectsEmptyMode(t *testing.T) {
	s := &ResumeState{ReviewMode: ReviewModeRange}
	err := s.ValidateOptions(SessionOptions{ReviewMode: ""})
	if err == nil {
		t.Fatal("expected error for empty review mode")
	}
}

func TestValidateOptions_RejectsMissingStateMode(t *testing.T) {
	s := &ResumeState{SessionID: "s1", ReviewMode: ""}
	err := s.ValidateOptions(SessionOptions{ReviewMode: ReviewModeRange})
	if err == nil {
		t.Fatal("expected error when state has no review mode")
	}
}

func TestValidateOptions_RejectsModeMismatch(t *testing.T) {
	s := &ResumeState{ReviewMode: ReviewModeRange}
	err := s.ValidateOptions(SessionOptions{ReviewMode: ReviewModeCommit})
	if err == nil {
		t.Fatal("expected error for mode mismatch")
	}
}

func TestValidateOptions_RangeMatches(t *testing.T) {
	s := &ResumeState{
		ReviewMode: ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "dev",
	}
	err := s.ValidateOptions(SessionOptions{
		ReviewMode: ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "dev",
	})
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestValidateOptions_RangeMismatch(t *testing.T) {
	s := &ResumeState{
		ReviewMode: ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature-a",
	}
	err := s.ValidateOptions(SessionOptions{
		ReviewMode: ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature-b",
	})
	if err == nil {
		t.Fatal("expected error for range mismatch")
	}
}

func TestValidateOptions_CommitMatches(t *testing.T) {
	s := &ResumeState{
		ReviewMode: ReviewModeCommit,
		DiffCommit: "abc123",
	}
	err := s.ValidateOptions(SessionOptions{
		ReviewMode: ReviewModeCommit,
		DiffCommit: "abc123",
	})
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestValidateOptions_CommitMismatch(t *testing.T) {
	s := &ResumeState{
		ReviewMode: ReviewModeCommit,
		DiffCommit: "abc123",
	}
	err := s.ValidateOptions(SessionOptions{
		ReviewMode: ReviewModeCommit,
		DiffCommit: "def456",
	})
	if err == nil {
		t.Fatal("expected error for commit mismatch")
	}
}

func TestValidateOptions_UnsupportedMode(t *testing.T) {
	s := &ResumeState{ReviewMode: "unknown_mode"}
	err := s.ValidateOptions(SessionOptions{ReviewMode: "unknown_mode"})
	if err == nil {
		t.Fatal("expected error for unsupported review mode")
	}
}

// --- applyResumeLine ---

func TestApplyResumeLine_SessionStart(t *testing.T) {
	s := &ResumeState{Items: make(map[string]ResumeItem)}
	line := mustJSON(t, resumeRecord{
		Type:       "session_start",
		SessionID:  "sess-1",
		Cwd:        "/repo",
		GitBranch:  "main",
		Model:      "gpt-4",
		ReviewMode: ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
	})
	if err := s.applyResumeLine(line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SessionID != "sess-1" {
		t.Errorf("SessionID = %q", s.SessionID)
	}
	if s.RepoDir != "/repo" {
		t.Errorf("RepoDir = %q", s.RepoDir)
	}
	if s.ReviewMode != ReviewModeRange {
		t.Errorf("ReviewMode = %q", s.ReviewMode)
	}
}

func TestApplyResumeLine_ReviewItemDone(t *testing.T) {
	s := &ResumeState{Items: make(map[string]ResumeItem)}
	line := mustJSON(t, resumeRecord{
		Type:        "review_item_done",
		FilePath:    "handler.go",
		OldPath:     "handler.go",
		NewPath:     "handler.go",
		Fingerprint: "fp-handler",
		Comments:    []model.LlmComment{{Content: "potential nil deref"}},
	})
	if err := s.applyResumeLine(line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.CompletedCount() != 1 {
		t.Fatalf("CompletedCount = %d, want 1", s.CompletedCount())
	}
	item, ok := s.Item("fp-handler")
	if !ok {
		t.Fatal("missing fp-handler")
	}
	if item.FilePath != "handler.go" {
		t.Errorf("FilePath = %q", item.FilePath)
	}
}

func TestApplyResumeLine_ReviewItemDone_FallbackToNewPath(t *testing.T) {
	s := &ResumeState{Items: make(map[string]ResumeItem)}
	line := mustJSON(t, resumeRecord{
		Type:        "review_item_done",
		NewPath:     "renamed.go",
		Fingerprint: "fp-renamed",
	})
	if err := s.applyResumeLine(line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	item, ok := s.Item("fp-renamed")
	if !ok {
		t.Fatal("missing fp-renamed")
	}
	if item.FilePath != "renamed.go" {
		t.Errorf("FilePath = %q, want renamed.go (fallback to NewPath)", item.FilePath)
	}
}

func TestApplyResumeLine_ReviewItemDone_EmptyFingerprint(t *testing.T) {
	s := &ResumeState{Items: make(map[string]ResumeItem)}
	line := mustJSON(t, resumeRecord{
		Type:     "review_item_done",
		FilePath: "skip.go",
	})
	if err := s.applyResumeLine(line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.CompletedCount() != 0 {
		t.Error("items with empty fingerprint should be skipped")
	}
}

func TestApplyResumeLine_ReviewItemReused(t *testing.T) {
	s := &ResumeState{Items: make(map[string]ResumeItem)}
	line := mustJSON(t, resumeRecord{
		Type:        "review_item_reused",
		FilePath:    "reused.go",
		Fingerprint: "fp-reused",
	})
	if err := s.applyResumeLine(line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := s.Item("fp-reused"); !ok {
		t.Error("review_item_reused should be tracked in Items")
	}
}

func TestApplyResumeLine_ReviewItemFailed(t *testing.T) {
	s := &ResumeState{Items: map[string]ResumeItem{
		"fp-fail": {FilePath: "will-fail.go", Fingerprint: "fp-fail"},
	}}
	line := mustJSON(t, resumeRecord{
		Type:        "review_item_failed",
		Fingerprint: "fp-fail",
	})
	if err := s.applyResumeLine(line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := s.Item("fp-fail"); ok {
		t.Error("failed item should be removed from Items")
	}
}

func TestApplyResumeLine_ReviewItemFailed_EmptyFingerprint(t *testing.T) {
	s := &ResumeState{Items: map[string]ResumeItem{
		"fp-keep": {FilePath: "keep.go"},
	}}
	line := mustJSON(t, resumeRecord{
		Type: "review_item_failed",
	})
	if err := s.applyResumeLine(line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should not affect existing items when fingerprint is empty.
	if s.CompletedCount() != 1 {
		t.Error("empty fingerprint failure should not affect existing items")
	}
}

func TestApplyResumeLine_UnknownType(t *testing.T) {
	s := &ResumeState{Items: make(map[string]ResumeItem)}
	line := mustJSON(t, resumeRecord{Type: "session_end"})
	if err := s.applyResumeLine(line); err != nil {
		t.Fatalf("unknown record types should be silently ignored, got: %v", err)
	}
	if s.CompletedCount() != 0 {
		t.Error("unknown type should not add items")
	}
}

func TestApplyResumeLine_InvalidJSON(t *testing.T) {
	s := &ResumeState{Items: make(map[string]ResumeItem)}
	err := s.applyResumeLine([]byte(`{invalid json}`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- applySessionStart ---

func TestApplySessionStart_PreservesRepoDirWhenCwdEmpty(t *testing.T) {
	s := &ResumeState{RepoDir: "/original"}
	s.applySessionStart(resumeRecord{SessionID: "s1", ReviewMode: ReviewModeCommit, DiffCommit: "abc"})
	if s.RepoDir != "/original" {
		t.Errorf("RepoDir = %q, want /original (preserved when Cwd is empty)", s.RepoDir)
	}
}

func TestApplySessionStart_OverridesRepoDir(t *testing.T) {
	s := &ResumeState{RepoDir: "/original"}
	s.applySessionStart(resumeRecord{SessionID: "s1", Cwd: "/new/repo"})
	if s.RepoDir != "/new/repo" {
		t.Errorf("RepoDir = %q, want /new/repo", s.RepoDir)
	}
}

func TestApplySessionStart_PreservesSessionIDWhenEmpty(t *testing.T) {
	s := &ResumeState{SessionID: "existing"}
	s.applySessionStart(resumeRecord{})
	if s.SessionID != "existing" {
		t.Errorf("SessionID = %q, want existing", s.SessionID)
	}
}

func TestApplySessionStart_SetsAllFields(t *testing.T) {
	s := &ResumeState{Items: make(map[string]ResumeItem)}
	s.applySessionStart(resumeRecord{
		SessionID:  "s1",
		Cwd:        "/repo",
		GitBranch:  "dev",
		Model:      "claude-3",
		ReviewMode: ReviewModeCommit,
		DiffFrom:   "main",
		DiffTo:     "dev",
		DiffCommit: "abc",
	})
	if s.GitBranch != "dev" {
		t.Errorf("GitBranch = %q", s.GitBranch)
	}
	if s.Model != "claude-3" {
		t.Errorf("Model = %q", s.Model)
	}
	if s.DiffCommit != "abc" {
		t.Errorf("DiffCommit = %q", s.DiffCommit)
	}
}

// --- copyLlmComments ---

func TestCopyLlmComments_Nil(t *testing.T) {
	if got := copyLlmComments(nil); got != nil {
		t.Errorf("copyLlmComments(nil) = %v, want nil", got)
	}
}

func TestCopyLlmComments_Empty(t *testing.T) {
	if got := copyLlmComments([]model.LlmComment{}); got != nil {
		t.Errorf("copyLlmComments([]) = %v, want nil", got)
	}
}

func TestCopyLlmComments_DeepCopy(t *testing.T) {
	original := []model.LlmComment{
		{Path: "a.go", Content: "fix", StartLine: 10, EndLine: 12},
		{Path: "b.go", Content: "refactor", Category: "maintainability"},
	}
	copied := copyLlmComments(original)

	if len(copied) != 2 {
		t.Fatalf("len(copied) = %d, want 2", len(copied))
	}
	if copied[0].Content != "fix" || copied[1].Category != "maintainability" {
		t.Errorf("copied content mismatch")
	}

	// Mutating copy should not affect original.
	copied[0].Content = "mutated"
	if original[0].Content != "fix" {
		t.Error("mutation of copy affected original")
	}
}

// --- LoadResumeState ---

func TestLoadResumeState_NonexistentFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	_, err := LoadResumeState("/some/repo", "nonexistent-session")
	if err == nil {
		t.Fatal("expected error for nonexistent session file")
	}
}

func TestLoadResumeState_EmptyFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	repoDir := "/test/repo"
	sessionID := "empty-session"
	path, err := SessionFilePath(repoDir, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	state, err := LoadResumeState(repoDir, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.CompletedCount() != 0 {
		t.Errorf("CompletedCount = %d, want 0", state.CompletedCount())
	}
}

func TestLoadResumeState_MultipleRecords(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	repoDir := "/test/multi"
	sessionID := "multi-session"
	path, err := SessionFilePath(repoDir, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}

	records := []resumeRecord{
		{
			Type:       "session_start",
			SessionID:  sessionID,
			Cwd:        repoDir,
			ReviewMode: ReviewModeRange,
			DiffFrom:   "main",
			DiffTo:     "feature",
		},
		{
			Type:        "review_item_done",
			FilePath:    "a.go",
			Fingerprint: "fp-a",
			Comments:    []model.LlmComment{{Content: "comment-a"}},
		},
		{
			Type:        "review_item_done",
			FilePath:    "b.go",
			Fingerprint: "fp-b",
			Comments:    []model.LlmComment{{Content: "comment-b"}},
		},
		{
			Type:        "review_item_failed",
			FilePath:    "c.go",
			Fingerprint: "fp-c",
			Error:       "timeout",
		},
		{Type: "session_end"},
	}

	var content []byte
	for _, rec := range records {
		b, _ := json.Marshal(rec)
		content = append(content, b...)
		content = append(content, '\n')
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}

	state, err := LoadResumeState(repoDir, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if state.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", state.SessionID, sessionID)
	}
	if state.ReviewMode != ReviewModeRange {
		t.Errorf("ReviewMode = %q, want range", state.ReviewMode)
	}
	if state.CompletedCount() != 2 {
		t.Errorf("CompletedCount = %d, want 2 (a.go and b.go)", state.CompletedCount())
	}
	if _, ok := state.Item("fp-c"); ok {
		t.Error("failed item fp-c should not be present")
	}
}

func TestLoadResumeState_InvalidJSON(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	repoDir := "/test/invalid"
	sessionID := "bad-json"
	path, err := SessionFilePath(repoDir, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{bad json}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err = LoadResumeState(repoDir, sessionID)
	if err == nil {
		t.Fatal("expected error for invalid JSON content")
	}
}

func TestLoadResumeState_FailThenRedone(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	repoDir := "/test/redo"
	sessionID := "redo-session"
	path, err := SessionFilePath(repoDir, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}

	// Simulate: item first completes, then fails (should be removed),
	// then completes again (should be re-added).
	records := []resumeRecord{
		{Type: "session_start", SessionID: sessionID, ReviewMode: ReviewModeCommit, DiffCommit: "abc"},
		{Type: "review_item_done", FilePath: "x.go", Fingerprint: "fp-x", Comments: []model.LlmComment{{Content: "first"}}},
		{Type: "review_item_failed", Fingerprint: "fp-x"},
		{Type: "review_item_done", FilePath: "x.go", Fingerprint: "fp-x", Comments: []model.LlmComment{{Content: "second"}}},
	}
	var content []byte
	for _, rec := range records {
		b, _ := json.Marshal(rec)
		content = append(content, b...)
		content = append(content, '\n')
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}

	state, err := LoadResumeState(repoDir, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	item, ok := state.Item("fp-x")
	if !ok {
		t.Fatal("fp-x should be present after re-completion")
	}
	if len(item.Comments) != 1 || item.Comments[0].Content != "second" {
		t.Errorf("expected latest comments, got: %+v", item.Comments)
	}
}

// --- helpers ---

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestApplySessionStart_SetsScanPathScope(t *testing.T) {
	paths := []string{"./internal/scan/", "cmd/opencodereview", "internal/scan"}
	s := &ResumeState{Items: make(map[string]ResumeItem)}
	s.applySessionStart(resumeRecord{
		SessionID:  "s1",
		ReviewMode: ReviewModeFullScan,
		ScanPaths:  &paths,
	})
	want := []string{"cmd/opencodereview", "internal/scan"}
	if !s.HasScanPathScope {
		t.Fatal("HasScanPathScope = false, want true")
	}
	if len(s.ScanPaths) != len(want) {
		t.Fatalf("ScanPaths = %v, want %v", s.ScanPaths, want)
	}
	for i := range want {
		if s.ScanPaths[i] != want[i] {
			t.Fatalf("ScanPaths = %v, want %v", s.ScanPaths, want)
		}
	}
}

func TestValidateScanOptions_AllowsLegacySessionWithoutPathScope(t *testing.T) {
	s := &ResumeState{SessionID: "s1", ReviewMode: ReviewModeFullScan}
	if err := s.ValidateScanOptions([]string{"internal/scan"}); err != nil {
		t.Fatalf("ValidateScanOptions: %v", err)
	}
}

func TestValidateScanOptions_ScanPathScopeMatchesNormalized(t *testing.T) {
	s := &ResumeState{
		SessionID:        "s1",
		ReviewMode:       ReviewModeFullScan,
		ScanPaths:        []string{"cmd/opencodereview", "internal/scan"},
		HasScanPathScope: true,
	}
	if err := s.ValidateScanOptions([]string{"./internal/scan/", "cmd/opencodereview"}); err != nil {
		t.Fatalf("ValidateScanOptions: %v", err)
	}
}

func TestValidateScanOptions_RejectsScanPathScopeMismatch(t *testing.T) {
	s := &ResumeState{
		SessionID:        "s1",
		ReviewMode:       ReviewModeFullScan,
		ScanPaths:        []string{"internal/agent"},
		HasScanPathScope: true,
	}
	err := s.ValidateScanOptions(nil)
	if err == nil {
		t.Fatal("expected mismatch when prior scoped scan is resumed as whole repo")
	}
	if !contains(err.Error(), "scan path scope") {
		t.Fatalf("error = %q, want scan path scope mismatch", err.Error())
	}
}

func TestValidateScanOptions_RejectsWholeRepoScopeMismatch(t *testing.T) {
	s := &ResumeState{
		SessionID:        "s1",
		ReviewMode:       ReviewModeFullScan,
		HasScanPathScope: true,
	}
	err := s.ValidateScanOptions([]string{"internal/agent"})
	if err == nil {
		t.Fatal("expected mismatch when prior whole-repo scan is resumed with --path")
	}
	if !contains(err.Error(), "<whole repo>") {
		t.Fatalf("error = %q, want whole-repo scope in message", err.Error())
	}
}

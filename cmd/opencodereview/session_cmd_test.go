package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/session"
)

func TestRunSessionList_TextIncludesSessionID(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{{Path: "a.go", Content: "note"}})
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionList([]string{"--repo", repoDir}); err != nil {
			t.Fatalf("runSessionList: %v", err)
		}
	})

	if !strings.Contains(got, sh.SessionID) {
		t.Errorf("expected list output to contain session id %s, got %q", sh.SessionID, got)
	}
	if !strings.Contains(got, "abc123") {
		t.Errorf("expected list output to contain commit range, got %q", got)
	}
	if !strings.Contains(got, "SESSION ID") {
		t.Errorf("expected header, got %q", got)
	}
}

func TestRunSessionList_JSON(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", nil)
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionList([]string{"--repo", repoDir, "--json"}); err != nil {
			t.Fatalf("runSessionList: %v", err)
		}
	})

	var decoded []session.Summary
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, got)
	}
	if len(decoded) != 1 || decoded[0].SessionID != sh.SessionID {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestRunSessionList_EmptyRepo(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	got := captureStdout(t, func() {
		if err := runSessionList([]string{"--repo", repoDir}); err != nil {
			t.Fatalf("runSessionList: %v", err)
		}
	})
	if !strings.Contains(got, "No sessions found") {
		t.Errorf("expected empty message, got %q", got)
	}
}

func TestRunSessionShow_Text(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{{Path: "a.go", Content: "note"}})
	sh.RecordReviewItemFailed("bad.go", "bad.go", "bad.go", "fp-bad", "boom")
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionShow([]string{"--repo", repoDir, sh.SessionID}); err != nil {
			t.Fatalf("runSessionShow: %v", err)
		}
	})

	for _, want := range []string{sh.SessionID, "abc123", "a.go", "bad.go", "boom", "Files:"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, got %q", want, got)
		}
	}
}

func TestRunSessionShow_JSON(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", nil)
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionShow([]string{"--repo", repoDir, "--json", sh.SessionID}); err != nil {
			t.Fatalf("runSessionShow: %v", err)
		}
	})

	var payload struct {
		Summary *session.Summary     `json:"summary"`
		Items   []session.ItemDetail `json:"items"`
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, got)
	}
	if payload.Summary == nil || payload.Summary.SessionID != sh.SessionID {
		t.Fatalf("summary mismatch: %+v", payload.Summary)
	}
	if len(payload.Items) != 1 || payload.Items[0].FilePath != "a.go" {
		t.Fatalf("items = %+v", payload.Items)
	}
}

func TestRunSessionShow_MissingID(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	got := captureStdout(t, func() {
		if err := runSessionShow([]string{}); err == nil {
			t.Fatal("expected error for missing session id")
		}
	})
	if !strings.Contains(got, "session show") {
		t.Errorf("expected usage output, got %q", got)
	}
}

func TestRunSessionComments_Text(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{
		{Path: "a.go", Content: "this is a finding", Category: "bug", Severity: "high", StartLine: 10, EndLine: 12},
	})
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionComments([]string{"--repo", repoDir, sh.SessionID}); err != nil {
			t.Fatalf("runSessionComments: %v", err)
		}
	})

	// Output parity with ocr review: content, [category · severity] badge, file path:line header.
	for _, want := range []string{"this is a finding", "[bug · high]", "a.go:10-12"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, got %q", want, got)
		}
	}
}

func TestRunSessionComments_JSON(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{
		{Path: "a.go", Content: "json finding", Category: "bug", Severity: "high"},
	})
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionComments([]string{"--repo", repoDir, "--json", sh.SessionID}); err != nil {
			t.Fatalf("runSessionComments: %v", err)
		}
	})

	// Element schema is []model.LlmComment (same as live review jsonOutput.Comments).
	var payload struct {
		SessionID string             `json:"session_id"`
		Comments  []model.LlmComment `json:"comments"`
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, got)
	}
	if payload.SessionID != sh.SessionID {
		t.Errorf("session_id = %q, want %q", payload.SessionID, sh.SessionID)
	}
	if len(payload.Comments) != 1 || payload.Comments[0].Content != "json finding" {
		t.Fatalf("comments = %+v", payload.Comments)
	}
}

func TestRunSessionComments_SeverityFilter(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{
		{Path: "a.go", Content: "keep me", Severity: "high"},
		{Path: "a.go", Content: "drop me", Severity: "low"},
	})
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionComments([]string{"--repo", repoDir, "--severity", "high", sh.SessionID}); err != nil {
			t.Fatalf("runSessionComments: %v", err)
		}
	})

	if !strings.Contains(got, "keep me") {
		t.Errorf("expected filtered output to keep high-severity comment, got %q", got)
	}
	if strings.Contains(got, "drop me") {
		t.Errorf("expected low-severity comment to be filtered out, got %q", got)
	}
}

func TestRunSessionComments_CategoryFilter(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{
		{Path: "a.go", Content: "a bug", Category: "bug", Severity: "high"},
		{Path: "a.go", Content: "a nit", Category: "style", Severity: "low"},
	})
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionComments([]string{"--repo", repoDir, "--category", "bug", sh.SessionID}); err != nil {
			t.Fatalf("runSessionComments: %v", err)
		}
	})

	if !strings.Contains(got, "a bug") {
		t.Errorf("expected bug-category comment kept, got %q", got)
	}
	if strings.Contains(got, "a nit") {
		t.Errorf("expected style-category comment filtered out, got %q", got)
	}
}

func TestRunSessionComments_Empty(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	// Failed-only session: no comment-bearing records.
	sh.RecordReviewItemFailed("c.go", "c.go", "c.go", "fp-c", "boom")
	sh.Finalize()

	// Text path: explicit "no comments" message, never silent.
	textOut := captureStdout(t, func() {
		if err := runSessionComments([]string{"--repo", repoDir, sh.SessionID}); err != nil {
			t.Fatalf("runSessionComments (text): %v", err)
		}
	})
	if !strings.Contains(strings.ToLower(textOut), "no comments") {
		t.Errorf("expected explicit no-comments message, got %q", textOut)
	}

	// JSON path: envelope with "comments": [] even when empty.
	jsonOut := captureStdout(t, func() {
		if err := runSessionComments([]string{"--repo", repoDir, "--json", sh.SessionID}); err != nil {
			t.Fatalf("runSessionComments (json): %v", err)
		}
	})
	var payload struct {
		Comments []model.LlmComment `json:"comments"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, jsonOut)
	}
	if payload.Comments == nil || len(payload.Comments) != 0 {
		t.Errorf("expected comments: [] envelope, got %+v (raw %q)", payload.Comments, jsonOut)
	}
}

func TestRunSessionComments_FilteredToEmpty(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{
		{Path: "a.go", Content: "low sev", Severity: "low"},
	})
	sh.Finalize()

	// Filter excludes the only comment → must still be explicit, not silent.
	got := captureStdout(t, func() {
		if err := runSessionComments([]string{"--repo", repoDir, "--severity", "critical", sh.SessionID}); err != nil {
			t.Fatalf("runSessionComments: %v", err)
		}
	})
	if !strings.Contains(strings.ToLower(got), "no comments") {
		t.Errorf("filtered-to-empty must print explicit message, got %q", got)
	}
}

func TestRunSessionComments_MissingID(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	got := captureStdout(t, func() {
		if err := runSessionComments([]string{}); err == nil {
			t.Fatal("expected error for missing session id")
		}
	})
	if !strings.Contains(got, "session comments") {
		t.Errorf("expected usage output, got %q", got)
	}
}

func TestTruncateUnicode(t *testing.T) {
	got := truncate("错误原因：超过限制", 6)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix, got %q", got)
	}
	if !strings.Contains(got, "错误") {
		t.Fatalf("expected valid truncated unicode text, got %q", got)
	}
}

func TestRunSession_UnknownSubcommand(t *testing.T) {
	err := runSession([]string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown sub-command")
	}
}

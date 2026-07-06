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

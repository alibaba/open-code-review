// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestRunSessionList_TextIncludesSessionID(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{{Path: "a.go", Content: "note"}})
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionListCompat([]string{"--repo", repoDir}); err != nil {
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
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", nil)
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionListCompat([]string{"--repo", repoDir, "--json"}); err != nil {
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
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	got := captureStdout(t, func() {
		if err := runSessionListCompat([]string{"--repo", repoDir}); err != nil {
			t.Fatalf("runSessionList: %v", err)
		}
	})
	if !strings.Contains(got, "No sessions found") {
		t.Errorf("expected empty message, got %q", got)
	}
}

func TestRunSessionShow_Text(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{{Path: "a.go", Content: "note"}})
	sh.RecordReviewItemFailed("bad.go", "bad.go", "bad.go", "fp-bad", "boom")
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionShowCompat([]string{"--repo", repoDir, sh.SessionID}); err != nil {
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
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", nil)
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionShowCompat([]string{"--repo", repoDir, "--json", sh.SessionID}); err != nil {
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

func TestRunSessionComments_TextRendersLikeReview(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{
		{Path: "a.go", Content: "possible nil deref", StartLine: 3, EndLine: 5, Severity: "high", Category: "bug"},
	})
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionCommentsCompat([]string{"--repo", repoDir, sh.SessionID}); err != nil {
			t.Fatalf("runSessionComments: %v", err)
		}
	})

	for _, want := range []string{"a.go:3-5", "[bug · high]", "possible nil deref"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, got %q", want, got)
		}
	}
}

func TestRunSessionComments_SeverityFilter(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
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
		if err := runSessionCommentsCompat([]string{"--repo", repoDir, "--severity", "HIGH", sh.SessionID}); err != nil {
			t.Fatalf("runSessionComments: %v", err)
		}
	})
	if !strings.Contains(got, "keep me") || strings.Contains(got, "drop me") {
		t.Errorf("severity filter not applied, got %q", got)
	}

	got = captureStdout(t, func() {
		if err := runSessionCommentsCompat([]string{"--repo", repoDir, "--severity", "critical", sh.SessionID}); err != nil {
			t.Fatalf("runSessionComments: %v", err)
		}
	})
	if !strings.Contains(got, "No comments match the given filters") {
		t.Errorf("expected filter-miss message, got %q", got)
	}
}

func TestRunSessionComments_JSON(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{
		{Path: "a.go", Content: "note", Severity: "medium", Category: "style"},
	})
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionCommentsCompat([]string{"--repo", repoDir, "--json", sh.SessionID}); err != nil {
			t.Fatalf("runSessionComments: %v", err)
		}
	})

	var decoded []model.LlmComment
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, got)
	}
	if len(decoded) != 1 || decoded[0].Content != "note" || decoded[0].Severity != "medium" {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestRunSessionComments_JSONEmptyIsArray(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", nil)
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionCommentsCompat([]string{"--repo", repoDir, "--json", sh.SessionID}); err != nil {
			t.Fatalf("runSessionComments: %v", err)
		}
	})
	if strings.TrimSpace(got) != "[]" {
		t.Errorf("expected empty JSON array, got %q", got)
	}
}

func TestRunSessionComments_NoCommentsMessage(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", nil)
	sh.Finalize()

	got := captureStdout(t, func() {
		if err := runSessionCommentsCompat([]string{"--repo", repoDir, sh.SessionID}); err != nil {
			t.Fatalf("runSessionComments: %v", err)
		}
	})
	if !strings.Contains(got, "No comments recorded in session") {
		t.Errorf("expected no-comments message, got %q", got)
	}
}

func TestRunSessionShow_MissingID(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	err := runSessionShowCompat([]string{})
	if err == nil {
		t.Fatal("expected error for missing session id")
	}
}

func TestTruncateUnicode(t *testing.T) {
	got := truncate("错误原因：超过限制", 6) // allow-non-english: fixture exercises rune-boundary truncation
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix, got %q", got)
	}
	if !strings.Contains(got, "错误") { // allow-non-english: fixture exercises rune-boundary truncation
		t.Fatalf("expected valid truncated unicode text, got %q", got)
	}
}

// TestTruncate covers the remaining branches of truncate: newline/tab
// normalization, the short-enough pass-through, and the n<=1 ellipsis-only case.
func TestTruncate(t *testing.T) {
	cases := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{"shorter than limit is unchanged", "abc", 10, "abc"},
		{"newlines and tabs become spaces", "a\nb\tc", 10, "a b c"},
		{"n of one collapses to ellipsis", "abcdef", 1, "…"},
		{"n of zero collapses to ellipsis", "abcdef", 0, "…"},
		{"exact length is unchanged", "abcd", 4, "abcd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncate(tc.s, tc.n); got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.want)
			}
		})
	}
}

func TestRunSession_UnknownSubcommand(t *testing.T) {
	err := runSession([]string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown sub-command")
	}
}

func TestSessionDisplayUsesManifestStatusAndCoverage(t *testing.T) {
	summary := session.Summary{
		SessionID:      "run-1",
		SelectedFiles:  4,
		CompletedFiles: 1,
		ReusedFiles:    1,
		FailedFiles:    1,
		WaivedFiles:    1,
		RunManifest: &session.RunManifest{
			TerminalState: session.StatePartial,
		},
	}
	if got := describeStatus(summary); got != "partial" {
		t.Fatalf("status = %q", got)
	}
	if got := describeFiles(summary); !strings.Contains(got, "4") || !strings.Contains(got, "failed 1") || !strings.Contains(got, "waived 1") {
		t.Fatalf("files = %q", got)
	}
	got := captureStdout(t, func() { printSessionDetail(os.Stdout, &summary, nil) })
	if !strings.Contains(got, "4 selected = 1 completed + 1 reused + 1 failed + 1 waived") {
		t.Fatalf("detail = %q", got)
	}
}

func TestSessionDisplayUsesUnknownForInvalidManifestStatus(t *testing.T) {
	for _, state := range []session.TerminalState{"", "bogus"} {
		summary := session.Summary{
			RunManifest: &session.RunManifest{TerminalState: state},
		}
		if got := describeStatus(summary); got != "unknown" {
			t.Errorf("terminal state %q displayed as %q, want unknown", state, got)
		}
	}
}

func TestSessionDisplayDoesNotInferLegacyComplete(t *testing.T) {
	summary := session.Summary{CompletedFiles: 2, Legacy: true}
	if got := describeStatus(summary); got != "legacy" {
		t.Fatalf("status = %q", got)
	}
	summary.Aborted = true
	if got := describeStatus(summary); got != "aborted" {
		t.Fatalf("status = %q", got)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/spf13/cobra"
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

// newCompareSession records one session with the given findings and returns it.
func newCompareSession(t *testing.T, repoDir string, opts session.SessionOptions, comments []model.LlmComment) *session.SessionHistory {
	t.Helper()
	sh := session.New(repoDir, "main", "test-model", opts)
	byFile := map[string][]model.LlmComment{}
	var order []string
	for _, c := range comments {
		if _, seen := byFile[c.Path]; !seen {
			order = append(order, c.Path)
		}
		byFile[c.Path] = append(byFile[c.Path], c)
	}
	for _, path := range order {
		sh.RecordReviewItemDone(path, path, path, "fp-"+path, byFile[path])
	}
	return sh
}

// useCompareRepo points the compare command at repoDir and restores the shared
// command flags afterwards.
func useCompareRepo(t *testing.T, repoDir string, asJSON bool) {
	t.Helper()
	sessionCompareRepoDir = repoDir
	sessionCompareJSON = asJSON
	t.Cleanup(func() {
		sessionCompareRepoDir = ""
		sessionCompareJSON = false
	})
}

func TestRunSessionCompare_TextShowsCounts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	opts := session.SessionOptions{ReviewMode: session.ReviewModeCommit, DiffCommit: "abc123"}

	before := newCompareSession(t, repoDir, opts, []model.LlmComment{
		{Path: "a.go", StartLine: 40, EndLine: 40, Category: "bug", ExistingCode: "x := 1", Content: "still here"},
		{Path: "a.go", StartLine: 70, EndLine: 70, Category: "bug", ExistingCode: "y := 2", Content: "went away"},
	})
	before.Finalize()
	after := newCompareSession(t, repoDir, opts, []model.LlmComment{
		{Path: "a.go", StartLine: 52, EndLine: 52, Category: "bug", ExistingCode: "\tx := 1", Content: "still here"},
		{Path: "b.go", StartLine: 3, EndLine: 3, Category: "style", ExistingCode: "z := 3", Content: "brand new"},
	})
	after.Finalize()

	useCompareRepo(t, repoDir, false)
	got := captureStdout(t, func() {
		if err := runSessionCompare(before.SessionID, after.SessionID); err != nil {
			t.Fatalf("runSessionCompare: %v", err)
		}
	})

	for _, want := range []string{
		"1 new, 1 persisting, 1 resolved",
		"=== New (1) ===",
		"=== Persisting (1) ===",
		"=== Resolved (1) ===",
		"abc123",
		"brand new",
		"went away",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, got %q", want, got)
		}
	}
	if strings.Contains(got, "did not review") {
		t.Errorf("unexpected not-reviewed line for a legacy session pair: %q", got)
	}
}

func TestRunSessionCompare_JSONShape(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	opts := session.SessionOptions{ReviewMode: session.ReviewModeRange, DiffFrom: "main", DiffTo: "HEAD"}

	before := newCompareSession(t, repoDir, opts, []model.LlmComment{
		{Path: "a.go", StartLine: 40, EndLine: 40, Category: "bug", ExistingCode: "x := 1", Content: "still here"},
		{Path: "a.go", StartLine: 70, EndLine: 70, Category: "bug", ExistingCode: "y := 2", Content: "went away"},
	})
	before.Finalize()
	after := newCompareSession(t, repoDir, opts, []model.LlmComment{
		{Path: "a.go", StartLine: 52, EndLine: 52, Category: "bug", ExistingCode: "x := 1", Content: "still here"},
		{Path: "b.go", StartLine: 3, EndLine: 3, Category: "style", ExistingCode: "z := 3", Content: "brand new"},
	})
	after.Finalize()

	useCompareRepo(t, repoDir, true)
	out := captureStdout(t, func() {
		if err := runSessionCompare(before.SessionID, after.SessionID); err != nil {
			t.Fatalf("runSessionCompare: %v", err)
		}
	})

	var decoded struct {
		Before      sessionCompareSide `json:"before"`
		After       sessionCompareSide `json:"after"`
		New         []model.LlmComment `json:"new"`
		Persisting  []model.LlmComment `json:"persisting"`
		Resolved    []model.LlmComment `json:"resolved"`
		NotReviewed []model.LlmComment `json:"not_reviewed"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, out)
	}
	if decoded.Before.SessionID != before.SessionID || decoded.After.SessionID != after.SessionID {
		t.Fatalf("sides = %+v / %+v", decoded.Before, decoded.After)
	}
	if decoded.Before.Range != "main..HEAD" || decoded.After.ReviewMode != session.ReviewModeRange {
		t.Fatalf("range/mode = %+v / %+v", decoded.Before, decoded.After)
	}
	if len(decoded.New) != 1 || decoded.New[0].Path != "b.go" {
		t.Fatalf("new = %+v", decoded.New)
	}
	if len(decoded.Persisting) != 1 || decoded.Persisting[0].StartLine != 52 {
		t.Fatalf("persisting = %+v (want the after copy at line 52)", decoded.Persisting)
	}
	if len(decoded.Resolved) != 1 || decoded.Resolved[0].StartLine != 70 {
		t.Fatalf("resolved = %+v", decoded.Resolved)
	}
	if len(decoded.NotReviewed) != 0 {
		t.Fatalf("not_reviewed = %+v", decoded.NotReviewed)
	}
}

func TestRunSessionCompare_JSONEmptyBucketsAreArrays(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	opts := session.SessionOptions{ReviewMode: session.ReviewModeCommit, DiffCommit: "abc123"}

	before := newCompareSession(t, repoDir, opts, nil)
	before.Finalize()
	after := newCompareSession(t, repoDir, opts, nil)
	after.Finalize()

	useCompareRepo(t, repoDir, true)
	out := captureStdout(t, func() {
		if err := runSessionCompare(before.SessionID, after.SessionID); err != nil {
			t.Fatalf("runSessionCompare: %v", err)
		}
	})

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, out)
	}
	for _, bucket := range []string{"new", "persisting", "resolved", "not_reviewed"} {
		got, ok := raw[bucket]
		if !ok {
			t.Fatalf("missing bucket %q in %q", bucket, out)
		}
		if string(got) != "[]" {
			t.Errorf("%s = %s, want []", bucket, got)
		}
	}
}

func TestRunSessionCompare_CrossRepoRefuses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoA := t.TempDir()
	repoB := t.TempDir()
	opts := session.SessionOptions{ReviewMode: session.ReviewModeCommit, DiffCommit: "abc123"}

	local := newCompareSession(t, repoA, opts, nil)
	local.Finalize()
	foreign := newCompareSession(t, repoB, opts, nil)
	foreign.Finalize()

	// Simulate a session file copied in from another checkout: it lands in this
	// repo's session dir but still records the other repo as its cwd.
	dirA, err := session.SessionsDir(repoA)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}
	dirB, err := session.SessionsDir(repoB)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dirB, foreign.SessionID+".jsonl"))
	if err != nil {
		t.Fatalf("read foreign session: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirA, foreign.SessionID+".jsonl"), data, 0o600); err != nil {
		t.Fatalf("write foreign session: %v", err)
	}

	useCompareRepo(t, repoA, false)
	err = runSessionCompare(local.SessionID, foreign.SessionID)
	if err == nil {
		t.Fatal("expected an error comparing sessions from different repositories")
	}
	for _, want := range []string{"different repositories", repoA, repoB} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

func TestRunSessionCompare_ModeMismatchWarnsNotFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()

	before := newCompareSession(t, repoDir, session.SessionOptions{ReviewMode: session.ReviewModeCommit, DiffCommit: "abc123"}, nil)
	before.Finalize()
	after := newCompareSession(t, repoDir, session.SessionOptions{ReviewMode: session.ReviewModeRange, DiffFrom: "main", DiffTo: "HEAD"}, nil)
	after.Finalize()

	useCompareRepo(t, repoDir, true)
	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			if err := runSessionCompare(before.SessionID, after.SessionID); err != nil {
				t.Errorf("runSessionCompare: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "review modes differ") {
		t.Errorf("expected a stderr warning, got %q", stderr)
	}
	if strings.Contains(stdout, "review modes differ") {
		t.Errorf("the warning must not reach stdout, got %q", stdout)
	}
	if err := json.Unmarshal([]byte(stdout), &map[string]json.RawMessage{}); err != nil {
		t.Errorf("stdout is not valid JSON: %v (out=%q)", err, stdout)
	}
}

func TestRunSessionCompare_MalformedSessionID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	sh := newCompareSession(t, repoDir, session.SessionOptions{ReviewMode: session.ReviewModeCommit, DiffCommit: "abc123"}, nil)
	sh.Finalize()

	useCompareRepo(t, repoDir, false)
	tests := []struct {
		name          string
		before, after string
	}{
		{"empty before", "", sh.SessionID},
		{"empty after", sh.SessionID, ""},
		{"unknown before", "11111111-2222-3333-4444-555555555555", sh.SessionID},
		{"unknown after", sh.SessionID, "11111111-2222-3333-4444-555555555555"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := runSessionCompare(tt.before, tt.after); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestRunSessionCompare_SameSessionTwice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	sh := newCompareSession(t, repoDir, session.SessionOptions{ReviewMode: session.ReviewModeCommit, DiffCommit: "abc123"}, []model.LlmComment{
		{Path: "a.go", StartLine: 40, EndLine: 40, Category: "bug", ExistingCode: "x := 1", Content: "one"},
		{Path: "b.go", StartLine: 5, EndLine: 5, Category: "style", ExistingCode: "y := 2", Content: "two"},
	})
	sh.Finalize()

	useCompareRepo(t, repoDir, false)
	got := captureStdout(t, func() {
		if err := runSessionCompare(sh.SessionID, sh.SessionID); err != nil {
			t.Fatalf("runSessionCompare: %v", err)
		}
	})
	if !strings.Contains(got, "0 new, 2 persisting, 0 resolved") {
		t.Errorf("comparing a session with itself should be all persisting, got %q", got)
	}
}

func TestRunSessionCompare_UnreviewedFilesAreNotResolved(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	opts := session.SessionOptions{ReviewMode: session.ReviewModeCommit, DiffCommit: "abc123"}

	before := newCompareSession(t, repoDir, opts, []model.LlmComment{
		{Path: "a.go", StartLine: 40, EndLine: 40, Category: "bug", ExistingCode: "x := 1", Content: "in the reviewed file"},
		{Path: "untouched.go", StartLine: 9, EndLine: 9, Category: "bug", ExistingCode: "y := 2", Content: "in a file the rerun skipped"},
	})
	before.Finalize()

	after := newCompareSession(t, repoDir, opts, nil)
	after.SetFinalManifest(&session.RunManifest{
		SchemaVersion: session.ManifestSchemaVersion,
		RunID:         after.SessionID,
		Operation:     "review",
		TerminalState: session.StateComplete,
		Coverage: session.Coverage{
			Selected:  []session.CoverageItem{{ItemID: "i1", Path: "a.go"}},
			Completed: []session.CoverageItem{{ItemID: "i1", Path: "a.go"}},
		},
	})
	after.Finalize()

	useCompareRepo(t, repoDir, false)
	got := captureStdout(t, func() {
		if err := runSessionCompare(before.SessionID, after.SessionID); err != nil {
			t.Fatalf("runSessionCompare: %v", err)
		}
	})

	for _, want := range []string{
		"0 new, 0 persisting, 1 resolved",
		"1 finding(s) in files the after session did not review (not counted as resolved)",
		"=== Not reviewed (1) ===",
		"untouched.go",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, got %q", want, got)
		}
	}
}

// TestSessionCompareCmd_HelpRoutes mirrors the parent-command routing checks:
// `session compare --help` must resolve through the session command tree.
func TestSessionCompareCmd_HelpRoutes(t *testing.T) {
	for _, args := range [][]string{
		{"session", "compare", "--help"},
		{"session", "diff", "--help"},
	} {
		root := rootCmd
		root.SetArgs(args)
		t.Cleanup(func() { root.SetArgs(nil) })
		if err := root.Execute(); err != nil {
			t.Fatalf("help on %v: %v", args, err)
		}
	}
}

// TestSessionCompareCompletesBothPositionals pins that typing the first session
// id does not stop completion of the second.
func TestSessionCompareCompletesBothPositionals(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	sh := newCompareSession(t, repoDir, session.SessionOptions{ReviewMode: session.ReviewModeCommit, DiffCommit: "abc123"}, nil)
	sh.Finalize()

	if err := sessionCompareCmd.Flags().Set("repo", repoDir); err != nil {
		t.Fatalf("set --repo: %v", err)
	}
	t.Cleanup(func() {
		_ = sessionCompareCmd.Flags().Set("repo", "")
		sessionCompareRepoDir = ""
	})

	comps, directive := sessionCompareCmd.ValidArgsFunction(sessionCompareCmd, []string{"already-typed-id"}, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", directive)
	}
	found := false
	for _, c := range comps {
		if strings.HasPrefix(c, sh.SessionID+"\t") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %s among second-positional completions, got %v", sh.SessionID, comps)
	}

	if comps, _ := sessionCompareCmd.ValidArgsFunction(sessionCompareCmd, []string{"a", "b"}, ""); comps != nil {
		t.Errorf("expected no completions past the second positional, got %v", comps)
	}
}

// TestRunSessionCompare_UnfinishedAfterRunIsNotResolved pins that only the
// coverage partitions that carry a current verdict (completed, reused) count as
// reviewed. A file the after run selected but failed on was never re-judged, so
// its earlier findings must not be reported as fixed.
func TestRunSessionCompare_UnfinishedAfterRunIsNotResolved(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	opts := session.SessionOptions{ReviewMode: session.ReviewModeCommit, DiffCommit: "abc123"}

	before := newCompareSession(t, repoDir, opts, []model.LlmComment{
		{Path: "completed.go", StartLine: 40, EndLine: 40, Category: "bug", ExistingCode: "x := 1", Content: "re-checked and gone"},
		{Path: "failed.go", StartLine: 9, EndLine: 9, Category: "bug", ExistingCode: "y := 2", Content: "the rerun never judged this"},
		{Path: "reused.go", StartLine: 3, EndLine: 3, Category: "bug", ExistingCode: "z := 3", Content: "carried forward as clean"},
	})
	before.Finalize()

	after := newCompareSession(t, repoDir, opts, nil)
	after.SetFinalManifest(&session.RunManifest{
		SchemaVersion: session.ManifestSchemaVersion,
		RunID:         after.SessionID,
		Operation:     session.OperationReview,
		TerminalState: session.StateFailed,
		Coverage: session.Coverage{
			Selected: []session.CoverageItem{
				{ItemID: "i1", Path: "completed.go"},
				{ItemID: "i2", Path: "failed.go"},
				{ItemID: "i3", Path: "reused.go"},
			},
			Completed: []session.CoverageItem{{ItemID: "i1", Path: "completed.go"}},
			Failed:    []session.CoverageItem{{ItemID: "i2", Path: "failed.go", Classification: session.FailureTimeout}},
			Reused:    []session.CoverageItem{{ItemID: "i3", Path: "reused.go"}},
		},
	})
	after.Finalize()

	useCompareRepo(t, repoDir, true)
	out := captureStdout(t, func() {
		if err := runSessionCompare(before.SessionID, after.SessionID); err != nil {
			t.Fatalf("runSessionCompare: %v", err)
		}
	})

	var decoded struct {
		Resolved    []model.LlmComment `json:"resolved"`
		NotReviewed []model.LlmComment `json:"not_reviewed"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, out)
	}
	resolved := []string{}
	for _, c := range decoded.Resolved {
		resolved = append(resolved, c.Path)
	}
	if got, want := strings.Join(resolved, ","), "completed.go,reused.go"; got != want {
		t.Errorf("resolved = %q, want %q", got, want)
	}
	if len(decoded.NotReviewed) != 1 || decoded.NotReviewed[0].Path != "failed.go" {
		t.Errorf("not_reviewed = %+v, want only failed.go", decoded.NotReviewed)
	}
}

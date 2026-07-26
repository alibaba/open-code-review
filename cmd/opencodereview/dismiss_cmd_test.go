package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/session"
)

// dismissTestSetup isolates the per-repo dismissal store under a temp HOME and
// returns the repo dir + a session id with one recorded comment to dismiss.
func dismissTestSetup(t *testing.T, comments []model.LlmComment) (repoDir, sessionID string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	repoDir = t.TempDir()
	sh := session.New(repoDir, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", comments)
	sh.Finalize()
	return repoDir, sh.SessionID
}

func TestDismissAddListRemove(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "a.go", StartLine: 1, EndLine: 2, Content: "fix the bug"},
	}
	repoDir, sessionID := dismissTestSetup(t, comments)

	// add: record the dismissal via 0-based index.
	out := captureStdout(t, func() {
		if err := runDismiss([]string{"add", "--repo", repoDir, sessionID, "0"}); err != nil {
			t.Fatalf("dismiss add: %v", err)
		}
	})
	fp := session.DismissalFingerprint(comments[0])
	if !strings.Contains(out, fp) {
		t.Errorf("dismiss add output missing fingerprint %q:\n%s", fp, out)
	}

	// list: the dismissal appears with the short fingerprint.
	listOut := captureStdout(t, func() {
		if err := runDismiss([]string{"list", "--repo", repoDir}); err != nil {
			t.Fatalf("dismiss list: %v", err)
		}
	})
	if !strings.Contains(listOut, "INDEX") || !strings.Contains(listOut, "a.go") {
		t.Errorf("dismiss list output missing table/a.go:\n%s", listOut)
	}

	// remove by short fingerprint prefix (first 12 chars, as list shows).
	short := fp[:12]
	if err := runDismiss([]string{"remove", "--repo", repoDir, short}); err != nil {
		t.Fatalf("dismiss remove: %v", err)
	}

	// list is now empty.
	emptyOut := captureStdout(t, func() {
		if err := runDismiss([]string{"list", "--repo", repoDir}); err != nil {
			t.Fatalf("dismiss list (2): %v", err)
		}
	})
	if !strings.Contains(emptyOut, "No dismissed findings") {
		t.Errorf("expected empty-list message after remove, got:\n%s", emptyOut)
	}
}

func TestDismissAddByIdempotent(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "a.go", StartLine: 1, EndLine: 1, Content: "dup"},
	}
	repoDir, sessionID := dismissTestSetup(t, comments)
	for i := 0; i < 2; i++ {
		if err := runDismiss([]string{"add", "--repo", repoDir, sessionID, "0"}); err != nil {
			t.Fatalf("dismiss add #%d: %v", i+1, err)
		}
	}
	store, err := session.LoadDismissals(repoDir)
	if err != nil {
		t.Fatalf("LoadDismissals: %v", err)
	}
	if got := len(store.List()); got != 1 {
		t.Errorf("after re-add, store has %d entries, want 1 (idempotent)", got)
	}
}

func TestDismissAddPathLineRef(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "a.go", StartLine: 10, EndLine: 20, Content: "in range"},
	}
	repoDir, sessionID := dismissTestSetup(t, comments)
	// Resolve by path:line where line is within [10,20].
	if err := runDismiss([]string{"add", "--repo", repoDir, sessionID, "a.go:15"}); err != nil {
		t.Fatalf("dismiss add path:line: %v", err)
	}
	store, err := session.LoadDismissals(repoDir)
	if err != nil {
		t.Fatalf("LoadDismissals: %v", err)
	}
	if !store.Contains(session.DismissalFingerprint(comments[0])) {
		t.Error("path:line ref did not record the matching comment's fingerprint")
	}
}

func TestDismissAddPathLineAmbiguous(t *testing.T) {
	// Two overlapping comments qualify for a.go:15 -> disambiguation error.
	comments := []model.LlmComment{
		{Path: "a.go", StartLine: 10, EndLine: 20, Content: "first"},
		{Path: "a.go", StartLine: 12, EndLine: 18, Content: "second"},
	}
	repoDir, sessionID := dismissTestSetup(t, comments)
	err := runDismiss([]string{"add", "--repo", repoDir, sessionID, "a.go:15"})
	if err == nil {
		t.Fatal("expected disambiguation error for overlapping comments")
	}
	if !strings.Contains(err.Error(), "multiple findings") {
		t.Fatalf("expected 'multiple findings' error, got: %v", err)
	}
	// Nothing should have been recorded.
	store, err := session.LoadDismissals(repoDir)
	if err != nil {
		t.Fatalf("LoadDismissals: %v", err)
	}
	if got := len(store.List()); got != 0 {
		t.Errorf("ambiguous add recorded %d entries, want 0", got)
	}
}

func TestDismissAddIndexOutOfRange(t *testing.T) {
	comments := []model.LlmComment{{Path: "a.go", StartLine: 1, EndLine: 1, Content: "x"}}
	repoDir, sessionID := dismissTestSetup(t, comments)
	err := runDismiss([]string{"add", "--repo", repoDir, sessionID, "9"})
	if err == nil {
		t.Fatal("expected out-of-range error")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDismissAddCorruptStoreFails(t *testing.T) {
	// D6: a corrupt store must not be silently wiped by `dismiss add`.
	comments := []model.LlmComment{{Path: "a.go", StartLine: 1, EndLine: 1, Content: "x"}}
	repoDir, sessionID := dismissTestSetup(t, comments)
	path, err := session.DismissalFilePath(repoDir)
	if err != nil {
		t.Fatalf("DismissalFilePath: %v", err)
	}
	garbage := []byte("{ not json")
	if err := os.MkdirAll(dirOf(path), 0700); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, garbage, 0600); err != nil {
		t.Fatalf("write corrupt store: %v", err)
	}

	err = runDismiss([]string{"add", "--repo", repoDir, sessionID, "0"})
	if err == nil {
		t.Fatal("expected error when store is corrupt")
	}
	if !strings.Contains(err.Error(), "corrupt") && !strings.Contains(err.Error(), "left untouched") {
		t.Fatalf("expected corruption-related error, got: %v", err)
	}
	// The corrupt file is untouched.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !bytes.Equal(after, garbage) {
		t.Errorf("corrupt store was modified by dismiss add:\n before=%q\n after=%q", garbage, after)
	}
}

func TestDismissRemoveNotFound(t *testing.T) {
	comments := []model.LlmComment{{Path: "a.go", StartLine: 1, EndLine: 1, Content: "x"}}
	repoDir, _ := dismissTestSetup(t, comments)
	err := runDismiss([]string{"remove", "--repo", repoDir, "deadbeefdead"})
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if !strings.Contains(err.Error(), "no dismissed finding") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDismissListCorruptStoreWarnsAndShowsEmpty(t *testing.T) {
	// D6: `dismiss list` on a corrupt store warns and shows empty, does not crash.
	repoDir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	path, err := session.DismissalFilePath(repoDir)
	if err != nil {
		t.Fatalf("DismissalFilePath: %v", err)
	}
	if err := os.MkdirAll(dirOf(path), 0700); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, []byte("{ broken"), 0600); err != nil {
		t.Fatalf("write corrupt store: %v", err)
	}
	out := captureStdout(t, func() {
		if err := runDismiss([]string{"list", "--repo", repoDir}); err != nil {
			t.Fatalf("dismiss list on corrupt store errored: %v", err)
		}
	})
	if !strings.Contains(out, "No dismissed findings") {
		t.Errorf("expected empty-list message on corrupt store, got:\n%s", out)
	}
}

func TestRunDismissUsage(t *testing.T) {
	out := captureStdout(t, func() {
		if err := runDismiss(nil); err != nil {
			t.Fatalf("runDismiss: %v", err)
		}
	})
	if !strings.Contains(out, "add") || !strings.Contains(out, "list") || !strings.Contains(out, "remove") {
		t.Errorf("usage missing subcommands:\n%s", out)
	}
}

func TestRunDismissUnknownSubcommand(t *testing.T) {
	err := runDismiss([]string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown dismiss sub-command") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParsePathLine(t *testing.T) {
	tests := []struct {
		ref     string
		wantP   string
		wantL   int
		wantErr bool
	}{
		{"a.go:42", "a.go", 42, false},
		{"dir/a.go:1", "dir/a.go", 1, false},
		{"a.go", "", 0, true},
		{"a.go:", "", 0, true},
		{":42", "", 0, true},
		{"a.go:0", "", 0, true},
		{"a.go:abc", "", 0, true},
	}
	for _, tt := range tests {
		gotP, gotL, err := parsePathLine(tt.ref)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parsePathLine(%q): expected error, got none", tt.ref)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePathLine(%q): unexpected error: %v", tt.ref, err)
			continue
		}
		if gotP != tt.wantP || gotL != tt.wantL {
			t.Errorf("parsePathLine(%q) = (%q,%d), want (%q,%d)", tt.ref, gotP, gotL, tt.wantP, tt.wantL)
		}
	}
}

func TestTruncateForPreview(t *testing.T) {
	short := "hello world"
	if got := truncateForPreview(short); got != short {
		t.Errorf("truncate short = %q, want %q", got, short)
	}
	// Newlines/tabs collapse.
	if got := truncateForPreview("a\nb\tc"); got != "a b c" {
		t.Errorf("truncate whitespace = %q, want %q", got, "a b c")
	}
	// Long content truncated with ellipsis.
	long := strings.Repeat("x", 300)
	got := truncateForPreview(long)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncate long missing ellipsis: %q", got[len(got)-5:])
	}
	if len([]rune(got)) != 160 {
		t.Errorf("truncate long length = %d runes, want 160", len([]rune(got)))
	}
}

// dirOf returns the directory portion of a path (filepath.Dir wrapper kept
// local to avoid an extra import dance in the test body).
func dirOf(p string) string {
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return "."
	}
	return p[:i]
}

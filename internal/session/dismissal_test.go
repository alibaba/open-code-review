package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
)

// dismissTestRepo returns a unique repo dir for a test so concurrent -race
// tests never collide on the same encoded store path. UseTestSessions (called
// in persist_test.go init) redirects the dismissal subdir to test-dismissals.
func dismissTestRepo(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "myrepo")
}

// writeDismissalStore writes raw bytes to the dismissal store path for a repo.
func writeDismissalStore(t *testing.T, repoDir string, data []byte) {
	t.Helper()
	path, err := dismissalPath(repoDir)
	if err != nil {
		t.Fatalf("dismissalPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write store: %v", err)
	}
}

func readDismissalStore(t *testing.T, repoDir string) []byte {
	t.Helper()
	path, err := dismissalPath(repoDir)
	if err != nil {
		t.Fatalf("dismissalPath: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	return b
}

func TestDismissalFingerprintStable(t *testing.T) {
	c := model.LlmComment{Path: "main.go", StartLine: 10, EndLine: 12, Content: "Fix this"}
	fp1 := DismissalFingerprint(c)
	fp2 := DismissalFingerprint(c)
	if fp1 != fp2 {
		t.Fatalf("fingerprint not stable across calls: %q vs %q", fp1, fp2)
	}
	if len(fp1) != 64 {
		t.Fatalf("fingerprint is not 64 hex chars: %d (%q)", len(fp1), fp1)
	}

	// Whitespace-only differences collapse via TrimSpace (AS8).
	trimmed := DismissalFingerprint(model.LlmComment{Path: "main.go", StartLine: 10, EndLine: 12, Content: "  \n\tFix this  \r\n"})
	if trimmed != fp1 {
		t.Errorf("TrimSpace normalization failed: trim=%q base=%q", trimmed, fp1)
	}

	// Case-only differences collapse via ToLower (AS8).
	lowered := DismissalFingerprint(model.LlmComment{Path: "main.go", StartLine: 10, EndLine: 12, Content: "FIX THIS"})
	if lowered != fp1 {
		t.Errorf("ToLower normalization failed: lower=%q base=%q", lowered, fp1)
	}

	// Different path/line/content produce different fingerprints (D4/AS8).
	differents := []model.LlmComment{
		{Path: "other.go", StartLine: 10, EndLine: 12, Content: "Fix this"}, // different path
		{Path: "main.go", StartLine: 99, EndLine: 12, Content: "Fix this"},  // different start line
		{Path: "main.go", StartLine: 10, EndLine: 99, Content: "Fix this"},  // different end line
		{Path: "main.go", StartLine: 10, EndLine: 12, Content: "different"}, // different content
		{Path: "main.go", StartLine: 10, EndLine: 12, Content: "Fix this "}, // trailing space -> same after trim (sanity)
	}
	for i, d := range differents {
		got := DismissalFingerprint(d)
		if i == 4 {
			if got != fp1 {
				t.Errorf("case %d: expected same FP after trim, got different: %q vs %q", i, got, fp1)
			}
			continue
		}
		if got == fp1 {
			t.Errorf("case %d: expected different fingerprint, got same %q", i, got)
		}
	}
}

func TestNormalizeComment(t *testing.T) {
	tests := []struct{ in, want string }{
		{"  Hello  ", "hello"},
		{"\n\tWorld\r\n", "world"},
		{"Already", "already"},
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		if got := normalizeComment(tt.in); got != tt.want {
			t.Errorf("normalizeComment(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDismissalStoreRoundTrip(t *testing.T) {
	repo := dismissTestRepo(t)
	store, err := LoadDismissals(repo)
	if err != nil {
		t.Fatalf("LoadDismissals (empty): %v", err)
	}
	entry := DismissalEntry{
		Fingerprint:    DismissalFingerprint(model.LlmComment{Path: "a.go", StartLine: 1, EndLine: 2, Content: "bug"}),
		Path:           "a.go",
		ContentPreview: "bug",
		DismissedAt:    "2026-07-26T10:00:00Z",
	}
	store.Record(entry)
	if !store.dirty {
		t.Fatal("Record did not mark store dirty")
	}
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if store.dirty {
		t.Fatal("Save did not clear dirty")
	}
	if !store.Contains(entry.Fingerprint) {
		t.Fatal("Contains false before reload despite in-memory record")
	}

	// Reload in a fresh store instance (simulates a new process — AS6/D5).
	reloaded, err := LoadDismissals(repo)
	if err != nil {
		t.Fatalf("LoadDismissals (reload): %v", err)
	}
	if !reloaded.Contains(entry.Fingerprint) {
		t.Fatal("reloaded store does not contain the recorded fingerprint")
	}
	got := reloaded.List()
	if len(got) != 1 {
		t.Fatalf("reloaded store has %d entries, want 1", len(got))
	}
	if got[0].Path != "a.go" || got[0].ContentPreview != "bug" {
		t.Errorf("reloaded entry mismatch: %+v", got[0])
	}
	// Save is a no-op when not dirty (round-trips to identical bytes).
	firstBytes := readDismissalStore(t, repo)
	if err := reloaded.Save(); err != nil {
		t.Fatalf("no-op Save: %v", err)
	}
	secondBytes := readDismissalStore(t, repo)
	if string(firstBytes) != string(secondBytes) {
		t.Errorf("no-op Save rewrote the file")
	}
}

func TestDismissalStoreMissingFileIsEmpty(t *testing.T) {
	repo := dismissTestRepo(t)
	store, err := LoadDismissals(repo)
	if err != nil {
		t.Fatalf("LoadDismissals on missing file returned error: %v (want nil)", err)
	}
	if store == nil {
		t.Fatal("LoadDismissals on missing file returned nil store")
	}
	if got := store.List(); len(got) != 0 {
		t.Fatalf("missing-file store has %d entries, want 0", len(got))
	}
	// No file should have been created merely by loading.
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Fatalf("load created a file at %q (want no touch)", store.Path())
	}
}

func TestDismissalStoreCorruptFailsSafe(t *testing.T) {
	repo := dismissTestRepo(t)
	garbage := []byte("{ this is : not valid json,,,")
	writeDismissalStore(t, repo, garbage)

	store, err := LoadDismissals(repo)
	if err == nil {
		t.Fatal("LoadDismissals on corrupt file returned nil error (want non-nil)")
	}
	if store == nil {
		t.Fatal("LoadDismissals on corrupt file returned nil store (want empty store)")
	}
	// Fail-safe: empty store, no suppression will happen.
	if got := store.List(); len(got) != 0 {
		t.Fatalf("corrupt-file store has %d entries, want 0 (fail-safe empty)", len(got))
	}
	// D6/AS5: the corrupt file must be left untouched on disk.
	after := readDismissalStore(t, repo)
	if string(after) != string(garbage) {
		t.Fatalf("corrupt file was modified on load:\n before=%q\n after=%q", garbage, after)
	}
}

func TestDismissalStoreUnreadableFileFailsSafe(t *testing.T) {
	// Cover the unreadable (non-IsNotExist) branch of LoadDismissals by making
	// the file unreadable. Skip on systems where we cannot enforce permission
	// bits (running as root ignores 0o000).
	if os.Geteuid() == 0 {
		t.Skip("cannot test unreadable file as root")
	}
	repo := dismissTestRepo(t)
	garbage := []byte("{not json}")
	writeDismissalStore(t, repo, garbage)
	path, err := dismissalPath(repo)
	if err != nil {
		t.Fatalf("dismissalPath: %v", err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o600) })

	store, err := LoadDismissals(repo)
	if err == nil {
		t.Fatal("expected non-nil error when file is unreadable")
	}
	if store == nil {
		t.Fatal("expected non-nil store on unreadable file (fail-safe)")
	}
	// File left untouched: restore read permission and compare bytes.
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("chmod restore: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after restore: %v", err)
	}
	if string(after) != string(garbage) {
		t.Errorf("unreadable file was modified on load:\n before=%q\n after=%q", garbage, after)
	}
}

func TestDismissalStoreRecordIdempotent(t *testing.T) {
	repo := dismissTestRepo(t)
	store, err := LoadDismissals(repo)
	if err != nil {
		t.Fatalf("LoadDismissals: %v", err)
	}
	fp := DismissalFingerprint(model.LlmComment{Path: "x.go", StartLine: 5, EndLine: 5, Content: "dup"})
	store.Record(DismissalEntry{Fingerprint: fp, Path: "x.go", ContentPreview: "first"})
	store.Record(DismissalEntry{Fingerprint: fp, Path: "x.go", ContentPreview: "second"})
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reloaded, err := LoadDismissals(repo)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := reloaded.List()
	if len(got) != 1 {
		t.Fatalf("idempotent Record produced %d entries, want 1", len(got))
	}
	// Last-write-wins on duplicate fingerprint within the map.
	if got[0].ContentPreview != "second" {
		t.Errorf("idempotent Record preview = %q, want %q (last-write-wins)", got[0].ContentPreview, "second")
	}
}

func TestDismissalStoreRemove(t *testing.T) {
	repo := dismissTestRepo(t)
	store, err := LoadDismissals(repo)
	if err != nil {
		t.Fatalf("LoadDismissals: %v", err)
	}
	fp := DismissalFingerprint(model.LlmComment{Path: "y.go", StartLine: 1, EndLine: 1, Content: "rm"})
	store.Record(DismissalEntry{Fingerprint: fp, Path: "y.go"})
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !store.Contains(fp) {
		t.Fatal("Contains false before Remove")
	}
	if store.Remove("not-present") {
		t.Error("Remove returned true for absent fingerprint")
	}
	if !store.Remove(fp) {
		t.Fatal("Remove returned false for present fingerprint")
	}
	if store.Contains(fp) {
		t.Error("Contains true after Remove")
	}
	if err := store.Save(); err != nil {
		t.Fatalf("Save after remove: %v", err)
	}
	reloaded, err := LoadDismissals(repo)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Contains(fp) {
		t.Error("removed fingerprint survived reload")
	}
}

func TestDismissalStoreListSorted(t *testing.T) {
	repo := dismissTestRepo(t)
	store, err := LoadDismissals(repo)
	if err != nil {
		t.Fatalf("LoadDismissals: %v", err)
	}
	store.Record(DismissalEntry{Fingerprint: "c", DismissedAt: "2026-07-26T03:00:00Z"})
	store.Record(DismissalEntry{Fingerprint: "a", DismissedAt: "2026-07-26T01:00:00Z"})
	store.Record(DismissalEntry{Fingerprint: "b", DismissedAt: "2026-07-26T02:00:00Z"})
	got := store.List()
	if len(got) != 3 {
		t.Fatalf("List returned %d entries, want 3", len(got))
	}
	// Sorted by DismissedAt ascending.
	if got[0].Fingerprint != "a" || got[1].Fingerprint != "b" || got[2].Fingerprint != "c" {
		t.Errorf("List not sorted by DismissedAt: %+v", got)
	}
}

func TestDismissalFilterSuppress(t *testing.T) {
	repo := dismissTestRepo(t)
	store, err := LoadDismissals(repo)
	if err != nil {
		t.Fatalf("LoadDismissals: %v", err)
	}
	cA := model.LlmComment{Path: "a.go", StartLine: 1, EndLine: 2, Content: "alpha"}
	cB := model.LlmComment{Path: "a.go", StartLine: 5, EndLine: 6, Content: "beta"}
	cC := model.LlmComment{Path: "b.go", StartLine: 1, EndLine: 1, Content: "gamma"}
	// Dismiss only cA's fingerprint.
	store.Record(DismissalEntry{Fingerprint: DismissalFingerprint(cA)})
	filter := NewDismissalFilter(store)

	input := []model.LlmComment{cA, cB, cC}
	// Snapshot input to prove Suppress does not mutate it (D3).
	inputCopy := make([]model.LlmComment, len(input))
	copy(inputCopy, input)

	out := filter.Suppress(input)
	if len(out) != 2 {
		t.Fatalf("Suppress returned %d comments, want 2 (cA dismissed)", len(out))
	}
	for _, c := range out {
		if c.Content == "alpha" {
			t.Errorf("dismissed comment alpha survived Suppress: %+v", c)
		}
	}
	// Order preserved: cB then cC.
	if out[0].Content != "beta" || out[1].Content != "gamma" {
		t.Errorf("Suppress changed order: %+v", out)
	}
	// Input slice unmodified (D3).
	for i := range input {
		if input[i] != inputCopy[i] {
			t.Errorf("Suppress mutated input[%d]: %+v vs %+v", i, input[i], inputCopy[i])
		}
	}
}

func TestDismissalFilterEmptySetReturnsInput(t *testing.T) {
	// Empty store -> empty set -> Suppress returns input slice as-is (zero-alloc fast path).
	store, err := LoadDismissals(dismissTestRepo(t))
	if err != nil {
		t.Fatalf("LoadDismissals: %v", err)
	}
	filter := NewDismissalFilter(store)
	input := []model.LlmComment{{Path: "a.go", Content: "x"}}
	out := filter.Suppress(input)
	if len(out) != 1 || &out[0] != &input[0] {
		t.Fatalf("empty-set Suppress did not return input slice as-is: len=%d same-alloc=%v", len(out), len(out) > 0 && &out[0] == &input[0])
	}
}

func TestDismissalFilterNilSafe(t *testing.T) {
	// NewDismissalFilter(nil) must not panic and yields a no-op filter.
	filter := NewDismissalFilter(nil)
	out := filter.Suppress([]model.LlmComment{{Path: "a.go", Content: "x"}})
	if len(out) != 1 {
		t.Fatalf("nil-store filter dropped comments: %d", len(out))
	}
	// Suppress on a nil receiver must not panic and returns the input.
	var nilFilter *DismissalFilter
	got := nilFilter.Suppress([]model.LlmComment{{Path: "a.go", Content: "x"}})
	if len(got) != 1 {
		t.Fatalf("nil-receiver Suppress dropped comments: %d", len(got))
	}
}

func TestDismissalFilePathResolvesUnderRoot(t *testing.T) {
	repo := dismissTestRepo(t)
	path, err := DismissalFilePath(repo)
	if err != nil {
		t.Fatalf("DismissalFilePath: %v", err)
	}
	if !strings.Contains(path, dismissalSubDir) {
		t.Errorf("DismissalFilePath %q does not contain dismissal subdir %q", path, dismissalSubDir)
	}
	// Distinct from the session path (no collision).
	sessPath, err := SessionFilePath(repo, "sess123")
	if err != nil {
		t.Fatalf("SessionFilePath: %v", err)
	}
	if path == sessPath {
		t.Errorf("dismissal path collides with session path: %q", path)
	}
}

func TestLoadCommentsWalksSessionJSONL(t *testing.T) {
	// Build a session JSONL by hand with two done records carrying comments,
	// plus a non-comment record type that must be skipped.
	repo := dismissTestRepo(t)
	path, err := SessionFilePath(repo, "sess-abc")
	if err != nil {
		t.Fatalf("SessionFilePath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	lines := []string{
		`{"type":"session_start","sessionId":"sess-abc","timestamp":"2026-07-26T10:00:00Z","cwd":"` + repo + `","gitBranch":"main","model":"fake"}`,
		`{"type":"review_item_done","filePath":"a.go","fingerprint":"fa","comments":[{"path":"a.go","content":"alpha","start_line":1,"end_line":2},{"path":"a.go","content":"alpha2","start_line":3,"end_line":4}]}`,
		`{"type":"llm_request","filePath":"a.go"}`,
		`{"type":"review_item_reused","filePath":"b.go","fingerprint":"fb","comments":[{"path":"b.go","content":"beta","start_line":1,"end_line":1}]}`,
		`{"type":"review_item_done","filePath":"c.go","fingerprint":"fc"}`,
		`{"type":"session_end","sessionId":"sess-abc"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatalf("write session: %v", err)
	}

	got, err := LoadComments(repo, "sess-abc")
	if err != nil {
		t.Fatalf("LoadComments: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("LoadComments returned %d comments, want 3", len(got))
	}
	// File-record order: a.go's two comments, then b.go's one comment. The
	// llm_request and the comment-less c.go record are skipped.
	wantContents := []string{"alpha", "alpha2", "beta"}
	for i, want := range wantContents {
		if got[i].Content != want {
			t.Errorf("LoadComments[%d].Content = %q, want %q", i, got[i].Content, want)
		}
	}
	// Fingerprint computed from LoadComments output matches one computed from
	// an equivalent in-memory comment (D1 cross-process consistency).
	fp := DismissalFingerprint(model.LlmComment{Path: "b.go", StartLine: 1, EndLine: 1, Content: "beta"})
	if DismissalFingerprint(got[2]) != fp {
		t.Errorf("LoadComments comment fingerprint mismatch: got %q want %q", DismissalFingerprint(got[2]), fp)
	}
}

func TestLoadCommentsMissingSession(t *testing.T) {
	got, err := LoadComments(dismissTestRepo(t), "nope")
	if err == nil {
		t.Fatal("LoadComments on missing session returned nil error")
	}
	if got != nil && len(got) != 0 {
		t.Errorf("LoadComments on missing session returned %d comments, want 0", len(got))
	}
}

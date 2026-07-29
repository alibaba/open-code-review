package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/open-code-review/open-code-review/internal/model"
)

// dismissalSubDir is the per-repo dismissal store directory, a sibling of
// sessionSubDir under the ~/.opencodereview root. Mirrored to a test dir by
// UseTestSessions so dismissal tests stay offline and hermetic.
var dismissalSubDir = "dismissals"

// DismissalEntry records one dismissed finding for human inspection
// (ocr dismiss list) and cross-run traceability. Suppression keys only on
// Fingerprint; the remaining fields are descriptive.
type DismissalEntry struct {
	Fingerprint     string `json:"fingerprint"`
	Path            string `json:"path"`
	ContentPreview  string `json:"content_preview"`
	DismissedAt     string `json:"dismissed_at"`
	SourceSessionID string `json:"source_session_id"`
}

// DismissalFile is the on-disk JSON document for a per-repo dismissal store.
// Version is serialized as 1; future schema changes (e.g. semantic identity)
// bump this so old stores migrate rather than break.
type DismissalFile struct {
	Version    int              `json:"version"`
	Repo       string           `json:"repo"`
	Dismissals []DismissalEntry `json:"dismissals"`
}

// DismissalStore is a per-repo, filesystem-backed record of dismissed findings.
//
// LoadDismissals opens the store; Save persists changes. Contains performs an
// O(1) membership check by fingerprint. The store holds an in-memory map keyed
// by Fingerprint so the suppression check during a review is a single map lookup.
type DismissalStore struct {
	repoDir string
	path    string
	entries map[string]DismissalEntry
	dirty   bool
}

// newDismissalStore resolves the on-disk path for a repo's dismissal store
// without touching disk. It mirrors the session substrate (encodeRepoPath +
// the ~/.opencodereview root) so a repo's dismissals and sessions never collide.
func newDismissalStore(repoDir string) (*DismissalStore, error) {
	path, err := dismissalPath(repoDir)
	if err != nil {
		return nil, err
	}
	return &DismissalStore{
		repoDir: repoDir,
		path:    path,
		entries: make(map[string]DismissalEntry),
	}, nil
}

// DismissalFilePath returns the on-disk path of a repo's dismissal store without
// touching disk. It is the cheap existence-check entry point used by the review
// path (one stat, no read when absent — D2). Missing home dir is an error.
func DismissalFilePath(repoDir string) (string, error) {
	return dismissalPath(repoDir)
}

func dismissalPath(repoDir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".opencodereview", dismissalSubDir, encodeRepoPath(repoDir)+".json"), nil
}

// LoadDismissals reads the per-repo dismissal store.
//
// Missing file → empty store, nil error (the common no-opt-in case; no warning).
// Unreadable or unparseable → empty store, non-nil error (caller logs a warning
// and proceeds stateless); the file is left untouched (D6/AS5: do not overwrite a
// corrupt store, which would destroy evidence of how it got corrupt).
func LoadDismissals(repoDir string) (*DismissalStore, error) {
	store, err := newDismissalStore(repoDir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		if os.IsNotExist(err) {
			// No opt-in: stateless behavior. Return an empty store so callers can
			// still Record/Save into it if they want (e.g. ocr dismiss add).
			return store, nil
		}
		return store, fmt.Errorf("read dismissal store %q: %w", store.path, err)
	}
	var file DismissalFile
	if err := json.Unmarshal(data, &file); err != nil {
		// Corrupt: fail safe. Return an empty store + error; leave the file on disk
		// untouched (D6/AS5). Callers print a warning and proceed stateless.
		return store, fmt.Errorf("parse dismissal store %q: %w", store.path, err)
	}
	for _, e := range file.Dismissals {
		// Last-write-wins on duplicate fingerprints within the file; canonical map.
		store.entries[e.Fingerprint] = e
	}
	return store, nil
}

// Path returns the resolved on-disk path of the store (for warnings/errors).
func (s *DismissalStore) Path() string { return s.path }

// Contains reports whether a fingerprint is recorded as dismissed.
func (s *DismissalStore) Contains(fp string) bool {
	_, ok := s.entries[fp]
	return ok
}

// Record adds or replaces a dismissal entry by fingerprint and marks the store
// dirty. Re-recording the same fingerprint is idempotent (AS3): the entry is
// updated in place, but Save produces one record, not duplicates.
func (s *DismissalStore) Record(entry DismissalEntry) {
	s.entries[entry.Fingerprint] = entry
	s.dirty = true
}

// Remove deletes a dismissal entry by fingerprint. It returns true if an entry
// was present. The store is marked dirty only when something was removed.
func (s *DismissalStore) Remove(fp string) bool {
	if _, ok := s.entries[fp]; !ok {
		return false
	}
	delete(s.entries, fp)
	s.dirty = true
	return true
}

// List returns the recorded dismissals sorted by DismissedAt (then Fingerprint)
// for stable output in `ocr dismiss list`.
func (s *DismissalStore) List() []DismissalEntry {
	out := make([]DismissalEntry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DismissedAt != out[j].DismissedAt {
			return out[i].DismissedAt < out[j].DismissedAt
		}
		return out[i].Fingerprint < out[j].Fingerprint
	})
	return out
}

// Save persists the store to disk if it has changed since load.
//
// It is a no-op (returns nil) when the store is not dirty. Otherwise it writes
// to a temp file in the same directory and atomically renames it over the
// target (atomic on POSIX and Windows for same-directory renames), guarding
// against torn reads/writes on crash (A7). Mode 0600 matches session JSONL so
// dismissal content (which may quote code) is not world-readable.
func (s *DismissalStore) Save() error {
	if !s.dirty {
		return nil
	}
	file := DismissalFile{
		Version:    1,
		Repo:       s.repoDir,
		Dismissals: s.List(),
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dismissal store: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create dismissal dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".dismissal-*.tmp")
	if err != nil {
		return fmt.Errorf("create dismissal temp file: %w", err)
	}
	tmpName := tmp.Name()
	// Clean up the temp file if any step below fails; rename takes ownership on success.
	cleanup := func() { os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("write dismissal temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close dismissal temp file: %w", err)
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		cleanup()
		return fmt.Errorf("chmod dismissal temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		cleanup()
		return fmt.Errorf("rename dismissal temp file: %w", err)
	}
	s.dirty = false
	return nil
}

// normalizeComment is the canonicalization contract for dismissal identity (A1).
// It is deliberately minimal: TrimSpace collapses trailing/leading whitespace
// differences (a likely serialization-vs-display divergence) and ToLower handles
// ASCII case. Rephrasing/stylistic differences are explicitly out of scope (A1
// known limitation); code-fence/citation stripping is intentionally not done so
// the contract stays trivially deterministic.
func normalizeComment(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// DismissalFingerprint returns the stable per-finding identity for a comment
// (A1): SHA256(path + "\x00" + start_line + "\x00" + end_line + "\x00" + normalizedContent).
// Path and line numbers are taken verbatim (already canonical); Content is
// normalized via normalizeComment. Two findings on the same file differ on
// StartLine/EndLine/Content (D4).
func DismissalFingerprint(c model.LlmComment) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d\x00%d\x00%s", c.Path, c.StartLine, c.EndLine, normalizeComment(c.Content))
	return hex.EncodeToString(h.Sum(nil))
}

// DismissalFilter is a read-only set of dismissed fingerprints used to suppress
// findings from a review's returned comment slice. It is constructed once at
// review start (single goroutine, before subtask dispatch) and passed by
// reference to Agent.Run.
type DismissalFilter struct {
	set map[string]struct{}
}

// NewDismissalFilter builds a filter from a store's recorded fingerprints. A
// nil store yields a filter with an empty set (Suppress is then a no-op that
// returns its input unchanged).
func NewDismissalFilter(store *DismissalStore) *DismissalFilter {
	f := &DismissalFilter{set: make(map[string]struct{})}
	if store == nil {
		return f
	}
	for fp := range store.entries {
		f.set[fp] = struct{}{}
	}
	return f
}

// Suppress returns a new slice omitting any comment whose dismissal fingerprint
// is in the filter's set (D1). It never mutates the input slice (D3) and
// preserves element order. When the set is empty it returns the input slice
// unchanged (zero-alloc fast path).
func (f *DismissalFilter) Suppress(comments []model.LlmComment) []model.LlmComment {
	if f == nil || len(f.set) == 0 {
		return comments
	}
	out := make([]model.LlmComment, 0, len(comments))
	for _, c := range comments {
		if _, dismissed := f.set[DismissalFingerprint(c)]; !dismissed {
			out = append(out, c)
		}
	}
	return out
}

// LoadComments walks a session JSONL and returns the flattened list of review
// comments in file-record order (the order records appear in the JSONL ×
// comment order within each review_item_done/review_item_reused record).
//
// It exists because session.LoadDetail exposes only comment *counts*
// (ItemDetail.Comments is an int; the raw json.RawMessage is discarded), so
// `ocr dismiss add` has no existing public path to the comment bodies. This
// helper is additive: no existing session function signature changes, and it
// reuses the same SessionFilePath substrate as LoadResumeState/LoadDetail.
func LoadComments(repoDir, sessionID string) ([]model.LlmComment, error) {
	path, err := SessionFilePath(repoDir, sessionID)
	if err != nil {
		return nil, err
	}
	var comments []model.LlmComment
	walkErr := walkSessionFile(path, func(rec summaryRecord) {
		if rec.Type != "review_item_done" && rec.Type != "review_item_reused" {
			return
		}
		if len(rec.Comments) == 0 {
			return
		}
		var batch []model.LlmComment
		if err := json.Unmarshal(rec.Comments, &batch); err != nil {
			return // malformed comment payload in one record does not poison the rest
		}
		comments = append(comments, batch...)
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return comments, nil
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestListSessions_EmptyRepoReturnsNil(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)

	got, err := ListSessions(t.TempDir())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d entries", len(got))
	}
}

func TestListSessions_SortsAndAggregates(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	older := writeTestSession(t, repoDir, "feature-a", "commit-x", []model.LlmComment{
		{Path: "a.go", Content: "one"},
	}, 1, 0, true)
	time.Sleep(1100 * time.Millisecond)
	newer := writeTestSession(t, repoDir, "feature-a", "commit-y", []model.LlmComment{
		{Path: "b.go", Content: "one"},
		{Path: "b.go", Content: "two"},
	}, 2, 1, false)

	got, err := ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(got))
	}
	if got[0].SessionID != newer {
		t.Errorf("expected newest first, got %q vs %q", got[0].SessionID, newer)
	}
	if got[1].SessionID != older {
		t.Errorf("expected older second, got %q", got[1].SessionID)
	}

	if !got[0].Aborted {
		t.Errorf("newest session was interrupted; expected Aborted=true")
	}
	if got[1].Aborted {
		t.Errorf("older session was finalized; expected Aborted=false")
	}
	if got[0].TotalComments != 2 {
		t.Errorf("newest TotalComments = %d, want 2", got[0].TotalComments)
	}
	if got[0].FailedFiles != 1 {
		t.Errorf("newest FailedFiles = %d, want 1", got[0].FailedFiles)
	}
	if got[0].CompletedFiles != 2 {
		t.Errorf("newest CompletedFiles = %d, want 2", got[0].CompletedFiles)
	}
}

func TestListSessions_MarksFreshUnfinishedSessionRunning(t *testing.T) {
	// A pre-heartbeat (legacy) run still in progress: records appended, no
	// session_end, no heartbeat records, file just written. Expect
	// Running=true via the legacy activity window while Aborted keeps its
	// file-based true, so resume logic and existing JSON consumers are
	// unaffected.
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	// Zero interval suppresses even the initial beat, leaving a file exactly
	// like a pre-heartbeat version writes.
	defer swapHeartbeatInterval(t, 0)()
	writeTestSession(t, repoDir, "feature-a", "commit-x", nil, 1, 0, false)

	got, err := ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 session, got %d", len(got))
	}
	if !got[0].Running {
		t.Errorf("unfinished session with a fresh file: Running = false, want true")
	}
	if !got[0].Aborted {
		t.Errorf("Aborted must stay true until session_end reaches disk")
	}
}

func TestListSessions_StaleUnfinishedSessionNotRunning(t *testing.T) {
	// A legacy-file run whose process is gone: no session_end, no heartbeat
	// records (so the activity window applies), and nothing appended for
	// longer than legacyActivityWindow. Expect Running=false, Aborted=true —
	// a real interruption, not an active session.
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	// Zero interval keeps the file heartbeat-free so the legacy window is
	// the one under test.
	defer swapHeartbeatInterval(t, 0)()
	id := writeTestSession(t, repoDir, "feature-a", "commit-x", nil, 1, 0, false)

	path, err := SessionFilePath(repoDir, id)
	if err != nil {
		t.Fatalf("SessionFilePath: %v", err)
	}
	stale := time.Now().Add(-2 * legacyActivityWindow)
	if err := os.Chtimes(path, stale, stale); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	got, err := ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 session, got %d", len(got))
	}
	if got[0].Running {
		t.Errorf("stale unfinished session: Running = true, want false")
	}
	if !got[0].Aborted {
		t.Errorf("stale unfinished session: Aborted = false, want true")
	}
}

func TestListSessions_QuietLiveHeartbeatWriterStaysRunning(t *testing.T) {
	// A live writer producing nothing but heartbeats — the quiet-request
	// case: the run sits inside one long provider call or retry wait, so no
	// review_item or request-boundary record is appended, yet heartbeat
	// records keep reaching disk. The writer stays alive (its heartbeat
	// goroutine still running) across the ListSessions call, pinning the
	// integration contract: a live-but-quiet session reports Running, which
	// no append-activity window of any fixed size could guarantee.
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	defer swapHeartbeatInterval(t, 20*time.Millisecond)()

	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeRange,
		DiffFrom:   "feature-a",
		DiffTo:     "commit-x",
	})
	if sh.persist == nil {
		t.Fatal("session writer was not created")
	}
	// Terminate the writer when the test ends, on every exit path: the
	// heartbeat goroutine must not outlive the test.
	defer sh.persist.flushAndClose()

	time.Sleep(120 * time.Millisecond) // quiet interval: beats land, nothing else

	path, err := SessionFilePath(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("SessionFilePath: %v", err)
	}
	heartbeats := countRecordTypes(t, path)["heartbeat"]
	if heartbeats < 2 {
		t.Fatalf("expected >= 2 heartbeat records in a quiet live session, got %d", heartbeats)
	}

	// The heartbeat goroutine is still running while the list is read.
	got, err := ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 session, got %d", len(got))
	}
	if !got[0].Running {
		t.Errorf("quiet but live heartbeat writer: Running = false, want true")
	}
	if !got[0].Aborted {
		t.Errorf("no session_end on disk: Aborted = false, want true")
	}
}

func TestListSessions_TerminatedHeartbeatWriterNotRunningAfterWindow(t *testing.T) {
	// A writer that died without session_end. Immediately after the kill the
	// session still reports Running — the documented residual of a
	// freshness-based signal, bounded by livenessWindow. Once the file goes
	// stale past livenessWindow, Running must flip off even though heartbeat
	// records are present in the file: a heartbeat selects the stricter
	// window, it does not pin the session as running forever.
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	defer swapHeartbeatInterval(t, 20*time.Millisecond)()
	id := writeKilledHeartbeatSession(t, repoDir)

	path, err := SessionFilePath(repoDir, id)
	if err != nil {
		t.Fatalf("SessionFilePath: %v", err)
	}
	if got := countRecordTypes(t, path)["heartbeat"]; got < 1 {
		t.Fatalf("expected heartbeat records in the file, got %d", got)
	}

	// Fresh kill: still inside livenessWindow — the bounded false-positive.
	fresh, err := LoadSummary(repoDir, id)
	if err != nil {
		t.Fatalf("LoadSummary: %v", err)
	}
	if !fresh.Running || !fresh.Aborted {
		t.Fatalf("freshly killed writer: running=%v aborted=%v, want true/true", fresh.Running, fresh.Aborted)
	}

	// Past livenessWindow: heartbeat records exist, but staleness wins.
	stale := time.Now().Add(-2 * livenessWindow)
	if err := os.Chtimes(path, stale, stale); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	got, err := ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 session, got %d", len(got))
	}
	if got[0].Running {
		t.Errorf("terminated writer past livenessWindow: Running = true, want false")
	}
	if !got[0].Aborted {
		t.Errorf("no session_end on disk: Aborted = false, want true")
	}
}

func TestListSessions_KilledBeforeFirstPeriodicBeatUsesLivenessWindow(t *testing.T) {
	// Review regression: a current-version writer dies before its first
	// PERIODIC heartbeat tick, so the only heartbeat in the file is the
	// synchronous initial beat written at session start. That beat must still
	// select the heartbeat liveness window, not the legacy activity window:
	// once aged past livenessWindow — while staying younger than
	// legacyActivityWindow — the session must stop reporting Running.
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	// A minute between periodic beats: no periodic tick fires during the
	// test, so any heartbeat in the file is the initial synchronous one.
	defer swapHeartbeatInterval(t, time.Minute)()

	id := writeKilledHeartbeatSession(t, repoDir)

	path, err := SessionFilePath(repoDir, id)
	if err != nil {
		t.Fatalf("SessionFilePath: %v", err)
	}
	if got := countRecordTypes(t, path)["heartbeat"]; got != 1 {
		t.Fatalf("heartbeat records = %d, want exactly the 1 initial beat written before the first periodic tick", got)
	}

	// Fresh kill: still inside livenessWindow, the session reports Running.
	fresh, err := LoadSummary(repoDir, id)
	if err != nil {
		t.Fatalf("LoadSummary: %v", err)
	}
	if !fresh.Running || !fresh.Aborted {
		t.Fatalf("freshly killed writer: running=%v aborted=%v, want true/true", fresh.Running, fresh.Aborted)
	}

	// Aged past livenessWindow but well inside legacyActivityWindow: had the
	// initial beat been missing, the legacy window would keep it Running.
	stale := time.Now().Add(-2 * livenessWindow)
	if err := os.Chtimes(path, stale, stale); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	got, err := ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 session, got %d", len(got))
	}
	if got[0].Running {
		t.Errorf("killed before the first periodic beat, aged past livenessWindow: Running = true, want false")
	}
	if !got[0].Aborted {
		t.Errorf("no session_end on disk: Aborted = false, want true")
	}
}

func TestListSessions_FinalizedFreshFileNeverRunning(t *testing.T) {
	// A finalized session: session_end is on disk, so the file is fresh but
	// the run is over. Running must stay false no matter how fresh the file
	// is — running is only meaningful for unfinished sessions.
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	writeTestSession(t, repoDir, "feature-a", "commit-x", nil, 1, 0, true)

	got, err := ListSessions(repoDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 session, got %d", len(got))
	}
	if got[0].Running {
		t.Errorf("finalized session: Running = true, want false")
	}
	if got[0].Aborted {
		t.Errorf("finalized session: Aborted = true, want false")
	}
}

// writeKilledHeartbeatSession starts a real writer under the test-swapped
// heartbeat interval, lets heartbeat records reach disk with no other progress
// records, then terminates it without session_end — the on-disk state a
// killed writer leaves behind. Returns the session id.
func writeKilledHeartbeatSession(t *testing.T, repoDir string) string {
	t.Helper()
	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeRange,
		DiffFrom:   "feature-a",
		DiffTo:     "commit-x",
	})
	time.Sleep(120 * time.Millisecond)
	if sh.persist == nil {
		t.Fatal("session writer was not created")
	}
	sh.persist.stopHeartbeat()
	sh.persist.mu.Lock()
	sh.persist.writer.Flush()
	sh.persist.file.Close()
	sh.persist.writer = nil
	sh.persist.file = nil
	sh.persist.mu.Unlock()
	return sh.SessionID
}

// swapHeartbeatInterval shrinks the heartbeat interval for tests that need
// real heartbeats to reach disk quickly, and returns a function restoring the
// production value. The session package's writer tests run sequentially, so
// the package-level variable is safe to swap.
func swapHeartbeatInterval(t *testing.T, d time.Duration) func() {
	t.Helper()
	old := heartbeatInterval
	heartbeatInterval = d
	return func() { heartbeatInterval = old }
}

// countRecordTypes reads a session JSONL file and tallies records by their
// type field, tolerating malformed lines the same way production readers do.
func countRecordTypes(t *testing.T, path string) map[string]int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	counts := make(map[string]int)
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		var rec struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		counts[rec.Type]++
	}
	return counts
}

func TestLoadDetail_ReturnsItems(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeCommit,
		DiffCommit: "abc123",
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", []model.LlmComment{{Path: "a.go", Content: "note"}})
	sh.RecordReviewItemReused("b.go", "b.go", "b.go", "fp-b", "prior-session", []model.LlmComment{{Path: "b.go", Content: "cached"}})
	sh.RecordReviewItemFailed("c.go", "c.go", "c.go", "fp-c", "boom")
	sh.Finalize()

	summary, items, err := LoadDetail(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("LoadDetail: %v", err)
	}
	if summary.CompletedFiles != 1 || summary.ReusedFiles != 1 || summary.FailedFiles != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.TotalComments != 2 {
		t.Errorf("TotalComments = %d, want 2", summary.TotalComments)
	}
	if summary.Aborted {
		t.Errorf("summary should not be aborted after Finalize")
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	byType := map[string]ItemDetail{}
	for _, it := range items {
		byType[it.Type] = it
	}
	if reused := byType["reused"]; reused.SourceSessionID != "prior-session" {
		t.Errorf("reused source = %q, want prior-session", reused.SourceSessionID)
	}
	if failed := byType["failed"]; failed.Error != "boom" {
		t.Errorf("failed error = %q, want boom", failed.Error)
	}
	if done := byType["done"]; done.Comments != 1 {
		t.Errorf("done comments = %d, want 1", done.Comments)
	}
}

func TestLoadSummary_FallsBackToSessionEndFilesReviewed(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeWorkspace,
	})
	sh.GetOrCreateFileSession("legacy-a.go")
	sh.GetOrCreateFileSession("legacy-b.go")
	sh.Finalize()

	summary, items, err := LoadDetail(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("LoadDetail: %v", err)
	}
	if summary.CompletedFiles != 2 {
		t.Fatalf("CompletedFiles = %d, want 2", summary.CompletedFiles)
	}
	if summary.ReusedFiles != 0 || summary.FailedFiles != 0 {
		t.Fatalf("unexpected checkpoint counts: %+v", summary)
	}
	if len(items) != 0 {
		t.Fatalf("legacy session should not synthesize item details, got %d", len(items))
	}
	if !summary.Legacy || summary.RunManifest != nil {
		t.Fatalf("legacy summary flags = legacy:%v manifest:%v", summary.Legacy, summary.RunManifest)
	}
}

func TestLoadSummaryPrefersV1RunManifest(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeWorkspace,
		Operation:  OperationReview,
	})
	b := sh.Manifest()
	b.SetInput(ManifestInput{Mode: InputModeWorkspace})
	items := []CoverageItem{
		{ItemID: "a", Path: "a.go"},
		{ItemID: "b", Path: "b.go"},
		{ItemID: "c", Path: "c.go"},
		{ItemID: "d", Path: "d.go"},
	}
	for _, item := range items {
		if err := b.RegisterSelected(item); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.SealSelected(); err != nil {
		t.Fatal(err)
	}
	if err := b.MarkCompleted("a"); err != nil {
		t.Fatal(err)
	}
	if err := b.MarkReused("b"); err != nil {
		t.Fatal(err)
	}
	if err := b.MarkFailed("c", FailureProvider, "provider request failed"); err != nil {
		t.Fatal(err)
	}
	if err := b.MarkWaived("d", "accepted by user"); err != nil {
		t.Fatal(err)
	}
	manifest, err := b.Finalize(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	sh.SetFinalManifest(&manifest)
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}

	summary, err := LoadSummary(repoDir, sh.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Legacy || summary.Aborted || summary.RunManifest == nil {
		t.Fatalf("summary flags = legacy:%v aborted:%v manifest:%v", summary.Legacy, summary.Aborted, summary.RunManifest)
	}
	if summary.RunManifest.TerminalState != StatePartial {
		t.Fatalf("terminal_state = %q", summary.RunManifest.TerminalState)
	}
	if summary.SelectedFiles != 4 || summary.CompletedFiles != 1 || summary.ReusedFiles != 1 || summary.FailedFiles != 1 || summary.WaivedFiles != 1 {
		t.Fatalf("coverage counts = %+v", summary)
	}
}

func TestLoadSummaryIgnoresUnknownManifestVersion(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	repoDir := t.TempDir()

	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	sh.SetFinalManifest(&RunManifest{SchemaVersion: "ocr.run-manifest/v999", TerminalState: StateComplete})
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}
	summary, err := LoadSummary(repoDir, sh.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !summary.Legacy || summary.RunManifest != nil {
		t.Fatalf("unknown schema must be treated as legacy: %+v", summary)
	}
}

func TestLoadSummary_MissingFile(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	if _, err := LoadSummary(t.TempDir(), "nonexistent"); err == nil {
		t.Fatal("expected error for missing session")
	}
}

// writeTestSession creates a real JSONL session using the persistence layer
// so tests exercise the same on-disk format that ListSessions consumes.
// It returns the session id.
func writeTestSession(t *testing.T, repoDir, from, to string, comments []model.LlmComment, doneCount, failedCount int, finalize bool) string {
	t.Helper()
	sh := New(repoDir, "main", "test-model", SessionOptions{
		ReviewMode: ReviewModeRange,
		DiffFrom:   from,
		DiffTo:     to,
	})
	for i := 0; i < doneCount; i++ {
		filePath := filepath.Base(t.TempDir()) + ".go"
		var perFile []model.LlmComment
		if i < len(comments) {
			perFile = []model.LlmComment{comments[i]}
		}
		sh.RecordReviewItemDone(filePath, filePath, filePath, "fp-"+filePath, perFile)
	}
	for i := 0; i < failedCount; i++ {
		filePath := "failed-" + filepath.Base(t.TempDir()) + ".go"
		sh.RecordReviewItemFailed(filePath, filePath, filePath, "fp-fail-"+filePath, "test error")
	}
	if finalize {
		sh.Finalize()
	} else {
		// Simulate an aborted run: flush the writer without emitting session_end.
		if sh.persist != nil {
			// Stop the heartbeat loop first: the manual close below bypasses
			// WriteSessionEnd/flushAndClose, which are the normal stop points.
			sh.persist.stopHeartbeat()
			sh.persist.mu.Lock()
			if sh.persist.writer != nil {
				sh.persist.writer.Flush()
			}
			if sh.persist.file != nil {
				_ = sh.persist.file.Close()
			}
			sh.persist.writer = nil
			sh.persist.file = nil
			sh.persist.mu.Unlock()
		}
	}
	return sh.SessionID
}

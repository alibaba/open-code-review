// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/session"
)

// writeSession creates a session file with the given JSONL lines under the
// repo directory the viewer serves, and returns its path.
func writeSession(t *testing.T, repoDir, id string, lines ...string) string {
	t.Helper()
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repoDir, id+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// liveSession starts a real run through the public session API and leaves it
// open, which is what puts the liveness lock on disk. Going through
// session.New rather than fabricating the sidecar means these tests also cover
// the writer half of the contract, so a regression there fails here too.
//
// repoDir must be the decoded working directory; the session file lands under a
// $HOME-derived path, which the caller redirects with t.Setenv.
func liveSession(t *testing.T, repoDir string) (*session.SessionHistory, string) {
	t.Helper()
	sh := session.New(repoDir, "main", "claude", session.SessionOptions{})
	if sh == nil {
		t.Fatal("session.New returned nil")
	}
	path, err := session.SessionFilePath(repoDir, sh.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	return sh, path
}

// An in-flight run has written session_start but no session_end. Reporting that
// as "aborted" was the original bug: it is indistinguishable from a run that
// died, and it is the one status that can never be true while the review is
// still going.
func TestLiveSession_ReportsRunningNotAborted(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := "/my/proj"
	sh, path := liveSession(t, repoDir)
	defer sh.Close()

	if !session.SessionIsActive(path) {
		t.Fatal("a run in progress did not report as active")
	}

	s, err := peekSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Running {
		t.Error("peekSession did not report the live run as running")
	}
	// Aborted stays true underneath - there is genuinely no session_end - which
	// is exactly why every template must test Running first.
	if !s.Aborted {
		t.Error("underlying Aborted flag should still be set while running")
	}
}

func TestListSessions_ReportsRunningFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repoDir := "/my/proj"
	sh, _ := liveSession(t, repoDir)
	defer sh.Close()

	summaries, err := ListSessions(filepath.Join(home, ".opencodereview", "sessions", "-my-proj"), "")
	if err == nil {
		t.Fatalf("expected an error listing from a repo dir that does not exist, got %v", summaries)
	}

	// The viewer's own ListSessions takes (root, encodedRepo).
	entries, err := os.ReadDir(filepath.Join(home, ".opencodereview", "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	var encoded string
	for _, e := range entries {
		if e.IsDir() {
			encoded = e.Name()
		}
	}
	if encoded == "" {
		t.Fatal("no encoded repo directory was created")
	}
	summaries, err = ListSessions(filepath.Join(home, ".opencodereview", "sessions"), encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("got %d summaries, want 1", len(summaries))
	}
	if !summaries[0].Running {
		t.Error("ListSessions did not report the live run as running")
	}
}

// A run that reached session_end is over, whatever the lock says: the record is
// the authoritative end, and the lock is only a fallback for runs that never
// got to write one.
func TestSessionEnd_ClearsRunning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sh, _ := liveSession(t, "/my/proj")

	if err := sh.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	// Re-open by id: Finalize closed the writer and released the lock.
	path, err := session.SessionFilePath("/my/proj", sh.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.SessionIsActive(path) {
		t.Error("session still active after Finalize")
	}

	s, err := peekSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Running {
		t.Error("finished session reported as running")
	}
	if s.Aborted {
		t.Error("finished session reported as aborted")
	}
	if s.EndTime.IsZero() {
		t.Error("EndTime not populated from the session_end timestamp")
	}
}

// Close stands in for a run that gave up: no session_end, lock released. That
// must read as aborted, not as still running.
func TestAbandonedRun_ReadsAsAborted(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sh, path := liveSession(t, "/my/proj")

	if err := sh.Close(); err != nil {
		t.Fatal(err)
	}
	if session.SessionIsActive(path) {
		t.Error("session still active after Close")
	}

	s, err := peekSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Running {
		t.Error("abandoned session reported as running")
	}
	if !s.Aborted {
		t.Error("abandoned session should read as aborted")
	}
}

func TestSessionsPage_ShowsRunningNotAborted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sh, _ := liveSession(t, "/my/proj")
	defer sh.Close()

	// The viewer's root is the sessions directory itself.
	root, err := session.SessionsDir("/my/proj")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/r/repo", nil)
	rr := httptest.NewRecorder()
	handleSessions(rr, req, filepath.Dir(root), filepath.Base(root))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, ">running</span>") {
		t.Error("sessions page status cell does not read 'running'")
	}
	if strings.Contains(body, ">aborted<") {
		t.Error("sessions page still reports the live run as aborted")
	}
	// The duration cell carries the timestamps the ticker needs.
	if !strings.Contains(body, "data-live-duration") {
		t.Error("running row has no live-duration element")
	}
}

func TestSessionPage_MetaShowsRunning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sh, _ := liveSession(t, "/my/proj")
	defer sh.Close()

	root, err := session.SessionsDir("/my/proj")
	if err != nil {
		t.Fatal(err)
	}
	rootDir, repo := filepath.Dir(root), filepath.Base(root)

	req := httptest.NewRequest("GET", "/r/repo/"+sh.SessionID, nil)
	rr := httptest.NewRecorder()
	handleSession(rr, req, rootDir, repo, sh.SessionID)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, `class="meta-status status-running"`) {
		t.Error("session page meta bar did not render a running status")
	}
	if strings.Contains(body, "status-aborted") {
		t.Error("session page still reports the live run as aborted")
	}
}

func TestReposPage_ShowsActiveSessionLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sh, _ := liveSession(t, "/my/proj")
	defer sh.Close()

	root, err := SessionsRoot()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handleRepos(rr, req, root)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "data-repo-active") {
		t.Error("repositories page did not mark the repo as running")
	}
	// The badge links straight into the live session.
	if !strings.Contains(body, `href="/r/`+filepath.Base(mustSessionsDir(t))+"/"+sh.SessionID+`"`) {
		t.Error("running badge does not link to the active session")
	}
}

func mustSessionsDir(t *testing.T) string {
	t.Helper()
	dir, err := session.SessionsDir("/my/proj")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReposPage_NoActiveSessionWhenFinished(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	writeSession(t, repoDir, "done2",
		`{"type":"session_start","timestamp":"2025-06-01T10:00:00Z","cwd":"/my/proj"}`,
		`{"type":"session_end","timestamp":"2025-06-01T10:05:00Z","duration_seconds":300}`,
	)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handleRepos(rr, req, root)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "data-repo-active") {
		t.Error("repositories page marked a finished repo as running")
	}
}

// DiscoverRepos picks the newest live session, so a repository with an archived
// history and one run in progress reports the run rather than the archive.
func TestDiscoverRepos_ActiveSessionIsNewestLive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repoDir := "/my/proj"

	// Two live runs cannot happen for one working tree in practice, but the
	// tie-break has to be defined rather than incidental.
	older, olderPath := liveSession(t, repoDir)
	defer older.Close()
	newer, newerPath := liveSession(t, repoDir)
	defer newer.Close()

	// Order by mtime explicitly, since a live run's file mtime is when its
	// session_start was flushed.
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(olderPath, past, past); err != nil {
		t.Fatal(err)
	}
	if !session.SessionIsActive(olderPath) || !session.SessionIsActive(newerPath) {
		t.Fatal("both runs should be active")
	}

	root, err := SessionsRoot()
	if err != nil {
		t.Fatal(err)
	}
	repos, err := DiscoverRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("got %d repos, want 1", len(repos))
	}
	if repos[0].ActiveSessionID != newer.SessionID {
		t.Errorf("ActiveSessionID = %q, want the newest run %q", repos[0].ActiveSessionID, newer.SessionID)
	}
	if repos[0].SessionCount != 2 {
		t.Errorf("SessionCount = %d, want 2", repos[0].SessionCount)
	}
}

// DiscoverRepos picks the newest live session, preferring the most recently
// modified. A live run sitting behind a pile of archived sessions must still be
// the one reported - the sidecar filter narrows the candidates but must not
// change which session wins.
func TestDiscoverRepos_LiveRunIsFoundAlongsideManyArchived(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repoDir := "/my/proj"

	// Archived sessions: each has been finalized, so each still carries the
	// sidecar its run left behind.
	for range 5 {
		old, _ := liveSession(t, repoDir)
		if err := old.Finalize(); err != nil {
			t.Fatal(err)
		}
	}
	// The live run goes in last, so a naive "first match" would find an
	// archived one only if the sidecar filter were broken.
	sh, _ := liveSession(t, repoDir)
	defer sh.Close()

	root, err := SessionsRoot()
	if err != nil {
		t.Fatal(err)
	}
	repos, err := DiscoverRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("got %d repos, want 1", len(repos))
	}
	if repos[0].ActiveSessionID != sh.SessionID {
		t.Errorf("ActiveSessionID = %q, want the live run %q", repos[0].ActiveSessionID, sh.SessionID)
	}
}

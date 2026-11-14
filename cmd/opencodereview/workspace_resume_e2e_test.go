// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/session"
)

// End-to-end coverage of workspace --resume. A workspace review has no fixed
// endpoints to seal its input, so a resume may only reuse checkpoints while the
// HEAD commit and the review input (the selected diff, including tracked-file
// deletions) are unchanged. Input identity is computed from the selected,
// parsed/normalized diff, not from a byte-for-byte snapshot of the worktree:
// equivalent inputs such as an untracked file that only gained or lost its
// final newline, or whose line endings changed between LF and CRLF, normalize
// to the same diff and may resume and reuse completed work. A substantive
// content edit changes the diff and is rejected. These tests drive the real CLI
// against real Git repositories and a controlled fake LLM, so the identity the
// CLI computes and the identity the agent persists are both derived from
// actual Git changes rather than hand-built digests.

// workspaceResumeFileLines pushes the change set past the grouping bundle
// threshold (200 changed lines), forcing per-file dispatch. That is what makes
// a partially completed parent deterministic: the fake LLM fails exactly one
// file, so the manifest records that file as unfinished while the others are
// completed and reusable.
const workspaceResumeFileLines = 120

// workspaceResumeFixture is a repository with a saved, partially completed
// workspace session ready to resume. Files a.go, c.go and d.go are modified;
// b.go stays tracked and unchanged unless deletedTracked is set, in which case
// the parent's input also contains b.go's deletion.
type workspaceResumeFixture struct {
	dir      string // symlink-resolved repo path, also the session store key
	srv      *fakeLLM
	parentID string
}

func workspaceResumeBody(name, marker, variant string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package p\n\n// %s\n// variant %s\n", marker, variant)
	for i := 0; i < workspaceResumeFileLines; i++ {
		fmt.Fprintf(&b, "// padding %03d\n", i)
	}
	fmt.Fprintf(&b, "\nfunc %s() int { return 1 }\n", strings.TrimSuffix(name, ".go"))
	return b.String()
}

func writeWorkspaceResumeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// workspaceUntrackedFile is an extra untracked file added to the fixture before
// the parent review. marker must occur only in this file's diff so the fake LLM
// can attribute its requests, exactly like the package-level markers.
type workspaceUntrackedFile struct {
	name   string
	marker string
	body   string
}

// newWorkspaceResumeFixture builds the repo, runs one workspace review in which
// c.go fails permanently and the other two files complete, then clears the
// failure so the returned fixture can be resumed successfully.
func newWorkspaceResumeFixture(t *testing.T, deleteTracked bool) *workspaceResumeFixture {
	t.Helper()
	return newWorkspaceResumeFixtureWithUntracked(t, deleteTracked, nil)
}

// newWorkspaceResumeFixtureWithUntracked is newWorkspaceResumeFixture plus
// untracked files that join the parent review input.
func newWorkspaceResumeFixtureWithUntracked(t *testing.T, deleteTracked bool, untracked []workspaceUntrackedFile) *workspaceResumeFixture {
	t.Helper()
	raw := t.TempDir()
	dir, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}

	retryTestGit(t, dir, "init", "-q", "-b", "main")
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go"} {
		writeWorkspaceResumeFile(t, dir, name, "package p\n\nfunc "+strings.TrimSuffix(name, ".go")+"() int { return 1 }\n")
	}
	retryTestGit(t, dir, "add", ".")
	retryTestGit(t, dir, "commit", "-q", "-m", "base")

	writeWorkspaceResumeFile(t, dir, "a.go", workspaceResumeBody("a.go", "MARKER_ALPHA", "parent"))
	writeWorkspaceResumeFile(t, dir, "c.go", workspaceResumeBody("c.go", "MARKER_GAMMA", "parent"))
	writeWorkspaceResumeFile(t, dir, "d.go", workspaceResumeBody("d.go", "MARKER_DELTA", "parent"))
	if deleteTracked {
		if err := os.Remove(filepath.Join(dir, "b.go")); err != nil {
			t.Fatalf("delete b.go: %v", err)
		}
	}

	for _, extra := range untracked {
		writeWorkspaceResumeFile(t, dir, extra.name, extra.body)
	}

	srv := newFakeLLM()
	for _, extra := range untracked {
		srv.markers[extra.marker] = extra.name
	}
	srv.hardFail["c.go"] = true
	startFakeLLM(t, srv)

	out, errOut, err := runWorkspaceResumeReview(t, dir, "")
	if err != nil {
		t.Fatalf("partially failed review must exit 0: %v\nstdout: %s\nstderr: %s", err, out, errOut)
	}
	sessions, err := session.ListSessions(dir)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) == 0 {
		t.Fatalf("partial review persisted no session\nstdout: %s\nstderr: %s", out, errOut)
	}
	parent := sessions[0]
	if parent.FailedFiles != 1 {
		t.Fatalf("parent failed_files = %d, want 1 (summary: %+v)", parent.FailedFiles, parent)
	}

	srv.mu.Lock()
	delete(srv.hardFail, "c.go")
	srv.mu.Unlock()

	return &workspaceResumeFixture{dir: dir, srv: srv, parentID: parent.SessionID}
}

// runWorkspaceResumeReview drives a workspace review through the CLI. An empty
// resumeID starts a fresh run; otherwise it resumes that session.
func runWorkspaceResumeReview(t *testing.T, repoDir, resumeID string) (string, string, error) {
	t.Helper()
	args := []string{
		"--repo", repoDir,
		"--format", "json",
		"--audience", "agent",
		"--concurrency", "1",
	}
	if resumeID != "" {
		args = append(args, "--resume", resumeID)
	}
	var err error
	var out string
	errOut := captureStderr(t, func() {
		out = captureStdout(t, func() { err = runReview(args) })
	})
	return out, errOut, err
}

// assertWorkspaceResumeRejected checks that a resume was refused before a child
// session or LLM request was made: the CLI must return an error mentioning the
// rejected resume, no session file may appear, and the fake LLM must not see a
// new request.
func assertWorkspaceResumeRejected(t *testing.T, f *workspaceResumeFixture, wantErrSubstring string) {
	t.Helper()
	before, err := session.ListSessions(f.dir)
	if err != nil {
		t.Fatalf("list sessions before: %v", err)
	}
	beforeAttempts := f.srv.attemptCounts()

	out, errOut, err := runWorkspaceResumeReview(t, f.dir, f.parentID)
	if err == nil {
		t.Fatalf("resume must be rejected\nstdout: %s\nstderr: %s", out, errOut)
	}
	if !strings.Contains(err.Error(), "resume rejected") {
		t.Fatalf("error %q does not look like a resume rejection\nstderr: %s", err, errOut)
	}
	if wantErrSubstring != "" && !strings.Contains(err.Error(), wantErrSubstring) {
		t.Fatalf("error %q does not mention %q", err, wantErrSubstring)
	}

	after, err := session.ListSessions(f.dir)
	if err != nil {
		t.Fatalf("list sessions after: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("rejected resume persisted a child session: %d -> %d sessions", len(before), len(after))
	}
	// Compare complete per-file attempt counts, not just the number of file
	// keys: a second request for an already-seen file must fail this check.
	if got := f.srv.attemptCounts(); !maps.Equal(got, beforeAttempts) {
		t.Fatalf("rejected resume issued an LLM request: attempts %v -> %v", beforeAttempts, got)
	}
}

// workspaceResumeChild returns the session that resumed parentID. Sessions can
// share a start timestamp, so the newest entry is not a reliable identifier.
func workspaceResumeChild(t *testing.T, sessions []session.Summary, parentID string) session.Summary {
	t.Helper()
	for _, s := range sessions {
		if s.ResumedFrom == parentID {
			return s
		}
	}
	t.Fatalf("no child session resumed %q (sessions: %+v)", parentID, sessions)
	return session.Summary{}
}

func TestWorkspaceResumeE2E_UnchangedInputReusesCompletedAndRetriesUnfinished(t *testing.T) {
	f := newWorkspaceResumeFixture(t, false)
	before := f.srv.attemptCounts()

	out, errOut, err := runWorkspaceResumeReview(t, f.dir, f.parentID)
	if err != nil {
		t.Fatalf("unchanged resume must succeed: %v\nstdout: %s\nstderr: %s", err, out, errOut)
	}

	sessions, err := session.ListSessions(f.dir)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want parent + child", len(sessions))
	}
	child := workspaceResumeChild(t, sessions, f.parentID)
	if child.FailedFiles != 0 || child.ReusedFiles != 2 || child.CompletedFiles != 1 {
		t.Errorf("child coverage = reused %d, completed %d, failed %d; want 2/1/0",
			child.ReusedFiles, child.CompletedFiles, child.FailedFiles)
	}
	if child.TotalComments != 0 {
		t.Errorf("child total_comments = %d, want 0 from the fake task_done replies", child.TotalComments)
	}

	// The completed files a.go and d.go must not be sent again; only the failed
	// c.go is retried (plan round + main round).
	after := f.srv.attemptCounts()
	if after["a.go"] != before["a.go"] || after["d.go"] != before["d.go"] {
		t.Errorf("completed files were re-requested: before %v, after %v", before, after)
	}
	if after["c.go"] <= before["c.go"] {
		t.Errorf("failed c.go was not retried: before %v, after %v", before, after)
	}
}

func TestWorkspaceResumeE2E_RejectsInputChanges(t *testing.T) {
	t.Run("edited selected file", func(t *testing.T) {
		f := newWorkspaceResumeFixture(t, false)
		writeWorkspaceResumeFile(t, f.dir, "a.go", workspaceResumeBody("a.go", "MARKER_ALPHA", "edited-after-parent"))
		assertWorkspaceResumeRejected(t, f, "workspace changed")
	})

	t.Run("deleted unchanged tracked file", func(t *testing.T) {
		f := newWorkspaceResumeFixture(t, false)
		if err := os.Remove(filepath.Join(f.dir, "b.go")); err != nil {
			t.Fatalf("delete b.go: %v", err)
		}
		assertWorkspaceResumeRejected(t, f, "workspace changed")
	})

	t.Run("changed HEAD", func(t *testing.T) {
		f := newWorkspaceResumeFixture(t, false)
		retryTestGit(t, f.dir, "commit", "-q", "--allow-empty", "-m", "move HEAD")
		assertWorkspaceResumeRejected(t, f, "workspace base commit changed")
	})

	t.Run("restored file deleted in parent input", func(t *testing.T) {
		f := newWorkspaceResumeFixture(t, true)
		retryTestGit(t, f.dir, "checkout", "--", "b.go")
		assertWorkspaceResumeRejected(t, f, "workspace changed")

		// Returning the workspace exactly to the parent's input is acceptable
		// again: re-deleting b.go restores the reviewed diff.
		if err := os.Remove(filepath.Join(f.dir, "b.go")); err != nil {
			t.Fatalf("re-delete b.go: %v", err)
		}
		out, errOut, err := runWorkspaceResumeReview(t, f.dir, f.parentID)
		if err != nil {
			t.Fatalf("exact parent input must resume: %v\nstdout: %s\nstderr: %s", err, out, errOut)
		}
		sessions, listErr := session.ListSessions(f.dir)
		if listErr != nil {
			t.Fatalf("list sessions: %v", listErr)
		}
		if len(sessions) != 2 {
			t.Fatalf("got %d sessions, want parent + child", len(sessions))
		}
		child := workspaceResumeChild(t, sessions, f.parentID)
		if child.FailedFiles != 0 || child.ReusedFiles != 2 || child.CompletedFiles != 1 {
			t.Errorf("child coverage = reused %d, completed %d, failed %d; want 2/1/0",
				child.ReusedFiles, child.CompletedFiles, child.FailedFiles)
		}
	})
}

// TestWorkspaceResumeE2E_NormalizedInputBoundary pins the documented contract:
// workspace resume identity is the selected, parsed/normalized diff, not a
// byte-for-byte snapshot of the worktree. Untracked files are synthesized into
// a patch from their content (untrackedFileDiffs), and the parser strips a
// trailing CR from every diff line, so adding or removing a final newline, or
// switching between LF and CRLF, normalizes to the same diff and may reuse
// completed work. A substantive edit still rejects.
func TestWorkspaceResumeE2E_NormalizedInputBoundary(t *testing.T) {
	body := workspaceResumeBody("u.go", "MARKER_EPSILON", "untracked")
	withoutFinalLF := strings.TrimSuffix(body, "\n")
	withFinalLF := body
	crlf := strings.ReplaceAll(body, "\n", "\r\n")

	// assertResumesAndReuses resumes the parent after mutate and asserts that
	// the normalized-equivalent input is accepted: a.go, d.go and the untracked
	// file u.go are reused, and only the failed c.go is retried.
	assertResumesAndReuses := func(t *testing.T, parentBody string, mutate func(t *testing.T, dir string)) {
		t.Helper()
		f := newWorkspaceResumeFixtureWithUntracked(t, false, []workspaceUntrackedFile{{
			name: "u.go", marker: "MARKER_EPSILON", body: parentBody,
		}})
		mutate(t, f.dir)
		before := f.srv.attemptCounts()

		out, errOut, err := runWorkspaceResumeReview(t, f.dir, f.parentID)
		if err != nil {
			t.Fatalf("normalized-equivalent resume must succeed: %v\nstdout: %s\nstderr: %s", err, out, errOut)
		}
		sessions, listErr := session.ListSessions(f.dir)
		if listErr != nil {
			t.Fatalf("list sessions: %v", listErr)
		}
		if len(sessions) != 2 {
			t.Fatalf("got %d sessions, want parent + child", len(sessions))
		}
		child := workspaceResumeChild(t, sessions, f.parentID)
		if child.FailedFiles != 0 || child.ReusedFiles != 3 || child.CompletedFiles != 1 {
			t.Errorf("child coverage = reused %d, completed %d, failed %d; want 3/1/0",
				child.ReusedFiles, child.CompletedFiles, child.FailedFiles)
		}

		after := f.srv.attemptCounts()
		for _, name := range []string{"a.go", "d.go", "u.go"} {
			if after[name] != before[name] {
				t.Errorf("completed file %s was re-requested: before %v, after %v", name, before, after)
			}
		}
		if after["c.go"] <= before["c.go"] {
			t.Errorf("failed c.go was not retried: before %v, after %v", before, after)
		}
	}

	t.Run("untracked file gains its final newline", func(t *testing.T) {
		assertResumesAndReuses(t, withoutFinalLF, func(t *testing.T, dir string) {
			writeWorkspaceResumeFile(t, dir, "u.go", withFinalLF)
		})
	})

	t.Run("untracked file loses its final newline", func(t *testing.T) {
		assertResumesAndReuses(t, withFinalLF, func(t *testing.T, dir string) {
			writeWorkspaceResumeFile(t, dir, "u.go", withoutFinalLF)
		})
	})

	t.Run("untracked file converts LF to CRLF", func(t *testing.T) {
		assertResumesAndReuses(t, withFinalLF, func(t *testing.T, dir string) {
			writeWorkspaceResumeFile(t, dir, "u.go", crlf)
		})
	})

	t.Run("untracked file converts CRLF to LF", func(t *testing.T) {
		assertResumesAndReuses(t, crlf, func(t *testing.T, dir string) {
			writeWorkspaceResumeFile(t, dir, "u.go", withFinalLF)
		})
	})

	t.Run("substantive edit of untracked file still rejects", func(t *testing.T) {
		f := newWorkspaceResumeFixtureWithUntracked(t, false, []workspaceUntrackedFile{{
			name: "u.go", marker: "MARKER_EPSILON", body: withFinalLF,
		}})
		writeWorkspaceResumeFile(t, f.dir, "u.go", workspaceResumeBody("u.go", "MARKER_EPSILON", "edited-after-parent"))
		assertWorkspaceResumeRejected(t, f, "workspace changed")
	})
}

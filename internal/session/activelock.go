// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"os"
	"strings"
)

// A session is "in flight" for exactly as long as the process that opened it
// holds an exclusive OS lock on a sidecar file. The lock, not the absence of a
// session_end record, is the liveness signal: a run that is killed -9, panics,
// or loses its terminal never writes session_end, so the record stream alone
// cannot tell a crashed run from one that is still working, and a viewer that
// guesses from the record stream has to guess from elapsed time instead.
//
// The sidecar outlives the run on purpose. Closing the descriptor releases the
// lock, but the file itself stays on disk so a later probe can still open it
// and observe the free state; deleting it would turn "this run is over" into
// "this file is missing", which is indistinguishable from a session that was
// never on this machine.

// activeLockSuffix is appended to a session id to name its sidecar. The
// separator cannot appear in a session id (those are UUIDs) and the ".lock"
// name cannot collide with a session file, which always ends ".jsonl".
const activeLockSuffix = ".lock"

// activeLockPath returns the sidecar path guarding one session file. Callers
// holding only a session id resolve it against the repo's session directory.
func activeLockPath(sessionFilePath string) string {
	return strings.TrimSuffix(sessionFilePath, ".jsonl") + activeLockSuffix
}

// holdActiveLock takes the exclusive lock for sessionID and returns the handle
// that keeps it. The caller must Close it when the run ends; the underlying OS
// lock is released by Close regardless of whether the run reached session_end,
// so a crash cannot leave a session permanently pinned as running.
//
// A failure to lock is not fatal. It costs the viewer its live status for this
// one run, which is strictly less than aborting a review, so the error is
// returned and the caller decides.
func holdActiveLock(sessionFilePath string) (*activeLock, error) {
	path := activeLockPath(sessionFilePath)
	// 0600 for the same reason the session JSONL uses it: records and the
	// sidecar name a repository path and a run, and this is a per-user tree.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockExclusive(f); err != nil {
		f.Close()
		return nil, err
	}
	return &activeLock{file: f, path: path}, nil
}

// SessionIsActive reports whether a run currently holds the lock for the
// session file at sessionFilePath.
//
// A missing sidecar means no run has ever claimed this session on this machine,
// which is the normal state for a session written by an older build or copied
// in from elsewhere. It is reported as inactive rather than as an error, so a
// viewer degrades to "aborted" instead of refusing to render.
func SessionIsActive(sessionFilePath string) bool {
	f, err := os.Open(activeLockPath(sessionFilePath))
	if err != nil {
		return false
	}
	defer f.Close()
	// A successful lock means nobody owns the run, i.e. it is finished. A
	// failure means the owning process is still alive. Any other error (an
	// unsupported filesystem, a permission problem) is treated as unknown, and
	// unknown must not read as "running": a stale "running" badge is a lie the
	// user cannot dismiss, while a missing one self-corrects on the next poll.
	err = lockExclusive(f)
	if err == nil {
		unlockFile(f)
		return false
	}
	return isLockContended(err)
}

// activeLock keeps a session's liveness sidecar locked for the run's lifetime.
type activeLock struct {
	file *os.File
	path string
}

// Close releases the lock. The sidecar is intentionally left in place; see the
// note on activeLockSuffix.
//
// Idempotent: a session's exit paths are not all exclusive - WriteSessionEnd
// closes on both its success and its marshal-failure branch, and a deferred
// flushAndClose may follow either - so a second Close must be a no-op rather
// than an "already closed" error nobody is positioned to handle.
func (l *activeLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	f := l.file
	l.file = nil
	unlockFile(f)
	return f.Close()
}
